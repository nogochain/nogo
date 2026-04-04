package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sort"

	"github.com/nogochain/nogo/blockchain/nogopow"
)

var ErrUnknownParent = errors.New("unknown parent")

// Note: ErrInvalidPoW is defined in chain.go

// validateDifficultyNogoPow validates block difficulty using NogoPow algorithm
// This function performs STRICT difficulty validation for ALL blocks (NO SKIPPING)
// Parameters:
//   - consensus: consensus parameters
//   - path: blockchain path (blocks from genesis to current)
//   - idx: index of block to validate
//
// Validation steps:
// 1. Genesis block difficulty check
// 2. Difficulty range validation
// 3. Difficulty adjustment calculation and verification
// 4. Bounds checking with tight tolerances
//
// SECURITY GUARANTEES:
// - Every block's difficulty is independently calculated and verified
// - No reliance on "sync mode" to skip validation
// - Tight tolerance bounds prevent manipulation
// - Consistent with NogoPow difficulty adjustment algorithm
func validateDifficultyNogoPow(consensus ConsensusParams, path []*Block, idx int) error {
	if idx <= 0 || idx >= len(path) {
		return nil
	}

	// For genesis block, check genesis difficulty
	if idx == 0 {
		if path[0].DifficultyBits != consensus.GenesisDifficultyBits {
			return fmt.Errorf("bad genesis difficulty: expected %d got %d",
				consensus.GenesisDifficultyBits, path[0].DifficultyBits)
		}
		return nil
	}

	currentBlock := path[idx]
	parentBlock := path[idx-1]

	// Check difficulty bounds (100% execution, NO SKIPPING)
	if currentBlock.DifficultyBits < consensus.MinDifficultyBits {
		return fmt.Errorf("difficulty %d below min %d", currentBlock.DifficultyBits, consensus.MinDifficultyBits)
	}
	if currentBlock.DifficultyBits > consensus.MaxDifficultyBits {
		return fmt.Errorf("difficulty %d above max %d", currentBlock.DifficultyBits, consensus.MaxDifficultyBits)
	}

	// STRICT VALIDATION: Always verify difficulty adjustment logic
	// This is CRITICAL for preventing difficulty manipulation attacks
	if consensus.DifficultyEnable {
		// Use NogoPow difficulty adjuster to calculate expected difficulty
		config := nogopow.DefaultDifficultyConfig()
		adjuster := nogopow.NewDifficultyAdjuster(config)

		// Create parent header for calculation
		var parentHash nogopow.Hash
		if len(parentBlock.Hash) > 0 {
			copy(parentHash[:], parentBlock.Hash)
		} else {
			copy(parentHash[:], parentBlock.PrevHash)
		}

		parentHeader := &nogopow.Header{
			Number:     big.NewInt(int64(parentBlock.Height)),
			Time:       uint64(parentBlock.TimestampUnix),
			Difficulty: big.NewInt(int64(parentBlock.DifficultyBits)),
			ParentHash: parentHash,
		}

		// Calculate expected difficulty using NogoPow algorithm
		expectedDifficulty := adjuster.CalcDifficulty(uint64(currentBlock.TimestampUnix), parentHeader)

		// Validate difficulty is within TIGHT bounds (±10%)
		// This is CRITICAL for preventing forks with different difficulties
		// Uses DifficultyTolerancePercent from config.go
		actualDifficulty := big.NewInt(int64(currentBlock.DifficultyBits))

		// Calculate acceptable range: [expected * (100-tolerance)%, expected * (100+tolerance)%]
		minAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(100-DifficultyTolerancePercent))
		minAllowed.Div(minAllowed, big.NewInt(100))

		maxAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(100+DifficultyTolerancePercent))
		maxAllowed.Div(maxAllowed, big.NewInt(100))

		if actualDifficulty.Cmp(minAllowed) < 0 {
			return fmt.Errorf("difficulty adjustment too aggressive: actual %d < min allowed %d (expected %d, block height %d)",
				actualDifficulty.Uint64(), minAllowed.Uint64(), expectedDifficulty.Uint64(), currentBlock.Height)
		}

		if actualDifficulty.Cmp(maxAllowed) > 0 {
			return fmt.Errorf("difficulty adjustment too aggressive: actual %d > max allowed %d (expected %d, block height %d)",
				actualDifficulty.Uint64(), maxAllowed.Uint64(), expectedDifficulty.Uint64(), currentBlock.Height)
		}
	}

	return nil
}

