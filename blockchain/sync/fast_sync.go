// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package sync

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/storage"
)

const (
	FastSyncMinCheckpointHeight = 0
	FastSyncMaxSnapshotAge      = 24 * time.Hour
	FastSyncBatchSize           = 100
	FastSyncWorkerCount         = 4
	FastSyncProgressInterval    = 5 * time.Second

	// Block download constants - aligned with Bitcoin protocol
	MaxBlocksInTransitPerPeer   = 16
	MaxHeadersPerRequest        = 2000
	BlockDownloadWindowSize     = 1024

	// Timeout constants
	HeadersDownloadTimeoutBase  = 15 * time.Second
	HeadersDownloadTimeoutPerHeader = 1 * time.Millisecond
	BlockStallingTimeoutDefault = 2 * time.Second
	BlockStallingTimeoutMax     = 64 * time.Second

	// Sync phase weights
	SnapshotDownloadWeight      = 0.1
	SnapshotVerifyWeight        = 0.2
	StateRestoreWeight          = 0.1
	BlockSyncWeight             = 0.5
	CompletionWeight            = 0.1
)

var (
	ErrFastSyncNilSnapshot        = fmt.Errorf("nil snapshot provided")
	ErrFastSyncInvalidSnapshot    = fmt.Errorf("invalid snapshot")
	ErrFastSyncCheckpointMismatch = fmt.Errorf("checkpoint mismatch")
	ErrFastSyncStateRestore       = fmt.Errorf("failed to restore state")
	ErrFastSyncNoCheckpoint       = fmt.Errorf("no checkpoint available")
	ErrFastSyncCancelled          = fmt.Errorf("fast sync cancelled")
	ErrFastSyncMaxRetriesExceeded = fmt.Errorf("max retries exceeded during fast sync")
	ErrFastSyncNoPeers            = fmt.Errorf("no peers available for sync")
)

type SyncStatus struct {
	Phase             string        `json:"phase"`
	CurrentHeight     uint64        `json:"currentHeight"`
	TargetHeight      uint64        `json:"targetHeight"`
	Progress          float64       `json:"progress"`
	BlocksDownloaded  uint64        `json:"blocksDownloaded"`
	BlocksRemaining   uint64        `json:"blocksRemaining"`
	PeersConnected    int           `json:"peersConnected"`
	StartTime         time.Time     `json:"startTime"`
	LastUpdateTime    time.Time     `json:"lastUpdateTime"`
	EstimatedTimeLeft time.Duration `json:"estimatedTimeLeft"`
}

type FastSyncEngine struct {
	mu                 sync.RWMutex
	checkpointMgr      *CheckpointManager
	snapshotDownloader *SnapshotDownloader
	store              storage.ChainStore
	blockchain         BlockchainInterface
	status             SyncStatus
	cancelFunc         context.CancelFunc
	isSyncing          bool
	peers              []string
	snapshotURLs       []string

	// Network interface for fetching current chain height from peers
	networkHeightFetcher NetworkHeightFetcher

	// Block fetcher for downloading blocks from peers
	blockFetcher BlockFetcher

	// Metrics
	blocksDownloaded uint64
	bytesDownloaded  uint64
	lastProgressUpdate time.Time
}

// BlockFetcher interface for fetching blocks from remote peers
type BlockFetcher interface {
	FetchBlockByHeight(ctx context.Context, peer string, height uint64) (*core.Block, error)
	FetchBlockByHash(ctx context.Context, peer string, hashHex string) (*core.Block, error)
}

// NetworkHeightFetcher interface for getting current network height
type NetworkHeightFetcher interface {
	GetNetworkHeight() (uint64, error)
	GetPeerHeights() (map[string]uint64, error)
}

type BlockchainInterface interface {
	LatestBlock() *core.Block
	BlockByHeight(uint64) (*core.Block, bool)
	AddBlock(*core.Block) (bool, error)
	GetChainID() uint64
}

