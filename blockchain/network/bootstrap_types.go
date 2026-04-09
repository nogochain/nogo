// Copyright 2026 NogoChain Team
// This file defines shared types for bootstrap node optimization

package network

import (
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// === Shared Priority Level Definitions ===

// PriorityLevel defines propagation priority for different block types
type PriorityLevel int

const (
	PriorityLow      PriorityLevel = iota // Historical blocks during sync
	PriorityNormal                        // Normal blocks from peers
	PriorityHigh                          // Bootstrap node's own blocks
	PriorityCritical                      // Critical system messages
)

// === Bootstrap Mining Coordination Types ===

// MiningState represents the mining operation state
type MiningState struct {
	IsMining         bool
	LastMinedTime    time.Time
	MiningEfficiency float64
	ActiveDifficulty uint64
	HashRateEstimate float64
	LastBlockMined   uint64    // Height of last successfully mined block
	TotalBlocksMined uint64    // Total number of blocks mined by this node
}

// SyncState represents comprehensive synchronization state
type SyncState struct {
	IsSyncing           bool
	SyncType            SyncType
	CurrentHeight       uint64
	TargetHeight        uint64
	SyncProgress        float64
	LastBlockTime       time.Time
	EstimatedCompletion time.Time
	PeerConsensus       float64
	DataConsistency     DataConsistencyLevel
	NetworkHealth       float64
	SyncQuality         SyncQualityLevel
	SyncStartHeight     uint64        // Height at which synchronization started
	SyncTargetHeight    uint64        // Target height for current sync operation
	SyncStartTime       time.Time     // When the current sync operation started
	PeerConnections     int           // Number of active peer connections
}

// ExecutionMode defines bootstrap node operation modes
type ExecutionMode int

const (
	ModeBalanced        ExecutionMode = iota // Balance mining and sync
	ModeSyncPriority                         // Prioritize synchronization
	ModeMiningPriority                       // Prioritize mining operations
	ModeNetworkRecovery                      // Network connectivity recovery
)

// AdaptiveParameters controls dynamic behavior adjustment
type AdaptiveParameters struct {
	SyncBatchSize        int
	MiningDelay          time.Duration
	NetworkTimeout       time.Duration
	RetryBackoffFactor   float64
	StabilityThreshold   float64
	EmergencyModeEnabled bool
	MiningBatchSize      int             // Size of mining batches
	RetryDelayBase       time.Duration   // Base retry delay
	RetryDelayMax        time.Duration   // Maximum retry delay
	HealthCheckInterval  time.Duration   // Health check interval
	ConnectionTimeout    time.Duration   // Connection timeout
}

// SyncEventType categorizes synchronization events
type SyncEventType int

const (
	SyncEventBlockReceived SyncEventType = iota
	SyncEventBlockProcessed
	SyncEventChainExtended
	SyncEventForkDetected
	SyncEventSyncCompleted
	SyncEventSyncFailed
)

// SyncEvent represents a synchronization event for coordination
type SyncEvent struct {
	EventType     SyncEventType
	BlockHeight   uint64
	TargetHeight  uint64
	Description   string
	RelatedBlocks []*core.Block
	Timestamp     time.Time
}

// NotifySyncEvent notifies all observers about sync events
type NotifySyncEvent func(eventType SyncEventType, blockHeight, targetHeight uint64, description string, blocks []*core.Block)

// === Block Propagation Types ===

// PropagationMetrics tracks block propagation performance
type PropagationMetrics struct {
	TotalBlocksPropagated   uint64
	TotalBlocksQueued       uint64
	BlocksLost              uint32
	MinPropagationTime      time.Duration
	MaxPropagationTime      time.Duration
	AveragePropagationTime  time.Duration
	LastPropagationTime     time.Duration
	ChannelUtilization      map[PriorityLevel]float64
	LastMetricsUpdate       time.Time
	HighPriorityPropagated  uint64
	NormalPriorityPropagated uint64
	LowPriorityPropagated   uint64
	AverageSuccessRate      float64
	AverageReachRatio       float64
	
	// Additional fields for success tracking
	HighPrioritySuccesses   uint64
	NormalPrioritySuccesses uint64
	LowPrioritySuccesses    uint64
	
	// System metrics for comprehensive monitoring
	SystemStartTime         time.Time
	ActiveConnectionCount   int32
	TotalRetries            uint64
	PeersAvailable          int32
}

// PropagationStatus provides comprehensive propagation monitoring
type PropagationStatus struct {
	ChannelUtilization    map[PriorityLevel]float64
	TotalBlocksPropagated uint64
	BlocksLost            uint32
	RateLimiterActive     bool
	BackPressureLevel     float64
}

// === Adaptive Sync Detection Types ===

// SyncType defines different synchronization modes
type SyncType int

const (
	SyncTypeBootstrap  SyncType = iota // Initial chain synchronization
	SyncTypeCatchup                    // Catching up after being offline
	SyncTypeRealTime                   // Real-time block propagation
	SyncTypeFullResync                 // Full chain resynchronization
)

// DataConsistencyLevel measures blockchain data integrity
type DataConsistencyLevel int

const (
	ConsistencyUnknown   DataConsistencyLevel = iota
	ConsistencyWarning                        // Minor inconsistencies detected
	ConsistencyGood                           // Basic consistency checks passed
	ConsistencyExcellent                      // Full validation passed
)

// SyncQualityLevel evaluates synchronization reliability
type SyncQualityLevel int

const (
	QualityUnknown   SyncQualityLevel = iota
	QualityPoor                       // Frequent sync failures
	QualityFair                       // Some issues but generally functional
	QualityGood                       // Reliable sync operation
	QualityExcellent                  // Optimal performance
)

// === Recovery Strategy Types ===

// RecoveryStrategy defines recovery approach
type RecoveryStrategy int

const (
	StrategyPeerReconnection RecoveryStrategy = iota
	StrategyPartialResync
	StrategyFullResync
	StrategyNetworkReset
	StrategyBootstrapReset
)

// RecoveryPriority indicates urgency of recovery
type RecoveryPriority int

const (
	RecoveryPriorityLow RecoveryPriority = iota
	RecoveryPriorityMedium
	RecoveryPriorityHigh
	RecoveryPriorityCritical
)

// === Sync Manager Interface ===

// SyncManager defines the interface for synchronization management
type SyncManager interface {
	// State queries
	IsSyncing() bool
	IsSynced() bool
	GetCurrentHeight() uint64
	GetTargetHeight() uint64
	GetSyncProgress() float64

	// Control operations
	StartSync() error
	StopSync() error
	PauseSync() error
	ResumeSync() error

	// Event management
	RegisterSyncHandler(handler SyncEventHandler)
	UnregisterSyncHandler(handler SyncEventHandler)

	// Status reporting
	GetSyncStatus() *SyncStatus
	GetSyncStatistics() *SyncStatistics
}

// SyncEventHandler handles synchronization events
type SyncEventHandler func(event SyncEventType, height uint64, data interface{})

// SyncStatus represents current synchronization status
type SyncStatus struct {
	IsSyncing      bool
	CurrentHeight  uint64
	TargetHeight   uint64
	Progress       float64
	PeersConnected int
	LastSyncTime   time.Time
	EstimatedTime  time.Duration
}

// SyncStatistics provides synchronization performance statistics
type SyncStatistics struct {
	TotalBlocksSynced uint64
	AverageBlockTime  time.Duration
	SuccessRate       float64
	LastSyncDuration  time.Duration
	PeersCount        int
}

// === Helper Functions ===

// String returns the string representation of PriorityLevel
func (p PriorityLevel) String() string {
	switch p {
	case PriorityLow:
		return "Low"
	case PriorityNormal:
		return "Normal"
	case PriorityHigh:
		return "High"
	case PriorityCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

// String returns the string representation of SyncType
func (s SyncType) String() string {
	switch s {
	case SyncTypeBootstrap:
		return "Bootstrap"
	case SyncTypeCatchup:
		return "Catchup"
	case SyncTypeRealTime:
		return "RealTime"
	case SyncTypeFullResync:
		return "FullResync"
	default:
		return "Unknown"
	}
}

// String returns the string representation of ExecutionMode
func (m ExecutionMode) String() string {
	switch m {
	case ModeBalanced:
		return "Balanced"
	case ModeSyncPriority:
		return "SyncPriority"
	case ModeMiningPriority:
		return "MiningPriority"
	case ModeNetworkRecovery:
		return "NetworkRecovery"
	default:
		return "Unknown"
	}
}

// DefaultSyncManager creates a basic sync manager implementation
func DefaultSyncManager() SyncManager {
	return &defaultSyncManager{}
}

// defaultSyncManager implements basic synchronization management
type defaultSyncManager struct {
	currentHeight uint64
	targetHeight  uint64
	isSyncing     bool
}

func (d *defaultSyncManager) IsSyncing() bool {
	return d.isSyncing
}

func (d *defaultSyncManager) IsSynced() bool {
	return !d.isSyncing && d.currentHeight >= d.targetHeight
}

func (d *defaultSyncManager) GetCurrentHeight() uint64 {
	return d.currentHeight
}

func (d *defaultSyncManager) GetTargetHeight() uint64 {
	return d.targetHeight
}

func (d *defaultSyncManager) GetSyncProgress() float64 {
	if d.targetHeight == 0 {
		return 0.0
	}
	return float64(d.currentHeight) / float64(d.targetHeight)
}

func (d *defaultSyncManager) StartSync() error {
	d.isSyncing = true
	return nil
}

func (d *defaultSyncManager) StopSync() error {
	d.isSyncing = false
	return nil
}

func (d *defaultSyncManager) PauseSync() error {
	d.isSyncing = false
	return nil
}

func (d *defaultSyncManager) ResumeSync() error {
	d.isSyncing = true
	return nil
}

func (d *defaultSyncManager) RegisterSyncHandler(handler SyncEventHandler) {
	// Implementation would maintain a list of handlers
}

func (d *defaultSyncManager) UnregisterSyncHandler(handler SyncEventHandler) {
	// Implementation would remove handler from list
}

func (d *defaultSyncManager) GetSyncStatus() *SyncStatus {
	return &SyncStatus{
		IsSyncing:      d.isSyncing,
		CurrentHeight:  d.currentHeight,
		TargetHeight:   d.targetHeight,
		Progress:       d.GetSyncProgress(),
		PeersConnected: 0,           // Would be implemented in real code
		LastSyncTime:   time.Time{}, // Would be implemented in real code
		EstimatedTime:  0,           // Would be calculated
	}
}

func (d *defaultSyncManager) GetSyncStatistics() *SyncStatistics {
	return &SyncStatistics{
		TotalBlocksSynced: d.currentHeight,
		AverageBlockTime:  0,   // Would be calculated
		SuccessRate:       1.0, // Would be calculated
		LastSyncDuration:  0,   // Would be recorded
		PeersCount:        0,   // Would be tracked
	}
}

// === Health Assessment System Types ===

// HealthAssessmentReport provides comprehensive system health evaluation
type HealthAssessmentReport struct {
	Timestamp       time.Time
	OverallScore    float64  // 0.0-1.0 overall health score
	ComponentScores map[string]float64
	CriticalIssues  []string
	Warnings        []string
	Recommendations []string
	SystemMetrics   SystemHealthMetrics
}

// SystemHealthMetrics contains detailed performance metrics for health assessment
type SystemHealthMetrics struct {
	NetworkLatency          time.Duration
	BlockPropagationTime    time.Duration
	SyncCompletionRate      float64
	PeerConnectionStability float64
	ResourceUtilization     ResourceMetrics
}

// ResourceMetrics tracks system resource usage
type ResourceMetrics struct {
	CPUUsage    float64
	MemoryUsage float64
	DiskUsage   float64
	NetworkIO   float64
}

// OptimalThresholds defines system operation thresholds
type OptimalThresholds struct {
	MaxBlockPropagationTime     time.Duration
	MinPeerConnections          int
	MaxNetworkLatency           time.Duration
	MinSyncSuccessRate          float64
	MaxResourceUtilization      float64
	MinBlockValidationRate      float64
	MaxMempoolSize              int
	MinTransactionConfirmationRate float64
}

// TelemetrySnapshot captures real-time system performance data
type TelemetrySnapshot struct {
	Timestamp         time.Time
	ActiveConnections int
	BlockHeight       uint64
	MemoryUsage      float64
	CPUUtilization   float64
	NetworkThroughput float64
	SyncProgress      float64
}

// === Rate Limiting and Back Pressure Types ===

// PropagationRateLimiter controls block propagation rate
type PropagationRateLimiter struct {
	maxRate    float64
	allowances map[string]time.Time
	mu         sync.RWMutex
	lastBlockTime time.Time
}

// NewPropagationRateLimiter creates a new rate limiter
func NewPropagationRateLimiter(maxRate float64) *PropagationRateLimiter {
	return &PropagationRateLimiter{
		maxRate:    maxRate,
		allowances: make(map[string]time.Time),
	}
}

// Allow checks if a block can be propagated
func (r *PropagationRateLimiter) Allow(blockID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	now := time.Now()
	if allowance, exists := r.allowances[blockID]; exists {
		if now.Before(allowance) {
			return false
		}
	}
	
	r.allowances[blockID] = now.Add(time.Second / time.Duration(r.maxRate))
	return true
}

// BackPressureController manages network congestion control
type BackPressureController struct {
	congestionThreshold float64
	currentPressure     float64
	mu                  sync.RWMutex
}

// NewBackPressureController creates a new back pressure controller
func NewBackPressureController(threshold float64) *BackPressureController {
	return &BackPressureController{
		congestionThreshold: threshold,
		currentPressure:     0.0,
	}
}

// IsCongested checks if system is under back pressure
func (b *BackPressureController) IsCongested() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentPressure >= b.congestionThreshold
}

// UpdatePressure updates the current pressure level
func (b *BackPressureController) UpdatePressure(pressure float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.currentPressure = pressure
}

// BlockDeduplicationTracker prevents duplicate block propagation
type BlockDeduplicationTracker struct {
	dedupWindow time.Duration
	blockHashes map[string]time.Time
	mu          sync.RWMutex
}

// NewBlockDeduplicationTracker creates a new deduplication tracker
func NewBlockDeduplicationTracker(window time.Duration) *BlockDeduplicationTracker {
	return &BlockDeduplicationTracker{
		blockHashes: make(map[string]time.Time),
		dedupWindow: window,
	}
}

// IsDuplicate checks if a block is a duplicate
func (b *BlockDeduplicationTracker) IsDuplicate(block *core.Block) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	hash := b.getBlockHash(block)
	timestamp, exists := b.blockHashes[hash]
	
	if !exists {
		return false
	}
	
	return time.Since(timestamp) < b.dedupWindow
}

// RegisterBlock registers a block for deduplication
func (b *BlockDeduplicationTracker) RegisterBlock(block *core.Block) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	hash := b.getBlockHash(block)
	b.blockHashes[hash] = time.Now()
}

// getBlockHash generates a hash for the block
func (b *BlockDeduplicationTracker) getBlockHash(block *core.Block) string {
	if block == nil {
		return ""
	}
	return string(block.Height) // Simplified hash - real implementation would use actual block hash
}

// CleanupExpired removes expired entries from tracking
func (b *BlockDeduplicationTracker) CleanupExpired() {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	now := time.Now()
	for hash, timestamp := range b.blockHashes {
		if now.Sub(timestamp) >= b.dedupWindow {
			delete(b.blockHashes, hash)
		}
	}
}
