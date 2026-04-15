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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
	nogoconfig "github.com/nogochain/nogo/config"
)

// =============================================================================
// P2P SYNC MESSAGE TYPES
// =============================================================================

// GetBlocksMessage requests blocks from a peer
type GetBlocksMessage struct {
	ParentHash  string `json:"parent_hash"`
	Limit       int    `json:"limit"`
	HeadersOnly bool   `json:"headers_only"`
}

// BlocksMessage responds with requested blocks
type BlocksMessage struct {
	Blocks     []*core.Block       `json:"blocks"`
	Headers    []*core.BlockHeader `json:"headers"`
	FromHeight uint64              `json:"from_height"`
	ToHeight   uint64              `json:"to_height"`
	Count      int                 `json:"count"`
}

// NotFoundMessage indicates requested blocks were not found
type NotFoundMessage struct {
	Hashes []string `json:"hashes"`
	Reason string   `json:"reason,omitempty"`
}

// SyncStatusMessage reports sync progress
type SyncStatusMessage struct {
	Height       uint64  `json:"height"`
	Hash         string  `json:"hash"`
	IsSyncing    bool    `json:"is_syncing"`
	SyncProgress float64 `json:"sync_progress"`
}

// P2PSyncProtocol implements Bitcoin-style fast sync protocol
type P2PSyncProtocol struct {
	bc                  BlockchainInterface
	pm                  PeerAPI
	scorer              *AdvancedPeerScorer
	pendingRequests     map[string]*pendingRequest
	requestMu           sync.RWMutex
	maxBlocksPerRequest int
	requestTimeout      time.Duration
}

type pendingRequest struct {
	hashes   []string
	response chan *BlocksMessage
	errChan  chan error
	timer    *time.Timer
}

// NewP2PSyncProtocol creates a new P2P sync protocol instance
func NewP2PSyncProtocol(bc BlockchainInterface, pm PeerAPI) *P2PSyncProtocol {
	scorer := NewAdvancedPeerScorer(nogoconfig.DefaultP2PMaxPeers)
	return &P2PSyncProtocol{
		bc:                  bc,
		pm:                  pm,
		scorer:              scorer,
		pendingRequests:     make(map[string]*pendingRequest),
		maxBlocksPerRequest: 500,
		requestTimeout:      30 * time.Second,
	}
}

// HandleGetBlocksMessage handles incoming getblocks requests
func (p *P2PSyncProtocol) HandleGetBlocksMessage(w http.ResponseWriter, r *http.Request) {
	var msg GetBlocksMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		p.sendError(w, "invalid request format", http.StatusBadRequest)
		return
	}

	log.Printf("[P2P Sync] Received getblocks request: parent_hash=%s limit=%d headers_only=%v",
		msg.ParentHash, msg.Limit, msg.HeadersOnly)

	if msg.Limit <= 0 || msg.Limit > p.maxBlocksPerRequest {
		msg.Limit = p.maxBlocksPerRequest
	}

	var startBlock *core.Block
	if msg.ParentHash == "" {
		startBlock = p.bc.LatestBlock()
		if startBlock == nil {
			p.sendError(w, "genesis block not found", http.StatusNotFound)
			return
		}
	} else {
		block, exists := p.bc.BlockByHash(msg.ParentHash)
		if !exists {
			p.sendError(w, fmt.Sprintf("parent block not found: %s", msg.ParentHash), http.StatusNotFound)
			return
		}
		startBlock = block
	}

	blocks := make([]*core.Block, 0, msg.Limit)
	headers := make([]*core.BlockHeader, 0, msg.Limit)

	currentHeight := startBlock.GetHeight() + 1
	endHeight := currentHeight + uint64(msg.Limit) - 1

	for h := currentHeight; h <= endHeight; h++ {
		block, ok := p.bc.BlockByHeight(h)
		if !ok || block == nil {
			break
		}

		if msg.HeadersOnly {
			header := &core.BlockHeader{
				TimestampUnix:  block.GetTimestampUnix(),
				PrevHash:       block.GetPrevHash(),
				DifficultyBits: block.GetDifficultyBits(),
				Nonce:          block.Header.Nonce,
				MerkleRoot:     block.Header.MerkleRoot,
			}
			headers = append(headers, header)
		} else {
			blocks = append(blocks, block)
		}
	}

	response := BlocksMessage{
		Blocks:     blocks,
		Headers:    headers,
		FromHeight: currentHeight,
		ToHeight:   currentHeight + uint64(len(blocks)) - 1,
		Count:      len(blocks),
	}

	log.Printf("[P2P Sync] Sending blocks response: from=%d to=%d count=%d",
		response.FromHeight, response.ToHeight, response.Count)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[P2P Sync] Failed to send response: %v", err)
	}
}

// RequestBlocks requests blocks from a peer
func (p *P2PSyncProtocol) RequestBlocks(ctx context.Context, peerAddr string, parentHash string, limit int, headersOnly bool) (*BlocksMessage, error) {
	if limit <= 0 {
		limit = p.maxBlocksPerRequest
	}

	req := &GetBlocksMessage{
		ParentHash:  parentHash,
		Limit:       limit,
		HeadersOnly: headersOnly,
	}

	url := fmt.Sprintf("http://%s/sync/getblocks", peerAddr)
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: p.requestTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var blocksMsg BlocksMessage
	if err := json.NewDecoder(resp.Body).Decode(&blocksMsg); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	log.Printf("[P2P Sync] Received %d blocks from peer %s (height %d-%d)",
		blocksMsg.Count, peerAddr, blocksMsg.FromHeight, blocksMsg.ToHeight)

	return &blocksMsg, nil
}

