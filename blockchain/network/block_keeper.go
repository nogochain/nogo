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
	syncCycle            = 1 * time.Second
	blockProcessChSize   = 1024
	blocksProcessChSize  = 128
	headersProcessChSize = 1024

	// NogoChain-style sync type classification for intelligent strategy selection
	syncTypeNone          = iota // no peer available or already synced
	syncTypeFast                 // fast sync via checkpoint skeleton download
	syncTypeRegular              // regular sequential block download
	syncTypeForkDetection        // equal height but peer has more cumulative work
)

var (
	maxBlockPerMsg        = uint64(512)
	maxBlockHeadersPerMsg = uint64(2048)
	syncTimeout           = 30 * time.Second
	fastSyncTimeout       = 15 * time.Second
	stuckEscapeThreshold  = 60 * time.Second

	errAppendHeaders  = errors.New("fail to append list due to order dismatch")
	errRequestTimeout = errors.New("request timeout")
	errPeerDropped    = errors.New("Peer dropped")
	errPeerMisbehave  = errors.New("peer is misbehave")
	errChainMismatch  = errors.New("chain mismatch detected")
)

type blockMsg struct {
	block  *core.Block
	peerID string
}

type blocksMsg struct {
	blocks []*core.Block
	peerID string
}

type headersMsg struct {
	headers []*HeaderLocator
	peerID  string
}

type blockKeeper struct {
	chain         ChainInterface
	peers         PeerSetInterface
	syncPeer      PeerInterface
	candidatePool *core.CandidatePool

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

	syncActive   bool
	syncActiveMu sync.Mutex

	lastSyncAttempt time.Time
	syncCooldown    time.Duration
}

func (bk *blockKeeper) isActive() bool {
	bk.syncActiveMu.Lock()
	defer bk.syncActiveMu.Unlock()
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
		syncBlockCh:              make(chan *blockMsg, 128),
		headerList:               list.New(),
		forkResolvedCh:           make(chan struct{}, 1),
		quit:                     make(chan struct{}),
		forkDetectionCooldown:    make(map[string]time.Time),
		forkDetectionCooldownDur: 1 * time.Second,
	}
	bk.resetHeaderState()
	go bk.syncWorker()
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

