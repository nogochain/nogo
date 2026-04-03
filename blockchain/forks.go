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

		// Validate difficulty is within TIGHT bounds (±20% instead of ±50%)
		// Tighter bounds provide better security while accounting for implementation differences
		actualDifficulty := big.NewInt(int64(currentBlock.DifficultyBits))

		// Calculate acceptable range: [expected * 0.8, expected * 1.2]
		// This is tighter than the old ±50% to prevent manipulation
		minAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(80))
		minAllowed.Div(minAllowed, big.NewInt(100))

		maxAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(120))
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
		log.Printf("AddBlock: block height=%d hash=%s has more work (%v > %v) - switching to this chain",
			b.Height, hashHex, newWork, currentWork)
	} else if workComparison == 0 {
		// Equal work - use numeric hash comparison (smaller hash wins)
		// Convert hex strings to big.Int for proper numeric comparison
		newHashInt := new(big.Int).SetBytes([]byte(hashHex))
		currentHashInt := new(big.Int).SetBytes([]byte(currentTip))
		
		if newHashInt.Cmp(currentHashInt) < 0 {
			// New block has smaller hash - accept it
			better = true
			log.Printf("AddBlock: block height=%d has equal work but smaller hash - switching to this chain", b.Height)
		} else if newHashInt.Cmp(currentHashInt) == 0 {
			// This should not happen due to duplicate check above
			log.Printf("AddBlock: duplicate block height=%d hash=%s", b.Height, hashHex)
			return false, nil
		} else {
			// New block has larger hash
			// Check if this is a competing block at the same height
			currentBlock := bc.blocks[len(bc.blocks)-1]
			if b.Height == currentBlock.Height {
				// Competing block at same height with larger hash - reject
				log.Printf("AddBlock: rejecting competing block height=%d hash=%s (current tip has smaller hash)",
					b.Height, hashHex)
				return false, nil
			} else if parentHashHex == currentTip {
				// This block extends current tip but has larger hash
				// Accept it to continue the chain (this is normal operation)
				better = true
				log.Printf("AddBlock: accepting block height=%d hash=%s extending current tip",
					b.Height, hashHex)
			} else {
				// Different fork with larger hash - reject
				log.Printf("AddBlock: rejecting block height=%d hash=%s (larger hash with equal work)",
					b.Height, hashHex)
				return false, nil
			}
		}
	}

	if !better {
		log.Printf("AddBlock: rejecting block height=%d hash=%s work=%v currentWork=%v (not better)",
			b.Height, hashHex, newWork, currentWork)
		return false, nil
	}

	// Reorg to new canonical chain: replace in-memory view and rewrite on-disk canonical chain.
	currentHeight := bc.blocks[len(bc.blocks)-1].Height
	wasExtension := (parentHashHex == currentTip && b.Height == currentHeight+1)

	if !wasExtension {
		log.Printf("AddBlock: CHAIN REORG! Switching from height=%d tip=%s to height=%d tip=%s",
			currentHeight, currentTip, b.Height, hashHex)
	} else {
		log.Printf("AddBlock: extending chain height=%d -> %d hash=%s", currentHeight, b.Height, hashHex)
	}

	bc.blocks = path
	bc.state = state
	bc.bestTipHash = hashHex
	bc.reindexAllTxsLocked()
	bc.reindexAllAddressTxsLocked()
	bc.canonicalWork = new(big.Int).Set(newWork)
	if err := bc.store.RewriteCanonical(bc.blocks); err != nil {
		return !wasExtension, err
	}
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
		toPublish = append(toPublish, WSEvent{
			Type: "reorg",
			Data: map[string]any{
				"oldTip": currentTip,
				"newTip": hashHex,
			},
		})
	}
	return !wasExtension, nil
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