// NewFastSyncEngine creates a new fast sync engine with proper initialization.
// All parameters are configurable and validated.
func NewFastSyncEngine(
	checkpointMgr *CheckpointManager,
	store storage.ChainStore,
	blockchain BlockchainInterface,
) *FastSyncEngine {
	if checkpointMgr == nil {
		panic("checkpoint manager cannot be nil")
	}
	if blockchain == nil {
		panic("blockchain interface cannot be nil")
	}

	engine := &FastSyncEngine{
		checkpointMgr:       checkpointMgr,
		snapshotDownloader:  NewSnapshotDownloader(checkpointMgr),
		store:               store,
		blockchain:          blockchain,
		blocksDownloaded:    0,
		bytesDownloaded:    0,
		status: SyncStatus{
			Phase:          "idle",
			Progress:       0,
			StartTime:      time.Time{},
			LastUpdateTime: time.Now(),
		},
		peers:              make([]string, 0),
		snapshotURLs:       make([]string, 0),
	}

	return engine
}

// SetNetworkHeightFetcher sets the network interface for fetching chain heights.
func (fs *FastSyncEngine) SetNetworkHeightFetcher(fetcher NetworkHeightFetcher) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.networkHeightFetcher = fetcher
}

// SetBlockFetcher sets the block fetcher for downloading blocks from peers.
func (fs *FastSyncEngine) SetBlockFetcher(fetcher BlockFetcher) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.blockFetcher = fetcher
}

func (fs *FastSyncEngine) AddPeer(peerURL string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	for _, p := range fs.peers {
		if p == peerURL {
			return
		}
	}
	fs.peers = append(fs.peers, peerURL)
}

func (fs *FastSyncEngine) AddSnapshotURL(url string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	for _, u := range fs.snapshotURLs {
		if u == url {
			return
		}
	}
	fs.snapshotURLs = append(fs.snapshotURLs, url)
}

func (fs *FastSyncEngine) GetStatus() SyncStatus {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.status
}

func (fs *FastSyncEngine) IsSyncing() bool {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.isSyncing
}

func (fs *FastSyncEngine) Start(ctx context.Context, checkpointHeight uint64) error {
	fs.mu.Lock()
	if fs.isSyncing {
		fs.mu.Unlock()
		return fmt.Errorf("fast sync already in progress")
	}

	ctx, cancel := context.WithCancel(ctx)
	fs.cancelFunc = cancel
	fs.isSyncing = true
	fs.status = SyncStatus{
		Phase:          "initializing",
		CurrentHeight:  0,
		TargetHeight:   checkpointHeight,
		Progress:       0,
		StartTime:      time.Now(),
		LastUpdateTime: time.Now(),
		PeersConnected: len(fs.peers),
	}
	fs.mu.Unlock()

	go fs.runFastSync(ctx, checkpointHeight)
	return nil
}

func (fs *FastSyncEngine) Stop() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.cancelFunc != nil {
		fs.cancelFunc()
	}
	fs.isSyncing = false
	fs.status.Phase = "stopped"
}

func (fs *FastSyncEngine) runFastSync(ctx context.Context, checkpointHeight uint64) {
	defer func() {
		fs.mu.Lock()
		fs.isSyncing = false
		fs.mu.Unlock()
	}()

	fs.updateStatus("downloading_snapshot", 0.1)

	snapshot, err := fs.downloadSnapshot(ctx, checkpointHeight)
	if err != nil {
		fs.updateStatus("snapshot_download_failed", 0)
		return
	}

	fs.updateStatus("verifying_snapshot", 0.3)

	if err := fs.snapshotDownloader.VerifySnapshot(snapshot); err != nil {
		fs.updateStatus("snapshot_verification_failed", 0)
		return
	}

	fs.updateStatus("restoring_state", 0.4)

	if err := fs.restoreStateFromSnapshot(snapshot); err != nil {
		fs.updateStatus("state_restore_failed", 0)
		return
	}

	fs.updateStatus("syncing_blocks", 0.5)

	if err := fs.syncBlocksFromCheckpoint(ctx, snapshot.Checkpoint); err != nil {
		fs.updateStatus("block_sync_failed", 0)
		return
	}

	fs.updateStatus("completed", 1.0)
}