func (bk *blockKeeper) regularBlockSync(targetHeight uint64) error {
	i := bk.chain.LatestBlock().GetHeight() + 1
	startHeight := i

	log.Printf("[BlockKeeper] Starting sequential sync: h=%d → target=%d (gap=%d)",
		startHeight, targetHeight, targetHeight-startHeight+1)

	for i <= targetHeight {
		block, err := bk.requireBlock(i)
		if err != nil {
			return fmt.Errorf("regularBlockSync requireBlock at height %d: %w", i, err)
		}

		isOrphan, addErr := bk.chain.AddBlock(block)
		if addErr != nil {
			return fmt.Errorf("regularBlockSync add block at height %d: %w", block.GetHeight(), addErr)
		}

		newTip := bk.chain.LatestBlock().GetHeight()

		if newTip < i {
			// Block rejected: orphan (parent missing) or fork (prevHash mismatch).
			// Root cause: external miner created a conflicting block at height i-1,
			// making the peer's block ineligible for canonical addition.
			// Resolution: fetch peer's block at i-1, rollback conflicting block,
			// add parent then child to restore sync continuity.
			if block.GetHeight() > 1 {
				reqHeight := block.GetHeight() - 1
				log.Printf("[BlockKeeper] Sync block h=%d rejected (tip=%d, orphan=%v), fetching parent h=%d",
					block.GetHeight(), newTip, isOrphan, reqHeight)

				parentBlock, pErr := bk.requireBlock(reqHeight)
				if pErr != nil {
					return fmt.Errorf("regularBlockSync fetch h=%d for conflict resolution: %w", reqHeight, pErr)
				}

				if rbErr := bk.chain.RollbackToHeight(reqHeight - 1); rbErr != nil {
					return fmt.Errorf("regularBlockSync rollback to h=%d: %w", reqHeight-1, rbErr)
				}

				if _, paErr := bk.chain.AddBlock(parentBlock); paErr != nil {
					return fmt.Errorf("regularBlockSync add parent h=%d: %w", reqHeight, paErr)
				}

				if _, addErr = bk.chain.AddBlock(block); addErr != nil {
					return fmt.Errorf("regularBlockSync re-add h=%d: %w", block.GetHeight(), addErr)
				}

				newTip = bk.chain.LatestBlock().GetHeight()
			}
		}

		if newTip == i {
			bk.recordSyncProgress(newTip)
			if newTip%500 == 0 || newTip >= targetHeight {
				log.Printf("[BlockKeeper] Sync progress: %d/%d (%.1f%%)",
					newTip, targetHeight, float64(newTip-startHeight)/float64(targetHeight-startHeight)*100)
			}
		} else {
			return fmt.Errorf("regularBlockSync block at height %d permanently rejected, local tip=%d",
				block.GetHeight(), newTip)
		}

		i = bk.chain.LatestBlock().GetHeight() + 1
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
	timeout := time.NewTimer(time.Duration(count)*fastSyncTimeout + 5*time.Second)
	defer timeout.Stop()

	expected := int(count)

	for len(result) < expected && expected > 0 {
		select {
		case msg := <-bk.blockProcessCh:
			if msg.peerID != bk.syncPeer.ID() {
				continue
			}
			if msg.block == nil {
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

	var bestByWork PeerInterface
	var bestWork *big.Int
	var bestWorkHeight uint64

	// Scan ALL peers and find the one with most cumulative work.
	// This is the canonical chain leader — sync from THIS peer,
	// not the tallest-height peer on a weak fork.
	for _, peerID := range peerIDs {
		peerHeight, peerWork, _, err := bk.peers.GetPeerChainInfo(peerID)
		if err != nil || peerWork == nil {
			continue
		}

		if bestWork == nil || peerWork.Cmp(bestWork) > 0 {
			bestWork = peerWork
			bestWorkHeight = peerHeight
			bestByWork = bk.wrapPeer(peerID, peerHeight)
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

	log.Printf("[BlockKeeper] checkSyncType: local(H=%d,W=%s) vs bestPeer(H=%d,W=%s,ID=%s)",
		blockHeight, localWork.String(), peerHeight, peerWork.String(), bestByWork.ID())

	// Priority 1: fast sync when checkpoint is ahead and peer supports it.
	if checkpoint != nil && peerHeight >= checkpoint.Height {
		minGap := uint64(128)
		if checkpoint.Height >= blockHeight+minGap {
			return syncTypeFast, bestByWork
		}
	}

	// CRITICAL FIX: Work comparison must come BEFORE height comparison.
	// A peer with higher height but LESS work is on a minority fork.
	// We should NOT sync from it.
	if peerWork.Cmp(localWork) > 0 {
		// Peer has more cumulative work → it IS the canonical chain.
		// Sync to catch up.
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
				log.Printf("[BlockKeeper] Synced (localH=%d >= peerH=%d), no sync needed", blockHeight, peer.Height())
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

// dispatchRegularSync handles standard block synchronization from a peer
func (bk *blockKeeper) dispatchRegularSync(peer PeerInterface, localHeight uint64) bool {
	// Resolve real peer: checkSyncType may return a simplePeer wrapper.
	// Regular sync needs the actual peer with active MConnection.
	if _, isSimple := peer.(*simplePeer); isSimple {
		if realPeer := bk.peers.bestPeer(SFFullNode); realPeer != nil && realPeer.ID() == peer.ID() {
			peer = realPeer
		} else {
			// The work-optimal peer is different from bestPeer's height-based pick.
			// This means bestPeer chose a weak-fork peer (taller but less work).
			// We should sync from the work-optimal peer, but bestPeer can't find it.
			// Fallback: log and skip sync — our chain is actually winning.
			log.Printf("[BlockKeeper] dispatchRegularSync: work-optimal peer %s not found via bestPeer (bestPeer=%s), local chain may be canonical",
				peer.ID(), func() string {
					if rp := bk.peers.bestPeer(SFFullNode); rp != nil {
						return rp.ID()
					}
					return "none"
				}())
			return false
		}
	}

	bk.syncPeer = peer

	peerHeight := peer.Height()
	if peerHeight == 0 {
		h, _, _, err := bk.peers.GetPeerChainInfo(peer.ID())
		if err == nil && h > 0 {
			peerHeight = h
			log.Printf("[BlockKeeper] dispatchRegularSync: peer %s Height()=0, resolved via GetPeerChainInfo to %d", peer.ID(), h)
		} else {
			log.Printf("[BlockKeeper] dispatchRegularSync: cannot determine peer height for %s (err=%v), skipping sync round", peer.ID(), err)
			return false
		}
	}

	targetHeight := localHeight + maxBlockPerMsg
	if targetHeight > peerHeight {
		targetHeight = peerHeight
	}

	log.Printf("[BlockKeeper] REGULAR sync: h=%d → h=%d (gap=%d) via peer %s (peerH=%d)",
		localHeight, targetHeight, targetHeight-localHeight, peer.ID(), peerHeight)

	if err := bk.regularBlockSync(targetHeight); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, errChainMismatch.Error()) {
			log.Printf("[BlockKeeper] Chain mismatch detected (peer on different fork): %v", err)
			bk.handleChainMismatchInSync(peer)
			bk.peers.DecSyncLoad(peer.ID())
			return true
		}

		// CRITICAL FIX: Check if peer has more work before marking as illegal
		// When regularBlockSync fails (e.g., dust block loop from fork blocks),
		// the peer may simply be on a different fork with more cumulative work.
		// Without this check, we'd ban the peer and stall sync indefinitely.
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

		// Do NOT call ProcessIllegal for network-level errors (timeout, peer drop).
		// These are transient conditions, not peer misbehavior, and calling
		// ProcessIllegal would increment FailCount and eventually ban the peer.
		// With 10-ban threshold and sync timeout of 30s, the peer would be
		// banned after just 5 minutes of slow responses, permanently stalling sync.
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

	newHeight := bk.chain.LatestBlock().GetHeight()
	log.Printf("[BlockKeeper] Sync SUCCESS: h=%d → h=%d (+%d blocks)", localHeight, newHeight, newHeight-localHeight)
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
	peerID := peer.ID()

	cooldownKey := fmt.Sprintf("%s:%d", peerID, peerHeight)
	if lastCheck, exists := bk.forkDetectionCooldown[cooldownKey]; exists {
		if time.Since(lastCheck) < bk.forkDetectionCooldownDur {
			log.Printf("[BlockKeeper] dispatchForkDetection: cooling down peer=%s height=%d (last attempt=%v ago)",
				peerID, peerHeight, time.Since(lastCheck).Round(time.Millisecond))
			return false
		}
	}
	bk.forkDetectionCooldown[cooldownKey] = time.Now()

	log.Printf("[BlockKeeper] dispatchForkDetection: fork candidate (local=%d peer=%d), evaluating work",
		localHeight, peerHeight)

	peerTip := bk.getPeerTipBlock(peer)
	if peerTip == nil {
		log.Printf("[BlockKeeper] dispatchForkDetection: cannot build peer tip block")
		delete(bk.forkDetectionCooldown, cooldownKey)
		return false
	}

	if !bk.forkResolver.ShouldReorg(peerTip) {
		log.Printf("[BlockKeeper] dispatchForkDetection: peer chain does not have more work, keeping local chain — will retry after cooldown")
		// Cooldown is set above, prevents hot loop but allows retry after cooldownDur
		return false
	}

	log.Printf("[BlockKeeper] dispatchForkDetection: peer chain has more work, rolling back and re-syncing")

	rollbackTarget := peerHeight
	if rollbackTarget > 0 {
		rollbackTarget = peerHeight - 1
	}

	if err := bk.forkResolver.RollbackToHeight(rollbackTarget); err != nil {
		log.Printf("[BlockKeeper] dispatchForkDetection: rollback to %d failed: %v", rollbackTarget, err)
		delete(bk.forkDetectionCooldown, cooldownKey)
		return false
	}

	delete(bk.forkDetectionCooldown, cooldownKey)

	log.Printf("[BlockKeeper] dispatchForkDetection: rolled back to height %d, triggering re-sync",
		rollbackTarget)

	bk.TriggerImmediateReSync()
	return true
}

// handleChainMismatchInSync processes chain mismatch events during sync.
// Uses work comparison via ForkResolver.ShouldReorg to determine whether
// the peer's chain is heavier before performing any rollback.
func (bk *blockKeeper) handleChainMismatchInSync(peer PeerInterface) {
	if bk.forkResolver == nil {
		return
	}

	// Anti-loop: prevent infinite rollback→sync→fail→handleChainMismatchInSync→rollback loop.
	// If a chain mismatch was handled too recently, skip until cooldown expires.
	cooldownKey := "chain_mismatch:" + peer.ID()
	if lastHandle, exists := bk.forkDetectionCooldown[cooldownKey]; exists {
		if time.Since(lastHandle) < 3*time.Second {
			log.Printf("[BlockKeeper] handleChainMismatchInSync: cooling down for peer %s (last attempt=%v ago), will retry later",
				peer.ID(), time.Since(lastHandle).Round(time.Millisecond))
			return
		}
	}
	bk.forkDetectionCooldown[cooldownKey] = time.Now()

	localTip := bk.chain.LatestBlock()
	if localTip == nil {
		return
	}

	// Build peer tip block for work comparison
	peerTip := bk.getPeerTipBlock(peer)
	if peerTip == nil {
		log.Printf("[BlockKeeper] handleChainMismatchInSync: cannot build peer tip block from %s", peer.ID())
		return
	}

	// Use work-weighted decision before any rollback
	if !bk.forkResolver.ShouldReorg(peerTip) {
		log.Printf("[BlockKeeper] handleChainMismatchInSync: peer chain does not have more work, keeping local chain")
		return
	}

	log.Printf("[BlockKeeper] Chain mismatch: peer=%s has more work, rolling back and re-syncing", peer.ID())

	if bk.multiNodeArbiter != nil {
		tipHash := hex.EncodeToString(localTip.Hash)
		bk.multiNodeArbiter.UpdatePeerState(
			peer.ID(),
			tipHash,
			localTip.GetHeight(),
			bk.chain.CanonicalWork(),
			8,
		)
	}

	// Rollback below the fork point and trigger re-sync.
	peerHeight := peer.Height()
	rollbackTarget := peerHeight
	if rollbackTarget > 0 {
		rollbackTarget = peerHeight - 1
	}
	if rollbackTarget > localTip.GetHeight() {
		rollbackTarget = localTip.GetHeight() - 1
	}

	if rbErr := bk.forkResolver.RollbackToHeight(rollbackTarget); rbErr != nil {
		log.Printf("[BlockKeeper] Chain mismatch rollback to %d failed: %v (will retry on next sync cycle)", rollbackTarget, rbErr)
		return
	}

	delete(bk.forkDetectionCooldown, "chain_mismatch:"+peer.ID())

	log.Printf("[BlockKeeper] Chain mismatch: rolled back to height %d, triggering re-sync", rollbackTarget)
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
// Populates TotalWork from the peer's chain info so that fork resolution
// (ShouldReorg / CalculateCumulativeWork) can accurately compare cumulative
// work without needing the full block from the peer chain.
// Previously TotalWork was not set, causing ShouldReorg to compute a near-zero
// work value from the empty DifficultyBits/PrevHash fields — permanently
// preventing fork switches.
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

	return &core.Block{
		Height: height,
		Header: core.BlockHeader{
			TimestampUnix: time.Now().Unix(),
		},
		Hash:      []byte(tipHash),
		TotalWork: workBig.String(),
	}
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
			bk.syncActive = true
			bk.syncActiveMu.Unlock()

			bk.safeStartSync(genesisBlock)

			bk.syncActiveMu.Lock()
			bk.syncActive = false
			bk.lastSyncAttempt = time.Now()
			bk.syncActiveMu.Unlock()

		case <-bk.forkResolvedCh:
			log.Printf("[BlockKeeper] Fork resolved, triggering immediate re-sync")
			bk.syncActiveMu.Lock()
			bk.syncActive = true
			bk.syncActiveMu.Unlock()

			bk.startSync()

			bk.syncActiveMu.Lock()
			bk.syncActive = false
			bk.lastSyncAttempt = time.Now()
			bk.syncActiveMu.Unlock()

		case <-bk.quit:
			log.Printf("[BlockKeeper] syncWorker shutting down")
			return
		}
	}
}

// safeStartSync wraps startSync/broadcastAfterSync/syncLaggingPeers with panic
// recovery to ensure syncWorker survives unexpected panics during sync.
func (bk *blockKeeper) safeStartSync(genesisBlock *core.Block) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BlockKeeper] PANIC in startSync/broadcast: %v - continuing sync loop", r)
		}
	}()
	if bk.startSync() {
		bk.broadcastAfterSync(genesisBlock)
	} else {
		localHeight := bk.chain.LatestBlock().GetHeight()
		if localHeight > 0 {
			bk.syncLaggingPeers(localHeight)
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
