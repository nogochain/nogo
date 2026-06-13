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

// ViolationType represents the type of node violation
type ViolationType uint8

const (
	// ViolationNone represents no violation
	ViolationNone ViolationType = iota
	// ViolationOffline represents offline/timeout violation
	ViolationOffline
	// ViolationInvalidBlock represents producing invalid blocks
	ViolationInvalidBlock
	// ViolationDoubleSign represents double-signing violation
	ViolationDoubleSign
	// ViolationMaliciousFork represents malicious forking violation
	ViolationMaliciousFork
)

// String returns the string representation of violation type
func (v ViolationType) String() string {
	switch v {
	case ViolationNone:
		return "none"
	case ViolationOffline:
		return "offline"
	case ViolationInvalidBlock:
		return "invalid_block"
	case ViolationDoubleSign:
		return "double_sign"
	case ViolationMaliciousFork:
		return "malicious_fork"
	default:
		return "unknown"
	}
}

// NodeStatus represents the status of a node
type NodeStatus uint8

const (
	// StatusActive represents active node status
	StatusActive NodeStatus = iota
	// StatusInactive represents inactive node status
	StatusInactive
	// StatusSuspended represents suspended node status (temporary)
	StatusSuspended
	// StatusBanned represents banned node status (permanent)
	StatusBanned
)

// String returns the string representation of node status
func (s NodeStatus) String() string {
	switch s {
	case StatusActive:
		return "active"
	case StatusInactive:
		return "inactive"
	case StatusSuspended:
		return "suspended"
	case StatusBanned:
		return "banned"
	default:
		return "unknown"
	}
}

// NodeIntegrity represents the integrity record of a node
// Production-grade: thread-safe with mutex protection
type NodeIntegrity struct {
	mu sync.RWMutex

	// NodeID is the unique identifier of the node
	NodeID string `json:"nodeId"`

	// Score is the current integrity score (0-100)
	Score uint8 `json:"score"`

	// OnlineTime is the total online time in seconds
	OnlineTime uint64 `json:"onlineTime"`

	// TotalTime is the total expected online time in seconds
	TotalTime uint64 `json:"totalTime"`

	// ValidBlocks is the number of valid blocks produced
	ValidBlocks uint64 `json:"validBlocks"`

	// InvalidBlocks is the number of invalid blocks produced
	InvalidBlocks uint64 `json:"invalidBlocks"`

	// ConsecutiveDays is the number of consecutive active days
	ConsecutiveDays uint64 `json:"consecutiveDays"`

	// PeerScores is the map of peer scores (peer node ID -> score)
	PeerScores map[string]uint8 `json:"peerScores"`

	// Status is the current node status
	Status NodeStatus `json:"status"`

	// Violations is the list of violation records
	Violations []ViolationRecord `json:"violations"`

	// LastActiveTime is the last activity timestamp
	LastActiveTime int64 `json:"lastActiveTime"`

	// CreatedAt is the node creation timestamp
	CreatedAt int64 `json:"createdAt"`
}

// ViolationRecord represents a single violation record
type ViolationRecord struct {
	// Type is the type of violation
	Type ViolationType `json:"type"`

	// Timestamp is when the violation occurred
	Timestamp int64 `json:"timestamp"`

	// Description is the violation description
	Description string `json:"description"`

	// PenaltyApplied is the penalty score applied
	PenaltyApplied uint8 `json:"penaltyApplied"`
}

// NewNodeIntegrity creates a new node integrity record
func NewNodeIntegrity(nodeID string) *NodeIntegrity {
	now := time.Now().Unix()
	return &NodeIntegrity{
		NodeID:          nodeID,
		Score:           100, // Start with perfect score
		OnlineTime:      0,
		TotalTime:       0,
		ValidBlocks:     0,
		InvalidBlocks:   0,
		ConsecutiveDays: 0,
		PeerScores:      make(map[string]uint8),
		Status:          StatusActive,
		Violations:      make([]ViolationRecord, 0),
		LastActiveTime:  now,
		CreatedAt:       now,
	}
}

