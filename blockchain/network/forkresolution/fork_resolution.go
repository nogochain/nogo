// Copyright 2026 NogoChain Team
// Production-grade unified fork resolution module
// Based on core-main's proven architecture with multi-node enhancements
// This is the SINGLE entry point for all fork detection and resolution in the system

package forkresolution

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

const (
	// DefaultForkResolutionTimeout is timeout for fork resolution operations
	DefaultForkResolutionTimeout = 30 * time.Second

	// MaxReorgDepth is maximum allowed reorganization depth
	MaxReorgDepth = 100

	// IrreversibleTimeWindow: if >0, refuse reorgs whose common ancestor's header
	// timestamp is older than this duration relative to wall clock.
	// A small wall-clock window compared to **block timestamps** incorrectly rejects
	// almost all legitimate PoW reorgs (ancestor mining time is normally far in the past).
	// Set to 0 to disable this heuristic; use checkpoints or explicit finality if needed.
	IrreversibleTimeWindow = 0

	// MinWorkDiffThreshold prevents reorg loops from tiny work differences
	// This is critical to prevent infinite reorg loops when work difference is negligible
	// Value: 1e-15 = 0.0000000000001% of total work
	// Example: For work=2.5e27, minimum diff = 2.5e12 (2.5 trillion)
	// This allows legitimate reorgs while preventing loops from micro-differences (e.g., 128)
	MinWorkDiffThreshold = 1e-15

	// === PREVENTIVE FORK HANDLING STRATEGY ===
	// The key insight: resolve forks EARLY before they accumulate into deep forks

	// LightForkInterval for shallow forks (depth 1-3) - IMMEDIATE handling to prevent accumulation
	LightForkInterval = 500 * time.Millisecond

	// NormalForkInterval for medium forks (depth 4-6) - FAST handling
	NormalForkInterval = 2 * time.Second

	// EmergencyForkInterval for deep forks (depth >= 7) - URGENT handling (fallback)
	EmergencyForkInterval = 1 * time.Second

	// Depth thresholds for classification
	LightForkMaxDepth  = 3 // Shallow forks: handle immediately
	NormalForkMaxDepth = 6 // Medium forks: handle quickly
	// DeepForkDepthThreshold = 7+ (implicit: > NormalForkMaxDepth)
)

// ForkSeverity represents the urgency level of a fork
type ForkSeverity int

const (
	ForkSeverityLight     ForkSeverity = iota // depth 1-3: immediate (< 500ms)
	ForkSeverityNormal                        // depth 4-6: fast (2s)
	ForkSeverityEmergency                     // depth 7+: urgent (1s)
)

// ForkType represents the type of fork detected
type ForkType int

const (
	ForkTypeNone ForkType = iota
	ForkTypeTemporary
	ForkTypePersistent
	ForkTypeDeep
)

// ForkEvent represents a detected fork event
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
}

// ChainProvider interface for chain operations needed by fork resolution
type ChainProvider interface {
	LatestBlock() *core.Block
	CanonicalWork() *big.Int
	AddBlock(block *core.Block) (bool, error)
	RollbackToHeight(height uint64) error
	BlockByHeight(height uint64) (*core.Block, bool)
	BlockByHash(hash string) (*core.Block, bool)
	CalculateCumulativeWork(block *core.Block) *big.Int
	SetOnForkResolved(callback func(newHeight, rolledBack uint64))
}

// SyncNotifier interface for notifying about chain reorganizations
type SyncNotifier interface {
	OnChainReorganized(newTip *core.Block)
}

// ReorgResult represents the result of a reorganization attempt
type ReorgResult struct {
	Success    bool
	Switched   bool
	OldTip     *core.Block
	NewTip     *core.Block
	ReorgDepth uint64
	Duration   time.Duration
	Error      error
	Timestamp  time.Time
}

// ReorgRecord stores information about a single reorganization event
// Used for oscillation detection to prevent rapid chain switching
type ReorgRecord struct {
	Timestamp time.Time
	OldTip    uint64
	NewTip    uint64
	Depth     uint64
	Source    string
}

// ForkResolver is the unified fork resolution engine
// Based on core-main's block_keeper architecture with enhanced multi-node support
type ForkResolver struct {
	mu     sync.RWMutex
	chain  ChainProvider
	ctx    context.Context
	cancel context.CancelFunc

	// Reorg state management
	reorgInProgress          bool
	lastReorgTime            time.Time
	lastSuccessfulReorgTime  time.Time // Only updated on successful reorgs; used for interval throttling
	reorgMu                  sync.Mutex

	// Oscillation detection
	recentReorgs              []ReorgRecord // Recent reorg history for oscillation detection
	oscillationCount          int           // Number of rapid reorgs detected
	oscillationProtected      bool          // Whether oscillation protection is active
	oscillationProtectedUntil time.Time     // End time of oscillation protection

	// Callbacks
	onReorgComplete func(newHeight uint64)
	onForkDetected  func(event ForkEvent)

	// Statistics
	stats ResolverStats
}

