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
	"sync"
)

// Scoring constants define the maximum scores for each category
const (
	// MaxOnlineScore is the maximum online time score (40% of total)
	MaxOnlineScore uint8 = 40
	// MaxQualityScore is the maximum block quality score (30% of total)
	MaxQualityScore uint8 = 30
	// MaxContinuityScore is the maximum continuity score (20% of total)
	MaxContinuityScore uint8 = 20
	// MaxPeerScore is the maximum peer score (10% of total)
	MaxPeerScore uint8 = 10
	// MaxTotalScore is the maximum total score (100)
	MaxTotalScore uint8 = 100
)

// ScoreCalculator provides thread-safe score calculation functions
type ScoreCalculator struct {
	mu sync.RWMutex
}

// NewScoreCalculator creates a new score calculator
func NewScoreCalculator() *ScoreCalculator {
	return &ScoreCalculator{}
}

// CalculateOnlineScore calculates online time score (0-40 points)
// Formula: (onlineTime / totalTime) * 40
// Math safety: uses integer arithmetic to avoid float64 precision issues
func (s *ScoreCalculator) CalculateOnlineScore(onlineTime, totalTime uint64) uint8 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if totalTime == 0 {
		return 0
	}

	// Calculate percentage with integer arithmetic
	// (onlineTime * 100 / totalTime) * 40 / 100 = (onlineTime * 40) / totalTime
	score := (onlineTime * uint64(MaxOnlineScore)) / totalTime

	if score > uint64(MaxOnlineScore) {
		return MaxOnlineScore
	}
	return uint8(score)
}

// CalculateQualityScore calculates block quality score (0-30 points)
// Formula: (validBlocks / totalBlocks) * 30
// Math safety: uses integer arithmetic, handles division by zero
func (s *ScoreCalculator) CalculateQualityScore(validBlocks, invalidBlocks uint64) uint8 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalBlocks := validBlocks + invalidBlocks
	if totalBlocks == 0 {
		// No blocks produced yet, return middle score
		return MaxQualityScore / 2
	}

	// Calculate percentage with integer arithmetic
	score := (validBlocks * uint64(MaxQualityScore)) / totalBlocks

	if score > uint64(MaxQualityScore) {
		return MaxQualityScore
	}
	return uint8(score)
}

// CalculateContinuityScore calculates continuity score (0-20 points)
// Formula: min(consecutiveDays * 2, 20)
// Rewards consistent long-term participation
func (s *ScoreCalculator) CalculateContinuityScore(consecutiveDays uint64) uint8 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 10 days = max score (20 points)
	// Each day = 2 points
	score := consecutiveDays * 2

	if score >= uint64(MaxContinuityScore) {
		return MaxContinuityScore
	}
	return uint8(score)
}

// CalculatePeerScore calculates peer score (0-10 points)
// Formula: average of all peer scores
// Math safety: uses integer arithmetic, handles empty peer list
func (s *ScoreCalculator) CalculatePeerScore(peerScores map[string]uint8) uint8 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(peerScores) == 0 {
		// No peer scores yet, return middle score
		return MaxPeerScore / 2
	}

	// Calculate average peer score
	var sum uint64
	for _, score := range peerScores {
		sum += uint64(score)
	}

	// Average the scores
	avgScore := sum / uint64(len(peerScores))

	// Scale to max peer score (10)
	// Peer scores are already 0-100, so divide by 10
	score := avgScore / 10

	if score > uint64(MaxPeerScore) {
		return MaxPeerScore
	}
	return uint8(score)
}

// CalculateTotalScore calculates the total integrity score (0-100 points)
// Combines all scoring categories:
// - Online time: 40 points max
// - Block quality: 30 points max
// - Continuity: 20 points max
// - Peer score: 10 points max
// Math safety: uses integer arithmetic, validates all inputs
func (s *ScoreCalculator) CalculateTotalScore(node *NodeIntegrity) uint8 {
	if node == nil {
		return 0
	}

	// Get thread-safe snapshot to avoid race conditions
	onlineTime, totalTime, validBlocks, invalidBlocks, consecutiveDays, peerScores := node.GetSnapshot()

	onlineScore := s.CalculateOnlineScore(onlineTime, totalTime)
	qualityScore := s.CalculateQualityScore(validBlocks, invalidBlocks)
	continuityScore := s.CalculateContinuityScore(consecutiveDays)
	peerScore := s.CalculatePeerScore(peerScores)

	// Sum all scores (max 100)
	total := uint64(onlineScore) + uint64(qualityScore) + uint64(continuityScore) + uint64(peerScore)

	if total > uint64(MaxTotalScore) {
		return MaxTotalScore
	}
	return uint8(total)
}

// CalculateTotalScoreWithDetails calculates total score and returns breakdown
// Useful for debugging and transparency
// Thread-safe: uses snapshot to avoid race conditions
func (s *ScoreCalculator) CalculateTotalScoreWithDetails(node *NodeIntegrity) (total uint8, online uint8, quality uint8, continuity uint8, peer uint8) {
	if node == nil {
		return 0, 0, 0, 0, 0
	}

	// Get thread-safe snapshot to avoid race conditions
	onlineTime, totalTime, validBlocks, invalidBlocks, consecutiveDays, peerScores := node.GetSnapshot()

	online = s.CalculateOnlineScore(onlineTime, totalTime)
	quality = s.CalculateQualityScore(validBlocks, invalidBlocks)
	continuity = s.CalculateContinuityScore(consecutiveDays)
	peer = s.CalculatePeerScore(peerScores)

	// Sum with overflow protection
	sum := uint64(online) + uint64(quality) + uint64(continuity) + uint64(peer)
	if sum > uint64(MaxTotalScore) {
		total = MaxTotalScore
	} else {
		total = uint8(sum)
	}

	return total, online, quality, continuity, peer
}

