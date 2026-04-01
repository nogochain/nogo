package ai

import (
	"sync"
	"time"
)

type NetworkAnomalyDetector struct {
	mu             sync.RWMutex
	peerStats      map[string]*PeerNetworkStats
	globalStats    *NetworkGlobalStats
	thresholds     *AnomalyThresholds
	windowDuration time.Duration
}

type PeerNetworkStats struct {
	mu              sync.RWMutex
	Address         string
	FirstSeen       time.Time
	MessagesSent    int64
	MessagesRecv    int64
	BytesSent       int64
	BytesRecv       int64
	DisconnectCount int64
	LastActivity    time.Time
	RequestRates    []RequestSample
	BanScore        float64
}

type RequestSample struct {
	Timestamp time.Time
	Count     int
	Type      string
}

type NetworkGlobalStats struct {
	mu                sync.RWMutex
	TotalPeers        int64
	ActivePeers       int64
	TotalMessages     int64
	TotalBytes        int64
	AnomaliesDetected int64
	LastUpdate        time.Time
}

type AnomalyThresholds struct {
	MaxMessagesPerSecond float64
	MaxBytesPerSecond    float64
	MaxRequestRate       int
	MaxConcurrentConns   int
	BanScoreThreshold    float64
}

type AnomalyResult struct {
	IsAnomaly   bool
	AnomalyType string
	Severity    string
	PeerAddress string
	Description string
	ShouldBan   bool
	BanScore    float64
}

const (
	AnomalyTypeDDoS         = "ddos"
	AnomalyTypeSpam         = "spam"
	AnomalyTypeLargePayload = "large_payload"
	AnomalyTypeRapidConnect = "rapid_connect"
	AnomalyTypeInvalidMsg   = "invalid_message"
	AnomalyTypeTimeout      = "timeout"
	AnomalyTypeEclipse      = "eclipse_attempt"
)

func NewNetworkAnomalyDetector() *NetworkAnomalyDetector {
	return &NetworkAnomalyDetector{
		peerStats:   make(map[string]*PeerNetworkStats),
		globalStats: &NetworkGlobalStats{},
		thresholds: &AnomalyThresholds{
			MaxMessagesPerSecond: 100,
			MaxBytesPerSecond:    10 * 1024 * 1024,
			MaxRequestRate:       50,
			MaxConcurrentConns:   10,
			BanScoreThreshold:    100,
		},
		windowDuration: 60 * time.Second,
	}
}

func (nad *NetworkAnomalyDetector) RecordMessage(peerAddr string, msgType string, size int, isOutgoing bool) {
	nad.mu.Lock()
	defer nad.mu.Unlock()

	stats, exists := nad.peerStats[peerAddr]
	if !exists {
		stats = &PeerNetworkStats{
			Address:      peerAddr,
			FirstSeen:    time.Now(),
			LastActivity: time.Now(),
		}
		nad.peerStats[peerAddr] = stats
	}

	stats.mu.Lock()
	stats.LastActivity = time.Now()
	if isOutgoing {
		stats.MessagesSent++
		stats.BytesSent += int64(size)
	} else {
		stats.MessagesRecv++
		stats.BytesRecv += int64(size)
	}

	stats.RequestRates = append(stats.RequestRates, RequestSample{
		Timestamp: time.Now(),
		Count:     1,
		Type:      msgType,
	})
	stats.mu.Unlock()

	nad.globalStats.mu.Lock()
	nad.globalStats.TotalMessages++
	nad.globalStats.TotalBytes += int64(size)
	nad.globalStats.LastUpdate = time.Now()
	nad.globalStats.mu.Unlock()
}

func (nad *NetworkAnomalyDetector) RecordDisconnect(peerAddr string, reason string) {
	nad.mu.Lock()
	defer nad.mu.Unlock()

	if stats, exists := nad.peerStats[peerAddr]; exists {
		stats.mu.Lock()
		stats.DisconnectCount++
		if reason == "timeout" {
			stats.BanScore += 10
		}
		stats.mu.Unlock()
	}
}

