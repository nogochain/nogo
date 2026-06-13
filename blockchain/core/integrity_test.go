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
	"testing"
)

// TestNodeIntegrity_Creation tests node integrity record creation
func TestNodeIntegrity_Creation(t *testing.T) {
	node := NewNodeIntegrity("node1")

	if node.NodeID != "node1" {
		t.Errorf("Expected NodeID 'node1', got '%s'", node.NodeID)
	}
	if node.Score != 100 {
		t.Errorf("Expected initial score 100, got %d", node.Score)
	}
	if node.Status != StatusActive {
		t.Errorf("Expected initial status Active, got %v", node.Status)
	}
	if node.OnlineTime != 0 {
		t.Errorf("Expected initial OnlineTime 0, got %d", node.OnlineTime)
	}
}

// TestNodeIntegrity_ScoreUpdate tests score update with thread safety
func TestNodeIntegrity_ScoreUpdate(t *testing.T) {
	node := NewNodeIntegrity("node1")

	// Test valid score update
	err := node.SetScore(80)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if node.GetScore() != 80 {
		t.Errorf("Expected score 80, got %d", node.GetScore())
	}

	// Test banned node cannot update score
	node.SetStatus(StatusBanned)
	err = node.SetScore(90)
	if err == nil {
		t.Fatal("Expected error for banned node, got nil")
	}
}

// TestNodeIntegrity_Violations tests violation tracking
func TestNodeIntegrity_Violations(t *testing.T) {
	node := NewNodeIntegrity("node1")

	// Add violations
	node.AddViolation(ViolationOffline, "Node offline for 1 hour", 10)
	node.AddViolation(ViolationInvalidBlock, "Produced invalid block", 20)

	violations := node.GetViolations()
	if len(violations) != 2 {
		t.Errorf("Expected 2 violations, got %d", len(violations))
	}

	if violations[0].Type != ViolationOffline {
		t.Errorf("Expected first violation type Offline, got %v", violations[0].Type)
	}
	if violations[0].PenaltyApplied != 10 {
		t.Errorf("Expected first violation penalty 10, got %d", violations[0].PenaltyApplied)
	}
}

// TestNodeIntegrity_OnlineRate tests online rate calculation
func TestNodeIntegrity_OnlineRate(t *testing.T) {
	node := NewNodeIntegrity("node1")

	// 0% online rate
	node.UpdateOnlineTime(0, 1000)
	if node.GetOnlineRate() != 0 {
		t.Errorf("Expected 0%% online rate, got %d", node.GetOnlineRate())
	}

	// 50% online rate
	node2 := NewNodeIntegrity("node2")
	node2.UpdateOnlineTime(500, 1000)
	rate := node2.GetOnlineRate()
	if rate != 50 {
		t.Errorf("Expected 50%% online rate, got %d", rate)
	}

	// 100% online rate
	node3 := NewNodeIntegrity("node3")
	node3.UpdateOnlineTime(1000, 1000)
	if node3.GetOnlineRate() != 100 {
		t.Errorf("Expected 100%% online rate, got %d", node3.GetOnlineRate())
	}
}

// TestNodeIntegrity_ValidBlockRate tests valid block rate calculation
func TestNodeIntegrity_ValidBlockRate(t *testing.T) {
	node := NewNodeIntegrity("node1")

	// No blocks yet - should return 100%
	if node.GetValidBlockRate() != 100 {
		t.Errorf("Expected 100%% valid block rate (no blocks), got %d", node.GetValidBlockRate())
	}

	// 50% valid blocks
	node2 := NewNodeIntegrity("node2")
	for i := 0; i < 5; i++ {
		node2.AddValidBlock()
		node2.AddInvalidBlock()
	}
	if node2.GetValidBlockRate() != 50 {
		t.Errorf("Expected 50%% valid block rate, got %d", node2.GetValidBlockRate())
	}

	// 100% valid blocks
	node3 := NewNodeIntegrity("node3")
	for i := 0; i < 10; i++ {
		node3.AddValidBlock()
	}
	if node3.GetValidBlockRate() != 100 {
		t.Errorf("Expected 100%% valid block rate, got %d", node3.GetValidBlockRate())
	}
}

// TestScoreCalculator_OnlineScore tests online score calculation
func TestScoreCalculator_OnlineScore(t *testing.T) {
	calc := NewScoreCalculator()

	// 0% online = 0 points
	score := calc.CalculateOnlineScore(0, 1000)
	if score != 0 {
		t.Errorf("Expected 0 points for 0%% online, got %d", score)
	}

	// 50% online = 20 points (50% of 40)
	score = calc.CalculateOnlineScore(500, 1000)
	if score != 20 {
		t.Errorf("Expected 20 points for 50%% online, got %d", score)
	}

	// 100% online = 40 points
	score = calc.CalculateOnlineScore(1000, 1000)
	if score != 40 {
		t.Errorf("Expected 40 points for 100%% online, got %d", score)
	}

	// Division by zero
	score = calc.CalculateOnlineScore(0, 0)
	if score != 0 {
		t.Errorf("Expected 0 points for division by zero, got %d", score)
	}
}

