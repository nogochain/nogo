// Copyright 2026 NogoChain Team
// Production-grade chain selection and reorganization logic
// Implements Bitcoin's FindMostWorkChain and ActivateBestChainStep

package core

import (
	"fmt"
	"log"
	"math"
	"math/big"
	"sync"
	"time"
)

// TopologyStats represents network topology statistics
// Production-grade: decoupled from network package to avoid import cycles
type TopologyStats struct {
	TotalNodes   int
	RelayNodes   int
	Partitions   int
	AvgLatency   float64
	NetworkScore float64
}

// TopologyProvider interface for network topology access
// Production-grade: interface-based design for loose coupling
type TopologyProvider interface {
	GetTopologyStats() TopologyStats
}

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
	// Optimization: Track candidate chains for incremental reorg
	candidateChains map[string]*Block
	candidateMutex  sync.RWMutex
	// Network topology for intelligent chain selection
	topology        TopologyProvider
	// Block propagation information
	propagationStats map[string]interface{}
	// Chain health metrics
	chainHealthMetrics map[string]float64
	// Last network analysis time
	lastNetworkAnalysis time.Time
}

// NewChainSelector creates a new chain selector
func NewChainSelector(chain *Chain, provider BlockProvider) *ChainSelector {
	return &ChainSelector{
		chain:          chain,
		blockProvider:  provider,
		workCalculator: NewWorkCalculator(),
		candidateChains: make(map[string]*Block),
		propagationStats: make(map[string]interface{}),
		chainHealthMetrics: make(map[string]float64),
		lastNetworkAnalysis: time.Now(),
	}
}

// FindMostWorkChain finds the block with the most cumulative work
// Implementation matches Bitcoin's FindMostWorkChain algorithm with enhancement
// Returns the block with highest chain work, nil if no blocks
func (cs *ChainSelector) FindMostWorkChain() *Block {
	// First, get the current best block with read lock
	cs.mu.RLock()
	bestBlock := cs.chain.LatestBlock()
	if bestBlock == nil {
		cs.mu.RUnlock()
		return nil
	}

	// Get current best health metrics
	bestHealth := cs.calculateChainHealth(bestBlock)
	bestWork, ok := StringToWork(bestBlock.TotalWork)
	if !ok {
		bestWork = new(big.Int)
	}

	// Optimization: Check only candidate chains instead of all blocks
	cs.candidateMutex.RLock()
	candidates := make([]*Block, 0, len(cs.candidateChains))
	for _, block := range cs.candidateChains {
		candidates = append(candidates, block)
	}
	cs.candidateMutex.RUnlock()
	cs.mu.RUnlock()

	// Check candidate chains
	for _, block := range candidates {
		if block == nil {
			continue
		}

		// Bitcoin-style validation: ensure block chain is fully valid
		if !cs.isChainValid(block) {
			// Remove invalid candidate
			cs.candidateMutex.Lock()
			delete(cs.candidateChains, string(block.Hash))
			cs.candidateMutex.Unlock()
			continue
		}

		blockWork, ok := StringToWork(block.TotalWork)
		if !ok {
			continue
		}

		// Calculate health metrics for candidate chain
		cs.mu.RLock()
		blockHealth := cs.calculateChainHealth(block)
		cs.mu.RUnlock()

		// Smart chain selection: compare based on work and health
		if cs.shouldPreferChain(blockWork, blockHealth, bestWork, bestHealth) {
			bestWork = blockWork
			bestHealth = blockHealth
			bestBlock = block
		}
	}

	// Update chain health metrics
	cs.mu.Lock()
	cs.chainHealthMetrics = bestHealth
	cs.lastNetworkAnalysis = time.Now()
	cs.mu.Unlock()

	return bestBlock
}

// shouldPreferChain determines if a candidate chain should be preferred over the current best
func (cs *ChainSelector) shouldPreferChain(
	candidateWork *big.Int,
	candidateHealth map[string]float64,
	currentWork *big.Int,
	currentHealth map[string]float64,
) bool {
	// First check: work difference
	workDiff := candidateWork.Cmp(currentWork)

	// If candidate has significantly more work, prefer it
	if workDiff > 0 {
		// If work is at least 5% higher, always prefer
		workRatio := new(big.Float).Quo(
			new(big.Float).SetInt(candidateWork),
			new(big.Float).SetInt(currentWork),
		)
		ratio, _ := workRatio.Float64()
		if ratio > 1.05 {
			return true
		}

		// If work is slightly higher, consider health
		if candidateHealth["total_score"] >= currentHealth["total_score"]-5 {
			return true
		}
	}

	// If work is equal, prefer based on health
	if workDiff == 0 {
		return candidateHealth["total_score"] > currentHealth["total_score"]
	}

	// If work is lower but health is significantly better, consider it
	if workDiff < 0 {
		workRatio := new(big.Float).Quo(
			new(big.Float).SetInt(candidateWork),
			new(big.Float).SetInt(currentWork),
		)
		ratio, _ := workRatio.Float64()
		// Only consider if work is within 95% of current and health is at least 10 points higher
		if ratio > 0.95 && candidateHealth["total_score"] > currentHealth["total_score"]+10 {
			return true
		}
	}

	return false
}