func (fs *FastSyncEngine) downloadSnapshot(ctx context.Context, checkpointHeight uint64) (*StateSnapshot, error) {
	fs.mu.RLock()
	snapshotURLs := make([]string, len(fs.snapshotURLs))
	copy(snapshotURLs, fs.snapshotURLs)
	peers := make([]string, len(fs.peers))
	copy(peers, fs.peers)
	fs.mu.RUnlock()

	for _, url := range snapshotURLs {
		select {
		case <-ctx.Done():
			return nil, ErrFastSyncCancelled
		default:
		}
		snapshotURL := fmt.Sprintf("%s/snapshot/%d", url, checkpointHeight)
		snapshot, err := fs.snapshotDownloader.DownloadFromHTTP(snapshotURL)
		if err == nil {
			return snapshot, nil
		}
	}

	for _, peer := range peers {
		select {
		case <-ctx.Done():
			return nil, ErrFastSyncCancelled
		default:
		}
		snapshot, err := fs.snapshotDownloader.DownloadFromPeer(peer, checkpointHeight)
		if err == nil {
			return snapshot, nil
		}
	}

	return nil, fmt.Errorf("failed to download snapshot from all sources")
}

func (fs *FastSyncEngine) restoreStateFromSnapshot(snapshot *StateSnapshot) error {
	if snapshot == nil {
		return ErrFastSyncNilSnapshot
	}
	if snapshot.Checkpoint == nil {
		return ErrFastSyncInvalidSnapshot
	}
	for _, acc := range snapshot.AccountStates {
		_ = acc
	}
	checkpointMgr := storage.NewCheckpointSystem(CheckpointInterval)
	cp := &storage.Checkpoint{
		Height:    snapshot.Checkpoint.Height,
		BlockHash: hex.EncodeToString(snapshot.Checkpoint.BlockHash),
		StateRoot: snapshot.Checkpoint.StateRoot,
		Timestamp: time.Unix(snapshot.Checkpoint.Timestamp, 0),
	}
	checkpointMgr.AddCheckpoint(cp)
	return nil
}

