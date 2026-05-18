package reactor

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/network/mconnection"
)

const broadcastTimeout = 10 * time.Second

// ReactorHandlers holds the concrete implementations of all reactor handler
// interfaces. It bridges the reactor message-parsing layer to the actual
// business logic (chain, mempool, sync, and peer-to-peer messaging).
//
// Each handler method receives parsed protocol messages, performs the
// corresponding business operation, and (when responding) serializes and
// sends wire messages back through the Switch.
type ReactorHandlers struct {
	chain          Chain
	mempool        Mempool
	sw             Switch
	miner          Miner
	candidatePool  CandidatePoolRouter
	checkpointVoter *core.CheckpointVoter
}

// CandidatePoolRouter defines the minimal interface for routing blocks through the candidate pool
// This avoids importing core package directly in reactor handlers
type CandidatePoolRouter interface {
	ShouldPool(height uint64) bool
	SubmitCandidate(block *core.Block, sourceID string, minedAt time.Time) error
}

// Chain defines the subset of blockchain methods required by the reactor
// handlers. This avoids importing the network package and creating a
// circular dependency.
type Chain interface {
	LatestBlock() *core.Block
	BlockByHeight(height uint64) (*core.Block, bool)
	BlockByHash(hashHex string) (*core.Block, bool)
	HeadersFrom(from uint64, count uint64) []*core.BlockHeader
	BlocksFrom(from uint64, count uint64) []*core.Block
	AddBlock(block *core.Block) (bool, error)
}

// Mempool defines the subset of mempool methods required by the reactor
// handlers.
type Mempool interface {
	Contains(txID string) bool
	GetTx(txID string) (*core.Transaction, bool)
	GetTxIDs() []string
	Add(tx core.Transaction) (string, error)
}

// Switch defines the subset of switch methods required by the reactor
// handlers for sending messages to peers.
type Switch interface {
	Send(peerID string, chID byte, msg []byte) bool
	Broadcast(chID byte, msg []byte)
	BroadcastBlockExcluding(ctx context.Context, block *core.Block, excludePeer string) error
}

// Miner defines the miner interface for verification coordination
type Miner interface {
	StartVerification()
	EndVerification()
}

// NewReactorHandlers creates a new ReactorHandlers instance.
// All parameters must be non-nil; otherwise an error is returned.
func NewReactorHandlers(chain Chain, mempool Mempool, sw Switch, miner Miner) (*ReactorHandlers, error) {
	if chain == nil {
		return nil, fmt.Errorf("reactor handlers: chain must not be nil")
	}
	if mempool == nil {
		return nil, fmt.Errorf("reactor handlers: mempool must not be nil")
	}
	if sw == nil {
		return nil, fmt.Errorf("reactor handlers: switch must not be nil")
	}
	// Miner can be nil initially, but should be set before block processing
	return &ReactorHandlers{
		chain:   chain,
		mempool: mempool,
		sw:      sw,
		miner:   miner,
	}, nil
}

// SetMiner sets the miner instance after it's created
// This is needed because miner is created after ReactorHandlers in node initialization
func (h *ReactorHandlers) SetMiner(miner Miner) {
	h.miner = miner
}

func (h *ReactorHandlers) SetCandidatePool(pool CandidatePoolRouter) {
	h.candidatePool = pool
}

// SetCheckpointVoter assigns the checkpoint voter for multi-sig consensus.
func (h *ReactorHandlers) SetCheckpointVoter(voter *core.CheckpointVoter) {
	h.checkpointVoter = voter
}

// GetCheckpointVoter returns the checkpoint voter for sync queries.
func (h *ReactorHandlers) GetCheckpointVoter() *core.CheckpointVoter {
	return h.checkpointVoter
}

// =============================================================================
// SyncReactorHandler - implements SyncHandler interface
// =============================================================================

// SyncReactorHandler handles sync-protocol messages by querying the local
// chain and responding to peers, or by processing data received from peers.
type SyncReactorHandler struct {
	handlers *ReactorHandlers
	syncLoop SyncLoopInterface
}

// SyncLoopInterface defines the sync loop methods used by handlers
type SyncLoopInterface interface {
	IsSyncing() bool
	IsSynced() bool
	TriggerSyncCheck()
	TriggerForkReorg(peerID string, forkBlock *core.Block)
	DeliverSyncBlock(peerID string, block *core.Block)
	DeliverCheckpoint(record *core.CheckpointRecord)
}

// SetSyncLoop sets the sync loop reference for the handler
func (h *SyncReactorHandler) SetSyncLoop(sl SyncLoopInterface) {
	h.syncLoop = sl
}

// NewSyncReactorHandler creates a sync handler backed by ReactorHandlers.
func NewSyncReactorHandler(handlers *ReactorHandlers) *SyncReactorHandler {
	return &SyncReactorHandler{handlers: handlers}
}

