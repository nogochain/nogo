package network

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// BlockSyncManager manages block synchronization using Bitcoin-style async GETDATA
// Fixes the protocol conflict by using channel-based synchronization instead of polling
type BlockSyncManager struct {
	server     *P2PServer
	syncMu     sync.RWMutex
	// Map of block hash -> channel to notify waiting goroutines
	waitingForBlock map[string]chan *core.Block
	// Map of block hash -> context for timeout control
	blockCtxs map[string]context.CancelFunc
}

// NewBlockSyncManager creates a new block sync manager
func NewBlockSyncManager(server *P2PServer) *BlockSyncManager {
	return &BlockSyncManager{
		server:         server,
		waitingForBlock: make(map[string]chan *core.Block),
		blockCtxs:     make(map[string]context.CancelFunc),
	}
}

// RequestBlockAsync requests a block via GETDATA (Bitcoin-style)
// Returns a channel that will receive the block when it arrives via BLOCK message
func (bsm *BlockSyncManager) RequestBlockAsync(ctx context.Context, c net.Conn, peerAddr string, blockHashHex string, timeout time.Duration) (<-chan *core.Block, error) {
	// Create response channel
	respCh := make(chan *core.Block, 1)
	blockCtx, cancel := context.WithTimeout(ctx, timeout)

	bsm.syncMu.Lock()
	
	// Check if already waiting for this block (deduplication)
	if existingCh, exists := bsm.waitingForBlock[blockHashHex]; exists {
		// Already waiting for this block, return existing channel
		bsm.syncMu.Unlock()
		log.Printf("p2p: already waiting for block %s, reusing channel", blockHashHex[:16])
		
		// Start a goroutine to wait on the existing channel
		go func() {
			select {
			case block := <-existingCh:
				select {
				case respCh <- block:
				case <-blockCtx.Done():
				}
			case <-blockCtx.Done():
			}
		}()
		
		return respCh, nil
	}
	
	// Register this block as being waited for
	bsm.waitingForBlock[blockHashHex] = respCh
	bsm.blockCtxs[blockHashHex] = cancel
	bsm.syncMu.Unlock()

	// Send GETDATA request
	inv := InventoryEntry{
		Type: InvTypeBlock,
		Hash: blockHashHex,
	}

	msg := p2pEnvelope{
		Type:    "getdata",
		Payload: mustJSON(p2pGetDataMsg{Entries: []InventoryEntry{inv}}),
	}

	if err := p2pWriteJSON(c, msg); err != nil {
		bsm.syncMu.Lock()
		delete(bsm.waitingForBlock, blockHashHex)
		delete(bsm.blockCtxs, blockHashHex)
		bsm.syncMu.Unlock()
		cancel()
		return nil, fmt.Errorf("failed to send GETDATA: %w", err)
	}

	log.Printf("p2p: sent GETDATA for block %s to %s", blockHashHex[:16], peerAddr)
	
	// Start a goroutine to handle cleanup on timeout
	go func() {
		<-blockCtx.Done()
		bsm.syncMu.Lock()
		if _, exists := bsm.waitingForBlock[blockHashHex]; exists {
			close(respCh)
			delete(bsm.waitingForBlock, blockHashHex)
			delete(bsm.blockCtxs, blockHashHex)
		}
		bsm.syncMu.Unlock()
	}()

	return respCh, nil
}

// NotifyBlockReceived is called when a BLOCK message arrives
// Delivers the block to all waiting channels (content-based matching)
func (bsm *BlockSyncManager) NotifyBlockReceived(block *core.Block) {
	blockHashHex := hex.EncodeToString(block.Hash)
	
	bsm.syncMu.Lock()
	defer bsm.syncMu.Unlock()

	// Get channel waiting for this block
	respCh, exists := bsm.waitingForBlock[blockHashHex]
	if !exists {
		// No one is waiting for this block
		return
	}

	log.Printf("p2p: notifying waiting goroutine for block %s",
		blockHashHex[:16])

	// Send block to waiting channel (non-blocking)
	select {
	case respCh <- block:
		// Successfully delivered
		log.Printf("p2p: successfully delivered block %s to waiting goroutine", blockHashHex[:16])
	default:
		// Channel closed or full, log and continue
		log.Printf("p2p: failed to deliver block %s to waiting goroutine", blockHashHex[:16])
	}

	// Cleanup: remove waiting channel for this block
	delete(bsm.waitingForBlock, blockHashHex)

	// Cancel and remove block context
	if cancelFunc, exists := bsm.blockCtxs[blockHashHex]; exists {
		cancelFunc()
		delete(bsm.blockCtxs, blockHashHex)
	}
}

// Cancel removes a pending block request
// Should be called when the request times out or connection closes
func (bsm *BlockSyncManager) Cancel(blockHashHex string) {
	bsm.syncMu.Lock()
	defer bsm.syncMu.Unlock()
	
	if ch, exists := bsm.waitingForBlock[blockHashHex]; exists {
		close(ch)
		delete(bsm.waitingForBlock, blockHashHex)
	}
	
	if cancelFunc, exists := bsm.blockCtxs[blockHashHex]; exists {
		cancelFunc()
		delete(bsm.blockCtxs, blockHashHex)
	}
}

// Cleanup removes all pending requests for a peer
func (bsm *BlockSyncManager) Cleanup() {
	bsm.syncMu.Lock()
	defer bsm.syncMu.Unlock()
	
	log.Printf("p2p: cleaning up %d pending block requests", len(bsm.waitingForBlock))

	// Close all response channels
	for blockHashHex, ch := range bsm.waitingForBlock {
		close(ch)
		log.Printf("p2p: cleaned up pending request for block %s", blockHashHex[:16])
	}
	
	// Cancel all contexts
	for _, cancel := range bsm.blockCtxs {
		cancel()
	}
	
	// Clear maps
	bsm.waitingForBlock = make(map[string]chan *core.Block)
	bsm.blockCtxs = make(map[string]context.CancelFunc)
}

// GetStats returns statistics about pending block requests
func (bsm *BlockSyncManager) GetStats() map[string]interface{} {
	bsm.syncMu.RLock()
	defer bsm.syncMu.RUnlock()
	
	return map[string]interface{}{
		"pending_blocks": len(bsm.waitingForBlock),
	}
}
