package main

import (
	"math"
	"sync"
	"time"
)

type NetworkAnomalyDetector struct {
	mu           sync.RWMutex
	blockTimes   []time.Duration
	txPerBlock   []int
	blockSizes   []int
	knownForks   map[string]int
	windowSize   int
	thresholdStd float64
}

func NewNetworkAnomalyDetector(windowSize int, thresholdStd float64) *NetworkAnomalyDetector {
	if windowSize <= 0 {
		windowSize = 100
	}
	if thresholdStd <= 0 {
		thresholdStd = 3.0
	}
	return &NetworkAnomalyDetector{
		blockTimes:   make([]time.Duration, 0, windowSize),
		txPerBlock:   make([]int, 0, windowSize),
		blockSizes:   make([]int, 0, windowSize),
		knownForks:   make(map[string]int),
		windowSize:   windowSize,
		thresholdStd: thresholdStd,
	}
}

type AnomalyReport struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"`
	Metric      string  `json:"metric"`
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold"`
	Description string  `json:"description"`
	Timestamp   int64   `json:"timestamp"`
}

func (d *NetworkAnomalyDetector) RecordBlock(blockTime time.Duration, txCount, blockSize int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.blockTimes = append(d.blockTimes, blockTime)
	d.txPerBlock = append(d.txPerBlock, txCount)
	d.blockSizes = append(d.blockSizes, blockSize)

	if len(d.blockTimes) > d.windowSize {
		d.blockTimes = d.blockTimes[1:]
		d.txPerBlock = d.txPerBlock[1:]
		d.blockSizes = d.blockSizes[1:]
	}
}

func (d *NetworkAnomalyDetector) RecordFork(forkID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.knownForks[forkID]++
}

func (d *NetworkAnomalyDetector) Detect() []AnomalyReport {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var reports []AnomalyReport
	now := time.Now().Unix()

	if len(d.blockTimes) < 10 {
		return reports
	}

	blockTimeAnomalies := d.detectBlockTimeAnomalies()
	reports = append(reports, blockTimeAnomalies...)

	txRateAnomalies := d.detectTransactionRateAnomalies()
	reports = append(reports, txRateAnomalies...)

	blockSizeAnomalies := d.detectBlockSizeAnomalies()
	reports = append(reports, blockSizeAnomalies...)

	for forkID, count := range d.knownForks {
		if count > 3 {
			reports = append(reports, AnomalyReport{
				Type:        "fork",
				Severity:    "high",
				Metric:      "fork_count",
				Value:       float64(count),
				Threshold:   3,
				Description: "Multiple forks detected from same chain: " + forkID,
				Timestamp:   now,
			})
		}
	}

	return reports
}

func (d *NetworkAnomalyDetector) detectBlockTimeAnomalies() []AnomalyReport {
	var reports []AnomalyReport

	mean := meanDuration(d.blockTimes)
	std := stdDuration(d.blockTimes, mean)
	threshold := mean + time.Duration(d.thresholdStd*float64(std))

	latest := d.blockTimes[len(d.blockTimes)-1]
	if latest > threshold {
		reports = append(reports, AnomalyReport{
			Type:        "timing",
			Severity:    "medium",
			Metric:      "block_time",
			Value:       float64(latest.Milliseconds()),
			Threshold:   float64(threshold.Milliseconds()),
			Description: "Block time significantly higher than average",
			Timestamp:   time.Now().Unix(),
		})
	}

	meanMs := float64(mean.Milliseconds())
	stdMs := float64(std.Milliseconds())
	if stdMs > meanMs*0.5 {
		reports = append(reports, AnomalyReport{
			Type:        "timing",
			Severity:    "high",
			Metric:      "block_time_variance",
			Value:       stdMs,
			Threshold:   meanMs * 0.5,
			Description: "High variance in block times - possible instability",
			Timestamp:   time.Now().Unix(),
		})
	}

	return reports
}

func (d *NetworkAnomalyDetector) detectTransactionRateAnomalies() []AnomalyReport {
	var reports []AnomalyReport

	mean := meanInt(d.txPerBlock)
	std := stdInt(d.txPerBlock, mean)

	latest := d.txPerBlock[len(d.txPerBlock)-1]
	threshold := float64(mean) + d.thresholdStd*std

	if float64(latest) > threshold {
		reports = append(reports, AnomalyReport{
			Type:        "transaction_rate",
			Severity:    "medium",
			Metric:      "tx_per_block",
			Value:       float64(latest),
			Threshold:   threshold,
			Description: "Unusually high transaction count per block",
			Timestamp:   time.Now().Unix(),
		})
	}

	return reports
}