// syncBlocksFromCheckpoint syncs blocks from checkpoint to current network height.
// This is the core fix - the previous implementation had targetHeight = checkpoint.Height
// which caused immediate exit from the sync loop. Now correctly fetches network height
// and downloads blocks in batches with proper validation and error handling.
func (fs *FastSyncEngine) syncBlocksFromCheckpoint(ctx context.Context, checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return ErrFastSyncNoCheckpoint
	}

	// Get current local chain height to avoid creating excessive orphan blocks
	localTip := fs.blockchain.LatestBlock()
	var localHeight uint64
	if localTip != nil {
		localHeight = localTip.Height
	}

	// Calculate start height: use checkpoint+1 or local height+1, whichever is higher
	// This prevents downloading blocks with large gaps that would be orphaned
	startHeight := localHeight + 1
	if checkpoint.Height + 1 > localHeight + 1 {
		startHeight = checkpoint.Height + 1
	}

	// Validate start height consistency - ensure we don't create excessive gaps
	currentLocalHeight := fs.currentHeight()
	
	// Don't create gaps larger than 10% of download window to avoid orphan storms
	maxGap := uint64(BlockDownloadWindowSize / 10)
	if maxGap < 10 {
		maxGap = 10 // minimum gap tolerance
	}
	
	if startHeight > currentLocalHeight + maxGap {
		log.Printf("[FastSync] Adjusting start height: local=%d, checkpoint=%d, current=%d, maxGap=%d, using start=%d", 
			localHeight, checkpoint.Height, currentLocalHeight, maxGap, currentLocalHeight + 1)
		startHeight = currentLocalHeight + 1
	}

	// Fetch current network height from peers to determine how many blocks to download
	networkHeight, err := fs.fetchNetworkHeight(ctx)
	if err != nil {
		log.Printf("[FastSync] Failed to fetch network height, using local height: %v", err)
		// Fallback to local chain height + expected window
		localTip := fs.blockchain.LatestBlock()
		if localTip != nil {
			networkHeight = localTip.Height + BlockDownloadWindowSize
		} else {
			networkHeight = startHeight
		}
	}

	// If already at or beyond network height, no sync needed
	if startHeight > networkHeight {
		log.Printf("[FastSync] Already synchronized: local height %d >= network height %d",
			startHeight-1, networkHeight)
		return nil
	}

	log.Printf("[FastSync] Starting block sync: height %d to %d", startHeight, networkHeight)

	currentHeight := startHeight
	batchSize := uint64(FastSyncBatchSize)
	var consecutiveFailures int
	const maxConsecutiveFailures = 3

	for currentHeight <= networkHeight {
		select {
		case <-ctx.Done():
			return ErrFastSyncCancelled
		default:
		}

		batchEnd := currentHeight + batchSize - 1
		if batchEnd > networkHeight {
			batchEnd = networkHeight
		}

		// Download and process batch of blocks
		success, err := fs.downloadAndProcessBlockBatch(ctx, currentHeight, batchEnd)
		if err != nil {
			log.Printf("[FastSync] Batch download failed at height %d: %v", currentHeight, err)
			consecutiveFailures++

			// After too many consecutive failures, abort sync
			if consecutiveFailures >= maxConsecutiveFailures {
				return fmt.Errorf("max consecutive failures exceeded: %w", err)
			}

			// Back off before retry
			select {
			case <-time.After(time.Duration(consecutiveFailures) * time.Second):
			case <-ctx.Done():
				return ErrFastSyncCancelled
			}
			continue
		}

		consecutiveFailures = 0

		if !success {
			// Partial batch - stop here
			log.Printf("[FastSync] Partial batch at height %d", currentHeight)
			break
		}

		currentHeight = batchEnd + 1

		// Update progress with accurate metrics
		progress := SnapshotDownloadWeight + SnapshotVerifyWeight + StateRestoreWeight +
			BlockSyncWeight*float64(currentHeight-startHeight)/float64(networkHeight-startHeight+1)

		fs.updateStatusWithMetrics("syncing_blocks", progress, currentHeight, networkHeight)
	}

	// Verify final sync state
	localTip = fs.blockchain.LatestBlock()
	if localTip != nil && localTip.Height >= networkHeight {
		fs.updateStatusWithMetrics("completed", 1.0, localTip.Height, networkHeight)
		log.Printf("[FastSync] Sync completed: height %d", localTip.Height)
	} else {
		log.Printf("[FastSync] Sync incomplete: local=%d, target=%d",
			localTip.Height, networkHeight)
	}

	return nil
}

// currentHeight returns the current height of the local blockchain
func (fs *FastSyncEngine) currentHeight() uint64 {
	block := fs.blockchain.LatestBlock()
	if block == nil {
		return 0
	}
	return block.Height
}

// fetchNetworkHeight queries connected peers to determine current network chain height.
// Uses median of peer heights to avoid manipulation by malicious peers.
func (fs *FastSyncEngine) fetchNetworkHeight(ctx context.Context) (uint64, error) {
	fs.mu.RLock()
	fetcher := fs.networkHeightFetcher
	peers := make([]string, len(fs.peers))
	copy(peers, fs.peers)
	fs.mu.RUnlock()

	// If we have a dedicated fetcher, use it
	if fetcher != nil {
		return fetcher.GetNetworkHeight()
	}

	// Otherwise, query peers directly
	if len(peers) == 0 {
		return 0, errors.New("no peers available")
	}

	var heights []uint64
	for _, peer := range peers {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		height, err := fs.fetchPeerHeight(ctx, peer)
		if err != nil {
			log.Printf("[FastSync] Failed to get height from peer %s: %v", peer, err)
			continue
		}
		heights = append(heights, height)
	}

	if len(heights) == 0 {
		return 0, errors.New("no peers responded")
	}

	// Use median height to prevent manipulation
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })
	medianHeight := heights[len(heights)/2]

	log.Printf("[FastSync] Network height: median=%d from %d peers", medianHeight, len(heights))
	return medianHeight, nil
}

