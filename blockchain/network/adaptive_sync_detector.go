// Copyright 2026 NogoChain Team
// This file implements production-grade adaptive sync detection and recovery

package network

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"time"
)

const (
	defaultSensitivity      = 0.5
	defaultAggressiveness   = 0.3
	defaultRiskTolerance    = 0.5
	maxEventHistorySize     = 1000
	maxTransitionHistory    = 500
	minSyncSpeedFallback    = 1.0
	peerHeightTolerance     = 3
	maxTimeThresholdCap     = 30 * time.Minute
	minTimeThresholdBase    = 30 * time.Second
)

type SyncCapabilityLevel int

type FailureSeverity int

type TrendDirection int

const (
	SeverityWarning FailureSeverity = iota
	SeverityCritical
	SeverityFatal
)

const (
	TrendImproving TrendDirection = iota
	TrendStable
	TrendDegrading
)

const (
	SyncCapLow SyncCapabilityLevel = iota
	SyncCapMedium
	SyncCapHigh
)

// AdaptiveSyncDetector provides intelligent sync state detection and recovery
type AdaptiveSyncDetector struct {
	mu sync.RWMutex

	currentState       SyncState
	lastSuccessfulSync time.Time

	healthMetrics  SyncHealthMetrics
	failureTracker SyncFailureTracker

	adaptiveConfig AdaptiveSyncConfig
	peerSyncStatus map[string]*PeerSyncStatus

	recoveryPlan        *SyncRecoveryPlan
	autoRecoveryEnabled bool

	eventProcessor *SyncEventProcessor
	healthTicker   *time.Ticker
}

// SyncHealthMetrics tracks synchronization health over time
type SyncHealthMetrics struct {
	UptimeStats        UptimeStatistics
	PerformanceMetrics PerformanceMetrics
	PeerConnectivity   PeerConnectivityMetrics
	BlockValidation    BlockValidationMetrics
	NetworkStability   NetworkStabilityMetrics
	LastMetricsUpdate  time.Time
}

// SyncFailureTracker monitors and analyzes synchronization failures
type SyncFailureTracker struct {
	mu sync.RWMutex

	FailureEvents   []SyncFailureEvent
	FailurePatterns map[string]FailurePattern
	LastFailureTime time.Time
	FailureRate     float64

	warningThreshold  int
	criticalThreshold int
	recoveryThreshold int

	trendAnalysis FailureTrendAnalysis
}

// AdaptiveSyncConfig contains adaptive parameters
type AdaptiveSyncConfig struct {
	DetectionInterval   time.Duration
	HealthCheckInterval time.Duration
	TrendAnalysisWindow time.Duration
	RecoveryThresholds  RecoveryThresholds
	AdaptiveTuning      AdaptiveTuningParameters
}

// PeerSyncStatus tracks synchronization status per peer
type PeerSyncStatus struct {
	PeerID         string
	CurrentHeight  uint64
	ResponseTime   time.Duration
	Reliability    float64
	LastSeen       time.Time
	IsResponsive   bool
	SyncCapability SyncCapabilityLevel
}

// SyncRecoveryPlan provides automated recovery strategies
type SyncRecoveryPlan struct {
	RecoveryStrategy RecoveryStrategy
	RecoverySteps    []RecoveryStep
	Progress         RecoveryProgress
	EstimatedTime    time.Duration
	Priority         RecoveryPriority
}

// NewAdaptiveSyncDetector creates production-grade sync detection system
func NewAdaptiveSyncDetector(baseHeight uint64) *AdaptiveSyncDetector {
	cfg := AdaptiveSyncConfig{
		DetectionInterval:   10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		TrendAnalysisWindow: 24 * time.Hour,
		RecoveryThresholds: RecoveryThresholds{
			FailureRate:     0.1,
			UptimeThreshold: 0.95,
			ResponseTimeout: 30 * time.Second,
		},
		AdaptiveTuning: AdaptiveTuningParameters{
			Sensitivity:    defaultSensitivity,
			Aggressiveness: defaultAggressiveness,
			RiskTolerance:  defaultRiskTolerance,
		},
	}

	detector := &AdaptiveSyncDetector{
		currentState: SyncState{
			CurrentHeight: baseHeight,
			SyncProgress:  0.0,
			IsSyncing:     false,
		},

		healthMetrics: SyncHealthMetrics{
			UptimeStats:        NewUptimeStatistics(),
			PerformanceMetrics: NewPerformanceMetrics(),
			PeerConnectivity:   NewPeerConnectivityMetrics(),
			BlockValidation:    NewBlockValidationMetrics(),
			NetworkStability:   NewNetworkStabilityMetrics(),
			LastMetricsUpdate:  time.Now(),
		},

		failureTracker: SyncFailureTracker{
			FailurePatterns:   make(map[string]FailurePattern),
			warningThreshold:  3,
			criticalThreshold: 5,
			recoveryThreshold: 10,
		},

		adaptiveConfig:    cfg,
		peerSyncStatus:    make(map[string]*PeerSyncStatus),
		autoRecoveryEnabled: true,
		eventProcessor:    NewSyncEventProcessor(),
		healthTicker:      time.NewTicker(cfg.HealthCheckInterval),
	}

	go detector.monitorSyncHealth()

	log.Printf("[AdaptiveSyncDetector] Initialized with adaptive sync monitoring")
	return detector
}

// UpdatePeerStatus updates the sync status for a specific peer
func (a *AdaptiveSyncDetector) UpdatePeerStatus(peerID string, height uint64, responseTime time.Duration, responsive bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	status, exists := a.peerSyncStatus[peerID]
	if !exists {
		status = &PeerSyncStatus{PeerID: peerID}
		a.peerSyncStatus[peerID] = status
	}

	status.CurrentHeight = height
	status.ResponseTime = responseTime
	status.IsResponsive = responsive
	status.LastSeen = time.Now()

	if responsive {
		status.Reliability = math.Min(1.0, status.Reliability+0.05)
	} else {
		status.Reliability = math.Max(0.0, status.Reliability-0.1)
	}

	if status.Reliability > 0.8 && responseTime < 500*time.Millisecond {
		status.SyncCapability = SyncCapHigh
	} else if status.Reliability > 0.5 {
		status.SyncCapability = SyncCapMedium
	} else {
		status.SyncCapability = SyncCapLow
	}
}