func (d *NetworkAnomalyDetector) detectBlockSizeAnomalies() []AnomalyReport {
	var reports []AnomalyReport

	mean := meanInt(d.blockSizes)
	std := stdInt(d.blockSizes, mean)

	latest := d.blockSizes[len(d.blockSizes)-1]
	threshold := float64(mean) + d.thresholdStd*std

	if float64(latest) > threshold {
		reports = append(reports, AnomalyReport{
			Type:        "block_size",
			Severity:    "high",
			Metric:      "block_bytes",
			Value:       float64(latest),
			Threshold:   threshold,
			Description: "Abnormally large block - potential attack",
			Timestamp:   time.Now().Unix(),
		})
	}

	return reports
}

type P2PSpamDetector struct {
	mu                   sync.RWMutex
	peerRequests         map[string]*peerRequestStats
	windowDuration       time.Duration
	maxRequestsPerWindow int
	maxBytesPerWindow    int64
}

type peerRequestStats struct {
	Requests  int
	Bytes     int64
	FirstSeen time.Time
	LastSeen  time.Time
	Banned    bool
}

func NewP2PSpamDetector(windowDuration time.Duration, maxRequests, maxBytes int) *P2PSpamDetector {
	if windowDuration <= 0 {
		windowDuration = time.Minute
	}
	if maxRequests <= 0 {
		maxRequests = 100
	}
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024
	}
	return &P2PSpamDetector{
		peerRequests:         make(map[string]*peerRequestStats),
		windowDuration:       windowDuration,
		maxRequestsPerWindow: maxRequests,
		maxBytesPerWindow:    int64(maxBytes),
	}
}

func (d *P2PSpamDetector) RecordRequest(peer string, bytes int64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	stats, exists := d.peerRequests[peer]

	if !exists || now.Sub(stats.LastSeen) > d.windowDuration {
		d.peerRequests[peer] = &peerRequestStats{
			Requests:  1,
			Bytes:     bytes,
			FirstSeen: now,
			LastSeen:  now,
		}
		return
	}

	stats.Requests++
	stats.Bytes += bytes
	stats.LastSeen = now
}

func (d *P2PSpamDetector) IsBanned(peer string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats, exists := d.peerRequests[peer]
	if !exists {
		return false
	}

	now := time.Now()
	if now.Sub(stats.LastSeen) > d.windowDuration {
		return false
	}

	return stats.Requests > d.maxRequestsPerWindow || stats.Bytes > d.maxBytesPerWindow
}

func (d *P2PSpamDetector) GetStats(peer string) *peerRequestStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if stats, exists := d.peerRequests[peer]; exists {
		return stats
	}
	return nil
}

func (d *P2PSpamDetector) GetAllStats() map[string]int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string]int)
	for peer, stats := range d.peerRequests {
		result[peer] = stats.Requests
	}
	return result
}

type FeeEstimator struct {
	mu         sync.RWMutex
	recentFees []uint64
	windowSize int
	minFeeRate uint64
}

func NewFeeEstimator(windowSize int, minFeeRate uint64) *FeeEstimator {
	if windowSize <= 0 {
		windowSize = 100
	}
	if minFeeRate <= 0 {
		minFeeRate = 1
	}
	return &FeeEstimator{
		recentFees: make([]uint64, 0, windowSize),
		windowSize: windowSize,
		minFeeRate: minFeeRate,
	}
}

type FeeRecommendation struct {
	FeeRate    uint64 `json:"fee_rate"`
	Confidence string `json:"confidence"`
	WaitBlocks int    `json:"wait_blocks"`
}

func (e *FeeEstimator) RecordFee(feeRate uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.recentFees = append(e.recentFees, feeRate)
	if len(e.recentFees) > e.windowSize {
		e.recentFees = e.recentFees[1:]
	}
}

func (e *FeeEstimator) Estimate(confirmations int) FeeRecommendation {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if len(e.recentFees) == 0 {
		return FeeRecommendation{
			FeeRate:    e.minFeeRate,
			Confidence: "low",
			WaitBlocks: confirmations,
		}
	}

	percentile := 50
	if confirmations <= 1 {
		percentile = 75
	} else if confirmations >= 6 {
		percentile = 25
	}

	fee := percentileFee(e.recentFees, percentile)
	if fee < e.minFeeRate {
		fee = e.minFeeRate
	}

	confidence := "medium"
	if len(e.recentFees) < 10 {
		confidence = "low"
	} else if len(e.recentFees) > 50 {
		confidence = "high"
	}

	return FeeRecommendation{
		FeeRate:    fee,
		Confidence: confidence,
		WaitBlocks: confirmations,
	}
}

