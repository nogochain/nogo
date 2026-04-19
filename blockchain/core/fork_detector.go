// Copyright 2026 NogoChain Team
// Production-grade fork detection and alerting system
// Monitors chain state and detects persistent forks

package core

import (
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"
)

// ForkType represents the type of fork detected
type ForkType int

const (
	// ForkTypeNone indicates no fork detected
	ForkTypeNone ForkType = iota
	// ForkTypeTemporary indicates a temporary fork that resolved automatically
	ForkTypeTemporary
	// ForkTypePersistent indicates a persistent fork requiring intervention
	ForkTypePersistent
// ForkTypeDeep indicates a deep fork (depth > 6 blocks)
ForkTypeDeep
// ForkTypeSymmetric indicates symmetric fork (equal work in multi-node)
ForkTypeSymmetric
)

// ForkEvent represents a fork detection event
type ForkEvent struct {
	Type         ForkType
	DetectedAt   time.Time
	LocalHeight  uint64
	LocalHash    string
	RemoteHeight uint64
	RemoteHash   string
	Depth        uint64
	LocalWork    *big.Int
	RemoteWork   *big.Int
	PeerID       string
	AlertLevel   string
}

// ForkDetectorConfig holds configurable parameters for fork detection
type ForkDetectorConfig struct {
	MaxEvents         int
	AlertCooldown     time.Duration
	DeepForkThreshold uint64
}

// DefaultForkDetectorConfig returns production-grade default configuration
func DefaultForkDetectorConfig() ForkDetectorConfig {
	return ForkDetectorConfig{
		MaxEvents:         1000,
		AlertCooldown:     30 * time.Second,
		DeepForkThreshold: 6,
	}
}

// ForkDetector monitors chain state for forks
// Production-grade: real-time fork detection with alerting
// Thread-safe: uses mutex for event management
type ForkDetector struct {
	mu            sync.RWMutex
	events        []ForkEvent
	cfg           ForkDetectorConfig
	alertCallback func(ForkEvent)
	lastAlert     time.Time
}

// NewForkDetector creates a new fork detector
func NewForkDetector(cfg ForkDetectorConfig) *ForkDetector {
	return &ForkDetector{
		events: make([]ForkEvent, 0),
		cfg:    cfg,
	}
}

// SetAlertCallback sets the callback function for fork alerts
func (fd *ForkDetector) SetAlertCallback(callback func(ForkEvent)) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	fd.alertCallback = callback
}

// DetectFork detects if a fork exists between local and remote chains
// Returns fork event if detected, nil if no fork
func (fd *ForkDetector) DetectFork(localBlock, remoteBlock *Block, peerID string) *ForkEvent {
	if localBlock == nil || remoteBlock == nil {
		return nil
	}

	// Same block means no fork
	if string(localBlock.Hash) == string(remoteBlock.Hash) {
		return nil
	}

	// Enhanced fork detection with symmetric fork handling
	event := &ForkEvent{
		DetectedAt:   time.Now(),
		LocalHeight:  localBlock.GetHeight(),
		LocalHash:    fmt.Sprintf("%x", localBlock.Hash),
		RemoteHeight: remoteBlock.GetHeight(),
		RemoteHash:   fmt.Sprintf("%x", remoteBlock.Hash),
		PeerID:       peerID,
	}

	// Parse work values
	localWork, localWorkOk := StringToWork(localBlock.TotalWork)
	remoteWork, remoteWorkOk := StringToWork(remoteBlock.TotalWork)
	event.LocalWork = localWork
	event.RemoteWork = remoteWork

	if !localWorkOk || !remoteWorkOk {
		event.Type = ForkTypePersistent
		event.AlertLevel = "HIGH"
		event.Depth = 0
		fd.recordEvent(event)
		fd.triggerAlert(event)
		return event
	}

	// Core-Geth style: Check for symmetric fork condition
	if localBlock.GetHeight() == remoteBlock.GetHeight() {
		// Same height but different hash = fork at this height
		event.Type = ForkTypePersistent
		event.Depth = 0
		event.AlertLevel = "HIGH"

		// Enhanced symmetric fork detection
		if localWork != nil && remoteWork != nil && localWork.Cmp(remoteWork) == 0 {
			event.Type = ForkTypeSymmetric
			event.AlertLevel = "CRITICAL"
			log.Printf("CRITICAL: symmetric fork detected at height=%d local_hash=%x remote_hash=%x",
				localBlock.GetHeight(), localBlock.Hash[:8], remoteBlock.Hash[:8])
		}

		// Determine alert level based on work difference
		if remoteWork != nil && localWork != nil {
			workDiff := new(big.Int).Sub(remoteWork, localWork)
			if workDiff.Sign() > 0 {
				event.AlertLevel = "CRITICAL"
				event.Type = ForkTypePersistent
			}
		}
	} else {
		// Different heights - calculate fork depth
		var deeperBlock *Block
		var shallowerBlock *Block

		if localBlock.GetHeight() > remoteBlock.GetHeight() {
			deeperBlock = localBlock
			shallowerBlock = remoteBlock
		} else {
			deeperBlock = remoteBlock
			shallowerBlock = localBlock
		}

		// Approximate fork depth as height difference
		// True fork depth requires walking back to common ancestor
		depth := deeperBlock.GetHeight() - shallowerBlock.GetHeight()
		event.Depth = depth
		event.Type = ForkTypeTemporary
		event.AlertLevel = "MEDIUM"

		// Classify fork type based on depth
		if depth >= fd.cfg.DeepForkThreshold {
			event.Type = ForkTypeDeep
			event.AlertLevel = "CRITICAL"
		}
	}

	fd.recordEvent(event)
	fd.triggerAlert(event)

	return event
}