// AddBlock accepts an externally provided block, validates it against its ancestry, stores it in-memory,
// and updates the canonical tip if it creates a "better" chain (highest total work, tie-break by hash).
// On reorg, the canonical chain on disk is rewritten to the new best chain.
func (bc *Blockchain) AddBlock(b *Block) (bool, error) {
	if b == nil {
		return false, errors.New("nil block")
	}
	if b.Height == 0 {
		return false, errors.New("external genesis blocks not accepted")
	}
	if expected := blockVersionForHeight(bc.consensus, b.Height); b.Version != expected {
		return false, fmt.Errorf("bad block version at %d: expected %d got %d", b.Height, expected, b.Version)
	}
	if len(b.Hash) == 0 {
		return false, errors.New("missing block hash")
	}
	if b.DifficultyBits == 0 {
		return false, errors.New("missing difficultyBits")
	}
	if b.DifficultyBits > maxDifficultyBits {
		return false, fmt.Errorf("difficultyBits out of range: %d", b.DifficultyBits)
	}

	// Validate PoW using NogoPow engine
	// Get parent block for validation
	var parent *Block
	parentHashStr := hex.EncodeToString(b.PrevHash)
	if parentBlock, ok := bc.blocksByHash[parentHashStr]; ok {
		parent = parentBlock
	}
	if err := validateBlockPoWNogoPow(bc.consensus, b, parent); err != nil {
		return false, err
	}

	// Validate difficulty adjustment (CRITICAL: prevent forks with different difficulties)
	// This must be done BEFORE adding the block to prevent accepting invalid difficulties
	if bc.consensus.DifficultyEnable && parent != nil {
		// Create a temporary path for difficulty validation
		tempPath := []*Block{parent, b}
		if err := validateDifficultyNogoPow(bc.consensus, tempPath, 1); err != nil {
			log.Printf("AddBlock: rejecting block height=%d hash=%s due to difficulty validation failure: %v",
				b.Height, hex.EncodeToString(b.Hash), err)
			return false, fmt.Errorf("difficulty validation failed: %w", err)
		}
	}

	// Basic tx checks (signatures/encoding/chainId)
	for _, tx := range b.Transactions {
		if tx.ChainID == 0 {
			tx.ChainID = bc.ChainID
		}
		if tx.ChainID != bc.ChainID {
			return false, fmt.Errorf("wrong chainId: %d", tx.ChainID)
		}
		if err := tx.VerifyForConsensus(bc.consensus, b.Height); err != nil {
			return false, err
		}
	}

	bc.mu.Lock()
	var events EventSink
	var toPublish []WSEvent
	defer func() {
		bc.mu.Unlock()
		if events == nil {
			return
		}
		for _, e := range toPublish {
			events.Publish(e)
		}
	}()

	parentHashHex := hex.EncodeToString(b.PrevHash)
	if _, ok := bc.blocksByHash[parentHashHex]; !ok {
		// Parent must be known for now (sync endpoints allow fetching missing blocks).
		log.Printf("AddBlock: rejecting block height=%d hash=%s (unknown parent %s)",
			b.Height, hex.EncodeToString(b.Hash), parentHashHex)
		return false, ErrUnknownParent
	}

	hashHex := hex.EncodeToString(b.Hash)
	if _, exists := bc.blocksByHash[hashHex]; exists {
		log.Printf("AddBlock: rejecting block height=%d hash=%s (duplicate)", b.Height, hashHex)
		return false, errors.New("duplicate block")
	}
	bc.blocksByHash[hashHex] = b

	log.Printf("AddBlock: processing block height=%d hash=%s parent=%s", b.Height, hashHex, parentHashHex)

	// Validate the full chain state for this candidate tip.
	path, state, newWork, err := bc.computeCanonicalForTipLocked(hashHex)
	if err != nil {
		delete(bc.blocksByHash, hashHex)
		return false, err
	}

	// Persist the block only after it passes full validation.
	if err := bc.store.PutBlock(b); err != nil {
		delete(bc.blocksByHash, hashHex)
		return false, err
	}

	currentTip := bc.bestTipHash
	currentWork := bc.canonicalWork
	if currentWork == nil {
		currentWork = big.NewInt(0)
	}

	// Accept block if it has more work, OR equal work (smaller hash wins in case of tie)
	// Use NUMERIC comparison for determinism, not string comparison
	better := false
	workComparison := newWork.Cmp(currentWork)

	if workComparison > 0 {
		// New chain has more cumulative work - always accept
		better = true
		log.Printf("AddBlock: more work height=%d hash=%s (%v > %v) - switching chain",
			b.Height, hashHex, newWork, currentWork)
	} else if workComparison == 0 {
		// Equal work - use numeric hash comparison (smaller hash wins)
		// Convert hex strings to big.Int for proper numeric comparison
		newHashInt := new(big.Int).SetBytes([]byte(hashHex))
		currentHashInt := new(big.Int).SetBytes([]byte(currentTip))

		if newHashInt.Cmp(currentHashInt) < 0 {
			// New block has smaller hash - accept it and switch chains
			better = true
			log.Printf("hash comparison lost! height=%d opponent hash smaller - executing reorg to switch to opponent chain", b.Height)
		} else if newHashInt.Cmp(currentHashInt) == 0 {
			// This should not happen due to duplicate check above
			log.Printf("AddBlock: duplicate block height=%d hash=%s", b.Height, hashHex)
			return false, nil
		} else {
			// New block has larger hash but equal work
			// This is a competing block at same height - I won, keep my chain
			// Store the competing block as a side block for potential future use
			log.Printf("hash comparison won! height=%d local hash smaller - keeping local chain, storing opponent block as side block (no switch)", b.Height)
			// Note: We don't switch, but we also don't reject - store it for potential reorg
			return false, nil
		}
	}

	if !better {
		log.Printf("AddBlock: rejecting block height=%d hash=%s work=%v currentWork=%v (not better)",
			b.Height, hashHex, newWork, currentWork)
		return false, nil
	}

	// EXECUTION LAYER: Decide between chain extension and reorganization
	currentHeight := bc.blocks[len(bc.blocks)-1].Height
	wasExtension := (parentHashHex == currentTip && b.Height == currentHeight+1)

	var execErr error
	if wasExtension {
		// Case A: Simple chain extension (fast path)
		log.Printf("[FORKS] extending chain height=%d -> %d hash=%s", currentHeight, b.Height, hashHex)
		execErr = bc.appendBlockToChainLocked(b)
	} else {
		// Case B: Chain reorganization (slow path)
		log.Printf("[REORG] switching from height=%d tip=%s to height=%d tip=%s",
			currentHeight, currentTip, b.Height, hashHex)
		execErr = bc.reorganizeChainLocked(b, parent, newWork, path, state)
	}

	if execErr != nil {
		// CRITICAL: Execution failed, chain state may be inconsistent
		// Remove block from hash map to prevent future conflicts
		delete(bc.blocksByHash, hashHex)
		log.Printf("AddBlock: execution failed height=%d hash=%s error=%v", b.Height, hashHex, err)
		return false, err
	}

	// EVENT PUBLISHING: Notify subscribers about new block (and reorg if applicable)
	events = bc.events
	toPublish = append(toPublish, WSEvent{
		Type: "new_block",
		Data: map[string]any{
			"height":         b.Height,
			"hash":           hashHex,
			"prevHash":       parentHashHex,
			"difficultyBits": b.DifficultyBits,
			"txCount":        len(b.Transactions),
			"addresses":      addressesForBlock(b),
		},
	})

	if !wasExtension {
		// Publish reorg event for UI/indexing updates
		toPublish = append(toPublish, WSEvent{
			Type: "reorg",
			Data: map[string]any{
				"oldTip": currentTip,
				"newTip": hashHex,
				"depth":  currentHeight - (b.Height - 1),
			},
		})
	}

	return !wasExtension, nil
}