type NodeHealthMonitor struct {
	mu              sync.RWMutex
	metrics         map[string]*nodeMetricHistory
	alertThresholds nodeThresholds
}

type nodeThresholds struct {
	MaxCPU     float64
	MaxMemory  float64
	MaxLatency time.Duration
	MinPeers   int
}

type nodeMetricHistory struct {
	cpu     []float64
	memory  []float64
	latency []time.Duration
	peers   []int
}

func NewNodeHealthMonitor() *NodeHealthMonitor {
	return &NodeHealthMonitor{
		metrics: make(map[string]*nodeMetricHistory),
		alertThresholds: nodeThresholds{
			MaxCPU:     90.0,
			MaxMemory:  90.0,
			MaxLatency: 5 * time.Second,
			MinPeers:   1,
		},
	}
}

type NodeHealthReport struct {
	NodeID    string   `json:"node_id"`
	Status    string   `json:"status"`
	CPU       float64  `json:"cpu_percent"`
	Memory    float64  `json:"memory_percent"`
	Latency   int64    `json:"latency_ms"`
	Peers     int      `json:"peers"`
	Alerts    []string `json:"alerts"`
	Timestamp int64    `json:"timestamp"`
}

func (m *NodeHealthMonitor) RecordMetrics(nodeID string, cpu, memory float64, latency time.Duration, peers int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	history, exists := m.metrics[nodeID]
	if !exists {
		history = &nodeMetricHistory{
			cpu:     make([]float64, 0, 100),
			memory:  make([]float64, 0, 100),
			latency: make([]time.Duration, 0, 100),
			peers:   make([]int, 0, 100),
		}
		m.metrics[nodeID] = history
	}

	history.cpu = append(history.cpu, cpu)
	history.memory = append(history.memory, memory)
	history.latency = append(history.latency, latency)
	history.peers = append(history.peers, peers)

	if len(history.cpu) > 100 {
		history.cpu = history.cpu[1:]
		history.memory = history.memory[1:]
		history.latency = history.latency[1:]
		history.peers = history.peers[1:]
	}
}

func (m *NodeHealthMonitor) GetHealth(nodeID string) NodeHealthReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history, exists := m.metrics[nodeID]
	if !exists {
		return NodeHealthReport{
			NodeID:    nodeID,
			Status:    "unknown",
			Timestamp: time.Now().Unix(),
		}
	}

	avgCPU := meanFloat(history.cpu)
	avgMemory := meanFloat(history.memory)
	avgLatency := avgDuration(history.latency)
	latestPeers := history.peers[len(history.peers)-1]

	var alerts []string
	status := "healthy"

	if avgCPU > m.alertThresholds.MaxCPU {
		alerts = append(alerts, "high_cpu")
		status = "warning"
	}
	if avgMemory > m.alertThresholds.MaxMemory {
		alerts = append(alerts, "high_memory")
		status = "warning"
	}
	if avgLatency > m.alertThresholds.MaxLatency {
		alerts = append(alerts, "high_latency")
		status = "warning"
	}
	if latestPeers < m.alertThresholds.MinPeers {
		alerts = append(alerts, "low_peers")
		status = "warning"
	}

	if len(alerts) >= 2 {
		status = "critical"
	}

	return NodeHealthReport{
		NodeID:    nodeID,
		Status:    status,
		CPU:       avgCPU,
		Memory:    avgMemory,
		Latency:   avgLatency.Milliseconds(),
		Peers:     latestPeers,
		Alerts:    alerts,
		Timestamp: time.Now().Unix(),
	}
}

type WalletBehaviorAnalyzer struct {
	mu           sync.RWMutex
	addressStats map[string]*walletStats
	windowSize   int
}

type walletStats struct {
	address       string
	txCount       int
	totalSent     uint64
	totalReceived uint64
	avgTxSize     float64
	lastActive    time.Time
	frequency     time.Duration
	firstSeen     time.Time
}

func NewWalletBehaviorAnalyzer(windowSize int) *WalletBehaviorAnalyzer {
	if windowSize <= 0 {
		windowSize = 1000
	}
	return &WalletBehaviorAnalyzer{
		addressStats: make(map[string]*walletStats),
		windowSize:   windowSize,
	}
}

type WalletRiskProfile struct {
	Address           string  `json:"address"`
	RiskLevel         string  `json:"risk_level"`
	RiskScore         float64 `json:"risk_score"`
	Transactions      int     `json:"transactions"`
	TotalVolume       uint64  `json:"total_volume"`
	ActivityFrequency string  `json:"activity_frequency"`
	FirstSeen         int64   `json:"first_seen"`
	LastActive        int64   `json:"last_active"`
	Age               string  `json:"age"`
}

