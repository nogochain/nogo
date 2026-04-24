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

package network

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
	broadcastQueueSize     = 1000
	maxBroadcastRetries    = 3
	broadcastRetryDelay    = 500 * time.Millisecond
	blockBroadcastTimeout  = 10 * time.Second
	statusBroadcastTimeout = 5 * time.Second
	maxConcurrentBroadcast = 50
)

type BroadcastItemType uint8

const (
	BroadcastItemBlock BroadcastItemType = iota
	BroadcastItemStatus
	BroadcastItemTransaction
)

type BroadcastItem struct {
	Type       BroadcastItemType
	Block      *core.Block
	PeerID     string
	Timestamp  time.Time
	RetryCount int
}

type BroadcastCache struct {
	mu         sync.RWMutex
	seenHashes map[string]time.Time
	maxAge     time.Duration
	cleanupCh  chan struct{}
}

func NewBroadcastCache(maxAge time.Duration) *BroadcastCache {
	cache := &BroadcastCache{
		seenHashes: make(map[string]time.Time),
		maxAge:     maxAge,
		cleanupCh:  make(chan struct{}, 1),
	}
	go cache.startCleanup()
	return cache
}

func (bc *BroadcastCache) startCleanup() {
	ticker := time.NewTicker(bc.maxAge / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bc.cleanup()
		case <-bc.cleanupCh:
			return
		}
	}
}

func (bc *BroadcastCache) cleanup() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	now := time.Now()
	for hash, timestamp := range bc.seenHashes {
		if now.Sub(timestamp) > bc.maxAge {
			delete(bc.seenHashes, hash)
		}
	}
}

func (bc *BroadcastCache) Seen(hash string) bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	_, exists := bc.seenHashes[hash]
	return exists
}

func (bc *BroadcastCache) MarkSeen(hash string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.seenHashes[hash] = time.Now()
}

func (bc *BroadcastCache) Close() {
	close(bc.cleanupCh)
}

type BroadcastEngine struct {
	mu                   sync.RWMutex
	pm                   PeerAPI
	bc                   BlockchainInterface
	miner                Miner
	queue                chan *BroadcastItem
	semaphore            chan struct{}
	blockCache           *BroadcastCache
	statusCache          *BroadcastCache
	isRunning            bool
	ctx                  context.Context
	cancel               context.CancelFunc
	broadcastCount       uint64
	duplicateCount       uint64
	lastBroadcast        time.Time
	rateLimitMu          sync.Mutex
	minBroadcastInterval time.Duration
}

func NewBroadcastEngine(pm PeerAPI, bc BlockchainInterface, miner Miner) *BroadcastEngine {
	return &BroadcastEngine{
		pm:                   pm,
		bc:                   bc,
		miner:                miner,
		queue:                make(chan *BroadcastItem, broadcastQueueSize),
		semaphore:            make(chan struct{}, maxConcurrentBroadcast),
		blockCache:           NewBroadcastCache(10 * time.Minute),
		statusCache:          NewBroadcastCache(5 * time.Minute),
		minBroadcastInterval: 100 * time.Millisecond,
	}
}

func (be *BroadcastEngine) Start(ctx context.Context) error {
	be.mu.Lock()
	defer be.mu.Unlock()

	if be.isRunning {
		return fmt.Errorf("broadcast engine already running")
	}

	be.ctx, be.cancel = context.WithCancel(ctx)
	be.isRunning = true

	go be.processBroadcastQueue()

	log.Printf("[Broadcast] Engine started with queue size=%d, max_concurrent=%d",
		broadcastQueueSize, maxConcurrentBroadcast)

	return nil
}

func (be *BroadcastEngine) Stop() {
	be.mu.Lock()
	defer be.mu.Unlock()

	if !be.isRunning {
		return
	}

	if be.cancel != nil {
		be.cancel()
	}

	be.isRunning = false
	be.blockCache.Close()
	be.statusCache.Close()

	log.Printf("[Broadcast] Engine stopped, total_broadcasts=%d, duplicates_prevented=%d",
		be.broadcastCount, be.duplicateCount)
}

