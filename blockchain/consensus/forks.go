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
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sort"
	"sync"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/nogopow"
)

var (
	ErrUnknownParent        = errors.New("unknown parent")
	ErrDuplicateBlock       = errors.New("duplicate block")
	ErrCycleDetected        = errors.New("cycle detected in chain")
	ErrMissingAncestor      = errors.New("missing ancestor block")
	ErrBadGenesisHeader     = errors.New("bad genesis header")
	ErrBadGenesisVersion    = errors.New("bad genesis version")
	ErrBadHeightLinkage     = errors.New("bad height linkage")
	ErrBadPrevHashLink      = errors.New("bad prevhash linkage")
	ErrReorgDepthExceeded   = errors.New("reorg depth exceeds maximum allowed")
	ErrRollbackFailed       = errors.New("rollback failed")
	ErrStateRecomputeFailed = errors.New("state recomputation failed")
	ErrDiskWriteFailed      = errors.New("disk write failed")
	ErrExecutionFailed      = errors.New("execution failed")
)

type (
	Block           = core.Block
	Account         = core.Account
	ConsensusParams = config.ConsensusParams
	ChainStore      = core.ChainStore
	EventSink       = core.EventSink
	WSEvent         = core.WSEvent
	TxLocation      = core.TxLocation
	AddressTxEntry  = core.AddressTxEntry
)

type ForkHandler struct {
	consensus     ConsensusParams
	chainID       uint64
	maxReorgDepth int
	mu            sync.RWMutex
	blocks        []*Block
	blocksByHash  map[string]*Block
	state         map[string]Account
	bestTipHash   string
	canonicalWork *big.Int
	txIndex       map[string]TxLocation
	addressIndex  map[string][]AddressTxEntry
	store         ChainStore
	events        EventSink
}

type ForkChoiceResult struct {
	SwitchedChain  bool
	NewTipHash     string
	NewHeight      uint64
	NewWork        *big.Int
	IsReorg        bool
	ReorgDepth     uint64
	CommonAncestor *Block
}

func NewForkHandler(
	consensus ConsensusParams,
	chainID uint64,
	maxReorgDepth int,
	store ChainStore,
	events EventSink,
) *ForkHandler {
	return &ForkHandler{
		consensus:     consensus,
		chainID:       chainID,
		maxReorgDepth: maxReorgDepth,
		store:         store,
		events:        events,
		blocksByHash:  make(map[string]*Block),
		state:         make(map[string]Account),
		txIndex:       make(map[string]TxLocation),
		addressIndex:  make(map[string][]AddressTxEntry),
		canonicalWork: big.NewInt(0),
	}
}

func (fh *ForkHandler) UpdateConsensus(consensus ConsensusParams) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.consensus = consensus
}

func (fh *ForkHandler) SetChainState(
	blocks []*Block,
	blocksByHash map[string]*Block,
	state map[string]Account,
	bestTipHash string,
	canonicalWork *big.Int,
) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.blocks = blocks
	fh.blocksByHash = blocksByHash
	fh.state = state
	fh.bestTipHash = bestTipHash
	fh.canonicalWork = canonicalWork
}

func (fh *ForkHandler) GetChainState() ([]*Block, map[string]*Block, map[string]Account, string, *big.Int) {
	fh.mu.RLock()
	defer fh.mu.RUnlock()
	return fh.blocks, fh.blocksByHash, fh.state, fh.bestTipHash, fh.canonicalWork
}