func (a *WalletBehaviorAnalyzer) RecordTransaction(address string, sent, received uint64, timestamp time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()

	stats, exists := a.addressStats[address]
	if !exists {
		stats = &walletStats{
			address:    address,
			firstSeen:  timestamp,
			lastActive: timestamp,
		}
		a.addressStats[address] = stats
	}

	stats.txCount++
	stats.totalSent += sent
	stats.totalReceived += received

	if stats.txCount > 1 {
		stats.avgTxSize = float64(stats.totalSent+stats.totalReceived) / float64(stats.txCount)
	}

	if timestamp.After(stats.lastActive) {
		stats.lastActive = timestamp
	}

	now := time.Now()
	if stats.txCount > 1 {
		stats.frequency = now.Sub(stats.firstSeen) / time.Duration(stats.txCount)
	}
}

func (a *WalletBehaviorAnalyzer) GetRiskProfile(address string) WalletRiskProfile {
	a.mu.RLock()
	defer a.mu.RUnlock()

	stats, exists := a.addressStats[address]
	if !exists {
		return WalletRiskProfile{
			Address:   address,
			RiskLevel: "unknown",
		}
	}

	riskScore := calculateWalletRiskScore(stats)
	riskLevel := "low"
	if riskScore >= 70 {
		riskLevel = "high"
	} else if riskScore >= 40 {
		riskLevel = "medium"
	}

	var frequency string
	if stats.frequency < time.Hour {
		frequency = "very_high"
	} else if stats.frequency < time.Hour*24 {
		frequency = "high"
	} else if stats.frequency < time.Hour*24*7 {
		frequency = "medium"
	} else {
		frequency = "low"
	}

	age := time.Since(stats.firstSeen)
	ageStr := "new"
	if age > time.Hour*24*365 {
		ageStr = "old"
	} else if age > time.Hour*24*30 {
		ageStr = "established"
	}

	return WalletRiskProfile{
		Address:           address,
		RiskLevel:         riskLevel,
		RiskScore:         riskScore,
		Transactions:      stats.txCount,
		TotalVolume:       stats.totalSent + stats.totalReceived,
		ActivityFrequency: frequency,
		FirstSeen:         stats.firstSeen.Unix(),
		LastActive:        stats.lastActive.Unix(),
		Age:               ageStr,
	}
}

func calculateWalletRiskScore(stats *walletStats) float64 {
	score := 0.0

	if stats.txCount == 0 {
		return 0
	}

	if stats.txCount > 1000 {
		score += 30
	} else if stats.txCount > 100 {
		score += 15
	}

	volume := stats.totalSent + stats.totalReceived
	if volume > 10_000_000 {
		score += 20
	} else if volume > 1_000_000 {
		score += 10
	}

	if stats.frequency < time.Minute {
		score += 40
	} else if stats.frequency < time.Hour {
		score += 20
	} else if stats.frequency < time.Hour*24 {
		score += 10
	}

	now := time.Now()
	if now.Sub(stats.lastActive) > time.Hour*24*30 {
		score += 10
	}

	return math.Min(score, 100)
}

func meanDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	sum := int64(0)
	for _, d := range durations {
		sum += d.Nanoseconds()
	}
	return time.Duration(sum/int64(len(durations))) * time.Nanosecond
}

func stdDuration(durations []time.Duration, mean time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var sumSq float64
	for _, d := range durations {
		diff := float64(d - mean)
		sumSq += diff * diff
	}
	return time.Duration(math.Sqrt(sumSq/float64(len(durations)))) * time.Nanosecond
}

func meanInt(arr []int) float64 {
	if len(arr) == 0 {
		return 0
	}
	sum := 0
	for _, v := range arr {
		sum += v
	}
	return float64(sum) / float64(len(arr))
}

func stdInt(arr []int, mean float64) float64 {
	if len(arr) == 0 {
		return 0
	}
	var sumSq float64
	for _, v := range arr {
		diff := float64(v) - mean
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(len(arr)))
}

func meanFloat(arr []float64) float64 {
	if len(arr) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range arr {
		sum += v
	}
	return sum / float64(len(arr))
}

func avgDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	sum := int64(0)
	for _, d := range durations {
		sum += d.Milliseconds()
	}
	return time.Duration(sum/int64(len(durations))) * time.Millisecond
}

func percentileFee(fees []uint64, percentile int) uint64 {
	if len(fees) == 0 {
		return 0
	}
	sorted := make([]uint64, len(fees))
	copy(sorted, fees)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	idx := len(sorted) * percentile / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