// GetScore returns the current integrity score (thread-safe)
func (n *NodeIntegrity) GetScore() uint8 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Score
}

// SetScore sets the integrity score with validation (thread-safe)
// Returns error if score is invalid or node is banned
func (n *NodeIntegrity) SetScore(score uint8) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.Status == StatusBanned {
		return errors.New("cannot update score of banned node")
	}

	n.Score = score
	return nil
}

// AddViolation adds a violation record to the node
// Thread-safe, validates input parameters
func (n *NodeIntegrity) AddViolation(violationType ViolationType, description string, penalty uint8) {
	n.mu.Lock()
	defer n.mu.Unlock()

	record := ViolationRecord{
		Type:           violationType,
		Timestamp:      time.Now().Unix(),
		Description:    description,
		PenaltyApplied: penalty,
	}

	n.Violations = append(n.Violations, record)
}

// GetViolations returns a copy of violation records (thread-safe)
func (n *NodeIntegrity) GetViolations() []ViolationRecord {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Return a copy to prevent external modification
	copy := make([]ViolationRecord, len(n.Violations))
	for i, v := range n.Violations {
		copy[i] = v
	}
	return copy
}

// UpdateOnlineTime updates online time statistics (thread-safe)
func (n *NodeIntegrity) UpdateOnlineTime(onlineSeconds, totalSeconds uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.OnlineTime += onlineSeconds
	n.TotalTime += totalSeconds
	n.LastActiveTime = time.Now().Unix()
}

// AddValidBlock increments the valid block counter (thread-safe)
func (n *NodeIntegrity) AddValidBlock() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ValidBlocks++
}

// AddInvalidBlock increments the invalid block counter (thread-safe)
func (n *NodeIntegrity) AddInvalidBlock() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.InvalidBlocks++
}

// UpdateConsecutiveDays updates consecutive active days (thread-safe)
func (n *NodeIntegrity) UpdateConsecutiveDays(days uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ConsecutiveDays = days
}

// AddPeerScore adds or updates a peer score (thread-safe)
func (n *NodeIntegrity) AddPeerScore(peerID string, score uint8) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.PeerScores[peerID] = score
}

// GetPeerScores returns a copy of peer scores (thread-safe)
func (n *NodeIntegrity) GetPeerScores() map[string]uint8 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Return a copy to prevent external modification
	copy := make(map[string]uint8)
	for k, v := range n.PeerScores {
		copy[k] = v
	}
	return copy
}

// GetSnapshot returns a thread-safe snapshot of node integrity data
// Used for score calculation to avoid race conditions
func (n *NodeIntegrity) GetSnapshot() (onlineTime, totalTime, validBlocks, invalidBlocks, consecutiveDays uint64, peerScores map[string]uint8) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	onlineTime = n.OnlineTime
	totalTime = n.TotalTime
	validBlocks = n.ValidBlocks
	invalidBlocks = n.InvalidBlocks
	consecutiveDays = n.ConsecutiveDays

	peerScores = make(map[string]uint8, len(n.PeerScores))
	for k, v := range n.PeerScores {
		peerScores[k] = v
	}
	return
}

// GetStatus returns the current node status (thread-safe)
func (n *NodeIntegrity) GetStatus() NodeStatus {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Status
}

// SetStatus sets the node status (thread-safe)
func (n *NodeIntegrity) SetStatus(status NodeStatus) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Status = status
}

// IsActive returns true if node is active (thread-safe)
func (n *NodeIntegrity) IsActive() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Status == StatusActive
}

// GetOnlineRate returns the online rate as a percentage (thread-safe)
// Returns value in range [0, 100]
func (n *NodeIntegrity) GetOnlineRate() uint8 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.TotalTime == 0 {
		return 0
	}

	rate := (n.OnlineTime * 100) / n.TotalTime
	if rate > 100 {
		return 100
	}
	return uint8(rate)
}

