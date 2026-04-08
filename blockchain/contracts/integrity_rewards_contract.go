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

package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Integrity reward contract constants
const (
	// DefaultDistributionInterval is the default interval for reward distribution (5082 blocks)
	// 5082 blocks ≈ 1 day (with 17 second block time)
	DefaultDistributionInterval = 5082

	// IntegrityPoolSharePercent is the percentage of block reward allocated to integrity pool (1%)
	IntegrityPoolSharePercent = 1

	// MinQualifiedScore is the minimum integrity score to receive rewards
	MinQualifiedScore = 60
)

// IntegrityNode represents an integrity node in the contract
type IntegrityNode struct {
	// NodeID is the unique node identifier
	NodeID string `json:"nodeId"`
	// Address is the node's reward receiving address
	Address string `json:"address"`
	// Score is the current integrity score (0-100)
	Score uint8 `json:"score"`
	// Status is the node status (active/inactive/suspended/banned)
	Status string `json:"status"`
	// TotalRewards is the total rewards received
	TotalRewards uint64 `json:"totalRewards"`
	// LastRewardHeight is the last reward distribution height
	LastRewardHeight uint64 `json:"lastRewardHeight"`
	// RegisteredAt is the registration timestamp
	RegisteredAt int64 `json:"registeredAt"`
}

// DistributionEvent represents a reward distribution event
type DistributionEvent struct {
	// Height is the block height of distribution
	Height uint64 `json:"height"`
	// Timestamp is the distribution timestamp
	Timestamp int64 `json:"timestamp"`
	// TotalDistributed is the total amount distributed
	TotalDistributed uint64 `json:"totalDistributed"`
	// QualifiedNodes is the number of qualified nodes
	QualifiedNodes int `json:"qualifiedNodes"`
	// Rewards is the map of node address to reward amount
	Rewards map[string]uint64 `json:"rewards"`
}

// IntegrityRewardContract represents the integrity reward smart contract
// Production-grade: pure on-chain governance, automatic distribution, no human intervention
type IntegrityRewardContract struct {
	mu sync.RWMutex

	// ContractAddress is the contract address (auto-generated on deployment)
	ContractAddress string `json:"contractAddress"`
	// DeployedAt is the deployment timestamp
	DeployedAt int64 `json:"deployedAt"`
	// RewardPool is the current reward pool balance in wei
	RewardPool uint64 `json:"rewardPool"`
	// DistributionInterval is the blocks between distributions
	DistributionInterval uint64 `json:"distributionInterval"`
	// LastDistributionHeight is the last distribution block height
	LastDistributionHeight uint64 `json:"lastDistributionHeight"`
	// NextDistributionHeight is the next distribution block height
	NextDistributionHeight uint64 `json:"nextDistributionHeight"`
	// TotalDistributed is the total historical rewards distributed
	TotalDistributed uint64 `json:"totalDistributed"`
	// Nodes is the map of node ID to integrity node
	Nodes map[string]*IntegrityNode `json:"nodes"`
	// NodeCount is the total number of registered nodes
	NodeCount uint64 `json:"nodeCount"`
	// DistributionHistory is the historical distribution events
	DistributionHistory []DistributionEvent `json:"distributionHistory"`
}

// NewIntegrityRewardContract creates a new integrity reward contract
// Contract address is auto-generated during deployment, cannot be manually created
// Address format: NOGO + version(1) + hash(32) + checksum(4) = 78 characters total
func NewIntegrityRewardContract() *IntegrityRewardContract {
	// Generate unique contract address using timestamp and random data
	// Format matches wallet address: NOGO + version byte + 32-byte hash + 4-byte checksum
	timestamp := time.Now().UnixNano()
	data := []byte(fmt.Sprintf("%d-INTEGRITY_REWARD_CONTRACT", timestamp))
	
	// Generate 32-byte hash
	hash := sha256.Sum256(data)
	
	// Build address: version byte (0x00) + 32-byte hash
	addressData := make([]byte, 1+32)
	addressData[0] = 0x00 // Version byte
	copy(addressData[1:], hash[:32])
	
	// Calculate 4-byte checksum
	checksumHash := sha256.Sum256(addressData)
	addressData = append(addressData, checksumHash[:4]...)
	
	// Encode to hex and add prefix
	contractAddress := "NOGO" + hex.EncodeToString(addressData)

	return &IntegrityRewardContract{
		ContractAddress:        contractAddress,
		DeployedAt:             time.Now().Unix(),
		RewardPool:             0,
		DistributionInterval:   DefaultDistributionInterval,
		LastDistributionHeight: 0,
		NextDistributionHeight: DefaultDistributionInterval,
		TotalDistributed:       0,
		Nodes:                  make(map[string]*IntegrityNode),
		NodeCount:              0,
		DistributionHistory:    make([]DistributionEvent, 0),
	}
}