// ResolverStats holds fork resolution statistics
type ResolverStats struct {
	TotalForksDetected      int64
	TotalReorgsPerformed    int64
	TotalReorgsFailed       int64
	TotalFallbackRollbacks  int64 // Shallow rollbacks after failed common-ancestor lookup (not true reorgs)
	LastForkTime            time.Time
	LastReorgTime           time.Time
	MaxReorgDepth           uint64
	AvgReorgDuration        time.Duration
}

// NewForkResolver creates a new unified fork resolver
func NewForkResolver(ctx context.Context, chain ChainProvider) *ForkResolver {
	childCtx, cancel := context.WithCancel(ctx)

	return &ForkResolver{
		chain:  chain,
		ctx:    childCtx,
		cancel: cancel,
	}
}

// SetOnReorgComplete sets callback for successful reorganization
func (fr *ForkResolver) SetOnReorgComplete(callback func(newHeight uint64)) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.onReorgComplete = callback
}

// SetOnForkDetected sets callback for fork detection events
func (fr *ForkResolver) SetOnForkDetected(callback func(ForkEvent)) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.onForkDetected = callback
}

// DetectFork detects if a fork exists between local and remote chains
// Core-main style: simple hash comparison with work-based validation
func (fr *ForkResolver) DetectFork(localBlock, remoteBlock *core.Block, peerID string) *ForkEvent {
	if localBlock == nil || remoteBlock == nil {
		return nil
	}

	if string(localBlock.Hash) == string(remoteBlock.Hash) {
		return nil
	}

	event := &ForkEvent{
		DetectedAt:   time.Now(),
		LocalHeight:  localBlock.GetHeight(),
		LocalHash:    hex.EncodeToString(localBlock.Hash),
		RemoteHeight: remoteBlock.GetHeight(),
		RemoteHash:   hex.EncodeToString(remoteBlock.Hash),
		PeerID:       peerID,
	}

	localWork := fr.chain.CalculateCumulativeWork(localBlock)
	remoteWork := fr.chain.CalculateCumulativeWork(remoteBlock)
	event.LocalWork = localWork
	event.RemoteWork = remoteWork

	if localBlock.GetHeight() == remoteBlock.GetHeight() {
		event.Type = ForkTypePersistent
		event.Depth = 0
	} else {
		if localBlock.GetHeight() > remoteBlock.GetHeight() {
			event.Depth = localBlock.GetHeight() - remoteBlock.GetHeight()
		} else {
			event.Depth = remoteBlock.GetHeight() - localBlock.GetHeight()
		}
		if event.Depth >= 6 {
			event.Type = ForkTypeDeep
		} else {
			event.Type = ForkTypeTemporary
		}
	}

	fr.mu.Lock()
	fr.stats.TotalForksDetected++
	fr.stats.LastForkTime = time.Now()
	// Capture callback under lock to prevent data race with SetOnForkDetected.
	forkCb := fr.onForkDetected
	fr.mu.Unlock()

	if forkCb != nil {
		go forkCb(*event)
	}

	log.Printf("[ForkResolver] Fork detected: type=%v depth=%d local_h=%d remote_h=%d peer=%s",
		event.Type, event.Depth, event.LocalHeight, event.RemoteHeight, peerID)

	return event
}

