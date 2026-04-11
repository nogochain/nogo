// Copyright 2026 NogoChain Team
// Production-grade automated fork handling system
// Integrates fork detection with chain reorganization

package core

import (
	"log"
	"sync"
	"time"
)

// ForkAutoHandler handles automated fork detection and recovery
// Production-grade: fully automated fork resolution system
// Thread-safe: uses mutex for internal state management
type ForkAutoHandler struct {
	mu            sync.RWMutex
	forkDetector  *ForkDetector
	chainSelector *ChainSelector
	chain         *Chain
	enabled       bool
	recoveryInterval time.Duration
	// Recovery statistics
	recoveryStats map[string]interface{}
	// Last recovery time
	lastRecovery time.Time
}

// NewForkAutoHandler creates a new fork auto handler
func NewForkAutoHandler(detector *ForkDetector, selector *ChainSelector, chain *Chain) *ForkAutoHandler {
	handler := &ForkAutoHandler{
		forkDetector:     detector,
		chainSelector:    selector,
		chain:            chain,
		enabled:          true,
		recoveryInterval: 30 * time.Second,
		recoveryStats:    make(map[string]interface{}),
		lastRecovery:     time.Time{},
	}

	// Initialize recovery stats
	handler.recoveryStats["total_recoveries"] = 0
	handler.recoveryStats["successful_recoveries"] = 0
	handler.recoveryStats["failed_recoveries"] = 0
	handler.recoveryStats["last_recovery_time"] = nil

	// Set up fork alert callback
	handler.forkDetector.SetAlertCallback(func(event ForkEvent) {
		handler.handleForkEvent(event)
	})

	return handler
}

// Start starts the automated fork handling service
func (fah *ForkAutoHandler) Start() {
	go fah.startRecoveryLoop()
	log.Println("Automated fork handling service started")
}

// Stop stops the automated fork handling service
func (fah *ForkAutoHandler) Stop() {
	fah.mu.Lock()
	fah.enabled = false
	fah.mu.Unlock()
	log.Println("Automated fork handling service stopped")
}

// IsEnabled returns whether the automated fork handling is enabled
func (fah *ForkAutoHandler) IsEnabled() bool {
	fah.mu.RLock()
	defer fah.mu.RUnlock()
	return fah.enabled
}

// SetRecoveryInterval sets the recovery interval
func (fah *ForkAutoHandler) SetRecoveryInterval(interval time.Duration) {
	fah.mu.Lock()
	defer fah.mu.Unlock()
	fah.recoveryInterval = interval
}

// GetRecoveryStats returns the recovery statistics
func (fah *ForkAutoHandler) GetRecoveryStats() map[string]interface{} {
	fah.mu.RLock()
	defer fah.mu.RUnlock()

	stats := make(map[string]interface{})
	for k, v := range fah.recoveryStats {
		stats[k] = v
	}

	return stats
}

// startRecoveryLoop starts the recovery loop
func (fah *ForkAutoHandler) startRecoveryLoop() {
	ticker := time.NewTicker(fah.recoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if fah.IsEnabled() {
				fah.performRecoveryCheck()
			}
		}
	}
}

// performRecoveryCheck performs a recovery check
func (fah *ForkAutoHandler) performRecoveryCheck() {
	// Get recent fork events
	recentForks := fah.forkDetector.GetRecentForks(time.Now().Add(-5 * time.Minute))

	for _, event := range recentForks {
		// Only handle high priority forks
		if event.AlertLevel == "CRITICAL" || event.AlertLevel == "HIGH" {
			fah.handleForkEvent(event)
		}
	}

	// Also check for best chain activation
	if !fah.chainSelector.IsReorgInProgress() {
		if err := fah.chainSelector.ActivateBestChain(); err != nil {
			log.Printf("Error activating best chain: %v", err)
		}
	}
}

// handleForkEvent handles a fork event
func (fah *ForkAutoHandler) handleForkEvent(event ForkEvent) {
	// Log the fork event
	log.Printf("Fork event detected: type=%s, alert_level=%s, depth=%d, local_height=%d, remote_height=%d",
		event.Type, event.AlertLevel, event.Depth, event.LocalHeight, event.RemoteHeight)

	// Attempt to auto-fix the fork
	err := fah.forkDetector.AutoFixFork(event, fah.chain, fah.chainSelector)
	if err != nil {
		log.Printf("Failed to auto-fix fork: %v", err)
		fah.updateRecoveryStats(false)
	} else {
		log.Printf("Successfully auto-fixed fork: type=%s", event.Type)
		fah.updateRecoveryStats(true)
	}
}

// updateRecoveryStats updates the recovery statistics
func (fah *ForkAutoHandler) updateRecoveryStats(success bool) {
	fah.mu.Lock()
	defer fah.mu.Unlock()

	// Update total recoveries
	total, ok := fah.recoveryStats["total_recoveries"].(int)
	if !ok {
		total = 0
	}
	fah.recoveryStats["total_recoveries"] = total + 1

	// Update success/failure count
	if success {
		successful, ok := fah.recoveryStats["successful_recoveries"].(int)
		if !ok {
			successful = 0
		}
		fah.recoveryStats["successful_recoveries"] = successful + 1
	} else {
		failed, ok := fah.recoveryStats["failed_recoveries"].(int)
		if !ok {
			failed = 0
		}
		fah.recoveryStats["failed_recoveries"] = failed + 1
	}

	// Update last recovery time
	fah.recoveryStats["last_recovery_time"] = time.Now()
	fah.lastRecovery = time.Now()
}

// GetLastRecoveryTime returns the last recovery time
func (fah *ForkAutoHandler) GetLastRecoveryTime() time.Time {
	fah.mu.RLock()
	defer fah.mu.RUnlock()
	return fah.lastRecovery
}

// ForkResolutionStrategy represents the strategy for fork resolution
type ForkResolutionStrategy struct {
	Name           string
	Priority       int
	ApplicableTypes []ForkType
	Description    string
}

// GetDefaultForkResolutionStrategies returns the default fork resolution strategies
func GetDefaultForkResolutionStrategies() []ForkResolutionStrategy {
	return []ForkResolutionStrategy{
		{
			Name:           "Work-Based Resolution",
			Priority:       1,
			ApplicableTypes: []ForkType{ForkTypePersistent, ForkTypeDeep},
			Description:    "Resolve fork based on cumulative work",
		},
		{
			Name:           "Health-Based Resolution",
			Priority:       2,
			ApplicableTypes: []ForkType{ForkTypeSymmetric},
			Description:    "Resolve symmetric forks based on chain health metrics",
		},
		{
			Name:           "Wait for Resolution",
			Priority:       3,
			ApplicableTypes: []ForkType{ForkTypeTemporary},
			Description:    "Wait for temporary forks to resolve automatically",
		},
		{
			Name:           "Manual Intervention",
			Priority:       4,
			ApplicableTypes: []ForkType{ForkTypeSoft, ForkTypeHard},
			Description:    "Require manual intervention for protocol forks",
		},
	}
}
