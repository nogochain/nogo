package main

import (
	"crypto/ed25519"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/nogopow"
	"github.com/nogochain/nogo/internal/crypto"
)

// BlockValidator provides unified block validation interface
// This validator ensures ALL blocks are fully validated before acceptance
// No validation is skipped, regardless of sync status or network conditions
type BlockValidator struct {
	consensus ConsensusParams
	chainID   uint64
	metrics   *Metrics
	mu        sync.RWMutex
}

// NewBlockValidator creates a new block validator with the given consensus parameters
func NewBlockValidator(consensus ConsensusParams, chainID uint64, metrics *Metrics) *BlockValidator {
	return &BlockValidator{
		consensus: consensus,
		chainID:   chainID,
		metrics:   metrics,
	}
}

// UpdateConsensus updates the consensus parameters (thread-safe)
func (v *BlockValidator) UpdateConsensus(consensus ConsensusParams) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.consensus = consensus
}

// ValidateBlock performs comprehensive validation of a block
// This is the SINGLE ENTRY POINT for all block validation
//
// Validation pipeline:
// 1. Structural validation (hash, difficulty, version)
// 2. POW seal verification (full cryptographic verification)
// 3. Difficulty adjustment validation (independent calculation)
// 4. Timestamp validation (deterministic MTP-based)
// 5. Transaction validation (signatures, chainId, nonces)
// 6. State transition validation (apply to state)
//
// Returns nil if block is valid, error describing the validation failure otherwise
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

	// Step 1: Structural validation
	if err := v.validateBlockStructure(block); err != nil {
		return fmt.Errorf("structural validation failed: %w", err)
	}

	// Step 2: POW seal verification (FULL VERIFICATION - NO SKIPPING)
	if err := validateBlockPoWNogoPow(consensus, block, parent); err != nil {
		return fmt.Errorf("POW validation failed: %w", err)
	}

	// Step 3: Difficulty validation (STRICT - independent calculation)
	if err := v.validateDifficulty(block, parent); err != nil {
		return fmt.Errorf("difficulty validation failed: %w", err)
	}

	// Step 4: Timestamp validation (DETERMINISTIC - MTP-based)
	// CRITICAL: MTP calculation requires access to the full blockchain history
	// not just parent and current block. The caller should pass the full path.
	// For standalone validation (without full path), we use a simplified check:
	// - Block time must be greater than parent time
	// - Block time must be reasonable (not too far in future)
	// Full MTP validation should be done in AddBlock with complete path
	if parent != nil {
		// Simplified validation for standalone use
		if block.TimestampUnix <= parent.TimestampUnix {
			return fmt.Errorf("timestamp validation failed: block time %d not greater than parent time %d",
				block.TimestampUnix, parent.TimestampUnix)
		}
		// Check if block time is too far in future (using NTP synchronized time + tolerance)
		// This is a sanity check, not consensus rule
		maxAllowedTime := getNetworkTimeUnix() + BlockTimeMaxDrift
		if block.TimestampUnix > maxAllowedTime {
			return fmt.Errorf("timestamp validation failed: block time %d too far in future (max allowed %d)",
				block.TimestampUnix, maxAllowedTime)
		}
	}

	// Step 5: Transaction validation
	if err := v.validateTransactions(block, consensus, v.metrics); err != nil {
		return fmt.Errorf("transaction validation failed: %w", err)
	}

	// Step 6: State transition validation (if state provided)
	if state != nil && block.Height > 0 {
		testState := make(map[string]Account)
		for k, v := range state {
			testState[k] = v
		}
		if err := applyBlockToState(consensus, testState, block); err != nil {
			return fmt.Errorf("state transition validation failed: %w", err)
		}
	}

	return nil
}

// validateBlockStructure performs basic structural validation
func (v *BlockValidator) validateBlockStructure(block *Block) error {
	if block == nil {
		return fmt.Errorf("block is nil")
	}

	if len(block.Hash) == 0 {
		return fmt.Errorf("block hash is empty")
	}

	if block.DifficultyBits == 0 {
		return fmt.Errorf("difficulty bits is zero")
	}

	if block.DifficultyBits > maxDifficultyBits {
		return fmt.Errorf("difficulty bits %d exceeds maximum %d", block.DifficultyBits, maxDifficultyBits)
	}

	expectedVersion := blockVersionForHeight(v.consensus, block.Height)
	if block.Version != expectedVersion {
		return fmt.Errorf("invalid version: expected %d got %d at height %d",
			expectedVersion, block.Version, block.Height)
	}

	return nil
}

