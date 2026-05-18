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
	"bytes"
	"container/list"
	"context"
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
	"github.com/nogochain/nogo/blockchain/network/forkresolution"
	"github.com/nogochain/nogo/blockchain/network/reactor"
)

const (
	syncCycle            = 3 * time.Second
	blockProcessChSize   = 1024
	blocksProcessChSize  = 128
	headersProcessChSize = 1024

	logCooldownDur = 30 * time.Second

	// Mining safety valve: after consecutiveSyncNoneThreshold no-op sync rounds
	// (all peers at same height/work), declare sync idle so mining can proceed.
	consecutiveSyncNoneThreshold = 3

	// NogoChain-style sync type classification for intelligent strategy selection
	syncTypeNone          = iota // no peer available or already synced
	syncTypeFast                 // fast sync via checkpoint skeleton download
	syncTypeRegular              // regular sequential block download
	syncTypeForkDetection        // equal height but peer has more cumulative work
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

	// UNIFIED FORK RESOLUTION: Uses core-main based architecture
	forkResolver     *forkresolution.ForkResolver
	multiNodeArbiter *forkresolution.MultiNodeArbitrator

	forkDetectionCooldown    map[string]time.Time
	forkDetectionCooldownDur time.Duration

	// Failed sync peer tracking: prevents infinite rollback→resync→fail→rollback loops.
	// When a peer causes a chain mismatch during sync, it is deprioritized for
	// failedSyncCooldownDur to allow sync from other peers.
	failedSyncPeers       map[string]time.Time
	failedSyncCooldownDur time.Duration

	syncActive   bool
	syncActiveMu sync.Mutex

	// syncStateMu protects forkDetectionCooldown and failedSyncPeers maps
	// from concurrent access by cleanupExpiredEntries and syncWorker goroutines
	syncStateMu sync.Mutex

	getReceivedCheckpoint func() *core.CheckpointRecord
	triggerSyncCheck      func()

	lastSyncAttempt          time.Time
	syncCooldown             time.Duration
	syncIdleConfirmed        bool // true when sync is confirmed complete, mining can proceed
	consecutiveSyncNoneCount int  // safety valve: tracks consecutive syncTypeNone rounds

	logLimiter *rateLimitedLogger

	lastSyncedLogHeight uint64
}

func (bk *blockKeeper) isActive() bool {
	bk.syncActiveMu.Lock()
	defer bk.syncActiveMu.Unlock()
	if bk.syncIdleConfirmed {
		return false
	}
	if bk.syncActive {
		return true
	}
	if bk.syncCooldown > 0 && time.Since(bk.lastSyncAttempt) < bk.syncCooldown {
		return true
	}
	return false
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
		chain:                    chain,
		peers:                    peers,
		candidatePool:            candidatePool,
		blockProcessCh:           make(chan *blockMsg, blockProcessChSize),
		blocksProcessCh:          make(chan *blocksMsg, blocksProcessChSize),
		headersProcessCh:         make(chan *headersMsg, headersProcessChSize),
		syncBlockCh:              make(chan *blockMsg, 2048),
		headerList:               list.New(),
		forkResolvedCh:           make(chan struct{}, 1),
		quit:                     make(chan struct{}),
		forkDetectionCooldown:    make(map[string]time.Time),
		forkDetectionCooldownDur: 1 * time.Second,
		failedSyncPeers:          make(map[string]time.Time),
		failedSyncCooldownDur:    15 * time.Second,
		logLimiter:               newRateLimitedLogger(logCooldownDur),
	}
	bk.resetHeaderState()
	go bk.syncWorker()
	go bk.cleanupExpiredEntries()
	return bk
}

// SetForkResolver sets the unified fork resolver (core-main based architecture)
// This must be called during node initialization after ForkResolver is created
// All fork operations from BlockKeeper will go through this unified engine
func (bk *blockKeeper) SetForkResolver(resolver *forkresolution.ForkResolver) {
	bk.forkResolver = resolver
	log.Printf("[BlockKeeper] ForkResolver (core-main architecture) set for unified fork resolution")
}

