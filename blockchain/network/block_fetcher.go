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
}

func newBlockFetcher(chain BlockFetcherChainInterface, peers BlockFetcherPeerSetInterface) *blockFetcher {
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

func (f *blockFetcher) insert(msg *blockMsg) {
	if msg == nil || msg.block == nil {
		return
	}

	isOrphan, err := f.chain.ProcessBlock(msg.block)
	if err != nil {
		log.Printf("[BlockFetcher] ProcessBlock failed height=%d hash=%s err=%v peer=%s",
			msg.block.GetHeight(), hex.EncodeToString(msg.block.Hash)[:16], err, msg.peerID)
		if f.peers != nil {
			f.peers.ProcessIllegal(msg.peerID, 30, fmt.Sprintf("invalid block: %v", err))
		}
		return
	}

	if isOrphan {
		log.Printf("[BlockFetcher] Block is orphan height=%d hash=%s",
			msg.block.GetHeight(), hex.EncodeToString(msg.block.Hash)[:16])
		return
	}

	if f.peers != nil {
		if err := f.peers.broadcastMinedBlock(msg.block); err != nil {
			log.Printf("[BlockFetcher] Broadcast failed height=%d err=%v",
				msg.block.GetHeight(), err)
			return
		}
		log.Printf("[BlockFetcher] Broadcast success height=%d hash=%s",
			msg.block.GetHeight(), hex.EncodeToString(msg.block.Hash)[:16])
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