func (be *BroadcastEngine) BroadcastBlock(block *core.Block, excludePeer string) error {
	if block == nil {
		return fmt.Errorf("cannot broadcast nil block")
	}

	blockHash := hex.EncodeToString(block.Hash)

	if be.blockCache.Seen(blockHash) {
		be.mu.Lock()
		be.duplicateCount++
		be.mu.Unlock()
		log.Printf("[Broadcast] Block %s already broadcast, skipping duplicate", blockHash[:16])
		return nil
	}

	be.blockCache.MarkSeen(blockHash)

	item := &BroadcastItem{
		Type:      BroadcastItemBlock,
		Block:     block,
		PeerID:    excludePeer,
		Timestamp: time.Now(),
	}

	select {
	case be.queue <- item:
		log.Printf("[Broadcast] Block %d (hash=%s) queued for broadcast",
			block.GetHeight(), blockHash[:16])
		return nil
	default:
		log.Printf("[Broadcast] WARNING: Broadcast queue full, dropping block %d", block.GetHeight())
		return fmt.Errorf("broadcast queue full")
	}
}

func (be *BroadcastEngine) BroadcastStatus(height uint64, work *big.Int, latestHash string) error {
	cacheKey := fmt.Sprintf("%d-%s", height, latestHash[:16])

	if be.statusCache.Seen(cacheKey) {
		return nil
	}

	be.statusCache.MarkSeen(cacheKey)

	item := &BroadcastItem{
		Type:      BroadcastItemStatus,
		Timestamp: time.Now(),
	}

	select {
	case be.queue <- item:
		return nil
	default:
		return nil
	}
}

func (be *BroadcastEngine) processBroadcastQueue() {
	for {
		select {
		case <-be.ctx.Done():
			return
		case item := <-be.queue:
			if item == nil {
				continue
			}

			be.rateLimitMu.Lock()
			if time.Since(be.lastBroadcast) < be.minBroadcastInterval {
				time.Sleep(be.minBroadcastInterval - time.Since(be.lastBroadcast))
			}
			be.lastBroadcast = time.Now()
			be.rateLimitMu.Unlock()

			be.semaphore <- struct{}{}

			go func(it *BroadcastItem) {
				defer func() { <-be.semaphore }()

				var err error
				switch it.Type {
				case BroadcastItemBlock:
					err = be.doBroadcastBlock(it.Block, it.PeerID)
				case BroadcastItemStatus:
					err = be.doBroadcastStatus()
				}

				if err != nil {
					log.Printf("[Broadcast] Broadcast failed: %v", err)

					if it.RetryCount < maxBroadcastRetries {
						it.RetryCount++
						time.AfterFunc(broadcastRetryDelay, func() {
							select {
							case be.queue <- it:
							default:
							}
						})
					}
				} else {
					be.mu.Lock()
					be.broadcastCount++
					be.mu.Unlock()
				}
			}(item)
		}
	}
}

func (be *BroadcastEngine) doBroadcastBlock(block *core.Block, excludePeer string) error {
	if block == nil {
		return fmt.Errorf("cannot broadcast nil block")
	}

	ctx, cancel := context.WithTimeout(be.ctx, blockBroadcastTimeout)
	defer cancel()

	peers := be.pm.GetActivePeers()
	if len(peers) == 0 {
		return fmt.Errorf("no active peers for block broadcast")
	}

	err := be.pm.BroadcastBlock(ctx, block)
	if err != nil {
		log.Printf("[Broadcast] Failed to broadcast block: %v", err)
		return err
	}

	log.Printf("[Broadcast] Block %d broadcast to %d peers",
		block.GetHeight(), len(peers))

	return nil
}

func (be *BroadcastEngine) doBroadcastStatus() error {
	if be.bc == nil {
		return nil
	}

	tip := be.bc.LatestBlock()
	if tip == nil {
		return nil
	}

	height := tip.GetHeight()
	work := be.bc.CanonicalWork()
	latestHash := hex.EncodeToString(tip.Hash)

	ctx, cancel := context.WithTimeout(be.ctx, statusBroadcastTimeout)
	defer cancel()

	be.pm.BroadcastNewStatus(ctx, height, work, latestHash)

	log.Printf("[Broadcast] Status broadcast: height=%d, work=%s, hash=%s",
		height, work.String(), latestHash[:16])

	return nil
}

func (be *BroadcastEngine) GetStats() map[string]interface{} {
	be.mu.RLock()
	defer be.mu.RUnlock()

	return map[string]interface{}{
		"is_running":        be.isRunning,
		"queue_length":      len(be.queue),
		"total_broadcasts":  be.broadcastCount,
		"duplicates":        be.duplicateCount,
		"active_workers":    len(be.semaphore),
		"max_workers":       maxConcurrentBroadcast,
		"block_cache_size":  len(be.blockCache.seenHashes),
		"status_cache_size": len(be.statusCache.seenHashes),
		"last_broadcast":    be.lastBroadcast,
	}
}

func (be *BroadcastEngine) SetMiner(miner Miner) {
	be.mu.Lock()
	defer be.mu.Unlock()
	be.miner = miner
}
