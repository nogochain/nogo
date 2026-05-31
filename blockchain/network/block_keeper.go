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
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/network/reactor"
)

const (
	syncCycle            = 3 * time.Second
	blockProcessChSize   = 1024
	blocksProcessChSize  = 128
	headersProcessChSize = 1024

	logCooldownDur = 30 * time.Second

	// After consecutiveSyncNoneThreshold no-op sync rounds, apply exponential
	// backoff instead of permanent disable — sync retries with increasing delays.
	consecutiveSyncNoneThreshold = 3
	maxSyncCooldown              = 5 * time.Minute

	// NogoChain-style sync type classification for intelligent strategy selection
	syncTypeNone    = iota // no peer available or already synced
	syncTypeRegular        // regular sequential block download
)

const (
	defaultBatchLimit = uint64(512)
	largeGapBatchMult = uint64(4)
)

var (
	maxBlockPerMsg        = uint64(512)
	maxBlockHeadersPerMsg = uint64(2048)
	syncTimeout           = 10 * time.Second
	fastSyncTimeout       = 15 * time.Second
	batchSyncTimeout      = 60 * time.Second
	stuckEscapeThreshold  = 60 * time.Second

	errAppendHeaders  = errors.New("fail to append list due to order dismatch")
	errRequestTimeout = errors.New("request timeout")
	errPeerDropped    = errors.New("Peer dropped")
	errPeerMisbehave  = errors.New("peer is misbehave")
	errChainMismatch  = errors.New("chain mismatch detected")
)

type blockMsg struct {
	block      *core.Block
	peerID     string
	sessionSeq uint64
}

type blocksMsg struct {
	blocks []*core.Block
	peerID string
}

type headersMsg struct {
	headers []*HeaderLocator
	peerID  string
}

type rateLimitedLogger struct {
	mu       sync.Mutex
	lastLog  map[string]time.Time
	duration time.Duration
}

func newRateLimitedLogger(d time.Duration) *rateLimitedLogger {
	return &rateLimitedLogger{
		lastLog:  make(map[string]time.Time),
		duration: d,
	}
}

func (rl *rateLimitedLogger) shouldLog(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if last, ok := rl.lastLog[key]; ok && time.Since(last) < rl.duration {
		return false
	}
	rl.lastLog[key] = time.Now()
	return true
}

// peerSyncInfo caches the best peer's chain info from updateSyncProgressFromPeers.
// Eliminates duplicate peer queries between sync reactor and blockKeeper.
type peerSyncInfo struct {
	peerID    string
	height    uint64
	work      *big.Int
	tipHash   string
	updatedAt time.Time
}

type blockKeeper struct {
	chain          ChainInterface
	peers          PeerSetInterface
	syncPeer       PeerInterface
	syncSessionSeq uint64 // incremented on each new sync, prevents stale block pollution
	candidatePool  *core.CandidatePool

	blockProcessCh   chan *blockMsg
	blocksProcessCh  chan *blocksMsg
	headersProcessCh chan *headersMsg
	syncBlockCh      chan *blockMsg

	headerList     *list.List
	forkResolvedCh chan struct{}
	quit           chan struct{}

	lastSuccessfulSyncHeight uint64
	lastSuccessfulSyncTime   time.Time

	// Failed sync peer tracking: prevents infinite rollback→resync→fail→rollback loops.
	// When a peer causes a chain mismatch during sync, it is deprioritized for
	// failedSyncCooldownDur to allow sync from other peers.
	failedSyncPeers       map[string]time.Time
	failedSyncCooldownDur time.Duration

	syncActive   bool
	syncActiveMu sync.Mutex

	// syncStateMu protects failedSyncPeers map
	// from concurrent access by cleanupExpiredEntries and syncWorker goroutines
	syncStateMu sync.Mutex

	getReceivedCheckpoint func() *core.CheckpointRecord
	triggerSyncCheck      func()

	lastSyncAttempt          time.Time
	syncCooldown             time.Duration
	consecutiveSyncNoneCount int // tracks consecutive syncTypeNone rounds for backoff escalation

	logLimiter *rateLimitedLogger

	lastSyncedLogHeight uint64

	peerSyncInfo   peerSyncInfo
	peerSyncInfoMu sync.RWMutex
}

func (bk *blockKeeper) isActive() bool {
	bk.syncActiveMu.Lock()
	defer bk.syncActiveMu.Unlock()
	return bk.syncActive
}

// setPeerSyncInfo is called by updateSyncProgressFromPeers to populate the shared cache.
// This eliminates duplicate peer queries between the sync reactor and blockKeeper.
func (bk *blockKeeper) setPeerSyncInfo(peerID string, height uint64, work *big.Int, tipHash string) {
	bk.peerSyncInfoMu.Lock()
	defer bk.peerSyncInfoMu.Unlock()
	bk.peerSyncInfo.peerID = peerID
	bk.peerSyncInfo.height = height
	bk.peerSyncInfo.work = new(big.Int).Set(work)
	bk.peerSyncInfo.tipHash = tipHash
	bk.peerSyncInfo.updatedAt = time.Now()
}

// getPeerSyncInfo returns cached peer chain info if fresh enough.
// This is the single source of truth shared between updateSyncProgressFromPeers and checkSyncType.
func (bk *blockKeeper) getPeerSyncInfo(maxAge time.Duration) (peerID string, height uint64, work *big.Int, tipHash string, ok bool) {
	bk.peerSyncInfoMu.RLock()
	defer bk.peerSyncInfoMu.RUnlock()
	if bk.peerSyncInfo.work == nil || time.Since(bk.peerSyncInfo.updatedAt) > maxAge {
		return "", 0, nil, "", false
	}
	return bk.peerSyncInfo.peerID, bk.peerSyncInfo.height, new(big.Int).Set(bk.peerSyncInfo.work), bk.peerSyncInfo.tipHash, true
}

// OnPeerDisconnected is called by Switch when a peer is removed.
// Clears the syncPeer reference if the disconnected peer is the current sync target,
// preventing the syncWorker from trying to request blocks from a disconnected peer.
func (bk *blockKeeper) OnPeerDisconnected(peerID string) {
	bk.syncActiveMu.Lock()
	defer bk.syncActiveMu.Unlock()
	if bk.syncPeer != nil && bk.syncPeer.ID() == peerID {
		log.Printf("[BlockKeeper] Sync peer %s disconnected, clearing syncPeer", peerID)
		bk.syncPeer = nil
	}
}

func newBlockKeeper(chain ChainInterface, peers PeerSetInterface, candidatePool *core.CandidatePool) *blockKeeper {
	bk := &blockKeeper{
		chain:                 chain,
		peers:                 peers,
		candidatePool:         candidatePool,
		blockProcessCh:        make(chan *blockMsg, blockProcessChSize),
		blocksProcessCh:       make(chan *blocksMsg, blocksProcessChSize),
		headersProcessCh:      make(chan *headersMsg, headersProcessChSize),
		syncBlockCh:           make(chan *blockMsg, 2048),
		headerList:            list.New(),
		forkResolvedCh:        make(chan struct{}, 1),
		quit:                  make(chan struct{}),
		failedSyncPeers:       make(map[string]time.Time),
		failedSyncCooldownDur: 15 * time.Second,
		logLimiter:            newRateLimitedLogger(logCooldownDur),
	}
	bk.resetHeaderState()
	go bk.syncWorker()
	go bk.cleanupExpiredEntries()
	return bk
}

