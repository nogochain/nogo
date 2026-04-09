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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package network

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/metrics"
)

const (
	MinBatchSize           = 50
	MaxBatchSize           = 2000
	MinMaxConcurrent       = 1
	MaxMaxConcurrent       = 32
	HighLatencyThreshold   = 500 * time.Millisecond
	LowLatencyThreshold    = 100 * time.Millisecond
	ProgressUpdateInterval = 1 * time.Second
	// BatchProgressReportInterval controls how often batch progress is reported
	BatchProgressReportInterval = 10
	// ConnectionReuseBatchSize is the number of blocks to download per TCP connection
	// This balances efficiency (fewer connections) with reliability (smaller batches on failure)
	ConnectionReuseBatchSize = 10
	// InterBatchDelay is the delay between batch downloads to avoid overwhelming the server
	InterBatchDelay = 100 * time.Millisecond
)

type DownloadProgress struct {
	CurrentHeight uint64
	TargetHeight  uint64
	Downloaded    uint64
	Failed        uint64
	StartTime     time.Time
	BlocksPerSec  float64
}

type BatchDownloadResult struct {
	Blocks   []*core.Block
	Start    uint64
	End      uint64
	Success  bool
	Error    error
	Duration time.Duration
}

type DownloaderConfig struct {
	BatchSize        int
	MaxConcurrent    int
	MemoryThreshold  uint64
	ProgressCallback func(progress DownloadProgress)
}

type BlockDownloader struct {
	mu                sync.RWMutex
	pm                PeerAPI
	bc                BlockchainInterface
	validator         BlockValidator
	config            DownloaderConfig
	currentBatchSize  int32
	currentConcurrency int32
	isDownloading     int32
	downloadedCount   uint64
	failedCount       uint64
	startTime         time.Time
	metrics           *metrics.Metrics
}

type BlockValidator interface {
	ValidateBlockFast(block *core.Block) error
}

func NewBlockDownloader(pm PeerAPI, bc BlockchainInterface, validator BlockValidator, metrics *metrics.Metrics, syncConfig config.SyncConfig) *BlockDownloader {
	batchSize := syncConfig.BatchSize
	if batchSize <= 0 {
		batchSize = 500
	}
	
	maxConcurrent := syncConfig.MaxConcurrentDownloads
	if maxConcurrent <= 0 {
		maxConcurrent = 8
	}
	
	memoryThreshold := syncConfig.MemoryThresholdMB
	if memoryThreshold == 0 {
		memoryThreshold = 1500
	}
	memoryThresholdBytes := memoryThreshold * 1024 * 1024

	cfg := DownloaderConfig{
		BatchSize:       batchSize,
		MaxConcurrent:   maxConcurrent,
		MemoryThreshold: memoryThresholdBytes,
	}

	downloader := &BlockDownloader{
		pm:                 pm,
		bc:                 bc,
		validator:          validator,
		config:             cfg,
		currentBatchSize:   int32(cfg.BatchSize),
		currentConcurrency: int32(cfg.MaxConcurrent),
		metrics:            metrics,
	}

	go downloader.monitorAndAdjust()

	return downloader
}

func (d *BlockDownloader) monitorAndAdjust() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		d.adjustConcurrency()
		d.adjustBatchSize()
	}
}

func (d *BlockDownloader) adjustConcurrency() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	current := atomic.LoadInt32(&d.currentConcurrency)
	adjusted := current

	if memStats.Alloc > d.config.MemoryThreshold {
		if adjusted > MinMaxConcurrent {
			adjusted--
		}
	} else if memStats.Alloc < d.config.MemoryThreshold/2 {
		if adjusted < MaxMaxConcurrent {
			adjusted++
		}
	}

	if adjusted != current {
		atomic.StoreInt32(&d.currentConcurrency, adjusted)
	}
}

func (d *BlockDownloader) adjustBatchSize() {
	current := atomic.LoadInt32(&d.currentBatchSize)
	adjusted := current

	failed := atomic.LoadUint64(&d.failedCount)
	downloaded := atomic.LoadUint64(&d.downloadedCount)

	if downloaded > 0 {
		failRate := float64(failed) / float64(downloaded)
		if failRate > 0.1 && adjusted > int32(MinBatchSize) {
			adjusted = int32(float64(adjusted) * 0.8)
			if adjusted < int32(MinBatchSize) {
				adjusted = int32(MinBatchSize)
			}
		} else if failRate < 0.01 && adjusted < int32(MaxBatchSize) {
			adjusted = int32(float64(adjusted) * 1.1)
			if adjusted > int32(MaxBatchSize) {
				adjusted = int32(MaxBatchSize)
			}
		}
	}

	if adjusted != current {
		atomic.StoreInt32(&d.currentBatchSize, adjusted)
	}
}

