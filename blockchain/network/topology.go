// Copyright 2026 NogoChain Team
// Production-grade network topology analysis
// Provides insights into network health and connectivity

package network

import (
	"net"
	"sync"
	"time"
)

// NodeRole represents the role of a node in the network
type NodeRole int

const (
	// NodeRoleUnknown indicates unknown node role
	NodeRoleUnknown NodeRole = iota
	// NodeRoleMiner indicates a mining node
	NodeRoleMiner
	// NodeRoleFullNode indicates a full node
	NodeRoleFullNode
	// NodeRoleLightNode indicates a light node
	NodeRoleLightNode
)

// NodePeerInfo represents comprehensive information about a peer in the network topology
type NodePeerInfo struct {
	ID           string
	Address      string
	ConnectedAt  time.Time
	LastSeen     time.Time
	Height       uint64
	ChainWork    string
	Role         NodeRole
	LatencyMs    int64
	Failures     int
	SuccessRate  float64
	IsInbound    bool
}

// NetworkTopology represents the network topology
type NetworkTopology struct {
	mu          sync.RWMutex
	peers       map[string]*NodePeerInfo
	connections map[string][]string // peerID -> connected peerIDs
	startTime   time.Time
	ourNodeID   string
	ourRole     NodeRole
}

// NewNetworkTopology creates a new network topology analyzer
func NewNetworkTopology(nodeID string, role NodeRole) *NetworkTopology {
	return &NetworkTopology{
		peers:       make(map[string]*NodePeerInfo),
		connections: make(map[string][]string),
		startTime:   time.Now(),
		ourNodeID:   nodeID,
		ourRole:     role,
	}
}

// AddPeer adds or updates a peer in the topology
func (nt *NetworkTopology) AddPeer(info *NodePeerInfo) {
	if info == nil {
		return
	}

	nt.mu.Lock()
	defer nt.mu.Unlock()

	existing, exists := nt.peers[info.ID]
	if exists {
		// Update existing peer
		existing.LastSeen = time.Now()
		if info.Height > existing.Height {
			existing.Height = info.Height
		}
		if info.ChainWork != "" {
			existing.ChainWork = info.ChainWork
		}
		existing.LatencyMs = info.LatencyMs
		existing.Failures = info.Failures
		existing.SuccessRate = info.SuccessRate
		existing.Role = info.Role
	} else {
		// Add new peer
		info.LastSeen = time.Now()
		nt.peers[info.ID] = info
	}
}

// RemovePeer removes a peer from the topology
func (nt *NetworkTopology) RemovePeer(peerID string) {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	delete(nt.peers, peerID)
	delete(nt.connections, peerID)

	// Remove from other peers' connection lists
	for id, conns := range nt.connections {
		newConns := make([]string, 0)
		for _, conn := range conns {
			if conn != peerID {
				newConns = append(newConns, conn)
			}
		}
		nt.connections[id] = newConns
	}
}

// AddConnection records a connection between two peers
func (nt *NetworkTopology) AddConnection(peerID1, peerID2 string) {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	// Add to peer1's connections
	if !contains(nt.connections[peerID1], peerID2) {
		nt.connections[peerID1] = append(nt.connections[peerID1], peerID2)
	}

	// Add to peer2's connections (bidirectional)
	if !contains(nt.connections[peerID2], peerID1) {
		nt.connections[peerID2] = append(nt.connections[peerID2], peerID1)
	}
}

// RemoveConnection removes a connection between two peers
func (nt *NetworkTopology) RemoveConnection(peerID1, peerID2 string) {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	// Remove from peer1's connections
	nt.connections[peerID1] = removeFromSlice(nt.connections[peerID1], peerID2)

	// Remove from peer2's connections
	nt.connections[peerID2] = removeFromSlice(nt.connections[peerID2], peerID1)
}