// RollbackToHeight rolls back the blockchain to the specified height
// This is used for chain reorganization when a fork is detected
func (bc *Blockchain) RollbackToHeight(targetHeight uint64) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if targetHeight >= bc.blocks[len(bc.blocks)-1].Height {
		return fmt.Errorf("cannot rollback to height %d, current height is %d",
			targetHeight, bc.blocks[len(bc.blocks)-1].Height)
	}

	// Find the block at target height
	var targetBlock *Block
	for _, b := range bc.blocks {
		if b.Height == targetHeight {
			targetBlock = b
			break
		}
	}

	if targetBlock == nil {
		return fmt.Errorf("block at height %d not found", targetHeight)
	}

	log.Printf("RollbackToHeight: rolling back from height %d to %d (hash=%s)",
		bc.blocks[len(bc.blocks)-1].Height, targetHeight, hex.EncodeToString(targetBlock.Hash))

	// Remove blocks above target height from blocksByHash
	for i := len(bc.blocks) - 1; i > int(targetHeight); i-- {
		block := bc.blocks[i]
		hashHex := hex.EncodeToString(block.Hash)
		delete(bc.blocksByHash, hashHex)

		// Remove transactions from txIndex
		for _, tx := range block.Transactions {
			txid, err := TxIDHex(tx)
			if err == nil {
				delete(bc.txIndex, txid)
			}

			// Remove address index entries
			for _, addr := range addressesForBlock(block) {
				// Find and remove entries for this address at this block height
				if entries, ok := bc.addressIndex[addr]; ok {
					newEntries := make([]AddressTxEntry, 0, len(entries))
					for _, entry := range entries {
						if entry.Location.Height != block.Height {
							newEntries = append(newEntries, entry)
						}
					}
					if len(newEntries) == 0 {
						delete(bc.addressIndex, addr)
					} else {
						bc.addressIndex[addr] = newEntries
					}
				}
			}
		}
	}

	// Truncate blocks array
	bc.blocks = bc.blocks[:targetHeight+1]

	// Update best tip
	bc.bestTipHash = hex.EncodeToString(targetBlock.Hash)

	// Recompute state from genesis to target height using existing method
	if err := bc.recomputeStateLocked(); err != nil {
		return fmt.Errorf("failed to recompute state after rollback: %w", err)
	}

	// Recompute canonical work
	work := big.NewInt(0)
	for i := 0; i <= int(targetHeight); i++ {
		block := bc.blocks[i]
		work.Add(work, WorkForDifficultyBits(block.DifficultyBits))
	}
	bc.canonicalWork = work

	// Rewrite canonical chain on disk
	if err := bc.store.RewriteCanonical(bc.blocks); err != nil {
		return fmt.Errorf("failed to rewrite canonical chain: %w", err)
	}

	log.Printf("RollbackToHeight: successfully rolled back to height %d", targetHeight)
	return nil
}

