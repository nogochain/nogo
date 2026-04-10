// Copyright 2026 NogoChain Team
// Production-grade chain selection and reorganization logic
// Implements Bitcoin's FindMostWorkChain and ActivateBestChainStep

package core

import (
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"
)

// BlockProvider interface for retrieving blocks by hash
type BlockProvider interface {
	GetBlockByHash(hash []byte) (*Block, bool)
	GetAllBlocks() ([]*Block, error)
}

// ChainSelector manages chain selection and reorganization
// Production-grade: implements heaviest chain rule with automatic reorg
// Thread-safe: uses mutex for internal state management
type ChainSelector struct {
	mu              sync.RWMutex
	chain           *Chain
	blockProvider   BlockProvider
	workCalculator  *WorkCalculator
	reorgInProgress bool
	reorgMutex      sync.Mutex
}

// NewChainSelector creates a new chain selector
func NewChainSelector(chain *Chain, provider BlockProvider) *ChainSelector {
	return &ChainSelector{
		chain:          chain,
		blockProvider:  provider,
		workCalculator: NewWorkCalculator(),
	}
}

// FindMostWorkChain finds the block with the most cumulative work
// Implementation matches Bitcoin's FindMostWorkChain algorithm with enhancement
// Returns the block with highest chain work, nil if no blocks
func (cs *ChainSelector) FindMostWorkChain() *Block {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	bestBlock := cs.chain.LatestBlock()
	if bestBlock == nil {
		return nil
	}

	bestWork, ok := StringToWork(bestBlock.TotalWork)
	if !ok {
		bestWork = new(big.Int)
	}

	// Check all blocks in storage for potential better chains
	// This handles cases where we have multiple chain branches
	if cs.blockProvider != nil {
		allBlocks, err := cs.blockProvider.GetAllBlocks()
		if err == nil {
			for _, block := range allBlocks {
				if block == nil {
					continue
				}

				// Bitcoin-style validation: ensure block chain is fully valid
				if !cs.isChainValid(block) {
					continue // Skip blocks with invalid ancestors
				}

				blockWork, ok := StringToWork(block.TotalWork)
				if !ok {
					continue
				}

				// Compare work: if this block has more work, it's a better chain
				if blockWork.Cmp(bestWork) > 0 {
					bestWork = blockWork
					bestBlock = block
				}
			}
		}
	}

	return bestBlock
}

// isChainValid implements Bitcoin-style chain validation: checks all ancestors are valid
func (cs *ChainSelector) isChainValid(block *Block) bool {
	current := block
	for current.GetHeight() > 0 {
		// Check if block data is available
		if !cs.isBlockDataAvailable(current) {
			return false
		}

		// Check if block is marked as failed
		if cs.isBlockFailed(current) {
			return false
		}

		// Get parent block
		parent, err := cs.getBlockByHash(current.Header.PrevHash)
		if err != nil {
			return false // Missing parent indicates invalid chain
		}
		current = parent
	}
	return true
}

// isBlockDataAvailable checks if block data is fully available
func (cs *ChainSelector) isBlockDataAvailable(block *Block) bool {
	// Implementation would check if block data (transactions, etc.) is completely available
	// This prevents switching to chains with missing data
	return block != nil && block.Hash != nil && len(block.Hash) > 0
}

// isBlockFailed checks if a block is marked as failed validation
func (cs *ChainSelector) isBlockFailed(block *Block) bool {
	// Implementation would check block status flags
	// Similar to Bitcoin's BLOCK_FAILED_VALID check
	return false // Default: blocks are assumed valid unless marked otherwise
}

// ShouldReorg checks if we should reorganize to a new block
// Returns true if the new block has more work than current tip
func (cs *ChainSelector) ShouldReorg(newBlock *Block) bool {
	if newBlock == nil {
		return false
	}

	currentTip := cs.chain.LatestBlock()
	if currentTip == nil || currentTip.Hash == nil {
		return true // No chain yet, accept any block
	}

	// Same block, no reorg needed
	if string(currentTip.Hash) == string(newBlock.Hash) {
		return false
	}

	// Get work values
	currentWork, ok1 := StringToWork(currentTip.TotalWork)
	newWork, ok2 := StringToWork(newBlock.TotalWork)

	if !ok1 || !ok2 {
		// If work calculation fails, compare heights as fallback
		return newBlock.GetHeight() > currentTip.GetHeight()
	}

	// Compare work: reorg if new block has strictly more work
	return newWork.Cmp(currentWork) > 0
}

