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
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package consensus

import (
	"math/big"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

func TestBlockTimeMaxDrift_Value(t *testing.T) {
	// Security: verify BlockTimeMaxDrift is hardened to 15 minutes (900 seconds)
	if BlockTimeMaxDrift != 900 {
		t.Errorf("BlockTimeMaxDrift should be 900 (15 min), got %d", BlockTimeMaxDrift)
	}
}

func TestDifficultyTolerancePercent_Value(t *testing.T) {
	if DifficultyTolerancePercent != 50 {
		t.Errorf("DifficultyTolerancePercent should be 50, got %d", DifficultyTolerancePercent)
	}
}

func TestNewBlockValidator(t *testing.T) {
	params := core.ConsensusParams{
		BlockTimeTargetSeconds:     17,
		MaxDifficultyChangePercent: 100,
	}

	validator := NewBlockValidator(params)
	if validator == nil {
		t.Fatal("NewBlockValidator returned nil")
	}

	if validator.consensus.BlockTimeTargetSeconds != 17 {
		t.Errorf("Expected BlockTimeTargetSeconds 17, got %d", validator.consensus.BlockTimeTargetSeconds)
	}
}

func TestValidateBlockTimestamp_FutureWithinLimit(t *testing.T) {
	params := core.ConsensusParams{
		BlockTimeTargetSeconds: 17,
	}
	validator := NewBlockValidator(params)

	parentBlock := &core.Block{
		Header: core.BlockHeader{
			TimestampUnix: time.Now().Unix() - 17,
		},
	}

	newBlock := &core.Block{
		Header: core.BlockHeader{
			TimestampUnix: time.Now().Unix() + 60, // 1 minute in future (within 15 min limit)
			PrevHash:      parentBlock.Hash,
		},
	}

	err := validator.ValidateBlockTimestamp(newBlock, parentBlock)
	if err != nil {
		t.Errorf("Timestamp within limit should be valid, got error: %v", err)
	}
}

func TestValidateBlockTimestamp_TooFarInFuture(t *testing.T) {
	params := core.ConsensusParams{
		BlockTimeTargetSeconds: 17,
	}
	validator := NewBlockValidator(params)

	parentBlock := &core.Block{
		Header: core.BlockHeader{
			TimestampUnix: time.Now().Unix() - 17,
		},
	}

	newBlock := &core.Block{
		Header: core.BlockHeader{
			TimestampUnix: time.Now().Unix() + 7200, // 2 hours in future (exceeds 15 min limit)
			PrevHash:      parentBlock.Hash,
		},
	}

	err := validator.ValidateBlockTimestamp(newBlock, parentBlock)
	if err == nil {
		t.Error("Timestamp too far in future should be invalid")
	}
}

func TestValidateBlockTimestamp_BeforeParent(t *testing.T) {
	params := core.ConsensusParams{
		BlockTimeTargetSeconds: 17,
	}
	validator := NewBlockValidator(params)

	parentBlock := &core.Block{
		Header: core.BlockHeader{
			TimestampUnix: time.Now().Unix(),
		},
	}

	newBlock := &core.Block{
		Header: core.BlockHeader{
			TimestampUnix: parentBlock.Header.TimestampUnix - 100, // Before parent
			PrevHash:      parentBlock.Hash,
		},
	}

	err := validator.ValidateBlockTimestamp(newBlock, parentBlock)
	if err == nil {
		t.Error("Timestamp before parent should be invalid")
	}
}

func TestValidateDifficulty_ValidRange(t *testing.T) {
	params := core.ConsensusParams{
		BlockTimeTargetSeconds:     17,
		MinDifficulty:             1,
		MaxDifficultyChangePercent: 100,
	}
	validator := NewBlockValidator(params)

	parentHeader := &nogopowHeaderWrapper{
		Difficulty: big.NewInt(1000000),
		Time:       uint64(time.Now().Unix()),
		Number:     big.NewInt(100),
	}

	currentTime := uint64(time.Now().Unix()) + 20
	newDifficulty := big.NewInt(1100000) // 10% increase (within 2x limit)

	err := validator.ValidateDifficulty(newDifficulty, currentTime, parentHeader)
	if err != nil {
		t.Errorf("Valid difficulty should pass, got error: %v", err)
	}
}

func TestValidateDifficulty_TooLow(t *testing.T) {
	params := core.ConsensusParams{
		BlockTimeTargetSeconds:     17,
		MinDifficulty:             1000,
		MaxDifficultyChangePercent: 100,
	}
	validator := NewBlockValidator(params)

	parentHeader := &nogopowHeaderWrapper{
		Difficulty: big.NewInt(1000000),
		Time:       uint64(time.Now().Unix()),
		Number:     big.NewInt(100),
	}

	currentTime := uint64(time.Now().Unix()) + 20
	newDifficulty := big.NewInt(500) // Below minimum

	err := validator.ValidateDifficulty(newDifficulty, currentTime, parentHeader)
	if err == nil {
		t.Error("Difficulty below minimum should fail")
	}
}

func TestValidateDifficulty_ExceedsMaxChange(t *testing.T) {
	params := core.ConsensusParams{
		BlockTimeTargetSeconds:     17,
		MinDifficulty:             1,
		MaxDifficultyChangePercent: 100,
	}
	validator := NewBlockValidator(params)

	parentHeader := &nogopowHeaderWrapper{
		Difficulty: big.NewInt(1000000),
		Time:       uint64(time.Now().Unix()),
		Number:     big.NewInt(100),
	}

	currentTime := uint64(time.Now().Unix()) + 20
	newDifficulty := big.NewInt(3000000) // 3x increase (exceeds 2x limit)

	err := validator.ValidateDifficulty(newDifficulty, currentTime, parentHeader)
	if err == nil {
		t.Error("Difficulty exceeding max change should fail")
	}
}

func TestValidateCoinbaseEconomics_ValidReward(t *testing.T) {
	policy := &MonetaryPolicy{
		InitialBlockReward:   50 * 1e8,
		MinerRewardShare:     80,
		CommunityFundShare:   10,
		GenesisShare:         5,
		IntegrityPoolShare:   5,
		HalvingInterval:      210000,
		TotalSupplyCap:       21000000 * 1e8,
	}
	policy.CalculateBlockReward = func(height uint64) uint64 {
		return policy.InitialBlockReward
	}
	policy.MinerFeeAmount = func(fees uint64) uint64 {
		return fees // All fees to miner for simplicity
	}

	params := core.ConsensusParams{
		MonetaryPolicy: policy,
	}
	validator := NewBlockValidator(params)

	minerAddr := "test_miner_address"
	block := &core.Block{
		Height:       100,
		MinerAddress: minerAddr,
		Transactions: []core.Transaction{
			{
				Type:      core.TxCoinbase,
				ToAddress: minerAddr,
				Amount:    40 * 1e8, // 80% of 50
				Data:      "test reward",
			},
		},
	}

	err := validator.validateCoinbaseEconomics(block)
	if err != nil {
		t.Errorf("Valid coinbase economics should pass, got error: %v", err)
	}
}

func TestValidateCoinbaseEconomics_WrongAddress(t *testing.T) {
	policy := &MonetaryPolicy{
		InitialBlockReward: 50 * 1e8,
		MinerRewardShare:   80,
	}
	policy.CalculateBlockReward = func(height uint64) uint64 {
		return policy.InitialBlockReward
	}
	policy.MinerFeeAmount = func(fees uint64) uint64 { return fees }

	params := core.ConsensusParams{MonetaryPolicy: policy}
	validator := NewBlockValidator(params)

	block := &core.Block{
		Height:       100,
		MinerAddress: "correct_address",
		Transactions: []core.Transaction{
			{
				Type:      core.TxCoinbase,
				ToAddress: "wrong_address", // Mismatch!
				Amount:    40 * 1e8,
			},
		},
	}

	err := validator.validateCoinbaseEconomics(block)
	if err == nil {
		t.Error("Coinbase with wrong address should fail")
	}
}

func TestValidateCoinbaseEconomics_FeeOverflowProtection(t *testing.T) {
	policy := &MonetaryPolicy{
		InitialBlockReward: 50 * 1e8,
		MinerRewardShare:   80,
	}
	policy.CalculateBlockReward = func(height uint64) uint64 {
		return policy.InitialBlockReward
	}
	policy.MinerFeeAmount = func(fees uint64) uint64 { return fees }

	params := core.ConsensusParams{MonetaryPolicy: policy}
	validator := NewBlockValidator(params)

	block := &core.Block{
		Height:       100,
		MinerAddress: "miner",
		Transactions: []core.Transaction{
			{
				Type:  core.TxCoinbase,
				ToAddress: "miner",
				Amount: 40 * 1e8,
			},
			// Add transactions with very high fees to test overflow protection
			{
				Type: core.TxTransfer,
				Fee:  math.MaxUint64 - 100, // Very high fee
			},
			{
				Type: core.TxTransfer,
				Fee:  200, // Should trigger overflow when added to previous
			},
		},
	}

	err := validator.validateCoinbaseEconomics(block)
	if err == nil {
		t.Error("Fee overflow should be detected and rejected")
	}
}

func TestVerifySignaturesBatch_EmptyList(t *testing.T) {
	params := core.ConsensusParams{}
	validator := NewBlockValidator(params)

	err := validator.VerifySignaturesBatch([]core.Transaction{})
	if err != nil {
		t.Errorf("Empty transaction list should be valid, got error: %v", err)
	}
}

func TestVerifySignaturesBatch_SingleValidTx(t *testing.T) {
	// This test requires a properly signed transaction
	// For now, we test that the method handles single tx correctly
	t.Skip("Requires wallet setup for signature generation")
}

func TestValidateBlock_NilBlock(t *testing.T) {
	params := core.ConsensusParams{}
	validator := NewBlockValidator(params)

	err := validator.ValidateBlock(nil, nil)
	if err == nil {
		t.Error("Nil block should fail validation")
	}
}

func TestValidateBlock_NilParent(t *testing.T) {
	params := core.ConsensusParams{}
	validator := NewBlockValidator(params)

	block := &core.Block{
		Height: 1,
		Header: core.BlockHeader{
			Version:        1,
			TimestampUnix:  time.Now().Unix(),
			DifficultyBits: 1,
		},
	}

	err := validator.ValidateBlock(block, nil)
	if err == nil {
		t.Error("Block without parent at height > 0 should fail")
	}
}

func TestValidateGenesisBlock(t *testing.T) {
	params := core.ConsensusParams{
		BlockTimeTargetSeconds: 17,
	}
	validator := NewBlockValidator(params)

	genesis := &core.Block{
		Height: 0,
		Header: core.BlockHeader{
			Version:        1,
			TimestampUnix:  time.Now().Unix(),
			DifficultyBits: 1,
		},
		Transactions: []core.Transaction{
			{
				Type:      core.TxCoinbase,
				ToAddress: "genesis_miner",
				Amount:    0, // Genesis has no reward
				Data:      "Memphis",
			},
		},
	}

	err := validator.ValidateGenesisBlock(genesis)
	if err != nil {
		t.Errorf("Valid genesis block should pass, got error: %v", err)
	}
}

func TestValidateGenesisBlock_InvalidHeight(t *testing.T) {
	params := core.ConsensusParams{}
	validator := NewBlockValidator(params)

	genesis := &core.Block{
		Height: 1, // Genesis must have height 0
		Header: core.BlockHeader{
			Version:        1,
			TimestampUnix:  time.Now().Unix(),
			DifficultyBits: 1,
		},
	}

	err := validator.ValidateGenesisBlock(genesis)
	if err == nil {
		t.Error("Genesis block with height != 0 should fail")
	}
}

// nogopowHeaderWrapper implements Header interface for testing
type nogopowHeaderWrapper struct {
	ParentHash [32]byte
	Coinbase   Address
	Number     *big.Int
	Time       uint64
	Difficulty *big.Int
}

func (h *nogopowHeaderWrapper) ParentHash() [32]byte         { return h.ParentHash }
func (h *nogopowHeaderWrapper) Coinbase() Address           { return h.Coinbase }
func (h *nogopowHeaderWrapper) Number() *big.Int            { return h.Number }
func (h *nogopowHeaderWrapper) Time() uint64                { return h.Time }
func (h *nogopowHeaderWrapper) Difficulty() *big.Int        { return h.Difficulty }

func BenchmarkValidateBlockTimestamp(b *testing.B) {
	params := core.ConsensusParams{
		BlockTimeTargetSeconds: 17,
	}
	validator := NewBlockValidator(params)

	parentBlock := &core.Block{
		Header: core.BlockHeader{
			TimestampUnix: time.Now().Unix() - 17,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newBlock := &core.Block{
			Header: core.BlockHeader{
				TimestampUnix: time.Now().Unix(),
				PrevHash:      parentBlock.Hash,
			},
		}
		validator.ValidateBlockTimestamp(newBlock, parentBlock)
	}
}

func BenchmarkValidateDifficulty(b *testing.B) {
	params := core.ConsensusParams{
		BlockTimeTargetSeconds:     17,
		MinDifficulty:             1,
		MaxDifficultyChangePercent: 100,
	}
	validator := NewBlockValidator(params)

	parentHeader := &nogopowHeaderWrapper{
		Difficulty: big.NewInt(1000000),
		Time:       uint64(time.Now().Unix()),
		Number:     big.NewInt(100),
	}

	currentTime := uint64(time.Now().Unix()) + 20
	newDifficulty := big.NewInt(1100000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateDifficulty(newDifficulty, currentTime, parentHeader)
	}
}
