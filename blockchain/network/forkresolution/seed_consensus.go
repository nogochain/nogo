// Copyright 2026 NogoChain Team
// Production-grade seed-to-seed fast consensus engine
// Prevents network partition by requiring inter-seed confirmation
// before blocks are locally finalized in the canonical chain

package forkresolution

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

const (
	// SyncMsgSeedVoteWire is the byte identifier for a seed consensus vote message
	// on the wire. This MUST match reactor.SyncMsgSeedVote (0x0A).
	SyncMsgSeedVoteWire byte = 0x0A

	// MinSeedConfirmations is the minimum number of peer seed confirmations
	// required before a seed node will finalize a newly received block.
	// With default 3 seed nodes (A,B,C), requiring 2 confirmations means
	// BOTH other seeds must agree, preventing any single seed bias.
	// Deploy value: 2 (requires 2 confirmations from peer seeds)
	MinSeedConfirmations = 2

	// MaxSeedConsensusWait is the maximum time a seed node will wait for
	// peer seed confirmations before falling back to local-only processing.
	// This prevents stalled consensus when some seeds are temporarily unreachable.
	// 500ms is aggressive: short enough to not delay block propagation,
	// long enough to gather confirmations from well-connected seeds.
	MaxSeedConsensusWait = 500 * time.Millisecond

	// SeedVoteExpiry is how long vote records are kept for garbage collection.
	SeedVoteExpiry = 30 * time.Second
)

// PendingBlock tracks a block awaiting seed consensus.
type PendingBlock struct {
	Block         *core.Block
	ArrivedAt     time.Time
	HashHex       string
	Confirmations int
	VoterSet      map[string]struct{}
	DoneCh        chan struct{}
}

// SeedConsensusEngine implements pre-consensus voting among seed nodes.
// Before a seed node finalizes a newly arrived block, it broadcasts a vote
// to all other known seed peers and waits for >=MinSeedConfirmations replies.
// This prevents the "first-seen bias" problem where geographically distributed
// seeds favor different miners, forming persistent fork networks.
//
// Architecture:
//
//	SeedA --Block X--> SeedB --Block X--> SeedC
//	   |                    |                    |
//	   +--Vote(X)--> SeedB -+--Vote(X)--> SeedC
//	   |                    |                    |
//	   +--Vote(X)--> SeedC -+--Vote(X)--> SeedB
//	   |                    |                    |
//	   <--Seeds reach quorum, X is finalized--->
//
// If fewer than MinPeerSeeds are connected, the engine falls back to
// immediate local processing without waiting.
type SeedConsensusEngine struct {
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	pending        map[string]*PendingBlock
	connectedSeeds map[string]bool
	voteDispatcher SeedVoteDispatcher
	isSeedMode     bool
	minConfirm     int
	maxWait        time.Duration
	minPeerSeeds   int
	localPeerID    string
}

// SeedVotePayload is the consensus vote message exchanged between seed nodes.
type SeedVotePayload struct {
	BlockHashHex string `json:"block_hash"`
	BlockHeight  uint64 `json:"block_height"`
	VoterID      string `json:"voter_id"`
	VotedAt      int64  `json:"voted_at"`
}

// SeedVoteDispatcher abstracts the P2P message sending for votes.
type SeedVoteDispatcher interface {
	BroadcastSyncMsg(msg []byte)
}

// NewSeedConsensusEngine creates a seed consensus engine.
func NewSeedConsensusEngine(ctx context.Context, dispatcher SeedVoteDispatcher, isSeedMode bool) *SeedConsensusEngine {
	childCtx, cancel := context.WithCancel(ctx)
	engine := &SeedConsensusEngine{
		ctx:            childCtx,
		cancel:         cancel,
		pending:        make(map[string]*PendingBlock),
		connectedSeeds: make(map[string]bool),
		voteDispatcher: dispatcher,
		isSeedMode:     isSeedMode,
		minConfirm:     MinSeedConfirmations,
		maxWait:        MaxSeedConsensusWait,
		minPeerSeeds:   MinSeedConfirmations,
	}
	go engine.startCleanupLoop()
	return engine
}

