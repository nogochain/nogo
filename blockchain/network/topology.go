// Copyright 2026 NogoChain Team
// Production-grade network topology analysis
// Provides insights into network health and connectivity

package network

import (
	"context"
	"math"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
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
	// NodeRoleRelay indicates a high-speed relay node
	NodeRoleRelay
)

// NodeLocation represents the geographic location of a node
type NodeLocation struct {
	Country      string
	Region       string
	City         string
	Latitude     float64
	Longitude    float64
	ASN          int
	ISP          string
}

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
	Location     *NodeLocation
	SpeedScore   float64 // 0-100, higher is better
	StabilityScore float64 // 0-100, higher is better
}

// NetworkTopology represents the network topology
type NetworkTopology struct {
	mu          sync.RWMutex
	peers       map[string]*NodePeerInfo
	connections map[string][]string // peerID -> connected peerIDs
	startTime   time.Time
	ourNodeID   string
	ourRole     NodeRole
	relayNodes  map[string]bool // Map of high-speed relay nodes
	geoClusters map[string][]string // Map of geographic clusters to peer IDs
	
	// Dynamic topology adjustment
	connectionLimit      int           // Maximum connections per node
	connectionScore      map[string]float64 // Connection quality scores
	lastAdjustmentTime   time.Time     // Last topology adjustment time
	adjustmentInterval   time.Duration // Interval for topology adjustments
	selfHealingEnabled   bool          // Whether self-healing is enabled
	
	// Node discovery optimization
	discoveryAttempts    map[string]int // Discovery attempts per peer
	discoveryCooldown    time.Duration // Cooldown period for failed discoveries
	lastDiscoveryTime    time.Time     // Last discovery time
	discoveryInterval    time.Duration // Interval for periodic discovery
}

// NewNetworkTopology creates a new network topology analyzer
func NewNetworkTopology(nodeID string, role NodeRole) *NetworkTopology {
	return &NetworkTopology{
		peers:       make(map[string]*NodePeerInfo),
		connections: make(map[string][]string),
		startTime:   time.Now(),
		ourNodeID:   nodeID,
		ourRole:     role,
		relayNodes:  make(map[string]bool),
		geoClusters: make(map[string][]string),
		
		// Dynamic topology adjustment
		connectionLimit:    16, // Default maximum connections
		connectionScore:    make(map[string]float64),
		lastAdjustmentTime: time.Now(),
		adjustmentInterval: 5 * time.Minute, // Adjust every 5 minutes
		selfHealingEnabled: true,
		
		// Node discovery optimization
		discoveryAttempts: make(map[string]int),
		discoveryCooldown: 30 * time.Minute, // 30 minute cooldown for failed discoveries
		lastDiscoveryTime: time.Now(),
		discoveryInterval: 10 * time.Minute, // Discover peers every 10 minutes
	}
}

// AddPeer adds or updates a peer in the topology
func (nt *NetworkTopology) AddPeer(info *NodePeerInfo) {
	if info == nil {
		return
	}

	nt.mu.Lock()
	defer nt.mu.Unlock()

	// Calculate speed score (based on latency and success rate)
	info.SpeedScore = calculateSpeedScore(info.LatencyMs, info.SuccessRate)
	
	// Calculate stability score (based on success rate and connection duration)
	info.StabilityScore = calculateStabilityScore(info.SuccessRate, info.ConnectedAt)

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
		existing.Location = info.Location
		existing.SpeedScore = info.SpeedScore
		existing.StabilityScore = info.StabilityScore
	} else {
		// Add new peer
		info.LastSeen = time.Now()
		nt.peers[info.ID] = info
	}

	// Update geographic clusters
	nt.updateGeoClusters(info)

	// Update relay nodes
	nt.updateRelayNodes(info)
}