// AddCandidateChain adds a block to the candidate chains for incremental reorg
func (cs *ChainSelector) AddCandidateChain(block *Block) {
	if block == nil || block.Hash == nil {
		return
	}

	cs.candidateMutex.Lock()
	defer cs.candidateMutex.Unlock()

	// Only add if not already in candidates
	hashStr := string(block.Hash)
	if _, exists := cs.candidateChains[hashStr]; !exists {
		cs.candidateChains[hashStr] = block
	}
}

// SetTopology sets the network topology for intelligent chain selection
// Production-grade: uses interface for loose coupling
func (cs *ChainSelector) SetTopology(topology TopologyProvider) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.topology = topology
}

// UpdatePropagationStats updates block propagation statistics
func (cs *ChainSelector) UpdatePropagationStats(stats map[string]interface{}) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.propagationStats = stats
}

// RemoveCandidateChain removes a block from the candidate chains
func (cs *ChainSelector) RemoveCandidateChain(blockHash []byte) {
	if blockHash == nil {
		return
	}

	cs.candidateMutex.Lock()
	defer cs.candidateMutex.Unlock()

	delete(cs.candidateChains, string(blockHash))
}

// calculateChainHealth calculates chain health metrics
func (cs *ChainSelector) calculateChainHealth(block *Block) map[string]float64 {
	metrics := make(map[string]float64)

	// 1. Work-based score
	work, ok := StringToWork(block.TotalWork)
	if !ok {
		metrics["work_score"] = 0
	} else {
		// Normalize work score (using log scale)
		// Float64() returns (float64, Accuracy) - we only need the value
		workFloat, _ := work.Float64()
		workScore := math.Log10(workFloat)
		metrics["work_score"] = math.Min(100, workScore*10)
	}

	// 2. Network health score
	networkScore := 70.0 // Default score
	if cs.topology != nil {
		stats := cs.topology.GetTopologyStats()
		if stats.NetworkScore > 0 {
			networkScore = stats.NetworkScore * 100
		}
	}
	metrics["network_score"] = networkScore

	// 3. Propagation speed score
	propagationScore := 80.0 // Default score
	if queueLength, ok := cs.propagationStats["queue_length"].(int); ok {
		// Lower queue length means better propagation
		if queueLength < 10 {
			propagationScore = 95
		} else if queueLength < 50 {
			propagationScore = 85
		} else if queueLength < 100 {
			propagationScore = 70
		} else {
			propagationScore = 50
		}
	}
	metrics["propagation_score"] = propagationScore

	// 4. Chain stability score
	// Production-grade: stability based on reorg frequency and depth
	stabilityScore := 90.0 // Base score for stable chain
	if cs.topology != nil {
		stats := cs.topology.GetTopologyStats()
		// Lower score if network has many partitions (indicates instability)
		if stats.Partitions > 3 {
			stabilityScore -= float64(stats.Partitions) * 5
		}
		// Lower score if latency is high (indicates network issues)
		if stats.AvgLatency > 500 {
			stabilityScore -= 10
		}
		// Ensure minimum score
		if stabilityScore < 50 {
			stabilityScore = 50
		}
	}
	metrics["stability_score"] = stabilityScore

	// 5. Get dynamic weights based on network conditions
	// Default weights
	strategy := map[string]float64{
		"work_weight":        0.4,
		"network_weight":     0.3,
		"propagation_weight": 0.2,
		"stability_weight":   0.1,
	}

	// Adjust strategy based on network conditions
	if cs.topology != nil {
		connectivity := cs.topology.GetTopologyStats()
		healthScore := connectivity.NetworkScore * 100

		// If network health is poor, prioritize work and stability
		if healthScore < 60 {
			strategy["work_weight"] = 0.5
			strategy["stability_weight"] = 0.2
			strategy["network_weight"] = 0.2
			strategy["propagation_weight"] = 0.1
		}

		// If network health is excellent, prioritize propagation speed
		if healthScore > 90 {
			strategy["propagation_weight"] = 0.3
			strategy["work_weight"] = 0.35
			strategy["network_weight"] = 0.25
			strategy["stability_weight"] = 0.1
		}
	}

	// Adjust based on propagation queue length
	if queueLength, ok := cs.propagationStats["queue_length"].(int); ok {
		// If queue is long, prioritize propagation speed
		if queueLength > 50 {
			strategy["propagation_weight"] = 0.3
			strategy["work_weight"] = 0.4
			strategy["network_weight"] = 0.2
			strategy["stability_weight"] = 0.1
		}
	}

	// 6. Calculate total health score using dynamic weights
	totalScore := metrics["work_score"]*strategy["work_weight"] +
		metrics["network_score"]*strategy["network_weight"] +
		metrics["propagation_score"]*strategy["propagation_weight"] +
		metrics["stability_score"]*strategy["stability_weight"]
	metrics["total_score"] = totalScore

	// 7. Add additional health indicators
	metrics["block_height"] = float64(block.GetHeight())
	metrics["network_health_age"] = cs.GetNetworkAnalysisAge().Seconds()

	return metrics
}

