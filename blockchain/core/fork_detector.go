// Copyright 2026 NogoChain Team
// Production-grade fork detection and alerting system
// Monitors chain state and detects persistent forks

package core

import (
	"fmt"
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
)

// ForkEvent represents a fork detection event
type ForkEvent struct {
	Type           ForkType
	DetectedAt     time.Time
	LocalHeight    uint64
	LocalHash      string
	RemoteHeight   uint64
	RemoteHash     string
	Depth          uint64
	LocalWork      *big.Int
	RemoteWork     *big.Int
	PeerID         string
	AlertLevel     string
}

// ForkDetector monitors chain state for forks
// Production-grade: real-time fork detection with alerting
// Thread-safe: uses mutex for event management
type ForkDetector struct {
	mu               sync.RWMutex
	events           []ForkEvent
	maxEvents        int
	alertCallback    func(ForkEvent)
	lastAlert        time.Time
	alertCooldown    time.Duration
	deepForkThreshold uint64
}

// NewForkDetector creates a new fork detector
func NewForkDetector() *ForkDetector {
	return &ForkDetector{
		events:            make([]ForkEvent, 0),
		maxEvents:         1000,
		alertCooldown:     30 * time.Second,
		deepForkThreshold: 6, // Bitcoin's 6-block confirmation rule
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

	// Same height but different hash = fork at this height
	if localBlock.Height == remoteBlock.Height {
		event := &ForkEvent{
			Type:         ForkTypePersistent,
			DetectedAt:   time.Now(),
			LocalHeight:  localBlock.Height,
			LocalHash:    fmt.Sprintf("%x", localBlock.Hash),
			RemoteHeight: remoteBlock.Height,
			RemoteHash:   fmt.Sprintf("%x", remoteBlock.Hash),
			Depth:        0,
			PeerID:       peerID,
			AlertLevel:   "HIGH",
		}

		// Parse work values
		localWork, _ := StringToWork(localBlock.TotalWork)
		remoteWork, _ := StringToWork(remoteBlock.TotalWork)
		event.LocalWork = localWork
		event.RemoteWork = remoteWork

		// Determine alert level based on work difference
		if remoteWork != nil && localWork != nil {
			workDiff := new(big.Int).Sub(remoteWork, localWork)
			if workDiff.Sign() > 0 {
				event.AlertLevel = "CRITICAL"
				event.Type = ForkTypePersistent
			}
		}

		fd.recordEvent(event)
		fd.triggerAlert(event)

		return event
	}

	// Different heights - calculate fork depth
	var deeperBlock *Block
	var shallowerBlock *Block

	if localBlock.Height > remoteBlock.Height {
		deeperBlock = localBlock
		shallowerBlock = remoteBlock
	} else {
		deeperBlock = remoteBlock
		shallowerBlock = localBlock
	}

	// Calculate approximate fork depth
	depth := deeperBlock.Height - shallowerBlock.Height

	event := &ForkEvent{
		Type:         ForkTypeTemporary,
		DetectedAt:   time.Now(),
		LocalHeight:  localBlock.Height,
		LocalHash:    fmt.Sprintf("%x", localBlock.Hash),
		RemoteHeight: remoteBlock.Height,
		RemoteHash:   fmt.Sprintf("%x", remoteBlock.Hash),
		Depth:        depth,
		PeerID:       peerID,
		AlertLevel:   "MEDIUM",
	}

	// Classify fork type based on depth
	if depth >= fd.deepForkThreshold {
		event.Type = ForkTypeDeep
		event.AlertLevel = "CRITICAL"
	}

	// Parse work values
	localWork, _ := StringToWork(localBlock.TotalWork)
	remoteWork, _ := StringToWork(remoteBlock.TotalWork)
	event.LocalWork = localWork
	event.RemoteWork = remoteWork

	fd.recordEvent(event)
	fd.triggerAlert(event)

	return event
}

// recordEvent records a fork event
func (fd *ForkDetector) recordEvent(event *ForkEvent) {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	fd.events = append(fd.events, *event)

	// Maintain max events limit
	if len(fd.events) > fd.maxEvents {
		fd.events = fd.events[len(fd.events)-fd.maxEvents:]
	}
}

// triggerAlert triggers an alert if cooldown period has passed
func (fd *ForkDetector) triggerAlert(event *ForkEvent) {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	// Check cooldown
	if time.Since(fd.lastAlert) < fd.alertCooldown {
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
		"total_forks":        len(fd.events),
		"persistent_forks":   0,
		"deep_forks":         0,
		"last_fork_time":     nil,
		"max_fork_depth":     0,
		"avg_fork_depth":     0.0,
		"critical_alerts":    0,
	}

	if len(fd.events) == 0 {
		return stats
	}

	var totalDepth uint64
	for _, event := range fd.events {
		switch event.Type {
		case ForkTypePersistent:
			stats["persistent_forks"] = stats["persistent_forks"].(int) + 1
		case ForkTypeDeep:
			stats["deep_forks"] = stats["deep_forks"].(int) + 1
		}

		if event.AlertLevel == "CRITICAL" {
			stats["critical_alerts"] = stats["critical_alerts"].(int) + 1
		}

		if event.Depth > stats["max_fork_depth"].(uint64) {
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

	fd.deepForkThreshold = threshold
}

// GetDeepForkThreshold returns the current deep fork threshold
func (fd *ForkDetector) GetDeepForkThreshold() uint64 {
	fd.mu.RLock()
	defer fd.mu.RUnlock()

	return fd.deepForkThreshold
}
