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

package core

import (
	"errors"
	"sync"
	"time"
)

// Integrity reward distribution constants
const (
	// DefaultDistributionInterval is the default interval for reward distribution (5082 blocks)
	// 5082 blocks ≈ 1 day (with 17 second block time: 5082 * 17 / 3600 / 24 ≈ 1.0 day)
	DefaultDistributionInterval = 5082

	// IntegrityPoolSharePercent is the percentage of block reward allocated to integrity pool (1%)
	IntegrityPoolSharePercent = 1
)

// IntegrityRewardDistributor manages integrity node reward distribution
// Production-grade: thread-safe with mutex protection
type IntegrityRewardDistributor struct {
	mu                     sync.RWMutex
	rewardPool             uint64               // Total rewards accumulated
	distributionInterval   uint64               // Blocks between distributions (default 5082)
	lastDistributionHeight uint64               // Last distribution block height
	nextDistributionHeight uint64               // Next distribution block height
	totalDistributed       uint64               // Total rewards distributed historically
	distributionHistory    []DistributionRecord // Historical distribution records
	calculator             *ScoreCalculator     // Score calculator instance
	thresholds             ScoreThresholds      // Score thresholds
}

// DistributionRecord represents a historical distribution record
type DistributionRecord struct {
	// Height is the block height at distribution
	Height uint64 `json:"height"`

	// Timestamp is the distribution timestamp
	Timestamp int64 `json:"timestamp"`

	// TotalReward is the total reward distributed
	TotalReward uint64 `json:"totalReward"`

	// QualifiedNodes is the number of qualified nodes
	QualifiedNodes int `json:"qualifiedNodes"`

	// Rewards is the map of node ID to reward amount
	Rewards map[string]uint64 `json:"rewards"`
}

// NewIntegrityRewardDistributor creates a new reward distributor
func NewIntegrityRewardDistributor() *IntegrityRewardDistributor {
	return &IntegrityRewardDistributor{
		rewardPool:             0,
		distributionInterval:   DefaultDistributionInterval,
		lastDistributionHeight: 0,
		nextDistributionHeight: DefaultDistributionInterval,
		totalDistributed:       0,
		distributionHistory:    make([]DistributionRecord, 0),
		calculator:             NewScoreCalculator(),
		thresholds:             DefaultScoreThresholds(),
	}
}

// AddToPool adds block reward to the integrity pool
// Called for each block - adds 1% of block reward
// Thread-safe with mutex protection
func (d *IntegrityRewardDistributor) AddToPool(blockReward uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Calculate 1% of block reward using integer arithmetic
	integrityShare := blockReward * IntegrityPoolSharePercent / 100

	// Add to pool with overflow protection
	if d.rewardPool > ^uint64(0)-integrityShare {
		// Overflow would occur, cap at max uint64
		d.rewardPool = ^uint64(0)
	} else {
		d.rewardPool += integrityShare
	}
}

// AddToPoolWithAmount adds a specific amount to the pool
// Useful for manual adjustments or testing
func (d *IntegrityRewardDistributor) AddToPoolWithAmount(amount uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.rewardPool > ^uint64(0)-amount {
		d.rewardPool = ^uint64(0)
	} else {
		d.rewardPool += amount
	}
}