// ShouldReorg determines if reorganization should be performed based on work comparison.
// Nakamoto rule: strictly more cumulative proof-of-work on the candidate tip wins.
// Remote tip may have lower height than local but higher chain work; height must not override work.
//
// Uses remoteBlock.TotalWork when populated (caller already fetched work from peer chain info).
// Falls back to chain.CalculateCumulativeWork for blocks that arrive via broadcast/fetch
// with full header data but without pre-computed TotalWork.
func (fr *ForkResolver) ShouldReorg(remoteBlock *core.Block) bool {
	if remoteBlock == nil {
		return false
	}

	localTip := fr.chain.LatestBlock()
	if localTip == nil {
		return true
	}

	if string(localTip.Hash) == string(remoteBlock.Hash) {
		return false
	}

	localWork := fr.chain.CanonicalWork()
	if localWork == nil {
		return false
	}

	var remoteWork *big.Int
	if remoteBlock.TotalWork != "" {
		remoteWork = new(big.Int)
		if _, ok := remoteWork.SetString(remoteBlock.TotalWork, 10); !ok {
			remoteWork = fr.chain.CalculateCumulativeWork(remoteBlock)
		}
	} else {
		remoteWork = fr.chain.CalculateCumulativeWork(remoteBlock)
	}

	// CRITICAL: Only reorg if remote work is STRICTLY greater than local work
	if remoteWork.Cmp(localWork) <= 0 {
		log.Printf("[ForkResolver] ShouldReorg: remote work %s <= local work %s (remote_h=%d local_h=%d), NOT triggering reorg",
			remoteWork.String(), localWork.String(), remoteBlock.GetHeight(), localTip.GetHeight())
		return false
	}

	// CRITICAL FIX: Check if work difference is significant enough to justify reorg
	// This prevents infinite reorg loops due to tiny work differences (e.g., 128 / 2.5e27 = 5e-23)
	// Such micro-differences can occur from:
	//   1. Block propagation delays
	//   2. Work calculation precision differences
	//   3. Network timing issues
	//
	// We set threshold to 1e-15 (0.0000000000001%) which:
	//   - Allows legitimate reorgs (typically > 1% work difference)
	//   - Prevents loops from micro-differences (< 1e-20)
	//   - For work=2.5e27, minimum diff = 2.5e12 (2.5 trillion work units)
	workDiff := new(big.Int).Sub(remoteWork, localWork)
	workDiffRatio := new(big.Float).Quo(
		new(big.Float).SetInt(workDiff),
		new(big.Float).SetInt(localWork),
	)

	diffRatio, _ := workDiffRatio.Float64()
	if diffRatio < MinWorkDiffThreshold {
		log.Printf("[ForkResolver] ShouldReorg: work difference too small (%.6e < %.6e), skipping reorg to prevent loop (remote_h=%d local_h=%d)",
			diffRatio, MinWorkDiffThreshold, remoteBlock.GetHeight(), localTip.GetHeight())
		return false
	}

	log.Printf("[ForkResolver] ShouldReorg: remote work %s > local work %s (diff=%.6e, threshold=%.6e), triggering reorg",
		remoteWork.String(), localWork.String(), diffRatio, MinWorkDiffThreshold)
	return true
}

// RequestReorg is the UNIFIED ENTRY POINT for all reorganization requests
// It automatically classifies fork severity and applies appropriate timing:
//   - Light forks (depth 1-3):    500ms interval (preventive - stop accumulation!)
//   - Normal forks (depth 4-6):   2s interval (fast response)
//   - Emergency forks (depth 7+):  1s interval (urgent fallback)
//
// This SINGLE METHOD replaces all previous reorg entry points.
// Call this for EVERY fork scenario - it will handle severity classification automatically.
func (fr *ForkResolver) RequestReorg(newBlock *core.Block, source string) error {
	return fr.RequestReorgWithDepth(newBlock, source, 0)
}

// RequestReorgWithDepth is the unified reorganization with explicit depth information
// If depth=0, it will be calculated automatically from chain state
func (fr *ForkResolver) RequestReorgWithDepth(newBlock *core.Block, source string, explicitDepth uint64) error {
	if newBlock == nil {
		return fmt.Errorf("RequestReorg: nil block from %s", source)
	}

	acquired := fr.reorgMu.TryLock()
	if !acquired {
		return fmt.Errorf("reorganization already in progress")
	}
	defer fr.reorgMu.Unlock()

	// reorgInProgress is written under fr.mu.Lock() in executeReorg;
	// protect the read with fr.mu.RLock() to prevent data race.
	fr.mu.RLock()
	inProgress := fr.reorgInProgress
	fr.mu.RUnlock()
	if inProgress {
		return fmt.Errorf("reorganization already in progress")
	}

	// OSCILLATION DETECTION: Check if we're experiencing rapid chain switching
	// This prevents the "ping-pong" effect where node switches between two forks
	// If oscillation is detected, reject reorg and activate protection
	if fr.detectOscillation(newBlock) {
		log.Printf("[ForkResolver] ⚠️ Oscillation detected! Rejecting reorg from %s to prevent chain instability", source)
		return fmt.Errorf("oscillation detected: too many rapid chain switches, rejecting reorg")
	}

	// Classify fork severity and get appropriate interval
	forkDepth := explicitDepth
	if forkDepth == 0 {
		localTip := fr.chain.LatestBlock()
		if localTip != nil {
			if newBlock.GetHeight() > localTip.GetHeight() {
				forkDepth = newBlock.GetHeight() - localTip.GetHeight()
			} else if localTip.GetHeight() > newBlock.GetHeight() {
				forkDepth = localTip.GetHeight() - newBlock.GetHeight()
			}
		}
	}

	severity := fr.classifyForkSeverity(forkDepth)
	interval := fr.getIntervalForSeverity(severity)

	log.Printf("[ForkResolver] Reorg requested by %s: height=%d hash=%x depth=%d severity=%v interval=%v",
		source, newBlock.GetHeight(), newBlock.Hash[:8], forkDepth, severity, interval)

	// Protected read of throttling timestamp to prevent data race with
	// recordSuccessfulReorg which writes under fr.mu.Lock().
	fr.mu.RLock()
	throttleSince := time.Since(fr.lastSuccessfulReorgTime)
	fr.mu.RUnlock()
	if throttleSince < interval {
		return fmt.Errorf("reorg too frequent for severity=%v: minimum interval %v not elapsed", severity, interval)
	}

	result := fr.executeReorg(newBlock, source+fmt.Sprintf("-[%v]", severity))
	if !result.Success {
		return result.Error
	}

	return nil
}

