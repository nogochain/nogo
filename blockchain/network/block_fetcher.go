package network

import (
	"encoding/hex"
	"fmt"
	"log"
	"sync"

	"gopkg.in/karalabe/cookiejar.v2/collections/prque"

	"github.com/nogochain/nogo/blockchain/core"
)

const (
	maxBlockDistance = 64
	maxMsgSetSize    = 128
	newBlockChSize   = 64
)

type BlockFetcherChainInterface interface {
	ProcessBlock(block *core.Block) (bool, error)
	BestBlockHeight() uint64
}

type BlockFetcherPeerSetInterface interface {
	broadcastMinedBlock(block *core.Block) error
	ProcessIllegal(peerID string, level uint32, reason string)
	getPeer(peerID string) *Peer
}

type blockFetcher struct {
	chain      BlockFetcherChainInterface
	peers      BlockFetcherPeerSetInterface
	newBlockCh chan *blockMsg
	queue      *prque.Prque
	msgSet     map[string]*blockMsg
	mu         sync.Mutex

	// onForkBlock is an optional callback triggered when a mined block
	// is stored as a fork (accepted=false, err=nil). This enables the
	// caller to initiate real-time fork resolution and reorg.
	onForkBlock func(peerID string, block *core.Block)
}

// SetOnForkBlock registers a callback for fork blocks detected during
// mined block processing. Used by SyncLoop to trigger TriggerForkReorg.
func (f *blockFetcher) SetOnForkBlock(cb func(peerID string, block *core.Block)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onForkBlock = cb
}

// newBlockFetcher creates a blockFetcher for P2P block processing.
// Blocks are directly validated and added to the chain.
func newBlockFetcher(chain BlockFetcherChainInterface, peers BlockFetcherPeerSetInterface) *blockFetcher {
	f := &blockFetcher{
		chain:      chain,
		peers:      peers,
		newBlockCh: make(chan *blockMsg, 1<<8),
		queue:      prque.New(),
		msgSet:     make(map[string]*blockMsg),
	}
	go f.blockProcessor()
	return f
}

func (f *blockFetcher) blockProcessor() {
	for {
		for !f.queue.Empty() {
			msg := f.queue.PopItem().(*blockMsg)
			if msg.block.GetHeight() > f.chain.BestBlockHeight()+1 {
				f.queue.Push(msg, -float32(msg.block.GetHeight()))
				break
			}
			f.insert(msg)
			blockHashHex := hex.EncodeToString(msg.block.Hash)
			f.mu.Lock()
			delete(f.msgSet, blockHashHex)
			f.mu.Unlock()
		}
		msg := <-f.newBlockCh
		f.add(msg)
	}
}

func (f *blockFetcher) add(msg *blockMsg) {
	if msg == nil || msg.block == nil {
		return
	}

	bestHeight := f.chain.BestBlockHeight()
	blockHeight := msg.block.GetHeight()

	f.mu.Lock()
	currentSetSize := len(f.msgSet)
	f.mu.Unlock()

	// CRITICAL FIX: Do NOT drop blocks based on maxBlockDistance.
	// Previously, if blockHeight-bestHeight > maxBlockDistance (64), the
	// block was silently dropped. This prevented deep fork detection when
	// a mining node on a fork chain produced blocks 65+ ahead of the
	// canonical chain. The dropped blocks never reached the chain's orphan
	// pool, so requestMissingParentAsync was never triggered, and the fork
	// became permanently invisible.
	//
	// Now we only enforce maxMsgSetSize (rate limiting). The chain's own
	// orphan/fork handling (addOrphanBlockLocked → requestMissingParentAsync
	// → EnsureAncestors → shouldReorgToHeaviestLocked) correctly manages
	// deep fork detection and reorganization.
	if currentSetSize > maxMsgSetSize || bestHeight > blockHeight {
		return
	}

	blockHashHex := hex.EncodeToString(msg.block.Hash)

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.msgSet[blockHashHex]; !exists {
		f.msgSet[blockHashHex] = msg
		f.queue.Push(msg, -float32(blockHeight))
		log.Printf("[BlockFetcher] Queued mine block height=%d hash=%s peer=%s",
			blockHeight, blockHashHex[:16], msg.peerID)
	}
}

// insert processes a received mined block from P2P broadcast.
// Validates the block via ProcessBlock and adds it to the chain.
// Reports invalid blocks to the peer scoring system.
func (f *blockFetcher) insert(msg *blockMsg) {
	if msg == nil || msg.block == nil {
		return
	}

	accepted, err := f.chain.ProcessBlock(msg.block)
	if err != nil {
		log.Printf("[BlockFetcher] ProcessBlock failed height=%d hash=%s peer=%s: %v",
			msg.block.GetHeight(), hex.EncodeToString(msg.block.Hash)[:16], msg.peerID, err)
		f.peers.ProcessIllegal(msg.peerID, 1, fmt.Sprintf("ProcessBlock invalid: %v", err))
		return
	}

	if accepted {
		log.Printf("[BlockFetcher] Block accepted height=%d hash=%s peer=%s",
			msg.block.GetHeight(), hex.EncodeToString(msg.block.Hash)[:16], msg.peerID)
		return
	}

	// Block was stored as fork (not rejected). The canonical chain
	// may still be on the other fork. Trigger fork resolution to
	// compare cumulative work and reorg if peer chain is heavier.
	log.Printf("[BlockFetcher] Block stored as fork height=%d hash=%s peer=%s, triggering fork reorg check",
		msg.block.GetHeight(), hex.EncodeToString(msg.block.Hash)[:16], msg.peerID)
	f.mu.Lock()
	cb := f.onForkBlock
	f.mu.Unlock()
	if cb != nil {
		cb(msg.peerID, msg.block)
	}
}

func (f *blockFetcher) processNewBlock(msg *blockMsg) {
	if msg == nil {
		return
	}

	select {
	case f.newBlockCh <- msg:
	default:
		log.Printf("[BlockFetcher] WARNING: channel full, dropping block height=%d",
			msg.block.GetHeight())
	}
}