func (bk *blockKeeper) appendHeaderList(headers []*HeaderLocator) error {
	if len(headers) == 0 {
		return nil
	}

	// Ensure the list has a stable starting point.
	// The list must represent a contiguous header chain so we can deterministically
	// validate and extend it using computed header hashes.
	if bk.headerList.Len() == 0 {
		best, err := bk.chain.BestBlockHeader()
		if err != nil {
			return fmt.Errorf("%w: get best header: %v", errAppendHeaders, err)
		}
		if best == nil {
			return fmt.Errorf("%w: best header is nil", errAppendHeaders)
		}
		bk.headerList.PushBack(best)
	}

	prevLoc, ok := bk.headerList.Back().Value.(*HeaderLocator)
	if !ok || prevLoc == nil {
		return fmt.Errorf("%w: invalid header list tail", errAppendHeaders)
	}

	prevHeight := prevLoc.Height
	prevHash, err := computeHeaderHash(&prevLoc.Header, prevLoc.Height, prevLoc.Header.MinerAddress)
	if err != nil {
		return fmt.Errorf("%w: compute previous header hash: %v", errAppendHeaders, err)
	}

	// Validate and append headers in order.
	for i := range headers {
		h := headers[i]
		if h == nil {
			return fmt.Errorf("%w: nil header at index %d", errAppendHeaders, i)
		}

		if h.Height != prevHeight+1 {
			return fmt.Errorf("%w: non-contiguous height at index %d: got %d want %d",
				errAppendHeaders, i, h.Height, prevHeight+1)
		}

		if len(h.Header.PrevHash) != core.HashLen {
			return fmt.Errorf("%w: invalid prevHash length at height %d: %d",
				errAppendHeaders, h.Height, len(h.Header.PrevHash))
		}
		if !equalBytes(h.Header.PrevHash, prevHash) {
			return fmt.Errorf("%w: prevHash mismatch at height %d", errAppendHeaders, h.Height)
		}

		// Compute this header hash for next-step linkage.
		curHash, hashErr := computeHeaderHash(&h.Header, h.Height, h.Header.MinerAddress)
		if hashErr != nil {
			return fmt.Errorf("%w: compute header hash at height %d: %v", errAppendHeaders, h.Height, hashErr)
		}

		bk.headerList.PushBack(h)
		prevHeight = h.Height
		prevHash = curHash
	}

	return nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (bk *blockKeeper) blockLocator() [][]byte {
	bestHeader, err := bk.chain.BestBlockHeader()
	if err != nil || bestHeader == nil {
		log.Printf("[BlockKeeper] Failed to get best block header: %v", err)
		return [][]byte{}
	}

	locator := [][]byte{}
	step := uint64(1)

	currentHeader := bestHeader
	for currentHeader != nil {
		hash := currentHeader.Header.PrevHash
		if currentHeader.Height == 0 {
			block, ok := bk.chain.BlockByHeight(0)
			if !ok || block == nil {
				break
			}
			hash = block.GetHash()
		} else {
			block, ok := bk.chain.BlockByHeight(currentHeader.Height)
			if !ok || block == nil {
				break
			}
			hash = block.GetHash()
		}

		if hash == nil {
			break
		}

		hashCopy := make([]byte, len(hash))
		copy(hashCopy, hash)
		locator = append(locator, hashCopy)

		if currentHeader.Height == 0 {
			break
		}

		var nextHeader *HeaderLocator
		if currentHeader.Height < step {
			nextHeader, err = bk.chain.GetHeaderByHeight(0)
		} else {
			nextHeader, err = bk.chain.GetHeaderByHeight(currentHeader.Height - step)
		}
		if err != nil || nextHeader == nil {
			log.Printf("[BlockKeeper] Failed to get header for locator at height %d: %v", currentHeader.Height-step, err)
			break
		}

		currentHeader = nextHeader

		if len(locator) >= 9 {
			step *= 2
		}
	}
	return locator
}

func (bk *blockKeeper) locateBlocks(locator [][]byte, stopHash []byte) ([]*core.Block, error) {
	headers, err := bk.locateHeaders(locator, stopHash)
	if err != nil {
		return nil, fmt.Errorf("locateBlocks locateHeaders: %w", err)
	}

	blocks := []*core.Block{}
	for i, header := range headers {
		if uint64(i) >= maxBlockPerMsg {
			break
		}

		block, ok := bk.chain.BlockByHeight(header.Height)
		if !ok || block == nil {
			return nil, fmt.Errorf("locateBlocks: block not found at height %d", header.Height)
		}

		blocks = append(blocks, block)
	}
	return blocks, nil
}

func (bk *blockKeeper) locateHeaders(locator [][]byte, stopHash []byte) ([]*HeaderLocator, error) {
	stopBlock, ok := bk.chain.BlockByHash(hex.EncodeToString(stopHash))
	if !ok || stopBlock == nil {
		return nil, fmt.Errorf("locateHeaders: stop block not found with hash %s", hex.EncodeToString(stopHash)[:16])
	}

	startHeader, err := bk.chain.GetHeaderByHeight(0)
	if err != nil || startHeader == nil {
		return nil, fmt.Errorf("locateHeaders: genesis header not found: %v", err)
	}

	for _, hash := range locator {
		hashHex := hex.EncodeToString(hash)
		block, found := bk.chain.BlockByHash(hashHex)
		if found && block != nil {
			header, headerErr := bk.chain.GetHeaderByHeight(block.GetHeight())
			if headerErr == nil && header != nil {
				startHeader = header
				break
			}
		}
	}

	totalHeaders := stopBlock.GetHeight() - startHeader.Height
	if totalHeaders > maxBlockHeadersPerMsg {
		totalHeaders = maxBlockHeadersPerMsg
	}

	headers := []*HeaderLocator{}
	for i := uint64(1); i <= totalHeaders; i++ {
		header, err := bk.chain.GetHeaderByHeight(startHeader.Height + i)
		if err != nil || header == nil {
			return nil, fmt.Errorf("locateHeaders: header not found at height %d: %v", startHeader.Height+i, err)
		}
		headers = append(headers, header)
	}
	return headers, nil
}

func (bk *blockKeeper) nextCheckpoint() *config.TrustedCheckpoint {
	bestHeader, err := bk.chain.BestBlockHeader()
	if err != nil || bestHeader == nil {
		log.Printf("[BlockKeeper] Failed to get best block header for nextCheckpoint: %v", err)
		return nil
	}

	height := bestHeader.Height
	checkpoints := config.ActiveCheckpoints()

	if len(checkpoints) == 0 || height >= checkpoints[len(checkpoints)-1].Height {
		return nil
	}

	nextCheckpoint := &checkpoints[len(checkpoints)-1]
	for i := len(checkpoints) - 2; i >= 0; i-- {
		if height >= checkpoints[i].Height {
			break
		}
		nextCheckpoint = &checkpoints[i]
	}
	return nextCheckpoint
}

func (bk *blockKeeper) processBlock(peerID string, block *core.Block) {
	select {
	case bk.blockProcessCh <- &blockMsg{block: block, peerID: peerID}:
	default:
		log.Printf("[BlockKeeper] Warning: blockProcessCh full, dropping block from peer %s", peerID)
	}
}

func (bk *blockKeeper) processBlocks(peerID string, blocks []*core.Block) {
	select {
	case bk.blocksProcessCh <- &blocksMsg{blocks: blocks, peerID: peerID}:
	default:
		log.Printf("[BlockKeeper] Warning: blocksProcessCh full, dropping blocks from peer %s", peerID)
	}
}

func (bk *blockKeeper) processHeaders(peerID string, headers []*HeaderLocator) {
	select {
	case bk.headersProcessCh <- &headersMsg{headers: headers, peerID: peerID}:
	default:
		log.Printf("[BlockKeeper] Warning: headersProcessCh full, dropping headers from peer %s", peerID)
	}
}

func (bk *blockKeeper) regularBlockSync(targetHeight uint64, batchLimit uint64) error {
	i := bk.chain.LatestBlock().GetHeight() + 1
	startHeight := i

	const fixedBatchSize = uint64(100)

	if bk.logLimiter.shouldLog("batch_sync_start") {
		log.Printf("[BlockKeeper] Starting batch sync: h=%d → target=%d (gap=%d, fixed batchSize=%d)",
			startHeight, targetHeight, targetHeight-startHeight+1, fixedBatchSize)
	}

	for i <= targetHeight {
		batchSize := targetHeight - i + 1
		if batchSize > fixedBatchSize {
			batchSize = fixedBatchSize
		}

		blocks, batchErr := bk.requireBlocksBatch(i, batchSize)

		if batchErr == nil && len(blocks) > 0 && len(blocks) < int(batchSize) {
			if bk.logLimiter.shouldLog("partial_batch") {
				log.Printf("[BlockKeeper] Partial batch received: requested %d blocks [%d-%d], got %d",
					batchSize, i, i+batchSize-1, len(blocks))
			}

			maxReceivedHeight := uint64(0)
			for h := range blocks {
				if h > maxReceivedHeight {
					maxReceivedHeight = h
				}
			}
			batchSize = maxReceivedHeight - i + 1
		}

		if batchErr != nil || len(blocks) == 0 {
			log.Printf("[BlockKeeper] Batch request failed [%d-%d], falling back to single block: %v",
				i, i+batchSize-1, batchErr)

			block, err := bk.requireBlock(i)
			if err != nil {
				return fmt.Errorf("regularBlockSync requireBlock at height %d: %w", i, err)
			}
			if _, addErr := bk.chain.AddBlock(block); addErr != nil {
				return fmt.Errorf("regularBlockSync add block at height %d: %w", block.GetHeight(), addErr)
			}
			i = bk.chain.LatestBlock().GetHeight() + 1
			continue
		}

		for h := i; h < i+batchSize && h <= targetHeight; h++ {
			block, ok := blocks[h]
			if !ok {
				var sErr error
				block, sErr = bk.requireBlock(h)
				if sErr != nil {
					return fmt.Errorf("regularBlockSync requireBlock at height %d: %w", h, sErr)
				}
			}

			_, addErr := bk.chain.AddBlock(block)
			if addErr != nil {
				return fmt.Errorf("regularBlockSync add block at height %d: %w", block.GetHeight(), addErr)
			}
		}

		newTip := bk.chain.LatestBlock().GetHeight()
		bk.recordSyncProgress(newTip)
		if newTip%500 == 0 || newTip >= targetHeight {
			pct := float64(0)
			if targetHeight > startHeight {
				pct = float64(newTip-startHeight+1) / float64(targetHeight-startHeight+1) * 100
			}
			log.Printf("[BlockKeeper] Batch sync progress: %d/%d (%.1f%%)", newTip, targetHeight, pct)
		}
		i = newTip + 1
		if i == startHeight {
			log.Printf("[BlockKeeper] Batch sync stalled: tip=%d unchanged (all fork blocks), waiting for missing parent to trigger reorg",
				newTip)
			break
		}
	}

	return nil
}

func (bk *blockKeeper) getSyncPeers() []PeerInterface {
	var peers []PeerInterface
	if bk.peers != nil {
		peerIDs := bk.peers.GetAllPeerIDs()
		for _, peerID := range peerIDs {
			if peer := bk.peers.PeerByID(peerID); peer != nil {
				peers = append(peers, peer)
			}
		}
	}
	if bk.syncPeer != nil {
		found := false
		for _, p := range peers {
			if p.ID() == bk.syncPeer.ID() {
				found = true
				break
			}
		}
		if !found {
			peers = append(peers, bk.syncPeer)
		}
	}
	return peers
}

func (bk *blockKeeper) requireBlock(height uint64) (*core.Block, error) {
	peers := bk.getSyncPeers()
	if len(peers) == 0 {
		return nil, fmt.Errorf("%w: no available sync peers", errPeerDropped)
	}

	var lastErr error
	for _, peer := range peers {
		log.Printf("[BlockKeeper] requireBlock: requesting block h=%d from peer=%s",
			height, peer.ID()[:min(12, len(peer.ID()))])

		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		defer cancel()

		block, err := peer.fetchBlock(ctx, height)
		if err != nil {
			log.Printf("[BlockKeeper] requireBlock: fetchBlock failed h=%d from peer=%s: %v",
				height, peer.ID()[:min(12, len(peer.ID()))], err)
			lastErr = err
			continue
		}

		log.Printf("[BlockKeeper] requireBlock: received block h=%d from peer=%s",
			height, peer.ID()[:min(12, len(peer.ID()))])
		return block, nil
	}

	return nil, fmt.Errorf("%w: requireBlock height=%d: all peers failed, last error: %w", errRequestTimeout, height, lastErr)
}

func (bk *blockKeeper) requireBlockFast(height uint64) (*core.Block, error) {
	if bk.syncPeer == nil {
		return nil, fmt.Errorf("%w: syncPeer is nil", errPeerDropped)
	}

	ctx, cancel := context.WithTimeout(context.Background(), fastSyncTimeout)
	defer cancel()

	block, err := bk.syncPeer.fetchBlock(ctx, height)
	if err != nil {
		return nil, fmt.Errorf("%w: requireBlockFast height=%d: %w", errRequestTimeout, height, err)
	}

	return block, nil
}

func (bk *blockKeeper) requireBlocksBatch(startHeight uint64, count uint64) (map[uint64]*core.Block, error) {
	if bk.syncPeer == nil {
		return nil, fmt.Errorf("%w: syncPeer is nil", errPeerDropped)
	}

	log.Printf("[BlockKeeper] requireBlocksBatch: fetching [%d-%d] (count=%d) from peer=%s sessionSeq=%d",
		startHeight, startHeight+count-1, count, bk.syncPeer.ID()[:min(12, len(bk.syncPeer.ID()))], bk.syncSessionSeq)

	ctx, cancel := context.WithTimeout(context.Background(), batchSyncTimeout)
	defer cancel()

	blocks, err := bk.syncPeer.fetchBlocksBatch(ctx, startHeight, count)
	if err != nil {
		log.Printf("[BlockKeeper] requireBlocksBatch: fetchBlocksBatch failed [%d-%d] from peer=%s: %v",
			startHeight, startHeight+count-1, bk.syncPeer.ID()[:min(12, len(bk.syncPeer.ID()))], err)
		return nil, fmt.Errorf("%w: requireBlocksBatch [%d-%d]: %w", errRequestTimeout, startHeight, startHeight+count-1, err)
	}

	result := make(map[uint64]*core.Block)
	for _, b := range blocks {
		if b != nil {
			result[b.GetHeight()] = b
		}
	}

	log.Printf("[BlockKeeper] requireBlocksBatch: received %d/%d blocks [%d-%d] from peer=%s",
		len(result), int(count), startHeight, startHeight+count-1, bk.syncPeer.ID()[:min(12, len(bk.syncPeer.ID()))])

	return result, nil
}

func (bk *blockKeeper) requireBlocks(locator [][]byte, stopHash []byte) ([]*core.Block, error) {
	if bk.syncPeer == nil {
		return nil, fmt.Errorf("%w: syncPeer is nil", errPeerDropped)
	}

	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()

	blocks, err := bk.syncPeer.fetchBlocksByLocator(ctx, locator, stopHash)
	if err != nil {
		return nil, fmt.Errorf("%w: requireBlocks: %w", errRequestTimeout, err)
	}

	return blocks, nil
}

func (bk *blockKeeper) requireHeaders(locator [][]byte, stopHash []byte) ([]*HeaderLocator, error) {
	if bk.syncPeer == nil {
		return nil, fmt.Errorf("%w: syncPeer is nil", errPeerDropped)
	}

	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()

	headers, err := bk.syncPeer.fetchHeadersByLocator(ctx, locator, stopHash)
	if err != nil {
		return nil, fmt.Errorf("%w: requireHeaders: %w", errRequestTimeout, err)
	}

	return headers, nil
}

func (bk *blockKeeper) resetHeaderState() {
	bestHeader, err := bk.chain.BestBlockHeader()
	if err != nil {
		log.Printf("[BlockKeeper] resetHeaderState failed to get best header: %v", err)
		bk.headerList.Init()
		return
	}

	bk.headerList.Init()
	if bk.nextCheckpoint() != nil && bestHeader != nil {
		bk.headerList.PushBack(bestHeader)
	}
}

// checkSyncType determines the optimal synchronization strategy.
// NogoChain-style tiered selection: evaluates peer capabilities, height gap,
// and chain state to classify the scenario as fast sync, regular sync,
// fork detection, or no-sync-needed.
//
// ARCHITECTURE: Single-source-of-truth via shared peerSyncInfo cache.
// updateSyncProgressFromPeers (sync reactor) populates the cache every 5s.
// checkSyncType (blockKeeper) reads the cache as primary source, with
// independent peer query as stale-cache fallback. Both paths share the same
// tip hash for precision-error detection, eliminating the conflict where
// sync reactor says "synced" but blockKeeper says "sync needed".
//
// CRITICAL FIX: Work-prioritized peer selection.
// Previously, bestPeer() selected the peer with the highest BLOCK HEIGHT,
// ignoring cumulative WORK. This caused a minority-fork node with many
// low-work blocks to be chosen as the sync target, resulting in:
//  1. ABCDE (most work, lower height) tries to sync from F (higher
//     height, less work) → F's blocks fail validation → retry loop
//  2. All ABCDE nodes stall, unable to mine or sync
func (bk *blockKeeper) checkSyncType() (int, PeerInterface) {
	blockHeight := bk.chain.LatestBlock().GetHeight()
	localWork := bk.chain.CanonicalWork()

	if localWork == nil {
		localWork = big.NewInt(0)
	}

	var peerWork *big.Int
	var peerHeight uint64
	var bestByWork PeerInterface
	var bestTipHash string

	// PRIMARY: use shared cache populated by updateSyncProgressFromPeers.
	// Eliminates duplicate peer queries and ensures hash comparison consistency.
	const maxCacheAge = 10 * time.Second
	cachedPID, cachedH, cachedW, cachedHash, cacheOK := bk.getPeerSyncInfo(maxCacheAge)

	if cacheOK {
		peerHeight = cachedH
		peerWork = cachedW
		bestByWork = bk.wrapPeer(cachedPID, cachedH)
		bestTipHash = cachedHash

		if bk.logLimiter.shouldLog("checkSyncType") {
			log.Printf("[BlockKeeper] checkSyncType: local(H=%d,W=%s) vs cachedPeer(H=%d,W=%s,ID=%s)",
				blockHeight, localWork.String(), peerHeight, peerWork.String(), bestByWork.ID())
		}
	} else {
		// FALLBACK: cache miss or stale — independent peer query.
		peerIDs := bk.peers.GetAllPeerIDs()
		if len(peerIDs) == 0 {
			return syncTypeNone, nil
		}

		type peerResult struct {
			peerID  string
			height  uint64
			work    *big.Int
			tipHash string
		}

		resultCh := make(chan peerResult, len(peerIDs))
		var wg sync.WaitGroup
		peerCtx, peerCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer peerCancel()

		for _, pid := range peerIDs {
			bk.syncStateMu.Lock()
			if lastFail, failed := bk.failedSyncPeers[pid]; failed {
				if time.Since(lastFail) < bk.failedSyncCooldownDur {
					bk.syncStateMu.Unlock()
					continue
				}
				delete(bk.failedSyncPeers, pid)
			}
			bk.syncStateMu.Unlock()

			wg.Add(1)
			go func(peerID string) {
				defer wg.Done()
				ph, pw, tipHash, err := bk.peers.GetPeerChainInfo(peerID)
				if err != nil || pw == nil {
					return
				}
				select {
				case resultCh <- peerResult{peerID: peerID, height: ph, work: pw, tipHash: tipHash}:
				case <-peerCtx.Done():
				}
			}(pid)
		}

		go func() {
			wg.Wait()
			close(resultCh)
		}()

		var bestWork *big.Int
		for r := range resultCh {
			if bestWork == nil || r.work.Cmp(bestWork) > 0 {
				bestWork = r.work
				peerHeight = r.height
				bestByWork = bk.wrapPeer(r.peerID, r.height)
				bestTipHash = r.tipHash
			}
		}

		if bestByWork == nil {
			log.Printf("[BlockKeeper] checkSyncType: no peer has valid work info (localH=%d)", blockHeight)
			return syncTypeNone, nil
		}

		peerWork = bestWork

		if bk.logLimiter.shouldLog("checkSyncType") {
			log.Printf("[BlockKeeper] checkSyncType: local(H=%d,W=%s) vs bestPeer(H=%d,W=%s,ID=%s)",
				blockHeight, localWork.String(), peerHeight, peerWork.String(), bestByWork.ID())
		}

		// Checkpoint-aware peer filtering (fallback path only).
		// When cache is fresh, updateSyncProgressFromPeers already found the best peer.
		if (blockHeight == 0 || blockHeight < 128) && bk.getReceivedCheckpoint != nil {
			if p2pCP := bk.getReceivedCheckpoint(); p2pCP != nil && p2pCP.Height > 0 {
				if peerHeight < p2pCP.Height {
					log.Printf("[BlockKeeper] checkSyncType: bestPeer(H=%d) doesn't reach checkpoint(H=%d), searching for checkpoint-compatible peer",
						peerHeight, p2pCP.Height)
					var cpPeer PeerInterface
					var cpWork *big.Int
					var cpHeight uint64
					for _, peerID := range peerIDs {
						ph, pw, _, err := bk.peers.GetPeerChainInfo(peerID)
						if err != nil || pw == nil {
							continue
						}
						if ph < p2pCP.Height {
							continue
						}
						if cpWork == nil || pw.Cmp(cpWork) > 0 {
							cpWork = pw
							cpHeight = ph
							cpPeer = bk.wrapPeer(peerID, ph)
						}
					}
					if cpPeer != nil {
						log.Printf("[BlockKeeper] checkSyncType: switched to checkpoint-compatible peer H=%d (checkpoint H=%d)",
							cpHeight, p2pCP.Height)
						bestByWork = cpPeer
						peerWork = cpWork
						peerHeight = cpHeight
					}
				}
			}
		}
	}

	// UNIFIED HASH COMPARISON: check tip hash BEFORE work comparison.
	// When local and peer tip hashes match at the same height, the work difference
	// is a precision artifact from big.Int→string→big.Int serialization round-trip,
	// not a real fork. Both primary (cache) and fallback paths use this check.
	if peerHeight == blockHeight && bestTipHash != "" {
		localTip := bk.chain.LatestBlock()
		if localTip != nil {
			localHashHex := hex.EncodeToString(localTip.Hash)
			if localHashHex == bestTipHash {
				if bk.logLimiter.shouldLog("checkSyncType") {
					log.Printf("[BlockKeeper] checkSyncType: tip hash matches at H=%d, work diff is precision error, considering synced",
						blockHeight)
				}
				return syncTypeNone, nil
			}
		}
	}

	// Work comparison: heaviest chain wins.
	if peerWork.Cmp(localWork) > 0 {
		if peerHeight > blockHeight {
			log.Printf("[BlockKeeper] checkSyncType: regular sync (peer has more work + higher height, gap=%d)",
				peerHeight-blockHeight)
			if realPeer := bk.peers.PeerByID(bestByWork.ID()); realPeer != nil && realPeer.Height() > 0 {
				return syncTypeRegular, realPeer
			}
			if fallback := bk.peers.bestPeer(SFFullNode); fallback != nil && fallback.Height() > 0 {
				log.Printf("[BlockKeeper] checkSyncType: PeerByID(%s) returned nil or height=0, using bestPeer fallback %s (h=%d)",
					bestByWork.ID(), fallback.ID(), fallback.Height())
				return syncTypeRegular, fallback
			}
			log.Printf("[BlockKeeper] checkSyncType: all peer resolution failed, using simplePeer wrapper (h=%d)", peerHeight)
			return syncTypeRegular, bestByWork
		}
		if peerHeight == blockHeight {
			const minForkHeight = uint64(128)
			if blockHeight < minForkHeight {
				log.Printf("[BlockKeeper] checkSyncType: fork reorg (peer has more work at same low height=%d < min=%d, hash differs)",
					blockHeight, minForkHeight)
				if realPeer := bk.peers.PeerByID(bestByWork.ID()); realPeer != nil && realPeer.Height() > 0 {
					return syncTypeRegular, realPeer
				}
				return syncTypeRegular, bestByWork
			}
			log.Printf("[BlockKeeper] checkSyncType: fork detection (peer has more work at same height=%d, hash differs)",
				blockHeight)
			if realPeer := bk.peers.PeerByID(bestByWork.ID()); realPeer != nil && realPeer.Height() > 0 {
				return syncTypeRegular, realPeer
			}
			return syncTypeRegular, bestByWork
		}
		log.Printf("[BlockKeeper] checkSyncType: fork detection (peer has more work but lower height=%d, local=%d) — deferring to chain.AddBlock via P2P propagation",
			peerHeight, blockHeight)
		return syncTypeNone, nil
	}

	// When work is EQUAL, do NOT trigger fork detection.
	// This prevents infinite sync loops when local and peer have identical chains.
	if peerWork.Cmp(localWork) == 0 {
		if peerHeight == blockHeight {
			log.Printf("[BlockKeeper] checkSyncType: same height and work (H=%d, W=%s), considering synced",
				blockHeight, localWork.String())
			return syncTypeNone, bestByWork
		}
		log.Printf("[BlockKeeper] checkSyncType: same work but different height (local=%d, peer=%d), considering synced",
			blockHeight, peerHeight)
		return syncTypeNone, bestByWork
	}

	// peerWork < localWork: we are winning.
	if peerHeight > blockHeight && peerWork.Cmp(localWork) <= 0 {
		log.Printf("[BlockKeeper] checkSyncType: peer has higher height (%d > %d) but LESS work (%s <= %s), rejecting weak fork",
			peerHeight, blockHeight, peerWork.String(), localWork.String())
	}

	return syncTypeNone, bestByWork
}

// startSync is the main synchronization entry point.
// NogoChain-style dispatch: uses checkSyncType() for intelligent strategy selection,
// then dispatches to the appropriate sync handler.
func (bk *blockKeeper) startSync() bool {
	if bk.checkStuckEscape() {
		return true
	}

	syncType, peer := bk.checkSyncType()
	if peer == nil {
		return false
	}

	// Resolve the REAL peer for sync operations that need actual connections.
	// checkSyncType returns a simplePeer wrapper (ID+height) from the work scan.
	// dispatchRegularSync/dispatchFastSync need the real peer with active MConnection.
	realPeer := bk.resolveRealPeer(peer)
	if realPeer != nil {
		peer = realPeer
	}

	blockHeight := bk.chain.LatestBlock().GetHeight()

	switch syncType {
	case syncTypeRegular:
		return bk.dispatchRegularSync(peer, blockHeight)

	case syncTypeNone:
		fallthrough
	default:
		if peer.Height() == 0 {
			log.Printf("[BlockKeeper] startSync: no peer with chain info available (localH=%d, fallback=%s)", blockHeight, peer.ID())
			return false
		}
		if peer.Height() <= blockHeight {
			localWork := bk.chain.CanonicalWork()
			peerWork := bk.getPeerWork(peer)
			if peerWork == nil || localWork == nil || peerWork.Cmp(localWork) <= 0 {
				if bk.lastSyncedLogHeight != blockHeight {
					log.Printf("[BlockKeeper] Synced (localH=%d >= peerH=%d), no sync needed", blockHeight, peer.Height())
					bk.lastSyncedLogHeight = blockHeight
				}
			}
			return false
		}
		return false
	}
}

// resolveRealPeer attempts to get the real PeerInterface for the given peer wrapper.
// checkSyncType returns a simplePeer (ID+height only); real sync operations need
// the actual peer with active MConnection for block/header requests.
func (bk *blockKeeper) resolveRealPeer(wrapper PeerInterface) PeerInterface {
	if wrapper == nil || bk.peers == nil {
		return nil
	}

	// If wrapper is already a real peer (from bestPeer), return as-is
	if _, isSimple := wrapper.(*simplePeer); !isSimple {
		return wrapper
	}

	// Try to find the same peer through bestPeer
	realPeer := bk.peers.bestPeer(SFFullNode)
	if realPeer != nil && realPeer.ID() == wrapper.ID() {
		return realPeer
	}

	// bestPeer returned a different peer — but checkSyncType says wrapper has most work.
	// Return wrapper anyway; regular sync uses peer's ID to look up chain info.
	return wrapper
}

// nextCheckpointHeight finds the nearest checkpoint height at or below the given height.
func (bk *blockKeeper) nextCheckpointHeight(maxHeight uint64) (uint64, string) {
	h, hash, err := bk.chain.LatestCheckpoint()
	if err != nil || h == 0 {
		return 0, ""
	}
	if maxHeight > 0 && h > maxHeight {
		roundDown := (maxHeight / 1000) * 1000
		for testH := roundDown; testH >= 1000; testH -= 1000 {
			hashStr, found, _ := bk.chain.GetCheckpointByHeight(testH)
			if found {
				return testH, hashStr
			}
		}
		return 0, ""
	}
	return h, hash
}

// dispatchRegularSync handles standard block synchronization from a peer.

func (bk *blockKeeper) dispatchRegularSync(peer PeerInterface, localHeight uint64) bool {
	if _, isSimple := peer.(*simplePeer); isSimple {
		resolved := false

		if realPeer := bk.peers.bestPeer(SFFullNode); realPeer != nil {
			if realPeer.ID() == peer.ID() {
				peer = realPeer
				resolved = true
			}
		}

		if !resolved {
			if realPeer := bk.peers.PeerByID(peer.ID()); realPeer != nil {
				peerWork := bk.getPeerWork(realPeer)
				localWork := bk.chain.CanonicalWork()
				if peerWork != nil && localWork != nil && peerWork.Cmp(localWork) > 0 {
					peer = realPeer
					resolved = true
					log.Printf("[BlockKeeper] dispatchRegularSync: resolved work-optimal peer %s via PeerByID (h=%d)",
						realPeer.ID(), realPeer.Height())
				}
			}
		}

		if !resolved {
			if realPeer := bk.peers.bestPeer(SFFullNode); realPeer != nil {
				fallbackWork := bk.getPeerWork(realPeer)
				localWork := bk.chain.CanonicalWork()
				if fallbackWork == nil || localWork == nil || fallbackWork.Cmp(localWork) <= 0 {
					log.Printf("[BlockKeeper] dispatchRegularSync: fallback peer %s has <= work (local=%s), rejecting to prevent weak-fork sync",
						realPeer.ID(), localWork.String())
					return false
				}
				log.Printf("[BlockKeeper] dispatchRegularSync: work-optimal peer %s not found directly, using fallback %s (h=%d, w=%s)",
					peer.ID(), realPeer.ID(), realPeer.Height(), fallbackWork.String())
				peer = realPeer
				resolved = true
			}
		}

		if !resolved {
			log.Printf("[BlockKeeper] dispatchRegularSync: no eligible peers available for work-optimal peer %s, skipping sync round",
				peer.ID())
			return false
		}
	}

	bk.syncPeer = peer
	bk.syncSessionSeq++

	peerHeight := peer.Height()
	peerHasLiveInfo := peerHeight > 0

	log.Printf("[BlockKeeper] dispatchRegularSync: syncPeer=%s height=%d sessionSeq=%d localH=%d",
		peer.ID()[:min(12, len(peer.ID()))], peerHeight, bk.syncSessionSeq, localHeight)

	freshHeight, _, _, fetchErr := bk.peers.GetPeerChainInfo(peer.ID())
	if fetchErr == nil && freshHeight > 0 {
		if freshHeight > peerHeight {
			if bk.logLimiter.shouldLog("fresh_height_" + peer.ID()) {
				log.Printf("[BlockKeeper] dispatchRegularSync: cached peer H=%d upgraded to live H=%d", peerHeight, freshHeight)
			}
		}
		peerHeight = freshHeight
		peerHasLiveInfo = true
	} else if !peerHasLiveInfo {
		if fetchErr != nil {
			log.Printf("[BlockKeeper] dispatchRegularSync: cannot determine peer height for %s (err=%v), skipping sync round", peer.ID(), fetchErr)
		} else {
			log.Printf("[BlockKeeper] dispatchRegularSync: peer %s has zero height (live+cached), skipping sync round", peer.ID())
		}
		return false
	}

	if localHeight >= peerHeight {
		peerWork := bk.getPeerWork(peer)
		localWork := bk.chain.CanonicalWork()
		if peerWork != nil && localWork != nil && peerWork.Cmp(localWork) > 0 {
			log.Printf("[BlockKeeper] dispatchRegularSync: fork recovery needed (localH=%d >= peerH=%d but peer has more work) — deferring to chain.AddBlock via P2P propagation",
				localHeight, peerHeight)
		} else {
			log.Printf("[BlockKeeper] dispatchRegularSync: local ahead (h=%d >= peerH=%d), no sync needed", localHeight, peerHeight)
		}
		return false
	}

	gapSize := peerHeight - localHeight
	log.Printf("[BlockKeeper] REGULAR sync: h=%d → h=%d (gap=%d) via peer %s (peerH=%d)",
		localHeight, peerHeight, gapSize, peer.ID(), peerHeight)

	batchLimit := uint64(maxBlockPerMsg)
	if gapSize > maxBlockPerMsg*10 {
		batchLimit = maxBlockPerMsg * largeGapBatchMult
		log.Printf("[BlockKeeper] Large sync gap (%d blocks), using expanded batch limit %d", gapSize, batchLimit)
	}
	if batchLimit > uint64(MaxSyncRange) {
		batchLimit = uint64(MaxSyncRange)
	}

	syncedAny := false
	for localHeight < peerHeight {
		targetHeight := localHeight + batchLimit
		if targetHeight > peerHeight {
			targetHeight = peerHeight
		}

		prevLocalHeight := localHeight
		err := bk.regularBlockSync(targetHeight, batchLimit)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, errChainMismatch.Error()) {
				log.Printf("[BlockKeeper] Chain mismatch detected (peer on different fork): %v", err)
				bk.peers.DecSyncLoad(peer.ID())
				return true
			}

			peerWork := bk.getPeerWork(peer)
			if peerWork != nil {
				localWork := bk.chain.CanonicalWork()
				if localWork != nil && peerWork.Cmp(localWork) > 0 {
					log.Printf("[BlockKeeper] Peer %s has more work (peer=%s > local=%s) but sync failed: %v - skipping, chain.AddBlock handles reorg",
						peer.ID(), peerWork.String(), localWork.String(), err)
					bk.peers.DecSyncLoad(peer.ID())
					return true
				}
			}

			if errors.Is(err, errRequestTimeout) || errors.Is(err, errPeerDropped) {
				log.Printf("[BlockKeeper] Sync transient failure at h=%d: %v (will retry)", localHeight, err)
				bk.peers.DecSyncLoad(peer.ID())
				return false
			}

			log.Printf("[BlockKeeper] Sync FAILED at h=%d: %v (will retry in 1s)", localHeight, err)
			bk.peers.ProcessIllegal(peer.ID(), LevelMsgIllegal, errMsg)
			bk.peers.DecSyncLoad(peer.ID())
			return false
		}

		localHeight = bk.chain.LatestBlock().GetHeight()
		if localHeight == prevLocalHeight {
			log.Printf("[BlockKeeper] dispatchRegularSync: tip stuck at h=%d (all fork blocks, missing ancestor), pausing sync to wait for reorg",
				localHeight)
			bk.peers.DecSyncLoad(peer.ID())
			return true
		}
		syncedAny = true
	}

	newHeight := bk.chain.LatestBlock().GetHeight()
	log.Printf("[BlockKeeper] Sync SUCCESS: h=%d → h=%d (synced=%v)", localHeight, newHeight, syncedAny)
	bk.recordSyncProgress(newHeight)
	bk.peers.DecSyncLoad(peer.ID())
	return true
}