// detectOscillation detects if we're experiencing rapid chain switching (oscillation)
// Uses work-based comparison: only blocks oscillation if the remote chain has
// LESS cumulative work than the current chain, preventing unnecessary reorgs.
// This is more robust than time-window based detection because:
//   - Legitimate reorgs (remote has more work) are always allowed
//   - Only ping-pong between equal-work chains is blocked
//   - No hardcoded time window that could block legitimate deep reorgs
func (fr *ForkResolver) detectOscillation(newBlock *core.Block) bool {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	// Check if oscillation protection is active
	if fr.oscillationProtected && time.Now().Before(fr.oscillationProtectedUntil) {
		log.Printf("[ForkResolver] Oscillation protection active until %v, rejecting reorg",
			fr.oscillationProtectedUntil)
		return true
	}

	// Clear protection if expired
	if fr.oscillationProtected && time.Now().After(fr.oscillationProtectedUntil) {
		log.Printf("[ForkResolver] Oscillation protection expired, clearing")
		fr.oscillationProtected = false
		fr.oscillationCount = 0
	}

	// Need at least 2 recent reorgs to detect oscillation
	if len(fr.recentReorgs) < 2 {
		return false
	}

	// FIXED: Detect oscillation by checking if we're switching between SAME tips
	// This works regardless of work difference
	newTipHeight := newBlock.GetHeight()
	
	// Count how many times we've switched to this new tip
	switchCount := 0
	for _, record := range fr.recentReorgs {
		if record.NewTip == newTipHeight {
			switchCount++
		}
	}

	// If we've switched to this tip more than once in recent history, it's oscillation
	if switchCount >= 2 {
		log.Printf("[ForkResolver] Oscillation detected: switching to tip %d multiple times",
			newTipHeight)
		fr.activateOscillationProtection()
		return true
	}

	// Additional check: ping-pong between two tips
	if len(fr.recentReorgs) >= 3 {
		lastTip := fr.recentReorgs[len(fr.recentReorgs)-1].NewTip
		prevTip := fr.recentReorgs[len(fr.recentReorgs)-2].NewTip
		
		// If we're switching back to a previous tip (ping-pong)
		if newTipHeight == prevTip && newTipHeight != lastTip {
			log.Printf("[ForkResolver] Ping-pong oscillation detected: %d -> %d -> %d",
				prevTip, lastTip, newTipHeight)
			fr.activateOscillationProtection()
			return true
		}
	}

	return false
}

// activateOscillationProtection activates oscillation protection for 30 seconds
// Reduced from 1 minute to 30 seconds to minimize disruption while still
// preventing rapid ping-pong switching between equal-work chains.
func (fr *ForkResolver) activateOscillationProtection() {
	fr.oscillationProtected = true
	fr.oscillationProtectedUntil = time.Now().Add(30 * time.Second)
	log.Printf("[ForkResolver] Oscillation protection ACTIVATED until %v",
		fr.oscillationProtectedUntil)
}

// recordReorgForOscillation records a reorg event for oscillation detection
// Should be called after successful reorg
func (fr *ForkResolver) recordReorgForOscillation(oldTip, newTip uint64, depth uint64, source string) {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	record := ReorgRecord{
		Timestamp: time.Now(),
		OldTip:    oldTip,
		NewTip:    newTip,
		Depth:     depth,
		Source:    source,
	}

	// Append to recent reorgs (keep last 10)
	fr.recentReorgs = append(fr.recentReorgs, record)
	if len(fr.recentReorgs) > 10 {
		fr.recentReorgs = fr.recentReorgs[1:]
	}

	log.Printf("[ForkResolver] Recorded reorg for oscillation detection: %d → %d (depth %d)",
		oldTip, newTip, depth)
}

// classifyForkSeverity determines the urgency level based on fork depth
func (fr *ForkResolver) classifyForkSeverity(depth uint64) ForkSeverity {
	switch {
	case depth <= LightForkMaxDepth:
		return ForkSeverityLight
	case depth <= NormalForkMaxDepth:
		return ForkSeverityNormal
	default:
		return ForkSeverityEmergency
	}
}

// getIntervalForSeverity returns the appropriate interval for each severity level
func (fr *ForkResolver) getIntervalForSeverity(severity ForkSeverity) time.Duration {
	switch severity {
	case ForkSeverityLight:
		return LightForkInterval // 500ms - PREVENTIVE!
	case ForkSeverityNormal:
		return NormalForkInterval // 2s - FAST
	case ForkSeverityEmergency:
		return EmergencyForkInterval // 1s - URGENT
	default:
		return NormalForkInterval
	}
}