type StoreFunc func(ctx context.Context, block *core.Block) error

func (d *BlockDownloader) BatchDownloadBlocks(ctx context.Context, peer string, startHeight uint64, count uint64, progressChan chan<- DownloadProgress, storeFunc StoreFunc) error {
	if !atomic.CompareAndSwapInt32(&d.isDownloading, 0, 1) {
		return fmt.Errorf("download already in progress")
	}
	defer atomic.StoreInt32(&d.isDownloading, 0)

	d.mu.Lock()
	d.startTime = time.Now()
	d.downloadedCount = 0
	d.failedCount = 0
	d.mu.Unlock()

	log.Printf("[Downloader] Starting sequential download with real-time storage: height %d to %d (%d blocks, batch size=%d)",
		startHeight, startHeight+count-1, count, ConnectionReuseBatchSize)

	totalStored := 0
	progressTicker := time.NewTicker(ProgressUpdateInterval)
	defer progressTicker.Stop()

	// Download blocks sequentially with real-time storage
	for batchStart := startHeight; batchStart < startHeight+count; batchStart += ConnectionReuseBatchSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Calculate batch end
		batchEnd := batchStart + ConnectionReuseBatchSize - 1
		if batchEnd >= startHeight+count {
			batchEnd = startHeight + count - 1
		}
		batchCount := batchEnd - batchStart + 1

		// Fetch blocks using a single connection for this batch
		blocks, err := d.pm.FetchBlocksByHeightRange(ctx, peer, batchStart, batchCount)
		if err != nil {
			log.Printf("[Downloader] Failed to fetch blocks %d-%d: %v", batchStart, batchEnd, err)
			atomic.AddUint64(&d.failedCount, batchCount)
			// Continue with next batch instead of failing completely
			continue
		}

		// Immediately store each block from this batch
		storedInBatch := 0
		for _, block := range blocks {
			if storeFunc != nil {
				err := storeFunc(ctx, block)
				if err != nil {
					log.Printf("[Downloader] Failed to store block %d: %v", block.Height, err)
					atomic.AddUint64(&d.failedCount, 1)
				} else {
					storedInBatch++
					totalStored++
				}
			}
		}

		atomic.AddUint64(&d.downloadedCount, uint64(len(blocks)))

		log.Printf("[Downloader] Progress: %d/%d blocks downloaded and stored (batch %d-%d complete, %d stored)",
			totalStored, count, batchStart, batchEnd, storedInBatch)

		// Send progress update
		if progressChan != nil {
			progress := DownloadProgress{
				CurrentHeight:    batchEnd,
				TargetHeight:     startHeight + count,
				Downloaded:       uint64(totalStored),
				Failed:           0, // Not tracking failures in real-time mode
				StartTime:        d.startTime,
				BlocksPerSec:     d.calculateBlocksPerSec(),
			}
			select {
			case progressChan <- progress:
			default:
			}
		}

		// Small delay between batches to avoid overwhelming the server
		if batchEnd < startHeight+count-1 {
			time.Sleep(InterBatchDelay)
		}
	}

	log.Printf("[Downloader] Download and storage complete: %d/%d blocks successfully stored in %v", 
		totalStored, count, time.Since(d.startTime))
	
	if uint64(totalStored) < count {
		return fmt.Errorf("incomplete sync: %d/%d blocks stored", totalStored, count)
	}
	return nil
}

