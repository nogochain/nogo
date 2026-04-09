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
	"encoding/hex"
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

func (d *BlockDownloader) BatchDownloadBlocks(ctx context.Context, peer string, startHeight uint64, count uint64, progressChan chan<- DownloadProgress) ([]*core.Block, error) {
	if !atomic.CompareAndSwapInt32(&d.isDownloading, 0, 1) {
		return nil, fmt.Errorf("download already in progress")
	}
	defer atomic.StoreInt32(&d.isDownloading, 0)

	d.mu.Lock()
	d.startTime = time.Now()
	d.downloadedCount = 0
	d.failedCount = 0
	d.mu.Unlock()

	batchSize := atomic.LoadInt32(&d.currentBatchSize)
	maxConcurrent := atomic.LoadInt32(&d.currentConcurrency)

	totalBatches := (count + uint64(batchSize) - 1) / uint64(batchSize)
	batchChan := make(chan *BatchDownloadResult, totalBatches)
	semaphore := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup

	for batchStart := startHeight; batchStart < startHeight+count; batchStart += uint64(batchSize) {
		batchEnd := batchStart + uint64(batchSize) - 1
		if batchEnd >= startHeight+count {
			batchEnd = startHeight + count - 1
		}

		wg.Add(1)
		go func(start, end uint64) {
			defer wg.Done()

			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				batchChan <- &BatchDownloadResult{
					Start:   start,
					End:     end,
					Success: false,
					Error:   ctx.Err(),
				}
				return
			}

			result := d.downloadBatch(ctx, peer, start, end)
			batchChan <- result
		}(batchStart, batchEnd)
	}

	go func() {
		wg.Wait()
		close(batchChan)
	}()

	progressTicker := time.NewTicker(ProgressUpdateInterval)
	defer progressTicker.Stop()

	var allBlocks []*core.Block
	batchesCompleted := 0

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case result, ok := <-batchChan:
			if !ok {
				if progressChan != nil {
					progress := d.calculateProgress(startHeight, startHeight+count)
					progressChan <- progress
				}
				return allBlocks, nil
			}

			if result.Success {
				allBlocks = append(allBlocks, result.Blocks...)
				atomic.AddUint64(&d.downloadedCount, uint64(len(result.Blocks)))
			} else {
				atomic.AddUint64(&d.failedCount, result.End-result.Start+1)
				rateLimitedLog("batch_fail", "[Downloader] Batch %d-%d failed: %v", result.Start, result.End, result.Error)
			}

			batchesCompleted++
			if progressChan != nil && batchesCompleted%10 == 0 {
				progress := d.calculateProgress(startHeight, startHeight+count)
				progressChan <- progress
			}

		case <-progressTicker.C:
			if progressChan != nil {
				progress := d.calculateProgress(startHeight, startHeight+count)
				progressChan <- progress
			}
		}
	}
}

func (d *BlockDownloader) downloadBatch(ctx context.Context, peer string, startHeight, endHeight uint64) *BatchDownloadResult {
	startTime := time.Now()

	count := int(endHeight - startHeight + 1)
	headers, err := d.pm.FetchHeadersFrom(ctx, peer, startHeight, count)
	if err != nil {
		return &BatchDownloadResult{
			Start:    startHeight,
			End:      endHeight,
			Success:  false,
			Error:    fmt.Errorf("failed to fetch headers: %w", err),
			Duration: time.Since(startTime),
		}
	}

	blocks := make([]*core.Block, 0, len(headers))
	blockChan := make(chan *core.Block, len(headers))
	errorChan := make(chan error, len(headers))

	var fetchWg sync.WaitGroup
	for i := range headers {
		fetchWg.Add(1)
		go func(idx int) {
			defer fetchWg.Done()

			h := headers[idx]
			hashHex := hex.EncodeToString(h.PrevHash)
			block, err := d.pm.FetchBlockByHash(ctx, peer, hashHex)
			if err != nil {
				errorChan <- fmt.Errorf("failed to fetch block %s: %w", hashHex, err)
				return
			}

			if err := d.validator.ValidateBlockFast(block); err != nil {
				errorChan <- fmt.Errorf("failed to validate block %s: %w", hashHex, err)
				return
			}

			blockChan <- block
		}(i)
	}

	go func() {
		fetchWg.Wait()
		close(blockChan)
		close(errorChan)
	}()

	var validationErrors []error
	for block := range blockChan {
		blocks = append(blocks, block)
	}

	for err := range errorChan {
		validationErrors = append(validationErrors, err)
	}

	if len(validationErrors) > 0 {
		return &BatchDownloadResult{
			Start:    startHeight,
			End:      endHeight,
			Success:  false,
			Error:    fmt.Errorf("batch validation failed with %d errors: %v", len(validationErrors), validationErrors[0]),
			Duration: time.Since(startTime),
		}
	}

	return &BatchDownloadResult{
		Blocks:   blocks,
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