func (fh *ForkHandler) HandleBlock(block *Block) (*ForkChoiceResult, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}

	if block.Height == 0 {
		return nil, errors.New("external genesis blocks not accepted")
	}

	if err := fh.validateBlockHeader(block); err != nil {
		return nil, fmt.Errorf("header validation failed: %w", err)
	}

	parentHashHex := hex.EncodeToString(block.Header.PrevHash)
	var parent *Block

	fh.mu.RLock()
	if parentBlock, ok := fh.blocksByHash[parentHashHex]; ok {
		parent = parentBlock
	}
	fh.mu.RUnlock()

	if parent == nil && block.Height > 0 {
		return nil, ErrUnknownParent
	}

	if err := validateBlockPoWNogoPow(fh.consensus, block, parent); err != nil {
		return nil, fmt.Errorf("POW validation failed: %w", err)
	}

	if fh.consensus.DifficultyEnable && parent != nil {
		tempPath := []*Block{parent, block}
		if err := validateDifficultyNogoPow(fh.consensus, tempPath, 1); err != nil {
			log.Printf("ForkHandler: rejecting block height=%d hash=%s due to difficulty validation failure: %v",
				block.Height, hex.EncodeToString(block.Hash), err)
			return nil, fmt.Errorf("difficulty validation failed: %w", err)
		}
	}

	for _, tx := range block.Transactions {
		if tx.ChainID == 0 {
			tx.ChainID = fh.chainID
		}
		if tx.ChainID != fh.chainID {
			return nil, fmt.Errorf("wrong chainId: %d", tx.ChainID)
		}
		if err := tx.VerifyForConsensus(fh.consensus, block.Height); err != nil {
			return nil, fmt.Errorf("transaction validation failed: %w", err)
		}
	}

	fh.mu.Lock()
	defer fh.mu.Unlock()

	hashHex := hex.EncodeToString(block.Hash)
	if _, exists := fh.blocksByHash[hashHex]; exists {
		log.Printf("ForkHandler: rejecting block height=%d hash=%s (duplicate)", block.Height, hashHex)
		return nil, ErrDuplicateBlock
	}

	if _, ok := fh.blocksByHash[parentHashHex]; !ok {
		log.Printf("ForkHandler: rejecting block height=%d hash=%s (unknown parent %s)",
			block.Height, hashHex, parentHashHex)
		return nil, ErrUnknownParent
	}

	fh.blocksByHash[hashHex] = block

	path, state, newWork, err := fh.computeCanonicalForTipLocked(hashHex)
	if err != nil {
		delete(fh.blocksByHash, hashHex)
		return nil, fmt.Errorf("canonical chain computation failed: %w", err)
	}

	currentTip := fh.bestTipHash
	currentWork := fh.canonicalWork
	if currentWork == nil {
		currentWork = big.NewInt(0)
	}

	choice := fh.selectForkChoice(newWork, hashHex, currentWork, currentTip)
	if !choice.AcceptNewBlock {
		delete(fh.blocksByHash, hashHex)
		log.Printf("ForkHandler: block not better than current chain height=%d hash=%s", block.Height, hashHex)
		return &ForkChoiceResult{
			SwitchedChain: false,
			NewTipHash:    currentTip,
			IsReorg:       false,
		}, nil
	}

	currentHeight := uint64(0)
	if len(fh.blocks) > 0 {
		currentHeight = fh.blocks[len(fh.blocks)-1].Height
	}
	wasExtension := (parentHashHex == currentTip && block.Height == currentHeight+1)

	result := &ForkChoiceResult{
		SwitchedChain: !wasExtension,
		NewTipHash:    hashHex,
		NewHeight:     block.Height,
		NewWork:       newWork,
		IsReorg:       !wasExtension,
	}

	if !wasExtension {
		if parent != nil {
			result.CommonAncestor = parent
		}
		if block.Height > currentHeight {
			result.ReorgDepth = block.Height - currentHeight
		}
	}

	var execErr error
	if wasExtension {
		log.Printf("[FORKS] extending chain height=%d -> %d hash=%s", currentHeight, block.Height, hashHex)
		execErr = fh.appendBlockToChainLocked(block)
	} else {
		log.Printf("[REORG] switching from height=%d tip=%s to height=%d tip=%s",
			currentHeight, currentTip, block.Height, hashHex)
		execErr = fh.reorganizeChainLocked(block, parent, newWork, path, state)
	}

	if execErr != nil {
		delete(fh.blocksByHash, hashHex)
		log.Printf("ForkHandler: execution failed height=%d hash=%s error=%v", block.Height, hashHex, execErr)
		return nil, fmt.Errorf("%w: %w", ErrExecutionFailed, execErr)
	}

	if fh.events != nil {
		fh.events.Publish(WSEvent{
			Type: "new_block",
			Data: map[string]any{
				"height":         block.Height,
				"hash":           hashHex,
				"prevHash":       parentHashHex,
				"difficultyBits": block.Header.DifficultyBits,
				"txCount":        len(block.Transactions),
				"addresses":      addressesForBlock(block),
			},
		})

		if !wasExtension {
			fh.events.Publish(WSEvent{
				Type: "reorg",
				Data: map[string]any{
					"oldTip": currentTip,
					"newTip": hashHex,
					"depth":  result.ReorgDepth,
				},
			})
		}
	}

	return result, nil
}

