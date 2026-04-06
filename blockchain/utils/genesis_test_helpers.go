package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type testConsensusJSON struct {
	DifficultyEnable               bool   `json:"difficultyEnable"`
	DifficultyTargetMs             int64  `json:"difficultyTargetMs"`
	DifficultyWindow               int    `json:"difficultyWindow"`
	DifficultyMaxStepBits          uint32 `json:"difficultyMaxStepBits"`
	DifficultyMinBits              uint32 `json:"difficultyMinBits"`
	DifficultyMaxBits              uint32 `json:"difficultyMaxBits"`
	GenesisDifficultyBits          uint32 `json:"genesisDifficultyBits"`
	MedianTimePastWindow           int    `json:"medianTimePastWindow"`
	MaxTimeDrift                   int64  `json:"maxTimeDrift"`
	MaxBlockSize                   uint64 `json:"maxBlockSize"`
	MerkleEnable                   bool   `json:"merkleEnable"`
	MerkleActivationHeight         uint64 `json:"merkleActivationHeight"`
	BinaryEncodingEnable           bool   `json:"binaryEncodingEnable"`
	BinaryEncodingActivationHeight uint64 `json:"binaryEncodingActivationHeight"`
}

type testGenesisJSON struct {
	Network             string            `json:"network"`
	ChainID             uint64            `json:"chainId"`
	Timestamp           int64             `json:"timestamp"`
	GenesisMinerAddress string            `json:"genesisMinerAddress"`
	InitialSupply       string            `json:"initialSupply"`
	GenesisMessage      string            `json:"genesisMessage,omitempty"`
	MonetaryPolicy      testMonetaryJSON  `json:"monetaryPolicy"`
	ConsensusParams     testConsensusJSON `json:"consensusParams"`
}

type testMonetaryJSON struct {
	InitialBlockReward string `json:"initialBlockReward"`
	HalvingInterval    uint64 `json:"halvingInterval"`
	MinerFeeShare      uint8  `json:"minerFeeShare"`
	TailEmission       string `json:"tailEmission"`
	MinimumBlockReward string `json:"minimumBlockReward,omitempty"`
}

func defaultTestConsensusJSON() testConsensusJSON {
	return testConsensusJSON{
		DifficultyEnable:               false,
		DifficultyTargetMs:             15000,
		DifficultyWindow:               20,
		DifficultyMaxStepBits:          1,
		DifficultyMinBits:              1,
		DifficultyMaxBits:              255,
		GenesisDifficultyBits:          1,
		MedianTimePastWindow:           11,
		MaxTimeDrift:                   7200,
		MaxBlockSize:                   1_000_000,
		MerkleEnable:                   false,
		MerkleActivationHeight:         0,
		BinaryEncodingEnable:           false,
		BinaryEncodingActivationHeight: 0,
	}
}

func defaultTestMonetaryJSON() testMonetaryJSON {
	return testMonetaryJSON{
		InitialBlockReward: "100000000", // 1 NOGO in wei (must be > minimum)
		HalvingInterval:    210000,
		MinerFeeShare:      100,
		TailEmission:       "0",
		MinimumBlockReward: "10000000", // 0.1 NOGO in wei
	}
}

func writeTestGenesisFile(t *testing.T, dir string, chainID uint64, minerAddr string, supply uint64, consensus testConsensusJSON) string {
	t.Helper()

	cfg := testGenesisJSON{
		Network:             "test",
		ChainID:             chainID,
		Timestamp:           1700000000,
		GenesisMinerAddress: minerAddr,
		InitialSupply:       fmt.Sprintf("%d", supply),
		MonetaryPolicy:      defaultTestMonetaryJSON(),
		ConsensusParams:     consensus,
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal genesis: %v", err)
	}
	path := filepath.Join(dir, "genesis.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write genesis: %v", err)
	}
	return path
}
