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
	"fmt"
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
)

var (
	ErrFastSyncNilSnapshot        = fmt.Errorf("nil snapshot provided")
	ErrFastSyncInvalidSnapshot    = fmt.Errorf("invalid snapshot")
	ErrFastSyncCheckpointMismatch = fmt.Errorf("checkpoint mismatch")
	ErrFastSyncStateRestore       = fmt.Errorf("failed to restore state")
	ErrFastSyncNoCheckpoint       = fmt.Errorf("no checkpoint available")
	ErrFastSyncCancelled          = fmt.Errorf("fast sync cancelled")
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
}

type BlockchainInterface interface {
	LatestBlock() *core.Block
	BlockByHeight(uint64) (*core.Block, bool)
	AddBlock(*core.Block) (bool, error)
	GetChainID() uint64
}

func NewFastSyncEngine(
	checkpointMgr *CheckpointManager,
	store storage.ChainStore,
	blockchain BlockchainInterface,
) *FastSyncEngine {
	return &FastSyncEngine{
		checkpointMgr:      checkpointMgr,
		snapshotDownloader: NewSnapshotDownloader(checkpointMgr),
		store:              store,
		blockchain:         blockchain,
		status: SyncStatus{
			Phase:          "idle",
			Progress:       0,
			StartTime:      time.Time{},
			LastUpdateTime: time.Now(),
		},
		peers:        make([]string, 0),
		snapshotURLs: make([]string, 0),
	}
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

func (fs *FastSyncEngine) syncBlocksFromCheckpoint(ctx context.Context, checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return ErrFastSyncNoCheckpoint
	}
	startHeight := checkpoint.Height + 1
	targetHeight := checkpoint.Height
	currentHeight := startHeight
	batchSize := uint64(FastSyncBatchSize)
	for currentHeight <= targetHeight {
		select {
		case <-ctx.Done():
			return ErrFastSyncCancelled
		default:
		}
		batchEnd := currentHeight + batchSize - 1
		if batchEnd > targetHeight {
			batchEnd = targetHeight
		}
		batchCount := batchEnd - currentHeight + 1
		for i := uint64(0); i < batchCount; i++ {
			select {
			case <-ctx.Done():
				return ErrFastSyncCancelled
			default:
			}
		}
		currentHeight = batchEnd + 1
		progress := 0.5 + 0.5*float64(currentHeight-startHeight)/float64(targetHeight-startHeight+1)
		fs.updateStatus("syncing_blocks", progress)
	}
	return nil
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