// DistributeRewards distributes rewards to qualified nodes
// Called every distributionInterval blocks
// Returns map of node ID to reward amount
// Thread-safe with mutex protection
func (d *IntegrityRewardDistributor) DistributeRewards(nodes []*NodeIntegrity, currentHeight uint64) (map[string]uint64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Validate parameters
	if nodes == nil || len(nodes) == 0 {
		return nil, errors.New("no nodes provided for reward distribution")
	}

	if d.rewardPool == 0 {
		return nil, errors.New("reward pool is empty")
	}

	// Check if it's distribution time
	if currentHeight < d.nextDistributionHeight {
		return nil, errors.New("not yet distribution time")
	}

	// Filter qualified nodes
	qualifiedNodes := make([]*NodeIntegrity, 0)
	for _, node := range nodes {
		if d.calculator.QualifiesForReward(node, d.thresholds) {
			qualifiedNodes = append(qualifiedNodes, node)
		}
	}

	if len(qualifiedNodes) == 0 {
		// No qualified nodes, keep rewards in pool for next distribution
		d.lastDistributionHeight = currentHeight
		d.nextDistributionHeight = currentHeight + d.distributionInterval
		return map[string]uint64{}, nil
	}

	// Calculate total weighted score for proportional distribution
	totalWeight := uint64(0)
	nodeWeights := make(map[string]uint64)

	for _, node := range qualifiedNodes {
		// Calculate reward share (0-1000, representing 0.0-1.0)
		share := d.calculator.CalculateRewardShare(node, d.thresholds)
		if share > 0 {
			nodeWeights[node.NodeID] = uint64(share)
			totalWeight += uint64(share)
		}
	}

	if totalWeight == 0 {
		// All nodes have zero weight, keep rewards in pool
		d.lastDistributionHeight = currentHeight
		d.nextDistributionHeight = currentHeight + d.distributionInterval
		return map[string]uint64{}, nil
	}

	// Distribute rewards proportionally based on weights
	rewards := make(map[string]uint64)
	totalDistributed := uint64(0)

	// First pass: calculate base rewards
	for _, node := range qualifiedNodes {
		weight, exists := nodeWeights[node.NodeID]
		if !exists || weight == 0 {
			continue
		}

		// Calculate reward: (nodeWeight / totalWeight) * rewardPool
		// Using integer arithmetic: (weight * rewardPool) / totalWeight
		reward := (weight * d.rewardPool) / totalWeight
		rewards[node.NodeID] = reward
		totalDistributed += reward
	}

	// Second pass: distribute remainder (due to integer division rounding)
	remainder := d.rewardPool - totalDistributed
	if remainder > 0 && len(qualifiedNodes) > 0 {
		// Give remainder to highest-scoring node
		// This ensures all rewards are distributed
		var topNode string
		var topWeight uint64 = 0

		for nodeID, weight := range nodeWeights {
			if weight > topWeight {
				topWeight = weight
				topNode = nodeID
			}
		}

		if topNode != "" {
			rewards[topNode] += remainder
			totalDistributed += remainder
		}
	}

	// Update state
	d.totalDistributed += totalDistributed
	d.rewardPool = 0 // Reset pool after distribution
	d.lastDistributionHeight = currentHeight
	d.nextDistributionHeight = currentHeight + d.distributionInterval

	// Record distribution history
	record := DistributionRecord{
		Height:         currentHeight,
		Timestamp:      time.Now().Unix(),
		TotalReward:    totalDistributed,
		QualifiedNodes: len(qualifiedNodes),
		Rewards:        make(map[string]uint64),
	}
	// Copy rewards map
	for k, v := range rewards {
		record.Rewards[k] = v
	}

	d.distributionHistory = append(d.distributionHistory, record)

	// Keep only last 100 distribution records
	if len(d.distributionHistory) > 100 {
		d.distributionHistory = d.distributionHistory[len(d.distributionHistory)-100:]
	}

	return rewards, nil
}

// GetRewardPool returns the current reward pool balance (thread-safe)
func (d *IntegrityRewardDistributor) GetRewardPool() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.rewardPool
}

// GetNextDistributionHeight returns the next distribution block height (thread-safe)
func (d *IntegrityRewardDistributor) GetNextDistributionHeight() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.nextDistributionHeight
}

// GetLastDistributionHeight returns the last distribution block height (thread-safe)
func (d *IntegrityRewardDistributor) GetLastDistributionHeight() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastDistributionHeight
}

// GetTotalDistributed returns total historical rewards distributed (thread-safe)
func (d *IntegrityRewardDistributor) GetTotalDistributed() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.totalDistributed
}

// GetDistributionHistory returns distribution history (thread-safe)
func (d *IntegrityRewardDistributor) GetDistributionHistory() []DistributionRecord {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Return a copy
	copy := make([]DistributionRecord, len(d.distributionHistory))
	for i, r := range d.distributionHistory {
		copy[i] = r
	}
	return copy
}

// SetDistributionInterval sets the distribution interval (thread-safe)
func (d *IntegrityRewardDistributor) SetDistributionInterval(interval uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if interval > 0 {
		d.distributionInterval = interval
		// Recalculate next distribution height
		if d.lastDistributionHeight > 0 {
			d.nextDistributionHeight = d.lastDistributionHeight + interval
		} else {
			d.nextDistributionHeight = interval
		}
	}
}