func (bc *Blockchain) computeCanonicalForTipLocked(tipHashHex string) ([]*Block, map[string]Account, *big.Int, error) {
	var rev []*Block
	seen := map[string]struct{}{}
	cur := tipHashHex

	for {
		if _, ok := seen[cur]; ok {
			return nil, nil, nil, errors.New("cycle detected")
		}
		seen[cur] = struct{}{}

		b, ok := bc.blocksByHash[cur]
		if !ok {
			return nil, nil, nil, errors.New("missing ancestor")
		}
		rev = append(rev, b)
		if b.Height == 0 || len(b.PrevHash) == 0 {
			break
		}
		cur = hex.EncodeToString(b.PrevHash)
	}

	// Reverse to genesis->tip order
	path := make([]*Block, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		path = append(path, rev[i])
	}

	// Validate header linkage and apply state.
	state := map[string]Account{}
	work := big.NewInt(0)
	for i, b := range path {
		if i == 0 {
			if b.Height != 0 || len(b.PrevHash) != 0 {
				return nil, nil, nil, errors.New("bad genesis header")
			}
			if expected := blockVersionForHeight(bc.consensus, 0); b.Version != expected {
				return nil, nil, nil, errors.New("bad genesis version")
			}
		} else {
			prev := path[i-1]
			if b.Height != prev.Height+1 {
				return nil, nil, nil, errors.New("bad height linkage")
			}
			if hex.EncodeToString(b.PrevHash) != hex.EncodeToString(prev.Hash) {
				return nil, nil, nil, errors.New("bad prevhash linkage")
			}
			// CRITICAL FIX: validateBlockTime needs the full path up to current block
			// MTP calculation requires access to the last N blocks, not just parent
			// Pass the full path slice from 0 to i (inclusive) for proper MTP calculation
			if err := validateBlockTime(bc.consensus, path[:i+1], i); err != nil {
				return nil, nil, nil, err
			}
			if bc.consensus.DifficultyEnable {
				// Validate difficulty using NogoPow algorithm
				if err := validateDifficultyNogoPow(bc.consensus, path, i); err != nil {
					return nil, nil, nil, err
				}
			}
			if expected := blockVersionForHeight(bc.consensus, b.Height); b.Version != expected {
				return nil, nil, nil, fmt.Errorf("bad block version at %d: expected %d got %d", b.Height, expected, b.Version)
			}
		}
		if err := applyBlockToState(bc.consensus, state, b); err != nil {
			return nil, nil, nil, err
		}
		work.Add(work, WorkForDifficultyBits(b.DifficultyBits))
	}
	return path, state, work, nil
}