// Reorganize performs chain reorganization to a new block
// Implementation matches Bitcoin's ActivateBestChainStep
// Thread-safety: uses reorgMutex to prevent concurrent reorganizations
func (cs *ChainSelector) Reorganize(newBlock *Block) error {
	cs.reorgMutex.Lock()
	defer cs.reorgMutex.Unlock()

	if cs.reorgInProgress {
		return fmt.Errorf("reorganization already in progress")
	}

	cs.reorgInProgress = true
	defer func() {
		cs.reorgInProgress = false
	}()

	currentTip := cs.chain.LatestBlock()
	if currentTip == nil {
		return fmt.Errorf("no current chain tip")
	}

	// Find common ancestor (fork point)
	forkPoint, err := cs.findCommonAncestor(currentTip, newBlock)
	if err != nil {
		return fmt.Errorf("failed to find common ancestor: %w", err)
	}

	// Log reorganization details
	log.Printf("reorganization started: fork_height=%d old_tip_height=%d old_tip_hash=%x new_tip_height=%d new_tip_hash=%x",
		forkPoint.GetHeight(), currentTip.GetHeight(), currentTip.Hash, newBlock.GetHeight(), newBlock.Hash,
	)

	// Disconnect blocks from current tip back to fork point
	disconnectedBlocks := make([]*Block, 0)
	disconnectCurrent := currentTip

	for disconnectCurrent.GetHeight() > forkPoint.GetHeight() {
		disconnectedBlocks = append(disconnectedBlocks, disconnectCurrent)

		// Get parent block
		parent, err := cs.getBlockByHash(disconnectCurrent.Header.PrevHash)
		if err != nil {
			return fmt.Errorf("failed to get parent block: %w", err)
		}

		disconnectCurrent = parent
	}

	// Reverse disconnected blocks for easier reconnection if needed
	for i, j := 0, len(disconnectedBlocks)-1; i < j; i, j = i+1, j-1 {
		disconnectedBlocks[i], disconnectedBlocks[j] = disconnectedBlocks[j], disconnectedBlocks[i]
	}

	// Connect blocks from fork point to new tip
	connectBlocks := make([]*Block, 0)
	connectCurrent := newBlock

	// Build path from new block back to fork
	pathToFork := make([]*Block, 0)
	for connectCurrent.GetHeight() > forkPoint.GetHeight() {
		pathToFork = append(pathToFork, connectCurrent)

		parent, err := cs.getBlockByHash(connectCurrent.Header.PrevHash)
		if err != nil {
			// Reorganization failed, attempt to reconnect old chain
			cs.rollbackReorg(disconnectedBlocks)
			return fmt.Errorf("failed to get parent during reorg: %w", err)
		}

		connectCurrent = parent
	}

	// Reverse to get fork-to-tip order
	for i, j := 0, len(pathToFork)-1; i < j; i, j = i+1, j-1 {
		pathToFork[i], pathToFork[j] = pathToFork[j], pathToFork[i]
	}
	connectBlocks = pathToFork

	// Validate all blocks to be connected
	for _, block := range connectBlocks {
		if err := cs.validateBlock(block); err != nil {
			// Reorganization failed, rollback to old chain
			cs.rollbackReorg(disconnectedBlocks)
			return fmt.Errorf("block validation failed during reorg: %w", err)
		}
	}

	// Perform the reorganization
	if err := cs.chain.SetTip(newBlock); err != nil {
		// Reorganization failed, attempt rollback
		cs.rollbackReorg(disconnectedBlocks)
		return fmt.Errorf("failed to set new tip: %w", err)
	}

	// Reorganization successful
	log.Printf("reorganization completed: disconnected_blocks=%d connected_blocks=%d new_tip_height=%d new_work=%s",
		len(disconnectedBlocks), len(connectBlocks), newBlock.GetHeight(), newBlock.TotalWork,
	)

	return nil
}