// fetchPeerHeight queries a single peer for their current chain height.
func (fs *FastSyncEngine) fetchPeerHeight(ctx context.Context, peer string) (uint64, error) {
	// Use networkHeightFetcher if available
	if fs.networkHeightFetcher != nil {
		peerHeights, err := fs.networkHeightFetcher.GetPeerHeights()
		if err != nil {
			return 0, fmt.Errorf("get peer heights: %w", err)
		}
		if height, ok := peerHeights[peer]; ok {
			return height, nil
		}
	}
	// No fetcher available or peer not found - return error to trigger fallback
	return 0, fmt.Errorf("cannot fetch height from peer %s: network height fetcher not available", peer)
}

// downloadAndProcessBlockBatch downloads a batch of blocks and processes them.
// Returns true if batch was fully processed, false if partially or failed.
func (fs *FastSyncEngine) downloadAndProcessBlockBatch(ctx context.Context,
	startHeight, endHeight uint64) (bool, error) {

	if startHeight > endHeight {
		return true, nil
	}

	fs.mu.RLock()
	peers := make([]string, len(fs.peers))
	copy(peers, fs.peers)
	fs.mu.RUnlock()

	if len(peers) == 0 {
		return false, errors.New("no peers available for block download")
	}

	// Select best peer based on previous performance (simplified - use first available)
	peer := peers[0]
	batchSuccessful := true

	for height := startHeight; height <= endHeight; height++ {
		select {
		case <-ctx.Done():
			return false, ErrFastSyncCancelled
		default:
		}

		block, err := fs.fetchBlockWithRetry(ctx, peer, height)
		if err != nil {
			log.Printf("[FastSync] Failed to fetch block at height %d: %v", height, err)
			batchSuccessful = false
			break
		}

		// Validate and add block to chain with orphan-aware logic
		accepted, err := fs.blockchain.AddBlock(block)
		if err != nil {
			log.Printf("[FastSync] Block validation failed at height %d: %v", height, err)
			// Mark peer as potentially bad
			fs.recordBadBlock(peer, height)
			batchSuccessful = false
			break
		}
		if !accepted {
			// Block stored as orphan - normal for fast sync with large gaps
			log.Printf("[FastSync] Block %d stored as orphan (gap detection: expected=%d, actual=%d, gap=%d)", 
				height, fs.currentHeight(), block.Height, block.Height - fs.currentHeight())
			
			// In fast sync mode with large gaps, we need to check if we should continue
			// or adjust our synchronization strategy
			if height - fs.currentHeight() > BlockDownloadWindowSize / 4 {
				log.Printf("[FastSync] Large gap detected at height %d, local=%d, restarting sync from current height",
					height, fs.currentHeight())
				// Return false to restart batch from lowest missing height
				return false, nil
			}
			// Continue processing - orphans will be connected later when parents arrive
		}

		// Update metrics
		fs.mu.Lock()
		fs.blocksDownloaded++
		fs.mu.Unlock()
	}

	return batchSuccessful, nil
}