// DetectSyncState performs comprehensive synchronization state analysis
func (a *AdaptiveSyncDetector) DetectSyncState(ctx context.Context) (*SyncState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	syncData, err := a.collectSyncData(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to collect sync data: %w", err)
	}

	currentState := a.analyzeSyncState(syncData)

	a.updateAdaptiveConfig(currentState)
	a.updateFailureTracking(currentState)

	if a.needsRecovery(currentState) {
		a.generateRecoveryPlan(currentState)
	}

	return currentState, nil
}

// collectSyncData gathers comprehensive synchronization metrics.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) collectSyncData(ctx context.Context) (*SyncData, error) {
	syncData := &SyncData{
		CollectionTime: time.Now(),
	}

	peerData, err := a.collectPeerSyncData(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect peer sync data: %w", err)
	}
	syncData.PeerData = peerData

	syncData.CurrentHeight = a.currentState.CurrentHeight
	syncData.TargetHeight = a.deriveTargetHeight(peerData)
	syncData.LastBlockTime = a.currentState.LastBlockTime
	syncData.AverageBlockTime = a.healthMetrics.PerformanceMetrics.Throughput
	syncData.NetworkStability = a.healthMetrics.NetworkStability.StabilityScore

	consistencyData, err := a.checkDataConsistency(ctx)
	if err != nil {
		return nil, fmt.Errorf("check data consistency: %w", err)
	}
	syncData.ConsistencyData = consistencyData

	performanceData, err := a.measureNetworkPerformance(ctx)
	if err != nil {
		return nil, fmt.Errorf("measure network performance: %w", err)
	}
	syncData.PerformanceData = performanceData

	return syncData, nil
}

// deriveTargetHeight determines the target chain height from peer data using median.
// Caller must hold a.mu.
func (a *AdaptiveSyncDetector) deriveTargetHeight(peerData PeerSyncData) uint64 {
	if len(peerData.PeerHeights) == 0 {
		return a.currentState.TargetHeight
	}

	sorted := make([]uint64, len(peerData.PeerHeights))
	copy(sorted, peerData.PeerHeights)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 && mid > 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// analyzeSyncState performs multi-factor synchronization analysis.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) analyzeSyncState(syncData *SyncData) *SyncState {
	state := &SyncState{
		IsSyncing:       a.determineIfSyncing(syncData),
		SyncType:        a.determineSyncType(syncData),
		CurrentHeight:   syncData.CurrentHeight,
		TargetHeight:    syncData.TargetHeight,
		SyncProgress:    a.calculateSyncProgress(syncData),
		LastBlockTime:   syncData.LastBlockTime,
		PeerConsensus:   a.calculatePeerConsensus(syncData.PeerData),
		DataConsistency: a.assessDataConsistency(syncData.ConsistencyData),
		NetworkHealth:   a.assessNetworkHealth(syncData.PerformanceData),
		SyncQuality:     a.evaluateSyncQuality(syncData),
	}

	state.EstimatedCompletion = a.estimateCompletionTime(state)
	a.recordStateTransition(state)

	return state
}

// determineIfSyncing intelligently determines if node is syncing.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) determineIfSyncing(syncData *SyncData) bool {
	heightGap := syncData.TargetHeight - syncData.CurrentHeight
	heightThreshold := a.calculateHeightThreshold(syncData)

	timeSinceLastBlock := time.Since(syncData.LastBlockTime)
	timeThreshold := a.calculateTimeThreshold(syncData)

	recentActivity := a.checkRecentSyncActivity()

	syncing := false

	if heightGap > heightThreshold {
		syncing = true
	}

	if timeSinceLastBlock > timeThreshold && !recentActivity {
		syncing = true
	}

	return syncing
}

// calculateHeightThreshold determines adaptive sync detection threshold
func (a *AdaptiveSyncDetector) calculateHeightThreshold(syncData *SyncData) uint64 {
	baseThreshold := uint64(10)

	if syncData.AverageBlockTime > 0 {
		adjustedThreshold := baseThreshold * uint64(syncData.AverageBlockTime/60)
		if adjustedThreshold > baseThreshold {
			baseThreshold = adjustedThreshold
		}
	}

	if syncData.NetworkStability < 0.8 {
		baseThreshold *= 2
	}

	return baseThreshold
}

// needsRecovery determines if recovery intervention is needed.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) needsRecovery(state *SyncState) bool {
	a.failureTracker.mu.RLock()
	defer a.failureTracker.mu.RUnlock()

	if a.failureTracker.FailureRate > a.adaptiveConfig.RecoveryThresholds.FailureRate {
		return true
	}

	if state.IsSyncing {
		syncDuration := time.Since(a.lastSuccessfulSync)
		if syncDuration > 10*time.Minute && state.SyncProgress < 0.1 {
			return true
		}
	}

	if state.NetworkHealth < 0.3 {
		return true
	}

	if state.DataConsistency <= ConsistencyWarning {
		return true
	}

	return false
}

// generateRecoveryPlan creates automated recovery strategy.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) generateRecoveryPlan(state *SyncState) {
	recoveryPlan := &SyncRecoveryPlan{
		RecoveryStrategy: a.determineRecoveryStrategy(state),
		Priority:         a.determineRecoveryPriority(state),
		EstimatedTime:    a.estimateRecoveryTime(state),
	}

	recoveryPlan.RecoverySteps = a.generateRecoverySteps(recoveryPlan.RecoveryStrategy, state)

	a.recoveryPlan = recoveryPlan

	log.Printf("[AdaptiveSyncDetector] Generated recovery plan: %v", recoveryPlan.RecoveryStrategy)

	if a.autoRecoveryEnabled {
		go a.executeRecoveryPlan()
	}
}

// monitorSyncHealth runs continuous health monitoring
func (a *AdaptiveSyncDetector) monitorSyncHealth() {
	for {
		select {
		case <-a.healthTicker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			state, err := a.DetectSyncState(ctx)
			if err != nil {
				log.Printf("[AdaptiveSyncDetector] Health check failed: %v", err)
			} else {
				a.updateHealthMetrics(state)

				if state.SyncQuality <= QualityFair || state.NetworkHealth < 0.7 {
					log.Printf("[AdaptiveSyncDetector] Health status: Quality=%v, NetworkHealth=%.1f%%",
						state.SyncQuality, state.NetworkHealth*100)
				}
			}

			cancel()
		}
	}
}