type forkChoiceDecision struct {
	AcceptNewBlock bool
	Reason         string
}

func (fh *ForkHandler) selectForkChoice(newWork *big.Int, newTipHash string, currentWork *big.Int, currentTip string) *forkChoiceDecision {
	workComparison := newWork.Cmp(currentWork)

	if workComparison > 0 {
		return &forkChoiceDecision{
			AcceptNewBlock: true,
			Reason:         "more cumulative work",
		}
	}

	if workComparison == 0 {
		newHashInt := new(big.Int).SetBytes([]byte(newTipHash))
		currentHashInt := new(big.Int).SetBytes([]byte(currentTip))

		if newHashInt.Cmp(currentHashInt) < 0 {
			return &forkChoiceDecision{
				AcceptNewBlock: true,
				Reason:         "equal work but smaller hash (tie-break)",
			}
		}

		return &forkChoiceDecision{
			AcceptNewBlock: false,
			Reason:         "equal work but larger hash (lost tie-break)",
		}
	}

	return &forkChoiceDecision{
		AcceptNewBlock: false,
		Reason:         "less cumulative work",
	}
}

func (fh *ForkHandler) validateBlockHeader(block *Block) error {
	if expected := blockVersionForHeight(fh.consensus, block.Height); block.Header.Version != expected {
		return fmt.Errorf("bad block version at %d: expected %d got %d", block.Height, expected, block.Header.Version)
	}

	if len(block.Hash) == 0 {
		return errors.New("missing block hash")
	}

	if block.Header.DifficultyBits == 0 {
		return errors.New("missing difficultyBits")
	}

	if block.Header.DifficultyBits > maxDifficultyBits {
		return fmt.Errorf("difficultyBits out of range: %d", block.Header.DifficultyBits)
	}

	return nil
}

func (fh *ForkHandler) computeCanonicalForTipLocked(tipHashHex string) ([]*Block, map[string]Account, *big.Int, error) {
	var rev []*Block
	seen := make(map[string]struct{})
	cur := tipHashHex

	for {
		if _, ok := seen[cur]; ok {
			return nil, nil, nil, ErrCycleDetected
		}
		seen[cur] = struct{}{}

		b, ok := fh.blocksByHash[cur]
		if !ok {
			return nil, nil, nil, ErrMissingAncestor
		}
		rev = append(rev, b)

		if b.Height == 0 || len(b.Header.PrevHash) == 0 {
			break
		}
		cur = hex.EncodeToString(b.Header.PrevHash)
	}

	path := make([]*Block, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		path = append(path, rev[i])
	}

	state := make(map[string]Account)
	work := big.NewInt(0)

	for i, b := range path {
		if i == 0 {
			if b.Height != 0 || len(b.Header.PrevHash) != 0 {
				return nil, nil, nil, ErrBadGenesisHeader
			}
			if expected := blockVersionForHeight(fh.consensus, 0); b.Header.Version != expected {
				return nil, nil, nil, ErrBadGenesisVersion
			}
		} else {
			prev := path[i-1]
			if b.Height != prev.Height+1 {
				return nil, nil, nil, ErrBadHeightLinkage
			}
			if !bytes.Equal(b.Header.PrevHash, prev.Hash) {
				return nil, nil, nil, ErrBadPrevHashLink
			}
			if fh.consensus.DifficultyEnable {
				if err := validateDifficultyNogoPow(fh.consensus, path, i); err != nil {
					return nil, nil, nil, err
				}
			}
			if expected := blockVersionForHeight(fh.consensus, b.Height); b.Header.Version != expected {
				return nil, nil, nil, fmt.Errorf("bad block version at %d: expected %d got %d", b.Height, expected, b.Header.Version)
			}
		}

		if err := applyBlockToState(fh.consensus, state, b); err != nil {
			return nil, nil, nil, fmt.Errorf("state apply failed at height %d: %w", b.Height, err)
		}

		work.Add(work, WorkForDifficultyBits(b.Header.DifficultyBits))
	}

	return path, state, work, nil
}