// wrapPeer creates a minimal PeerInterface wrapper from a peerID and height.
// Used by checkSyncType to return the work-optimal peer without requiring
// a full PeerSet.PeerByID lookup.
func (bk *blockKeeper) wrapPeer(peerID string, height uint64) PeerInterface {
	return &simplePeer{id: peerID, height: height}
}

type simplePeer struct {
	id     string
	height uint64
}

func (p *simplePeer) ID() string                                        { return p.id }
func (p *simplePeer) Height() uint64                                    { return p.height }
func (p *simplePeer) getBlockByHeight(height uint64) bool               { return false }
func (p *simplePeer) getBlocksByHeights(heights []uint64) bool          { return false }
func (p *simplePeer) getBlocks(locator [][]byte, stopHash []byte) bool  { return false }
func (p *simplePeer) getHeaders(locator [][]byte, stopHash []byte) bool { return false }
func (p *simplePeer) fetchBlock(_ context.Context, _ uint64) (*core.Block, error) {
	return nil, errors.New("simplePeer: fetchBlock not supported")
}
func (p *simplePeer) fetchBlocksBatch(_ context.Context, _ uint64, _ uint64) ([]*core.Block, error) {
	return nil, errors.New("simplePeer: fetchBlocksBatch not supported")
}
func (p *simplePeer) fetchBlocksByLocator(_ context.Context, _ [][]byte, _ []byte) ([]*core.Block, error) {
	return nil, errors.New("simplePeer: fetchBlocksByLocator not supported")
}
func (p *simplePeer) fetchHeadersByLocator(_ context.Context, _ [][]byte, _ []byte) ([]*HeaderLocator, error) {
	return nil, errors.New("simplePeer: fetchHeadersByLocator not supported")
}

