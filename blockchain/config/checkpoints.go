// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// TrustedCheckpoint represents a hardcoded checkpoint verified by developers
type TrustedCheckpoint struct {
	Height uint64 `json:"height"`
	Hash   string `json:"hash"`
}

// MainnetCheckpoints is the list of trusted checkpoints for mainnet
// These are verified by developers and embedded in the binary
// When adding new checkpoints, ensure the hash matches the canonical chain
var MainnetCheckpoints = []TrustedCheckpoint{}

// NextCheckpoint returns the next checkpoint after the given height
// Returns nil if no checkpoint is available beyond the given height
func NextCheckpoint(localHeight uint64, checkpoints []TrustedCheckpoint) *TrustedCheckpoint {
	for i := range checkpoints {
		if checkpoints[i].Height > localHeight {
			return &checkpoints[i]
		}
	}
	return nil
}

// LatestCheckpoint returns the latest checkpoint at or before the given height
// Returns nil if no checkpoint exists at or before the given height
func LatestCheckpoint(height uint64, checkpoints []TrustedCheckpoint) *TrustedCheckpoint {
	var latest *TrustedCheckpoint
	for i := range checkpoints {
		cp := &checkpoints[i]
		if cp.Height <= height {
			latest = cp
		} else {
			break
		}
	}
	return latest
}

// CheckpointHashes returns a map of height -> hash for quick lookup
func CheckpointHashes(checkpoints []TrustedCheckpoint) map[uint64]string {
	hashes := make(map[uint64]string, len(checkpoints))
	for _, cp := range checkpoints {
		hashes[cp.Height] = cp.Hash
	}
	return hashes
}

var (
	externalCheckpoints     []TrustedCheckpoint
	externalCheckpointsOnce sync.Once
	externalCheckpointsErr  error
)

// LoadExternalCheckpoints loads checkpoints from NOGO_CHECKPOINTS_DIR environment variable
// The directory should contain a checkpoints.json file with the same format as TrustedCheckpoint
func LoadExternalCheckpoints() ([]TrustedCheckpoint, error) {
	externalCheckpointsOnce.Do(func() {
		dir := os.Getenv("NOGO_CHECKPOINTS_DIR")
		if dir == "" {
			externalCheckpoints = MainnetCheckpoints
			return
		}

		filePath := filepath.Join(dir, "checkpoints.json")
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			externalCheckpointsErr = fmt.Errorf("read checkpoints file %s: %w", filePath, readErr)
			externalCheckpoints = MainnetCheckpoints
			return
		}

		var checkpoints []TrustedCheckpoint
		if unmarshalErr := json.Unmarshal(data, &checkpoints); unmarshalErr != nil {
			externalCheckpointsErr = fmt.Errorf("parse checkpoints file %s: %w", filePath, unmarshalErr)
			externalCheckpoints = MainnetCheckpoints
			return
		}

		merged := make([]TrustedCheckpoint, 0, len(MainnetCheckpoints)+len(checkpoints))
		merged = append(merged, MainnetCheckpoints...)
		merged = append(merged, checkpoints...)
		externalCheckpoints = merged
	})

	return externalCheckpoints, externalCheckpointsErr
}

// ActiveCheckpoints returns the effective checkpoint list
// Uses external checkpoints if available, otherwise falls back to hardcoded
func ActiveCheckpoints() []TrustedCheckpoint {
	checkpoints, _ := LoadExternalCheckpoints()
	return checkpoints
}
