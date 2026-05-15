package network

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

const (
	skeletonMaxHeadersPerRequest = 512
	skeletonMaxConcurrentFetches = 4
	skeletonBatchSize            = 128
	skeletonProgressInterval     = 50
)

type headerWithHash struct {
	Header core.BlockHeader
	Hash   []byte
}

type SkeletonSync struct {
	mu            sync.RWMutex
	bc            BlockchainInterface
	pm            PeerAPI
	validator     blockValidator
	ctx           context.Context
	cancel        context.CancelFunc
	active        bool
	progress      float64
	startTime     time.Time
	targetHeight  uint64
	currentHeight uint64
}

type blockValidator interface {
	ValidateBlock(block *core.Block, checkPoW bool) error
	ValidateHeader(header *core.BlockHeader, parent *core.Block) error
}

func NewSkeletonSync(bc BlockchainInterface, pm PeerAPI, validator blockValidator) *SkeletonSync {
	return &SkeletonSync{
		bc:        bc,
		pm:        pm,
		validator: validator,
	}
}

func (ss *SkeletonSync) Start(ctx context.Context, targetHeight uint64) error {
	ss.mu.Lock()
	if ss.active {
		ss.mu.Unlock()
		return fmt.Errorf("skeleton sync already in progress")
	}
	ss.ctx, ss.cancel = context.WithCancel(ctx)
	ss.active = true
	ss.startTime = time.Now()
	ss.targetHeight = targetHeight
	ss.progress = 0
	ss.mu.Unlock()

	go ss.runSkeletonSync()
	return nil
}

func (ss *SkeletonSync) Stop() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.cancel != nil {
		ss.cancel()
	}
	ss.active = false
}

func (ss *SkeletonSync) IsActive() bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.active
}

func (ss *SkeletonSync) Progress() float64 {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.progress
}

func (ss *SkeletonSync) runSkeletonSync() {
	defer func() {
		ss.mu.Lock()
		ss.active = false
		ss.mu.Unlock()
	}()

	localTip := ss.bc.LatestBlock()
	if localTip == nil {
		log.Printf("[SkeletonSync] No local tip, cannot start skeleton sync")
		return
	}

	localHeight := localTip.GetHeight()
	if localHeight >= ss.targetHeight {
		log.Printf("[SkeletonSync] Local height %d already >= target %d", localHeight, ss.targetHeight)
		ss.mu.Lock()
		ss.progress = 1.0
		ss.mu.Unlock()
		return
	}

	log.Printf("[SkeletonSync] Starting skeleton sync: local=%d, target=%d, gap=%d",
		localHeight, ss.targetHeight, ss.targetHeight-localHeight)

	headers, err := ss.downloadHeadersSkeleton(localHeight, ss.targetHeight)
	if err != nil {
		log.Printf("[SkeletonSync] Header skeleton download failed: %v", err)
		return
	}

	log.Printf("[SkeletonSync] Downloaded %d skeleton headers", len(headers))

	if err := ss.validateHeaderChain(headers); err != nil {
		log.Printf("[SkeletonSync] Header chain validation failed: %v", err)
		return
	}

	if err := ss.downloadBlockBodies(headers); err != nil {
		log.Printf("[SkeletonSync] Block body download failed: %v", err)
		return
	}

	ss.mu.Lock()
	ss.progress = 1.0
	ss.mu.Unlock()

	log.Printf("[SkeletonSync] Completed: synced from %d to %d in %v",
		localHeight, ss.targetHeight, time.Since(ss.startTime))
}

func (ss *SkeletonSync) downloadHeadersSkeleton(fromHeight, toHeight uint64) ([]headerWithHash, error) {
	peers := ss.pm.GetActivePeers()
	if len(peers) == 0 {
		return nil, fmt.Errorf("no active peers for skeleton download")
	}

	allHeaders := make([]headerWithHash, 0)
	currentHeight := fromHeight + 1

	for currentHeight <= toHeight {
		select {
		case <-ss.ctx.Done():
			return nil, ss.ctx.Err()
		default:
		}

		remaining := toHeight - currentHeight + 1
		count := skeletonMaxHeadersPerRequest
		if uint64(count) > remaining {
			count = int(remaining)
		}

		var headers []core.BlockHeader
		var lastErr error

		for _, peer := range peers {
			h, err := ss.pm.FetchHeadersFrom(ss.ctx, peer, currentHeight, count)
			if err == nil && len(h) > 0 {
				headers = h
				break
			}
			lastErr = err
		}

		if len(headers) == 0 {
			return nil, fmt.Errorf("failed to fetch headers at height %d: %v", currentHeight, lastErr)
		}

		for i := range headers {
			block, fetchErr := ss.pm.FetchBlockByHeight(ss.ctx, peers[0], headers[i].Height)
			var hash []byte
			if fetchErr == nil && block != nil {
				hash = block.Hash
			}
			allHeaders = append(allHeaders, headerWithHash{
				Header: headers[i],
				Hash:   hash,
			})
		}

		currentHeight += uint64(len(headers))

		progress := float64(currentHeight-fromHeight) / float64(toHeight-fromHeight)
		if progress > 1.0 {
			progress = 1.0
		}
		ss.mu.Lock()
		ss.progress = progress * 0.7
		ss.currentHeight = currentHeight
		ss.mu.Unlock()

		if len(headers) < count {
			break
		}
	}

	return allHeaders, nil
}

