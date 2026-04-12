// Copyright 2026 NogoChain Team
// Production-grade Gossip Protocol fanout control and metrics
// Implements intelligent peer selection and comprehensive monitoring

package network

import (
	"sync"
	"sync/atomic"
	"time"
)

// GossipFanoutControl implements intelligent fanout control
// Design: adaptive fanout based on network topology and load
// Security: prevents broadcast storms, respects rate limits
type GossipFanoutControl struct {
	mu sync.RWMutex

	config GossipConfig

	// Network state
	peerScores      map[string]*GossipPeerScore
	networkSize     int
	networkDensity  float64

	// Fanout history
	fanoutHistory   []int
	successRates    []float64

	// Adaptive parameters
	currentFanout   int
	optimalFanout   int
}

// GossipPeerScore represents a peer's quality score for gossip selection
type GossipPeerScore struct {
	PeerID           string
	ConnectionScore  float64 // Network connection quality (0-1)
	PropagationScore float64 // Message propagation success rate (0-1)
	TrustScore       float64 // Trust level (0-1)
	LatencyScore     float64 // Latency quality (0-1, higher is better)
	Availability     float64 // Availability ratio (0-1)
	LastUpdate       time.Time

	// Historical data
	MessagesSent     uint64
	MessagesSuccess  uint64
	MessagesFailed   uint64
	AvgLatency       time.Duration
}

// NewGossipFanoutControl creates a new fanout controller
func NewGossipFanoutControl(config GossipConfig) *GossipFanoutControl {
	return &GossipFanoutControl{
		config:        config,
		peerScores:    make(map[string]*GossipPeerScore),
		fanoutHistory: make([]int, 0, 100),
		successRates:  make([]float64, 0, 100),
		currentFanout: config.DefaultFanout,
		optimalFanout: config.DefaultFanout,
	}
}

// UpdatePeerScore updates a peer's quality score
func (fc *GossipFanoutControl) UpdatePeerScore(peerID string, success bool, latency time.Duration) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	score, exists := fc.peerScores[peerID]
	if !exists {
		score = &GossipPeerScore{
			PeerID:           peerID,
			ConnectionScore:  0.5,
			PropagationScore: 0.5,
			TrustScore:       0.5,
			LatencyScore:     0.5,
			Availability:     0.5,
			LastUpdate:       time.Now(),
		}
		fc.peerScores[peerID] = score
	}

	// Update statistics
	score.MessagesSent++
	if success {
		score.MessagesSuccess++
	} else {
		score.MessagesFailed++
	}

	// Update average latency (exponential moving average)
	if score.AvgLatency == 0 {
		score.AvgLatency = latency
	} else {
		// EMA with alpha=0.1
		alpha := 0.1
		score.AvgLatency = time.Duration(float64(score.AvgLatency)*(1-alpha) + float64(latency)*alpha)
	}

	// Recalculate scores
	score.PropagationScore = float64(score.MessagesSuccess) / float64(score.MessagesSent)
	score.LatencyScore = calculateLatencyScore(score.AvgLatency)
	score.LastUpdate = time.Now()

	// Update composite score
	fc.updateNetworkMetrics()
}

// calculateLatencyScore converts latency to a quality score
func calculateLatencyScore(latency time.Duration) float64 {
	// Score based on latency thresholds
	// < 50ms = 1.0 (excellent)
	// < 100ms = 0.8 (good)
	// < 200ms = 0.6 (fair)
	// < 500ms = 0.4 (poor)
	// >= 500ms = 0.2 (bad)

	switch {
	case latency < 50*time.Millisecond:
		return 1.0
	case latency < 100*time.Millisecond:
		return 0.8
	case latency < 200*time.Millisecond:
		return 0.6
	case latency < 500*time.Millisecond:
		return 0.4
	default:
		return 0.2
	}
}