func (d *BlockDownloader) downloadBatch(ctx context.Context, peer string, startHeight, endHeight uint64) *BatchDownloadResult {
	startTime := time.Now()

	totalCount := int(endHeight - startHeight + 1)
	log.Printf("[Downloader] Starting batch download with connection reuse: height %d to %d (%d blocks, batch size=%d)",
		startHeight, endHeight, totalCount, ConnectionReuseBatchSize)

	allBlocks := make([]*core.Block, 0, totalCount)
	var fetchErrors []error

	// Download in batches, reusing one TCP connection per batch
	for batchStart := startHeight; batchStart <= endHeight; batchStart += ConnectionReuseBatchSize {
		// Check context cancellation
		select {
		case <-ctx.Done():
			fetchErrors = append(fetchErrors, ctx.Err())
			break
		default:
		}

		// Calculate batch end
		batchEnd := batchStart + ConnectionReuseBatchSize - 1
		if batchEnd > endHeight {
			batchEnd = endHeight
		}
		batchCount := batchEnd - batchStart + 1

		// Fetch blocks using a single connection for this batch
		blocks, err := d.pm.FetchBlocksByHeightRange(ctx, peer, batchStart, batchCount)
		if err != nil {
			fetchErrors = append(fetchErrors, fmt.Errorf("failed to fetch blocks %d-%d: %w", batchStart, batchEnd, err))
			// If partial success, keep the blocks we got
			if len(blocks) > 0 {
				allBlocks = append(allBlocks, blocks...)
			}
			continue
		}

		allBlocks = append(allBlocks, blocks...)

		// Log progress after each batch
		log.Printf("[Downloader] Progress: %d/%d blocks downloaded (batch %d-%d complete, %d blocks)",
			len(allBlocks), totalCount, batchStart, batchEnd, len(blocks))

		// Small delay between batches to avoid overwhelming the server
		if batchEnd < endHeight {
			time.Sleep(InterBatchDelay)
		}
	}

	if len(fetchErrors) > 0 {
		log.Printf("[Downloader] Download completed with %d errors (got %d/%d blocks)",
			len(fetchErrors), len(allBlocks), totalCount)
		// Return partial success if we got some blocks
		if len(allBlocks) > 0 {
			return &BatchDownloadResult{
				Blocks:   allBlocks,
				Start:    startHeight,
				End:      endHeight,
				Success:  true,
				Duration: time.Since(startTime),
			}
		}
		return &BatchDownloadResult{
			Start:    startHeight,
			End:      endHeight,
			Success:  false,
			Error:    fmt.Errorf("download failed with %d errors: %v", len(fetchErrors), fetchErrors[0]),
			Duration: time.Since(startTime),
		}
	}

	log.Printf("[Downloader] Download complete: got %d blocks in %v", len(allBlocks), time.Since(startTime))
	return &BatchDownloadResult{
		Blocks:   allBlocks,
		Start:    startHeight,
		End:      endHeight,
		Success:  true,
		Duration: time.Since(startTime),
	}
}

func (d *BlockDownloader) calculateProgress(startHeight, targetHeight uint64) DownloadProgress {
	downloaded := atomic.LoadUint64(&d.downloadedCount)
	failed := atomic.LoadUint64(&d.failedCount)

	d.mu.RLock()
	elapsed := time.Since(d.startTime)
	d.mu.RUnlock()

	blocksPerSec := float64(0)
	if elapsed > 0 {
		blocksPerSec = float64(downloaded) / elapsed.Seconds()
	}

	currentHeight := startHeight + downloaded
	target := targetHeight

	return DownloadProgress{
		CurrentHeight: currentHeight,
		TargetHeight:  target,
		Downloaded:    downloaded,
		Failed:        failed,
		StartTime:     d.startTime,
		BlocksPerSec:  blocksPerSec,
	}
}

func (d *BlockDownloader) GetConfig() DownloaderConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.config
}

func (d *BlockDownloader) UpdateConfig(cfg DownloaderConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if cfg.BatchSize >= MinBatchSize && cfg.BatchSize <= MaxBatchSize {
		d.config.BatchSize = cfg.BatchSize
		atomic.StoreInt32(&d.currentBatchSize, int32(cfg.BatchSize))
	}

	if cfg.MaxConcurrent >= MinMaxConcurrent && cfg.MaxConcurrent <= MaxMaxConcurrent {
		d.config.MaxConcurrent = cfg.MaxConcurrent
		atomic.StoreInt32(&d.currentConcurrency, int32(cfg.MaxConcurrent))
	}

	if cfg.MemoryThreshold > 0 {
		d.config.MemoryThreshold = cfg.MemoryThreshold
	}

	if cfg.ProgressCallback != nil {
		d.config.ProgressCallback = cfg.ProgressCallback
	}

	log.Printf("[Downloader] Config updated: batchSize=%d, maxConcurrent=%d", cfg.BatchSize, cfg.MaxConcurrent)
}

func (d *BlockDownloader) GetStats() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return map[string]interface{}{
		"is_downloading":      atomic.LoadInt32(&d.isDownloading) == 1,
		"current_batch_size":  atomic.LoadInt32(&d.currentBatchSize),
		"current_concurrency": atomic.LoadInt32(&d.currentConcurrency),
		"downloaded_count":    atomic.LoadUint64(&d.downloadedCount),
		"failed_count":        atomic.LoadUint64(&d.failedCount),
		"start_time":          d.startTime,
		"elapsed_seconds":     time.Since(d.startTime).Seconds(),
	}
}

// calculateBlocksPerSec calculates the current blocks per second rate
func (d *BlockDownloader) calculateBlocksPerSec() float64 {
	duration := time.Since(d.startTime).Seconds()
	if duration <= 0 {
		return 0
	}
	return float64(atomic.LoadUint64(&d.downloadedCount)) / duration
}
