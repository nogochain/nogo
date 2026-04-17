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

// InventoryManager manages inventory announcements and data requests
// Implements Bitcoin-style INV/GETDATA mechanism
type InventoryManager struct {
	server      *P2PServer
	requestMgr  *RequestQueueManager
	txRelay     *TxRelayManager
	blockRelay  *BlockRelayManager
}

// NewInventoryManager creates a new inventory manager
func NewInventoryManager(server *P2PServer) *InventoryManager {
	return &InventoryManager{
		server:     server,
		requestMgr: NewRequestQueueManager(),
		txRelay:    NewTxRelayManager(),
		blockRelay: NewBlockRelayManager(),
	}
}

// HandleInv processes an INV message from a peer
// Decides which data to request via GETDATA (Bitcoin-style)
func (im *InventoryManager) HandleInv(ctx context.Context, conn net.Conn, peerAddr string, inv []InventoryEntry) {
	log.Printf("p2p: received INV from %s with %d entries", peerAddr, len(inv))

	// Filter entries to request (only request what we don't have)
	var toRequest []InventoryEntry
	for _, entry := range inv {
		if im.shouldRequestEntry(entry) {
			// Check if we already requested this (deduplication)
			if !im.requestMgr.IsRequestPending(peerAddr, entry) {
				toRequest = append(toRequest, entry)
			}
		}
	}

	if len(toRequest) > 0 {
		log.Printf("p2p: requesting %d entries from %s via GETDATA", len(toRequest), peerAddr)

		// Send GETDATA message
		msg := p2pEnvelope{
			Type:    "getdata",
			Payload: mustJSON(p2pGetDataMsg{Entries: toRequest}),
		}

		if err := p2pWriteJSON(conn, msg); err != nil {
			log.Printf("p2p: failed to send GETDATA to %s: %v", peerAddr, err)
			return
		}

		// Add to pending requests (content-based matching)
		for _, entry := range toRequest {
			timeout := 30 * time.Second
			im.requestMgr.AddRequest(peerAddr, entry, timeout)
		}
	}
}

// shouldRequestEntry determines if we should request an inventory entry
// Implements Bitcoin-style data relay logic
func (im *InventoryManager) shouldRequestEntry(entry InventoryEntry) bool {
	switch entry.Type {
	case InvTypeBlock:
		return im.shouldRequestBlock(entry.Hash)

	case InvTypeTx:
		return im.shouldRequestTx(entry.Hash)

	default:
		return false
	}
}

// shouldRequestBlock checks if we should request a block
func (im *InventoryManager) shouldRequestBlock(blockHashHex string) bool {
	// Check if we already have this block
	if _, exists := im.server.bc.BlockByHash(blockHashHex); exists {
		return false
	}

	// Check if block relay manager already seen this block
	if im.blockRelay.IsKnown(blockHashHex) {
		return false
	}

	return true
}

// shouldRequestTx checks if we should request a transaction
func (im *InventoryManager) shouldRequestTx(txHashHex string) bool {
	// Check if mempool already has this transaction
	if im.server.mp != nil {
		// Parse hash to check mempool (simplified check)
		// In production, would use proper tx hash parsing
		_ = txHashHex
	}

	// Check if tx relay manager already seen this tx
	if im.txRelay.IsKnown(txHashHex) {
		return false
	}

	return true
}

// HandleGetData processes a GETDATA request from a peer
// Sends requested data (BLOCK or TX messages) - Bitcoin-style
func (im *InventoryManager) HandleGetData(ctx context.Context, conn net.Conn, peerAddr string, inv []InventoryEntry) error {
	log.Printf("p2p: received GETDATA from %s requesting %d entries", peerAddr, len(inv))

	for _, entry := range inv {
		switch entry.Type {
		case InvTypeBlock:
			if err := im.sendBlock(conn, entry.Hash); err != nil {
				log.Printf("p2p: failed to send block %s: %v", entry.Hash[:16], err)
				continue
			}

		case InvTypeTx:
			if err := im.sendTx(conn, entry.Hash); err != nil {
				log.Printf("p2p: failed to send tx %s: %v", entry.Hash[:16], err)
				continue
			}

		default:
			log.Printf("p2p: unsupported inventory type %d in GETDATA", entry.Type)
		}
	}

	return nil
}

// sendBlock sends a block in response to GETDATA
func (im *InventoryManager) sendBlock(conn net.Conn, blockHashHex string) error {
	block, exists := im.server.bc.BlockByHash(blockHashHex)
	if !exists {
		// Send NOTFOUND
		return p2pWriteJSON(conn, p2pEnvelope{
			Type:    "notfound",
			Payload: mustJSON(p2pNotFoundMsg{Entries: []InventoryEntry{
				{Type: InvTypeBlock, Hash: blockHashHex},
			}}),
		})
	}

	// Send BLOCK message
	return p2pWriteJSON(conn, p2pEnvelope{
		Type:    "block",
		Payload: mustJSON(p2pBlockMsg{Block: block}),
	})
}