// DetectDynamicTopologyFork detects forks in dynamic node environments
// Addresses issues where node exit causes synchronization paralysis
func (fd *ForkDetector) DetectDynamicTopologyFork(localBlock, remoteBlock *Block, 
	peerID string, topologyMetrics map[string]interface{}) *ForkEvent {
	
	// Check for dynamic topology indicators
	if topologyMetrics != nil {
		nodeCount, ok := topologyMetrics["active_nodes"].(int)
		if ok && nodeCount <= 2 {
			// Special handling for small network scenarios
			log.Printf("dynamic topology: small_network_detected active_nodes=%d", nodeCount)
			
			// Enhance fork detection sensitivity for small networks
			forkEvent := fd.DetectFork(localBlock, remoteBlock, peerID)
			if forkEvent != nil && forkEvent.Type == ForkTypePersistent {
				// Upgrade to critical for small networks
				forkEvent.AlertLevel = "CRITICAL"
			}
			return forkEvent
		}
	}

	return fd.DetectFork(localBlock, remoteBlock, peerID)
}

// recordEvent records a fork event
func (fd *ForkDetector) recordEvent(event *ForkEvent) {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	fd.events = append(fd.events, *event)

	// Maintain max events limit
	if len(fd.events) > fd.cfg.MaxEvents {
		fd.events = fd.events[len(fd.events)-fd.cfg.MaxEvents:]
	}
}

// triggerAlert triggers an alert if cooldown period has passed
func (fd *ForkDetector) triggerAlert(event *ForkEvent) {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	// Check cooldown
	if time.Since(fd.lastAlert) < fd.cfg.AlertCooldown {
		return
	}

	// Trigger alert
	if fd.alertCallback != nil {
		fd.alertCallback(*event)
		fd.lastAlert = time.Now()
	}
}

// GetRecentForks returns recent fork events
func (fd *ForkDetector) GetRecentForks(since time.Time) []ForkEvent {
	fd.mu.RLock()
	defer fd.mu.RUnlock()

	var recent []ForkEvent
	for i := len(fd.events) - 1; i >= 0; i-- {
		if fd.events[i].DetectedAt.Before(since) {
			break
		}
		recent = append(recent, fd.events[i])
	}

	// Reverse to chronological order
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}

	return recent
}

// GetForkStats returns statistics about detected forks
func (fd *ForkDetector) GetForkStats() map[string]interface{} {
	fd.mu.RLock()
	defer fd.mu.RUnlock()

	stats := map[string]interface{}{
		"total_forks":      len(fd.events),
		"persistent_forks": 0,
		"deep_forks":       0,
		"last_fork_time":   nil,
		"max_fork_depth":   0,
		"avg_fork_depth":   0.0,
		"critical_alerts":  0,
	}

	if len(fd.events) == 0 {
		return stats
	}

	var totalDepth uint64
	for _, event := range fd.events {
		switch event.Type {
		case ForkTypePersistent:
			persistentForks, _ := stats["persistent_forks"].(int)
			stats["persistent_forks"] = persistentForks + 1
		case ForkTypeDeep:
			deepForks, _ := stats["deep_forks"].(int)
			stats["deep_forks"] = deepForks + 1
		}

		if event.AlertLevel == "CRITICAL" {
			criticalAlerts, _ := stats["critical_alerts"].(int)
			stats["critical_alerts"] = criticalAlerts + 1
		}

		maxForkDepth, _ := stats["max_fork_depth"].(uint64)
		if event.Depth > maxForkDepth {
			stats["max_fork_depth"] = event.Depth
		}
		totalDepth += event.Depth
	}

	stats["last_fork_time"] = fd.events[len(fd.events)-1].DetectedAt
	stats["avg_fork_depth"] = float64(totalDepth) / float64(len(fd.events))

	return stats
}

// ClearEvents clears all recorded fork events
func (fd *ForkDetector) ClearEvents() {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	fd.events = make([]ForkEvent, 0)
	fd.lastAlert = time.Time{}
}

// SetDeepForkThreshold sets the threshold for deep fork detection
func (fd *ForkDetector) SetDeepForkThreshold(threshold uint64) {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	fd.cfg.DeepForkThreshold = threshold
}

// GetDeepForkThreshold returns the current deep fork threshold
func (fd *ForkDetector) GetDeepForkThreshold() uint64 {
	fd.mu.RLock()
	defer fd.mu.RUnlock()

	return fd.cfg.DeepForkThreshold
}