// getPeerWork retrieves the cumulative work of a peer's chain.
// Uses PeerSetInterface.GetPeerChainInfo for direct access.
// Returns nil if work cannot be determined.
func (bk *blockKeeper) getPeerWork(peer PeerInterface) *big.Int {
	if bk.peers == nil || peer == nil {
		return nil
	}

	_, work, _, err := bk.peers.GetPeerChainInfo(peer.ID())
	if err != nil || work == nil {
		return nil
	}

	return work
}

func (bk *blockKeeper) recordSyncProgress(height uint64) {
	if height > bk.lastSuccessfulSyncHeight {
		bk.lastSuccessfulSyncHeight = height
		bk.lastSuccessfulSyncTime = time.Now()
	}
}

// checkStuckEscape detects sync stalls and forces peer rotation.
// NogoChain-style recovery: when the sync peer fails to deliver progress
// within stuckEscapeThreshold, reset sync state to force peer reselection
// on the next sync cycle. This prevents indefinite stalls with a dead peer.
func (bk *blockKeeper) checkStuckEscape() bool {
	if bk.lastSuccessfulSyncTime.IsZero() {
		return false
	}

	currentHeight := bk.chain.LatestBlock().GetHeight()
	timeSinceProgress := time.Since(bk.lastSuccessfulSyncTime)

	if timeSinceProgress <= stuckEscapeThreshold {
		return false
	}

	if currentHeight > bk.lastSuccessfulSyncHeight {
		return false
	}

	log.Printf("[BlockKeeper] STUCK ESCAPE: no progress for %v at h=%d (last progress at h=%d), forcing peer rotation",
		timeSinceProgress, currentHeight, bk.lastSuccessfulSyncHeight)

	// NogoChain-style recovery: reset sync peer to force reselection on next cycle
	if bk.syncPeer != nil {
		bk.peers.DecSyncLoad(bk.syncPeer.ID())
		bk.syncPeer = nil
	}

	// Force re-broadcast of latest block to catch up any peers that may have
	// missed the initial broadcast due to candidate pool processing delays.
	latestBlock := bk.chain.LatestBlock()
	if latestBlock != nil && bk.peers != nil {
		if broadcaster, ok := bk.peers.(interface {
			broadcastMinedBlock(block *core.Block) error
		}); ok {
			if err := broadcaster.broadcastMinedBlock(latestBlock); err != nil {
				log.Printf("[BlockKeeper] stuck escape: re-broadcast failed h=%d: %v",
					latestBlock.GetHeight(), err)
			} else {
				log.Printf("[BlockKeeper] stuck escape: re-broadcasted block h=%d to peers",
					latestBlock.GetHeight())
			}
		}
	}

	// Reset stall tracking to allow immediate retry with a new peer
	bk.lastSuccessfulSyncTime = time.Now()
	bk.lastSuccessfulSyncHeight = currentHeight

	return true
}