// SelectBestPeers selects the best peers for message propagation
func (fc *GossipFanoutControl) SelectBestPeers(peers []string, fanout int) []string {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if len(peers) <= fanout {
		return peers
	}

	// Score and rank peers
	type peerRank struct {
		peerID string
		score  float64
	}

	ranked := make([]peerRank, 0, len(peers))
	for _, peerID := range peers {
		score, exists := fc.peerScores[peerID]
		overallScore := 0.5 // Default score

		if exists {
			// Weighted composite score
			// Prioritize: propagation success > latency > trust > availability
			overallScore = score.PropagationScore*0.4 +
				score.LatencyScore*0.3 +
				score.TrustScore*0.2 +
				score.Availability*0.1
		}

		ranked = append(ranked, peerRank{peerID: peerID, score: overallScore})
	}

	// Sort by score (descending) - using simple insertion sort for small lists
	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && ranked[j].score > ranked[j-1].score; j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}

	// Select top fanout peers
	selected := make([]string, fanout)
	for i := 0; i < fanout && i < len(ranked); i++ {
		selected[i] = ranked[i].peerID
	}

	return selected
}

// GetOptimalFanout returns the optimal fanout for current network conditions
func (fc *GossipFanoutControl) GetOptimalFanout() int {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.optimalFanout
}

// RecordPropagationResult records the result of a propagation attempt
func (fc *GossipFanoutControl) RecordPropagationResult(fanout int, successRate float64) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.fanoutHistory = append(fc.fanoutHistory, fanout)
	fc.successRates = append(fc.successRates, successRate)

	// Trim history
	if len(fc.fanoutHistory) > 100 {
		fc.fanoutHistory = fc.fanoutHistory[len(fc.fanoutHistory)-100:]
		fc.successRates = fc.successRates[len(fc.successRates)-100:]
	}

	// Adjust optimal fanout based on success rate
	fc.adjustOptimalFanout()
}

// updateNetworkMetrics updates network-wide metrics
func (fc *GossipFanoutControl) updateNetworkMetrics() {
	fc.networkSize = len(fc.peerScores)

	// Calculate network density (average peer quality)
	if fc.networkSize > 0 {
		totalQuality := 0.0
		for _, score := range fc.peerScores {
			quality := score.PropagationScore*0.4 +
				score.LatencyScore*0.3 +
				score.TrustScore*0.2 +
				score.Availability*0.1
			totalQuality += quality
		}
		fc.networkDensity = totalQuality / float64(fc.networkSize)
	}
}

// adjustOptimalFanout adjusts the optimal fanout based on propagation history
func (fc *GossipFanoutControl) adjustOptimalFanout() {
	if len(fc.successRates) < 10 {
		return // Need more data
	}

	// Calculate average success rate
	avgSuccessRate := 0.0
	for _, rate := range fc.successRates[len(fc.successRates)-10:] {
		avgSuccessRate += rate
	}
	avgSuccessRate /= 10.0

	// Adjust fanout based on success rate
	// Target: 90% success rate
	// < 80%: decrease fanout (network congested)
	// 80-95%: maintain current fanout
	// > 95%: increase fanout (can handle more load)

	switch {
	case avgSuccessRate < 0.8:
		// Decrease fanout
		fc.optimalFanout = max(fc.optimalFanout-1, fc.config.MinFanout)
	case avgSuccessRate > 0.95:
		// Increase fanout
		fc.optimalFanout = min(fc.optimalFanout+1, fc.config.MaxFanout)
	default:
		// Maintain current fanout
	}
}

// GetPeerStats returns peer statistics
func (fc *GossipFanoutControl) GetPeerStats() map[string]interface{} {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	peerStats := make(map[string]interface{})
	for peerID, score := range fc.peerScores {
		peerStats[peerID] = map[string]interface{}{
			"propagation_score": score.PropagationScore,
			"latency_score":     score.LatencyScore,
			"trust_score":       score.TrustScore,
			"avg_latency_ms":    score.AvgLatency.Milliseconds(),
			"messages_sent":     score.MessagesSent,
			"success_rate":      score.PropagationScore,
		}
	}

	return map[string]interface{}{
		"network_size":     fc.networkSize,
		"network_density":  fc.networkDensity,
		"current_fanout":   fc.currentFanout,
		"optimal_fanout":   fc.optimalFanout,
		"peer_count":       len(fc.peerScores),
		"peers":            peerStats,
	}
}