// TestScoreCalculator_QualityScore tests quality score calculation
func TestScoreCalculator_QualityScore(t *testing.T) {
	calc := NewScoreCalculator()

	// No blocks = middle score (15)
	score := calc.CalculateQualityScore(0, 0)
	if score != 15 {
		t.Errorf("Expected 15 points for no blocks, got %d", score)
	}

	// 50% valid = 15 points (50% of 30)
	score = calc.CalculateQualityScore(5, 5)
	if score != 15 {
		t.Errorf("Expected 15 points for 50%% valid, got %d", score)
	}

	// 100% valid = 30 points
	score = calc.CalculateQualityScore(10, 0)
	if score != 30 {
		t.Errorf("Expected 30 points for 100%% valid, got %d", score)
	}
}

// TestScoreCalculator_ContinuityScore tests continuity score calculation
func TestScoreCalculator_ContinuityScore(t *testing.T) {
	calc := NewScoreCalculator()

	// 0 days = 0 points
	score := calc.CalculateContinuityScore(0)
	if score != 0 {
		t.Errorf("Expected 0 points for 0 days, got %d", score)
	}

	// 5 days = 10 points (5 * 2)
	score = calc.CalculateContinuityScore(5)
	if score != 10 {
		t.Errorf("Expected 10 points for 5 days, got %d", score)
	}

	// 10+ days = 20 points (max)
	score = calc.CalculateContinuityScore(10)
	if score != 20 {
		t.Errorf("Expected 20 points for 10+ days, got %d", score)
	}

	score = calc.CalculateContinuityScore(100)
	if score != 20 {
		t.Errorf("Expected 20 points max for 100 days, got %d", score)
	}
}

// TestScoreCalculator_PeerScore tests peer score calculation
func TestScoreCalculator_PeerScore(t *testing.T) {
	calc := NewScoreCalculator()

	// No peer scores = middle score (5)
	score := calc.CalculatePeerScore(map[string]uint8{})
	if score != 5 {
		t.Errorf("Expected 5 points for no peers, got %d", score)
	}

	// Average peer score 80 = 8 points (80/10)
	peerScores := map[string]uint8{
		"peer1": 80,
		"peer2": 80,
		"peer3": 80,
	}
	score = calc.CalculatePeerScore(peerScores)
	if score != 8 {
		t.Errorf("Expected 8 points for avg 80 peer score, got %d", score)
	}

	// Perfect peer scores = 10 points
	peerScores = map[string]uint8{
		"peer1": 100,
		"peer2": 100,
	}
	score = calc.CalculatePeerScore(peerScores)
	if score != 10 {
		t.Errorf("Expected 10 points for perfect peer scores, got %d", score)
	}
}

// TestScoreCalculator_TotalScore tests total score calculation
func TestScoreCalculator_TotalScore(t *testing.T) {
	calc := NewScoreCalculator()

	node := &NodeIntegrity{
		OnlineTime:      1000,
		TotalTime:       1000, // 100% online = 40 points
		ValidBlocks:     10,   // 100% valid = 30 points
		InvalidBlocks:   0,
		ConsecutiveDays: 10, // 10+ days = 20 points
		PeerScores: map[string]uint8{
			"peer1": 100, // 100/10 = 10 points
		},
		Score:  100,
		Status: StatusActive,
	}

	total := calc.CalculateTotalScore(node)
	if total != 100 {
		t.Errorf("Expected 100 total points, got %d", total)
	}

	// Test with details
	total, online, quality, continuity, peer := calc.CalculateTotalScoreWithDetails(node)
	if total != 100 {
		t.Errorf("Expected 100 total points, got %d", total)
	}
	if online != 40 {
		t.Errorf("Expected 40 online points, got %d", online)
	}
	if quality != 30 {
		t.Errorf("Expected 30 quality points, got %d", quality)
	}
	if continuity != 20 {
		t.Errorf("Expected 20 continuity points, got %d", continuity)
	}
	if peer != 10 {
		t.Errorf("Expected 10 peer points, got %d", peer)
	}
}