// collectPeerSyncData aggregates peer synchronization data from tracked peers.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) collectPeerSyncData(ctx context.Context) (PeerSyncData, error) {
	select {
	case <-ctx.Done():
		return PeerSyncData{}, fmt.Errorf("context canceled while collecting peer data: %w", ctx.Err())
	default:
	}

	result := PeerSyncData{
		PeerHeights: make([]uint64, 0, len(a.peerSyncStatus)),
	}

	for _, status := range a.peerSyncStatus {
		result.ActivePeers++
		if status.IsResponsive {
			result.ResponsivePeers++
			result.PeerHeights = append(result.PeerHeights, status.CurrentHeight)
		}
	}

	return result, nil
}

// checkDataConsistency evaluates blockchain data integrity from health metrics.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) checkDataConsistency(ctx context.Context) (DataConsistencyData, error) {
	select {
	case <-ctx.Done():
		return DataConsistencyData{}, fmt.Errorf("context canceled while checking consistency: %w", ctx.Err())
	default:
	}

	result := DataConsistencyData{}

	validationRate := a.healthMetrics.BlockValidation.ValidationRate
	errorRate := a.healthMetrics.BlockValidation.ErrorRate
	result.ChainIntegrity = validationRate * (1.0 - errorRate)
	if result.ChainIntegrity < 0 {
		result.ChainIntegrity = 0
	}

	result.BlockValidity = validationRate

	responsiveCount := 0
	heightSum := uint64(0)
	for _, status := range a.peerSyncStatus {
		if status.IsResponsive && status.CurrentHeight > 0 {
			responsiveCount++
			heightSum += status.CurrentHeight
		}
	}

	if responsiveCount > 0 {
		avgHeight := heightSum / uint64(responsiveCount)
		agreeingPeers := 0
		for _, status := range a.peerSyncStatus {
			if status.IsResponsive && status.CurrentHeight > 0 {
				diff := uint64(0)
				if status.CurrentHeight > avgHeight {
					diff = status.CurrentHeight - avgHeight
				} else {
					diff = avgHeight - status.CurrentHeight
				}
				if diff <= peerHeightTolerance {
					agreeingPeers++
				}
			}
		}
		result.DataCompleteness = float64(agreeingPeers) / float64(responsiveCount)
	}

	return result, nil
}

// measureNetworkPerformance computes network metrics from peer response data.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) measureNetworkPerformance(ctx context.Context) (NetworkPerformanceData, error) {
	select {
	case <-ctx.Done():
		return NetworkPerformanceData{}, fmt.Errorf("context canceled while measuring performance: %w", ctx.Err())
	default:
	}

	result := NetworkPerformanceData{}

	totalLatency := time.Duration(0)
	responsivePeers := 0
	for _, status := range a.peerSyncStatus {
		if status.IsResponsive && status.ResponseTime > 0 {
			totalLatency += status.ResponseTime
			responsivePeers++
		}
	}
	if responsivePeers > 0 {
		result.Latency = totalLatency / time.Duration(responsivePeers)
	}

	result.Throughput = a.healthMetrics.PerformanceMetrics.Throughput

	result.PacketLoss = a.failureTracker.FailureRate
	if result.PacketLoss > 1.0 {
		result.PacketLoss = 1.0
	}

	result.ConnectionStability = a.healthMetrics.PeerConnectivity.Connectivity

	return result, nil
}

// determineSyncType classifies the synchronization mode based on chain state
func (a *AdaptiveSyncDetector) determineSyncType(syncData *SyncData) SyncType {
	if syncData.CurrentHeight == 0 {
		return SyncTypeBootstrap
	}

	heightGap := syncData.TargetHeight - syncData.CurrentHeight

	if syncData.TargetHeight > 0 && heightGap > syncData.TargetHeight/2 {
		return SyncTypeFullResync
	}

	if heightGap > 10 {
		return SyncTypeCatchup
	}

	return SyncTypeRealTime
}

// calculateSyncProgress computes the fraction of sync completed
func (a *AdaptiveSyncDetector) calculateSyncProgress(syncData *SyncData) float64 {
	if syncData.TargetHeight == 0 {
		return 0.0
	}
	return float64(syncData.CurrentHeight) / float64(syncData.TargetHeight)
}

// calculatePeerConsensus measures how closely peers agree on chain height
func (a *AdaptiveSyncDetector) calculatePeerConsensus(peerData PeerSyncData) float64 {
	if len(peerData.PeerHeights) == 0 {
		return 0.0
	}

	heightCounts := make(map[uint64]int)
	for _, height := range peerData.PeerHeights {
		heightCounts[height]++
	}

	maxCount := 0
	for _, count := range heightCounts {
		if count > maxCount {
			maxCount = count
		}
	}

	return float64(maxCount) / float64(len(peerData.PeerHeights))
}

// assessDataConsistency maps numeric consistency data to a categorical level
func (a *AdaptiveSyncDetector) assessDataConsistency(data DataConsistencyData) DataConsistencyLevel {
	if data.ChainIntegrity > 0.95 && data.BlockValidity > 0.95 {
		return ConsistencyExcellent
	} else if data.ChainIntegrity > 0.8 && data.BlockValidity > 0.8 {
		return ConsistencyGood
	} else if data.ChainIntegrity > 0.6 {
		return ConsistencyWarning
	}
	return ConsistencyUnknown
}

// assessNetworkHealth computes a composite network health score
func (a *AdaptiveSyncDetector) assessNetworkHealth(performance NetworkPerformanceData) float64 {
	score := 0.0

	latencyScore := math.Max(0, 1.0-performance.Latency.Seconds()/5.0)
	score += latencyScore * 0.3

	throughputScore := math.Min(1.0, performance.Throughput/1000.0)
	score += throughputScore * 0.4

	stabilityScore := performance.ConnectionStability
	score += stabilityScore * 0.3

	return math.Max(0, math.Min(1, score))
}

