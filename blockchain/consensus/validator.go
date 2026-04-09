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

package consensus

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/nogopow"
	"github.com/nogochain/nogo/internal/crypto"
	"github.com/nogochain/nogo/internal/ntp"
)

const (
	BlockTimeMaxDrift          = 7200
	DifficultyTolerancePercent = 50
	maxDifficultyBits          = uint32(math.MaxUint32)
)

// Type aliases for convenience
type (
	Transaction      = core.Transaction
	TransactionType  = core.TransactionType
	MetricsCollector = core.MetricsCollector
	NoopMetrics      = core.NoopMetrics
)

const (
	TxCoinbase = core.TxCoinbase
	TxTransfer = core.TxTransfer
)

var (
	ErrNilBlock                  = errors.New("block is nil")
	ErrEmptyBlockHash            = errors.New("block hash is empty")
	ErrZeroDifficultyBits        = errors.New("difficulty bits is zero")
	ErrDifficultyTooHigh         = errors.New("difficulty bits exceeds maximum")
	ErrInvalidVersion            = errors.New("invalid block version")
	ErrNoTransactions            = errors.New("block has no transactions")
	ErrInvalidCoinbasePos        = errors.New("first transaction must be coinbase")
	ErrWrongChainID              = errors.New("transaction has wrong chainId")
	ErrInvalidSignature          = errors.New("transaction signature verification failed")
	ErrInvalidCoinbaseAmt        = errors.New("invalid coinbase amount")
	ErrInvalidMinerAddress       = errors.New("invalid miner address")
	ErrTimestampNotIncreasing    = errors.New("block timestamp not greater than parent")
	ErrTimestampTooFarFuture     = errors.New("block timestamp too far in future")
	ErrGenesisDifficultyMismatch = errors.New("genesis difficulty mismatch")
	ErrParentBlockNil            = errors.New("parent block is nil for non-genesis block")
	ErrDifficultyTooLow          = errors.New("difficulty below minimum")
	ErrDifficultyTooHighRange    = errors.New("difficulty above maximum")
	ErrDifficultyAdjustmentLow   = errors.New("difficulty adjustment too low")
	ErrDifficultyAdjustmentHigh  = errors.New("difficulty adjustment too high")
)

type BlockValidator struct {
	consensus ConsensusParams
	chainID   uint64
	metrics   MetricsCollector
	mu        sync.RWMutex
}

func NewBlockValidator(consensus ConsensusParams, chainID uint64, metrics MetricsCollector) *BlockValidator {
	return &BlockValidator{
		consensus: consensus,
		chainID:   chainID,
		metrics:   metrics,
	}
}

func (v *BlockValidator) UpdateConsensus(consensus ConsensusParams) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.consensus = consensus
}

func (v *BlockValidator) ValidateBlock(block *Block, parent *Block, state map[string]Account) error {
	startTime := time.Now()
	defer func() {
		if v.metrics != nil {
			v.metrics.ObserveBlockVerification(time.Since(startTime))
		}
	}()

	v.mu.RLock()
	consensus := v.consensus
	v.mu.RUnlock()

	if err := v.validateBlockStructure(block); err != nil {
		return fmt.Errorf("structural validation failed: %w", err)
	}

	if err := validateBlockPoWNogoPow(consensus, block, parent); err != nil {
		return fmt.Errorf("POW validation failed: %w", err)
	}

	if err := v.validateDifficulty(block, parent); err != nil {
		return fmt.Errorf("difficulty validation failed: %w", err)
	}

	if parent != nil {
		if err := v.validateTimestamp(block, parent); err != nil {
			return fmt.Errorf("timestamp validation failed: %w", err)
		}
	}

	if err := v.validateTransactions(block, consensus); err != nil {
		return fmt.Errorf("transaction validation failed: %w", err)
	}

	if state != nil && block.Height > 0 {
		testState := make(map[string]Account, len(state))
		for k, val := range state {
			testState[k] = val
		}
		if err := applyBlockToState(consensus, testState, block); err != nil {
			return fmt.Errorf("state transition validation failed: %w", err)
		}
	}

	return nil
}

func (v *BlockValidator) validateBlockStructure(block *Block) error {
	if block == nil {
		return ErrNilBlock
	}

	if len(block.Hash) == 0 {
		return ErrEmptyBlockHash
	}

	if block.DifficultyBits == 0 {
		return ErrZeroDifficultyBits
	}

	if block.DifficultyBits > maxDifficultyBits {
		return fmt.Errorf("difficulty bits %d exceeds maximum %d", block.DifficultyBits, maxDifficultyBits)
	}

	expectedVersion := blockVersionForHeight(v.consensus, block.Height)
	if block.Version != expectedVersion {
		return fmt.Errorf("%w: expected %d got %d at height %d", ErrInvalidVersion, expectedVersion, block.Version, block.Height)
	}

	return nil
}