// TestScoreCalculator_Penalty tests penalty application
func TestScoreCalculator_Penalty(t *testing.T) {
	calc := NewScoreCalculator()

	// Test Offline penalty (-10 points)
	node := NewNodeIntegrity("node1")
	node.SetScore(100)
	newScore := calc.ApplyPenalty(node, ViolationOffline)
	if newScore != 90 {
		t.Errorf("Expected score 90 after offline penalty, got %d", newScore)
	}
	if node.ConsecutiveDays != 0 {
		t.Errorf("Expected consecutive days reset to 0, got %d", node.ConsecutiveDays)
	}

	// Test InvalidBlock penalty (-20 points)
	node2 := NewNodeIntegrity("node2")
	node2.SetScore(100)
	newScore = calc.ApplyPenalty(node2, ViolationInvalidBlock)
	if newScore != 80 {
		t.Errorf("Expected score 80 after invalid block penalty, got %d", newScore)
	}

	// Test DoubleSign penalty (score = 0, ban)
	node3 := NewNodeIntegrity("node3")
	node3.SetScore(100)
	newScore = calc.ApplyPenalty(node3, ViolationDoubleSign)
	if newScore != 0 {
		t.Errorf("Expected score 0 after double sign penalty, got %d", newScore)
	}
	if node3.Status != StatusBanned {
		t.Errorf("Expected status Banned after double sign, got %v", node3.Status)
	}

	// Test MaliciousFork penalty (score = 0, ban)
	node4 := NewNodeIntegrity("node4")
	node4.SetScore(100)
	newScore = calc.ApplyPenalty(node4, ViolationMaliciousFork)
	if newScore != 0 {
		t.Errorf("Expected score 0 after malicious fork penalty, got %d", newScore)
	}
	if node4.Status != StatusBanned {
		t.Errorf("Expected status Banned after malicious fork, got %v", node4.Status)
	}
}

// TestIntegrityRewardDistributor_AddToPool tests reward pool accumulation
func TestIntegrityRewardDistributor_AddToPool(t *testing.T) {
	distributor := NewIntegrityRewardDistributor()

	// Add 1% of 1000000 block reward = 10000
	distributor.AddToPool(1000000)
	pool := distributor.GetRewardPool()
	if pool != 10000 {
		t.Errorf("Expected pool 10000, got %d", pool)
	}

	// Add another block
	distributor.AddToPool(1000000)
	pool = distributor.GetRewardPool()
	if pool != 20000 {
		t.Errorf("Expected pool 20000 after 2 blocks, got %d", pool)
	}
}

// TestIntegrityRewardDistributor_Distribution tests reward distribution
func TestIntegrityRewardDistributor_Distribution(t *testing.T) {
	distributor := NewIntegrityRewardDistributor()

	// Add rewards to pool
	for i := 0; i < 100; i++ {
		distributor.AddToPool(1000000) // 10000 per block
	}

	pool := distributor.GetRewardPool()
	if pool == 0 {
		t.Fatal("Expected non-zero reward pool")
	}

	// Create qualified nodes
	node1 := NewNodeIntegrity("node1")
	node1.SetScore(100) // Perfect score
	node2 := NewNodeIntegrity("node2")
	node2.SetScore(80) // Good score
	node3 := NewNodeIntegrity("node3")
	node3.SetScore(50) // Below qualified threshold (60)

	nodes := []*NodeIntegrity{node1, node2, node3}

	// Distribute at distribution height (5082)
	rewards, err := distributor.DistributeRewards(nodes, 5082)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Only node1 and node2 should receive rewards (node3 below threshold)
	if len(rewards) != 2 {
		t.Errorf("Expected 2 rewarded nodes, got %d", len(rewards))
	}

	// Node1 should receive more than node2 (higher score)
	if rewards["node1"] <= rewards["node2"] {
		t.Errorf("Expected node1 to receive more than node2")
	}

	// Pool should be empty after distribution
	pool = distributor.GetRewardPool()
	if pool != 0 {
		t.Errorf("Expected pool 0 after distribution, got %d", pool)
	}
}

// TestIntegrityRewardDistributor_DistributionInterval tests distribution interval logic
func TestIntegrityRewardDistributor_DistributionInterval(t *testing.T) {
	distributor := NewIntegrityRewardDistributor()

	// Add rewards
	distributor.AddToPool(1000000)

	node := NewNodeIntegrity("node1")
	node.SetScore(100)
	nodes := []*NodeIntegrity{node}

	// Try to distribute before interval (height 1000)
	_, err := distributor.DistributeRewards(nodes, 1000)
	if err == nil {
		t.Fatal("Expected error for early distribution")
	}

	// Distribute at correct height (5082)
	_, err = distributor.DistributeRewards(nodes, 5082)
	if err != nil {
		t.Fatalf("Unexpected error at distribution height: %v", err)
	}

	// Next distribution should be at 5082 + 5082 = 10164
	nextHeight := distributor.GetNextDistributionHeight()
	if nextHeight != 10164 {
		t.Errorf("Expected next distribution height 10164, got %d", nextHeight)
	}
}

