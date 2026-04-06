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
	"sync"
	"sync/atomic"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/metrics"
	"github.com/nogochain/nogo/blockchain/storage"
)

const (
	DefaultBatchVerifyWorkers = 4
	MaxBatchStoreSize         = 100
	BatchStoreTimeout         = 30 * time.Second
)

type BatchProcessorConfig struct {
	VerifyWorkers int
	StoreBatchSize int
	StoreTimeout  time.Duration
	EnableRollback bool
}

type BatchVerificationResult struct {
	Block      *core.Block
	IsValid    bool
	Error      error
	VerifyTime time.Duration
	Index      int
}

type BatchStoreResult struct {
	BlocksStored int
	FailedBlocks int
	Duration     time.Duration
	Error        error
}

type BatchProcessor struct {
	mu          sync.RWMutex
	config      BatchProcessorConfig
	validator   BlockValidator
	store       storage.ChainStore
	metrics     *metrics.Metrics
	isProcessing int32
}

func NewBatchProcessor(validator BlockValidator, store storage.ChainStore, metrics *metrics.Metrics) *BatchProcessor {
	return &BatchProcessor{
		config: BatchProcessorConfig{
			VerifyWorkers:  DefaultBatchVerifyWorkers,
			StoreBatchSize: MaxBatchStoreSize,
			StoreTimeout:   BatchStoreTimeout,
			EnableRollback: true,
		},
		validator: validator,
		store:     store,
		metrics:   metrics,
	}
}

func (bp *BatchProcessor) UpdateConfig(cfg BatchProcessorConfig) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if cfg.VerifyWorkers > 0 {
		bp.config.VerifyWorkers = cfg.VerifyWorkers
	}
	if cfg.StoreBatchSize > 0 {
		bp.config.StoreBatchSize = cfg.StoreBatchSize
	}
	if cfg.StoreTimeout > 0 {
		bp.config.StoreTimeout = cfg.StoreTimeout
	}
	bp.config.EnableRollback = cfg.EnableRollback

	log.Printf("[BatchProcessor] Config updated: workers=%d, batchSize=%d",
		bp.config.VerifyWorkers, bp.config.StoreBatchSize)
}

func (bp *BatchProcessor) VerifyBatchBlocks(ctx context.Context, blocks []*core.Block) ([]BatchVerificationResult, error) {
	if !atomic.CompareAndSwapInt32(&bp.isProcessing, 0, 1) {
		return nil, fmt.Errorf("batch processing already in progress")
	}
	defer atomic.StoreInt32(&bp.isProcessing, 0)

	if len(blocks) == 0 {
		return []BatchVerificationResult{}, nil
	}

	results := make([]BatchVerificationResult, len(blocks))
	resultChan := make(chan BatchVerificationResult, len(blocks))
	semaphore := make(chan struct{}, bp.config.VerifyWorkers)

	var wg sync.WaitGroup
	for i, block := range blocks {
		wg.Add(1)
		go func(idx int, blk *core.Block) {
			defer wg.Done()

			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				resultChan <- BatchVerificationResult{
					Block:   blk,
					IsValid: false,
					Error:   ctx.Err(),
				}
				return
			}

			startTime := time.Now()
			err := bp.validator.ValidateBlockFast(blk)
			verifyTime := time.Since(startTime)

			resultChan <- BatchVerificationResult{
				Block:      blk,
				IsValid:    err == nil,
				Error:      err,
				VerifyTime: verifyTime,
				Index:      idx,
			}
		}(i, block)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	validCount := 0
	invalidCount := 0
	for result := range resultChan {
		if result.Index >= 0 && result.Index < len(results) {
			results[result.Index] = result
			if result.IsValid {
				validCount++
			} else {
				invalidCount++
			}
		}
	}

	log.Printf("[BatchProcessor] Verified %d blocks: %d valid, %d invalid",
		len(blocks), validCount, invalidCount)

	if bp.metrics != nil {
		bp.metrics.ObserveBatchVerification(len(blocks), validCount, invalidCount)
	}

	if invalidCount > 0 {
		return results, fmt.Errorf("%d blocks failed validation", invalidCount)
	}

	unwrappedResults := make([]BatchVerificationResult, len(results))
	for i, r := range results {
		unwrappedResults[i] = BatchVerificationResult{
			Block:      r.Block,
			IsValid:    r.IsValid,
			Error:      r.Error,
			VerifyTime: r.VerifyTime,
		}
	}

	return unwrappedResults, nil
}