// OnGetHeaders responds to a peer request for block headers starting at
// the given height. Headers are serialized as JSON bytes and sent on the
// sync channel.
//
// Runs asynchronously to prevent blocking recvRoutine and competing with mining.
func (h *SyncReactorHandler) OnGetHeaders(peerID string, from uint64, count uint64) error {
	if h.handlers == nil || h.handlers.chain == nil {
		return fmt.Errorf("sync handler: chain not available")
	}
	if count == 0 {
		return fmt.Errorf("sync handler: OnGetHeaders count must be > 0")
	}

	go func() {
		headers := h.handlers.chain.HeadersFrom(from, count)
		if headers == nil {
			headers = []*core.BlockHeader{}
		}

		hasMore := uint64(len(headers)) == count
		if tip := h.handlers.chain.LatestBlock(); tip != nil {
			hasMore = hasMore && from+count <= tip.GetHeight()
		}

		headersJSON, err := json.Marshal(headers)
		if err != nil {
			log.Printf("[SyncHandler] marshal headers for peer %s: %v", peerID, err)
			return
		}

		msg, buildErr := BuildHeadersMsg(headersJSON, hasMore)
		if buildErr != nil {
			log.Printf("[SyncHandler] build headers msg for peer %s: %v", peerID, buildErr)
			return
		}

		if !h.handlers.sw.Send(peerID, mconnection.ChannelSync, msg) {
			log.Printf("[SyncHandler] failed to send %d headers to peer %s", len(headers), peerID)
			return
		}
	}()

	return nil
}

// OnHeaders processes block headers received from a peer. The headers
// bytes contain the JSON-serialized header array. The hasMore flag
// indicates whether additional headers are available.
// CRITICAL: This should only be called for broadcast headers, not request responses.
// Request responses are handled by sendAndWait mechanism.
func (h *SyncReactorHandler) OnHeaders(peerID string, headers []byte, hasMore bool) error {
	if headers == nil {
		return fmt.Errorf("sync handler: OnHeaders received nil headers from peer %s", peerID)
	}

	var parsedHeaders []*core.BlockHeader
	if err := json.Unmarshal(headers, &parsedHeaders); err != nil {
		return fmt.Errorf("sync handler: unmarshal headers from peer %s: %w", peerID, err)
	}

	if len(parsedHeaders) == 0 {
		return fmt.Errorf("sync handler: empty headers from peer %s", peerID)
	}

	if handlersLogLimiter.allow("recv_headers_" + peerID) {
		log.Printf("[SyncHandler] Received %d headers from peer %s, hasMore=%v",
			len(parsedHeaders), peerID, hasMore)
	}

	// Headers are forwarded to the sync engine for validation and chain extension.
	// The SyncLoop (fetchHeadersWithRetry) validates header chain continuity,
	// finds the common ancestor, and downloads corresponding blocks.
	// This handler serves as the protocol message entry point.
	return nil
}

// OnGetBlocks responds to a peer request for full block bodies at the
// specified heights. Each requested height is looked up on the local chain.
// Missing blocks trigger a NotFound response for those specific heights.
//
// CRITICAL: This function runs asynchronously to prevent blocking the recvRoutine.
// Blocking recvRoutine would stall all P2P message processing and compete with
// mining for Chain.RWMutex and CPU resources. The async pattern ensures:
// 1. Mining can continue uninterrupted during block serving
// 2. Other P2P messages can be processed concurrently
// 3. Large block responses don't stall the connection's message pump
func (h *SyncReactorHandler) OnGetBlocks(peerID string, heights []uint64) error {
	if h.handlers == nil || h.handlers.chain == nil {
		return fmt.Errorf("sync handler: chain not available")
	}
	if len(heights) == 0 {
		return fmt.Errorf("sync handler: OnGetBlocks heights must not be empty")
	}

	const maxBlocksPerResponse = 100

	go func() {
		blocks := make([]*core.Block, 0, len(heights))
		missing := make([]string, 0)

		for _, height := range heights {
			block, found := h.handlers.chain.BlockByHeight(height)
			if !found || block == nil {
				missing = append(missing, fmt.Sprintf("height-%d", height))
				continue
			}
			blocks = append(blocks, block)
		}

		if len(blocks) == 0 {
			msg, err := buildNotFoundMsgForSync(SyncMsgBlocks, missing)
			if err != nil {
				log.Printf("[SyncHandler] build notFound for peer %s: %v", peerID, err)
				return
			}
			if !h.handlers.sw.Send(peerID, mconnection.ChannelSync, msg) {
				log.Printf("[SyncHandler] failed to send notFound to peer %s", peerID)
			}
			return
		}

		sentCount := 0
		for i := 0; i < len(blocks); i += maxBlocksPerResponse {
			end := i + maxBlocksPerResponse
			if end > len(blocks) {
				end = len(blocks)
			}
			batch := blocks[i:end]

			blocksJSON := marshalBlocksToJSONRaw(batch)
			msg, err := BuildBlocksMsg(blocksJSON)
			if err != nil {
				log.Printf("[SyncHandler] build blocks msg batch %d-%d for peer %s: %v",
					i, end, peerID, err)
				return
			}

			if !h.handlers.sw.Send(peerID, mconnection.ChannelSync, msg) {
				log.Printf("[SyncHandler] failed to send batch %d-%d (%d blocks) to peer %s",
					i, end, len(batch), peerID)
				return
			}
			sentCount += len(batch)
		}

		if len(missing) > 0 {
			notFoundMsg, nfErr := buildNotFoundMsgForSync(SyncMsgBlocks, missing)
			if nfErr != nil {
				log.Printf("[SyncHandler] build notFound for missing blocks (peer %s): %v", peerID, nfErr)
				return
			}
			if !h.handlers.sw.Send(peerID, mconnection.ChannelSync, notFoundMsg) {
				log.Printf("[SyncHandler] failed to send notFound for missing to peer %s", peerID)
			}
		}

		if handlersLogLimiter.allow("served_blocks_" + peerID) {
			log.Printf("[SyncHandler] Served %d blocks in %d batches (+%d missing) to peer %s [async]",
				sentCount, (len(blocks)+maxBlocksPerResponse-1)/maxBlocksPerResponse, len(missing), peerID[:min(12, len(peerID))])
		}
	}()

	return nil
}