func (v *BlockValidator) validateDifficulty(block *Block, parent *Block) error {
	if block.Height == 0 {
		if block.DifficultyBits != v.consensus.GenesisDifficultyBits {
			return fmt.Errorf("%w: expected %d got %d", ErrGenesisDifficultyMismatch, v.consensus.GenesisDifficultyBits, block.DifficultyBits)
		}
		return nil
	}

	if parent == nil {
		return ErrParentBlockNil
	}

	if block.DifficultyBits < v.consensus.MinDifficultyBits {
		return fmt.Errorf("%w: %d < min %d", ErrDifficultyTooLow, block.DifficultyBits, v.consensus.MinDifficultyBits)
	}

	if block.DifficultyBits > v.consensus.MaxDifficultyBits {
		return fmt.Errorf("%w: %d > max %d", ErrDifficultyTooHighRange, block.DifficultyBits, v.consensus.MaxDifficultyBits)
	}

	if v.consensus.DifficultyEnable {
		// Use the same consensus parameters as mining to ensure consistency
		// This matches the parameters used in core/mining.go MineTransfers()
		adjuster := nogopow.NewDifficultyAdjuster(&v.consensus)

		var parentHash nogopow.Hash
		if len(parent.Hash) > 0 {
			copy(parentHash[:], parent.Hash)
		} else {
			copy(parentHash[:], parent.PrevHash)
		}

		parentHeader := &nogopow.Header{
			Number:     big.NewInt(int64(parent.Height)),
			Time:       uint64(parent.TimestampUnix),
			Difficulty: big.NewInt(int64(parent.DifficultyBits)),
			ParentHash: parentHash,
		}

		expectedDifficulty := adjuster.CalcDifficulty(uint64(block.TimestampUnix), parentHeader)
		actualDifficulty := big.NewInt(int64(block.DifficultyBits))

		minAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(100-DifficultyTolerancePercent))
		minAllowed.Div(minAllowed, big.NewInt(100))

		maxAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(100+DifficultyTolerancePercent))
		maxAllowed.Div(maxAllowed, big.NewInt(100))

		if actualDifficulty.Cmp(minAllowed) < 0 {
			return fmt.Errorf("%w: actual %d < min %d (expected %d)", ErrDifficultyAdjustmentLow, actualDifficulty.Uint64(), minAllowed.Uint64(), expectedDifficulty.Uint64())
		}

		if actualDifficulty.Cmp(maxAllowed) > 0 {
			return fmt.Errorf("%w: actual %d > max %d (expected %d)", ErrDifficultyAdjustmentHigh, actualDifficulty.Uint64(), maxAllowed.Uint64(), expectedDifficulty.Uint64())
		}
	}

	return nil
}

func (v *BlockValidator) validateTimestamp(block *Block, parent *Block) error {
	if block.TimestampUnix <= parent.TimestampUnix {
		return fmt.Errorf("%w: block time %d not greater than parent time %d", ErrTimestampNotIncreasing, block.TimestampUnix, parent.TimestampUnix)
	}

	maxAllowedTime := ntp.NowUnix() + BlockTimeMaxDrift
	if block.TimestampUnix > maxAllowedTime {
		return fmt.Errorf("%w: block time %d too far in future (max allowed %d)", ErrTimestampTooFarFuture, block.TimestampUnix, maxAllowedTime)
	}

	return nil
}

func (v *BlockValidator) validateTransactions(block *Block, consensus ConsensusParams) error {
	if len(block.Transactions) == 0 {
		return ErrNoTransactions
	}

	if block.Transactions[0].Type != TxCoinbase {
		return ErrInvalidCoinbasePos
	}

	for i := range block.Transactions {
		if block.Transactions[i].ChainID == 0 {
			block.Transactions[i].ChainID = v.chainID
		}
		if block.Transactions[i].ChainID != v.chainID {
			return fmt.Errorf("%w: transaction %d has chainId %d", ErrWrongChainID, i, block.Transactions[i].ChainID)
		}
	}

	if err := v.verifyTransactionsBatch(block, consensus); err != nil {
		return err
	}

	if block.Height > 0 {
		if err := v.validateCoinbaseEconomics(block); err != nil {
			return fmt.Errorf("coinbase economics validation failed: %w", err)
		}
	}

	return nil
}