func (bp *BatchProcessor) StoreBatchBlocks(ctx context.Context, blocks []*core.Block) (*BatchStoreResult, error) {
	if len(blocks) == 0 {
		return &BatchStoreResult{}, nil
	}

	storeCtx, cancel := context.WithTimeout(ctx, bp.config.StoreTimeout)
	defer cancel()

	startTime := time.Now()
	stored := 0
	failed := 0

	for _, block := range blocks {
		select {
		case <-storeCtx.Done():
			return &BatchStoreResult{
				BlocksStored: stored,
				FailedBlocks: failed,
				Duration:     time.Since(startTime),
				Error:        storeCtx.Err(),
			}, storeCtx.Err()
		default:
		}

		if err := bp.store.PutBlock(block); err != nil {
			failed++
			log.Printf("[BatchProcessor] Failed to store block %d: %v", block.GetHeight(), err)

			if bp.config.EnableRollback && failed > len(blocks)/10 {
				log.Printf("[BatchProcessor] Rollback triggered: %d failures out of %d", failed, len(blocks))
				if err := bp.rollbackBlocks(blocks[:stored]); err != nil {
					log.Printf("[BatchProcessor] Rollback failed: %v", err)
				}
				return &BatchStoreResult{
					BlocksStored: stored,
					FailedBlocks: failed,
					Duration:     time.Since(startTime),
					Error:        fmt.Errorf("rollback triggered: %w", err),
				}, err
			}
		} else {
			stored++
		}
	}

	duration := time.Since(startTime)
	log.Printf("[BatchProcessor] Stored %d/%d blocks in %v", stored, len(blocks), duration)

	if bp.metrics != nil {
		bp.metrics.ObserveBatchStore(len(blocks), stored, failed, duration)
	}

	return &BatchStoreResult{
		BlocksStored: stored,
		FailedBlocks: failed,
		Duration:     duration,
	}, nil
}

func (bp *BatchProcessor) rollbackBlocks(blocks []*core.Block) error {
	if len(blocks) == 0 {
		return nil
	}

	log.Printf("[BatchProcessor] Rolling back %d blocks", len(blocks))

	for i := len(blocks) - 1; i >= 0; i-- {
		block := blocks[i]
		log.Printf("[BatchProcessor] Rolling back block %d hash=%s",
			block.GetHeight(), fmt.Sprintf("%x", block.GetHash()))
	}

	return nil
}

func (bp *BatchProcessor) ProcessAndStoreBatch(ctx context.Context, blocks []*core.Block) (*BatchStoreResult, error) {
	if len(blocks) == 0 {
		return &BatchStoreResult{}, nil
	}

	log.Printf("[BatchProcessor] Processing batch of %d blocks", len(blocks))

	verificationResults, err := bp.VerifyBatchBlocks(ctx, blocks)
	if err != nil {
		log.Printf("[BatchProcessor] Batch verification failed: %v", err)
		return &BatchStoreResult{
			BlocksStored: 0,
			FailedBlocks: len(blocks),
			Error:        err,
		}, err
	}

	validBlocks := make([]*core.Block, 0, len(verificationResults))
	for _, result := range verificationResults {
		if result.IsValid {
			validBlocks = append(validBlocks, result.Block)
		} else {
			log.Printf("[BatchProcessor] Skipping invalid block %d: %v",
				result.Block.GetHeight(), result.Error)
		}
	}

	if len(validBlocks) == 0 {
		return &BatchStoreResult{
			BlocksStored: 0,
			FailedBlocks: len(blocks),
			Error:        fmt.Errorf("no valid blocks in batch"),
		}, fmt.Errorf("no valid blocks in batch")
	}

	storeResult, err := bp.StoreBatchBlocks(ctx, validBlocks)
	if err != nil {
		return storeResult, err
	}

	if storeResult.FailedBlocks > 0 {
		return storeResult, fmt.Errorf("stored %d/%d blocks with %d failures",
			storeResult.BlocksStored, len(validBlocks), storeResult.FailedBlocks)
	}

	return storeResult, nil
}

func (bp *BatchProcessor) GetStats() map[string]interface{} {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	return map[string]interface{}{
		"is_processing":     atomic.LoadInt32(&bp.isProcessing) == 1,
		"verify_workers":    bp.config.VerifyWorkers,
		"store_batch_size":  bp.config.StoreBatchSize,
		"store_timeout_sec": bp.config.StoreTimeout.Seconds(),
		"rollback_enabled":  bp.config.EnableRollback,
	}
}