// SetLocalPeerID sets the local node's peer identifier used in vote messages.
func (se *SeedConsensusEngine) SetLocalPeerID(id string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.localPeerID = id
}

// SetMinConfirmations overrides the default minimum confirmation count.
func (se *SeedConsensusEngine) SetMinConfirmations(n int) {
	se.mu.Lock()
	defer se.mu.Unlock()
	if n > 0 {
		se.minConfirm = n
	}
}

// SetMaxWait overrides the default maximum consensus wait timeout.
func (se *SeedConsensusEngine) SetMaxWait(d time.Duration) {
	se.mu.Lock()
	defer se.mu.Unlock()
	if d > 0 {
		se.maxWait = d
	}
}

// UpdateSeedPeer marks a peer as a seed node or removes it.
func (se *SeedConsensusEngine) UpdateSeedPeer(peerID string, connected bool) {
	se.mu.Lock()
	defer se.mu.Unlock()
	if connected {
		se.connectedSeeds[peerID] = true
	} else {
		delete(se.connectedSeeds, peerID)
	}
}

// ConnectedSeedCount returns how many peer seeds are currently connected.
func (se *SeedConsensusEngine) ConnectedSeedCount() int {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return len(se.connectedSeeds)
}

// RequestConsensus submits a block for seed pre-consensus voting.
// Returns true if consensus is reached (or not required for non-seed nodes).
func (se *SeedConsensusEngine) RequestConsensus(block *core.Block) bool {
	if block == nil {
		return false
	}

	se.mu.RLock()
	isSeed := se.isSeedMode
	seedCount := len(se.connectedSeeds)
	localID := se.localPeerID
	se.mu.RUnlock()

	if !isSeed {
		return true
	}

	hashHex := hex.EncodeToString(block.Hash)
	hashPreview := hashHex
	if len(hashPreview) > 16 {
		hashPreview = hashPreview[:16]
	}

	log.Printf("[SeedConsensus] Requesting consensus for block %d hash=%s seeds=%d minConfirm=%d",
		block.GetHeight(), hashPreview, seedCount, se.minConfirm)

	if seedCount < se.minPeerSeeds {
		log.Printf("[SeedConsensus] Insufficient seeds (%d < %d), accepting block %d immediately",
			seedCount, se.minPeerSeeds, block.GetHeight())
		return true
	}

	pb := &PendingBlock{
		Block:         block,
		ArrivedAt:     time.Now(),
		HashHex:       hashHex,
		Confirmations: 0,
		VoterSet:      make(map[string]struct{}),
		DoneCh:        make(chan struct{}),
	}

	se.mu.Lock()
	se.pending[hashHex] = pb
	se.mu.Unlock()

	se.broadcastVote(block, localID)

	select {
	case <-pb.DoneCh:
		confirmed := pb.Confirmations >= se.minConfirm
		log.Printf("[SeedConsensus] Block %d hash=%s consensus: confirmations=%d required=%d result=%v",
			block.GetHeight(), hashPreview, pb.Confirmations, se.minConfirm, confirmed)
		se.removePending(hashHex)
		return confirmed
	case <-time.After(se.maxWait):
		se.mu.RLock()
		currentConfirm := 0
		if p, exists := se.pending[hashHex]; exists {
			currentConfirm = p.Confirmations
		}
		se.mu.RUnlock()
		log.Printf("[SeedConsensus] Block %d hash=%s TIMEOUT after %v confirmations=%d, accepting by fallback",
			block.GetHeight(), hashPreview, se.maxWait, currentConfirm)
		se.removePending(hashHex)
		return true
	}
}