// GossipMetrics implements comprehensive gossip protocol metrics
type GossipMetrics struct {
	mu sync.RWMutex

	// Message counters
	messagesProcessed   uint64
	messagesPropagated  uint64
	messagesDropped     uint64
	messagesDuplicates  uint64
	messagesRateLimited uint64
	messagesInvalid     uint64
	messagesBroadcast   uint64

	// Performance metrics
	totalLatency       time.Duration
	latencyCount       uint64
	propagationLatency time.Duration
	propagationCount   uint64

	// Success/failure tracking
	successCount uint64
	failureCount uint64

	// Per-type metrics
	typeMetrics map[GossipMessageType]*TypeMetrics

	// Load calculation
	recentLoad      []float64
	currentLoad     float64
	lastCalculation time.Time

	// Per-peer metrics
	peerMessages map[string]*PeerMessageMetrics
}

// TypeMetrics tracks metrics for a specific message type
type TypeMetrics struct {
	Received    uint64
	Propagated  uint64
	Dropped     uint64
	AvgLatency  time.Duration
	SuccessRate float64
}

// PeerMessageMetrics tracks per-peer message statistics
type PeerMessageMetrics struct {
	MessagesReceived uint64
	MessagesSent     uint64
	LastActivity     time.Time
}

// NewGossipMetrics creates a new metrics collector
func NewGossipMetrics() *GossipMetrics {
	return &GossipMetrics{
		typeMetrics:  make(map[GossipMessageType]*TypeMetrics),
		recentLoad:   make([]float64, 0, 60), // 60 seconds of load data
		peerMessages: make(map[string]*PeerMessageMetrics),
	}
}

// RecordProcessed records a processed message
func (m *GossipMetrics) RecordProcessed() {
	atomic.AddUint64(&m.messagesProcessed, 1)
}

// RecordPropagated records a propagated message
func (m *GossipMetrics) RecordPropagated(msgType GossipMessageType) {
	atomic.AddUint64(&m.messagesPropagated, 1)
	m.recordTypeMetric(msgType, func(tm *TypeMetrics) { tm.Propagated++ })
}

// RecordDropped records a dropped message
func (m *GossipMetrics) RecordDropped() {
	atomic.AddUint64(&m.messagesDropped, 1)
}

// RecordDuplicate records a duplicate message
func (m *GossipMetrics) RecordDuplicate() {
	atomic.AddUint64(&m.messagesDuplicates, 1)
}

// RecordRateLimited records a rate-limited message
func (m *GossipMetrics) RecordRateLimited() {
	atomic.AddUint64(&m.messagesRateLimited, 1)
}

// RecordInvalid records an invalid message
func (m *GossipMetrics) RecordInvalid() {
	atomic.AddUint64(&m.messagesInvalid, 1)
}

// RecordBroadcast records a broadcast message
func (m *GossipMetrics) RecordBroadcast(msgType GossipMessageType) {
	atomic.AddUint64(&m.messagesBroadcast, 1)
	m.recordTypeMetric(msgType, func(tm *TypeMetrics) { tm.Received++ })
}

// RecordSuccess records a successful propagation with latency
func (m *GossipMetrics) RecordSuccess(latency time.Duration) {
	atomic.AddUint64(&m.successCount, 1)

	m.mu.Lock()
	m.totalLatency += latency
	m.latencyCount++
	m.mu.Unlock()
}

// RecordPropagationFailure records a propagation failure
func (m *GossipMetrics) RecordPropagationFailure() {
	atomic.AddUint64(&m.failureCount, 1)
}

// RecordPeerMessage records a message from/to a peer
func (m *GossipMetrics) RecordPeerMessage(peerID string, msgType GossipMessageType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics, exists := m.peerMessages[peerID]
	if !exists {
		metrics = &PeerMessageMetrics{}
		m.peerMessages[peerID] = metrics
	}

	metrics.MessagesReceived++
	metrics.LastActivity = time.Now()
}