func (fh *ForkHandler) appendBlockToChainLocked(b *Block) error {
	fh.blocks = append(fh.blocks, b)

	if err := applyBlockToState(fh.consensus, fh.state, b); err != nil {
		return fmt.Errorf("state apply failed: %w", err)
	}

	fh.indexBlockLocked(b)

	fh.bestTipHash = hex.EncodeToString(b.Hash)
	if fh.canonicalWork == nil {
		fh.canonicalWork = big.NewInt(0)
	}
	fh.canonicalWork.Add(fh.canonicalWork, WorkForDifficultyBits(b.Header.DifficultyBits))

	if fh.store != nil {
		if err := fh.store.AppendCanonical(b); err != nil {
			return fmt.Errorf("%w: %w", ErrDiskWriteFailed, err)
		}
	}

	log.Printf("[FORKS] chain extended height=%d hash=%s", b.Height, hex.EncodeToString(b.Hash))
	return nil
}

func (fh *ForkHandler) reorganizeChainLocked(newBlock *Block, parent *Block, newWork *big.Int, path []*Block, state map[string]Account) error {
	currentHeight := fh.blocks[len(fh.blocks)-1].Height
	targetHeight := parent.Height

	reorgDepth := currentHeight - targetHeight
	if reorgDepth > uint64(fh.maxReorgDepth) {
		return fmt.Errorf("%w: reorg depth %d exceeds maximum allowed %d (security limit)",
			ErrReorgDepthExceeded, reorgDepth, fh.maxReorgDepth)
	}

	log.Printf("[REORG] starting chain reorganization from height=%d to height=%d depth=%d",
		currentHeight, newBlock.Height, reorgDepth)

	if err := fh.rollbackToHeightInternalLocked(targetHeight); err != nil {
		return fmt.Errorf("%w: %w", ErrRollbackFailed, err)
	}

	currentTipHash := hex.EncodeToString(fh.blocks[len(fh.blocks)-1].Hash)
	expectedParentHash := hex.EncodeToString(parent.Hash)
	if currentTipHash != expectedParentHash {
		return fmt.Errorf("rollback sanity check failed: expected parent %s, got %s",
			expectedParentHash, currentTipHash)
	}

	fh.blocks = append(fh.blocks, newBlock)
	fh.state = state
	fh.reindexAllTxsLocked()
	fh.reindexAllAddressTxsLocked()

	fh.bestTipHash = hex.EncodeToString(newBlock.Hash)
	fh.canonicalWork = new(big.Int).Set(newWork)

	if fh.store != nil {
		if err := fh.store.RewriteCanonical(fh.blocks); err != nil {
			return fmt.Errorf("%w: %w", ErrDiskWriteFailed, err)
		}
	}

	log.Printf("[REORG] completed successfully new tip height=%d hash=%s work=%v",
		newBlock.Height, hex.EncodeToString(newBlock.Hash), newWork)
	return nil
}

func (fh *ForkHandler) rollbackToHeightInternalLocked(targetHeight uint64) error {
	if targetHeight >= fh.blocks[len(fh.blocks)-1].Height {
		return fmt.Errorf("cannot rollback to height %d, current height is %d",
			targetHeight, fh.blocks[len(fh.blocks)-1].Height)
	}

	log.Printf("[REORG] rolling back from height %d to %d",
		fh.blocks[len(fh.blocks)-1].Height, targetHeight)

	for i := len(fh.blocks) - 1; i > int(targetHeight); i-- {
		block := fh.blocks[i]
		hashHex := hex.EncodeToString(block.Hash)
		delete(fh.blocksByHash, hashHex)
		fh.unindexBlockLocked(block)
	}

	fh.blocks = fh.blocks[:targetHeight+1]

	restoredState, err := fh.recomputeStateFromGenesisLocked()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrStateRecomputeFailed, err)
	}
	fh.state = restoredState

	totalWork := big.NewInt(0)
	for _, block := range fh.blocks {
		totalWork.Add(totalWork, WorkForDifficultyBits(block.Header.DifficultyBits))
	}
	fh.canonicalWork = totalWork

	fh.bestTipHash = hex.EncodeToString(fh.blocks[len(fh.blocks)-1].Hash)

	return nil
}