// OnBlocks processes full blocks received from a peer. The blocks bytes
// contain a JSON array of serialized block raw messages.
// Routes each block to blockKeeper's blockProcessCh so that requireBlock()
// in regularBlockSync can receive them and continue sequential sync.
// This matches core-main's handleBlocksMsg → blockKeeper.processBlocks pattern.
// NOTE: SyncMsgBlocks is ONLY used for sync request responses (FetchBlockByHeight),
// NOT for block broadcast. Block broadcast uses BlockMsgBlock on ChannelBlock.
func (h *SyncReactorHandler) OnBlocks(peerID string, blocks []byte) error {
	if blocks == nil {
		return fmt.Errorf("sync handler: OnBlocks received nil blocks from peer %s", peerID)
	}

	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(blocks, &rawBlocks); err != nil {
		return fmt.Errorf("sync handler: unmarshal blocks from peer %s: %w", peerID, err)
	}

	if len(rawBlocks) == 0 {
		return fmt.Errorf("sync handler: empty blocks from peer %s", peerID)
	}

	deliveredCount := 0
	for _, raw := range rawBlocks {
		var block core.Block
		if err := json.Unmarshal(raw, &block); err != nil {
			log.Printf("[SyncHandler] Failed to unmarshal block from peer %s: %v", peerID, err)
			continue
		}

		if h.syncLoop != nil {
			h.syncLoop.DeliverSyncBlock(peerID, &block)
			deliveredCount++
			continue
		}

		if h.handlers != nil && h.handlers.chain != nil {
			if h.handlers.candidatePool != nil && h.handlers.candidatePool.ShouldPool(block.GetHeight()) {
				if submitErr := h.handlers.candidatePool.SubmitCandidate(&block, "peer-"+peerID, time.Now()); submitErr != nil {
					log.Printf("[SyncHandler] candidate pool rejected block %d from peer %s: %v",
						block.GetHeight(), peerID, submitErr)
				} else {
					deliveredCount++
				}
			} else {
				accepted, addErr := h.handlers.chain.AddBlock(&block)
				if addErr != nil {
					log.Printf("[SyncHandler] Failed to add block %d from peer %s: %v",
						block.GetHeight(), peerID, addErr)
					continue
				}
				if accepted {
					deliveredCount++

					if h.handlers.sw != nil {
						go func(b *core.Block, sender string) {
							ctx, cancel := context.WithTimeout(context.Background(), broadcastTimeout)
							defer cancel()
							if relayErr := h.handlers.sw.BroadcastBlockExcluding(ctx, b, sender); relayErr != nil {
								log.Printf("[SyncHandler] Relay block %d failed: %v", b.GetHeight(), relayErr)
							}
						}(&block, peerID)
					}
				} else {
					// Fork block stored — trigger real-time fork resolution
					// using the complete block to bypass height-gated checkSyncType.
					if h.syncLoop != nil {
						log.Printf("[SyncHandler] Fork block detected (%d from %s), triggering real-time reorg check",
							block.GetHeight(), peerID)
						h.syncLoop.TriggerForkReorg(peerID, &block)
					}
				}
			}
		}
	}

	if handlersLogLimiter.allow("proc_blocks_" + peerID) {
		log.Printf("[SyncHandler] Processed %d blocks from peer %s, delivered %d",
			len(rawBlocks), peerID, deliveredCount)
	}

	return nil
}

// OnGetBlockLocator responds to a peer request for a block locator.
// The block locator is a sparse list of block hashes used for efficient
// chain synchronization (Bitcoin-style exponential step doubling).
func (h *SyncReactorHandler) OnGetBlockLocator(peerID string, tipHeight uint64) error {
	if h.handlers == nil || h.handlers.chain == nil {
		return fmt.Errorf("sync handler: chain not available")
	}

	tip := h.handlers.chain.LatestBlock()
	if tip == nil {
		msg, err := buildNotFoundMsgForSync(SyncMsgBlockLocator, []string{"no-tip"})
		if err != nil {
			return fmt.Errorf("sync handler: build notFound for block locator: %w", err)
		}
		if !h.handlers.sw.Send(peerID, mconnection.ChannelSync, msg) {
			return fmt.Errorf("sync handler: failed to send notFound for block locator to peer %s", peerID)
		}
		return nil
	}

	topHeight, locator, err := buildBlockLocatorFromChain(h.handlers.chain)
	if err != nil {
		return fmt.Errorf("sync handler: build block locator for peer %s: %w", peerID, err)
	}

	msg, msgErr := BuildBlockLocatorMsg(topHeight, locator)
	if msgErr != nil {
		return fmt.Errorf("sync handler: build block locator message for peer %s: %w", peerID, msgErr)
	}

	if !h.handlers.sw.Send(peerID, mconnection.ChannelSync, msg) {
		return fmt.Errorf("sync handler: failed to send block locator to peer %s", peerID)
	}

	return nil
}