// recordTypeMetric records a metric for a specific message type
func (m *GossipMetrics) recordTypeMetric(msgType GossipMessageType, update func(*TypeMetrics)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.typeMetrics[msgType]; !exists {
		m.typeMetrics[msgType] = &TypeMetrics{}
	}

	update(m.typeMetrics[msgType])
}

// UpdateLoad updates the current load factor
func (m *GossipMetrics) UpdateLoad(queueSize, maxQueueSize int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	load := float64(0)
	if maxQueueSize > 0 {
		load = float64(queueSize) / float64(maxQueueSize)
	}

	m.recentLoad = append(m.recentLoad, load)
	if len(m.recentLoad) > 60 {
		m.recentLoad = m.recentLoad[len(m.recentLoad)-60:]
	}

	// Calculate exponential moving average
	if len(m.recentLoad) > 0 {
		alpha := 0.2 // Smoothing factor
		ema := m.recentLoad[0]
		for i := 1; i < len(m.recentLoad); i++ {
			ema = alpha*m.recentLoad[i] + (1-alpha)*ema
		}
		m.currentLoad = ema
	}

	m.lastCalculation = time.Now()
}

// MessagesProcessed returns total processed messages
func (m *GossipMetrics) MessagesProcessed() uint64 {
	return atomic.LoadUint64(&m.messagesProcessed)
}

// MessagesPropagated returns total propagated messages
func (m *GossipMetrics) MessagesPropagated() uint64 {
	return atomic.LoadUint64(&m.messagesPropagated)
}

// MessagesDropped returns total dropped messages
func (m *GossipMetrics) MessagesDropped() uint64 {
	return atomic.LoadUint64(&m.messagesDropped)
}

// MessagesDuplicates returns total duplicate messages
func (m *GossipMetrics) MessagesDuplicates() uint64 {
	return atomic.LoadUint64(&m.messagesDuplicates)
}

// MessagesRateLimited returns total rate-limited messages
func (m *GossipMetrics) MessagesRateLimited() uint64 {
	return atomic.LoadUint64(&m.messagesRateLimited)
}

// MessagesInvalid returns total invalid messages
func (m *GossipMetrics) MessagesInvalid() uint64 {
	return atomic.LoadUint64(&m.messagesInvalid)
}

// GetCurrentLoad returns the current load factor
func (m *GossipMetrics) GetCurrentLoad() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentLoad
}

// GetSuccessRate returns the overall success rate
func (m *GossipMetrics) GetSuccessRate() float64 {
	success := atomic.LoadUint64(&m.successCount)
	failure := atomic.LoadUint64(&m.failureCount)

	total := success + failure
	if total == 0 {
		return 0
	}

	return float64(success) / float64(total)
}

// GetAverageLatency returns the average processing latency
func (m *GossipMetrics) GetAverageLatency() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.latencyCount == 0 {
		return 0
	}

	return m.totalLatency / time.Duration(m.latencyCount)
}

// GetAllMetrics returns all metrics as a map
func (m *GossipMetrics) GetAllMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"messages_processed":    m.MessagesProcessed(),
		"messages_propagated":   m.MessagesPropagated(),
		"messages_dropped":      m.MessagesDropped(),
		"messages_duplicates":   m.MessagesDuplicates(),
		"messages_rate_limited": m.MessagesRateLimited(),
		"messages_invalid":      m.MessagesInvalid(),
		"messages_broadcast":    atomic.LoadUint64(&m.messagesBroadcast),
		"success_count":         atomic.LoadUint64(&m.successCount),
		"failure_count":         atomic.LoadUint64(&m.failureCount),
		"success_rate":          m.GetSuccessRate(),
		"average_latency_ms":    m.GetAverageLatency().Milliseconds(),
		"current_load":          m.currentLoad,
		"type_metrics":          m.typeMetrics,
		"peer_count":            len(m.peerMessages),
	}
}