// ApplyPenalty applies a penalty to a node's score based on violation type
// Returns the new score after penalty
// Penalty rules:
// - Offline: -10 points, reset consecutive days
// - InvalidBlock: -20 points
// - DoubleSign: score = 0, ban node
// - MaliciousFork: score = 0, ban node
func (s *ScoreCalculator) ApplyPenalty(node *NodeIntegrity, violationType ViolationType) uint8 {
	if node == nil {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	currentScore := node.Score

	switch violationType {
	case ViolationOffline:
		// -10 points penalty
		if currentScore >= 10 {
			node.Score = currentScore - 10
		} else {
			node.Score = 0
		}
		// Reset consecutive days
		node.ConsecutiveDays = 0
		node.AddViolation(violationType, "Offline/timeout violation", 10)

	case ViolationInvalidBlock:
		// -20 points penalty
		if currentScore >= 20 {
			node.Score = currentScore - 20
		} else {
			node.Score = 0
		}
		node.AddViolation(violationType, "Produced invalid block", 20)

	case ViolationDoubleSign:
		// Score = 0, ban node
		node.Score = 0
		node.Status = StatusBanned
		node.AddViolation(violationType, "Double-signing violation", currentScore)

	case ViolationMaliciousFork:
		// Score = 0, ban node
		node.Score = 0
		node.Status = StatusBanned
		node.AddViolation(violationType, "Malicious forking violation", currentScore)

	default:
		// No penalty for unknown violations
		return currentScore
	}

	return node.Score
}

// UpdateNodeScore updates a node's score based on current metrics
// Returns the new score
func (s *ScoreCalculator) UpdateNodeScore(node *NodeIntegrity) uint8 {
	if node == nil {
		return 0
	}

	newScore := s.CalculateTotalScore(node)

	// Update node score atomically
	node.mu.Lock()
	node.Score = newScore
	node.mu.Unlock()

	return newScore
}

// ScoreThresholds defines score thresholds for different actions
type ScoreThresholds struct {
	// MinQualifiedScore is the minimum score to receive rewards
	MinQualifiedScore uint8
	// WarningScore is the score below which node receives warning
	WarningScore uint8
	// SuspensionScore is the score below which node is suspended
	SuspensionScore uint8
	// BanScore is the score below which node is banned
	BanScore uint8
}

// DefaultScoreThresholds returns default score thresholds
func DefaultScoreThresholds() ScoreThresholds {
	return ScoreThresholds{
		MinQualifiedScore: 60, // Must have 60+ to receive rewards
		WarningScore:      50, // Warning at 50
		SuspensionScore:   30, // Suspension at 30
		BanScore:          10, // Ban at 10
	}
}

// EvaluateNodeStatus evaluates and updates node status based on score
// Returns the new status
func (s *ScoreCalculator) EvaluateNodeStatus(node *NodeIntegrity, thresholds ScoreThresholds) NodeStatus {
	if node == nil {
		return StatusInactive
	}

	score := node.GetScore()

	var newStatus NodeStatus
	switch {
	case score >= thresholds.MinQualifiedScore:
		newStatus = StatusActive
	case score >= thresholds.SuspensionScore:
		newStatus = StatusSuspended
	case score >= thresholds.BanScore:
		newStatus = StatusSuspended // Still suspended, not banned yet
	default:
		newStatus = StatusBanned
	}

	// Update node status if changed
	oldStatus := node.GetStatus()
	if newStatus != oldStatus {
		node.SetStatus(newStatus)
	}

	return newStatus
}

// QualifiesForReward checks if a node qualifies for integrity rewards
// Returns true if score >= MinQualifiedScore and status is Active
func (s *ScoreCalculator) QualifiesForReward(node *NodeIntegrity, thresholds ScoreThresholds) bool {
	if node == nil {
		return false
	}

	score := node.GetScore()
	status := node.GetStatus()

	return score >= thresholds.MinQualifiedScore && status == StatusActive
}

// CalculateRewardShare calculates a node's share of rewards based on score
// Returns the reward multiplier (0.0 to 1.0 represented as uint32/1000)
// Example: 750 = 0.75 (75% of full reward)
// Math safety: uses integer arithmetic
func (s *ScoreCalculator) CalculateRewardShare(node *NodeIntegrity, thresholds ScoreThresholds) uint32 {
	if node == nil {
		return 0
	}

	if !s.QualifiesForReward(node, thresholds) {
		return 0
	}

	score := node.GetScore()

	// Calculate reward share as percentage of score
	// Score 100 = 100% reward, Score 60 = 60% reward
	// Represent as uint32 with 3 decimal places (1000 = 1.0)
	share := uint32(score) * 10

	if share > 1000 {
		return 1000
	}
	return share
}

// Global score calculator instance
var globalScoreCalculator = NewScoreCalculator()

// GetGlobalScoreCalculator returns the global score calculator instance
func GetGlobalScoreCalculator() *ScoreCalculator {
	return globalScoreCalculator
}