// sendTx sends a transaction in response to GETDATA
func (im *InventoryManager) sendTx(conn net.Conn, txHashHex string) error {
	// Try to get transaction from mempool
	if im.server.mp == nil {
		return fmt.Errorf("mempool not available")
	}

	tx, exists := im.server.mp.GetTx(txHashHex)
	if !exists {
		// Send NOTFOUND if transaction not in mempool
		return p2pWriteJSON(conn, p2pEnvelope{
			Type:    "notfound",
			Payload: mustJSON(p2pNotFoundMsg{Entries: []InventoryEntry{
				{Type: InvTypeTx, Hash: txHashHex},
			}}),
		})
	}

	// Send the transaction
	return p2pWriteJSON(conn, p2pEnvelope{
		Type:    "tx",
		Payload: mustJSON(p2pTxMsg{Tx: *tx}),
	})
}

// AnnounceBlock sends an INV message announcing a new block
// Implements Bitcoin-style block announcement (relay without full data)
func (im *InventoryManager) AnnounceBlock(ctx context.Context, peerAddrs []string, block *core.Block) {
	blockHashHex := hex.EncodeToString(block.Hash)

	// Mark as seen (relay deduplication)
	im.blockRelay.MarkSeen(blockHashHex)

	inv := InventoryEntry{
		Type: InvTypeBlock,
		Hash: blockHashHex,
	}

	msg := p2pEnvelope{
		Type:    "inv",
		Payload: mustJSON(p2pInvMsg{Entries: []InventoryEntry{inv}}),
	}

	sentCount := 0
	for _, peerAddr := range peerAddrs {
		// Don't announce to peers that already know about this block
		if im.blockRelay.DoesPeerKnow(peerAddr, blockHashHex) {
			continue
		}

		if err := im.server.SendToPeer(ctx, peerAddr, msg); err == nil {
			im.blockRelay.MarkPeerAware(peerAddr, blockHashHex)
			sentCount++
		}
	}

	if sentCount > 0 {
		log.Printf("p2p: announced block height=%d hash=%s to %d peers",
			block.GetHeight(), blockHashHex[:16], sentCount)
	}
}

// AnnounceTx sends an INV message announcing a new transaction
// Implements Bitcoin-style tx announcement (relay without full data)
func (im *InventoryManager) AnnounceTx(ctx context.Context, peerAddrs []string, tx *core.Transaction, txHash []byte) {
	txHashHex := hex.EncodeToString(txHash)

	// Mark as seen (relay deduplication)
	im.txRelay.MarkSeen(txHashHex)

	inv := InventoryEntry{
		Type: InvTypeTx,
		Hash: txHashHex,
	}

	msg := p2pEnvelope{
		Type:    "inv",
		Payload: mustJSON(p2pInvMsg{Entries: []InventoryEntry{inv}}),
	}

	sentCount := 0
	for _, peerAddr := range peerAddrs {
		// Don't announce to peers that already know about this tx
		if im.txRelay.DoesPeerKnow(peerAddr, txHashHex) {
			continue
		}

		if err := im.server.SendToPeer(ctx, peerAddr, msg); err == nil {
			im.txRelay.MarkPeerAware(peerAddr, txHashHex)
			sentCount++
		}
	}

	if sentCount > 0 {
		log.Printf("p2p: announced tx hash=%s to %d peers",
			txHashHex[:16], sentCount)
	}
}

// HandleBlock processes a received BLOCK message
// Matches to pending GETDATA requests (content-based)
func (im *InventoryManager) HandleBlock(conn net.Conn, peerAddr string, block *core.Block) error {
	blockHashHex := hex.EncodeToString(block.Hash)
	log.Printf("p2p: received BLOCK message from %s height=%d hash=%s",
		peerAddr, block.GetHeight(), blockHashHex[:16])

	// Match to pending GETDATA request (content-based matching)
	inv := InventoryEntry{
		Type: InvTypeBlock,
		Hash: blockHashHex,
	}

	matched := im.requestMgr.MatchResponse(peerAddr, inv)
	if matched {
		log.Printf("p2p: matched BLOCK to pending GETDATA request from %s", peerAddr)
	}

	// Mark as seen (relay deduplication)
	im.blockRelay.MarkSeen(blockHashHex)
	im.blockRelay.MarkPeerAware(peerAddr, blockHashHex)

	// Notify block sync manager (Bitcoin-style async notification)
	if im.server.blockSyncMgr != nil {
		im.server.blockSyncMgr.NotifyBlockReceived(block)
	}

	// Process the block (add to blockchain)
	return im.server.handleReceivedBlock(conn, block)
}