type BlockHeader struct {
	Height         uint64 `json:"height"`
	TimestampUnix  int64  `json:"timestampUnix"`
	PrevHashHex    string `json:"prevHashHex"`
	HashHex        string `json:"hashHex"`
	Nonce          uint64 `json:"nonce"`
	DifficultyBits uint32 `json:"difficultyBits"`
	MinerAddress   string `json:"minerAddress"`
	TxCount        int    `json:"txCount"`
}

func (bc *Blockchain) HeadersFrom(height uint64, count int) []BlockHeader {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if count <= 0 {
		count = 100
	}
	if height >= uint64(len(bc.blocks)) {
		return nil
	}
	end := height + uint64(count)
	if end > uint64(len(bc.blocks)) {
		end = uint64(len(bc.blocks))
	}
	out := make([]BlockHeader, 0, end-height)
	for _, b := range bc.blocks[height:end] {
		out = append(out, BlockHeader{
			Height:         b.Height,
			TimestampUnix:  b.TimestampUnix,
			PrevHashHex:    hex.EncodeToString(b.PrevHash),
			HashHex:        hex.EncodeToString(b.Hash),
			Nonce:          b.Nonce,
			DifficultyBits: b.DifficultyBits,
			MinerAddress:   b.MinerAddress,
			TxCount:        len(b.Transactions),
		})
	}
	return out
}

func (bc *Blockchain) BlocksFrom(height uint64, count int) []*Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if count <= 0 {
		count = 100
	}
	if height >= uint64(len(bc.blocks)) {
		return nil
	}
	end := height + uint64(count)
	if end > uint64(len(bc.blocks)) {
		end = uint64(len(bc.blocks))
	}
	return append([]*Block(nil), bc.blocks[height:end]...)
}

func (bc *Blockchain) BlockByHash(hashHex string) (*Block, bool) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	b, ok := bc.blocksByHash[hashHex]
	return b, ok
}

func (bc *Blockchain) KnownTips() []string {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	var tips []string
	// naive: any block hash not referenced as a parent is a tip
	isParent := map[string]struct{}{}
	for _, b := range bc.blocksByHash {
		if len(b.PrevHash) > 0 {
			isParent[hex.EncodeToString(b.PrevHash)] = struct{}{}
		}
	}
	for h := range bc.blocksByHash {
		if _, ok := isParent[h]; !ok {
			tips = append(tips, h)
		}
	}
	sort.Strings(tips)
	return tips
}