// GetContractAddress returns the contract address (read-only)
// This address is auto-generated and cannot be manually created
func (c *IntegrityRewardContract) GetContractAddress() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ContractAddress
}

// GetRewardPool returns the current reward pool balance (read-only)
func (c *IntegrityRewardContract) GetRewardPool() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.RewardPool
}

// AddToRewardPool adds funds to the reward pool
// Called for each block - adds 1% of block reward
// Thread-safe with mutex protection
func (c *IntegrityRewardContract) AddToRewardPool(blockReward uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Calculate 1% of block reward using integer arithmetic
	integrityShare := blockReward * IntegrityPoolSharePercent / 100

	// Add to pool with overflow protection
	if c.RewardPool > ^uint64(0)-integrityShare {
		c.RewardPool = ^uint64(0)
	} else {
		c.RewardPool += integrityShare
	}
}

// RegisterNode registers a new integrity node
// Thread-safe with mutex protection
func (c *IntegrityRewardContract) RegisterNode(nodeID string, address string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if node already exists
	if _, exists := c.Nodes[nodeID]; exists {
		return errors.New("node already registered")
	}

	// Validate address
	if address == "" {
		return errors.New("address required")
	}

	// Create new node
	node := &IntegrityNode{
		NodeID:           nodeID,
		Address:          address,
		Score:            100, // Start with perfect score
		Status:           "active",
		TotalRewards:     0,
		LastRewardHeight: 0,
		RegisteredAt:     time.Now().Unix(),
	}

	c.Nodes[nodeID] = node
	c.NodeCount++

	return nil
}

// UpdateNodeScore updates a node's integrity score
// Called by the blockchain consensus layer
// Thread-safe with mutex protection
func (c *IntegrityRewardContract) UpdateNodeScore(nodeID string, score uint8, status string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.Nodes[nodeID]
	if !exists {
		return errors.New("node not found")
	}

	node.Score = score
	if status != "" {
		node.Status = status
	}

	return nil
}

// DistributeRewards distributes rewards to qualified nodes
// Called every distributionInterval blocks
// Returns map of node address to reward amount
// Thread-safe with mutex protection
func (c *IntegrityRewardContract) DistributeRewards(currentHeight uint64) (map[string]uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if it's distribution time
	if currentHeight < c.NextDistributionHeight {
		return nil, errors.New("not yet distribution time")
	}

	if c.RewardPool == 0 {
		// No rewards to distribute, update next distribution height
		c.LastDistributionHeight = currentHeight
		c.NextDistributionHeight = currentHeight + c.DistributionInterval
		return map[string]uint64{}, nil
	}

	// Filter qualified nodes
	qualifiedNodes := make([]*IntegrityNode, 0)
	for _, node := range c.Nodes {
		if node.Status == "active" && node.Score >= MinQualifiedScore {
			qualifiedNodes = append(qualifiedNodes, node)
		}
	}

	if len(qualifiedNodes) == 0 {
		// No qualified nodes, keep rewards in pool for next distribution
		c.LastDistributionHeight = currentHeight
		c.NextDistributionHeight = currentHeight + c.DistributionInterval
		return map[string]uint64{}, nil
	}

	// Calculate total weighted score for proportional distribution
	totalWeight := uint64(0)
	nodeWeights := make(map[string]uint64)

	for _, node := range qualifiedNodes {
		// Weight = node score (higher score = more rewards)
		weight := uint64(node.Score)
		nodeWeights[node.NodeID] = weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		// All nodes have zero weight, keep rewards in pool
		c.LastDistributionHeight = currentHeight
		c.NextDistributionHeight = currentHeight + c.DistributionInterval
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
		reward := (weight * c.RewardPool) / totalWeight
		rewards[node.Address] = reward
		totalDistributed += reward

		// Update node's total rewards
		node.TotalRewards += reward
		node.LastRewardHeight = currentHeight
	}

	// Second pass: distribute remainder (due to integer division rounding)
	remainder := c.RewardPool - totalDistributed
	if remainder > 0 && len(qualifiedNodes) > 0 {
		// Give remainder to highest-scoring node
		var topNode *IntegrityNode
		var topWeight uint64 = 0

		for _, node := range qualifiedNodes {
			weight := nodeWeights[node.NodeID]
			if weight > topWeight {
				topWeight = weight
				topNode = node
			}
		}

		if topNode != nil {
			rewards[topNode.Address] += remainder
			totalDistributed += remainder
			topNode.TotalRewards += remainder
		}
	}

	// Update state
	c.TotalDistributed += totalDistributed
	c.RewardPool = 0 // Reset pool after distribution
	c.LastDistributionHeight = currentHeight
	c.NextDistributionHeight = currentHeight + c.DistributionInterval

	// Record distribution history
	event := DistributionEvent{
		Height:           currentHeight,
		Timestamp:        time.Now().Unix(),
		TotalDistributed: totalDistributed,
		QualifiedNodes:   len(qualifiedNodes),
		Rewards:          make(map[string]uint64),
	}
	// Copy rewards map
	for k, v := range rewards {
		event.Rewards[k] = v
	}

	c.DistributionHistory = append(c.DistributionHistory, event)

	// Keep only last 100 distribution events
	if len(c.DistributionHistory) > 100 {
		c.DistributionHistory = c.DistributionHistory[len(c.DistributionHistory)-100:]
	}

	return rewards, nil
}

