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

package network

import (
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/metrics"
	"github.com/nogochain/nogo/blockchain/utils"
)

func TestBlockDownloader_ConfigConsistency(t *testing.T) {
	pm := &mockPeerAPI{}
	bc := &mockBlockchain{height: 100}
	validator := &mockValidator{}
	m := &metrics.Metrics{}

	syncConfig := config.SyncConfig{
		BatchSize:              500,
		MaxConcurrentDownloads: 8,
		MemoryThresholdMB:      1500,
	}

	downloader := NewBlockDownloader(pm, bc, validator, m, syncConfig)
	if downloader == nil {
		t.Fatal("expected downloader to be created")
	}

	cfg := downloader.GetConfig()

	if cfg.BatchSize != 500 {
		t.Errorf("expected batch size 500, got %d", cfg.BatchSize)
	}

	if cfg.MaxConcurrent != 8 {
		t.Errorf("expected max concurrent 8, got %d", cfg.MaxConcurrent)
	}

	expectedMemory := uint64(1500 * 1024 * 1024)
	if cfg.MemoryThreshold != expectedMemory {
		t.Errorf("expected memory threshold %d, got %d", expectedMemory, cfg.MemoryThreshold)
	}
}

func TestBlockDownloader_ConfigValidation(t *testing.T) {
	pm := &mockPeerAPI{}
	bc := &mockBlockchain{height: 100}
	validator := &mockValidator{}
	m := &metrics.Metrics{}

	tests := []struct {
		name       string
		syncConfig config.SyncConfig
		wantBatch  int
		wantConc   int
		wantMem    uint64
	}{
		{
			name: "default values when zero",
			syncConfig: config.SyncConfig{
				BatchSize:              0,
				MaxConcurrentDownloads: 0,
				MemoryThresholdMB:      0,
			},
			wantBatch: 500,
			wantConc:  8,
			wantMem:   1500 * 1024 * 1024,
		},
		{
			name: "custom values",
			syncConfig: config.SyncConfig{
				BatchSize:              1000,
				MaxConcurrentDownloads: 16,
				MemoryThresholdMB:      2000,
			},
			wantBatch: 1000,
			wantConc:  16,
			wantMem:   2000 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			downloader := NewBlockDownloader(pm, bc, validator, m, tt.syncConfig)
			cfg := downloader.GetConfig()

			if cfg.BatchSize != tt.wantBatch {
				t.Errorf("expected batch size %d, got %d", tt.wantBatch, cfg.BatchSize)
			}

			if cfg.MaxConcurrent != tt.wantConc {
				t.Errorf("expected max concurrent %d, got %d", tt.wantConc, cfg.MaxConcurrent)
			}

			if cfg.MemoryThreshold != tt.wantMem {
				t.Errorf("expected memory threshold %d, got %d", tt.wantMem, cfg.MemoryThreshold)
			}
		})
	}
}

func TestSyncLoop_ConfigConsistency(t *testing.T) {
	pm := &mockPeerAPI{}
	bc := &mockBlockchain{height: 100}
	miner := &mockMiner{}
	m := &metrics.Metrics{}
	orphanPool := utils.NewOrphanPool(100, time.Hour)
	validator := createTestValidator(m)

	syncConfig := config.SyncConfig{
		BatchSize:              500,
		MaxConcurrentDownloads: 8,
		MemoryThresholdMB:      1500,
	}

	syncLoop := NewSyncLoop(pm, bc, miner, m, orphanPool, validator, syncConfig)
	if syncLoop == nil {
		t.Fatal("expected SyncLoop to be created")
	}

	cfg := syncLoop.downloader.GetConfig()

	if cfg.BatchSize != 500 {
		t.Errorf("expected batch size 500, got %d", cfg.BatchSize)
	}

	if cfg.MaxConcurrent != 8 {
		t.Errorf("expected max concurrent 8, got %d", cfg.MaxConcurrent)
	}
}

func createTestValidator(m *metrics.Metrics) *consensus.BlockValidator {
	consensusParams := config.GetConsensusParams()
	return consensus.NewBlockValidator(consensusParams, 1, m)
}