func (nad *NetworkAnomalyDetector) AnalyzePeer(peerAddr string) *AnomalyResult {
	nad.mu.RLock()
	stats, exists := nad.peerStats[peerAddr]
	nad.mu.RUnlock()

	if !exists {
		return &AnomalyResult{IsAnomaly: false, Severity: "none"}
	}

	result := &AnomalyResult{
		PeerAddress: peerAddr,
		IsAnomaly:   false,
	}

	stats.mu.RLock()

	windowStart := time.Now().Add(-nad.windowDuration)
	recentRequests := 0
	var msgTypes = make(map[string]int)
	var recentBytes int64

	for _, sample := range stats.RequestRates {
		if sample.Timestamp.After(windowStart) {
			recentRequests += sample.Count
			msgTypes[sample.Type]++
		}
	}

	for i := len(stats.RequestRates) - 1; i >= 0; i-- {
		if stats.RequestRates[i].Timestamp.Before(windowStart) {
			break
		}
		recentBytes += int64(stats.RequestRates[i].Count * 100)
	}

	stats.mu.RUnlock()

	msgRate := float64(recentRequests) / nad.windowDuration.Seconds()
	bytesRate := float64(recentBytes) / nad.windowDuration.Seconds()

	if msgRate > nad.thresholds.MaxMessagesPerSecond {
		result.IsAnomaly = true
		result.AnomalyType = AnomalyTypeDDoS
		result.Description = "Message rate exceeds threshold"
		result.Severity = "high"
		result.BanScore += 30
	}

	if bytesRate > nad.thresholds.MaxBytesPerSecond {
		result.IsAnomaly = true
		result.AnomalyType = AnomalyTypeLargePayload
		result.Description = "Data rate exceeds threshold"
		result.Severity = "medium"
		result.BanScore += 20
	}

	if recentRequests > nad.thresholds.MaxRequestRate {
		result.IsAnomaly = true
		result.AnomalyType = AnomalyTypeSpam
		result.Description = "Too many requests in time window"
		result.Severity = "high"
		result.BanScore += 25
	}

	stats.mu.RLock()
	if stats.DisconnectCount > 10 {
		result.IsAnomaly = true
		result.AnomalyType = AnomalyTypeTimeout
		result.Description = "Too many disconnects"
		result.Severity = "medium"
		result.BanScore += 15
	}
	stats.mu.RUnlock()

	_, hasInvalid := msgTypes["invalid"]
	if hasInvalid {
		result.IsAnomaly = true
		result.AnomalyType = AnomalyTypeInvalidMsg
		result.Description = "Sent invalid messages"
		result.Severity = "medium"
		result.BanScore += 20
	}

	if msgTypes["getdata"] > 30 && msgTypes["block"] == 0 {
		result.IsAnomaly = true
		result.AnomalyType = AnomalyTypeEclipse
		result.Description = "Possible eclipse attempt - requesting hashes but not blocks"
		result.Severity = "high"
		result.BanScore += 40
	}

	if result.BanScore >= nad.thresholds.BanScoreThreshold {
		result.ShouldBan = true
	}

	return result
}

func (nad *NetworkAnomalyDetector) GetGlobalStats() NetworkGlobalStats {
	nad.mu.RLock()
	defer nad.mu.RUnlock()

	nad.globalStats.mu.RLock()
	defer nad.globalStats.mu.RUnlock()

	return NetworkGlobalStats{
		TotalPeers:        int64(len(nad.peerStats)),
		ActivePeers:       nad.globalStats.ActivePeers,
		TotalMessages:     nad.globalStats.TotalMessages,
		TotalBytes:        nad.globalStats.TotalBytes,
		AnomaliesDetected: nad.globalStats.AnomaliesDetected,
		LastUpdate:        nad.globalStats.LastUpdate,
	}
}

func (nad *NetworkAnomalyDetector) GetPeerStats(peerAddr string) *PeerNetworkStats {
	nad.mu.RLock()
	defer nad.mu.RUnlock()
	return nad.peerStats[peerAddr]
}

func (nad *NetworkAnomalyDetector) CleanupStalePeers(maxAge time.Duration) {
	nad.mu.Lock()
	defer nad.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for addr, stats := range nad.peerStats {
		stats.mu.RLock()
		if stats.LastActivity.Before(cutoff) {
			delete(nad.peerStats, addr)
		}
		stats.mu.RUnlock()
	}
}