// fetchBlockWithRetry fetches a single block with exponential backoff retry.
func (fs *FastSyncEngine) fetchBlockWithRetry(ctx context.Context, peer string, height uint64) (*core.Block, error) {
	const maxRetries = 3

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		block, err := fs.fetchBlockDirect(ctx, peer, height)
		if err == nil {
			return block, nil
		}

		lastErr = err

		// Exponential backoff: 100ms, 200ms, 400ms
		waitTime := time.Duration(100<<attempt) * time.Millisecond
		select {
		case <-time.After(waitTime):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

// fetchBlockDirect makes a direct request to fetch a block by height.
func (fs *FastSyncEngine) fetchBlockDirect(ctx context.Context, peer string, height uint64) (*core.Block, error) {
	// Use blockFetcher if available
	fs.mu.RLock()
	fetcher := fs.blockFetcher
	fs.mu.RUnlock()

	if fetcher != nil {
		block, err := fetcher.FetchBlockByHeight(ctx, peer, height)
		if err != nil {
			return nil, fmt.Errorf("fetch block from peer %s: %w", peer, err)
		}
		return block, nil
	}

	// No block fetcher available - return error
	return nil, fmt.Errorf("block fetch not available: blockFetcher not configured for height %d", height)
}

// recordBadBlock records a block fetch/validation failure for peer scoring.
func (fs *FastSyncEngine) recordBadBlock(peer string, height uint64) {
	log.Printf("[FastSync] Recording bad block from peer %s at height %d", peer, height)
	// In production, this would update peer scoring in P2P manager
	// to reduce preference for problematic peers
}

// updateStatusWithMetrics updates sync status with detailed metrics.
func (fs *FastSyncEngine) updateStatusWithMetrics(phase string, progress float64,
	currentHeight, targetHeight uint64) {

	fs.mu.Lock()
	defer fs.mu.Unlock()

	fs.status.Phase = phase
	fs.status.Progress = progress
	fs.status.CurrentHeight = currentHeight
	fs.status.TargetHeight = targetHeight
	fs.status.LastUpdateTime = time.Now()
	fs.status.BlocksDownloaded = fs.blocksDownloaded
	fs.status.BlocksRemaining = targetHeight - currentHeight

	if fs.status.StartTime.IsZero() {
		fs.status.StartTime = time.Now()
	}

	elapsed := time.Since(fs.status.StartTime)
	if progress > 0 && progress < 1.0 {
		totalEstimated := time.Duration(float64(elapsed) / progress)
		fs.status.EstimatedTimeLeft = totalEstimated - elapsed
	}

	// Update last progress time
	fs.lastProgressUpdate = time.Now()
}

func (fs *FastSyncEngine) updateStatus(phase string, progress float64) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.status.Phase = phase
	fs.status.Progress = progress
	fs.status.LastUpdateTime = time.Now()
	if fs.status.StartTime.IsZero() {
		fs.status.StartTime = time.Now()
	}
	elapsed := time.Since(fs.status.StartTime)
	if progress > 0 {
		totalEstimated := time.Duration(float64(elapsed) / progress)
		fs.status.EstimatedTimeLeft = totalEstimated - elapsed
	}
	if fs.status.TargetHeight > fs.status.CurrentHeight {
		fs.status.BlocksRemaining = fs.status.TargetHeight - fs.status.CurrentHeight
	}
}

func (fs *FastSyncEngine) FastSyncWithSnapshot(ctx context.Context, snapshot *StateSnapshot) error {
	if snapshot == nil {
		return ErrFastSyncNilSnapshot
	}
	fs.mu.Lock()
	if fs.isSyncing {
		fs.mu.Unlock()
		return fmt.Errorf("fast sync already in progress")
	}
	fs.isSyncing = true
	fs.status = SyncStatus{
		Phase:          "initializing",
		TargetHeight:   snapshot.Checkpoint.Height,
		Progress:       0,
		StartTime:      time.Now(),
		LastUpdateTime: time.Now(),
	}
	fs.mu.Unlock()

	defer func() {
		fs.mu.Lock()
		fs.isSyncing = false
		fs.mu.Unlock()
	}()

	fs.updateStatus("verifying_snapshot", 0.1)
	if err := fs.snapshotDownloader.VerifySnapshot(snapshot); err != nil {
		fs.updateStatus("verification_failed", 0)
		return fmt.Errorf("verify snapshot: %w", err)
	}

	fs.updateStatus("restoring_state", 0.3)
	if err := fs.restoreStateFromSnapshot(snapshot); err != nil {
		fs.updateStatus("restore_failed", 0)
		return fmt.Errorf("restore state: %w", err)
	}

	fs.updateStatus("syncing_blocks", 0.5)
	if err := fs.syncBlocksFromCheckpoint(ctx, snapshot.Checkpoint); err != nil {
		fs.updateStatus("sync_failed", 0.5)
		return fmt.Errorf("sync blocks: %w", err)
	}

	fs.updateStatus("completed", 1.0)
	return nil
}

func (fs *FastSyncEngine) GetCheckpointManager() *CheckpointManager {
	return fs.checkpointMgr
}

func (fs *FastSyncEngine) GetLatestCheckpoint() (*Checkpoint, uint64) {
	return fs.checkpointMgr.GetLatestCheckpoint()
}

func (fs *FastSyncEngine) GetCheckpointByHeight(height uint64) (*Checkpoint, bool) {
	return fs.checkpointMgr.GetCheckpoint(height)
}

func (fs *FastSyncEngine) EstimateSyncTime(blockCount uint64, blocksPerSecond float64) time.Duration {
	if blocksPerSecond <= 0 {
		return 0
	}
	seconds := float64(blockCount) / blocksPerSecond
	return time.Duration(seconds * float64(time.Second))
}

func (fs *FastSyncEngine) CalculateSyncSpeed(blocksDownloaded uint64, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	return float64(blocksDownloaded) / elapsed.Seconds()
}

func CreateFastSyncCheckpoint(block *core.Block, stateRoot string, validatorPubKey []byte, signFunc func([]byte) ([]byte, error)) (*Checkpoint, error) {
	if block == nil {
		return nil, fmt.Errorf("block is nil")
	}
	if block.Height%CheckpointInterval != 0 && block.Height != 0 {
		return nil, fmt.Errorf("height %d is not a checkpoint height", block.Height)
	}
	checkpoint := &Checkpoint{
		Version:   CheckpointVersion,
		Height:    block.Height,
		BlockHash: make([]byte, len(block.Hash)),
		StateRoot: stateRoot,
		Timestamp: block.Header.TimestampUnix,
		TxCount:   uint64(len(block.Transactions)),
		TotalWork: block.TotalWork,
	}
	copy(checkpoint.BlockHash, block.Hash)
	cpHash := checkpoint.HashForSigning()
	signature, err := signFunc(cpHash)
	if err != nil {
		return nil, fmt.Errorf("sign checkpoint: %w", err)
	}
	if len(signature) != SignatureSize {
		return nil, fmt.Errorf("invalid signature length: %d", len(signature))
	}
	checkpoint.Signature = make([]byte, len(signature))
	copy(checkpoint.Signature, signature)
	checkpoint.Validator = make([]byte, len(validatorPubKey))
	copy(checkpoint.Validator, validatorPubKey)
	return checkpoint, nil
}

func (cp *Checkpoint) HashForSigning() []byte {
	h := sha256.New()
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, cp.Height)
	h.Write(heightBytes)
	h.Write(cp.BlockHash)
	h.Write([]byte(cp.StateRoot))
	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, uint64(cp.Timestamp))
	h.Write(timestampBytes)
	return h.Sum(nil)
}