// GetDistributionInterval returns the current distribution interval (thread-safe)
func (d *IntegrityRewardDistributor) GetDistributionInterval() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.distributionInterval
}

// ShouldDistribute checks if rewards should be distributed at current height
func (d *IntegrityRewardDistributor) ShouldDistribute(currentHeight uint64) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return currentHeight >= d.nextDistributionHeight
}

// GetDistributionProgress returns the progress towards next distribution (0.0 to 1.0)
// Represented as uint32/1000 (e.g., 750 = 0.75 = 75%)
func (d *IntegrityRewardDistributor) GetDistributionProgress(currentHeight uint64) uint32 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.nextDistributionHeight <= d.lastDistributionHeight {
		return 0
	}

	progress := currentHeight - d.lastDistributionHeight
	interval := d.nextDistributionHeight - d.lastDistributionHeight

	if interval == 0 {
		return 0
	}

	// Calculate progress as percentage (0-1000)
	p := (progress * 1000) / interval
	if p > 1000 {
		return 1000
	}
	return uint32(p)
}

// Reset resets the distributor state (for testing only)
func (d *IntegrityRewardDistributor) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.rewardPool = 0
	d.lastDistributionHeight = 0
	d.nextDistributionHeight = d.distributionInterval
	d.totalDistributed = 0
	d.distributionHistory = make([]DistributionRecord, 0)
}

// CalculateExpectedReward calculates expected reward for a node at next distribution
// Useful for node operators to estimate earnings
func (d *IntegrityRewardDistributor) CalculateExpectedReward(node *NodeIntegrity, allNodes []*NodeIntegrity) uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if node == nil || d.rewardPool == 0 {
		return 0
	}

	// Check if node qualifies
	if !d.calculator.QualifiesForReward(node, d.thresholds) {
		return 0
	}

	// Calculate node's weight
	nodeWeight := d.calculator.CalculateRewardShare(node, d.thresholds)
	if nodeWeight == 0 {
		return 0
	}

	// Calculate total weight of all qualified nodes
	totalWeight := uint64(0)
	for _, n := range allNodes {
		if d.calculator.QualifiesForReward(n, d.thresholds) {
			weight := d.calculator.CalculateRewardShare(n, d.thresholds)
			totalWeight += uint64(weight)
		}
	}

	if totalWeight == 0 {
		return 0
	}

	// Calculate expected reward
	expectedReward := (uint64(nodeWeight) * d.rewardPool) / totalWeight
	return expectedReward
}

// GetNodeRewardInfo returns detailed reward information for a node
type NodeRewardInfo struct {
	NodeID         string `json:"nodeId"`
	Score          uint8  `json:"score"`
	Qualifies      bool   `json:"qualifies"`
	RewardShare    uint32 `json:"rewardShare"` // 0-1000 (0.0-1.0)
	ExpectedReward uint64 `json:"expectedReward"`
	LastReward     uint64 `json:"lastReward"`
	TotalRewards   uint64 `json:"totalRewards"`
}

// GetNodeRewardInfo gets reward information for a specific node
func (d *IntegrityRewardDistributor) GetNodeRewardInfo(node *NodeIntegrity, allNodes []*NodeIntegrity) NodeRewardInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	info := NodeRewardInfo{
		NodeID:    node.NodeID,
		Score:     node.GetScore(),
		Qualifies: d.calculator.QualifiesForReward(node, d.thresholds),
	}

	if info.Qualifies {
		info.RewardShare = d.calculator.CalculateRewardShare(node, d.thresholds)
		info.ExpectedReward = d.CalculateExpectedReward(node, allNodes)
	}

	// Calculate last and total rewards from history
	if len(d.distributionHistory) > 0 {
		lastRecord := d.distributionHistory[len(d.distributionHistory)-1]
		if reward, exists := lastRecord.Rewards[node.NodeID]; exists {
			info.LastReward = reward
		}
	}

	// Sum all historical rewards
	for _, record := range d.distributionHistory {
		if reward, exists := record.Rewards[node.NodeID]; exists {
			info.TotalRewards += reward
		}
	}

	return info
}