// SetMultiNodeArbitrator sets the multi-node arbitrator for enhanced consensus
// Enables weighted voting for 3+ node networks
func (bk *blockKeeper) SetMultiNodeArbitrator(arbiter *forkresolution.MultiNodeArbitrator) {
	bk.multiNodeArbiter = arbiter
	log.Printf("[BlockKeeper] MultiNodeArbitrator set for enhanced fork arbitration")
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

func (bk *blockKeeper) fastBlockSync(checkpoint *config.TrustedCheckpoint) error {
	bk.resetHeaderState()

	bestHeader, err := bk.chain.BestBlockHeader()
	if err != nil {
		return fmt.Errorf("fastBlockSync get best header: %w", err)
	}

	if bestHeader == nil {
		return errors.New("fastBlockSync: best header is nil")
	}

	lastHeader := bestHeader
	checkpointHash, decodeErr := hex.DecodeString(checkpoint.Hash)
	if decodeErr != nil {
		return fmt.Errorf("fastBlockSync decode checkpoint hash: %w", decodeErr)
	}

	for ; !equalBytes(lastHeader.Header.PrevHash, checkpointHash); lastHeader = bk.headerList.Back().Value.(*HeaderLocator) {
		if lastHeader.Height >= checkpoint.Height {
			return fmt.Errorf("%w: peer is not in the checkpoint branch", errPeerMisbehave)
		}

		var lastHash []byte
		block, ok := bk.chain.BlockByHeight(lastHeader.Height)
		if !ok || block == nil {
			lastHash = lastHeader.Header.PrevHash
		} else {
			lastHash = block.GetHash()
		}

		if lastHash == nil {
			return fmt.Errorf("%w: cannot determine last hash for height %d", errPeerMisbehave, lastHeader.Height)
		}

		headers, err := bk.requireHeaders([][]byte{lastHash}, checkpointHash)
		if err != nil {
			return fmt.Errorf("fastBlockSync requireHeaders: %w", err)
		}

		if len(headers) == 0 {
			return fmt.Errorf("%w: requireHeaders return empty list", errPeerMisbehave)
		}

		if err := bk.appendHeaderList(headers); err != nil {
			return fmt.Errorf("fastBlockSync appendHeaderList: %w", err)
		}
	}

	fastHeader := bk.headerList.Front()
	for bk.chain.LatestBlock().GetHeight() < checkpoint.Height {
		locator := bk.blockLocator()
		blocks, err := bk.requireBlocks(locator, checkpointHash)
		if err != nil {
			return fmt.Errorf("fastBlockSync requireBlocks: %w", err)
		}

		if len(blocks) == 0 {
			return fmt.Errorf("%w: requireBlocks return empty list", errPeerMisbehave)
		}

		for _, block := range blocks {
			if fastHeader == nil {
				return errors.New("get block that is higher than checkpoint")
			}

			fastHeader = fastHeader.Next()
			if fastHeader == nil {
				return errors.New("get block that is higher than checkpoint")
			}

			expectedHeader := fastHeader.Value.(*HeaderLocator)
			blockHash := block.GetHash()
			expectedBlock, ok := bk.chain.BlockByHeight(expectedHeader.Height)
			if !ok {
				return fmt.Errorf("%w: expected block not found at height %d", errPeerMisbehave, expectedHeader.Height)
			}

			if !equalBytes(blockHash, expectedBlock.GetHash()) {
				return errPeerMisbehave
			}

			isOrphan, processErr := bk.chain.AddBlock(block)
			if processErr != nil {
				return fmt.Errorf("fastBlockSync process block at height %d: %w", block.GetHeight(), processErr)
			}

			if isOrphan {
				log.Printf("[BlockKeeper] Warning: orphan block during fast sync at height %d", block.GetHeight())
			}
		}
	}
	return nil
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

	if bk.logLimiter.shouldLog("batch_sync_start") {
		log.Printf("[BlockKeeper] Starting batch sync: h=%d → target=%d (gap=%d, batchSize=%d)",
			startHeight, targetHeight, targetHeight-startHeight+1, batchLimit)
	}

	for i <= targetHeight {
		batchSize := targetHeight - i + 1
		if batchSize > batchLimit {
			batchSize = batchLimit
		}

		blocks, batchErr := bk.requireBlocksBatch(i, batchSize)
		if batchErr != nil || len(blocks) == 0 {
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
	}

	return nil
}

func (bk *blockKeeper) requireBlock(height uint64) (*core.Block, error) {
	if bk.syncPeer == nil {
		return nil, fmt.Errorf("%w: syncPeer is nil", errPeerDropped)
	}

	if ok := bk.syncPeer.getBlockByHeight(height); !ok {
		return nil, errPeerDropped
	}

	sessionSeq := bk.syncSessionSeq

	timeout := time.NewTimer(syncTimeout)
	defer timeout.Stop()

	for {
		select {
		case msg := <-bk.syncBlockCh:
			if msg.peerID != bk.syncPeer.ID() {
				continue
			}
			if msg.block == nil {
				continue
			}
			if msg.block.GetHeight() != height {
				continue
			}
			if msg.sessionSeq != sessionSeq {
				// Block from a previous sync session (stale).
				// This happens after RollbackToHeight triggers a new sync
				// while old session blocks are still in-flight.
				log.Printf("[BlockKeeper] requireBlock h=%d skipping stale block (sess=%d, current=%d)",
					height, msg.sessionSeq, sessionSeq)
				continue
			}
			return msg.block, nil
		case <-timeout.C:
			return nil, fmt.Errorf("%w: requireBlock height=%d", errRequestTimeout, height)
		case <-bk.quit:
			return nil, errors.New("blockKeeper shutdown")
		}
	}
}

func (bk *blockKeeper) requireBlockFast(height uint64) (*core.Block, error) {
	if bk.syncPeer == nil {
		return nil, fmt.Errorf("%w: syncPeer is nil", errPeerDropped)
	}

	if ok := bk.syncPeer.getBlockByHeight(height); !ok {
		return nil, errPeerDropped
	}

	sessionSeq := bk.syncSessionSeq

	timeout := time.NewTimer(fastSyncTimeout)
	defer timeout.Stop()

	for {
		select {
		case msg := <-bk.syncBlockCh:
			if msg.peerID != bk.syncPeer.ID() {
				continue
			}
			if msg.block == nil {
				continue
			}
			if msg.block.GetHeight() != height {
				continue
			}
			if msg.sessionSeq != sessionSeq {
				continue
			}
			return msg.block, nil
		case <-timeout.C:
			return nil, fmt.Errorf("%w: requireBlockFast height=%d", errRequestTimeout, height)
		case <-bk.quit:
			return nil, errors.New("blockKeeper shutdown")
		}
	}
}

func (bk *blockKeeper) requireBlocksBatch(startHeight uint64, count uint64) (map[uint64]*core.Block, error) {
	if bk.syncPeer == nil {
		return nil, fmt.Errorf("%w: syncPeer is nil", errPeerDropped)
	}

	heights := make([]uint64, 0, count)
	for i := uint64(0); i < count; i++ {
		heights = append(heights, startHeight+i)
	}

	if ok := bk.syncPeer.getBlocksByHeights(heights); !ok {
		return nil, errPeerDropped
	}

	result := make(map[uint64]*core.Block)
	sessionSeq := bk.syncSessionSeq
	timeout := time.NewTimer(batchSyncTimeout)
	defer timeout.Stop()

	expected := int(count)

	for len(result) < expected && expected > 0 {
		select {
		case msg := <-bk.syncBlockCh:
			if msg.peerID != bk.syncPeer.ID() {
				continue
			}
			if msg.block == nil {
				continue
			}
			if msg.sessionSeq != sessionSeq {
				continue
			}
			h := msg.block.GetHeight()
			if h >= startHeight && h < startHeight+count {
				if _, exists := result[h]; !exists {
					result[h] = msg.block
				}
			}
		case <-timeout.C:
			log.Printf("[BlockKeeper] requireBlocksBatch: timeout waiting for %d/%d blocks (start=%d)",
				len(result), expected, startHeight)
			if len(result) > 0 {
				return result, nil
			}
			return nil, fmt.Errorf("%w: requireBlocksBatch timeout (got %d/%d)", errRequestTimeout, len(result), expected)
		case <-bk.quit:
			return result, nil
		}
	}

	return result, nil
}

func (bk *blockKeeper) requireBlocks(locator [][]byte, stopHash []byte) ([]*core.Block, error) {
	if bk.syncPeer == nil {
		return nil, fmt.Errorf("%w: syncPeer is nil", errPeerDropped)
	}

	if ok := bk.syncPeer.getBlocks(locator, stopHash); !ok {
		return nil, errPeerDropped
	}

	timeout := time.NewTimer(syncTimeout)
	defer timeout.Stop()

	for {
		select {
		case msg := <-bk.blocksProcessCh:
			if msg.peerID != bk.syncPeer.ID() {
				continue
			}
			return msg.blocks, nil
		case <-timeout.C:
			return nil, fmt.Errorf("%w: requireBlocks", errRequestTimeout)
		case <-bk.quit:
			return nil, errors.New("blockKeeper shutdown")
		}
	}
}

func (bk *blockKeeper) requireHeaders(locator [][]byte, stopHash []byte) ([]*HeaderLocator, error) {
	if bk.syncPeer == nil {
		return nil, fmt.Errorf("%w: syncPeer is nil", errPeerDropped)
	}

	if ok := bk.syncPeer.getHeaders(locator, stopHash); !ok {
		return nil, errPeerDropped
	}

	timeout := time.NewTimer(syncTimeout)
	defer timeout.Stop()

	for {
		select {
		case msg := <-bk.headersProcessCh:
			if msg.peerID != bk.syncPeer.ID() {
				continue
			}
			return msg.headers, nil
		case <-timeout.C:
			return nil, fmt.Errorf("%w: requireHeaders", errRequestTimeout)
		case <-bk.quit:
			return nil, errors.New("blockKeeper shutdown")
		}
	}
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
// CRITICAL FIX: Work-prioritized peer selection.
// Previously, bestPeer() selected the peer with the highest BLOCK HEIGHT,
// ignoring cumulative WORK. This caused a minority-fork node with many
// low-work blocks to be chosen as the sync target, resulting in:
//  1. ABCDE (most work, lower height) tries to sync from F (higher
//     height, less work) → F's blocks fail validation → retry loop
//  2. All ABCDE nodes stall, unable to mine or sync
//
// Now: compares cumulative work FIRST, then height.
func (bk *blockKeeper) checkSyncType() (int, PeerInterface) {
	blockHeight := bk.chain.LatestBlock().GetHeight()
	localWork := bk.chain.CanonicalWork()
	checkpoint := bk.nextCheckpoint()

	peerIDs := bk.peers.GetAllPeerIDs()
	if len(peerIDs) == 0 {
		return syncTypeNone, nil
	}

	type peerResult struct {
		peerID string
		height uint64
		work   *big.Int
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
			ph, pw, _, err := bk.peers.GetPeerChainInfo(peerID)
			if err != nil || pw == nil {
				return
			}
			select {
			case resultCh <- peerResult{peerID: peerID, height: ph, work: pw}:
			case <-peerCtx.Done():
			}
		}(pid)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var bestByWork PeerInterface
	var bestWork *big.Int
	var bestWorkHeight uint64

	for r := range resultCh {
		if bestWork == nil || r.work.Cmp(bestWork) > 0 {
			bestWork = r.work
			bestWorkHeight = r.height
			bestByWork = bk.wrapPeer(r.peerID, r.height)
		}
	}

	if bestByWork == nil {
		log.Printf("[BlockKeeper] checkSyncType: no peer has valid work info (localH=%d)", blockHeight)
		return syncTypeNone, nil
	}

	peerWork := bestWork
	peerHeight := bestWorkHeight

	// Compare cumulative work: the heaviest chain wins.
	if localWork == nil {
		localWork = big.NewInt(0)
	}

	if bk.logLimiter.shouldLog("checkSyncType") {
		log.Printf("[BlockKeeper] checkSyncType: local(H=%d,W=%s) vs bestPeer(H=%d,W=%s,ID=%s)",
			blockHeight, localWork.String(), peerHeight, peerWork.String(), bestByWork.ID())
	}

	// Checkpoint-aware peer filtering for new nodes:
	// When a P2P checkpoint record exists, filter out peers that cannot verify
	// the checkpoint. This prevents new nodes from syncing to wrong/malicious peers.
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

	// Priority 1: fast sync when hardcoded checkpoint is ahead and peer supports it.
	if checkpoint != nil && peerHeight >= checkpoint.Height {
		const minGap = uint64(128)
		if checkpoint.Height >= blockHeight+minGap {
			return syncTypeFast, bestByWork
		}
	}

	// Priority 2: fast sync when BoltDB checkpoint is ahead and peer supports it.
	if checkpoint == nil && peerHeight > blockHeight {
		boltCheckpointH, _ := bk.nextCheckpointHeight(peerHeight)
		const boltMinGap = uint64(128)
		if boltCheckpointH >= blockHeight+boltMinGap {
			log.Printf("[BlockKeeper] checkSyncType: fast sync via BoltDB checkpoint h=%d (local=%d, peer=%d)",
				boltCheckpointH, blockHeight, peerHeight)
			return syncTypeFast, bestByWork
		}
	}

	// Priority 3: fast sync for very large gaps without checkpoints.
	// When gap is huge, skeleton download is still more efficient than sequential.
	if checkpoint == nil && peerHeight > blockHeight {
		const largeGap = uint64(5000)
		if peerHeight-blockHeight >= largeGap {
			boltCheckpointH, _ := bk.nextCheckpointHeight(peerHeight)
			if boltCheckpointH > blockHeight && boltCheckpointH <= peerHeight {
				log.Printf("[BlockKeeper] checkSyncType: fast sync for large gap (gap=%d, boltCp=%d)",
					peerHeight-blockHeight, boltCheckpointH)
				return syncTypeFast, bestByWork
			}
		}
	}

	// Priority 4: fast sync via P2P received checkpoint (new node).
	// When local has no checkpoints, use checkpoint received from peer via P2P.
	if checkpoint == nil && peerHeight > blockHeight {
		if bk.getReceivedCheckpoint != nil {
			if cp := bk.getReceivedCheckpoint(); cp != nil && cp.Height > blockHeight {
				const p2pMinGap = uint64(128)
				if cp.Height >= blockHeight+p2pMinGap {
					log.Printf("[BlockKeeper] checkSyncType: fast sync via P2P checkpoint h=%d (local=%d, peer=%d)",
						cp.Height, blockHeight, peerHeight)
					return syncTypeFast, bestByWork
				}
			}
		}
	}

	// CRITICAL FIX: Work comparison must come BEFORE height comparison.
	// A peer with higher height but LESS work is on a minority fork.
	// We should NOT sync from it.
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
			minForkHeight := uint64(128)
			if blockHeight < minForkHeight {
				log.Printf("[BlockKeeper] checkSyncType: regular sync (peer has more work at same low height=%d < min=%d)",
					blockHeight, minForkHeight)
				if realPeer := bk.peers.PeerByID(bestByWork.ID()); realPeer != nil && realPeer.Height() > 0 {
					return syncTypeRegular, realPeer
				}
				return syncTypeRegular, bestByWork
			}
			log.Printf("[BlockKeeper] checkSyncType: fork detection (peer has more work at same height=%d)",
				blockHeight)
			if realPeer := bk.peers.PeerByID(bestByWork.ID()); realPeer != nil && realPeer.Height() > 0 {
				return syncTypeForkDetection, realPeer
			}
			return syncTypeForkDetection, bestByWork
		}
		log.Printf("[BlockKeeper] checkSyncType: fork detection (peer has more work but lower height=%d, local=%d)",
			peerHeight, blockHeight)
		if realPeer := bk.peers.PeerByID(bestByWork.ID()); realPeer != nil && realPeer.Height() > 0 {
			return syncTypeForkDetection, realPeer
		}
		return syncTypeForkDetection, bestByWork
	}

	// peerWork <= localWork: we are winning or equal.
	// Do NOT sync from a peer that has less work, even if it has higher height.
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
	case syncTypeFast:
		return bk.dispatchFastSync(peer, blockHeight)

	case syncTypeRegular:
		return bk.dispatchRegularSync(peer, blockHeight)

	case syncTypeForkDetection:
		return bk.dispatchForkDetection(peer, blockHeight)

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

// dispatchFastSync executes fast synchronization via checkpoint skeleton download.
// NogoChain-style pattern: single responsibility dispatch with clear error handling.
func (bk *blockKeeper) dispatchFastSync(peer PeerInterface, localHeight uint64) bool {
	if _, isSimple := peer.(*simplePeer); isSimple {
		if realPeer := bk.peers.bestPeer(SFFullNode); realPeer != nil && realPeer.ID() == peer.ID() {
			peer = realPeer
		} else {
			log.Printf("[BlockKeeper] dispatchFastSync: work-optimal peer %s not found via bestPeer, skipping", peer.ID())
			return false
		}
	}

	checkpoint := bk.nextCheckpoint()
	if checkpoint == nil {
		if boltH, boltHash, err := bk.chain.LatestCheckpoint(); err == nil && boltH > localHeight {
			checkpoint = &config.TrustedCheckpoint{Height: boltH, Hash: boltHash}
			log.Printf("[BlockKeeper] dispatchFastSync: using BoltDB checkpoint h=%d (local=%d)", boltH, localHeight)
		}
	}
	if checkpoint == nil {
		if bk.getReceivedCheckpoint != nil {
			if cp := bk.getReceivedCheckpoint(); cp != nil && cp.Height > localHeight {
				checkpoint = &config.TrustedCheckpoint{Height: cp.Height, Hash: cp.BlockHash}
				log.Printf("[BlockKeeper] dispatchFastSync: using P2P checkpoint h=%d (local=%d)", cp.Height, localHeight)
			}
		}
	}
	if checkpoint == nil {
		return false
	}

	bk.syncPeer = peer
	log.Printf("[BlockKeeper] FAST sync to checkpoint h=%d via peer %s (peerH=%d)",
		checkpoint.Height, peer.ID(), peer.Height())

	if err := bk.fastBlockSync(checkpoint); err != nil {
		log.Printf("[BlockKeeper] fastBlockSync failed: %v", err)
		bk.peers.ProcessIllegal(peer.ID(), LevelMsgIllegal, err.Error())
		bk.peers.DecSyncLoad(peer.ID())
		return false
	}

	newHeight := bk.chain.LatestBlock().GetHeight()
	log.Printf("[BlockKeeper] Fast sync completed (peer=%s, now at h=%d)", peer.ID(), newHeight)
	bk.recordSyncProgress(newHeight)
	bk.peers.DecSyncLoad(peer.ID())
	return true
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
	// Return wrapper anyway; fork detection only needs ID (via getPeerTipBlock),
	// and regular sync will use the peer's ID to look up chain info.
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

	gapSize := peerHeight - localHeight
	log.Printf("[BlockKeeper] REGULAR sync: h=%d → h=%d (gap=%d) via peer %s (peerH=%d)",
		localHeight, peerHeight, gapSize, peer.ID(), peerHeight)

	batchLimit := uint64(maxBlockPerMsg)
	if gapSize > maxBlockPerMsg*10 {
		batchLimit = maxBlockPerMsg * largeGapBatchMult
		log.Printf("[BlockKeeper] Large sync gap (%d blocks), using expanded batch limit %d", gapSize, batchLimit)
	}

	syncedAny := false
	for localHeight < peerHeight {
		targetHeight := localHeight + batchLimit
		if targetHeight > peerHeight {
			targetHeight = peerHeight
		}

		err := bk.regularBlockSync(targetHeight, batchLimit)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, errChainMismatch.Error()) {
				log.Printf("[BlockKeeper] Chain mismatch detected (peer on different fork): %v", err)
				bk.handleChainMismatchInSync(peer)
				bk.peers.DecSyncLoad(peer.ID())
				return true
			}

			peerWork := bk.getPeerWork(peer)
			if peerWork != nil {
				localWork := bk.chain.CanonicalWork()
				if localWork != nil && peerWork.Cmp(localWork) > 0 {
					log.Printf("[BlockKeeper] Peer %s has more work (peer=%s > local=%s) but sync failed: %v - treating as chain mismatch",
						peer.ID(), peerWork.String(), localWork.String(), err)
					bk.handleChainMismatchInSync(peer)
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

// dispatchForkDetection handles the case where local and peer are at the same height
// but the peer has a chain with greater cumulative work — indicating a fork.
// Uses ForkResolver.ShouldReorg for work-weighted decision before any rollback.
func (bk *blockKeeper) dispatchForkDetection(peer PeerInterface, localHeight uint64) bool {
	if bk.forkResolver == nil {
		log.Printf("[BlockKeeper] dispatchForkDetection: forkResolver is nil")
		return false
	}

	peerHeight := peer.Height()
	if peerHeight == 0 {
		h, _, _, err := bk.peers.GetPeerChainInfo(peer.ID())
		if err == nil && h > 0 {
			peerHeight = h
		} else {
			log.Printf("[BlockKeeper] dispatchForkDetection: cannot determine peer height for %s (err=%v), skipping",
				peer.ID(), err)
			return false
		}
	}
	peerID := peer.ID()

	cooldownKey := fmt.Sprintf("%s:%d", peerID, peerHeight)
	bk.syncStateMu.Lock()
	if lastCheck, exists := bk.forkDetectionCooldown[cooldownKey]; exists {
		if time.Since(lastCheck) < bk.forkDetectionCooldownDur {
			bk.syncStateMu.Unlock()
			log.Printf("[BlockKeeper] dispatchForkDetection: cooling down peer=%s height=%d (last attempt=%v ago)",
				peerID, peerHeight, time.Since(lastCheck).Round(time.Millisecond))
			return false
		}
	}
	bk.forkDetectionCooldown[cooldownKey] = time.Now()
	bk.syncStateMu.Unlock()

	log.Printf("[BlockKeeper] dispatchForkDetection: fork candidate (local=%d peer=%d), evaluating work",
		localHeight, peerHeight)

	peerTip := bk.getPeerTipBlock(peer)
	if peerTip == nil {
		log.Printf("[BlockKeeper] dispatchForkDetection: cannot build peer tip block")
		bk.syncStateMu.Lock()
		delete(bk.forkDetectionCooldown, cooldownKey)
		bk.syncStateMu.Unlock()
		return false
	}

	if !bk.forkResolver.ShouldReorg(peerTip) {
		log.Printf("[BlockKeeper] dispatchForkDetection: peer chain does not have more work, keeping local chain — will retry after cooldown")
		// Cooldown is set above, prevents hot loop but allows retry after cooldownDur
		return false
	}

	log.Printf("[BlockKeeper] dispatchForkDetection: peer chain has more work, running height-aligned fork switch")
	bk.syncStateMu.Lock()
	delete(bk.forkDetectionCooldown, cooldownKey)
	bk.syncStateMu.Unlock()
	bk.reorgChainToHeaviestPeer(peer)
	return true
}

const (
	maxReorgSearchDepth   = uint64(100)
	maxReorgSearchTimeout = 90 * time.Second
)

// handleChainMismatchInSync processes chain mismatch events during sync.
//
// NAKAMOTO CONSENSUS PRINCIPLE:
// The chain with the most cumulative proof-of-work is the correct chain.
// When the heaviest peer's blocks don't connect to our chain, we MUST find
// the common ancestor and switch — NOT blacklist the heaviest peer and
// settle for a lighter chain. That would violate the fundamental consensus rule.
//
// ALGORITHM (progressive deep rollback):
//  1. Roll back 1 block → request peer's block at rollbackTarget+1
//  2. Check if peer's block PrevHash matches our new tip hash
//  3. If YES → found common ancestor → add block → trigger resync
//  4. If NO  → roll back deeper → repeat up to maxReorgSearchDepth
//  5. If max depth exceeded without match → deprioritize peer temporarily
//
// This is equivalent to Bitcoin Core's chain reorganization logic:
// find the fork point by walking backward, then switch to the heavier chain.
func (bk *blockKeeper) handleChainMismatchInSync(peer PeerInterface) {
	if bk.forkResolver == nil || bk.chain == nil {
		return
	}

	cooldownKey := "chain_mismatch:" + peer.ID()
	bk.syncStateMu.Lock()
	if lastHandle, exists := bk.forkDetectionCooldown[cooldownKey]; exists {
		if time.Since(lastHandle) < 5*time.Second {
			bk.syncStateMu.Unlock()
			log.Printf("[BlockKeeper] handleChainMismatchInSync: cooling down for peer %s (last attempt=%v ago)",
				peer.ID(), time.Since(lastHandle).Round(time.Millisecond))
			return
		}
	}
	bk.forkDetectionCooldown[cooldownKey] = time.Now()
	bk.syncStateMu.Unlock()

	localTip := bk.chain.LatestBlock()
	if localTip == nil {
		return
	}

	peerWork := bk.getPeerWork(peer)
	localWork := bk.chain.CanonicalWork()
	if peerWork == nil || localWork == nil || peerWork.Cmp(localWork) <= 0 {
		log.Printf("[BlockKeeper] handleChainMismatchInSync: peer %s work not greater than local, skipping reorg",
			peer.ID())
		return
	}

	log.Printf("[BlockKeeper] Chain mismatch with heaviest peer %s (peerW=%s > localW=%s, peerH reported), searching for common ancestor via progressive rollback (max depth=%d, timeout=%v)",
		peer.ID(), peerWork.String(), localWork.String(), maxReorgSearchDepth, maxReorgSearchTimeout)

	startTime := time.Now()
	origSyncPeer := bk.syncPeer
	bk.syncPeer = peer
	defer func() {
		bk.syncPeer = origSyncPeer
	}()

	originalHeight := localTip.GetHeight()
	maxRollbackDepth := uint64(100)
	if originalHeight < maxRollbackDepth {
		maxRollbackDepth = originalHeight - 1
	}
	if maxRollbackDepth > maxReorgSearchDepth {
		maxRollbackDepth = maxReorgSearchDepth
	}

	for depth := uint64(1); depth <= maxRollbackDepth; depth++ {
		if time.Since(startTime) > maxReorgSearchTimeout {
			log.Printf("[BlockKeeper] Chain mismatch: progressive rollback timed out after %v (depth=%d)",
				maxReorgSearchTimeout, depth)
			break
		}
		rollbackTarget := originalHeight - depth

		if err := bk.chain.RollbackToHeight(rollbackTarget); err != nil {
			log.Printf("[BlockKeeper] Chain mismatch: rollback to h=%d failed: %v", rollbackTarget, err)
			break
		}

		newTip := bk.chain.LatestBlock()
		if newTip == nil {
			break
		}

		reconnectHeight := rollbackTarget + 1
		peerBlock, err := bk.requireBlock(reconnectHeight)
		if err != nil {
			log.Printf("[BlockKeeper] Chain mismatch: request peer block h=%d failed: %v — continuing deeper search",
				reconnectHeight, err)
			continue
		}

		if len(peerBlock.Header.PrevHash) > 0 && len(newTip.Hash) > 0 &&
			bytes.Equal(peerBlock.Header.PrevHash, newTip.Hash) {
			_, addErr := bk.chain.AddBlock(peerBlock)
			if addErr != nil {
				log.Printf("[BlockKeeper] Chain mismatch: found ancestor at h=%d but add block h=%d failed: %v",
					rollbackTarget, reconnectHeight, addErr)
				break
			}
			newHeight := bk.chain.LatestBlock().GetHeight()
			log.Printf("[BlockKeeper] Chain mismatch: FOUND common ancestor at h=%d, reorg depth=%d, new tip=%d, heaviest chain switch successful — triggering resync",
				rollbackTarget, depth, newHeight)
			bk.recordSyncProgress(newHeight)
			bk.TriggerImmediateReSync()
			return
		}

		if depth%10 == 0 {
			log.Printf("[BlockKeeper] Chain mismatch: progressive rollback at depth=%d, still searching for common ancestor...",
				depth)
		}
	}

	currentHeight := bk.chain.LatestBlock().GetHeight()
	if currentHeight < originalHeight {
		log.Printf("[BlockKeeper] Chain mismatch: search FAILED, chain rolled back from h=%d to h=%d "+
			"(%d blocks lost, bounded). Relying on syncWorker to restore via normal resync.",
			originalHeight, currentHeight, originalHeight-currentHeight)
	}

	log.Printf("[BlockKeeper] Chain mismatch: exhausted search depth %d without finding common ancestor — peer %s deprioritized for %v",
		maxReorgSearchDepth, peer.ID(), bk.failedSyncCooldownDur)
	bk.syncStateMu.Lock()
	bk.failedSyncPeers[peer.ID()] = time.Now()
	bk.syncStateMu.Unlock()
	bk.TriggerImmediateReSync()
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

// getPeerTipBlock constructs a block representing the peer's tip.
// Populates TotalWork from the peer's chain info and attempts to look up
// the parent hash from the local chain. A valid PrevHash is essential for
// ForkResolver.findAncestorForReorg to locate the actual fork point.
// Without it, the fallback rollback is always shallow (1 block), which
// causes the infinite "rollback → resync → mismatch → rollback" loop for
// forks deeper than 1 block.
func (bk *blockKeeper) getPeerTipBlock(peer PeerInterface) *core.Block {
	if bk.peers == nil || peer == nil {
		return nil
	}

	height, workBig, tipHash, err := bk.peers.GetPeerChainInfo(peer.ID())
	if err != nil {
		return nil
	}
	if workBig == nil {
		return nil
	}

	var prevHash []byte
	if f, ok := bk.peers.(peerChainMetaFetcher); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		meta, mErr := f.FetchPeerChainMeta(ctx, peer.ID())
		cancel()
		if mErr == nil && meta != nil && meta.TipPrevHash != "" {
			prevHash = decodeFlexibleHash([]byte(strings.TrimSpace(meta.TipPrevHash)))
		}
	}
	if len(prevHash) == 0 && height > 0 {
		if parentBlock, ok := bk.chain.BlockByHeight(height - 1); ok && parentBlock != nil && len(parentBlock.Hash) > 0 {
			prevHash = append([]byte(nil), parentBlock.Hash...)
		}
	}

	tipBytes := decodeFlexibleHash([]byte(strings.TrimSpace(tipHash)))

	return &core.Block{
		Height: height,
		Header: core.BlockHeader{
			TimestampUnix: time.Now().Unix(),
			PrevHash:      prevHash,
		},
		Hash:      tipBytes,
		TotalWork: workBig.String(),
	}
}

// decodeFlexibleHash accepts raw bytes or lowercase hex (with optional 0x prefix).
func decodeFlexibleHash(s []byte) []byte {
	if len(s) == 0 {
		return nil
	}
	str := strings.TrimSpace(string(s))
	str = strings.TrimPrefix(str, "0x")
	if str == "" {
		return nil
	}
	if raw, err := hex.DecodeString(str); err == nil && len(raw) > 0 {
		return raw
	}
	out := make([]byte, len(s))
	copy(out, s)
	return out
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
			if bk.syncIdleConfirmed {
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
				bk.syncIdleConfirmed = true
				bk.syncCooldown = 0
				log.Printf("[BlockKeeper] Sync idle confirmed after %d no-op rounds — mining may now proceed", consecutiveSyncNoneThreshold)
			}
			bk.syncActiveMu.Unlock()

		case <-bk.forkResolvedCh:
			log.Printf("[BlockKeeper] Fork resolved, triggering immediate re-sync")
			bk.syncActiveMu.Lock()
			bk.syncActive = true
			bk.syncIdleConfirmed = false
			bk.consecutiveSyncNoneCount = 0
			bk.syncCooldown = 0
			bk.syncActiveMu.Unlock()

			synced := bk.startSync()

			bk.syncActiveMu.Lock()
			bk.syncActive = false
			bk.lastSyncAttempt = time.Now()
			if !synced {
				bk.consecutiveSyncNoneCount = consecutiveSyncNoneThreshold
				bk.syncIdleConfirmed = true
				log.Printf("[BlockKeeper] Fork resolved, no sync needed — mining may resume immediately")
			}
			bk.syncActiveMu.Unlock()

		case <-bk.quit:
			log.Printf("[BlockKeeper] syncWorker shutting down")
			return
		}
	}
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
	bk.syncIdleConfirmed = false
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
		if localHeight > 0 {
			bk.consecutiveSyncNoneCount++
			if bk.consecutiveSyncNoneCount < consecutiveSyncNoneThreshold {
				bk.syncLaggingPeers(localHeight)
			} else if bk.consecutiveSyncNoneCount == consecutiveSyncNoneThreshold {
				log.Printf("[BlockKeeper] %d consecutive syncTypeNone rounds — entering mining-friendly cooldown, skipping syncLaggingPeers",
					bk.consecutiveSyncNoneCount)
			}
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

// cleanupExpiredEntries periodically removes expired entries from forkDetectionCooldown
// and failedSyncPeers maps to prevent unbounded memory growth.
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
			// Clean up expired fork detection cooldown entries
			for key, expiry := range bk.forkDetectionCooldown {
				if now.After(expiry) {
					delete(bk.forkDetectionCooldown, key)
				}
			}
			// Clean up expired failed sync peer entries
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
}

// PeerDisconnectNotifier defines the interface for receiving peer disconnection notifications.
// Switch calls OnPeerDisconnected when a peer is removed so that components like
// blockKeeper can clear stale references to the disconnected peer.
type PeerDisconnectNotifier interface {
	OnPeerDisconnected(peerID string)
}