func (bk *blockKeeper) syncWorker() {
	genesisBlock, ok := bk.chain.BlockByHeight(0)
	if !ok || genesisBlock == nil {
		log.Printf("[BlockKeeper] syncWorker failed to get genesis block")
		return
	}

	syncTicker := time.NewTicker(syncCycle)
	defer syncTicker.Stop()

	for {
		select {
		case <-syncTicker.C:
			bk.syncActiveMu.Lock()
			if bk.syncCooldown > 0 && time.Since(bk.lastSyncAttempt) < bk.syncCooldown {
				bk.syncActiveMu.Unlock()
				continue
			}
			bk.syncActive = true
			bk.syncActiveMu.Unlock()

			bk.safeStartSync(genesisBlock)

			bk.syncActiveMu.Lock()
			bk.syncActive = false
			bk.lastSyncAttempt = time.Now()
			if bk.consecutiveSyncNoneCount >= consecutiveSyncNoneThreshold {
				bk.applyExponentialBackoff()
			}
			bk.syncActiveMu.Unlock()

		case <-bk.forkResolvedCh:
			log.Printf("[BlockKeeper] Fork resolved, triggering immediate re-sync")
			bk.syncActiveMu.Lock()
			bk.syncActive = true
			bk.consecutiveSyncNoneCount = 0
			bk.syncCooldown = 0
			bk.syncActiveMu.Unlock()

			synced := bk.startSync()

			bk.syncActiveMu.Lock()
			bk.syncActive = false
			bk.lastSyncAttempt = time.Now()
			if !synced {
				bk.consecutiveSyncNoneCount = consecutiveSyncNoneThreshold
				bk.applyExponentialBackoff()
			}
			bk.syncActiveMu.Unlock()

		case <-bk.quit:
			log.Printf("[BlockKeeper] syncWorker shutting down")
			return
		}
	}
}