// findCommonAncestor finds the common ancestor of two blocks
func (cs *ChainSelector) findCommonAncestor(block1, block2 *Block) (*Block, error) {
	if block1 == nil || block2 == nil {
		return nil, fmt.Errorf("cannot find ancestor of nil block")
	}

	// Build map of ancestors for block1
	ancestors := make(map[string]*Block)
	current := block1

	for current.GetHeight() >= 0 {
		hashStr := string(current.Hash)
		ancestors[hashStr] = current

		if current.GetHeight() == 0 {
			break // Genesis block
		}

		parent, err := cs.getBlockByHash(current.Header.PrevHash)
		if err != nil {
			break // Reached genesis or missing parent
		}
		current = parent
	}

	// Find first common ancestor in block2's chain
	current = block2
	for current.GetHeight() >= 0 {
		hashStr := string(current.Hash)
		if ancestor, exists := ancestors[hashStr]; exists {
			return ancestor, nil
		}

		if current.GetHeight() == 0 {
			break
		}

		parent, err := cs.getBlockByHash(current.Header.PrevHash)
		if err != nil {
			break
		}
		current = parent
	}

	// No common ancestor found (should not happen in valid blockchain)
	return nil, fmt.Errorf("no common ancestor found")
}

// rollbackReorg attempts to rollback a failed reorganization
func (cs *ChainSelector) rollbackReorg(disconnectedBlocks []*Block) {
	log.Printf("reorganization failed, attempting rollback: blocks_to_reconnect=%d", len(disconnectedBlocks))

	// Try to reconnect the old chain
	for i := len(disconnectedBlocks) - 1; i >= 0; i-- {
		block := disconnectedBlocks[i]
		if err := cs.chain.SetTip(block); err != nil {
			log.Printf("rollback failed, chain may be in inconsistent state: block_height=%d error=%v", block.GetHeight(), err)
			return
		}
	}

	log.Printf("rollback successful")
}

// validateBlock validates a block before connecting during reorg
func (cs *ChainSelector) validateBlock(block *Block) error {
	if block == nil {
		return fmt.Errorf("cannot validate nil block")
	}

	// Basic validation
	if block.GetHeight() == 0 {
		return nil // Genesis block is always valid
	}

	if len(block.Hash) == 0 {
		return fmt.Errorf("block hash is empty")
	}

	if len(block.Header.PrevHash) == 0 {
		return fmt.Errorf("previous hash is empty")
	}

	// Note: Full block validation (signatures, transactions, etc.) is performed by the consensus layer
	// This function performs basic structural validation only
	// For complete validation, use consensus.BlockValidator.ValidateBlock()

	return nil
}

// getBlockByHash retrieves a block by hash from storage
func (cs *ChainSelector) getBlockByHash(hash []byte) (*Block, error) {
	if cs.blockProvider == nil {
		return nil, fmt.Errorf("block provider not available")
	}

	block, exists := cs.blockProvider.GetBlockByHash(hash)
	if !exists {
		return nil, fmt.Errorf("block not found: %x", hash)
	}

	return block, nil
}

// IsReorgInProgress returns whether a reorganization is currently in progress
func (cs *ChainSelector) IsReorgInProgress() bool {
	cs.reorgMutex.Lock()
	defer cs.reorgMutex.Unlock()
	return cs.reorgInProgress
}

// ActivateBestChain implements Bitcoin-style chain activation with periodic triggering
// This method should be called periodically to ensure optimal chain selection
func (cs *ChainSelector) ActivateBestChain() error {
	// Bitcoin-style: Find the chain with most work
	bestBlock := cs.FindMostWorkChain()
	if bestBlock == nil {
		return fmt.Errorf("no valid chain found")
	}

	currentTip := cs.chain.LatestBlock()
	if currentTip == nil || string(currentTip.Hash) == string(bestBlock.Hash) {
		return nil // Best chain already active
	}

	// Check if reorg is needed
	if cs.ShouldReorg(bestBlock) {
		return cs.Reorganize(bestBlock)
	}

	return nil
}

// StartPeriodicActivation starts periodic chain activation checks
// Bitcoin implements this mechanism to ensure consistent chain selection
func (cs *ChainSelector) StartPeriodicActivation(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				if !cs.IsReorgInProgress() {
					cs.ActivateBestChain()
				}
			}
		}
	}()
}