// RemovePeer removes a peer from the topology
func (nt *NetworkTopology) RemovePeer(peerID string) {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	// Get peer info before deletion
	peer, exists := nt.peers[peerID]

	delete(nt.peers, peerID)
	delete(nt.connections, peerID)
	delete(nt.relayNodes, peerID)

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

	// Update geographic clusters if peer existed
	if exists && peer != nil {
		nt.updateGeoClustersAfterRemoval(peer)
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

// calculateSpeedScore calculates a speed score (0-100) based on latency and success rate
func calculateSpeedScore(latencyMs int64, successRate float64) float64 {
	// Calculate latency score (inverse relationship)
	latencyScore := 100.0
	if latencyMs > 0 {
		latencyScore = math.Max(0, 100 - (float64(latencyMs) / 10))
	}

	// Calculate success rate score
	successScore := successRate * 100

	// Combine scores with weights
	return (latencyScore * 0.6) + (successScore * 0.4)
}

// calculateStabilityScore calculates a stability score (0-100) based on success rate and connection duration
func calculateStabilityScore(successRate float64, connectedAt time.Time) float64 {
	// Calculate success rate score
	successScore := successRate * 100

	// Calculate connection duration score (up to 30 days)
	duration := time.Since(connectedAt)
	durationDays := duration.Hours() / 24
	durationScore := math.Min(100, durationDays * 3.33) // 30 days = 100 points

	// Combine scores with weights
	return (successScore * 0.7) + (durationScore * 0.3)
}

// updateGeoClusters updates geographic clusters based on node location
func (nt *NetworkTopology) updateGeoClusters(info *NodePeerInfo) {
	if info.Location == nil || info.Location.Country == "" {
		return
	}

	// Use country + region as cluster key
	clusterKey := info.Location.Country
	if info.Location.Region != "" {
		clusterKey += ":" + info.Location.Region
	}

	// Add peer to cluster if not already present
	if !contains(nt.geoClusters[clusterKey], info.ID) {
		nt.geoClusters[clusterKey] = append(nt.geoClusters[clusterKey], info.ID)
	}
}

// updateGeoClustersAfterRemoval updates geographic clusters after a peer is removed
func (nt *NetworkTopology) updateGeoClustersAfterRemoval(peer *NodePeerInfo) {
	if peer.Location == nil || peer.Location.Country == "" {
		return
	}

	// Use country + region as cluster key
	clusterKey := peer.Location.Country
	if peer.Location.Region != "" {
		clusterKey += ":" + peer.Location.Region
	}

	// Remove peer from cluster
	nt.geoClusters[clusterKey] = removeFromSlice(nt.geoClusters[clusterKey], peer.ID)

	// Remove empty clusters
	if len(nt.geoClusters[clusterKey]) == 0 {
		delete(nt.geoClusters, clusterKey)
	}
}

// updateRelayNodes updates the list of high-speed relay nodes
func (nt *NetworkTopology) updateRelayNodes(info *NodePeerInfo) {
	// Consider a node as relay if it has high speed and stability scores
	isRelay := info.SpeedScore >= 80 && info.StabilityScore >= 70

	if isRelay {
		nt.relayNodes[info.ID] = true
		// Update node role to relay
		info.Role = NodeRoleRelay
	} else {
		delete(nt.relayNodes, info.ID)
	}
}

// GetGeoClusters returns the geographic clusters
func (nt *NetworkTopology) GetGeoClusters() map[string][]string {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	clusters := make(map[string][]string)
	for k, v := range nt.geoClusters {
		clusters[k] = make([]string, len(v))
		copy(clusters[k], v)
	}
	return clusters
}

// GetRelayNodes returns the list of high-speed relay nodes
func (nt *NetworkTopology) GetRelayNodes() []string {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	relayNodes := make([]string, 0, len(nt.relayNodes))
	for nodeID := range nt.relayNodes {
		relayNodes = append(relayNodes, nodeID)
	}
	return relayNodes
}

// GetOptimalRelayPath finds the optimal relay path between two nodes
func (nt *NetworkTopology) GetOptimalRelayPath(sourceID, targetID string) []string {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	// If direct connection exists, return direct path
	if contains(nt.connections[sourceID], targetID) {
		return []string{sourceID, targetID}
	}

	// Find relay nodes with highest speed scores
	relayCandidates := make([]*NodePeerInfo, 0)
	for nodeID := range nt.relayNodes {
		if nodeID != sourceID && nodeID != targetID {
			if peer, exists := nt.peers[nodeID]; exists {
				relayCandidates = append(relayCandidates, peer)
			}
		}
	}

	// Sort relay candidates by speed score
	sort.Slice(relayCandidates, func(i, j int) bool {
		return relayCandidates[i].SpeedScore > relayCandidates[j].SpeedScore
	})

	// Find first relay node that connects to both source and target
	for _, relay := range relayCandidates {
		if contains(nt.connections[sourceID], relay.ID) && contains(nt.connections[relay.ID], targetID) {
			return []string{sourceID, relay.ID, targetID}
		}
	}

	// No optimal path found
	return nil
}

// UpdateNodeRoles dynamically updates node roles based on performance and network conditions
func (nt *NetworkTopology) UpdateNodeRoles() {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	for nodeID, peer := range nt.peers {
		// Re-evaluate node role based on current performance
		if peer.SpeedScore >= 80 && peer.StabilityScore >= 70 {
			// High-performance node, designate as relay
			peer.Role = NodeRoleRelay
			nt.relayNodes[nodeID] = true
		} else if peer.SuccessRate >= 0.9 && peer.Height > 0 {
			// Stable node with good blockchain data, designate as full node
			peer.Role = NodeRoleFullNode
			delete(nt.relayNodes, nodeID)
		} else {
			// Default to light node if not meeting other criteria
			peer.Role = NodeRoleLightNode
			delete(nt.relayNodes, nodeID)
		}
	}
}

// GetClusterStats returns statistics about geographic clusters
func (nt *NetworkTopology) GetClusterStats() map[string]interface{} {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	stats := map[string]interface{}{
		"total_clusters": len(nt.geoClusters),
		"clusters":       make(map[string]int),
	}

	clusters := stats["clusters"].(map[string]int)
	for cluster, peers := range nt.geoClusters {
		clusters[cluster] = len(peers)
	}

	return stats
}

// GetRelayNodeStats returns statistics about relay nodes
func (nt *NetworkTopology) GetRelayNodeStats() map[string]interface{} {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	var totalSpeedScore, totalStabilityScore float64
	relayCount := len(nt.relayNodes)

	for nodeID := range nt.relayNodes {
		if peer, exists := nt.peers[nodeID]; exists {
			totalSpeedScore += peer.SpeedScore
			totalStabilityScore += peer.StabilityScore
		}
	}

	stats := map[string]interface{}{
		"total_relay_nodes": relayCount,
		"avg_speed_score":   0.0,
		"avg_stability_score": 0.0,
	}

	if relayCount > 0 {
		stats["avg_speed_score"] = totalSpeedScore / float64(relayCount)
		stats["avg_stability_score"] = totalStabilityScore / float64(relayCount)
	}

	return stats
}

// EvaluateConnectionQuality evaluates the quality of a connection to a peer
func (nt *NetworkTopology) EvaluateConnectionQuality(peerID string) float64 {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	peer, exists := nt.peers[peerID]
	if !exists {
		return 0
	}

	// Calculate connection quality score (0-100)
	score := 0.0
	
	// Speed score (40% weight)
	score += peer.SpeedScore * 0.4
	
	// Stability score (40% weight)
	score += peer.StabilityScore * 0.4
	
	// Success rate (20% weight)
	score += peer.SuccessRate * 100 * 0.2
	
	return score
}

// UpdateConnectionScores updates connection quality scores for all peers
func (nt *NetworkTopology) UpdateConnectionScores() {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	for peerID := range nt.peers {
		score := nt.EvaluateConnectionQuality(peerID)
		nt.connectionScore[peerID] = score
	}
}

// AdjustTopology dynamically adjusts the network topology based on network state
func (nt *NetworkTopology) AdjustTopology() map[string]interface{} {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	// Check if adjustment is needed based on interval
	if time.Since(nt.lastAdjustmentTime) < nt.adjustmentInterval {
		return map[string]interface{}{
			"status": "skipped",
			"reason": "adjustment interval not reached",
		}
	}

	// Update connection scores
	nt.UpdateConnectionScores()

	// Sort peers by connection quality
	sortedPeers := make([]*NodePeerInfo, 0, len(nt.peers))
	for _, peer := range nt.peers {
		sortedPeers = append(sortedPeers, peer)
	}

	sort.Slice(sortedPeers, func(i, j int) bool {
		scoreI := nt.connectionScore[sortedPeers[i].ID]
		scoreJ := nt.connectionScore[sortedPeers[j].ID]
		return scoreI > scoreJ
	})

	// Determine which connections to keep
	connectionsToKeep := make(map[string]bool)
	newConnections := make([]string, 0)

	// Keep high-quality connections up to limit
	for i, peer := range sortedPeers {
		if i >= nt.connectionLimit {
			break
		}
		connectionsToKeep[peer.ID] = true
		newConnections = append(newConnections, peer.ID)
	}

	// Remove low-quality connections
	removedConnections := 0
	for _, conn := range nt.connections[nt.ourNodeID] {
		if !connectionsToKeep[conn] {
			nt.RemoveConnection(nt.ourNodeID, conn)
			removedConnections++
		}
	}

	// Update our connections
	nt.connections[nt.ourNodeID] = newConnections

	nt.lastAdjustmentTime = time.Now()

	return map[string]interface{}{
		"status":              "completed",
		"current_connections": len(newConnections),
		"removed_connections": removedConnections,
		"connection_limit":    nt.connectionLimit,
		"timestamp":           nt.lastAdjustmentTime,
	}
}

// OptimizeNodeDiscovery optimizes node discovery based on network conditions
func (nt *NetworkTopology) OptimizeNodeDiscovery() map[string]interface{} {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	// Check if discovery is needed based on interval
	if time.Since(nt.lastDiscoveryTime) < nt.discoveryInterval {
		return map[string]interface{}{
			"status": "skipped",
			"reason": "discovery interval not reached",
		}
	}

	// Identify potential discovery candidates
	discoveryCandidates := make([]*NodePeerInfo, 0)

	for _, peer := range nt.peers {
		// Skip peers with too many failed attempts
		if nt.discoveryAttempts[peer.ID] >= 3 {
			continue
		}

		// Prioritize high-quality peers for discovery
		if peer.SpeedScore >= 70 && peer.StabilityScore >= 60 {
			discoveryCandidates = append(discoveryCandidates, peer)
		}
	}

	// Sort candidates by quality
	sort.Slice(discoveryCandidates, func(i, j int) bool {
		scoreI := nt.connectionScore[discoveryCandidates[i].ID]
		scoreJ := nt.connectionScore[discoveryCandidates[j].ID]
		return scoreI > scoreJ
	})

	// Limit to top 5 candidates
	if len(discoveryCandidates) > 5 {
		discoveryCandidates = discoveryCandidates[:5]
	}

	nt.lastDiscoveryTime = time.Now()

	return map[string]interface{}{
		"status":              "completed",
		"discovery_candidates": len(discoveryCandidates),
		"timestamp":           nt.lastDiscoveryTime,
	}
}

// SelfHealTopology provides topology self-healing mechanisms
func (nt *NetworkTopology) SelfHealTopology() map[string]interface{} {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	if !nt.selfHealingEnabled {
		return map[string]interface{}{
			"status": "disabled",
			"reason": "self-healing is disabled",
		}
	}

	// Check for network partitions
	connectivity := nt.AnalyzeConnectivity()
	partitionRisk := connectivity["partition_risk"].(string)

	actions := make([]string, 0)

	// If high partition risk, take action
	if partitionRisk == "CRITICAL" || partitionRisk == "HIGH" {
		// Identify missing geographic diversity
		clusters := nt.GetGeoClusters()
		if len(clusters) < 2 {
			actions = append(actions, "need_geographic_diversity")
		}

		// Check for insufficient connections
		currentConnections := len(nt.connections[nt.ourNodeID])
		if currentConnections < 4 {
			actions = append(actions, "need_more_connections")
		}

		// Check for low-quality connections
		lowQualityCount := 0
		for _, peer := range nt.peers {
			if peer.SpeedScore < 50 || peer.StabilityScore < 50 {
				lowQualityCount++
			}
		}

		if lowQualityCount > len(nt.peers)/2 {
			actions = append(actions, "replace_low_quality_connections")
		}
	}

	return map[string]interface{}{
		"status":          "completed",
		"partition_risk":  partitionRisk,
		"recommended_actions": actions,
		"timestamp":       time.Now(),
	}
}

// StartTopologyManager starts the topology management loop
func (nt *NetworkTopology) StartTopologyManager(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Adjust topology
				nt.AdjustTopology()

				// Optimize node discovery
				nt.OptimizeNodeDiscovery()

				// Self-heal topology
				nt.SelfHealTopology()
			}
		}
	}()
}