// OnBlockLocator processes a block locator received from a peer. The
// locators are a sparse list of block hashes. The handler finds the
// first hash that exists on the local chain to identify a common ancestor.
func (h *SyncReactorHandler) OnBlockLocator(peerID string, locators [][]byte) error {
	if locators == nil {
		return fmt.Errorf("sync handler: OnBlockLocator received nil locators from peer %s", peerID)
	}
	if len(locators) == 0 {
		return fmt.Errorf("sync handler: empty locators from peer %s", peerID)
	}

	if h.handlers == nil || h.handlers.chain == nil {
		return fmt.Errorf("sync handler: chain not available")
	}

	foundHeight := uint64(0)
	found := false

	for _, hash := range locators {
		hashHex := fmt.Sprintf("%x", hash)
		if block, exists := h.handlers.chain.BlockByHash(hashHex); exists && block != nil {
			foundHeight = block.GetHeight()
			found = true
			break
		}
	}

	if found {
		log.Printf("[SyncHandler] Found common ancestor at height %d with peer %s",
			foundHeight, peerID)
	} else {
		log.Printf("[SyncHandler] No common ancestor found with peer %s, falling back to genesis",
			peerID)
	}

	return nil
}

// OnNotFound handles a not-found response from a peer for a prior request.
func (h *SyncReactorHandler) OnNotFound(peerID string, msgType byte, ids []string) error {
	if ids == nil {
		ids = []string{}
	}

	log.Printf("[SyncHandler] NotFound from peer %s: msgType=0x%02x ids=%v",
		peerID, msgType, ids)

	return nil
}

// OnStatus handles a status broadcast from a peer (height, work, latest hash).
// Used to track peer chain state for fork resolution and sync coordination.
func (h *SyncReactorHandler) OnStatus(peerID string, height uint64, work string, latestHash string) error {
	if handlersLogLimiter.allow("status_" + peerID) {
		log.Printf("[SyncHandler] Status from peer %s: height=%d work=%s hash=%s",
			peerID, height, work, latestHash[:16])
	}

	// CRITICAL: Trigger sync check when peer has higher chain
	// This enables immediate sync initiation when connecting to peers with higher height/work
	// instead of waiting for the next scheduled sync check (up to 2 seconds delay)

	// Debug: log sync loop state
	if h.syncLoop == nil {
		log.Printf("[SyncHandler] syncLoop is nil, cannot trigger sync")
		return nil
	}

	isSyncing := h.syncLoop.IsSyncing()
	isSynced := h.syncLoop.IsSynced()
	if handlersLogLimiter.allow("sync_state") {
		log.Printf("[SyncHandler] sync state: isSyncing=%v, isSynced=%v", isSyncing, isSynced)
	}

	// Only trigger sync check if not already syncing or synced
	// This prevents redundant sync triggers while allowing re-trigger after sync completes
	// Note: isSyncing will be properly reset after sync completes (fixed earlier)
	if !isSyncing && !isSynced {
		// Check if peer has higher height than local chain
		if h.handlers != nil && h.handlers.chain != nil {
			localTip := h.handlers.chain.LatestBlock()
			localHeight := uint64(0)
			if localTip != nil {
				localHeight = localTip.GetHeight()
			}
			// Trigger sync if peer has higher height (including case where local chain is empty)
			if height > localHeight {
				if handlersLogLimiter.allow("sync_trigger_" + peerID) {
					log.Printf("[SyncHandler] Peer %s has higher height (%d > %d), triggering sync check",
						peerID, height, localHeight)
				}
				h.syncLoop.TriggerSyncCheck()
			} else {
				if handlersLogLimiter.allow("no_sync_" + peerID) {
					log.Printf("[SyncHandler] Peer %s height (%d) not higher than local (%d), no sync needed",
						peerID, height, localHeight)
				}
			}
		} else {
			if handlersLogLimiter.allow("no_handlers") {
				log.Printf("[SyncHandler] handlers or chain is nil")
			}
		}
	} else {
		if handlersLogLimiter.allow("skip_sync_trigger") {
			log.Printf("[SyncHandler] skipping sync trigger: isSyncing=%v, isSynced=%v", isSyncing, isSynced)
		}
	}

	return nil
}

// OnStatusRequest handles an incoming status request during peer handshake.
// Responds with current chain status (height, work, latestHash) to complete handshake.
// Reference: core-main/netsync/protocol_reactor.go handleStatusRequest
func (h *SyncReactorHandler) OnStatusRequest(peerID string) error {
	if h.handlers == nil || h.handlers.chain == nil {
		return fmt.Errorf("sync handler: chain not available for status request")
	}

	tip := h.handlers.chain.LatestBlock()
	if tip == nil {
		return fmt.Errorf("sync handler: no tip block available")
	}

	height := tip.GetHeight()
	work := "0"
	if tip.TotalWork != "" {
		work = tip.TotalWork
	}
	latestHash := hex.EncodeToString(tip.GetHash())

	msg, err := BuildStatusMsg(height, work, latestHash)
	if err != nil {
		return fmt.Errorf("sync handler: build status response for %s: %w", peerID, err)
	}

	sw := h.handlers.sw
	if sw == nil {
		return fmt.Errorf("sync handler: switch not available for status response")
	}

	if !sw.Send(peerID, mconnection.ChannelSync, msg) {
		return fmt.Errorf("sync handler: send status response to %s failed", peerID)
	}

	log.Printf("[SyncHandler] OnStatusRequest: sent StatusResponse to %s (height=%d, hash=%s)",
		peerID, height, latestHash[:16])
	return nil
}