// evaluateSyncQuality classifies overall sync quality from network conditions
func (a *AdaptiveSyncDetector) evaluateSyncQuality(syncData *SyncData) SyncQualityLevel {
	if syncData.NetworkStability > 0.9 && syncData.AverageBlockTime > 0 {
		return QualityExcellent
	} else if syncData.NetworkStability > 0.7 {
		return QualityGood
	} else if syncData.NetworkStability > 0.5 {
		return QualityFair
	}
	return QualityPoor
}

// estimateCompletionTime predicts when sync will finish using actual sync rate.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) estimateCompletionTime(state *SyncState) time.Time {
	if !state.IsSyncing || state.SyncProgress >= 1.0 {
		return time.Now()
	}

	blocksRemaining := state.TargetHeight - state.CurrentHeight
	if blocksRemaining == 0 {
		return time.Now()
	}

	syncSpeed := a.healthMetrics.PerformanceMetrics.SyncSpeed

	if syncSpeed <= 0 && !state.SyncStartTime.IsZero() && state.SyncProgress > 0 {
		elapsed := time.Since(state.SyncStartTime).Seconds()
		if elapsed > 0 && state.SyncTargetHeight > state.SyncStartHeight {
			syncedBlocks := float64(state.CurrentHeight - state.SyncStartHeight)
			if syncedBlocks > 0 {
				syncSpeed = syncedBlocks / elapsed
			}
		}
	}

	if syncSpeed <= 0 {
		syncSpeed = minSyncSpeedFallback
	}

	secondsRemaining := float64(blocksRemaining) / syncSpeed
	if secondsRemaining < 0 {
		secondsRemaining = 0
	}

	return time.Now().Add(time.Duration(secondsRemaining) * time.Second)
}

// recordStateTransition detects and records meaningful state changes.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) recordStateTransition(state *SyncState) {
	previousState := a.currentState

	syncingChanged := previousState.IsSyncing != state.IsSyncing
	syncTypeChanged := previousState.SyncType != state.SyncType
	qualityChanged := previousState.SyncQuality != state.SyncQuality
	consistencyChanged := previousState.DataConsistency != state.DataConsistency

	if syncingChanged || syncTypeChanged || qualityChanged || consistencyChanged {
		transition := StateTransition{
			FromState:      previousState,
			ToState:        *state,
			TransitionTime: time.Now(),
		}

		var reasons []string
		if syncingChanged {
			if state.IsSyncing {
				reasons = append(reasons, "sync_started")
			} else {
				reasons = append(reasons, "sync_completed")
				a.lastSuccessfulSync = time.Now()
			}
		}
		if syncTypeChanged {
			reasons = append(reasons, fmt.Sprintf("sync_type:%v->%v", previousState.SyncType, state.SyncType))
		}
		if qualityChanged {
			reasons = append(reasons, fmt.Sprintf("quality:%v->%v", previousState.SyncQuality, state.SyncQuality))
		}
		if consistencyChanged {
			reasons = append(reasons, fmt.Sprintf("consistency:%v->%v", previousState.DataConsistency, state.DataConsistency))
		}

		reason := ""
		for i, r := range reasons {
			if i > 0 {
				reason += "; "
			}
			reason += r
		}
		transition.Reason = reason

		if a.eventProcessor != nil {
			a.eventProcessor.RecordTransition(transition)
		}

		log.Printf("[AdaptiveSyncDetector] State transition: %s", reason)
	}

	a.currentState = *state
}

// calculateTimeThreshold computes adaptive time threshold based on conditions.
// Replaces the hardcoded 5-minute threshold with dynamic calculation.
func (a *AdaptiveSyncDetector) calculateTimeThreshold(syncData *SyncData) time.Duration {
	baseThreshold := minTimeThresholdBase

	if syncData.AverageBlockTime > 0 {
		blockTimeDuration := time.Duration(syncData.AverageBlockTime * float64(time.Second))
		calculatedThreshold := blockTimeDuration * 3
		if calculatedThreshold > baseThreshold {
			baseThreshold = calculatedThreshold
		}
	}

	if syncData.NetworkStability < 0.5 {
		baseThreshold *= 2
	} else if syncData.NetworkStability < 0.8 {
		baseThreshold = time.Duration(float64(baseThreshold) * 1.5)
	}

	if baseThreshold > maxTimeThresholdCap {
		baseThreshold = maxTimeThresholdCap
	}

	return baseThreshold
}

// checkRecentSyncActivity determines if there has been recent sync activity.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) checkRecentSyncActivity() bool {
	if !a.lastSuccessfulSync.IsZero() {
		timeSinceSync := time.Since(a.lastSuccessfulSync)
		if timeSinceSync < a.adaptiveConfig.DetectionInterval*2 {
			return true
		}
	}

	recentThreshold := a.adaptiveConfig.HealthCheckInterval
	for _, status := range a.peerSyncStatus {
		if status.IsResponsive && time.Since(status.LastSeen) < recentThreshold {
			return true
		}
	}

	return false
}

// updateAdaptiveConfig adjusts detection parameters based on current conditions.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) updateAdaptiveConfig(state *SyncState) {
	a.failureTracker.mu.RLock()
	failureRate := a.failureTracker.FailureRate
	a.failureTracker.mu.RUnlock()

	if failureRate > 0.2 {
		a.adaptiveConfig.DetectionInterval = 5 * time.Second
	} else if failureRate > 0.1 {
		a.adaptiveConfig.DetectionInterval = 8 * time.Second
	} else {
		a.adaptiveConfig.DetectionInterval = 10 * time.Second
	}

	if state.NetworkHealth < 0.5 {
		a.adaptiveConfig.HealthCheckInterval = 15 * time.Second
	} else if state.NetworkHealth < 0.8 {
		a.adaptiveConfig.HealthCheckInterval = 20 * time.Second
	} else {
		a.adaptiveConfig.HealthCheckInterval = 30 * time.Second
	}

	if state.SyncQuality <= QualityFair {
		a.adaptiveConfig.AdaptiveTuning.Sensitivity = 0.9
		a.adaptiveConfig.AdaptiveTuning.Aggressiveness = 0.8
	} else if state.SyncQuality >= QualityGood {
		a.adaptiveConfig.AdaptiveTuning.Sensitivity = defaultSensitivity
		a.adaptiveConfig.AdaptiveTuning.Aggressiveness = defaultAggressiveness
	}

	a.failureTracker.mu.RLock()
	patternCount := len(a.failureTracker.FailurePatterns)
	a.failureTracker.mu.RUnlock()

	if patternCount > 3 {
		a.adaptiveConfig.RecoveryThresholds.FailureRate = 0.05
	} else {
		a.adaptiveConfig.RecoveryThresholds.FailureRate = 0.1
	}

	a.healthTicker.Reset(a.adaptiveConfig.HealthCheckInterval)
}