func (v *BlockValidator) verifyTransactionsBatch(block *Block, consensus ConsensusParams) error {
	n := len(block.Transactions)
	results := make([]bool, n)
	results[0] = true

	if n <= crypto.BATCH_VERIFY_THRESHOLD {
		for i := 1; i < n; i++ {
			tx := block.Transactions[i]
			if tx.Type != TxTransfer {
				results[i] = true
				continue
			}
			err := tx.VerifyForConsensus(consensus, block.Height)
			results[i] = (err == nil)
		}
	} else {
		batchPubKeys := make([]crypto.PublicKey, 0, n)
		batchMessages := make([][]byte, 0, n)
		batchSignatures := make([][]byte, 0, n)
		batchIndices := make([]int, 0, n)

		for i := 1; i < n; i++ {
			tx := block.Transactions[i]
			if tx.Type != TxTransfer {
				results[i] = true
				continue
			}
			if len(tx.FromPubKey) != ed25519.PublicKeySize || len(tx.Signature) != ed25519.SignatureSize {
				results[i] = false
				continue
			}

			h, err := txSigningHashForConsensus(tx, consensus, block.Height)
			if err != nil {
				results[i] = false
				continue
			}

			batchPubKeys = append(batchPubKeys, tx.FromPubKey)
			batchMessages = append(batchMessages, h)
			batchSignatures = append(batchSignatures, tx.Signature)
			batchIndices = append(batchIndices, i)
		}

		if len(batchPubKeys) > 0 {
			batchResults, err := crypto.VerifyBatch(batchPubKeys, batchMessages, batchSignatures)
			if err != nil {
				for _, idx := range batchIndices {
					results[idx] = false
				}
			} else {
				for k, idx := range batchIndices {
					results[idx] = batchResults[k]
				}
			}
		}
	}

	for i, valid := range results {
		if !valid {
			return fmt.Errorf("%w: transaction %d", ErrInvalidSignature, i)
		}
	}

	return nil
}

func (v *BlockValidator) validateCoinbaseEconomics(block *Block) error {
	if err := validateAddress(block.MinerAddress); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMinerAddress, err)
	}

	var totalFees uint64
	for _, tx := range block.Transactions[1:] {
		if tx.Type == TxTransfer {
			totalFees += tx.Fee
		}
	}

	policy := v.consensus.MonetaryPolicy
	// Miner receives 96% of block reward + 100% of fees (fees are burned)
	expectedAmount := policy.BlockReward(block.Height)*uint64(policy.MinerRewardShare)/100 + policy.MinerFeeAmount(totalFees)

	coinbase := block.Transactions[0]
	if coinbase.ToAddress != block.MinerAddress {
		return fmt.Errorf("coinbase toAddress %s does not match minerAddress %s", coinbase.ToAddress, block.MinerAddress)
	}

	if coinbase.Amount != expectedAmount {
		return fmt.Errorf("%w: expected %d got %d (height=%d)", ErrInvalidCoinbaseAmt, expectedAmount, coinbase.Amount, block.Height)
	}

	return nil
}

func (v *BlockValidator) ValidateBlockFast(block *Block) error {
	if block == nil {
		return ErrNilBlock
	}

	if len(block.Hash) == 0 {
		return ErrEmptyBlockHash
	}

	// Normalize block fields from alternative JSON formats
	// Remote nodes may return fields at top level or nested in "header"
	// This ensures consistent field access regardless of serialization source
	v.normalizeBlockFields(block)

	// After normalization, check for zero difficulty
	if block.DifficultyBits == 0 {
		return ErrZeroDifficultyBits
	}

	if len(block.Transactions) == 0 {
		return ErrNoTransactions
	}

	return nil
}

// normalizeBlockFields maps fields from Header to top-level if top-level is empty
// This handles JSON deserialization where fields may be in either location
func (v *BlockValidator) normalizeBlockFields(block *Block) {
	// Normalize DifficultyBits
	if block.DifficultyBits == 0 {
		if block.Difficulty > 0 {
			block.DifficultyBits = block.Difficulty
		} else if block.Header.DifficultyBits > 0 {
			block.DifficultyBits = block.Header.DifficultyBits
		} else if block.Header.Difficulty > 0 {
			block.DifficultyBits = block.Header.Difficulty
		}
	}

	// Normalize PrevHash (critical for chain continuity validation)
	if len(block.PrevHash) == 0 && len(block.Header.PrevHash) > 0 {
		block.PrevHash = make([]byte, len(block.Header.PrevHash))
		copy(block.PrevHash, block.Header.PrevHash)
	}

	// Normalize TimestampUnix (critical for timestamp validation)
	if block.TimestampUnix == 0 && block.Header.TimestampUnix > 0 {
		block.TimestampUnix = block.Header.TimestampUnix
	}

	// Normalize Nonce
	if block.Nonce == 0 && block.Header.Nonce > 0 {
		block.Nonce = block.Header.Nonce
	}

	// Normalize Version
	if block.Version == 0 && block.Header.Version > 0 {
		block.Version = block.Header.Version
	}
}