// TestIntegrityRewardDistributor_Qualification tests reward qualification
func TestIntegrityRewardDistributor_Qualification(t *testing.T) {
	calc := NewScoreCalculator()
	thresholds := DefaultScoreThresholds()

	// Node with score 60 (minimum qualified)
	node1 := NewNodeIntegrity("node1")
	node1.SetScore(60)
	if !calc.QualifiesForReward(node1, thresholds) {
		t.Error("Expected node with score 60 to qualify")
	}

	// Node with score 59 (below qualified)
	node2 := NewNodeIntegrity("node2")
	node2.SetScore(59)
	if calc.QualifiesForReward(node2, thresholds) {
		t.Error("Expected node with score 59 to not qualify")
	}

	// Suspended node doesn't qualify even with high score
	node3 := NewNodeIntegrity("node3")
	node3.SetScore(100)
	node3.SetStatus(StatusSuspended)
	if calc.QualifiesForReward(node3, thresholds) {
		t.Error("Expected suspended node to not qualify")
	}
}

// TestIntegrityRewardDistributor_Progress tests distribution progress calculation
func TestIntegrityRewardDistributor_Progress(t *testing.T) {
	distributor := NewIntegrityRewardDistributor()

	// At start (height 0)
	progress := distributor.GetDistributionProgress(0)
	if progress != 0 {
		t.Errorf("Expected 0%% progress at height 0, got %d", progress)
	}

	// Halfway (height 2541, interval is 5082)
	progress = distributor.GetDistributionProgress(2541)
	if progress < 499 || progress > 501 {
		t.Errorf("Expected ~500 (50%%) progress at height 2541, got %d", progress)
	}

	// Complete (height 5082)
	progress = distributor.GetDistributionProgress(5082)
	if progress != 1000 {
		t.Errorf("Expected 1000 (100%%) progress at height 5082, got %d", progress)
	}
}

// TestNodeIntegrityManager tests node management
func TestNodeIntegrityManager(t *testing.T) {
	manager := NewNodeIntegrityManager()

	// Get or create node
	node1 := manager.GetOrCreateNode("node1")
	if node1 == nil {
		t.Fatal("Expected non-nil node")
	}

	// Get existing node
	node1Again := manager.GetOrCreateNode("node1")
	if node1Again != node1 {
		t.Error("Expected same node instance")
	}

	// Count nodes
	if manager.Count() != 1 {
		t.Errorf("Expected 1 node, got %d", manager.Count())
	}

	// Add more nodes
	manager.GetOrCreateNode("node2")
	manager.GetOrCreateNode("node3")

	if manager.Count() != 3 {
		t.Errorf("Expected 3 nodes, got %d", manager.Count())
	}

	// Get active nodes (all should be active)
	activeNodes := manager.GetActiveNodes()
	if len(activeNodes) != 3 {
		t.Errorf("Expected 3 active nodes, got %d", len(activeNodes))
	}

	// Ban one node
	node3 := manager.GetNode("node3")
	node3.SetStatus(StatusBanned)

	activeNodes = manager.GetActiveNodes()
	if len(activeNodes) != 2 {
		t.Errorf("Expected 2 active nodes after ban, got %d", len(activeNodes))
	}

	if manager.CountActive() != 2 {
		t.Errorf("Expected 2 active nodes count, got %d", manager.CountActive())
	}
}

// TestIntegrity_ConcurrentAccess tests thread safety
func TestIntegrity_ConcurrentAccess(t *testing.T) {
	manager := NewNodeIntegrityManager()
	distributor := NewIntegrityRewardDistributor()
	calc := NewScoreCalculator()

	done := make(chan bool)

	// Concurrent node creation and updates
	go func() {
		for i := 0; i < 100; i++ {
			node := manager.GetOrCreateNode("node")
			node.UpdateOnlineTime(100, 1000)
			node.AddValidBlock()
			calc.UpdateNodeScore(node)
		}
		done <- true
	}()

	// Concurrent reward pool updates
	go func() {
		for i := 0; i < 100; i++ {
			distributor.AddToPool(1000000)
			_ = distributor.GetRewardPool()
		}
		done <- true
	}()

	// Concurrent score calculations and reward checks
	go func() {
		for i := 0; i < 100; i++ {
			node := manager.GetOrCreateNode("node")
			_ = calc.CalculateTotalScore(node)
			_ = distributor.GetRewardPool()
		}
		done <- true
	}()

	<-done
	<-done
	<-done
}