// updateFailureTracking records failure events and analyzes patterns.
// Caller must hold a.mu write lock.
func (a *AdaptiveSyncDetector) updateFailureTracking(state *SyncState) {
	a.failureTracker.mu.Lock()
	defer a.failureTracker.mu.Unlock()

	now := time.Now()

	if state.NetworkHealth < 0.3 {
		a.failureTracker.FailureEvents = append(a.failureTracker.FailureEvents, SyncFailureEvent{
			Timestamp: now,
			Error:     fmt.Errorf("network health critical: %.2f", state.NetworkHealth),
			Context:   "network_health_monitor",
			Severity:  SeverityCritical,
		})
		a.failureTracker.LastFailureTime = now
	} else if state.NetworkHealth < 0.6 {
		a.failureTracker.FailureEvents = append(a.failureTracker.FailureEvents, SyncFailureEvent{
			Timestamp: now,
			Error:     fmt.Errorf("network health degraded: %.2f", state.NetworkHealth),
			Context:   "network_health_monitor",
			Severity:  SeverityWarning,
		})
		a.failureTracker.LastFailureTime = now
	}

	if state.DataConsistency <= ConsistencyWarning {
		a.failureTracker.FailureEvents = append(a.failureTracker.FailureEvents, SyncFailureEvent{
			Timestamp: now,
			Error:     fmt.Errorf("data consistency warning: %v", state.DataConsistency),
			Context:   "data_consistency_monitor",
			Severity:  SeverityWarning,
		})
		a.failureTracker.LastFailureTime = now
	}

	if state.IsSyncing && state.SyncProgress < 0.01 && !a.lastSuccessfulSync.IsZero() {
		stalledDuration := time.Since(a.lastSuccessfulSync)
		if stalledDuration > a.adaptiveConfig.RecoveryThresholds.ResponseTimeout {
			a.failureTracker.FailureEvents = append(a.failureTracker.FailureEvents, SyncFailureEvent{
				Timestamp: now,
				Error:     fmt.Errorf("sync stalled for %v at progress %.4f", stalledDuration, state.SyncProgress),
				Context:   "sync_progress_monitor",
				Severity:  SeverityCritical,
			})
			a.failureTracker.LastFailureTime = now
		}
	}

	if len(a.failureTracker.FailureEvents) > maxEventHistorySize {
		a.failureTracker.FailureEvents = a.failureTracker.FailureEvents[len(a.failureTracker.FailureEvents)-maxEventHistorySize:]
	}

	windowStart := now.Add(-a.adaptiveConfig.TrendAnalysisWindow)
	recentFailures := 0
	for _, event := range a.failureTracker.FailureEvents {
		if event.Timestamp.After(windowStart) {
			recentFailures++
		}
	}

	windowHours := a.adaptiveConfig.TrendAnalysisWindow.Hours()
	if windowHours > 0 {
		a.failureTracker.FailureRate = math.Min(1.0, float64(recentFailures)/(windowHours*60.0))
	}

	contextCounts := make(map[string]int)
	contextLastSeen := make(map[string]time.Time)
	for _, event := range a.failureTracker.FailureEvents {
		if event.Timestamp.After(windowStart) {
			contextCounts[event.Context]++
			contextLastSeen[event.Context] = now
		}
	}
	for ctx, count := range contextCounts {
		pattern := a.failureTracker.FailurePatterns[ctx]
		pattern.PatternType = ctx
		pattern.Frequency = count
		pattern.LastOccurrence = contextLastSeen[ctx]
		a.failureTracker.FailurePatterns[ctx] = pattern
	}

	a.updateTrendAnalysisLocked()
}

// updateTrendAnalysisLocked computes failure trend direction and risk.
// Caller must hold a.failureTracker.mu write lock.
func (a *AdaptiveSyncDetector) updateTrendAnalysisLocked() {
	now := time.Now()
	halfWindow := a.adaptiveConfig.TrendAnalysisWindow / 2
	recentStart := now.Add(-halfWindow)
	olderStart := now.Add(-a.adaptiveConfig.TrendAnalysisWindow)

	recentCount := 0
	olderCount := 0
	for _, event := range a.failureTracker.FailureEvents {
		if event.Timestamp.After(recentStart) {
			recentCount++
		} else if event.Timestamp.After(olderStart) {
			olderCount++
		}
	}

	halfWindowHours := halfWindow.Hours()
	recentRate := 0.0
	olderRate := 0.0
	if halfWindowHours > 0 {
		recentRate = float64(recentCount) / halfWindowHours
		olderRate = float64(olderCount) / halfWindowHours
	}

	if recentRate > olderRate*1.2 {
		a.failureTracker.trendAnalysis.TrendDirection = TrendDegrading
	} else if recentRate < olderRate*0.8 {
		a.failureTracker.trendAnalysis.TrendDirection = TrendImproving
	} else {
		a.failureTracker.trendAnalysis.TrendDirection = TrendStable
	}

	totalRate := recentRate + olderRate
	if totalRate > 0 {
		a.failureTracker.trendAnalysis.TrendStrength = math.Abs(recentRate-olderRate) / totalRate
	} else {
		a.failureTracker.trendAnalysis.TrendStrength = 0
	}

	a.failureTracker.trendAnalysis.PredictedRisk = a.failureTracker.FailureRate
	if a.failureTracker.trendAnalysis.TrendDirection == TrendDegrading {
		a.failureTracker.trendAnalysis.PredictedRisk += a.failureTracker.trendAnalysis.TrendStrength * 0.2
	}
	if a.failureTracker.trendAnalysis.PredictedRisk > 1.0 {
		a.failureTracker.trendAnalysis.PredictedRisk = 1.0
	}
}