func validateBlockPoWNogoPow(consensus ConsensusParams, block *Block, parent *Block) error {
	if block == nil || len(block.Hash) == 0 {
		return errors.New("invalid block for POW verification")
	}

	if block.Height == 0 {
		return nil
	}

	if parent == nil {
		return errors.New("parent block is nil for POW verification")
	}

	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()

	var parentHash nogopow.Hash
	copy(parentHash[:], parent.Hash)

	// Convert miner address string to nogopow.Address using reusable function
	// This ensures consistent address conversion across the codebase
	powCoinbase, err := core.StringToAddress(block.MinerAddress)
	if err != nil {
		return fmt.Errorf("invalid miner address: %w", err)
	}

	header := &nogopow.Header{
		Number:     big.NewInt(int64(block.Height)),
		Time:       uint64(block.TimestampUnix),
		ParentHash: parentHash,
		Difficulty: big.NewInt(int64(block.DifficultyBits)),
		Coinbase:   powCoinbase,
	}

	binary.LittleEndian.PutUint64(header.Nonce[:8], block.Nonce)

	if err := engine.VerifySealOnly(header); err != nil {
		return fmt.Errorf("NogoPow seal verification failed for block %d: %w", block.Height, err)
	}

	return nil
}

func validateAddress(addr string) error {
	if len(addr) < 14 {
		return errors.New("address too short")
	}

	if len(addr) >= 4 && addr[:4] == "NOGO" {
		encoded := addr[4:]
		decoded, err := hex.DecodeString(encoded)
		if err != nil {
			return fmt.Errorf("invalid hex: %w", err)
		}
		if len(decoded) < 5 {
			return errors.New("invalid address length")
		}
		return nil
	}

	if _, err := hex.DecodeString(addr); err != nil {
		return fmt.Errorf("invalid hex address: %w", err)
	}

	return nil
}

func txSigningHashForConsensus(tx Transaction, p ConsensusParams, height uint64) ([]byte, error) {
	if p.BinaryEncodingActive(height) {
		return txSigningHashBinary(tx, p, height)
	}
	return txSigningHashJSON(tx, p, height)
}

func txSigningHashJSON(tx Transaction, p ConsensusParams, height uint64) ([]byte, error) {
	type signingView struct {
		Type      TransactionType `json:"type"`
		ChainID   uint64          `json:"chainId"`
		FromAddr  string          `json:"fromAddr,omitempty"`
		ToAddress string          `json:"toAddress"`
		Amount    uint64          `json:"amount"`
		Fee       uint64          `json:"fee"`
		Nonce     uint64          `json:"nonce,omitempty"`
		Data      string          `json:"data,omitempty"`
		Height    uint64          `json:"height"`
	}

	v := signingView{
		Type:      tx.Type,
		ChainID:   tx.ChainID,
		ToAddress: tx.ToAddress,
		Amount:    tx.Amount,
		Fee:       tx.Fee,
		Nonce:     tx.Nonce,
		Data:      tx.Data,
		Height:    height,
	}

	if tx.Type == TxTransfer {
		fromAddr, err := tx.FromAddress()
		if err != nil {
			return nil, err
		}
		v.FromAddr = fromAddr
	}

	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	return sum[:], nil
}