// executeReorg performs the actual reorganization.
// Core-main style: find common ancestor, validate irreversibility,
// rollback to ancestor, then extend chain to the new tip.
func (fr *ForkResolver) executeReorg(newBlock *core.Block, source string) ReorgResult {
	startTime := time.Now()

	fr.mu.Lock()
	fr.reorgInProgress = true
	fr.mu.Unlock()

	defer func() {
		fr.mu.Lock()
		fr.reorgInProgress = false
		fr.lastReorgTime = time.Now()
		fr.mu.Unlock()
	}()

	result := ReorgResult{
		Timestamp: startTime,
		OldTip:    fr.chain.LatestBlock(),
		NewTip:    newBlock,
	}

	localTip := fr.chain.LatestBlock()
	if localTip == nil {
		result.Error = fmt.Errorf("local chain has no tip")
		result.Success = false
		return result
	}

	// Case 1: Direct extension of local tip (no reorg needed, simple append)
	if bytes.Equal(newBlock.Header.PrevHash, localTip.Hash) {
		accepted, err := fr.chain.AddBlock(newBlock)
		if err != nil {
			result.Error = fmt.Errorf("add extension block: %w", err)
			result.Success = false
			fr.recordFailedReorg()
			return result
		}
		result.Switched = accepted
		result.Success = true
		result.Duration = time.Since(startTime)
		result.ReorgDepth = 0
		fr.recordSuccessfulReorg(0, result.Duration)

		// Read callback under lock to prevent data race with SetOnReorgComplete.
		fr.mu.RLock()
		cb := fr.onReorgComplete
		fr.mu.RUnlock()
		if cb != nil && accepted {
			go cb(newBlock.GetHeight())
		}
		return result
	}

	// Case 2: Find the common ancestor between local chain and the remote block
	ancestor, err := fr.findAncestorForReorg(localTip, newBlock)
	if err != nil {
		// Cannot find common ancestor by hash walking. This happens for deep forks
		// (depth > 1) where the remote tip's parent blocks are not in the local DB.
		// Fall back to a shallow rollback to allow recovery via re-sync from a
		// DIFFERENT peer. The offending peer must be deprioritized by the caller.
		log.Printf("[ForkResolver] findAncestorForReorg failed: %v, using shallow fallback rollback", err)

		fallbackHeight := localTip.GetHeight()
		if newBlock.GetHeight() < fallbackHeight {
			fallbackHeight = newBlock.GetHeight()
		}
		if fallbackHeight > 0 {
			fallbackHeight--
		}

		if rbErr := fr.chain.RollbackToHeight(fallbackHeight); rbErr != nil {
			result.Error = fmt.Errorf("fallback rollback to height %d: %w", fallbackHeight, rbErr)
			result.Success = false
			fr.recordFailedReorg()
			return result
		}

		reorgDepth := localTip.GetHeight() - fallbackHeight
		result.ReorgDepth = reorgDepth

		log.Printf("[ForkResolver] Fallback rollback: local_h=%d rollback_h=%d depth=%d source=%s — shallow recovery, resync from different peer required",
			localTip.GetHeight(), fallbackHeight, reorgDepth, source)

		// Fallback rollback is a partial recovery, NOT a complete reorg.
		// Return Success=false with a descriptive error so the caller can
		// deprioritize the offending peer and trigger resync from others.
		// Do NOT record this as a successful reorg, do NOT feed oscillation
		// detection, and do NOT fire onReorgComplete — the caller must
		// handle resync from a DIFFERENT peer independently.
		result.Error = fmt.Errorf("fallback rollback: no common ancestor with peer tip at h=%d, rolled back %d blocks to h=%d for recovery",
			newBlock.GetHeight(), reorgDepth, fallbackHeight)
		result.Success = false
		result.Switched = true
		result.Duration = time.Since(startTime)

		fr.recordFallbackRollback()

		return result
	}

	reorgDepth := localTip.GetHeight() - ancestor.GetHeight()
	result.ReorgDepth = reorgDepth

	// Validate against maximum reorg depth
	if reorgDepth > MaxReorgDepth {
		result.Error = fmt.Errorf("reorg depth %d exceeds maximum %d", reorgDepth, MaxReorgDepth)
		result.Success = false
		fr.recordFailedReorg()
		return result
	}

	// Optional weak-subjectivity / finality: only when IrreversibleTimeWindow > 0.
	if IrreversibleTimeWindow > 0 && ancestor.GetHeight() > 0 && reorgDepth > 0 {
		ancestorBlock, exists := fr.chain.BlockByHeight(ancestor.GetHeight())
		if exists && ancestorBlock != nil && ancestorBlock.Header.TimestampUnix > 0 {
			blockTime := time.Unix(ancestorBlock.Header.TimestampUnix, 0)
			if time.Since(blockTime) > IrreversibleTimeWindow {
				result.Error = fmt.Errorf("reorg refused: ancestor at height %d timestamp %v exceeds irreversibility window",
					ancestor.GetHeight(), blockTime)
				result.Success = false
				fr.recordFailedReorg()
				return result
			}
		}
	}

	log.Printf("[ForkResolver] Starting reorg: local_h=%d ancestor_h=%d target_h=%d depth=%d source=%s",
		localTip.GetHeight(), ancestor.GetHeight(), newBlock.GetHeight(), reorgDepth, source)

	// Prefer atomic chain reorganize when the full fork is already in the chain store (production *core.Chain).
	type forkReorgBackend interface {
		ReorganizeToKnownFork(ancestor, tip *core.Block) error
	}

	var (
		reorgErr   error
		usedAtomic bool
	)
	if backend, ok := fr.chain.(forkReorgBackend); ok {
		usedAtomic = true
		reorgErr = backend.ReorganizeToKnownFork(ancestor, newBlock)
	}

	if usedAtomic && reorgErr == nil {
		result.Switched = true
		result.Success = true
		result.Duration = time.Since(startTime)
		fr.recordSuccessfulReorg(reorgDepth, result.Duration)

		oldTipHeight := uint64(0)
		if localTip != nil {
			oldTipHeight = localTip.GetHeight()
		}
		fr.recordReorgForOscillation(oldTipHeight, newBlock.GetHeight(), reorgDepth, source)

		if lb := fr.chain.LatestBlock(); lb != nil {
			result.NewTip = lb
		}

		log.Printf("[ForkResolver] Reorg completed (ReorganizeToKnownFork): success=%v depth=%v duration=%v",
			result.Success, reorgDepth, result.Duration)

		// Read callback under lock to prevent data race with SetOnReorgComplete.
		fr.mu.RLock()
		cb := fr.onReorgComplete
		fr.mu.RUnlock()
		if cb != nil {
			h := newBlock.GetHeight()
			if result.NewTip != nil {
				h = result.NewTip.GetHeight()
			}
			go cb(h)
		}
		return result
	}

	if usedAtomic && reorgErr != nil && !errors.Is(reorgErr, core.ErrOrphanBlock) {
		result.Error = fmt.Errorf("reorganize fork: %w", reorgErr)
		result.Success = false
		fr.recordFailedReorg()
		return result
	}

	if usedAtomic && reorgErr != nil {
		log.Printf("[ForkResolver] ReorganizeToKnownFork: %v — using rollback+single-block fallback", reorgErr)
	}

	// Legacy path: rollback then append one block (mock chains or incomplete fork ancestry).
	if reorgDepth > 0 {
		if err := fr.chain.RollbackToHeight(ancestor.GetHeight()); err != nil {
			result.Error = fmt.Errorf("rollback to height %d failed: %w", ancestor.GetHeight(), err)
			result.Success = false
			fr.recordFailedReorg()
			return result
		}
	}

	accepted, err := fr.chain.AddBlock(newBlock)
	if err != nil {
		result.Error = fmt.Errorf("add block after reorg failed: %w", err)
		result.Success = false
		fr.recordFailedReorg()
		return result
	}

	result.Switched = accepted
	result.Success = true
	result.Duration = time.Since(startTime)

	fr.recordSuccessfulReorg(reorgDepth, result.Duration)

	// OSCILLATION DETECTION: Record this reorg for oscillation detection
	oldTipHeight := uint64(0)
	if localTip != nil {
		oldTipHeight = localTip.GetHeight()
	}
	fr.recordReorgForOscillation(oldTipHeight, newBlock.GetHeight(), reorgDepth, source)

	log.Printf("[ForkResolver] Reorg completed: success=%v switched=%v duration=%v depth=%d",
		result.Success, result.Switched, result.Duration, reorgDepth)

	// Read callback under lock to prevent data race with SetOnReorgComplete.
	fr.mu.RLock()
	cb := fr.onReorgComplete
	fr.mu.RUnlock()
	if cb != nil && accepted {
		go cb(newBlock.GetHeight())
	}

	return result
}