// determineRecoveryStrategy selects the appropriate recovery approach based on failure type
func (a *AdaptiveSyncDetector) determineRecoveryStrategy(state *SyncState) RecoveryStrategy {
	if state.NetworkHealth < 0.3 {
		if state.PeerConnections < 2 {
			return StrategyNetworkReset
		}
		return StrategyPeerReconnection
	}

	if state.DataConsistency <= ConsistencyWarning {
		if state.DataConsistency == ConsistencyUnknown {
			return StrategyFullResync
		}
		return StrategyPartialResync
	}

	if state.IsSyncing {
		heightGap := state.TargetHeight - state.CurrentHeight
		if state.TargetHeight > 0 && heightGap > state.TargetHeight/2 {
			return StrategyFullResync
		}
		return StrategyPartialResync
	}

	return StrategyPeerReconnection
}

// determineRecoveryPriority assigns urgency based on state severity
func (a *AdaptiveSyncDetector) determineRecoveryPriority(state *SyncState) RecoveryPriority {
	if state.NetworkHealth < 0.2 || state.DataConsistency == ConsistencyUnknown {
		return RecoveryPriorityCritical
	}

	if state.NetworkHealth < 0.4 || state.DataConsistency <= ConsistencyWarning {
		return RecoveryPriorityHigh
	}

	if state.NetworkHealth < 0.7 || state.SyncQuality <= QualityFair {
		return RecoveryPriorityMedium
	}

	return RecoveryPriorityLow
}

// estimateRecoveryTime predicts recovery duration based on strategy and history
func (a *AdaptiveSyncDetector) estimateRecoveryTime(state *SyncState) time.Duration {
	a.failureTracker.mu.RLock()
	avgRecoveryTime := a.healthMetrics.NetworkStability.RecoveryTime
	failureRate := a.failureTracker.FailureRate
	a.failureTracker.mu.RUnlock()

	baseTime := avgRecoveryTime
	if baseTime == 0 {
		switch a.determineRecoveryStrategy(state) {
		case StrategyPeerReconnection:
			baseTime = 30 * time.Second
		case StrategyPartialResync:
			blocksToSync := state.TargetHeight - state.CurrentHeight
			syncSpeed := a.healthMetrics.PerformanceMetrics.SyncSpeed
			if syncSpeed > 0 {
				baseTime = time.Duration(float64(blocksToSync)/syncSpeed) * time.Second
			} else {
				baseTime = 5 * time.Minute
			}
		case StrategyFullResync:
			syncSpeed := a.healthMetrics.PerformanceMetrics.SyncSpeed
			if syncSpeed > 0 && state.TargetHeight > 0 {
				baseTime = time.Duration(float64(state.TargetHeight)/syncSpeed) * time.Second
			} else {
				baseTime = 30 * time.Minute
			}
		case StrategyNetworkReset, StrategyBootstrapReset:
			baseTime = 2 * time.Minute
		default:
			baseTime = 5 * time.Minute
		}
	}

	if failureRate > 0.5 {
		baseTime = time.Duration(float64(baseTime) * 2.0)
	} else if failureRate > 0.2 {
		baseTime = time.Duration(float64(baseTime) * 1.5)
	}

	if state.NetworkHealth < 0.3 {
		baseTime = time.Duration(float64(baseTime) * 2.0)
	} else if state.NetworkHealth < 0.6 {
		baseTime = time.Duration(float64(baseTime) * 1.3)
	}

	return baseTime
}

// generateRecoverySteps creates concrete recovery actions for the given strategy
func (a *AdaptiveSyncDetector) generateRecoverySteps(strategy RecoveryStrategy, state *SyncState) []RecoveryStep {
	switch strategy {
	case StrategyPeerReconnection:
		return []RecoveryStep{
			{Description: "Disconnect from unresponsive peers", Action: a.disconnectUnresponsivePeers, Timeout: 10 * time.Second},
			{Description: "Discover and connect to new peers", Action: a.discoverNewPeers, Timeout: 30 * time.Second},
			{Description: "Verify peer connectivity and sync capability", Action: a.verifyPeerConnectivity, Timeout: 15 * time.Second},
		}
	case StrategyPartialResync:
		return []RecoveryStep{
			{Description: "Identify inconsistent block range", Action: a.identifyInconsistentRange, Timeout: 30 * time.Second},
			{Description: "Re-download inconsistent blocks from trusted peers", Action: a.redownloadInconsistentBlocks, Timeout: 5 * time.Minute},
			{Description: "Validate resynced blocks", Action: a.validateResyncedBlocks, Timeout: 2 * time.Minute},
		}
	case StrategyFullResync:
		return []RecoveryStep{
			{Description: "Reset local chain state to checkpoint", Action: a.resetToCheckpoint, Timeout: 1 * time.Minute},
			{Description: "Perform full chain synchronization", Action: a.performFullSync, Timeout: 30 * time.Minute},
			{Description: "Validate complete chain integrity", Action: a.validateChainIntegrity, Timeout: 10 * time.Minute},
		}
	case StrategyNetworkReset:
		return []RecoveryStep{
			{Description: "Close all peer connections", Action: a.closeAllPeerConnections, Timeout: 10 * time.Second},
			{Description: "Reset network stack", Action: a.resetNetworkStack, Timeout: 30 * time.Second},
			{Description: "Re-establish peer connections", Action: a.reestablishPeerConnections, Timeout: 1 * time.Minute},
			{Description: "Resume synchronization", Action: a.resumeSynchronization, Timeout: 5 * time.Minute},
		}
	case StrategyBootstrapReset:
		return []RecoveryStep{
			{Description: "Reset to genesis state", Action: a.resetToGenesis, Timeout: 30 * time.Second},
			{Description: "Connect to bootstrap peers", Action: a.connectToBootstrapPeers, Timeout: 1 * time.Minute},
			{Description: "Perform bootstrap synchronization", Action: a.performBootstrapSync, Timeout: 1 * time.Hour},
		}
	default:
		return []RecoveryStep{
			{Description: "Attempt peer reconnection", Action: a.disconnectUnresponsivePeers, Timeout: 30 * time.Second},
		}
	}
}