// RequestBlockHeaders requests block headers from a peer
func (p *P2PSyncProtocol) RequestBlockHeaders(ctx context.Context, peerAddr string, parentHash string, limit int) ([]*core.BlockHeader, error) {
	blocksMsg, err := p.RequestBlocks(ctx, peerAddr, parentHash, limit, true)
	if err != nil {
		return nil, err
	}

	return blocksMsg.Headers, nil
}

func (p *P2PSyncProtocol) sendError(w http.ResponseWriter, reason string, statusCode int) {
	errMsg := NotFoundMessage{
		Hashes: []string{},
		Reason: reason,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(errMsg)

	log.Printf("[P2P Sync] Error response: %s (status=%d)", reason, statusCode)
}

// RegisterRoutes registers HTTP handlers
func (p *P2PSyncProtocol) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/sync/getblocks", p.HandleGetBlocksMessage)
	log.Printf("[P2P Sync] Registered routes: /sync/getblocks")
}

// GetPeerAddresses returns list of available peer addresses
func GetPeerAddresses(pm PeerAPI) []string {
	if pm == nil {
		return []string{}
	}

	peers := pm.Peers()
	addresses := make([]string, 0, len(peers))
	for _, peer := range peers {
		addresses = append(addresses, peer)
	}

	return addresses
}

// SelectBestPeer selects the best peer based on scoring algorithm
// Uses AdvancedPeerScorer.GetTopPeersByScore() to get highest-scoring peer
func (p *P2PSyncProtocol) SelectBestPeer(ctx context.Context, peerAddrs []string) (string, error) {
	if len(peerAddrs) == 0 {
		return "", fmt.Errorf("no peers available")
	}

	if p.scorer == nil {
		return peerAddrs[0], nil
	}

	topPeers := p.scorer.GetTopPeersByScore(len(peerAddrs))
	if len(topPeers) == 0 {
		return peerAddrs[0], nil
	}

	for _, peer := range topPeers {
		for _, addr := range peerAddrs {
			if addr == peer {
				return addr, nil
			}
		}
	}

	return peerAddrs[0], nil
}

// RecordSyncSuccess records successful sync operation for peer scoring
func (p *P2PSyncProtocol) RecordSyncSuccess(peerAddr string, latencyMs int64) {
	if p.scorer != nil {
		p.scorer.RecordSuccess(peerAddr, latencyMs)
	}
}

// RecordSyncFailure records failed sync operation for peer scoring
func (p *P2PSyncProtocol) RecordSyncFailure(peerAddr string) {
	if p.scorer != nil {
		p.scorer.RecordFailure(peerAddr)
	}
}

// BatchDownloadBlocks downloads blocks in parallel from multiple peers with scoring
func (p *P2PSyncProtocol) BatchDownloadBlocks(ctx context.Context, peerAddrs []string, startHeight uint64, endHeight uint64) ([]*core.Block, error) {
	if len(peerAddrs) == 0 {
		return nil, fmt.Errorf("no peers available")
	}

	log.Printf("[P2P Sync] Batch downloading blocks %d-%d from %d peers",
		startHeight, endHeight, len(peerAddrs))

	batchSize := uint64(p.maxBlocksPerRequest)
	var batches [][]uint64

	for h := startHeight; h <= endHeight; h += batchSize {
		batchEnd := h + batchSize - 1
		if batchEnd > endHeight {
			batchEnd = endHeight
		}

		batch := make([]uint64, 0, batchSize)
		for i := h; i <= batchEnd; i++ {
			batch = append(batch, i)
		}
		batches = append(batches, batch)
	}

	allBlocks := make([]*core.Block, 0, endHeight-startHeight+1)
	blocksChan := make(chan []*core.Block, len(batches))
	errChan := make(chan error, len(batches))

	for i, batch := range batches {
		var peerAddr string
		var err error

		if p.scorer != nil {
			peerAddr, err = p.SelectBestPeer(ctx, peerAddrs)
			if err != nil {
				peerAddr = peerAddrs[i%len(peerAddrs)]
			}
		} else {
			peerAddr = peerAddrs[i%len(peerAddrs)]
		}

		go func(batchHeights []uint64, peer string, batchIdx int) {
			startTime := time.Now()
			parentHash := ""
			if len(batchHeights) > 0 && batchHeights[0] > 0 {
				parentBlock, ok := p.bc.BlockByHeight(batchHeights[0] - 1)
				if ok && parentBlock != nil {
					consensusParams := p.bc.GetConsensus()
					hash, _ := core.BlockHashHex(parentBlock, consensusParams)
					parentHash = hash
				}
			}

			blocksMsg, err := p.RequestBlocks(ctx, peer, parentHash, len(batchHeights), false)
			latencyMs := time.Since(startTime).Milliseconds()

			if err != nil {
				p.RecordSyncFailure(peer)
				errChan <- err
				return
			}

			p.RecordSyncSuccess(peer, latencyMs)
			blocksChan <- blocksMsg.Blocks
		}(batch, peerAddr, i)
	}

	for i := 0; i < len(batches); i++ {
		select {
		case blocks := <-blocksChan:
			allBlocks = append(allBlocks, blocks...)
		case err := <-errChan:
			log.Printf("[P2P Sync] Batch download error: %v", err)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	log.Printf("[P2P Sync] Batch download completed: %d blocks", len(allBlocks))

	return allBlocks, nil
}