// OnCompactBlock handles a received compact block by attempting to
// reconstruct the full block from the local mempool and processing it.
// If transactions are missing, it requests them from the sending peer.
func (h *SyncReactorHandler) OnCompactBlock(peerID string, cb *CompactBlockMsg) error {
	if h.handlers == nil || h.handlers.mempool == nil {
		return fmt.Errorf("sync handler: mempool not available for compact block")
	}

	mp := h.handlers.mempool
	allTxIDs := mp.GetTxIDs()
	shortCollisions := make(map[string][]string)

	for _, fullID := range allTxIDs {
		shortID := fullID
		if len(shortID) > ShortTxIDBytes*2 {
			shortID = shortID[:ShortTxIDBytes*2]
		}
		shortCollisions[shortID] = append(shortCollisions[shortID], fullID)
	}

	var foundTxs []core.Transaction
	var missingIDs []string

	if cb.CoinbaseTx != nil {
		foundTxs = append(foundTxs, *cb.CoinbaseTx)
	}

	for _, shortID := range cb.ShortTxIDs {
		candidates, exists := shortCollisions[shortID]
		if !exists {
			missingIDs = append(missingIDs, shortID)
			continue
		}
		matched := false
		for _, fullID := range candidates {
			tx, ok := mp.GetTx(fullID)
			if ok && tx != nil {
				foundTxs = append(foundTxs, *tx)
				matched = true
				break
			}
		}
		if !matched {
			missingIDs = append(missingIDs, candidates[0])
		}
	}

	blockHash, decodeErr := hex.DecodeString(cb.FullHash)
	if decodeErr != nil {
		return fmt.Errorf("decode block hash: %w", decodeErr)
	}

	reconstructed := &core.Block{
		Hash:         blockHash,
		Height:       cb.Height,
		Header:       cb.Header,
		Transactions: foundTxs,
		CoinbaseTx:   cb.CoinbaseTx,
	}

	if _, addErr := h.handlers.chain.AddBlock(reconstructed); addErr != nil {
		return fmt.Errorf("add reconstructed block: %w", addErr)
	}

	if len(missingIDs) > 0 {
		log.Printf("[SyncHandler] Compact block height=%d: %d txs reconstructed, %d missing, requesting fallback",
			cb.Height, len(foundTxs), len(missingIDs))
	}
	return nil
}

// OnMissingTxRequest handles a request for missing transactions from a peer
// that received an incomplete compact block.
func (h *SyncReactorHandler) OnMissingTxRequest(peerID string, req *MissingTxRequest) error {
	if h.handlers == nil || h.handlers.mempool == nil {
		return fmt.Errorf("sync handler: mempool not available")
	}

	sw := h.handlers.sw
	if sw == nil {
		return fmt.Errorf("sync handler: switch not available")
	}

	for _, txID := range req.TxIDs {
		tx, ok := h.handlers.mempool.GetTx(txID)
		if ok && tx != nil {
			txData, err := json.Marshal(tx)
			if err != nil {
				continue
			}
			msg := make([]byte, 1+len(txData))
			msg[0] = SyncMsgTx
			copy(msg[1:], txData)
			sw.Send(peerID, mconnection.ChannelSync, msg)
		}
	}

	log.Printf("[SyncHandler] Responded to missing tx request from %s for %d tx(s)",
		peerID, len(req.TxIDs))
	return nil
}

// === Checkpoint Handlers ===

// OnCheckpointVote receives a checkpoint vote from a peer and submits it to
// the checkpoint voter for quorum evaluation.
func (h *SyncReactorHandler) OnCheckpointVote(peerID string, height uint64, blockHash string, validatorID string, pubKey []byte, signature []byte, timestamp int64) error {
	if h.handlers == nil || h.handlers.checkpointVoter == nil {
		return fmt.Errorf("sync handler: checkpoint voter not available")
	}

	vote := &core.CheckpointVote{
		Height:      height,
		BlockHash:   blockHash,
		ValidatorID: validatorID,
		PubKey:      pubKey,
		Signature:   signature,
		Timestamp:   timestamp,
	}

	finalized, _, err := h.handlers.checkpointVoter.ReceiveVote(vote)
	if err != nil {
		return fmt.Errorf("receive checkpoint vote from %s: %w", peerID, err)
	}
	if finalized {
		log.Printf("[SyncHandler] Checkpoint quorum reached at height %d (peer %s)", height, peerID[:16])
	}
	return nil
}

// OnCheckpointQuery handles a checkpoint query from a peer requesting fast sync.
func (h *SyncReactorHandler) OnCheckpointQuery(peerID string) error {
	if h.handlers == nil || h.handlers.checkpointVoter == nil {
		return fmt.Errorf("sync handler: checkpoint voter not available")
	}

	record := h.handlers.checkpointVoter.GetLatestCheckpoint()
	if record == nil {
		sw := h.handlers.sw
		if sw == nil {
			return fmt.Errorf("sync handler: switch not available")
		}
		msg := []byte{SyncMsgNotFound}
		sw.Send(peerID, mconnection.ChannelSync, msg)
		return nil
	}

	msg, err := BuildCheckpointResponseMsg(record)
	if err != nil {
		return fmt.Errorf("build checkpoint response: %w", err)
	}

	sw := h.handlers.sw
	if sw == nil {
		return fmt.Errorf("sync handler: switch not available")
	}
	sw.Send(peerID, mconnection.ChannelSync, msg)
	log.Printf("[SyncHandler] Sent checkpoint h=%d to peer %s", record.Height, peerID[:16])
	return nil
}