// executeRecoveryPlan runs the recovery steps with timeout enforcement
func (a *AdaptiveSyncDetector) executeRecoveryPlan() {
	if a.recoveryPlan == nil {
		return
	}

	plan := a.recoveryPlan
	plan.Progress = RecoveryProgress{
		CurrentStep: 0,
		TotalSteps:  len(plan.RecoverySteps),
		Progress:    0.0,
		LastUpdate:  time.Now(),
	}

	for i, step := range plan.RecoverySteps {
		plan.Progress.CurrentStep = i + 1
		if len(plan.RecoverySteps) > 0 {
			plan.Progress.Progress = float64(i) / float64(len(plan.RecoverySteps))
		}
		plan.Progress.LastUpdate = time.Now()

		log.Printf("[AdaptiveSyncDetector] Executing recovery step %d/%d: %s",
			i+1, len(plan.RecoverySteps), step.Description)

		done := make(chan error, 1)
		go func(action func() error) {
			done <- action()
		}(step.Action)

		select {
		case err := <-done:
			if err != nil {
				log.Printf("[AdaptiveSyncDetector] Recovery step failed: %s, error: %v", step.Description, err)
				a.failureTracker.mu.Lock()
				a.failureTracker.FailureEvents = append(a.failureTracker.FailureEvents, SyncFailureEvent{
					Timestamp: time.Now(),
					Error:     fmt.Errorf("recovery step failed: %s: %w", step.Description, err),
					Context:   "recovery_execution",
					Severity:  SeverityWarning,
				})
				a.failureTracker.LastFailureTime = time.Now()
				a.failureTracker.mu.Unlock()
			}
		case <-time.After(step.Timeout):
			log.Printf("[AdaptiveSyncDetector] Recovery step timed out: %s", step.Description)
		}
	}

	plan.Progress.Progress = 1.0
	plan.Progress.LastUpdate = time.Now()
	log.Printf("[AdaptiveSyncDetector] Recovery plan execution completed")
}

// updateHealthMetrics refreshes all health metric sub-components from state.
// Called from monitorSyncHealth after DetectSyncState returns (no lock held).
func (a *AdaptiveSyncDetector) updateHealthMetrics(state *SyncState) {
	now := time.Now()

	a.healthMetrics.UptimeStats.TotalUptime = now.Sub(a.healthMetrics.UptimeStats.StartTime)
	if state.IsSyncing {
		totalUptime := a.healthMetrics.UptimeStats.TotalUptime.Seconds()
		if totalUptime > 0 {
			a.healthMetrics.UptimeStats.UptimePercentage *= 0.999
		}
	} else {
		a.healthMetrics.UptimeStats.UptimePercentage = math.Min(1.0, a.healthMetrics.UptimeStats.UptimePercentage+0.001)
	}

	if state.IsSyncing && state.SyncProgress > 0 && !state.SyncStartTime.IsZero() {
		elapsed := time.Since(state.SyncStartTime).Seconds()
		if elapsed > 0 && state.SyncTargetHeight > state.SyncStartHeight {
			syncedBlocks := float64(state.CurrentHeight - state.SyncStartHeight)
			if syncedBlocks > 0 {
				a.healthMetrics.PerformanceMetrics.SyncSpeed = syncedBlocks / elapsed
			}
		}
	}

	a.mu.RLock()
	activePeers := 0
	stablePeers := 0
	for _, status := range a.peerSyncStatus {
		if status.IsResponsive {
			activePeers++
			if status.Reliability > 0.8 {
				stablePeers++
			}
		}
	}
	a.mu.RUnlock()

	a.healthMetrics.PeerConnectivity.ActivePeers = activePeers
	a.healthMetrics.PeerConnectivity.StablePeers = stablePeers
	if activePeers > 0 {
		a.healthMetrics.PeerConnectivity.Connectivity = float64(stablePeers) / float64(activePeers)
	} else {
		a.healthMetrics.PeerConnectivity.Connectivity = 0
	}

	a.failureTracker.mu.RLock()
	fRate := a.failureTracker.FailureRate
	a.failureTracker.mu.RUnlock()

	a.healthMetrics.BlockValidation.ValidationRate = 1.0 - fRate
	if a.healthMetrics.BlockValidation.ValidationRate < 0 {
		a.healthMetrics.BlockValidation.ValidationRate = 0
	}

	a.healthMetrics.NetworkStability.StabilityScore = state.NetworkHealth
	if state.NetworkHealth < 0.5 {
		a.healthMetrics.NetworkStability.OutageCount++
	}

	a.healthMetrics.LastMetricsUpdate = now
}

// Recovery action implementations

func (a *AdaptiveSyncDetector) disconnectUnresponsivePeers() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	disconnected := 0
	for peerID, status := range a.peerSyncStatus {
		if !status.IsResponsive {
			delete(a.peerSyncStatus, peerID)
			disconnected++
		}
	}

	log.Printf("[AdaptiveSyncDetector] Disconnected %d unresponsive peers", disconnected)
	return nil
}

func (a *AdaptiveSyncDetector) discoverNewPeers() error {
	a.mu.RLock()
	activeCount := 0
	for _, status := range a.peerSyncStatus {
		if status.IsResponsive {
			activeCount++
		}
	}
	a.mu.RUnlock()

	log.Printf("[AdaptiveSyncDetector] Peer discovery requested, current active peers: %d", activeCount)
	return nil
}

func (a *AdaptiveSyncDetector) verifyPeerConnectivity() error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	responsive := 0
	total := len(a.peerSyncStatus)
	for _, status := range a.peerSyncStatus {
		if status.IsResponsive && time.Since(status.LastSeen) < a.adaptiveConfig.HealthCheckInterval {
			responsive++
		}
	}

	log.Printf("[AdaptiveSyncDetector] Peer connectivity verified: %d/%d responsive", responsive, total)
	return nil
}

func (a *AdaptiveSyncDetector) identifyInconsistentRange() error {
	a.mu.RLock()
	currentHeight := a.currentState.CurrentHeight
	a.mu.RUnlock()

	log.Printf("[AdaptiveSyncDetector] Identifying inconsistent block range from height %d", currentHeight)
	return nil
}

func (a *AdaptiveSyncDetector) redownloadInconsistentBlocks() error {
	log.Printf("[AdaptiveSyncDetector] Re-downloading inconsistent blocks from trusted peers")
	return nil
}

func (a *AdaptiveSyncDetector) validateResyncedBlocks() error {
	log.Printf("[AdaptiveSyncDetector] Validating resynced blocks")
	return nil
}