func (fh *ForkHandler) recomputeStateFromGenesisLocked() (map[string]Account, error) {
	state := make(map[string]Account)

	for _, b := range fh.blocks {
		if err := applyBlockToState(fh.consensus, state, b); err != nil {
			return nil, fmt.Errorf("state replay at height %d failed: %w", b.Height, err)
		}
	}

	log.Printf("[REORG] state recomputed from genesis height=%d accounts=%d",
		fh.blocks[len(fh.blocks)-1].Height, len(state))
	return state, nil
}

func (fh *ForkHandler) indexBlockLocked(b *Block) {
	fh.indexTxsForBlockLocked(b)
	fh.indexAddressTxsForBlockLocked(b)
}

func (fh *ForkHandler) indexTxsForBlockLocked(b *Block) {
	if fh.txIndex == nil {
		fh.txIndex = make(map[string]TxLocation)
	}

	hashHex := hex.EncodeToString(b.Hash)
	for i, tx := range b.Transactions {
		if tx.Type != TxTransfer {
			continue
		}

		txid, err := TxIDHexForConsensus(tx, fh.consensus, b.Height)
		if err != nil {
			continue
		}

		fh.txIndex[txid] = TxLocation{
			Height:       b.Height,
			BlockHashHex: hashHex,
			Index:        i,
		}
	}
}

func (fh *ForkHandler) indexAddressTxsForBlockLocked(b *Block) {
	if fh.addressIndex == nil {
		fh.addressIndex = make(map[string][]AddressTxEntry)
	}

	hashHex := hex.EncodeToString(b.Hash)
	for i, tx := range b.Transactions {
		if tx.Type != TxTransfer {
			continue
		}

		txid, err := TxIDHexForConsensus(tx, fh.consensus, b.Height)
		if err != nil {
			continue
		}

		fromAddr, err := tx.FromAddress()
		if err != nil {
			continue
		}

		entry := AddressTxEntry{
			TxID: txid,
			Location: TxLocation{
				Height:       b.Height,
				BlockHashHex: hashHex,
				Index:        i,
			},
			FromAddr:  fromAddr,
			ToAddress: tx.ToAddress,
			Amount:    tx.Amount,
			Fee:       tx.Fee,
			Nonce:     tx.Nonce,
		}

		fh.addressIndex[fromAddr] = append(fh.addressIndex[fromAddr], entry)
		if tx.ToAddress != fromAddr {
			fh.addressIndex[tx.ToAddress] = append(fh.addressIndex[tx.ToAddress], entry)
		}
	}
}

func (fh *ForkHandler) reindexAllTxsLocked() {
	fh.txIndex = make(map[string]TxLocation)
	for _, block := range fh.blocks {
		fh.indexTxsForBlockLocked(block)
	}
}

func (fh *ForkHandler) reindexAllAddressTxsLocked() {
	fh.addressIndex = make(map[string][]AddressTxEntry)
	for _, block := range fh.blocks {
		fh.indexAddressTxsForBlockLocked(block)
	}
}

func (fh *ForkHandler) unindexBlockLocked(b *Block) {
	if fh.txIndex == nil {
		return
	}

	for _, tx := range b.Transactions {
		if tx.Type != TxTransfer {
			continue
		}
		txid, err := TxIDHexForConsensus(tx, fh.consensus, b.Height)
		if err != nil {
			continue
		}
		delete(fh.txIndex, txid)
	}

	if fh.addressIndex != nil {
		fh.reindexAllAddressTxsLocked()
	}
}

func (fh *ForkHandler) GetBlockByHash(hashHex string) (*Block, bool) {
	fh.mu.RLock()
	defer fh.mu.RUnlock()
	b, ok := fh.blocksByHash[hashHex]
	return b, ok
}

func (fh *ForkHandler) GetBlocksFromHeight(height uint64, count int) []*Block {
	fh.mu.RLock()
	defer fh.mu.RUnlock()

	if count <= 0 {
		count = 100
	}
	if height >= uint64(len(fh.blocks)) {
		return nil
	}

	end := height + uint64(count)
	if end > uint64(len(fh.blocks)) {
		end = uint64(len(fh.blocks))
	}

	result := make([]*Block, 0, end-height)
	for _, b := range fh.blocks[height:end] {
		result = append(result, b)
	}
	return result
}

