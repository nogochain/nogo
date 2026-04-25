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
	"github.com/nogochain/nogo/blockchain/network/reactor"
)

const (
	syncCycle            = 5 * time.Second
	blockProcessChSize   = 1024
	blocksProcessChSize  = 128
	headersProcessChSize = 1024
)

var (
	maxBlockPerMsg        = uint64(128)
	maxBlockHeadersPerMsg = uint64(2048)
	syncTimeout           = 30 * time.Second

	errAppendHeaders  = errors.New("fail to append list due to order dismatch")
	errRequestTimeout = errors.New("request timeout")
	errPeerDropped    = errors.New("Peer dropped")
	errPeerMisbehave  = errors.New("peer is misbehave")
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
	chain            ChainInterface
	peers            PeerSetInterface
	syncPeer         PeerInterface

	blockProcessCh   chan *blockMsg
	blocksProcessCh  chan *blocksMsg
	headersProcessCh chan *headersMsg

	headerList       *list.List
	forkResolvedCh   chan struct{}
	quit             chan struct{}
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

func (bk *blockKeeper) appendHeaderList(headers []*HeaderLocator) error {
	for _, header := range headers {
		if bk.headerList.Len() == 0 {
			return fmt.Errorf("%w: header list is empty", errAppendHeaders)
		}
		prevHeader := bk.headerList.Back().Value.(*HeaderLocator)
		if !equalBytes(prevHeader.Header.PrevHash, header.Header.PrevHash) && bk.headerList.Len() > 1 {
			prevElement := bk.headerList.Back().Prev()
			if prevElement != nil {
				prevPrevHeader := prevElement.Value.(*HeaderLocator)
				if !equalBytes(prevPrevHeader.Header.PrevHash, header.Header.PrevHash) {
					return errAppendHeaders
				}
			}
		}
		bk.headerList.PushBack(header)
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
	for i <= targetHeight {
		block, err := bk.requireBlock(i)
		if err != nil {
			return fmt.Errorf("regularBlockSync requireBlock at height %d: %w", i, err)
		}

		isOrphan, err := bk.chain.AddBlock(block)
		if err != nil {
			return fmt.Errorf("regularBlockSync add block at height %d: %w", block.GetHeight(), err)
		}

		if isOrphan {
			if i > 1 {
				i--
			}
			continue
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
	checkpoint := bk.nextCheckpoint()
	peer := bk.peers.bestPeer(SFFullNode)

	if peer != nil && checkpoint != nil && peer.Height() >= checkpoint.Height {
		bk.syncPeer = peer
		log.Printf("[BlockKeeper] startSync: Selected peer=%s for FAST sync to checkpoint height=%d (peerHeight=%d)",
			peer.ID(), checkpoint.Height, peer.Height())
		log.Printf("[BlockKeeper] Peer locked for entire sync duration - will not switch peers")

		if err := bk.fastBlockSync(checkpoint); err != nil {
			log.Printf("[BlockKeeper] fastBlockSync failed: %v", err)
			bk.peers.ProcessIllegal(peer.ID(), LevelMsgIllegal, err.Error())
			log.Printf("[BlockKeeper] Peer %s penalized for fastBlockSync failure, releasing lock", peer.ID())
			return false
		}
		log.Printf("[BlockKeeper] Fast sync completed successfully with peer=%s", peer.ID())
		return true
	}

	blockHeight := bk.chain.LatestBlock().GetHeight()
	peer = bk.peers.bestPeer(SFFullNode)

	if peer == nil {
		log.Printf("[BlockKeeper] startSync: no peer available (localHeight=%d)", blockHeight)
		return false
	}

	peerHeight, peerWork, peerTipHash, chainInfoErr := bk.peers.GetPeerChainInfo(peer.ID())
	if chainInfoErr != nil {
		log.Printf("[BlockKeeper] startSync: GetPeerChainInfo for %s failed: %v (will use cached height=%d)",
			peer.ID(), chainInfoErr, peer.Height())
	}

	if peer.Height() > blockHeight {
		bk.syncPeer = peer
		targetHeight := blockHeight + maxBlockPerMsg
		if targetHeight > peer.Height() {
			targetHeight = peer.Height()
		}

		log.Printf("[BlockKeeper] startSync: Selected peer=%s for REGULAR sync from height=%d to target=%d (peerHeight=%d)",
			peer.ID(), blockHeight, targetHeight, peer.Height())
		log.Printf("[BlockKeeper] Peer locked for entire sync duration - will not switch peers")

		if err := bk.regularBlockSync(targetHeight); err != nil {
			log.Printf("[BlockKeeper] regularBlockSync failed: %v", err)
			bk.peers.ProcessIllegal(peer.ID(), LevelMsgIllegal, err.Error())
			log.Printf("[BlockKeeper] Peer %s penalized for regularBlockSync failure, releasing lock", peer.ID())
			return false
		}
		log.Printf("[BlockKeeper] Regular sync completed successfully with peer=%s", peer.ID())
		return true
	}

	if peer.Height() <= blockHeight {
		log.Printf("[BlockKeeper] startSync: peer %s height(%d) <= localHeight(%d), checking for fork...",
			peer.ID(), peer.Height(), blockHeight)
		if bk.detectAndHandleForkWithInfo(peer, peerHeight, peerWork, peerTipHash, chainInfoErr) {
			return true
		}
	}

	log.Printf("[BlockKeeper] startSync: No sync needed (localHeight=%d, peer=%s, peerHeight=%d)",
		blockHeight, peer.ID(), peer.Height())
	return false
}

func (bk *blockKeeper) detectAndHandleFork(peer PeerInterface) bool {
	localBlock := bk.chain.LatestBlock()
	if localBlock == nil {
		log.Printf("[BlockKeeper] ForkDetection: skipped - local block is nil")
		return false
	}

	peerHeight, peerWork, peerTipHash, err := bk.peers.GetPeerChainInfo(peer.ID())
	if err != nil {
		log.Printf("[BlockKeeper] ForkDetection: skipped - GetPeerChainInfo failed for %s: %v", peer.ID(), err)
		return false
	}

	localTipHash := hex.EncodeToString(localBlock.Hash)

	if localTipHash == peerTipHash {
		log.Printf("[BlockKeeper] ForkDetection: same chain (tip=%s), no fork", localTipHash[:16])
		return false
	}

	log.Printf("[BlockKeeper] ForkDetection: local tip=%s (height=%d) vs peer %s tip=%s (height=%d, work=%s)",
		localTipHash[:16], localBlock.GetHeight(), peer.ID(), peerTipHash[:16], peerHeight, peerWork.String())

	localWork := big.NewInt(0)
	if cp, ok := bk.chain.(interface{ CanonicalWork() *big.Int }); ok {
		localWork = cp.CanonicalWork()
	}

	workCmp := peerWork.Cmp(localWork)

	if workCmp > 0 {
		log.Printf("[BlockKeeper] ForkDetection: PEER HAS HEAVIER CHAIN! localWork=%s < peerWork=%s, triggering reorg",
			localWork.String(), peerWork.String())

		bk.rollbackForReorg(peerHeight, localBlock.GetHeight(), peer.ID())
		return true
	}

	if workCmp == 0 {
		log.Printf("[BlockKeeper] ForkDetection: WORK TIE (%s), comparing tip hashes as tiebreaker", localWork.String())

		if strings.Compare(peerTipHash, localTipHash) < 0 {
			log.Printf("[BlockKeeper] ForkDetection: PEER WINS TIEBREAKER! peer tip=%s < local tip=%s, triggering reorg",
				peerTipHash[:16], localTipHash[:16])

			bk.rollbackForReorg(peerHeight, localBlock.GetHeight(), peer.ID())
			return true
		}

		log.Printf("[BlockKeeper] ForkDetection: LOCAL WINS TIEBREAKER or hash equal, no reorg needed")
		return false
	}

	log.Printf("[BlockKeeper] ForkDetection: tips differ but local chain is heavier, no reorg needed")
	return false
}

func (bk *blockKeeper) detectAndHandleForkWithInfo(peer PeerInterface, peerHeight uint64, peerWork *big.Int, peerTipHash string, chainInfoErr error) bool {
	localBlock := bk.chain.LatestBlock()
	if localBlock == nil {
		log.Printf("[BlockKeeper] ForkDetection: skipped - local block is nil")
		return false
	}

	if chainInfoErr != nil {
		log.Printf("[BlockKeeper] ForkDetection: using cached height=%d (ChainInfo query failed: %v)", peer.Height(), chainInfoErr)
		peerHeight = peer.Height()
		peerWork = big.NewInt(0)
		peerTipHash = ""
	}

	localTipHash := hex.EncodeToString(localBlock.Hash)

	if peerTipHash != "" && localTipHash == peerTipHash {
		log.Printf("[BlockKeeper] ForkDetection: same chain (tip=%s), no fork", localTipHash[:16])
		return false
	}

	if peerTipHash == "" {
		log.Printf("[BlockKeeper] ForkDetection: no tip hash available (query failed), checking if heights differ")
		if peerHeight == localBlock.GetHeight() {
			log.Printf("[BlockKeeper] ForkDetection: same height (%d) but unknown tips, assuming synced", peerHeight)
			return false
		}
		log.Printf("[BlockKeeper] ForkDetection: height mismatch (local=%d, peer=%d) without tip hash, cannot determine fork direction", localBlock.GetHeight(), peerHeight)
		return false
	}

	log.Printf("[BlockKeeper] ForkDetection: local tip=%s (height=%d) vs peer %s tip=%s (height=%d, work=%s)",
		localTipHash[:16], localBlock.GetHeight(), peer.ID(), peerTipHash[:16], peerHeight, peerWork.String())

	localWork := big.NewInt(0)
	if cp, ok := bk.chain.(interface{ CanonicalWork() *big.Int }); ok {
		localWork = cp.CanonicalWork()
	}

	workCmp := peerWork.Cmp(localWork)

	if workCmp > 0 {
		log.Printf("[BlockKeeper] ForkDetection: PEER HAS HEAVIER CHAIN! localWork=%s < peerWork=%s, triggering reorg",
			localWork.String(), peerWork.String())

		bk.rollbackForReorg(peerHeight, localBlock.GetHeight(), peer.ID())
		return true
	}

	if workCmp == 0 {
		log.Printf("[BlockKeeper] ForkDetection: WORK TIE (%s), comparing tip hashes as tiebreaker", localWork.String())

		if strings.Compare(peerTipHash, localTipHash) < 0 {
			log.Printf("[BlockKeeper] ForkDetection: PEER WINS TIEBREAKER! peer tip=%s < local tip=%s, triggering reorg",
				peerTipHash[:16], localTipHash[:16])

			bk.rollbackForReorg(peerHeight, localBlock.GetHeight(), peer.ID())
			return true
		}

		log.Printf("[BlockKeeper] ForkDetection: LOCAL WINS TIEBREAKER or hash equal, no reorg needed")
		return false
	}

	log.Printf("[BlockKeeper] ForkDetection: tips differ but local chain is heavier, no reorg needed")
	return false
}

func (bk *blockKeeper) rollbackForReorg(peerHeight, localHeight uint64, peerID string) bool {
	rollbackHeight := uint64(0)
	if peerHeight > 0 {
		rollbackHeight = peerHeight
	}
	if rollbackHeight >= localHeight {
		rollbackHeight = localHeight
		if rollbackHeight > 0 {
			rollbackHeight--
		}
	}

	if chainProvider, ok := bk.chain.(interface {
		RollbackToHeight(height uint64) error
	}); ok {
		if err := chainProvider.RollbackToHeight(rollbackHeight); err != nil {
			log.Printf("[BlockKeeper] ForkDetection: RollbackToHeight(%d) failed: %v", rollbackHeight, err)
			return false
		}
		log.Printf("[BlockKeeper] ForkDetection: Rolled back to height=%d, will re-sync from peer %s on next cycle", rollbackHeight, peerID)
		return true
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