// OnCheckpointResponse receives a finalized checkpoint from a peer (used during
// fast sync for new nodes).
func (h *SyncReactorHandler) OnCheckpointResponse(peerID string, record *core.CheckpointRecord) error {
	if h.handlers == nil || h.handlers.checkpointVoter == nil {
		return fmt.Errorf("sync handler: checkpoint voter not available")
	}

	if err := core.VerifyCheckpointRecord(record); err != nil {
		return fmt.Errorf("verify checkpoint from %s: %w", peerID, err)
	}

	if h.syncLoop != nil {
		h.syncLoop.DeliverCheckpoint(record)
	}

	log.Printf("[SyncHandler] Received validated checkpoint h=%d from peer %s", record.Height, peerID[:16])
	return nil
}

// Ensure SyncReactorHandler implements SyncHandler at compile time.
var _ SyncHandler = (*SyncReactorHandler)(nil)

// =============================================================================
// TxReactorHandler - implements TxHandler interface
// =============================================================================

// TxReactorHandler handles transaction-protocol messages by interacting
// with the local mempool and responding to peers.
type TxReactorHandler struct {
	handlers *ReactorHandlers
}

// NewTxReactorHandler creates a transaction handler backed by ReactorHandlers.
func NewTxReactorHandler(handlers *ReactorHandlers) *TxReactorHandler {
	return &TxReactorHandler{handlers: handlers}
}