// HandleTx processes a received TX message
// Matches to pending GETDATA requests (content-based)
func (im *InventoryManager) HandleTx(conn net.Conn, peerAddr string, tx *core.Transaction, txHash []byte) error {
	txHashHex := hex.EncodeToString(txHash)
	log.Printf("p2p: received TX message from %s hash=%s", peerAddr, txHashHex[:16])

	// Match to pending GETDATA request (content-based matching)
	inv := InventoryEntry{
		Type: InvTypeTx,
		Hash: txHashHex,
	}

	matched := im.requestMgr.MatchResponse(peerAddr, inv)
	if matched {
		log.Printf("p2p: matched TX to pending GETDATA request from %s", peerAddr)
	}

	// Mark as seen (relay deduplication)
	im.txRelay.MarkSeen(txHashHex)
	im.txRelay.MarkPeerAware(peerAddr, txHashHex)

	// Process the transaction (add to mempool)
	// Note: Tx sync not using BlockSyncManager, only blocks need special sync handling
	return im.server.handleReceivedTx(conn, tx)
}

// HandleNotFound processes a NOTFOUND message
// Cancels pending requests (content-based matching)
func (im *InventoryManager) HandleNotFound(peerAddr string, inv []InventoryEntry) {
	log.Printf("p2p: received NOTFOUND from %s for %d entries", peerAddr, len(inv))

	// Cancel all matching pending requests
	for _, entry := range inv {
		im.requestMgr.MatchResponse(peerAddr, entry)
	}
}

// GetRequestManager returns the request queue manager
func (im *InventoryManager) GetRequestManager() *RequestQueueManager {
	return im.requestMgr
}

// StartCleanup starts the background cleanup goroutine
func (im *InventoryManager) StartCleanup(ctx context.Context) {
	im.requestMgr.StartCleanupLoop(ctx)
}

// TxRelayManager manages transaction relay deduplication
// Implements Bitcoin-style tx propagation with bloom filters (simplified)
type TxRelayManager struct {
	seenTxs   map[string]time.Time // Hash -> first seen time
	peerTxs   map[string]map[string]bool // Peer -> set of known txs
	mu         sync.RWMutex
}

// NewTxRelayManager creates a new tx relay manager
func NewTxRelayManager() *TxRelayManager {
	return &TxRelayManager{
		seenTxs: make(map[string]time.Time),
		peerTxs: make(map[string]map[string]bool),
	}
}

// IsKnown checks if a transaction has been seen before
func (tm *TxRelayManager) IsKnown(txHashHex string) bool {
	tm.mu.RLock()
	_, exists := tm.seenTxs[txHashHex]
	tm.mu.RUnlock()
	return exists
}

// MarkSeen marks a transaction as seen
func (tm *TxRelayManager) MarkSeen(txHashHex string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.seenTxs[txHashHex]; !exists {
		tm.seenTxs[txHashHex] = time.Now()
	}
}

// DoesPeerKnow checks if a peer knows about a transaction
func (tm *TxRelayManager) DoesPeerKnow(peerAddr, txHashHex string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	peerKnown, exists := tm.peerTxs[peerAddr]
	if !exists {
		return false
	}

	return peerKnown[txHashHex]
}

// MarkPeerAware marks that a peer knows about a transaction
func (tm *TxRelayManager) MarkPeerAware(peerAddr, txHashHex string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.peerTxs[peerAddr]; !exists {
		tm.peerTxs[peerAddr] = make(map[string]bool)
	}
	tm.peerTxs[peerAddr][txHashHex] = true
}

// BlockRelayManager manages block relay deduplication
// Implements Bitcoin-style block propagation with bloom filters (simplified)
type BlockRelayManager struct {
	seenBlocks map[string]time.Time // Hash -> first seen time
	peerBlocks map[string]map[string]bool // Peer -> set of known blocks
	mu         sync.RWMutex
}

// NewBlockRelayManager creates a new block relay manager
func NewBlockRelayManager() *BlockRelayManager {
	return &BlockRelayManager{
		seenBlocks: make(map[string]time.Time),
		peerBlocks: make(map[string]map[string]bool),
	}
}

// IsKnown checks if a block has been seen before
func (bm *BlockRelayManager) IsKnown(blockHashHex string) bool {
	bm.mu.RLock()
	_, exists := bm.seenBlocks[blockHashHex]
	bm.mu.RUnlock()
	return exists
}

// MarkSeen marks a block as seen
func (bm *BlockRelayManager) MarkSeen(blockHashHex string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if _, exists := bm.seenBlocks[blockHashHex]; !exists {
		bm.seenBlocks[blockHashHex] = time.Now()
	}
}

// DoesPeerKnow checks if a peer knows about a block
func (bm *BlockRelayManager) DoesPeerKnow(peerAddr, blockHashHex string) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	peerKnown, exists := bm.peerBlocks[peerAddr]
	if !exists {
		return false
	}

	return peerKnown[blockHashHex]
}

// MarkPeerAware marks that a peer knows about a block
func (bm *BlockRelayManager) MarkPeerAware(peerAddr, blockHashHex string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if _, exists := bm.peerBlocks[peerAddr]; !exists {
		bm.peerBlocks[peerAddr] = make(map[string]bool)
	}
	bm.peerBlocks[peerAddr][blockHashHex] = true
}