// applyExponentialBackoff computes cooldown based on consecutive failure count.
// Escalation: 30s → 1min → 2min → 5min (capped at maxSyncCooldown).
func (bk *blockKeeper) applyExponentialBackoff() {
	const baseCooldown = 30 * time.Second
	periods := bk.consecutiveSyncNoneCount / consecutiveSyncNoneThreshold
	switch {
	case periods <= 1:
		bk.syncCooldown = baseCooldown
	case periods == 2:
		bk.syncCooldown = 1 * time.Minute
	case periods == 3:
		bk.syncCooldown = 2 * time.Minute
	default:
		bk.syncCooldown = maxSyncCooldown
	}
	log.Printf("[BlockKeeper] Sync backoff: %d consecutive failures (period=%d), cooldown=%v",
		bk.consecutiveSyncNoneCount, periods, bk.syncCooldown)
}

// SetCheckpointCallbacks registers callbacks for checkpoint-based sync.
func (bk *blockKeeper) SetCheckpointCallbacks(getCp func() *core.CheckpointRecord, triggerSync func()) {
	bk.getReceivedCheckpoint = getCp
	bk.triggerSyncCheck = triggerSync
}

// TriggerSyncCheck triggers an immediate sync cycle evaluation.
func (bk *blockKeeper) TriggerSyncCheck() {
	bk.syncActiveMu.Lock()
	bk.syncActive = true
	bk.consecutiveSyncNoneCount = 0
	bk.syncCooldown = 0
	bk.syncActiveMu.Unlock()
	bk.lastSyncAttempt = time.Time{}
}