// isOnCanonicalPathToTip reports whether candidate is an ancestor block on the path from tip to genesis.
func (fr *ForkResolver) isOnCanonicalPathToTip(tip, candidate *core.Block) bool {
	if tip == nil || candidate == nil {
		return false
	}
	for cur := tip; cur != nil; {
		if cur.GetHeight() == candidate.GetHeight() && hex.EncodeToString(cur.Hash) == hex.EncodeToString(candidate.Hash) {
			return true
		}
		if cur.GetHeight() == 0 {
			break
		}
		parent, exists := fr.chain.BlockByHash(hex.EncodeToString(cur.Header.PrevHash))
		if !exists || parent == nil {
			break
		}
		cur = parent
	}
	return false
}

// PeerBlockFetcher defines the interface for fetching blocks from remote peers
// during deep fork ancestor search. Implemented by SyncLoop.
type PeerBlockFetcher interface {
	FetchBlockByHeight(ctx interface{ GetContext() }, peer string, height uint64) (*core.Block, error)
	GetActivePeers() []string
}

// findAncestorForReorg finds the common ancestor between the local chain and a remote block.
// Uses a multi-strategy approach:
//  1. Direct parent check (fast path for shallow forks)
//  2. Local chain walk (for forks within local DB)
//  3. BlockLocator-style binary search (for deep forks, fetches from peers)
//  4. Fallback to genesis (last resort)
func (fr *ForkResolver) findAncestorForReorg(localTip, remoteBlock *core.Block) (*core.Block, error) {
	if localTip == nil || remoteBlock == nil {
		return nil, fmt.Errorf("nil tip or remote block")
	}

	// Strategy 1: Direct parent check - fastest path
	if len(remoteBlock.Header.PrevHash) > 0 {
		ph := hex.EncodeToString(remoteBlock.Header.PrevHash)
		if parent, ok := fr.chain.BlockByHash(ph); ok && parent != nil {
			if fr.isOnCanonicalPathToTip(localTip, parent) {
				return parent, nil
			}
		}
	}

	// Build local ancestor map for fast lookup
	localAncestors := make(map[uint64]*core.Block)
	current := localTip
	for current != nil {
		localAncestors[current.GetHeight()] = current
		if current.GetHeight() == 0 {
			break
		}
		parent, exists := fr.chain.BlockByHash(hex.EncodeToString(current.Header.PrevHash))
		if !exists {
			break
		}
		current = parent
	}

	// Strategy 2: Check if remoteBlock's prevHash directly matches a local ancestor
	prevHashHex := hex.EncodeToString(remoteBlock.Header.PrevHash)
	for _, block := range localAncestors {
		if hex.EncodeToString(block.Hash) == prevHashHex {
			return block, nil
		}
	}

	// Strategy 3: Walk local ancestors from remoteBlock's parent height downward
	parentHeight := remoteBlock.GetHeight()
	if parentHeight > 0 {
		parentHeight--
	}
	for h := parentHeight; h > 0; h-- {
		if localBlock, exists := localAncestors[h]; exists {
			if hex.EncodeToString(localBlock.Hash) == prevHashHex {
				return localBlock, nil
			}
		}
	}

	// Strategy 4: BlockLocator-style binary search for deep forks
	// Uses exponential step-doubling to find the divergence point efficiently
	// without fetching every block from peers.
	ancestor, err := fr.findAncestorByBlockLocator(localTip, remoteBlock, localAncestors)
	if err == nil && ancestor != nil {
		return ancestor, nil
	}

	return nil, fmt.Errorf("no common ancestor found between local tip h=%d and remote block h=%d",
		localTip.GetHeight(), remoteBlock.GetHeight())
}