// ShouldDistribute checks if rewards should be distributed at current height
func (c *IntegrityRewardContract) ShouldDistribute(currentHeight uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return currentHeight >= c.NextDistributionHeight
}

// GetNextDistributionHeight returns the next distribution block height (read-only)
func (c *IntegrityRewardContract) GetNextDistributionHeight() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.NextDistributionHeight
}

// GetLastDistributionHeight returns the last distribution block height (read-only)
func (c *IntegrityRewardContract) GetLastDistributionHeight() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.LastDistributionHeight
}

// GetNode returns a node by ID (read-only copy)
func (c *IntegrityRewardContract) GetNode(nodeID string) (*IntegrityNode, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	node, exists := c.Nodes[nodeID]
	if !exists {
		return nil, errors.New("node not found")
	}

	// Return a copy to prevent external modification
	nodeCopy := *node
	return &nodeCopy, nil
}

// GetActiveNodes returns all active nodes
func (c *IntegrityRewardContract) GetActiveNodes() []*IntegrityNode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	nodes := make([]*IntegrityNode, 0)
	for _, node := range c.Nodes {
		if node.Status == "active" {
			nodes = append(nodes, node)
		}
	}

	return nodes
}

// GetQualifiedNodes returns all nodes qualified for rewards
func (c *IntegrityRewardContract) GetQualifiedNodes() []*IntegrityNode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	nodes := make([]*IntegrityNode, 0)
	for _, node := range c.Nodes {
		if node.Status == "active" && node.Score >= MinQualifiedScore {
			nodes = append(nodes, node)
		}
	}

	return nodes
}

// GetDistributionHistory returns distribution history (read-only copy)
func (c *IntegrityRewardContract) GetDistributionHistory() []DistributionEvent {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy
	copy := make([]DistributionEvent, len(c.DistributionHistory))
	for i, event := range c.DistributionHistory {
		copy[i] = event
	}
	return copy
}

// GetDistributionProgress returns the progress towards next distribution (0.0 to 1.0)
// Represented as uint32/1000 (e.g., 750 = 0.75 = 75%)
func (c *IntegrityRewardContract) GetDistributionProgress(currentHeight uint64) uint32 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.NextDistributionHeight <= c.LastDistributionHeight {
		return 0
	}

	progress := currentHeight - c.LastDistributionHeight
	interval := c.NextDistributionHeight - c.LastDistributionHeight

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

// GetNodeCount returns the total number of registered nodes
func (c *IntegrityRewardContract) GetNodeCount() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.NodeCount
}

// GetActiveNodeCount returns the number of active nodes
func (c *IntegrityRewardContract) GetActiveNodeCount() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	count := uint64(0)
	for _, node := range c.Nodes {
		if node.Status == "active" {
			count++
		}
	}
	return count
}