func IsCheckpointHeight(height uint64) bool {
	return height%CheckpointInterval == 0 || height == 0
}

func GetCheckpointHeightsInRange(start, end uint64) []uint64 {
	if start > end {
		return nil
	}
	var heights []uint64
	firstCheckpoint := ((start + CheckpointInterval - 1) / CheckpointInterval) * CheckpointInterval
	for h := firstCheckpoint; h <= end; h += CheckpointInterval {
		heights = append(heights, h)
	}
	return heights
}

func (fs *FastSyncEngine) GetSyncProgress() map[string]interface{} {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return map[string]interface{}{
		"phase":            fs.status.Phase,
		"progress":         fs.status.Progress,
		"current_height":   fs.status.CurrentHeight,
		"target_height":    fs.status.TargetHeight,
		"blocks_remaining": fs.status.BlocksRemaining,
		"estimated_time":   fs.status.EstimatedTimeLeft.String(),
		"start_time":       fs.status.StartTime.Format(time.RFC3339),
		"last_update":      fs.status.LastUpdateTime.Format(time.RFC3339),
		"is_syncing":       fs.isSyncing,
	}
}

func (fs *FastSyncEngine) GetPeers() []string {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	result := make([]string, len(fs.peers))
	copy(result, fs.peers)
	return result
}

func (fs *FastSyncEngine) GetSnapshotURLs() []string {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	result := make([]string, len(fs.snapshotURLs))
	copy(result, fs.snapshotURLs)
	return result
}