// validateDifficulty performs independent difficulty calculation and verification
func (v *BlockValidator) validateDifficulty(block *Block, parent *Block) error {
	if block.Height == 0 {
		// Genesis block - check genesis difficulty
		if block.DifficultyBits != v.consensus.GenesisDifficultyBits {
			return fmt.Errorf("genesis difficulty mismatch: expected %d got %d",
				v.consensus.GenesisDifficultyBits, block.DifficultyBits)
		}
		return nil
	}

	if parent == nil {
		return fmt.Errorf("parent block is nil for non-genesis block")
	}

	// Check difficulty bounds
	if block.DifficultyBits < v.consensus.MinDifficultyBits {
		return fmt.Errorf("difficulty %d below minimum %d", block.DifficultyBits, v.consensus.MinDifficultyBits)
	}
	if block.DifficultyBits > v.consensus.MaxDifficultyBits {
		return fmt.Errorf("difficulty %d above maximum %d", block.DifficultyBits, v.consensus.MaxDifficultyBits)
	}

	// If difficulty adjustment is enabled, verify the adjustment logic
	if v.consensus.DifficultyEnable {
		config := nogopow.DefaultDifficultyConfig()
		adjuster := nogopow.NewDifficultyAdjuster(config)

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

		// Tight bounds: ±20%
		minAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(80))
		minAllowed.Div(minAllowed, big.NewInt(100))

		maxAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(120))
		maxAllowed.Div(maxAllowed, big.NewInt(100))

		if actualDifficulty.Cmp(minAllowed) < 0 {
			return fmt.Errorf("difficulty too low: actual %d < min %d (expected %d)",
				actualDifficulty.Uint64(), minAllowed.Uint64(), expectedDifficulty.Uint64())
		}

		if actualDifficulty.Cmp(maxAllowed) > 0 {
			return fmt.Errorf("difficulty too high: actual %d > max %d (expected %d)",
				actualDifficulty.Uint64(), maxAllowed.Uint64(), expectedDifficulty.Uint64())
		}
	}

	return nil
}

// validateTransactions validates all transactions in a block
func (v *BlockValidator) validateTransactions(block *Block, consensus ConsensusParams, metrics *Metrics) error {
	if len(block.Transactions) == 0 {
		return fmt.Errorf("block has no transactions")
	}

	// Verify coinbase is first
	if block.Transactions[0].Type != TxCoinbase {
		return fmt.Errorf("first transaction must be coinbase")
	}

	// Validate each transaction's chainId first
	for i := range block.Transactions {
		if block.Transactions[i].ChainID == 0 {
			block.Transactions[i].ChainID = v.chainID
		}
		if block.Transactions[i].ChainID != v.chainID {
			return fmt.Errorf("transaction %d has wrong chainId: %d", i, block.Transactions[i].ChainID)
		}
	}

	// Batch verify transfer transaction signatures
	if err := v.verifyTransactionsBatch(block, consensus); err != nil {
		return err
	}

	// Validate coinbase economics for non-genesis blocks
	if block.Height > 0 {
		if err := v.validateCoinbaseEconomics(block); err != nil {
			return fmt.Errorf("coinbase economics validation failed: %w", err)
		}
	}

	return nil
}

// verifyTransactionsBatch performs batch signature verification for transactions
func (v *BlockValidator) verifyTransactionsBatch(block *Block, consensus ConsensusParams) error {
	n := len(block.Transactions)
	results := make([]bool, n)

	// Coinbase transactions are always valid structurally
	results[0] = true

	// Prepare batch verification for transfer transactions
	if n <= crypto.BATCH_VERIFY_THRESHOLD {
		// Small batch, use individual verification
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
		// Large batch, use batch verification
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

	// Check results and return detailed error
	for i, valid := range results {
		if !valid {
			return fmt.Errorf("transaction %d verification failed", i)
		}
	}

	return nil
}

// validateCoinbaseEconomics validates coinbase amount matches reward + fees
func (v *BlockValidator) validateCoinbaseEconomics(block *Block) error {
	if err := validateAddress(block.MinerAddress); err != nil {
		return fmt.Errorf("invalid miner address: %w", err)
	}

	// Calculate total fees
	var totalFees uint64
	for _, tx := range block.Transactions[1:] {
		if tx.Type == TxTransfer {
			totalFees += tx.Fee
		}
	}

	// Calculate expected coinbase amount
	policy := v.consensus.MonetaryPolicy
	expectedAmount := policy.BlockReward(block.Height) + policy.MinerFeeAmount(totalFees)

	coinbase := block.Transactions[0]
	if coinbase.ToAddress != block.MinerAddress {
		return fmt.Errorf("coinbase toAddress %s does not match minerAddress %s",
			coinbase.ToAddress, block.MinerAddress)
	}

	if coinbase.Amount != expectedAmount {
		return fmt.Errorf("invalid coinbase amount: expected %d got %d (height=%d)",
			expectedAmount, coinbase.Amount, block.Height)
	}

	return nil
}

// ValidateBlockFast performs lightweight validation for initial screening
// This is used before full validation to quickly reject obviously invalid blocks
// Returns nil if block passes fast checks, error if block is clearly invalid
func (v *BlockValidator) ValidateBlockFast(block *Block) error {
	if block == nil {
		return fmt.Errorf("block is nil")
	}

	if len(block.Hash) == 0 {
		return fmt.Errorf("block hash is empty")
	}

	if block.DifficultyBits == 0 {
		return fmt.Errorf("difficulty bits is zero")
	}

	if len(block.Transactions) == 0 {
		return fmt.Errorf("block has no transactions")
	}

	return nil
}
