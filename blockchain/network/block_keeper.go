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
)

var (
	maxBlockPerMsg        = uint64(128)
	maxBlockHeadersPerMsg = uint64(2048)
	syncTimeout           = 30 * time.Second
	fastSyncTimeout       = 5 * time.Second
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
	chain    ChainInterface
	peers    PeerSetInterface
	syncPeer PeerInterface

	blockProcessCh   chan *blockMsg
	blocksProcessCh  chan *blocksMsg
	headersProcessCh chan *headersMsg

	headerList     *list.List
	forkResolvedCh chan struct{}
	quit           chan struct{}

	lastSuccessfulSyncHeight uint64
	lastSuccessfulSyncTime   time.Time

	// UNIFIED FORK RESOLUTION: Uses core-main based architecture
	forkResolver     *forkresolution.ForkResolver
	multiNodeArbiter *forkresolution.MultiNodeArbitrator
}

func newBlockKeeper(chain ChainInterface, peers PeerSetInterface) *blockKeeper {
	bk := &blockKeeper{
		chain:            chain,
		peers:            peers,
		blockProcessCh:   make(chan *blockMsg, blockProcessChSize),
		blocksProcessCh:  make(chan *blocksMsg, blocksProcessChSize),
		headersProcessCh: make(chan *headersMsg, headersProcessChSize),
		headerList:       list.New(),
		forkResolvedCh:   make(chan struct{}, 1),
		quit:             make(chan struct{}),
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
	consecutiveOrphan := 0
	maxConsecutiveOrphanWalkback := 500

	for i <= targetHeight {
		block, reqErr := bk.requireBlockFast(i)
		if reqErr != nil {
			return fmt.Errorf("regularBlockSync requireBlock at height %d: %w", i, reqErr)
		}

		heightBeforeAdd := bk.chain.LatestBlock().GetHeight()
		isOrphan, addErr := bk.chain.AddBlock(block)
		if addErr != nil {
			return fmt.Errorf("regularBlockSync add block at height %d: %w", block.GetHeight(), addErr)
		}

		heightAfterAdd := bk.chain.LatestBlock().GetHeight()

		if heightAfterAdd > heightBeforeAdd {
			bk.recordSyncProgress(heightAfterAdd)
			consecutiveOrphan = 0
			i = heightAfterAdd + 1
			continue
		}

		if isOrphan {
			currentLocalHeight := bk.chain.LatestBlock().GetHeight()
			if currentLocalHeight >= i {
				i = currentLocalHeight + 1
				continue
			}

			consecutiveOrphan++
			if consecutiveOrphan > maxConsecutiveOrphanWalkback {
				log.Printf("[BlockKeeper] regularBlockSync: walked back %d heights from %d without finding connectable ancestor (local=%d), peer may be on unreachable fork",
					consecutiveOrphan, i-uint64(consecutiveOrphan)+1, currentLocalHeight)
				return fmt.Errorf("%w: cannot find common ancestor after walking back %d from height %d (local=%d)",
					errChainMismatch, consecutiveOrphan, i-uint64(consecutiveOrphan)+1, currentLocalHeight)
			}

			if i > 1 {
				i--
			}
			continue
		}

		currentLocalHeight := bk.chain.LatestBlock().GetHeight()
		if currentLocalHeight >= i {
			i = currentLocalHeight + 1
			continue
		}

		i++
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
		case msg := <-bk.blockProcessCh:
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
		case msg := <-bk.blockProcessCh:
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

func (bk *blockKeeper) startSync() bool {
	if bk.checkStuckEscape() {
		return true
	}

	blockHeight := bk.chain.LatestBlock().GetHeight()
	checkpoint := bk.nextCheckpoint()

	peer := bk.peers.bestPeer(SFFullNode)
	if peer == nil {
		log.Printf("[BlockKeeper] startSync: no peer available (localHeight=%d)", blockHeight)
		return false
	}

	if checkpoint != nil && peer.Height() >= checkpoint.Height {
		bk.syncPeer = peer
		log.Printf("[BlockKeeper] startSync: FAST sync to checkpoint h=%d via peer %s (peerH=%d)",
			checkpoint.Height, peer.ID(), peer.Height())

		if err := bk.fastBlockSync(checkpoint); err != nil {
			log.Printf("[BlockKeeper] fastBlockSync failed: %v", err)
			bk.peers.ProcessIllegal(peer.ID(), LevelMsgIllegal, err.Error())
			return false
		}
		log.Printf("[BlockKeeper] Fast sync completed (peer=%s)", peer.ID())
		bk.recordSyncProgress(bk.chain.LatestBlock().GetHeight())
		return true
	}

	if peer.Height() > blockHeight {
		bk.syncPeer = peer
		targetHeight := blockHeight + maxBlockPerMsg
		if targetHeight > peer.Height() {
			targetHeight = peer.Height()
		}

		log.Printf("[BlockKeeper] startSync: REGULAR sync from h=%d to h=%d via peer %s (peerH=%d)",
			blockHeight, targetHeight, peer.ID(), peer.Height())

		if err := bk.regularBlockSync(targetHeight); err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, errChainMismatch.Error()) {
				log.Printf("[BlockKeeper] Chain mismatch detected (peer on different fork): %v", err)

				// UNIFIED PREVENTIVE FORK HANDLING
				// All fork resolution goes through SINGLE entry point: ForkResolver.RequestReorgWithDepth()
				// It automatically classifies severity and applies appropriate timing:
				//   - Light (depth 1-3):   500ms interval → PREVENT accumulation!
				//   - Normal (depth 4-6):  2s interval   → Fast response
				//   - Emergency (depth 7+): 1s interval   → Urgent fallback
				if bk.forkResolver != nil {
					localTip := bk.chain.LatestBlock()
					if localTip != nil {
						forkEvent := bk.forkResolver.DetectFork(localTip, nil, peer.ID())
						if forkEvent != nil {
							log.Printf("[BlockKeeper] 🔄 Fork detected: type=%v depth=%d local_h=%d peer=%s",
								forkEvent.Type, forkEvent.Depth, forkEvent.LocalHeight, peer.ID())

							// Update multi-node arbitrator state (if available)
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

							// Create representative remote block for reorg decision
							remoteBlock := &core.Block{
								Height: peer.Height(),
								Header: core.BlockHeader{
									TimestampUnix: time.Now().Unix(),
								},
								TotalWork: fmt.Sprintf("%d", int64(peer.Height())*1000+500),
							}

							// SINGLE UNIFIED CALL - Let ForkResolver handle everything!
							// It will automatically:
							// 1. Classify severity based on depth
							// 2. Apply appropriate interval (500ms/2s/1s)
							// 3. Execute reorg if allowed
							reorgErr := bk.forkResolver.RequestReorgWithDepth(
								remoteBlock,
								fmt.Sprintf("blockkeeper-unified-%s", peer.ID()),
								forkEvent.Depth, // Pass depth for accurate classification
							)

							if reorgErr != nil {
								log.Printf("[BlockKeeper] Reorg attempt: %v (will retry on next sync cycle)", reorgErr)

								if strings.Contains(reorgErr.Error(), "too frequent") {
									log.Printf("[BlockKeeper] ℹ️ Rate-limited - this is normal preventive behavior")
								}
							} else {
								log.Printf("[BlockKeeper] ✅ Reorg SUCCESS! Fork at depth %d resolved PREVENTIVELY", forkEvent.Depth)
							}
						}
					}
				}

				// Always trigger resync as fallback mechanism
				bk.TriggerImmediateReSync()

				return true
			}
			log.Printf("[BlockKeeper] regularBlockSync failed: %v", err)
			bk.peers.ProcessIllegal(peer.ID(), LevelMsgIllegal, errMsg)
			return false
		}
		log.Printf("[BlockKeeper] Regular sync completed (peer=%s)", peer.ID())
		bk.recordSyncProgress(bk.chain.LatestBlock().GetHeight())
		return true
	}

	log.Printf("[BlockKeeper] startSync: synced (localH=%d >= peerH=%d, peer=%s)",
		blockHeight, peer.Height(), peer.ID())
	return false
}

func (bk *blockKeeper) recordSyncProgress(height uint64) {
	if height > bk.lastSuccessfulSyncHeight {
		bk.lastSuccessfulSyncHeight = height
		bk.lastSuccessfulSyncTime = time.Now()
	}
}

func (bk *blockKeeper) checkStuckEscape() bool {
	if bk.lastSuccessfulSyncTime.IsZero() {
		return false
	}

	currentHeight := bk.chain.LatestBlock().GetHeight()
	timeSinceProgress := time.Since(bk.lastSuccessfulSyncTime)

	if currentHeight <= bk.lastSuccessfulSyncHeight && timeSinceProgress > stuckEscapeThreshold {
		log.Printf("[BlockKeeper] STUCK WARNING: no progress for %v (stuck at h=%d, last progress at h=%d)",
			timeSinceProgress, currentHeight, bk.lastSuccessfulSyncHeight)
	}

	return false
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
			if bk.startSync() {
				bk.broadcastAfterSync(genesisBlock)
			} else {
				localHeight := bk.chain.LatestBlock().GetHeight()
				if localHeight > 0 {
					bk.syncLaggingPeers(localHeight)
				}
			}

		case <-bk.forkResolvedCh:
			log.Printf("[BlockKeeper] Fork resolved, triggering immediate re-sync")
			bk.startSync()

		case <-bk.quit:
			log.Printf("[BlockKeeper] syncWorker shutting down")
			return
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
}

type PeerInterface interface {
	ID() string
	Height() uint64
	getBlockByHeight(height uint64) bool
	getBlocks(locator [][]byte, stopHash []byte) bool
	getHeaders(locator [][]byte, stopHash []byte) bool
}