func txSigningHashBinary(tx Transaction, p ConsensusParams, height uint64) ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := buf.WriteByte(0x01); err != nil {
		return nil, fmt.Errorf("write version: %w", err)
	}

	if err := binary.Write(buf, binary.LittleEndian, tx.Type); err != nil {
		return nil, fmt.Errorf("write type: %w", err)
	}

	if err := binary.Write(buf, binary.LittleEndian, tx.ChainID); err != nil {
		return nil, fmt.Errorf("write chainId: %w", err)
	}

	if tx.Type == TxTransfer && len(tx.FromPubKey) > 0 {
		if err := binary.Write(buf, binary.LittleEndian, uint8(len(tx.FromPubKey))); err != nil {
			return nil, fmt.Errorf("write fromPubKey length: %w", err)
		}
		if _, err := buf.Write(tx.FromPubKey); err != nil {
			return nil, fmt.Errorf("write fromPubKey: %w", err)
		}
	}

	toBytes := []byte(tx.ToAddress)
	if err := binary.Write(buf, binary.LittleEndian, uint8(len(toBytes))); err != nil {
		return nil, fmt.Errorf("write toAddress length: %w", err)
	}
	if _, err := buf.Write(toBytes); err != nil {
		return nil, fmt.Errorf("write toAddress: %w", err)
	}

	if err := binary.Write(buf, binary.LittleEndian, tx.Amount); err != nil {
		return nil, fmt.Errorf("write amount: %w", err)
	}

	if err := binary.Write(buf, binary.LittleEndian, tx.Fee); err != nil {
		return nil, fmt.Errorf("write fee: %w", err)
	}

	if err := binary.Write(buf, binary.LittleEndian, tx.Nonce); err != nil {
		return nil, fmt.Errorf("write nonce: %w", err)
	}

	dataBytes := []byte(tx.Data)
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(dataBytes))); err != nil {
		return nil, fmt.Errorf("write data length: %w", err)
	}
	if len(dataBytes) > 0 {
		if _, err := buf.Write(dataBytes); err != nil {
			return nil, fmt.Errorf("write data: %w", err)
		}
	}

	if err := binary.Write(buf, binary.LittleEndian, height); err != nil {
		return nil, fmt.Errorf("write height: %w", err)
	}

	sum := sha256.Sum256(buf.Bytes())
	return sum[:], nil
}

func applyBlockToState(p ConsensusParams, state map[string]Account, b *Block) error {
	if len(b.Transactions) == 0 {
		return errors.New("block has no transactions")
	}
	if b.Transactions[0].Type != TxCoinbase {
		return errors.New("first tx must be coinbase")
	}

	if b.Height > 0 {
		if err := validateAddress(b.MinerAddress); err != nil {
			return fmt.Errorf("invalid minerAddress: %w", err)
		}
		var fees uint64
		for _, tx := range b.Transactions[1:] {
			if tx.Type != TxTransfer {
				continue
			}
			fees += tx.Fee
		}
		cb := b.Transactions[0]
		if cb.ToAddress != b.MinerAddress {
			return errors.New("coinbase toAddress must match minerAddress")
		}
		policy := p.MonetaryPolicy
		// Miner receives 96% of block reward + 100% of fees (fees are burned)
		expected := policy.BlockReward(b.Height)*uint64(policy.MinerRewardShare)/100 + policy.MinerFeeAmount(fees)
		if cb.Amount != expected {
			return fmt.Errorf("bad coinbase amount: expected %d got %d", expected, cb.Amount)
		}
	}

	for i, tx := range b.Transactions {
		switch tx.Type {
		case TxCoinbase:
			if i != 0 {
				return errors.New("coinbase must be first")
			}
			if err := tx.VerifyForConsensus(p, b.Height); err != nil {
				return err
			}
			acct := state[tx.ToAddress]
			// Safe overflow check: use comparison instead of wraparound detection
			if acct.Balance > math.MaxUint64-tx.Amount {
				return errors.New("coinbase balance overflow")
			}
			acct.Balance += tx.Amount
			state[tx.ToAddress] = acct
		case TxTransfer:
			if err := tx.VerifyForConsensus(p, b.Height); err != nil {
				return err
			}
			fromAddr, err := tx.FromAddress()
			if err != nil {
				return err
			}
			from := state[fromAddr]
			if from.Nonce+1 != tx.Nonce {
				return fmt.Errorf("bad nonce for %s: expected %d got %d", fromAddr, from.Nonce+1, tx.Nonce)
			}
			totalDebit := tx.Amount + tx.Fee
			if from.Balance < totalDebit {
				return fmt.Errorf("insufficient funds for %s", fromAddr)
			}
			from.Balance -= totalDebit
			from.Nonce = tx.Nonce
			state[fromAddr] = from

			to := state[tx.ToAddress]
			// Safe overflow check: use comparison instead of wraparound detection
			if to.Balance > math.MaxUint64-tx.Amount {
				return errors.New("transfer balance overflow")
			}
			to.Balance += tx.Amount
			state[tx.ToAddress] = to
		default:
			return fmt.Errorf("unknown tx type: %q", tx.Type)
		}
	}
	return nil
}