// safeStartSync wraps startSync/broadcastAfterSync/syncLaggingPeers with panic
// recovery to ensure syncWorker survives unexpected panics during sync.
func (bk *blockKeeper) safeStartSync(genesisBlock *core.Block) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BlockKeeper] PANIC in startSync/broadcast: %v - continuing sync loop", r)
		}
	}()
	synced := bk.startSync()
	if synced {
		bk.broadcastAfterSync(genesisBlock)
		bk.consecutiveSyncNoneCount = 0
	} else {
		localHeight := bk.chain.LatestBlock().GetHeight()
		bk.consecutiveSyncNoneCount++
		if bk.consecutiveSyncNoneCount < consecutiveSyncNoneThreshold {
			bk.syncLaggingPeers(localHeight)
		} else if bk.consecutiveSyncNoneCount == consecutiveSyncNoneThreshold {
			log.Printf("[BlockKeeper] %d consecutive syncTypeNone rounds (localH=%d) — entering mining-friendly cooldown, skipping syncLaggingPeers",
				bk.consecutiveSyncNoneCount, localHeight)
		}
	}
}

func (bk *blockKeeper) broadcastAfterSync(genesisBlock *core.Block) {
	bestBlock := bk.chain.LatestBlock()
	if bestBlock == nil {
		log.Printf("[BlockKeeper] broadcastAfterSync: best block is nil")
		return
	}

	if err := bk.peers.broadcastMinedBlock(bestBlock); err != nil {
		log.Printf("[BlockKeeper] broadcastAfterSync broadcastMinedBlock failed: %v", err)
	}

	if err := bk.peers.broadcastNewStatus(bestBlock, genesisBlock); err != nil {
		log.Printf("[BlockKeeper] broadcastAfterSync broadcastNewStatus failed: %v", err)
	}

	bk.syncLaggingPeers(bestBlock.GetHeight())
}