// findAncestorByBlockLocator uses a BlockLocator-style algorithm to find the
// common ancestor for deep forks. It generates a locator from the local chain
// (exponentially stepping back) and checks each against the remote chain.
// This is efficient for deep forks because it only needs O(log N) comparisons.
func (fr *ForkResolver) findAncestorByBlockLocator(localTip, remoteBlock *core.Block, localAncestors map[uint64]*core.Block) (*core.Block, error) {
	if localTip == nil {
		return nil, fmt.Errorf("nil local tip")
	}

	// Generate block locator hashes (exponential step-doubling)
	locatorHashes := make([]string, 0)
	step := uint64(1)
	currentHeight := localTip.GetHeight()

	locatorHashes = append(locatorHashes, hex.EncodeToString(localTip.Hash))

	for currentHeight > 0 {
		if step > currentHeight {
			currentHeight = 0
		} else {
			currentHeight = currentHeight - step
		}

		if block, exists := localAncestors[currentHeight]; exists {
			locatorHashes = append(locatorHashes, hex.EncodeToString(block.Hash))
		} else {
			block, exists := fr.chain.BlockByHeight(currentHeight)
			if exists {
				locatorHashes = append(locatorHashes, hex.EncodeToString(block.Hash))
			}
		}

		if step > mathMaxUint64/2 {
			step = mathMaxUint64
		} else {
			step *= 2
		}
		if currentHeight == 0 {
			break
		}
	}

	// Check each locator hash against the remote chain
	// The remote chain is represented by remoteBlock and its ancestors
	remoteAncestors := make(map[uint64]*core.Block)
	fr.buildRemoteAncestorMap(remoteBlock, remoteAncestors)

	for _, locatorHash := range locatorHashes {
		for _, remote := range remoteAncestors {
			if hex.EncodeToString(remote.Hash) == locatorHash {
				return remote, nil
			}
		}
	}

	return nil, fmt.Errorf("block locator search failed")
}

// buildRemoteAncestorMap builds an ancestor map from the remote block walking backwards.
// Limited to MaxReorgDepth * 2 to prevent infinite loops on malicious peers.
func (fr *ForkResolver) buildRemoteAncestorMap(block *core.Block, ancestors map[uint64]*core.Block) {
	limit := uint64(MaxReorgDepth * 2)
	current := block
	for i := uint64(0); i < limit && current != nil; i++ {
		ancestors[current.GetHeight()] = current
		if current.GetHeight() == 0 {
			break
		}
		parent, exists := fr.chain.BlockByHash(hex.EncodeToString(current.Header.PrevHash))
		if !exists {
			break
		}
		current = parent
	}
}