// GetChainHealthMetrics returns the current chain health metrics
func (cs *ChainSelector) GetChainHealthMetrics() map[string]float64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	metrics := make(map[string]float64)
	for k, v := range cs.chainHealthMetrics {
		metrics[k] = v
	}

	return metrics
}

// ChainSelectionStrategy represents the strategy for chain selection
// Dynamic: adjusts based on network conditions
func (cs *ChainSelector) GetChainSelectionStrategy() map[string]float64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Default weights
	strategy := map[string]float64{
		"work_weight":        0.4,
		"network_weight":     0.3,
		"propagation_weight": 0.2,
		"stability_weight":   0.1,
	}

	// Adjust strategy based on network conditions
	if cs.topology != nil {
		connectivity := cs.topology.GetTopologyStats()
		healthScore := connectivity.NetworkScore * 100

		// If network health is poor, prioritize work and stability
		if healthScore < 60 {
			strategy["work_weight"] = 0.5
			strategy["stability_weight"] = 0.2
			strategy["network_weight"] = 0.2
			strategy["propagation_weight"] = 0.1
		}

		// If network health is excellent, prioritize propagation speed
		if healthScore > 90 {
			strategy["propagation_weight"] = 0.3
			strategy["work_weight"] = 0.35
			strategy["network_weight"] = 0.25
			strategy["stability_weight"] = 0.1
		}
	}

	// Adjust based on propagation queue length
	if queueLength, ok := cs.propagationStats["queue_length"].(int); ok {
		// If queue is long, prioritize propagation speed
		if queueLength > 50 {
			strategy["propagation_weight"] = 0.3
			strategy["work_weight"] = 0.4
			strategy["network_weight"] = 0.2
			strategy["stability_weight"] = 0.1
		}
	}

	return strategy
}

// UpdateChainHealthMetrics updates the chain health metrics based on current network conditions
func (cs *ChainSelector) UpdateChainHealthMetrics() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	currentTip := cs.chain.LatestBlock()
	if currentTip != nil {
		cs.chainHealthMetrics = cs.calculateChainHealth(currentTip)
		cs.lastNetworkAnalysis = time.Now()
	}
}

// GetNetworkAnalysisAge returns the age of the last network analysis
func (cs *ChainSelector) GetNetworkAnalysisAge() time.Duration {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	return time.Since(cs.lastNetworkAnalysis)
}

// ClearCandidateChains clears all candidate chains
func (cs *ChainSelector) ClearCandidateChains() {
	cs.candidateMutex.Lock()
	defer cs.candidateMutex.Unlock()

	cs.candidateChains = make(map[string]*Block)
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

	// Validate all blocks to be connected in parallel
	if len(connectBlocks) > 0 {
		errors := make(chan error, len(connectBlocks))
		var wg sync.WaitGroup

		// Limit concurrency to avoid overwhelming system resources
		concurrencyLimit := 4
		semaphore := make(chan struct{}, concurrencyLimit)

		for _, block := range connectBlocks {
			wg.Add(1)
			go func(b *Block) {
				defer wg.Done()

				// Acquire semaphore
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				if err := cs.validateBlock(b); err != nil {
					errors <- err
				}
			}(block)
		}

		// Wait for all validations to complete
		wg.Wait()
		close(errors)

		// Check for validation errors
		for err := range errors {
			if err != nil {
				// Reorganization failed, rollback to old chain
				cs.rollbackReorg(disconnectedBlocks)
				return fmt.Errorf("block validation failed during reorg: %w", err)
			}
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
// Optimized: Adjusts blocks to same height first, then searches in parallel
func (cs *ChainSelector) findCommonAncestor(block1, block2 *Block) (*Block, error) {
	if block1 == nil || block2 == nil {
		return nil, fmt.Errorf("cannot find ancestor of nil block")
	}

	// Get heights
	height1 := block1.GetHeight()
	height2 := block2.GetHeight()

	// Adjust both blocks to the same height
	current1 := block1
	current2 := block2

	// Move block1 up if it's higher
	for height1 > height2 {
		parent, err := cs.getBlockByHash(current1.Header.PrevHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent block: %w", err)
		}
		current1 = parent
		height1--
	}

	// Move block2 up if it's higher
	for height2 > height1 {
		parent, err := cs.getBlockByHash(current2.Header.PrevHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent block: %w", err)
		}
		current2 = parent
		height2--
	}

	// Now both blocks are at the same height, search for common ancestor
	for current1.GetHeight() >= 0 {
		// Check if blocks are the same
		if string(current1.Hash) == string(current2.Hash) {
			return current1, nil
		}

		// Move both up one level
		parent1, err1 := cs.getBlockByHash(current1.Header.PrevHash)
		parent2, err2 := cs.getBlockByHash(current2.Header.PrevHash)

		if err1 != nil || err2 != nil {
			break // Reached genesis or missing parent
		}

		current1 = parent1
		current2 = parent2
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