func (bk *blockKeeper) syncLaggingPeers(localHeight uint64) {
	const lagThreshold uint64 = 3
	const invBatchSize = 50
	peerIDs := bk.peers.GetAllPeerIDs()

	if len(peerIDs) == 0 || localHeight == 0 {
		return
	}

	for _, peerID := range peerIDs {
		peerHeight, _, _, err := bk.peers.GetPeerChainInfo(peerID)
		if err != nil {
			continue
		}

		if peerHeight >= localHeight || localHeight-peerHeight < lagThreshold {
			continue
		}

		log.Printf("[BlockKeeper] syncLaggingPeers: peer %s behind by %d blocks, sending INV notification (local=%d, peer=%d)",
			peerID, localHeight-peerHeight, localHeight, peerHeight)

		fromHeight := peerHeight + 1
		totalMissing := localHeight - peerHeight

		for offset := uint64(0); offset < totalMissing; offset += invBatchSize {
			end := offset + invBatchSize
			if end > totalMissing {
				end = totalMissing
			}
			count := end - offset

			blocks := bk.chain.GetBlocksFrom(fromHeight+offset, count)
			if len(blocks) == 0 {
				break
			}

			hashes := make([]string, 0, len(blocks))
			for _, b := range blocks {
				hashes = append(hashes, hex.EncodeToString(b.Hash))
			}

			invMsg, invErr := reactor.BuildBlockInvMsg(hashes)
			if invErr != nil {
				log.Printf("[BlockKeeper] failed to build INV msg for %s: %v", peerID, invErr)
				continue
			}

			if !bk.peers.SendInvToPeer(peerID, invMsg) {
				log.Printf("[BlockKeeper] failed to send INV to peer %s", peerID)
				continue
			}

			log.Printf("[BlockKeeper] Sent INV with %d block hashes [%d..%d] to peer %s",
				len(hashes), fromHeight+offset, fromHeight+offset+uint64(len(hashes))-1, peerID)
		}

		log.Printf("[BlockKeeper] INV notification complete for peer %s (%d blocks announced)", peerID, totalMissing)
	}
}

func (bk *blockKeeper) TriggerImmediateReSync() {
	select {
	case bk.forkResolvedCh <- struct{}{}:
		log.Printf("[BlockKeeper] TriggerImmediateReSync signal sent")
	default:
		log.Printf("[BlockKeeper] TriggerImmediateReSync signal already pending")
	}
}

func (bk *blockKeeper) Stop() {
	close(bk.quit)
	log.Printf("[BlockKeeper] BlockKeeper stopped")
}

// cleanupExpiredEntries periodically removes expired entries from failedSyncPeers
// map to prevent unbounded memory growth.
func (bk *blockKeeper) cleanupExpiredEntries() {
	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()
	for {
		select {
		case <-bk.quit:
			return
		case <-cleanupTicker.C:
			now := time.Now()
			bk.syncStateMu.Lock()
			for peerID, lastFail := range bk.failedSyncPeers {
				if now.After(lastFail.Add(bk.failedSyncCooldownDur * 2)) {
					delete(bk.failedSyncPeers, peerID)
				}
			}
			bk.syncStateMu.Unlock()
		}
	}
}

const (
	SFFullNode = 1 << iota
)

const (
	LevelMsgIllegal = 0x01
)

type ChainInterface interface {
	LatestBlock() *core.Block
	BlockByHeight(height uint64) (*core.Block, bool)
	BlockByHash(hashHex string) (*core.Block, bool)
	BestBlockHeader() (*HeaderLocator, error)
	GetHeaderByHeight(height uint64) (*HeaderLocator, error)
	AddBlock(block *core.Block) (bool, error)
	GetBlockByHash(hash []byte) (*core.Block, bool)
	GetBlocksFrom(from uint64, count uint64) []*core.Block
	CanonicalWork() *big.Int
	RollbackToHeight(height uint64) error
	LatestCheckpoint() (uint64, string, error)
	GetCheckpointByHeight(height uint64) (string, bool, error)
}

type PeerSetInterface interface {
	bestPeer(serviceFlag int) PeerInterface
	ProcessIllegal(peerID string, level byte, reason string)
	broadcastMinedBlock(block *core.Block) error
	broadcastNewStatus(bestBlock, genesisBlock *core.Block) error
	GetPeerChainInfo(peerID string) (height uint64, work *big.Int, tipHash string, err error)
	PushBlocksToPeer(peerID string, blocks []*core.Block) (int, error)
	GetAllPeerIDs() []string
	SendInvToPeer(peerID string, invMsg []byte) bool
	IncSyncLoad(peerAddr string)
	DecSyncLoad(peerAddr string)
	PeerByID(peerID string) PeerInterface
}

type PeerInterface interface {
	ID() string
	Height() uint64
	getBlockByHeight(height uint64) bool
	getBlocksByHeights(heights []uint64) bool
	getBlocks(locator [][]byte, stopHash []byte) bool
	getHeaders(locator [][]byte, stopHash []byte) bool
	fetchBlock(ctx context.Context, height uint64) (*core.Block, error)
	fetchBlocksBatch(ctx context.Context, startHeight uint64, count uint64) ([]*core.Block, error)
	fetchBlocksByLocator(ctx context.Context, locator [][]byte, stopHash []byte) ([]*core.Block, error)
	fetchHeadersByLocator(ctx context.Context, locator [][]byte, stopHash []byte) ([]*HeaderLocator, error)
}

// PeerDisconnectNotifier defines the interface for receiving peer disconnection notifications.
// Switch calls OnPeerDisconnected when a peer is removed so that components like
// blockKeeper can clear stale references to the disconnected peer.
type PeerDisconnectNotifier interface {
	OnPeerDisconnected(peerID string)
}

// decodeMinerAddress decodes a miner address from hex string to 32-byte format
func decodeMinerAddress(addrHex string) ([32]byte, error) {
	var out [32]byte
	if len(addrHex) > 4 && addrHex[:4] == core.AddressPrefix {
		encoded := addrHex[4:]
		b, err := hex.DecodeString(encoded)
		if err != nil {
			return out, fmt.Errorf("decode NOGO address: %w", err)
		}
		if len(b) < 33 {
			return out, errors.New("NOGO address too short")
		}
		if len(b) == 37 {
			copy(out[:], b[1:33])
		} else {
			return out, errors.New("invalid NOGO address length")
		}
	} else {
		b, err := hex.DecodeString(addrHex)
		if err != nil {
			return out, fmt.Errorf("decode hex: %w", err)
		}
		if len(b) != 32 {
			return out, fmt.Errorf("expected 32 bytes, got %d", len(b))
		}
		copy(out[:], b)
	}
	return out, nil
}

// computeHeaderHash computes the hash of a block header using SHA256
func computeHeaderHash(h *core.BlockHeader, height uint64, minerAddress string) ([]byte, error) {
	hasher := sha256.New()
	hasher.Write(h.PrevHash)

	timestampBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(timestampBytes, uint64(h.TimestampUnix))
	hasher.Write(timestampBytes)

	difficultyBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(difficultyBytes, h.DifficultyBits)
	hasher.Write(difficultyBytes)

	nonceBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(nonceBytes, h.Nonce)
	hasher.Write(nonceBytes)

	hasher.Write(h.MerkleRoot)

	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, height)
	hasher.Write(heightBytes)

	miner, err := decodeMinerAddress(minerAddress)
	if err != nil {
		return nil, fmt.Errorf("decode miner address: %w", err)
	}
	hasher.Write(miner[:])

	return hasher.Sum(nil), nil
}
