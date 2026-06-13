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
	"testing"
)

// TestIntegrityRewardContract_Creation tests contract creation
func TestIntegrityRewardContract_Creation(t *testing.T) {
	contract := NewIntegrityRewardContract()

	if contract.ContractAddress == "" {
		t.Fatal("Expected non-empty contract address")
	}

	// Verify address format
	if len(contract.ContractAddress) < 10 {
		t.Errorf("Contract address too short: %s", contract.ContractAddress)
	}

	if contract.RewardPool != 0 {
		t.Errorf("Expected initial reward pool 0, got %d", contract.RewardPool)
	}

	if contract.DistributionInterval != DefaultDistributionInterval {
		t.Errorf("Expected distribution interval %d, got %d", DefaultDistributionInterval, contract.DistributionInterval)
	}

	if contract.NodeCount != 0 {
		t.Errorf("Expected initial node count 0, got %d", contract.NodeCount)
	}
}

// TestIntegrityRewardContract_AddToRewardPool tests adding rewards to pool
func TestIntegrityRewardContract_AddToRewardPool(t *testing.T) {
	contract := NewIntegrityRewardContract()

	// Add 1% of 1000000 block reward = 10000
	contract.AddToRewardPool(1000000)
	pool := contract.GetRewardPool()
	if pool != 10000 {
		t.Errorf("Expected pool 10000, got %d", pool)
	}

	// Add another block
	contract.AddToRewardPool(1000000)
	pool = contract.GetRewardPool()
	if pool != 20000 {
		t.Errorf("Expected pool 20000 after 2 blocks, got %d", pool)
	}
}