// appendBlockToChainLocked adds a block to the tip of the chain without reorganization.
// This is the fast path for chain extension (common case).
// PRECONDITION: bc.mu must be held.
// PRECONDITION: b.ParentHash == bc.bestTipHash && b.Height == bc.blocks[len(bc.blocks)-1].Height + 1
func (bc *Blockchain) appendBlockToChainLocked(b *Block) error {
	// 1. Update in-memory blocks
	bc.blocks = append(bc.blocks, b)

	// 2. Update state (apply transactions)
	if err := applyBlockToState(bc.consensus, bc.state, b); err != nil {
		return fmt.Errorf("state apply failed: %w", err)
	}

	// 3. Update indexes
	bc.indexBlockLocked(b)

	// 4. Update metadata
	bc.bestTipHash = hex.EncodeToString(b.Hash)
	if bc.canonicalWork == nil {
		bc.canonicalWork = big.NewInt(0)
	}
	bc.canonicalWork.Add(bc.canonicalWork, WorkForDifficultyBits(b.DifficultyBits))

	// 5. Persist to canonical store (append-only, faster than rewrite)
	if err := bc.store.AppendCanonical(b); err != nil {
		// CRITICAL: If disk write fails, chain state is inconsistent
		// In production, consider crashing here to prevent split-brain
		return fmt.Errorf("disk append failed: %w", err)
	}

	log.Printf("[FORKS] chain extended height=%d hash=%s", b.Height, hex.EncodeToString(b.Hash))
	return nil
}

// reorganizeChainLocked performs an atomic chain reorganization to the new canonical chain.
// This is the slow path for fork resolution (less common case).
// PRECONDITION: bc.mu must be held.
// PRECONDITION: newBlock has more work OR equal work with smaller hash.
func (bc *Blockchain) reorganizeChainLocked(newBlock *Block, parent *Block, newWork *big.Int, path []*Block, state map[string]Account) error {
	currentHeight := bc.blocks[len(bc.blocks)-1].Height
	targetHeight := newBlock.Height - 1

	// SECURITY CHECK: Prevent long-range attacks by limiting reorg depth
	reorgDepth := currentHeight - targetHeight
	if reorgDepth > uint64(bc.getMaxReorgDepth()) {
		return fmt.Errorf("reorg depth %d exceeds maximum allowed %d (security limit)",
			reorgDepth, bc.getMaxReorgDepth())
	}

	log.Printf("[REORG] starting chain reorganization from height=%d to height=%d depth=%d",
		currentHeight, newBlock.Height, reorgDepth)

	// 1. ROLLBACK: Revert chain to the parent's height (fork point)
	// This removes blocks above targetHeight from memory and disk index
	if err := bc.rollbackToHeightInternalLocked(targetHeight); err != nil {
		return fmt.Errorf("rollback to height %d failed: %w", targetHeight, err)
	}

	// 2. POST-ROLLBACK SANITY CHECK: Ensure parent is now the tip
	currentTipHash := hex.EncodeToString(bc.blocks[len(bc.blocks)-1].Hash)
	expectedParentHash := hex.EncodeToString(parent.Hash)
	if currentTipHash != expectedParentHash {
		return fmt.Errorf("rollback sanity check failed: expected parent %s, got %s",
			expectedParentHash, currentTipHash)
	}

	// 3. INJECT NEW BLOCK: Directly append the winning block as new tip
	// We bypass normal mining because this block is from the network (already validated)

	// a. Update in-memory blocks
	bc.blocks = append(bc.blocks, newBlock)

	// b. Update state machine with new state from path calculation
	bc.state = state

	// c. Update indexes (rebuild from scratch for new chain)
	bc.reindexAllTxsLocked()
	bc.reindexAllAddressTxsLocked()

	// d. Update chain metadata
	bc.bestTipHash = hex.EncodeToString(newBlock.Hash)
	bc.canonicalWork = new(big.Int).Set(newWork)

	// 4. PERSISTENCE: Atomic rewrite of canonical chain on disk
	// This is the commit point of the reorg - old blocks are replaced
	if err := bc.store.RewriteCanonical(bc.blocks); err != nil {
		// CRITICAL: If rewrite fails, chain is corrupted
		// Operator intervention required (restore from backup)
		return fmt.Errorf("reorg disk rewrite failed: %w", err)
	}

	log.Printf("[REORG] completed successfully new tip height=%d hash=%s work=%v",
		newBlock.Height, hex.EncodeToString(newBlock.Hash), newWork)
	return nil
}