// ReceiveVote handles an incoming consensus vote from a peer seed.
func (se *SeedConsensusEngine) ReceiveVote(vote SeedVotePayload) {
	se.mu.Lock()
	defer se.mu.Unlock()

	pb, exists := se.pending[vote.BlockHashHex]
	if !exists {
		return
	}

	if _, alreadyVoted := pb.VoterSet[vote.VoterID]; alreadyVoted {
		return
	}

	pb.VoterSet[vote.VoterID] = struct{}{}
	pb.Confirmations++

	voterPreview := vote.VoterID
	if len(voterPreview) > 16 {
		voterPreview = voterPreview[:16]
	}
	log.Printf("[SeedConsensus] Received vote from %s for block %s (confirmations=%d/%d)",
		voterPreview, vote.BlockHashHex[:16], pb.Confirmations, se.minConfirm)

	if pb.Confirmations >= se.minConfirm {
		close(pb.DoneCh)
	}
}

func (se *SeedConsensusEngine) broadcastVote(block *core.Block, localPeerID string) {
	payload := SeedVotePayload{
		BlockHashHex: hex.EncodeToString(block.Hash),
		BlockHeight:  block.GetHeight(),
		VoterID:      localPeerID,
		VotedAt:      time.Now().UnixNano(),
	}

	msg := BuildSeedVoteMsg(payload)
	if msg == nil {
		log.Printf("[SeedConsensus] Failed to build vote message for block %d", block.GetHeight())
		return
	}

	se.mu.RLock()
	dispatcher := se.voteDispatcher
	se.mu.RUnlock()

	if dispatcher != nil {
		dispatcher.BroadcastSyncMsg(msg)
	}
}

// IsPending checks if a block is still awaiting consensus.
func (se *SeedConsensusEngine) IsPending(hashHex string) bool {
	se.mu.RLock()
	defer se.mu.RUnlock()
	_, exists := se.pending[hashHex]
	return exists
}

// SetSeedMode dynamically enables or disables seed consensus mode.
func (se *SeedConsensusEngine) SetSeedMode(active bool) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.isSeedMode = active
}

func (se *SeedConsensusEngine) removePending(hashHex string) {
	pb, exists := se.pending[hashHex]
	if !exists {
		return
	}
	select {
	case <-pb.DoneCh:
	default:
		close(pb.DoneCh)
	}
	delete(se.pending, hashHex)
}

func (se *SeedConsensusEngine) startCleanupLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-se.ctx.Done():
			return
		case <-ticker.C:
			se.cleanupExpired()
		}
	}
}

func (se *SeedConsensusEngine) cleanupExpired() {
	se.mu.Lock()
	defer se.mu.Unlock()
	expiry := time.Now().Add(-SeedVoteExpiry)
	for hashHex, pb := range se.pending {
		if pb.ArrivedAt.Before(expiry) {
			close(pb.DoneCh)
			delete(se.pending, hashHex)
		}
	}
}

// Stop shuts down the engine and releases all pending blocks.
func (se *SeedConsensusEngine) Stop() {
	se.cancel()
	se.mu.Lock()
	defer se.mu.Unlock()
	for hashHex, pb := range se.pending {
		select {
		case <-pb.DoneCh:
		default:
			close(pb.DoneCh)
		}
		delete(se.pending, hashHex)
	}
}

// BuildSeedVoteMsg serializes a SeedVotePayload into a P2P message byte slice.
func BuildSeedVoteMsg(vote SeedVotePayload) []byte {
	data, err := json.Marshal(vote)
	if err != nil {
		log.Printf("[SeedConsensus] Marshal vote failed: %v", err)
		return nil
	}
	msg := make([]byte, 1+len(data))
	msg[0] = SyncMsgSeedVoteWire
	copy(msg[1:], data)
	return msg
}

// ParseSeedVoteMsg deserializes a SeedVotePayload from a message byte slice.
func ParseSeedVoteMsg(data []byte) (*SeedVotePayload, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("seed vote message too short: %d bytes", len(data))
	}
	if data[0] != SyncMsgSeedVoteWire {
		return nil, fmt.Errorf("invalid message type: 0x%02x", data[0])
	}
	var vote SeedVotePayload
	if err := json.Unmarshal(data[1:], &vote); err != nil {
		return nil, fmt.Errorf("unmarshal seed vote: %w", err)
	}
	if vote.BlockHashHex == "" {
		return nil, fmt.Errorf("seed vote missing block hash")
	}
	return &vote, nil
}