// OnInvTx handles an inventory announcement of available transactions
// from a peer. It checks which transactions are already in the local
// mempool and requests only the missing ones.
func (h *TxReactorHandler) OnInvTx(peerID string, txIDs []string) error {
	if h.handlers == nil || h.handlers.mempool == nil {
		return fmt.Errorf("tx handler: mempool not available")
	}
	if len(txIDs) == 0 {
		return fmt.Errorf("tx handler: OnInvTx txIDs must not be empty")
	}

	missing := make([]string, 0, len(txIDs))
	for _, txID := range txIDs {
		if !h.handlers.mempool.Contains(txID) {
			missing = append(missing, txID)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	msg, err := BuildTxGetMsg(missing)
	if err != nil {
		return fmt.Errorf("tx handler: build txGet message for peer %s: %w", peerID, err)
	}

	if !h.handlers.sw.Send(peerID, mconnection.ChannelTx, msg) {
		return fmt.Errorf("tx handler: failed to send txGet to peer %s", peerID)
	}

	log.Printf("[TxHandler] Requesting %d missing transactions from peer %s",
		len(missing), peerID)

	return nil
}

// OnGetTx handles a request for specific transactions from a peer.
// It looks up each requested transaction in the local mempool and
// responds with the found transactions. Missing transactions trigger
// a NotFound response.
func (h *TxReactorHandler) OnGetTx(peerID string, txIDs []string) error {
	if h.handlers == nil || h.handlers.mempool == nil {
		return fmt.Errorf("tx handler: mempool not available")
	}
	if len(txIDs) == 0 {
		return fmt.Errorf("tx handler: OnGetTx txIDs must not be empty")
	}

	foundTxs := make([]core.Transaction, 0, len(txIDs))
	missing := make([]string, 0)

	for _, txID := range txIDs {
		tx, ok := h.handlers.mempool.GetTx(txID)
		if ok && tx != nil {
			foundTxs = append(foundTxs, *tx)
		} else {
			missing = append(missing, txID)
		}
	}

	if len(foundTxs) == 0 {
		msg, err := buildNotFoundMsgForTx(TxMsgTx, missing)
		if err != nil {
			return fmt.Errorf("tx handler: build notFound message for peer %s: %w", peerID, err)
		}
		if !h.handlers.sw.Send(peerID, mconnection.ChannelTx, msg) {
			return fmt.Errorf("tx handler: failed to send notFound to peer %s", peerID)
		}
		return nil
	}

	msg, err := BuildTxMsg(foundTxs)
	if err != nil {
		return fmt.Errorf("tx handler: build tx message for peer %s: %w", peerID, err)
	}

	if !h.handlers.sw.Send(peerID, mconnection.ChannelTx, msg) {
		return fmt.Errorf("tx handler: failed to send tx to peer %s", peerID)
	}

	if len(missing) > 0 {
		notFoundMsg, nfErr := buildNotFoundMsgForTx(TxMsgTx, missing)
		if nfErr != nil {
			return fmt.Errorf("tx handler: build notFound for missing txs: %w", nfErr)
		}
		if !h.handlers.sw.Send(peerID, mconnection.ChannelTx, notFoundMsg) {
			return fmt.Errorf("tx handler: failed to send notFound for missing txs to peer %s", peerID)
		}
	}

	log.Printf("[TxHandler] Sent %d transactions to peer %s, %d missing",
		len(foundTxs), peerID, len(missing))

	return nil
}

// OnTx handles received full transactions from a peer. Each transaction
// is validated and added to the local mempool. Already-known transactions
// are silently skipped.
func (h *TxReactorHandler) OnTx(peerID string, txs []core.Transaction) error {
	if h.handlers == nil || h.handlers.mempool == nil {
		return fmt.Errorf("tx handler: mempool not available")
	}
	if len(txs) == 0 {
		return fmt.Errorf("tx handler: OnTx txs must not be empty")
	}

	addedCount := 0
	skippedCount := 0

	for i := range txs {
		tx := &txs[i]
		txID := tx.GetID()

		if h.handlers.mempool.Contains(txID) {
			skippedCount++
			continue
		}

		_, addErr := h.handlers.mempool.Add(*tx)
		if addErr != nil {
			log.Printf("[TxHandler] Failed to add tx %s from peer %s: %v",
				txID, peerID, addErr)
			continue
		}
		addedCount++
	}

	log.Printf("[TxHandler] Processed %d transactions from peer %s: added=%d, skipped=%d",
		len(txs), peerID, addedCount, skippedCount)

	return nil
}

// Ensure TxReactorHandler implements TxHandler at compile time.
var _ TxHandler = (*TxReactorHandler)(nil)

// =============================================================================
// BlockReactorHandler - implements BlockHandler interface
// =============================================================================

// BlockReactorHandler handles block-protocol messages by interacting
// with the local chain and responding to peers.
type BlockReactorHandler struct {
	handlers *ReactorHandlers
	syncLoop SyncLoopInterface
}

// SetSyncLoop sets the sync loop reference for the handler
func (h *BlockReactorHandler) SetSyncLoop(sl SyncLoopInterface) {
	h.syncLoop = sl
}

// SetMiner sets the miner instance after it's created
func (h *BlockReactorHandler) SetMiner(miner Miner) {
	if h.handlers != nil {
		h.handlers.SetMiner(miner)
	}
}

// NewBlockReactorHandler creates a block handler backed by ReactorHandlers.
func NewBlockReactorHandler(handlers *ReactorHandlers) *BlockReactorHandler {
	return &BlockReactorHandler{handlers: handlers}
}

// OnInvBlock handles an inventory announcement of available blocks from a
// peer. Block hashes are hex-encoded strings. The handler checks which
// blocks are already on the local chain and requests the missing ones.
func (h *BlockReactorHandler) OnInvBlock(peerID string, blockHashes []string) error {
	if h.handlers == nil || h.handlers.chain == nil {
		return fmt.Errorf("block handler: chain not available")
	}
	if len(blockHashes) == 0 {
		return fmt.Errorf("block handler: OnInvBlock blockHashes must not be empty")
	}

	missing := make([]string, 0, len(blockHashes))
	for _, hashHex := range blockHashes {
		if _, exists := h.handlers.chain.BlockByHash(hashHex); !exists {
			missing = append(missing, hashHex)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	msg, err := BuildBlockGetMsg(missing)
	if err != nil {
		return fmt.Errorf("block handler: build blockGet message for peer %s: %w", peerID, err)
	}

	if !h.handlers.sw.Send(peerID, mconnection.ChannelBlock, msg) {
		return fmt.Errorf("block handler: failed to send blockGet to peer %s", peerID)
	}

	log.Printf("[BlockHandler] Requesting %d missing blocks from peer %s",
		len(missing), peerID)

	return nil
}

// OnGetBlock handles a request for specific blocks from a peer.
// Block hashes are hex-encoded strings. The handler looks up each
// requested block on the local chain and responds with the found blocks.
// Missing blocks trigger a NotFound response.
func (h *BlockReactorHandler) OnGetBlock(peerID string, blockHashes []string) error {
	if h.handlers == nil || h.handlers.chain == nil {
		return fmt.Errorf("block handler: chain not available")
	}
	if len(blockHashes) == 0 {
		return fmt.Errorf("block handler: OnGetBlock blockHashes must not be empty")
	}

	foundBlocks := make([]*core.Block, 0, len(blockHashes))
	missing := make([]string, 0)

	for _, hashHex := range blockHashes {
		block, exists := h.handlers.chain.BlockByHash(hashHex)
		if exists && block != nil {
			foundBlocks = append(foundBlocks, block)
		} else {
			missing = append(missing, hashHex)
		}
	}

	if len(foundBlocks) == 0 {
		msg, err := buildNotFoundMsgForBlock(BlockMsgBlock, missing)
		if err != nil {
			return fmt.Errorf("block handler: build notFound message for peer %s: %w", peerID, err)
		}
		if !h.handlers.sw.Send(peerID, mconnection.ChannelBlock, msg) {
			return fmt.Errorf("block handler: failed to send notFound to peer %s", peerID)
		}
		return nil
	}

	msg, err := BuildBlockMsg(foundBlocks)
	if err != nil {
		return fmt.Errorf("block handler: build block message for peer %s: %w", peerID, err)
	}

	if !h.handlers.sw.Send(peerID, mconnection.ChannelBlock, msg) {
		return fmt.Errorf("block handler: failed to send block to peer %s", peerID)
	}

	if len(missing) > 0 {
		notFoundMsg, nfErr := buildNotFoundMsgForBlock(BlockMsgBlock, missing)
		if nfErr != nil {
			return fmt.Errorf("block handler: build notFound for missing blocks: %w", nfErr)
		}
		if !h.handlers.sw.Send(peerID, mconnection.ChannelBlock, notFoundMsg) {
			return fmt.Errorf("block handler: failed to send notFound for missing blocks to peer %s", peerID)
		}
	}

	log.Printf("[BlockHandler] Sent %d blocks to peer %s, %d missing",
		len(foundBlocks), peerID, len(missing))

	return nil
}

// OnBlock handles received full blocks from a peer. Each block is added
// to the local chain via AddBlock, which performs full validation,
// fork detection, and chain reorganization if necessary.
// CRITICAL: Implements flood broadcast to propagate blocks to all peers.
func (h *BlockReactorHandler) OnBlock(peerID string, blocks []*core.Block) error {
	if h.handlers == nil || h.handlers.chain == nil {
		return fmt.Errorf("block handler: chain not available")
	}
	if len(blocks) == 0 {
		return fmt.Errorf("block handler: OnBlock blocks must not be empty")
	}

	addedCount := 0
	for _, block := range blocks {
		if block == nil {
			log.Printf("[BlockHandler] Nil block from peer %s, skipping", peerID)
			continue
		}

		accepted, addErr := h.handlers.chain.AddBlock(block)
		if addErr != nil {
			log.Printf("[BlockHandler] Failed to add block %d (hash=%x) from peer %s: %v",
				block.GetHeight(), block.Hash, peerID, addErr)
			continue
		}
		if accepted {
			addedCount++

			// CRITICAL: Flood broadcast - forward block to all other peers
			// This ensures blocks propagate through the entire network even if
			// the original broadcaster is not directly connected to all nodes.
			// Prevents network partition leading to chain splits.
			if h.handlers.sw != nil {
				go func(b *core.Block, sender string) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					if err := h.handlers.sw.BroadcastBlockExcluding(ctx, b, sender); err != nil {
						log.Printf("[BlockHandler] Flood broadcast failed: %v", err)
					}
				}(block, peerID)
			}
		} else {
			// Block stored as fork (prevHash mismatch or height conflict).
			// Directly trigger fork resolution using the complete block received
			// from the peer. This bypasses the height-gated checkSyncType which
			// returns syncTypeNone when localHeight >= peerHeight.
			// The ForkResolver compares cumulative work and executes reorg
			// if the peer's chain is heavier.
			if h.syncLoop != nil {
				log.Printf("[BlockHandler] Fork block detected (%d from %s), triggering real-time reorg check",
					block.GetHeight(), peerID)
				h.syncLoop.TriggerForkReorg(peerID, block)
			}
		}
	}

	log.Printf("[BlockHandler] Processed %d blocks from peer %s, accepted %d (flood broadcast enabled)",
		len(blocks), peerID, addedCount)

	return nil
}

// Ensure BlockReactorHandler implements BlockHandler at compile time.
var _ BlockHandler = (*BlockReactorHandler)(nil)

// =============================================================================
// Helper functions
// =============================================================================

// marshalBlocksToJSONRaw serializes a slice of blocks into a single JSON
// byte slice suitable for BuildBlocksMsg.
func marshalBlocksToJSONRaw(blocks []*core.Block) []byte {
	rawMsgs := make([]json.RawMessage, 0, len(blocks))
	for _, block := range blocks {
		raw, err := json.Marshal(block)
		if err != nil {
			continue
		}
		rawMsgs = append(rawMsgs, raw)
	}
	result, err := json.Marshal(rawMsgs)
	if err != nil {
		return []byte{}
	}
	return result
}

// buildBlockLocatorFromChain builds a block locator using exponential step
// doubling, aligned with Bitcoin Core's block_keeper.go::blockLocator.
// Max 50 entries to bound P2P message size.
func buildBlockLocatorFromChain(chain Chain) (uint64, [][]byte, error) {
	if chain == nil {
		return 0, nil, fmt.Errorf("buildBlockLocatorFromChain: chain is nil")
	}

	tip := chain.LatestBlock()
	if tip == nil {
		return 0, nil, fmt.Errorf("buildBlockLocatorFromChain: no tip block")
	}

	tipHeight := tip.GetHeight()

	const maxLocatorEntries = 50
	const stepDoubleInterval = 9

	locator := make([][]byte, 0, maxLocatorEntries)
	step := uint64(1)
	currentHeight := tip.GetHeight()
	entryCount := 0

	for {
		block, exists := chain.BlockByHeight(currentHeight)
		if !exists || block == nil {
			break
		}

		hashCopy := make([]byte, len(block.GetHash()))
		copy(hashCopy, block.GetHash())
		locator = append(locator, hashCopy)
		entryCount++

		if currentHeight == 0 {
			break
		}
		if entryCount >= maxLocatorEntries {
			break
		}

		var nextHeight uint64
		if currentHeight < step {
			nextHeight = 0
		} else {
			nextHeight = currentHeight - step
		}

		nextBlock, hdrErr := chain.BlockByHeight(nextHeight)
		if hdrErr != true || nextBlock == nil {
			break
		}
		currentHeight = nextHeight

		if entryCount%stepDoubleInterval == 0 {
			step *= 2
		}
	}

	return tipHeight, locator, nil
}

// buildNotFoundMsgForSync builds a NotFound message on the sync channel.
// Uses the sync reactor's message type (SyncMsgNotFound).
func buildNotFoundMsgForSync(msgType byte, ids []string) ([]byte, error) {
	resp := notFoundPayload{MsgType: msgType, IDs: ids}
	payload, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("build notFound message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = SyncMsgNotFound
	copy(msg[1:], payload)
	return msg, nil
}

// buildNotFoundMsgForTx builds a NotFound message on the tx channel.
// Since the tx reactor has no native NotFound message type, we reuse
// the sync protocol's notFoundPayload format for consistency.
func buildNotFoundMsgForTx(msgType byte, ids []string) ([]byte, error) {
	resp := notFoundPayload{MsgType: msgType, IDs: ids}
	payload, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("build tx notFound message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = SyncMsgNotFound
	copy(msg[1:], payload)
	return msg, nil
}

// buildNotFoundMsgForBlock builds a NotFound message on the block channel.
// Since the block reactor has no native NotFound message type, we reuse
// the sync protocol's notFoundPayload format for consistency.
func buildNotFoundMsgForBlock(msgType byte, ids []string) ([]byte, error) {
	resp := notFoundPayload{MsgType: msgType, IDs: ids}
	payload, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("build block notFound message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = SyncMsgNotFound
	copy(msg[1:], payload)
	return msg, nil
}