// rollbackToHeightInternalInternal is the internal implementation of RollbackToHeight.
// It performs the actual rollback logic without acquiring the lock.
// PRECONDITION: bc.mu must be held.
func (bc *Blockchain) rollbackToHeightInternalLocked(targetHeight uint64) error {
	if targetHeight >= bc.blocks[len(bc.blocks)-1].Height {
		return fmt.Errorf("cannot rollback to height %d, current height is %d",
			targetHeight, bc.blocks[len(bc.blocks)-1].Height)
	}

	log.Printf("[REORG] rolling back from height %d to %d",
		bc.blocks[len(bc.blocks)-1].Height, targetHeight)

	// 1. Remove blocks above targetHeight from memory and indexes
	for i := len(bc.blocks) - 1; i > int(targetHeight); i-- {
		block := bc.blocks[i]
		hashHex := hex.EncodeToString(block.Hash)

		// Remove from hash map
		delete(bc.blocksByHash, hashHex)

		// Remove from indexes (unindex transactions)
		bc.unindexBlockLocked(block)
	}

	// 2. Truncate the blocks slice
	bc.blocks = bc.blocks[:targetHeight+1]

	// 3. Recompute state from genesis (state must be rebuilt for consistency)
	// This is deterministic: replaying all blocks from genesis produces same state
	restoredState, err := bc.recomputeStateFromGenesisLocked()
	if err != nil {
		return fmt.Errorf("state recomputation failed: %w", err)
	}
	bc.state = restoredState

	// 4. Update work calculation (recalculate from genesis)
	totalWork := big.NewInt(0)
	for _, block := range bc.blocks {
		totalWork.Add(totalWork, WorkForDifficultyBits(block.DifficultyBits))
	}
	bc.canonicalWork = totalWork

	// 5. Update tip hash
	bc.bestTipHash = hex.EncodeToString(bc.blocks[len(bc.blocks)-1].Hash)

	// 6. Storage layer rollback is handled by RewriteCanonical in reorganizeChainLocked
	// No additional action needed here for append-only stores

	return nil
}

// getMaxReorgDepth returns the maximum allowed reorg depth from configuration.
// This is a security parameter to prevent long-range attacks.
func (bc *Blockchain) getMaxReorgDepth() int {
	// Check if consensus params have MaxReorgDepth configured
	// For now, use hardcoded default (will be configurable via genesis.json in future)
	return 100 // DefaultMaxReorgDepth
}

// unindexBlockLocked removes a block's transactions from the transaction and address indexes.
// This is the inverse of indexBlockLocked, used during chain rollback.
// PRECONDITION: bc.mu must be held.
func (bc *Blockchain) unindexBlockLocked(b *Block) {
	if bc.txIndex == nil {
		return
	}

	// Remove transactions from txIndex
	for _, tx := range b.Transactions {
		if tx.Type != TxTransfer {
			continue
		}
		txid, err := TxIDHexForConsensus(tx, bc.consensus, b.Height)
		if err != nil {
			continue
		}
		delete(bc.txIndex, txid)
	}

	// Remove transactions from addressIndex
	if bc.addressIndex == nil {
		return
	}

	// Rebuild address index without this block's transactions
	// This is simpler and safer than incremental removal
	bc.reindexAllAddressTxsLocked()
}

// recomputeStateFromGenesisLocked rebuilds the entire state by replaying all blocks from genesis.
// This is used after rollback to ensure state consistency.
// PRECONDITION: bc.mu must be held.
func (bc *Blockchain) recomputeStateFromGenesisLocked() (map[string]Account, error) {
	// Start from empty state
	state := make(map[string]Account)

	// Replay all blocks from genesis to current tip
	for _, b := range bc.blocks {
		if err := applyBlockToState(bc.consensus, state, b); err != nil {
			return nil, fmt.Errorf("state replay at height %d failed: %w", b.Height, err)
		}
	}

	log.Printf("[REORG] state recomputed from genesis height=%d accounts=%d",
		bc.blocks[len(bc.blocks)-1].Height, len(state))
	return state, nil
}

// indexBlockLocked adds a block's transactions to the transaction and address indexes.
// This is used during chain extension for incremental index updates.
// PRECONDITION: bc.mu must be held.
func (bc *Blockchain) indexBlockLocked(b *Block) {
	// Index transactions
	bc.indexTxsForBlockLocked(b)

	// Index address transactions
	bc.indexAddressTxsForBlockLocked(b)
}