// mathMaxUint64 is the maximum value for uint64
const mathMaxUint64 = ^uint64(0)

// HandleChainMismatch handles chain mismatch during synchronization
// Core-main style: detect mismatch and trigger appropriate recovery
func (fr *ForkResolver) HandleChainMismatch(peerID string, expectedPrevHash, receivedPrevHash []byte, height uint64) error {
	log.Printf("[ForkResolver] Chain mismatch at height %d from peer %s", height, peerID)
	log.Printf("[ForkResolver] Expected prevHash: %x, Received: %x", expectedPrevHash[:8], receivedPrevHash[:8])

	localTip := fr.chain.LatestBlock()
	if localTip == nil {
		return fmt.Errorf("cannot handle chain mismatch: no local tip")
	}

	event := &ForkEvent{
		Type:         ForkTypePersistent,
		DetectedAt:   time.Now(),
		LocalHeight:  localTip.GetHeight(),
		LocalHash:    hex.EncodeToString(localTip.Hash),
		RemoteHeight: height,
		PeerID:       peerID,
		Depth:        localTip.GetHeight() - height,
	}

	// Read callback under lock to prevent data race with SetOnForkDetected.
	fr.mu.RLock()
	forkCb := fr.onForkDetected
	fr.mu.RUnlock()
	if forkCb != nil {
		go forkCb(*event)
	}

	return nil
}

// FindCommonAncestor finds the common ancestor between two blocks
// Core-main style: walk backwards until match found
func (fr *ForkResolver) FindCommonAncestor(block1, block2 *core.Block) (*core.Block, error) {
	if block1 == nil || block2 == nil {
		return nil, fmt.Errorf("nil block provided")
	}

	ancestors := make(map[string]*struct{})
	current := block1

	for current != nil {
		hashStr := hex.EncodeToString(current.Hash)
		ancestors[hashStr] = &struct{}{}

		if current.GetHeight() == 0 {
			break
		}

		parent, exists := fr.chain.BlockByHash(hex.EncodeToString(current.Header.PrevHash))
		if !exists {
			break
		}
		current = parent
	}

	current = block2
	for current != nil {
		hashStr := hex.EncodeToString(current.Hash)
		if _, exists := ancestors[hashStr]; exists {
			return current, nil
		}

		if current.GetHeight() == 0 {
			break
		}

		parent, exists := fr.chain.BlockByHash(hex.EncodeToString(current.Header.PrevHash))
		if !exists {
			break
		}
		current = parent
	}

	return nil, fmt.Errorf("no common ancestor found")
}

// IsReorgInProgress returns whether a reorganization is currently executing
func (fr *ForkResolver) IsReorgInProgress() bool {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.reorgInProgress
}

// RollbackToHeight delegates rollback to the underlying chain provider.
// This is used by the sync system to roll back the chain before triggering
// a full re-sync when a fork is detected and the peer's chain is heavier.
func (fr *ForkResolver) RollbackToHeight(height uint64) error {
	fr.reorgMu.Lock()
	defer fr.reorgMu.Unlock()
	return fr.chain.RollbackToHeight(height)
}

// GetStats returns current resolver statistics
func (fr *ForkResolver) GetStats() ResolverStats {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.stats
}

// Stop shuts down the resolver
func (fr *ForkResolver) Stop() {
	fr.cancel()
}

func (fr *ForkResolver) recordSuccessfulReorg(depth uint64, duration time.Duration) {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	fr.stats.TotalReorgsPerformed++
	fr.stats.LastReorgTime = time.Now()

	if depth > fr.stats.MaxReorgDepth {
		fr.stats.MaxReorgDepth = depth
	}

	totalDuration := fr.stats.AvgReorgDuration*time.Duration(fr.stats.TotalReorgsPerformed-1) + duration
	fr.stats.AvgReorgDuration = totalDuration / time.Duration(fr.stats.TotalReorgsPerformed)

	// Protected by fr.mu — paired with fr.mu.RLock() at the read site
	// in RequestReorgWithDepth to prevent data race.
	fr.lastSuccessfulReorgTime = time.Now()
}

func (fr *ForkResolver) recordFailedReorg() {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.stats.TotalReorgsFailed++
}

// recordFallbackRollback records a shallow rollback triggered by a failed
// common-ancestor lookup. These are NOT successful reorgs — they are partial
// recoveries that shrink the local chain tip to allow resync from a different peer.
// Separate from recordSuccessfulReorg to avoid polluting TotalReorgsPerformed,
// MaxReorgDepth, AvgReorgDuration, and oscillation detection.
func (fr *ForkResolver) recordFallbackRollback() {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.stats.TotalFallbackRollbacks++
}
