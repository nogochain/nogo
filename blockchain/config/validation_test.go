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
	"testing"
)

func TestSyncConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    SyncConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid config",
			config: SyncConfig{
				BatchSize:              500,
				MaxConcurrentDownloads: 8,
				MemoryThresholdMB:      1500,
			},
			wantError: false,
		},
		{
			name: "batch size too small",
			config: SyncConfig{
				BatchSize:              0,
				MaxConcurrentDownloads: 8,
				MemoryThresholdMB:      1500,
			},
			wantError: true,
			errorMsg:  "batchSize must be > 0",
		},
		{
			name: "batch size too large",
			config: SyncConfig{
				BatchSize:              2001,
				MaxConcurrentDownloads: 8,
				MemoryThresholdMB:      1500,
			},
			wantError: true,
			errorMsg:  "batchSize must be <= 2000",
		},
		{
			name: "max concurrent too small",
			config: SyncConfig{
				BatchSize:              500,
				MaxConcurrentDownloads: 0,
				MemoryThresholdMB:      1500,
			},
			wantError: true,
			errorMsg:  "maxConcurrentDownloads must be > 0",
		},
		{
			name: "max concurrent too large",
			config: SyncConfig{
				BatchSize:              500,
				MaxConcurrentDownloads: 33,
				MemoryThresholdMB:      1500,
			},
			wantError: true,
			errorMsg:  "maxConcurrentDownloads must be <= 32",
		},
		{
			name: "memory threshold zero",
			config: SyncConfig{
				BatchSize:              500,
				MaxConcurrentDownloads: 8,
				MemoryThresholdMB:      0,
			},
			wantError: true,
			errorMsg:  "memoryThresholdMB must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestMempoolConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    MempoolConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid config",
			config: MempoolConfig{
				MaxTransactions: 10000,
				MaxMemoryMB:     100,
			},
			wantError: false,
		},
		{
			name: "max transactions zero",
			config: MempoolConfig{
				MaxTransactions: 0,
				MaxMemoryMB:     100,
			},
			wantError: true,
			errorMsg:  "maxTransactions must be > 0",
		},
		{
			name: "max memory zero",
			config: MempoolConfig{
				MaxTransactions: 10000,
				MaxMemoryMB:     0,
			},
			wantError: true,
			errorMsg:  "maxMemoryMB must be > 0",
		},
		{
			name: "max memory too large",
			config: MempoolConfig{
				MaxTransactions: 10000,
				MaxMemoryMB:     10001,
			},
			wantError: true,
			errorMsg:  "maxMemoryMB must be <= 10000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestConsensusParams_Validate(t *testing.T) {
	tests := []struct {
		name      string
		params    ConsensusParams
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid params",
			params: ConsensusParams{
				ChainID:                        1,
				BlockTimeTargetSeconds:         15,
				DifficultyAdjustmentInterval:   1,
				MaxBlockTimeDriftSeconds:       900,
				MinDifficulty:                  1,
				MaxDifficulty:                  4294967295,
				MinDifficultyBits:              1,
				MaxDifficultyBits:              255,
				MaxDifficultyChangePercent:     20,
				MedianTimePastWindow:           11,
				GenesisDifficultyBits:          18,
				MonetaryPolicy: MonetaryPolicy{
					InitialBlockReward:     800000000,
					AnnualReductionPercent: 9,
					MinimumBlockReward:     10000000,
					MinerRewardShare:       50,
					CommunityFundShare:     30,
					GenesisShare:           10,
					IntegrityPoolShare:     10,
				},
			},
			wantError: false,
		},
		{
			name: "chain id zero",
			params: ConsensusParams{
				ChainID:                        0,
				BlockTimeTargetSeconds:         15,
				DifficultyAdjustmentInterval:   1,
				MaxBlockTimeDriftSeconds:       900,
				MinDifficulty:                  1,
				MaxDifficulty:                  4294967295,
				MinDifficultyBits:              1,
				MaxDifficultyBits:              255,
				MaxDifficultyChangePercent:     20,
				MedianTimePastWindow:           11,
				GenesisDifficultyBits:          18,
				MonetaryPolicy: MonetaryPolicy{
					InitialBlockReward:     800000000,
					AnnualReductionPercent: 9,
					MinimumBlockReward:     10000000,
					MinerRewardShare:       50,
					CommunityFundShare:     30,
					GenesisShare:           10,
					IntegrityPoolShare:     10,
				},
			},
			wantError: true,
			errorMsg:  "chainId must be > 0",
		},
		{
			name: "block time zero",
			params: ConsensusParams{
				ChainID:                        1,
				BlockTimeTargetSeconds:         0,
				DifficultyAdjustmentInterval:   1,
				MaxBlockTimeDriftSeconds:       900,
				MinDifficulty:                  1,
				MaxDifficulty:                  4294967295,
				MinDifficultyBits:              1,
				MaxDifficultyBits:              255,
				MaxDifficultyChangePercent:     20,
				MedianTimePastWindow:           11,
				GenesisDifficultyBits:          18,
				MonetaryPolicy: MonetaryPolicy{
					InitialBlockReward:     800000000,
					AnnualReductionPercent: 9,
					MinimumBlockReward:     10000000,
					MinerRewardShare:       50,
					CommunityFundShare:     30,
					GenesisShare:           10,
					IntegrityPoolShare:     10,
				},
			},
			wantError: true,
			errorMsg:  "blockTimeTargetSeconds must be > 0",
		},
		{
			name: "min difficulty exceeds max",
			params: ConsensusParams{
				ChainID:                        1,
				BlockTimeTargetSeconds:         15,
				DifficultyAdjustmentInterval:   1,
				MaxBlockTimeDriftSeconds:       900,
				MinDifficulty:                  100,
				MaxDifficulty:                  50,
				MinDifficultyBits:              1,
				MaxDifficultyBits:              255,
				MaxDifficultyChangePercent:     20,
				MedianTimePastWindow:           11,
				GenesisDifficultyBits:          18,
				MonetaryPolicy: MonetaryPolicy{
					InitialBlockReward:     800000000,
					AnnualReductionPercent: 9,
					MinimumBlockReward:     10000000,
					MinerRewardShare:       50,
					CommunityFundShare:     30,
					GenesisShare:           10,
					IntegrityPoolShare:     10,
				},
			},
			wantError: true,
			errorMsg:  "minDifficulty cannot exceed maxDifficulty",
		},
		{
			name: "difficulty change percent too large",
			params: ConsensusParams{
				ChainID:                        1,
				BlockTimeTargetSeconds:         15,
				DifficultyAdjustmentInterval:   1,
				MaxBlockTimeDriftSeconds:       900,
				MinDifficulty:                  1,
				MaxDifficulty:                  4294967295,
				MinDifficultyBits:              1,
				MaxDifficultyBits:              255,
				MaxDifficultyChangePercent:     101,
				MedianTimePastWindow:           11,
				GenesisDifficultyBits:          18,
				MonetaryPolicy: MonetaryPolicy{
					InitialBlockReward:     800000000,
					AnnualReductionPercent: 9,
					MinimumBlockReward:     10000000,
					MinerRewardShare:       50,
					CommunityFundShare:     30,
					GenesisShare:           10,
					IntegrityPoolShare:     10,
				},
			},
			wantError: true,
			errorMsg:  "maxDifficultyChangePercent must be between 1 and 100",
		},
		{
			name: "median time window even",
			params: ConsensusParams{
				ChainID:                        1,
				BlockTimeTargetSeconds:         15,
				DifficultyAdjustmentInterval:   1,
				MaxBlockTimeDriftSeconds:       900,
				MinDifficulty:                  1,
				MaxDifficulty:                  4294967295,
				MinDifficultyBits:              1,
				MaxDifficultyBits:              255,
				MaxDifficultyChangePercent:     20,
				MedianTimePastWindow:           10,
				GenesisDifficultyBits:          18,
				MonetaryPolicy: MonetaryPolicy{
					InitialBlockReward:     800000000,
					AnnualReductionPercent: 9,
					MinimumBlockReward:     10000000,
					MinerRewardShare:       50,
					CommunityFundShare:     30,
					GenesisShare:           10,
					IntegrityPoolShare:     10,
				},
			},
			wantError: true,
			errorMsg:  "medianTimePastWindow must be positive and odd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Validate()
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestConfig_Validate_Integration(t *testing.T) {
	validSyncConfig := SyncConfig{
		BatchSize:              500,
		MaxConcurrentDownloads: 8,
		MemoryThresholdMB:      1500,
	}

	validMempoolConfig := MempoolConfig{
		MaxTransactions: 10000,
		MaxMemoryMB:     100,
	}

	t.Run("valid sync config", func(t *testing.T) {
		err := validSyncConfig.Validate()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid mempool config", func(t *testing.T) {
		err := validMempoolConfig.Validate()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid sync config propagation", func(t *testing.T) {
		invalidSync := SyncConfig{
			BatchSize:              0,
			MaxConcurrentDownloads: 8,
			MemoryThresholdMB:      1500,
		}
		err := invalidSync.Validate()
		if err == nil {
			t.Errorf("expected error, got nil")
		}
	})

	t.Run("invalid mempool config propagation", func(t *testing.T) {
		invalidMempool := MempoolConfig{
			MaxTransactions: 0,
			MaxMemoryMB:     100,
		}
		err := invalidMempool.Validate()
		if err == nil {
			t.Errorf("expected error, got nil")
		}
	})
}