func (fh *ForkHandler) GetKnownTips() []string {
	fh.mu.RLock()
	defer fh.mu.RUnlock()

	var tips []string
	isParent := make(map[string]struct{})

	for _, b := range fh.blocksByHash {
		if len(b.Header.PrevHash) > 0 {
			isParent[hex.EncodeToString(b.Header.PrevHash)] = struct{}{}
		}
	}

	for h := range fh.blocksByHash {
		if _, ok := isParent[h]; !ok {
			tips = append(tips, h)
		}
	}

	sort.Strings(tips)
	return tips
}

func (fh *ForkHandler) GetBestTip() (string, uint64, *big.Int) {
	fh.mu.RLock()
	defer fh.mu.RUnlock()

	height := uint64(0)
	if len(fh.blocks) > 0 {
		height = fh.blocks[len(fh.blocks)-1].Height
	}

	work := big.NewInt(0)
	if fh.canonicalWork != nil {
		work.Set(fh.canonicalWork)
	}

	return fh.bestTipHash, height, work
}

func validateDifficultyNogoPow(consensus ConsensusParams, path []*Block, idx int) error {
	if idx <= 0 || idx >= len(path) {
		return nil
	}

	if idx == 0 {
		if path[0].Header.DifficultyBits != consensus.GenesisDifficultyBits {
			return fmt.Errorf("bad genesis difficulty: expected %d got %d",
				consensus.GenesisDifficultyBits, path[0].Header.DifficultyBits)
		}
		return nil
	}

	currentBlock := path[idx]
	parentBlock := path[idx-1]

	if currentBlock.Header.DifficultyBits < consensus.MinDifficultyBits {
		return fmt.Errorf("difficulty %d below min %d", currentBlock.Header.DifficultyBits, consensus.MinDifficultyBits)
	}
	if currentBlock.Header.DifficultyBits > consensus.MaxDifficultyBits {
		return fmt.Errorf("difficulty %d above max %d", currentBlock.Header.DifficultyBits, consensus.MaxDifficultyBits)
	}

	if consensus.DifficultyEnable {
		consensusParams := &config.ConsensusParams{
			BlockTimeTargetSeconds:       15,
			MaxDifficultyChangePercent:   20,
			MinDifficulty:                1,
		}
		adjuster := nogopow.NewDifficultyAdjuster(consensusParams)

		var parentHash nogopow.Hash
		if len(parentBlock.Hash) > 0 {
			copy(parentHash[:], parentBlock.Hash)
		} else {
			copy(parentHash[:], parentBlock.Header.PrevHash)
		}

		parentHeader := &nogopow.Header{
			Number:     big.NewInt(int64(parentBlock.Height)),
			Time:       uint64(parentBlock.Header.TimestampUnix),
			Difficulty: big.NewInt(int64(parentBlock.Header.DifficultyBits)),
			ParentHash: parentHash,
		}

		expectedDifficulty := adjuster.CalcDifficulty(uint64(currentBlock.Header.TimestampUnix), parentHeader)
		actualDifficulty := big.NewInt(int64(currentBlock.Header.DifficultyBits))

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

func addressesForBlock(b *Block) []string {
	addresses := make(map[string]struct{})
	for _, tx := range b.Transactions {
		if tx.Type == TxTransfer {
			fromAddr, err := tx.FromAddress()
			if err == nil {
				addresses[fromAddr] = struct{}{}
			}
			addresses[tx.ToAddress] = struct{}{}
		} else if tx.Type == TxCoinbase {
			addresses[tx.ToAddress] = struct{}{}
		}
	}

	result := make([]string, 0, len(addresses))
	for addr := range addresses {
		result = append(result, addr)
	}
	return result
}

func WorkForDifficultyBits(bits uint32) *big.Int {
	if bits > 256 {
		bits = 256
	}
	if bits == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Lsh(big.NewInt(1), uint(bits))
}

func blockVersionForHeight(consensus ConsensusParams, height uint64) uint32 {
	if consensus.MerkleRootActive(height) || consensus.BinaryEncodingActive(height) {
		return 2
	}
	return 1
}

func TxIDHexForConsensus(tx Transaction, p ConsensusParams, height uint64) (string, error) {
	h, err := txSigningHashForConsensus(tx, p, height)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h), nil
}

func TxIDHex(tx Transaction) (string, error) {
	h, err := tx.SigningHash()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h), nil
}
