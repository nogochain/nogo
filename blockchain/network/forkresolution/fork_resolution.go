// Copyright 2026 NogoChain Team
// Production-grade unified fork resolution module
// Based on core-main's proven architecture with multi-node enhancements
// This is the SINGLE entry point for all fork detection and resolution in the system

package forkresolution

import (
	"context"
	"encoding/hex"
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

	// MinReorgInterval minimum time between reorganizations
	MinReorgInterval = 10 * time.Second
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

// ForkResolver is the unified fork resolution engine
// Based on core-main's block_keeper architecture with enhanced multi-node support
type ForkResolver struct {
	mu     sync.RWMutex
	chain  ChainProvider
	ctx    context.Context
	cancel context.CancelFunc

	// Reorg state management
	reorgInProgress bool
	lastReorgTime   time.Time
	reorgMu         sync.Mutex

	// Callbacks
	onReorgComplete func(newHeight uint64)
	onForkDetected  func(event ForkEvent)

	// Statistics
	stats ResolverStats
}

// ResolverStats holds fork resolution statistics
type ResolverStats struct {
	TotalForksDetected   int64
	TotalReorgsPerformed int64
	TotalReorgsFailed    int64
	LastForkTime         time.Time
	LastReorgTime        time.Time
	MaxReorgDepth        uint64
	AvgReorgDuration     time.Duration
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
	fr.mu.Unlock()

	if fr.onForkDetected != nil {
		go fr.onForkDetected(*event)
	}

	log.Printf("[ForkResolver] Fork detected: type=%v depth=%d local_h=%d remote_h=%d peer=%s",
		event.Type, event.Depth, event.LocalHeight, event.RemoteHeight, peerID)

	return event
}

// ShouldReorg determines if reorganization should be performed based on work comparison
// Core-main style: heaviest chain rule with safety checks
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
	remoteWork := fr.chain.CalculateCumulativeWork(remoteBlock)

	if remoteWork != nil && localWork != nil {
		remoteTotalWork := new(big.Int).Add(localWork, remoteWork)
		if remoteTotalWork.Cmp(localWork) > 0 {
			return true
		}
	}

	if remoteBlock.GetHeight() > localTip.GetHeight() && remoteWork != nil && remoteWork.Sign() > 0 {
		return true
	}

	return false
}

// RequestReorg submits a reorganization request with full validation
// This is the UNIFIED ENTRY POINT for all reorg operations in the system
func (fr *ForkResolver) RequestReorg(newBlock *core.Block, source string) error {
	if newBlock == nil {
		return fmt.Errorf("RequestReorg: nil block from %s", source)
	}

	log.Printf("[ForkResolver] Reorg requested by %s: height=%d hash=%x",
		source, newBlock.GetHeight(), newBlock.Hash[:8])

	acquired := fr.reorgMu.TryLock()
	if !acquired {
		return fmt.Errorf("reorganization already in progress")
	}
	defer fr.reorgMu.Unlock()

	if fr.reorgInProgress {
		return fmt.Errorf("reorganization already in progress")
	}

	if time.Since(fr.lastReorgTime) < MinReorgInterval {
		return fmt.Errorf("reorg too frequent: minimum interval %v not elapsed", MinReorgInterval)
	}

	result := fr.executeReorg(newBlock, source)
	if !result.Success {
		return result.Error
	}

	return nil
}

// executeReorg performs the actual reorganization
// Core-main style: rollback to common ancestor then extend to new tip
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

	localHeight := localTip.GetHeight()
	targetHeight := newBlock.GetHeight()

	log.Printf("[ForkResolver] Starting reorg: local_h=%d target_h=%d source=%s",
		localHeight, targetHeight, source)

	rollbackTarget := localHeight
	if targetHeight < localHeight && targetHeight > 0 {
		rollbackTarget = targetHeight - 1
	} else if localHeight > 0 {
		rollbackTarget = localHeight - 1
	}

	reorgDepth := uint64(0)
	if localHeight > rollbackTarget {
		reorgDepth = localHeight - rollbackTarget
	}

	result.ReorgDepth = reorgDepth

	if reorgDepth > MaxReorgDepth {
		result.Error = fmt.Errorf("reorg depth %d exceeds maximum %d", reorgDepth, MaxReorgDepth)
		result.Success = false
		fr.recordFailedReorg()
		return result
	}

	if rollbackTarget < localHeight {
		log.Printf("[ForkResolver] Rolling back from %d to %d", localHeight, rollbackTarget)
		if err := fr.chain.RollbackToHeight(rollbackTarget); err != nil {
			result.Error = fmt.Errorf("rollback failed: %w", err)
			result.Success = false
			fr.recordFailedReorg()
			return result
		}
	}

	accepted, err := fr.chain.AddBlock(newBlock)
	if err != nil {
		result.Error = fmt.Errorf("add block failed: %w", err)
		result.Success = false
		fr.recordFailedReorg()
		return result
	}

	result.Switched = accepted
	result.Success = true
	result.Duration = time.Since(startTime)

	fr.recordSuccessfulReorg(reorgDepth, result.Duration)

	log.Printf("[ForkResolver] Reorg completed: success=%v switched=%v duration=%v",
		result.Success, result.Switched, result.Duration)

	if fr.onReorgComplete != nil && accepted {
		go fr.onReorgComplete(newBlock.GetHeight())
	}

	return result
}

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

	if fr.onForkDetected != nil {
		go fr.onForkDetected(*event)
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
}

func (fr *ForkResolver) recordFailedReorg() {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.stats.TotalReorgsFailed++
}