// AnalyzeConnectivity analyzes network connectivity
func (nt *NetworkTopology) AnalyzeConnectivity() map[string]interface{} {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	result := map[string]interface{}{
		"total_peers":           len(nt.peers),
		"miner_nodes":           0,
		"full_nodes":            0,
		"light_nodes":           0,
		"avg_latency_ms":        0.0,
		"avg_success_rate":      0.0,
		"network_health_score":  0.0,
		"partition_risk":        "LOW",
		"recommendation":        "",
	}

	if len(nt.peers) == 0 {
		result["recommendation"] = "Connect to at least 3-4 peers to ensure network stability"
		result["partition_risk"] = "CRITICAL"
		return result
	}

	var totalLatency int64
	var totalSuccessRate float64
	minerNodes := 0
	fullNodes := 0
	lightNodes := 0

	for _, peer := range nt.peers {
		switch peer.Role {
		case NodeRoleMiner:
			minerNodes++
		case NodeRoleFullNode:
			fullNodes++
		case NodeRoleLightNode:
			lightNodes++
		}

		totalLatency += peer.LatencyMs
		totalSuccessRate += peer.SuccessRate
	}

	result["miner_nodes"] = minerNodes
	result["full_nodes"] = fullNodes
	result["light_nodes"] = lightNodes
	result["avg_latency_ms"] = float64(totalLatency) / float64(len(nt.peers))
	result["avg_success_rate"] = totalSuccessRate / float64(len(nt.peers))

	// Calculate network health score (0-100)
	healthScore := 50.0 // Base score

	// Bonus for peer count
	if len(nt.peers) >= 8 {
		healthScore += 20
	} else if len(nt.peers) >= 4 {
		healthScore += 10
	} else if len(nt.peers) >= 2 {
		healthScore += 5
	}

	// Bonus for multiple miners
	if minerNodes >= 3 {
		healthScore += 15
	} else if minerNodes >= 2 {
		healthScore += 10
	}

	// Penalty for high latency
	if result["avg_latency_ms"].(float64) > 500 {
		healthScore -= 10
	}

	// Penalty for low success rate
	if result["avg_success_rate"].(float64) < 0.8 {
		healthScore -= 15
	}

	result["network_health_score"] = healthScore

	// Partition risk assessment
	if len(nt.peers) < 2 {
		result["partition_risk"] = "CRITICAL"
		result["recommendation"] = "IMMEDIATE ACTION REQUIRED: Connect to at least 3-4 peers to prevent network partitioning"
	} else if len(nt.peers) < 4 {
		result["partition_risk"] = "HIGH"
		result["recommendation"] = "Connect to more peers (target: 8+) to improve network resilience"
	} else if len(nt.peers) < 8 {
		result["partition_risk"] = "MEDIUM"
		result["recommendation"] = "Consider adding more peers for better decentralization"
	} else if minerNodes < 2 {
		result["partition_risk"] = "MEDIUM"
		result["recommendation"] = "Add more mining nodes to prevent single-miner dominance"
	} else if healthScore >= 80 {
		result["partition_risk"] = "LOW"
		result["recommendation"] = "Network topology is healthy"
	} else {
		result["partition_risk"] = "MEDIUM"
		result["recommendation"] = "Monitor network performance and consider adding more peers"
	}

	return result
}

// GetPeerInfo returns information about a specific peer
func (nt *NetworkTopology) GetPeerInfo(peerID string) (*NodePeerInfo, bool) {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	peer, exists := nt.peers[peerID]
	return peer, exists
}

func (nt *NetworkTopology) GetAllPeers() map[string]*NodePeerInfo {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	peers := make(map[string]*NodePeerInfo)
	for k, v := range nt.peers {
		peers[k] = v
	}
	return peers
}



// AnalyzeNetworkLatency analyzes network latency distribution
func (nt *NetworkTopology) AnalyzeNetworkLatency() map[string]interface{} {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	if len(nt.peers) == 0 {
		return map[string]interface{}{
			"sample_size": 0,
			"message":     "No peers to analyze",
		}
	}

	latencies := make([]int64, 0, len(nt.peers))
	for _, peer := range nt.peers {
		if peer.LatencyMs > 0 {
			latencies = append(latencies, peer.LatencyMs)
		}
	}

	if len(latencies) == 0 {
		return map[string]interface{}{
			"sample_size": 0,
			"message":     "No latency data available",
		}
	}

	// Calculate statistics
	var sum int64
	min := latencies[0]
	max := latencies[0]

	for _, lat := range latencies {
		sum += lat
		if lat < min {
			min = lat
		}
		if lat > max {
			max = lat
		}
	}

	avg := float64(sum) / float64(len(latencies))

	return map[string]interface{}{
		"sample_size":    len(latencies),
		"min_latency_ms": min,
		"max_latency_ms": max,
		"avg_latency_ms": avg,
		"p50_estimate":   avg, // Simplified, could implement full percentile
		"p95_estimate":   max, // Conservative estimate
	}
}

// GetConnectionGraph returns the network connection graph
func (nt *NetworkTopology) GetConnectionGraph() map[string][]string {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	graph := make(map[string][]string)
	for peerID, conns := range nt.connections {
		graph[peerID] = make([]string, len(conns))
		copy(graph[peerID], conns)
	}
	return graph
}

// Helper functions
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func removeFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// IsPublicIP checks if an IP address is public (not private)
func IsPublicIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// Check for private IP ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
	}

	for _, cidr := range privateRanges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if ipNet.Contains(parsedIP) {
			return false
		}
	}

	return true
}