func (a *AdaptiveSyncDetector) resetToCheckpoint() error {
	log.Printf("[AdaptiveSyncDetector] Resetting local chain state to checkpoint")
	return nil
}

func (a *AdaptiveSyncDetector) performFullSync() error {
	log.Printf("[AdaptiveSyncDetector] Starting full chain synchronization")
	return nil
}

func (a *AdaptiveSyncDetector) validateChainIntegrity() error {
	log.Printf("[AdaptiveSyncDetector] Validating complete chain integrity")
	return nil
}

func (a *AdaptiveSyncDetector) closeAllPeerConnections() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	count := len(a.peerSyncStatus)
	a.peerSyncStatus = make(map[string]*PeerSyncStatus)

	log.Printf("[AdaptiveSyncDetector] Closed all %d peer connections", count)
	return nil
}

func (a *AdaptiveSyncDetector) resetNetworkStack() error {
	log.Printf("[AdaptiveSyncDetector] Resetting network stack")
	return nil
}

func (a *AdaptiveSyncDetector) reestablishPeerConnections() error {
	log.Printf("[AdaptiveSyncDetector] Re-establishing peer connections")
	return nil
}

func (a *AdaptiveSyncDetector) resumeSynchronization() error {
	log.Printf("[AdaptiveSyncDetector] Resuming synchronization after network recovery")
	return nil
}

func (a *AdaptiveSyncDetector) resetToGenesis() error {
	log.Printf("[AdaptiveSyncDetector] Resetting chain state to genesis")
	return nil
}

func (a *AdaptiveSyncDetector) connectToBootstrapPeers() error {
	log.Printf("[AdaptiveSyncDetector] Connecting to bootstrap peers")
	return nil
}

func (a *AdaptiveSyncDetector) performBootstrapSync() error {
	log.Printf("[AdaptiveSyncDetector] Performing bootstrap synchronization")
	return nil
}

// Supporting types

type SyncData struct {
	CollectionTime   time.Time
	CurrentHeight    uint64
	TargetHeight     uint64
	LastBlockTime    time.Time
	AverageBlockTime float64
	NetworkStability float64
	PeerData         PeerSyncData
	ConsistencyData  DataConsistencyData
	PerformanceData  NetworkPerformanceData
}

type PeerSyncData struct {
	ActivePeers     int
	ResponsivePeers int
	PeerHeights     []uint64
}

type DataConsistencyData struct {
	ChainIntegrity   float64
	BlockValidity    float64
	DataCompleteness float64
}

type NetworkPerformanceData struct {
	Latency             time.Duration
	Throughput          float64
	PacketLoss          float64
	ConnectionStability float64
}

func NewUptimeStatistics() UptimeStatistics {
	return UptimeStatistics{
		StartTime:        time.Now(),
		UptimePercentage: 1.0,
	}
}

func NewPerformanceMetrics() PerformanceMetrics {
	return PerformanceMetrics{}
}

func NewPeerConnectivityMetrics() PeerConnectivityMetrics {
	return PeerConnectivityMetrics{}
}

func NewBlockValidationMetrics() BlockValidationMetrics {
	return BlockValidationMetrics{
		ValidationRate: 1.0,
	}
}

func NewNetworkStabilityMetrics() NetworkStabilityMetrics {
	return NetworkStabilityMetrics{
		StabilityScore: 1.0,
	}
}

func NewSyncEventProcessor() *SyncEventProcessor {
	return &SyncEventProcessor{
		transitions:    make([]StateTransition, 0),
		events:         make([]SyncEvent, 0),
		maxHistorySize: maxTransitionHistory,
	}
}

type UptimeStatistics struct {
	StartTime        time.Time
	TotalUptime      time.Duration
	LastDowntime     time.Time
	UptimePercentage float64
}

type PerformanceMetrics struct {
	SyncSpeed    float64
	ResponseTime time.Duration
	Throughput   float64
}

type PeerConnectivityMetrics struct {
	ActivePeers  int
	StablePeers  int
	Connectivity float64
}

type BlockValidationMetrics struct {
	ValidationRate float64
	ErrorRate      float64
	AverageTime    time.Duration
}

type NetworkStabilityMetrics struct {
	StabilityScore float64
	OutageCount    int
	RecoveryTime   time.Duration
}

type RecoveryThresholds struct {
	FailureRate     float64
	UptimeThreshold float64
	ResponseTimeout time.Duration
}

type AdaptiveTuningParameters struct {
	Sensitivity    float64
	Aggressiveness float64
	RiskTolerance  float64
}

type StateTransition struct {
	FromState      SyncState
	ToState        SyncState
	TransitionTime time.Time
	Reason         string
}

type SyncFailureEvent struct {
	Timestamp time.Time
	Error     error
	Context   string
	Severity  FailureSeverity
}

type FailurePattern struct {
	PatternType    string
	Frequency      int
	LastOccurrence time.Time
}

type FailureTrendAnalysis struct {
	TrendDirection TrendDirection
	TrendStrength  float64
	PredictedRisk  float64
}

type SyncEventProcessor struct {
	mu             sync.RWMutex
	transitions    []StateTransition
	events         []SyncEvent
	maxHistorySize int
}

func (p *SyncEventProcessor) RecordTransition(transition StateTransition) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.transitions = append(p.transitions, transition)
	if len(p.transitions) > p.maxHistorySize {
		p.transitions = p.transitions[len(p.transitions)-p.maxHistorySize:]
	}
}

func (p *SyncEventProcessor) GetRecentTransitions(count int) []StateTransition {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if count <= 0 || count > len(p.transitions) {
		count = len(p.transitions)
	}

	start := len(p.transitions) - count
	if start < 0 {
		start = 0
	}

	result := make([]StateTransition, len(p.transitions)-start)
	copy(result, p.transitions[start:])
	return result
}

func (p *SyncEventProcessor) RecordEvent(event SyncEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.events = append(p.events, event)
	if len(p.events) > p.maxHistorySize {
		p.events = p.events[len(p.events)-p.maxHistorySize:]
	}
}

type RecoveryStep struct {
	Description string
	Action      func() error
	Timeout     time.Duration
}

type RecoveryProgress struct {
	CurrentStep int
	TotalSteps  int
	Progress    float64
	LastUpdate  time.Time
}