// GetQualifiedNodeCount returns the number of qualified nodes
func (c *IntegrityRewardContract) GetQualifiedNodeCount() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	count := uint64(0)
	for _, node := range c.Nodes {
		if node.Status == "active" && node.Score >= MinQualifiedScore {
			count++
		}
	}
	return count
}

// CalculateExpectedReward calculates expected reward for a node at next distribution
// Useful for node operators to estimate earnings
func (c *IntegrityRewardContract) CalculateExpectedReward(nodeID string) uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	node, exists := c.Nodes[nodeID]
	if !exists || c.RewardPool == 0 {
		return 0
	}

	// Check if node qualifies
	if node.Status != "active" || node.Score < MinQualifiedScore {
		return 0
	}

	// Calculate node's weight
	nodeWeight := uint64(node.Score)

	// Calculate total weight of all qualified nodes
	totalWeight := uint64(0)
	for _, n := range c.Nodes {
		if n.Status == "active" && n.Score >= MinQualifiedScore {
			totalWeight += uint64(n.Score)
		}
	}

	if totalWeight == 0 {
		return 0
	}

	// Calculate expected reward
	expectedReward := (nodeWeight * c.RewardPool) / totalWeight
	return expectedReward
}

// SetDistributionInterval sets the distribution interval (admin function)
// Should only be callable by governance contract
func (c *IntegrityRewardContract) SetDistributionInterval(interval uint64) error {
	if interval == 0 {
		return errors.New("interval must be greater than 0")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.DistributionInterval = interval
	// Recalculate next distribution height
	if c.LastDistributionHeight > 0 {
		c.NextDistributionHeight = c.LastDistributionHeight + interval
	} else {
		c.NextDistributionHeight = interval
	}

	return nil
}

// GetContractInfo returns comprehensive contract information
type ContractInfo struct {
	ContractAddress        string `json:"contractAddress"`
	RewardPool             uint64 `json:"rewardPool"`
	DistributionInterval   uint64 `json:"distributionInterval"`
	LastDistributionHeight uint64 `json:"lastDistributionHeight"`
	NextDistributionHeight uint64 `json:"nextDistributionHeight"`
	TotalDistributed       uint64 `json:"totalDistributed"`
	TotalNodes             uint64 `json:"totalNodes"`
	ActiveNodes            uint64 `json:"activeNodes"`
	QualifiedNodes         uint64 `json:"qualifiedNodes"`
	DistributionProgress   uint32 `json:"distributionProgress"`
}

// GetContractInfo returns comprehensive contract information
func (c *IntegrityRewardContract) GetContractInfo(currentHeight uint64) ContractInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return ContractInfo{
		ContractAddress:        c.ContractAddress,
		RewardPool:             c.RewardPool,
		DistributionInterval:   c.DistributionInterval,
		LastDistributionHeight: c.LastDistributionHeight,
		NextDistributionHeight: c.NextDistributionHeight,
		TotalDistributed:       c.TotalDistributed,
		TotalNodes:             c.NodeCount,
		ActiveNodes:            c.getActiveNodeCount(),
		QualifiedNodes:         c.getQualifiedNodeCount(),
		DistributionProgress:   c.getDistributionProgress(currentHeight),
	}
}

// getActiveNodeCount returns active node count (internal, no lock)
func (c *IntegrityRewardContract) getActiveNodeCount() uint64 {
	count := uint64(0)
	for _, node := range c.Nodes {
		if node.Status == "active" {
			count++
		}
	}
	return count
}

// getQualifiedNodeCount returns qualified node count (internal, no lock)
func (c *IntegrityRewardContract) getQualifiedNodeCount() uint64 {
	count := uint64(0)
	for _, node := range c.Nodes {
		if node.Status == "active" && node.Score >= MinQualifiedScore {
			count++
		}
	}
	return count
}

// getDistributionProgress returns distribution progress (internal, no lock)
func (c *IntegrityRewardContract) getDistributionProgress(currentHeight uint64) uint32 {
	if c.NextDistributionHeight <= c.LastDistributionHeight {
		return 0
	}

	progress := currentHeight - c.LastDistributionHeight
	interval := c.NextDistributionHeight - c.LastDistributionHeight

	if interval == 0 {
		return 0
	}

	p := (progress * 1000) / interval
	if p > 1000 {
		return 1000
	}
	return uint32(p)
}
