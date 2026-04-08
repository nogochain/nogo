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
	"errors"
)

// Validate validates sync configuration
func (s *SyncConfig) Validate() error {
	if s.BatchSize <= 0 {
		return errors.New("batchSize must be > 0")
	}

	if s.BatchSize > 2000 {
		return errors.New("batchSize must be <= 2000")
	}

	if s.MaxConcurrentDownloads <= 0 {
		return errors.New("maxConcurrentDownloads must be > 0")
	}

	if s.MaxConcurrentDownloads > 32 {
		return errors.New("maxConcurrentDownloads must be <= 32")
	}

	if s.MemoryThresholdMB == 0 {
		return errors.New("memoryThresholdMB must be > 0")
	}

	if s.MaxRollbackDepth < 0 {
		return errors.New("maxRollbackDepth must be >= 0")
	}

	return nil
}

// Validate validates mempool configuration
func (m *MempoolConfig) Validate() error {
	if m.MaxTransactions <= 0 {
		return errors.New("maxTransactions must be > 0")
	}

	if m.MaxMemoryMB == 0 {
		return errors.New("maxMemoryMB must be > 0")
	}

	if m.MaxMemoryMB > 10000 {
		return errors.New("maxMemoryMB must be <= 10000")
	}

	return nil
}

// Validate validates mining configuration
func (m *MiningConfig) Validate() error {
	if m.MaxTxPerBlock <= 0 {
		return errors.New("maxTxPerBlock must be > 0")
	}

	if m.MineInterval <= 0 {
		return errors.New("mineInterval must be > 0")
	}

	return nil
}