func (ss *SkeletonSync) validateHeaderChain(headers []headerWithHash) error {
	if len(headers) == 0 {
		return nil
	}

	for i := 0; i < len(headers); i++ {
		select {
		case <-ss.ctx.Done():
			return ss.ctx.Err()
		default:
		}

		h := headers[i]

		var parent *core.Block
		if i == 0 {
			parent = ss.bc.LatestBlock()
			if parent != nil && !bytes.Equal(parent.Hash, h.Header.PrevHash) {
				parent, _ = ss.bc.BlockByHash(hex.EncodeToString(h.Header.PrevHash))
			}
		} else {
			parent, _ = ss.bc.BlockByHash(hex.EncodeToString(headers[i-1].Header.PrevHash))
		}

		if parent == nil {
			return fmt.Errorf("parent not found for header at height %d", h.Header.Height)
		}

		if err := ss.validator.ValidateHeader(&h.Header, parent); err != nil {
			return fmt.Errorf("header validation failed at height %d: %w", h.Header.Height, err)
		}
	}

	return nil
}

func (ss *SkeletonSync) downloadBlockBodies(headers []headerWithHash) error {
	peers := ss.pm.GetActivePeers()
	if len(peers) == 0 {
		return fmt.Errorf("no active peers for block body download")
	}

	totalHeaders := len(headers)
	processedBatches := 0

	for i := 0; i < totalHeaders; i += skeletonBatchSize {
		select {
		case <-ss.ctx.Done():
			return ss.ctx.Err()
		default:
		}

		end := i + skeletonBatchSize
		if end > totalHeaders {
			end = totalHeaders
		}

		batch := headers[i:end]
		if err := ss.downloadBlockBatch(batch, peers); err != nil {
			return fmt.Errorf("batch download failed at header %d/%d: %w", i, totalHeaders, err)
		}

		processedBatches++
		if processedBatches%skeletonProgressInterval == 0 {
			progress := 0.7 + 0.3*float64(end)/float64(totalHeaders)
			ss.mu.Lock()
			ss.progress = progress
			ss.mu.Unlock()
			log.Printf("[SkeletonSync] Block body progress: %d/%d headers (%.1f%%)",
				end, totalHeaders, progress*100)
		}
	}

	return nil
}

func (ss *SkeletonSync) downloadBlockBatch(headers []headerWithHash, peers []string) error {
	type blockResult struct {
		index int
		block *core.Block
		err   error
	}

	numWorkers := skeletonMaxConcurrentFetches
	if len(headers) < numWorkers {
		numWorkers = len(headers)
	}

	results := make(chan blockResult, len(headers))
	var wg sync.WaitGroup

	workCh := make(chan int, len(headers))
	for i := range headers {
		workCh <- i
	}
	close(workCh)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range workCh {
				h := headers[idx]
				block, err := ss.fetchBlockWithRetry(&h.Header, h.Hash, peers)
				results <- blockResult{index: idx, block: block, err: err}
			}
		}()
	}

	wg.Wait()
	close(results)

	blocks := make([]*core.Block, len(headers))
	for result := range results {
		if result.err != nil {
			return fmt.Errorf("failed to fetch block at height %d: %w",
				headers[result.index].Header.Height, result.err)
		}
		blocks[result.index] = result.block
	}

	for _, block := range blocks {
		if block == nil {
			continue
		}
		if err := ss.validator.ValidateBlock(block, true); err != nil {
			return fmt.Errorf("block validation failed at height %d: %w", block.GetHeight(), err)
		}
		if _, err := ss.bc.AddBlock(block); err != nil {
			return fmt.Errorf("failed to add block at height %d: %w", block.GetHeight(), err)
		}
	}

	return nil
}

func (ss *SkeletonSync) fetchBlockWithRetry(header *core.BlockHeader, hash []byte, peers []string) (*core.Block, error) {
	if len(hash) > 0 {
		hashHex := hex.EncodeToString(hash)
		for _, peer := range peers {
			block, err := ss.pm.FetchBlockByHash(ss.ctx, peer, hashHex)
			if err == nil && block != nil {
				return block, nil
			}
		}
	}

	for _, peer := range peers {
		block, err := ss.pm.FetchBlockByHeight(ss.ctx, peer, header.Height)
		if err == nil && block != nil {
			return block, nil
		}
	}

	return nil, fmt.Errorf("failed to fetch block at height %d from any peer", header.Height)
}

func (ss *SkeletonSync) GetCurrentHeight() uint64 {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.currentHeight
}

func (ss *SkeletonSync) GetTargetHeight() uint64 {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.targetHeight
}

func (ss *SkeletonSync) GetElapsedTime() time.Duration {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.startTime.IsZero() {
		return 0
	}
	return time.Since(ss.startTime)
}

func (ss *SkeletonSync) GetSyncRate() float64 {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.startTime.IsZero() {
		return 0
	}
	elapsed := time.Since(ss.startTime).Seconds()
	if elapsed <= 0 {
		return 0
	}
	synced := ss.currentHeight
	if synced == 0 {
		return 0
	}
	return float64(synced) / elapsed
}

func (ss *SkeletonSync) EstimateRemainingTime() time.Duration {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.startTime.IsZero() || ss.currentHeight == 0 {
		return 0
	}
	elapsed := time.Since(ss.startTime).Seconds()
	if elapsed <= 0 {
		return 0
	}
	rate := float64(ss.currentHeight) / elapsed
	remaining := float64(ss.targetHeight-ss.currentHeight) / rate
	return time.Duration(remaining) * time.Second
}

func (ss *SkeletonSync) String() string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return fmt.Sprintf("SkeletonSync{active=%v, progress=%.1f%%, current=%d, target=%d, elapsed=%v}",
		ss.active, ss.progress*100, ss.currentHeight, ss.targetHeight, time.Since(ss.startTime).Round(time.Second))
}

var _ = (*SkeletonSync)(nil).String