// SetConnectionLimit sets the maximum number of connections
func (nt *NetworkTopology) SetConnectionLimit(limit int) {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	if limit > 0 && limit <= 100 {
		nt.connectionLimit = limit
	}
}

// GetConnectionLimit returns the current connection limit
func (nt *NetworkTopology) GetConnectionLimit() int {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	return nt.connectionLimit
}

// EnableSelfHealing enables or disables self-healing
func (nt *NetworkTopology) EnableSelfHealing(enabled bool) {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	nt.selfHealingEnabled = enabled
}

// GetConnectionScores returns connection quality scores
func (nt *NetworkTopology) GetConnectionScores() map[string]float64 {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	scores := make(map[string]float64)
	for k, v := range nt.connectionScore {
		scores[k] = v
	}
	return scores
}

// GetTopologyStats returns current topology statistics
// Production-grade: implements core.TopologyProvider interface for cross-package use
func (nt *NetworkTopology) GetTopologyStats() core.TopologyStats {
	nt.mu.RLock()
	defer nt.mu.RUnlock()

	stats := core.TopologyStats{
		TotalNodes:  len(nt.peers),
		RelayNodes:  len(nt.relayNodes),
		Partitions:  len(nt.geoClusters),
	}

	// Calculate average latency
	totalLatency := 0.0
	validCount := 0
	for _, peer := range nt.peers {
		if peer.LatencyMs > 0 {
			totalLatency += float64(peer.LatencyMs)
			validCount++
		}
	}
	if validCount > 0 {
		stats.AvgLatency = totalLatency / float64(validCount)
	}

	// Calculate network score based on connectivity
	if stats.TotalNodes > 0 {
		relayRatio := float64(stats.RelayNodes) / float64(stats.TotalNodes)
		stats.NetworkScore = relayRatio*0.5 + (1.0/float64(stats.Partitions+1))*0.5
	}

	return stats
}
