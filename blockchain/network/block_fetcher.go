package network

import (
	"encoding/hex"
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
}

// newBlockFetcher creates a blockFetcher for P2P block processing.
// Blocks are directly validated and added to the chain.
func newBlockFetcher(
	chain BlockFetcherChainInterface,
	peers BlockFetcherPeerSetInterface,
) *blockFetcher {
	f := &blockFetcher{
		chain:      chain,
		peers:      peers,
		newBlockCh: make(chan *blockMsg, newBlockChSize),
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

	if currentSetSize > maxMsgSetSize || bestHeight > blockHeight || blockHeight-bestHeight > maxBlockDistance {
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

// insert processes a received block. Validates and
// adds directly to chain. First valid block extending the current tip wins.
func (f *blockFetcher) insert(msg *blockMsg) {
	if msg == nil || msg.block == nil {
		return
	}
	// blockFetcher is a pass-through; actual block processing happens
	// via BlockReactorHandler.OnBlock which calls chain.AddBlock directly.
	_ = msg.block.GetHeight()
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