// TestIntegrityRewardContract_RegisterNode tests node registration
func TestIntegrityRewardContract_RegisterNode(t *testing.T) {
	contract := NewIntegrityRewardContract()

	// Register node
	err := contract.RegisterNode("node1", "address1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify node was registered
	if contract.GetNodeCount() != 1 {
		t.Errorf("Expected 1 node, got %d", contract.GetNodeCount())
	}

	node, err := contract.GetNode("node1")
	if err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	if node.Address != "address1" {
		t.Errorf("Expected address 'address1', got '%s'", node.Address)
	}

	if node.Score != 100 {
		t.Errorf("Expected initial score 100, got %d", node.Score)
	}

	if node.Status != "active" {
		t.Errorf("Expected status 'active', got '%s'", node.Status)
	}
}

// TestIntegrityRewardContract_RegisterNode_Duplicate tests duplicate registration
func TestIntegrityRewardContract_RegisterNode_Duplicate(t *testing.T) {
	contract := NewIntegrityRewardContract()

	// Register node
	contract.RegisterNode("node1", "address1")

	// Try to register again
	err := contract.RegisterNode("node1", "address2")
	if err == nil {
		t.Fatal("Expected error for duplicate registration, got nil")
	}
}

// TestIntegrityRewardContract_UpdateNodeScore tests score updates
func TestIntegrityRewardContract_UpdateNodeScore(t *testing.T) {
	contract := NewIntegrityRewardContract()
	contract.RegisterNode("node1", "address1")

	// Update score
	err := contract.UpdateNodeScore("node1", 80, "active")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	node, _ := contract.GetNode("node1")
	if node.Score != 80 {
		t.Errorf("Expected score 80, got %d", node.Score)
	}
}

// TestIntegrityRewardContract_Distribution tests reward distribution
func TestIntegrityRewardContract_Distribution(t *testing.T) {
	contract := NewIntegrityRewardContract()

	// Add rewards to pool
	for i := 0; i < 100; i++ {
		contract.AddToRewardPool(1000000) // 10000 per block
	}

	// Register and qualify nodes
	contract.RegisterNode("node1", "address1")
	contract.RegisterNode("node2", "address2")
	contract.RegisterNode("node3", "address3")

	// Update scores (node3 below qualified threshold)
	contract.UpdateNodeScore("node1", 100, "active")
	contract.UpdateNodeScore("node2", 80, "active")
	contract.UpdateNodeScore("node3", 50, "active")

	// Distribute at distribution height (5082)
	rewards, err := contract.DistributeRewards(5082)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Only node1 and node2 should receive rewards (node3 below threshold)
	if len(rewards) != 2 {
		t.Errorf("Expected 2 rewarded nodes, got %d", len(rewards))
	}

	// Node1 should receive more than node2 (higher score)
	if rewards["address1"] <= rewards["address2"] {
		t.Errorf("Expected address1 to receive more than address2")
	}

	// Pool should be empty after distribution
	pool := contract.GetRewardPool()
	if pool != 0 {
		t.Errorf("Expected pool 0 after distribution, got %d", pool)
	}

	// Verify total distributed
	if contract.TotalDistributed == 0 {
		t.Error("Expected non-zero total distributed")
	}
}

// TestIntegrityRewardContract_DistributionInterval tests distribution interval logic
func TestIntegrityRewardContract_DistributionInterval(t *testing.T) {
	contract := NewIntegrityRewardContract()

	// Add rewards
	contract.AddToRewardPool(1000000)

	contract.RegisterNode("node1", "address1")
	contract.UpdateNodeScore("node1", 100, "active")

	// Try to distribute before interval (height 1000)
	_, err := contract.DistributeRewards(1000)
	if err == nil {
		t.Fatal("Expected error for early distribution")
	}

	// Distribute at correct height (5082)
	_, err = contract.DistributeRewards(5082)
	if err != nil {
		t.Fatalf("Unexpected error at distribution height: %v", err)
	}

	// Next distribution should be at 5082 + 5082 = 10164
	nextHeight := contract.GetNextDistributionHeight()
	if nextHeight != 10164 {
		t.Errorf("Expected next distribution height 10164, got %d", nextHeight)
	}
}

// TestIntegrityRewardContract_Qualification tests reward qualification
func TestIntegrityRewardContract_Qualification(t *testing.T) {
	contract := NewIntegrityRewardContract()

	// Register nodes with different scores
	contract.RegisterNode("node1", "address1")
	contract.RegisterNode("node2", "address2")
	contract.RegisterNode("node3", "address3")

	// Update scores
	contract.UpdateNodeScore("node1", 100, "active") // Qualified
	contract.UpdateNodeScore("node2", 60, "active")  // Just qualified
	contract.UpdateNodeScore("node3", 59, "active")  // Not qualified

	qualifiedNodes := contract.GetQualifiedNodes()
	if len(qualifiedNodes) != 2 {
		t.Errorf("Expected 2 qualified nodes, got %d", len(qualifiedNodes))
	}
}

// TestIntegrityRewardContract_Progress tests distribution progress calculation
func TestIntegrityRewardContract_Progress(t *testing.T) {
	contract := NewIntegrityRewardContract()

	// At start (height 0)
	progress := contract.GetDistributionProgress(0)
	if progress != 0 {
		t.Errorf("Expected 0%% progress at height 0, got %d", progress)
	}

	// Halfway (height 2541, interval is 5082)
	progress = contract.GetDistributionProgress(2541)
	if progress < 499 || progress > 501 {
		t.Errorf("Expected ~500 (50%%) progress at height 2541, got %d", progress)
	}

	// Complete (height 5082)
	progress = contract.GetDistributionProgress(5082)
	if progress != 1000 {
		t.Errorf("Expected 1000 (100%%) progress at height 5082, got %d", progress)
	}
}

// TestIntegrityRewardContract_ExpectedReward tests expected reward calculation
func TestIntegrityRewardContract_ExpectedReward(t *testing.T) {
	contract := NewIntegrityRewardContract()

	// Add rewards
	contract.AddToRewardPool(1000000)

	// Register nodes
	contract.RegisterNode("node1", "address1")
	contract.RegisterNode("node2", "address2")

	// Update scores
	contract.UpdateNodeScore("node1", 100, "active")
	contract.UpdateNodeScore("node2", 100, "active")

	// Calculate expected rewards (should be equal)
	reward1 := contract.CalculateExpectedReward("node1")
	reward2 := contract.CalculateExpectedReward("node2")

	if reward1 != reward2 {
		t.Errorf("Expected equal rewards, got %d vs %d", reward1, reward2)
	}

	// Update node1 score to be higher
	contract.UpdateNodeScore("node1", 100, "active")
	contract.UpdateNodeScore("node2", 50, "active")

	// Node1 should expect more (but can't distribute yet)
	reward1 = contract.CalculateExpectedReward("node1")
	reward2 = contract.CalculateExpectedReward("node2")

	if reward1 <= reward2 {
		t.Errorf("Expected node1 to have higher expected reward")
	}
}

// TestIntegrityRewardContract_NodeCounts tests node count methods
func TestIntegrityRewardContract_NodeCounts(t *testing.T) {
	contract := NewIntegrityRewardContract()

	// Register nodes
	contract.RegisterNode("node1", "address1")
	contract.RegisterNode("node2", "address2")
	contract.RegisterNode("node3", "address3")
	contract.RegisterNode("node4", "address4")

	// Update scores and status
	contract.UpdateNodeScore("node1", 100, "active")
	contract.UpdateNodeScore("node2", 80, "active")
	contract.UpdateNodeScore("node3", 50, "active") // Below qualified
	contract.UpdateNodeScore("node4", 100, "suspended")

	if contract.GetNodeCount() != 4 {
		t.Errorf("Expected 4 total nodes, got %d", contract.GetNodeCount())
	}

	if contract.GetActiveNodeCount() != 3 {
		t.Errorf("Expected 3 active nodes, got %d", contract.GetActiveNodeCount())
	}

	if contract.GetQualifiedNodeCount() != 2 {
		t.Errorf("Expected 2 qualified nodes, got %d", contract.GetQualifiedNodeCount())
	}
}

// TestIntegrityRewardContract_ContractInfo tests contract info retrieval
func TestIntegrityRewardContract_ContractInfo(t *testing.T) {
	contract := NewIntegrityRewardContract()

	// Add rewards
	contract.AddToRewardPool(1000000)

	// Register node
	contract.RegisterNode("node1", "address1")
	contract.UpdateNodeScore("node1", 100, "active")

	// Get contract info at height 2541 (halfway)
	info := contract.GetContractInfo(2541)

	if info.ContractAddress != contract.ContractAddress {
		t.Errorf("Contract address mismatch")
	}

	if info.RewardPool != 10000 {
		t.Errorf("Expected reward pool 10000, got %d", info.RewardPool)
	}

	if info.TotalNodes != 1 {
		t.Errorf("Expected 1 total node, got %d", info.TotalNodes)
	}

	if info.ActiveNodes != 1 {
		t.Errorf("Expected 1 active node, got %d", info.ActiveNodes)
	}

	if info.QualifiedNodes != 1 {
		t.Errorf("Expected 1 qualified node, got %d", info.QualifiedNodes)
	}

	if info.DistributionProgress < 499 || info.DistributionProgress > 501 {
		t.Errorf("Expected ~500 progress, got %d", info.DistributionProgress)
	}
}

// TestIntegrityRewardContract_ConcurrentAccess tests thread safety
func TestIntegrityRewardContract_ConcurrentAccess(t *testing.T) {
	contract := NewIntegrityRewardContract()

	done := make(chan bool)

	// Concurrent node registration
	go func() {
		for i := 0; i < 50; i++ {
			nodeID := string(rune('a'+i%26)) + string(rune('0'+i/26))
			contract.RegisterNode(nodeID, "address"+string(rune('0'+i)))
		}
		done <- true
	}()

	// Concurrent reward pool updates
	go func() {
		for i := 0; i < 50; i++ {
			contract.AddToRewardPool(1000000)
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < 50; i++ {
			_ = contract.GetRewardPool()
			_ = contract.GetNodeCount()
			_ = contract.GetContractInfo(1000)
		}
		done <- true
	}()

	<-done
	<-done
	<-done
}