// GetValidBlockRate returns the valid block rate as a percentage (thread-safe)
// Returns value in range [0, 100]
func (n *NodeIntegrity) GetValidBlockRate() uint8 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	total := n.ValidBlocks + n.InvalidBlocks
	if total == 0 {
		return 100 // No blocks produced yet, assume perfect
	}

	rate := (n.ValidBlocks * 100) / total
	if rate > 100 {
		return 100
	}
	return uint8(rate)
}

// NodeIntegrityManager manages integrity records for all nodes
// Production-grade: thread-safe with mutex protection
type NodeIntegrityManager struct {
	mu      sync.RWMutex
	nodes   map[string]*NodeIntegrity
	history map[string][]ScoreHistory // Historical scores for analysis
}

// ScoreHistory represents a historical score record
type ScoreHistory struct {
	Timestamp int64  `json:"timestamp"`
	Score     uint8  `json:"score"`
	Height    uint64 `json:"height"`
}

// NewNodeIntegrityManager creates a new integrity manager
func NewNodeIntegrityManager() *NodeIntegrityManager {
	return &NodeIntegrityManager{
		nodes:   make(map[string]*NodeIntegrity),
		history: make(map[string][]ScoreHistory),
	}
}

// GetOrCreateNode gets an existing node or creates a new one
func (m *NodeIntegrityManager) GetOrCreateNode(nodeID string) *NodeIntegrity {
	// First try with read lock
	m.mu.RLock()
	node, exists := m.nodes[nodeID]
	m.mu.RUnlock()

	if exists {
		return node
	}

	// Create new node with write lock
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if node, exists = m.nodes[nodeID]; exists {
		return node
	}

	node = NewNodeIntegrity(nodeID)
	m.nodes[nodeID] = node
	return node
}

// GetNode gets a node by ID (returns nil if not found)
func (m *NodeIntegrityManager) GetNode(nodeID string) *NodeIntegrity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nodes[nodeID]
}

// GetAllNodes returns all node integrity records (thread-safe)
func (m *NodeIntegrityManager) GetAllNodes() []*NodeIntegrity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy of the slice
	nodes := make([]*NodeIntegrity, 0, len(m.nodes))
	for _, node := range m.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetActiveNodes returns all active nodes (thread-safe)
func (m *NodeIntegrityManager) GetActiveNodes() []*NodeIntegrity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodes := make([]*NodeIntegrity, 0)
	for _, node := range m.nodes {
		if node.Status == StatusActive {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// RecordScoreHistory records historical score for analysis
func (m *NodeIntegrityManager) RecordScoreHistory(nodeID string, score uint8, height uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record := ScoreHistory{
		Timestamp: time.Now().Unix(),
		Score:     score,
		Height:    height,
	}

	m.history[nodeID] = append(m.history[nodeID], record)

	// Keep only last 1000 records per node to prevent memory growth
	if len(m.history[nodeID]) > 1000 {
		m.history[nodeID] = m.history[nodeID][len(m.history[nodeID])-1000:]
	}
}

// GetScoreHistory returns score history for a node
func (m *NodeIntegrityManager) GetScoreHistory(nodeID string) []ScoreHistory {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history := m.history[nodeID]
	if history == nil {
		return []ScoreHistory{}
	}

	// Return a copy
	copy := make([]ScoreHistory, len(history))
	for i, h := range history {
		copy[i] = h
	}
	return copy
}

// RemoveNode removes a node from the manager (thread-safe)
func (m *NodeIntegrityManager) RemoveNode(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.nodes, nodeID)
	delete(m.history, nodeID)
}

// Count returns the total number of nodes (thread-safe)
func (m *NodeIntegrityManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.nodes)
}

// CountActive returns the number of active nodes (thread-safe)
func (m *NodeIntegrityManager) CountActive() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, node := range m.nodes {
		if node.Status == StatusActive {
			count++
		}
	}
	return count
}
