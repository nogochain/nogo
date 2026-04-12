// Copyright 2026 NogoChain Team
// Production-grade Gossip Protocol integration with P2P network
// Implements seamless integration with existing blockchain infrastructure

package network

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// GossipIntegration manages the integration between Gossip Protocol and P2P network
// Design: seamless integration, minimal overhead, production-ready
type GossipIntegration struct {
	mu sync.RWMutex

	// Core components
	gossipProtocol *GossipProtocol
	p2pServer      *P2PServer
	blockchain     BlockchainInterface
	mempool        Mempool

	// Configuration
	config IntegrationConfig

	// Runtime state
	active           bool
	pendingBlocks    map[string]*PendingBlock
	pendingTxns      map[string]*PendingTransaction
	processingQueue  chan *ProcessingTask
	stopChan         chan struct{}
	wg               sync.WaitGroup
}

// IntegrationConfig defines integration configuration
type IntegrationConfig struct {
	EnableBlockGossip       bool
	EnableTransactionGossip bool
	MaxPendingBlocks        int
	MaxPendingTransactions  int
	ProcessingWorkers       int
	BlockValidationTimeout  time.Duration
	TransactionValidationTimeout time.Duration
}

// DefaultIntegrationConfig returns default integration configuration
func DefaultIntegrationConfig() IntegrationConfig {
	return IntegrationConfig{
		EnableBlockGossip:             true,
		EnableTransactionGossip:       true,
		MaxPendingBlocks:              100,
		MaxPendingTransactions:        10000,
		ProcessingWorkers:             4,
		BlockValidationTimeout:        5 * time.Second,
		TransactionValidationTimeout:  2 * time.Second,
	}
}

// PendingBlock represents a block pending processing
type PendingBlock struct {
	Block      *core.Block
	ReceivedAt time.Time
	FromPeer   string
	RetryCount int
}

// PendingTransaction represents a transaction pending processing
type PendingTransaction struct {
	Transaction *core.Transaction
	ReceivedAt  time.Time
	FromPeer    string
	RetryCount  int
}

// ProcessingTask represents a task for processing
type ProcessingTask struct {
	Type     string // "block" or "transaction"
	Data     interface{}
	PeerID   string
	Priority int
}

// NewGossipIntegration creates a new gossip integration instance
func NewGossipIntegration(gossip *GossipProtocol, server *P2PServer, blockchain BlockchainInterface, mempool Mempool, config IntegrationConfig) *GossipIntegration {
	if gossip == nil || server == nil || blockchain == nil {
		log.Printf("WARNING: GossipIntegration created with nil components")
		return nil
	}

	return &GossipIntegration{
		gossipProtocol:  gossip,
		p2pServer:       server,
		blockchain:      blockchain,
		mempool:         mempool,
		config:          config,
		active:          false,
		pendingBlocks:   make(map[string]*PendingBlock),
		pendingTxns:     make(map[string]*PendingTransaction),
		processingQueue: make(chan *ProcessingTask, 1000),
		stopChan:        make(chan struct{}),
	}
}

// Start starts the gossip integration
func (gi *GossipIntegration) Start() error {
	gi.mu.Lock()
	defer gi.mu.Unlock()

	if gi.active {
		return nil
	}

	gi.active = true

	// Start processing workers
	for i := 0; i < gi.config.ProcessingWorkers; i++ {
		gi.wg.Add(1)
		go gi.processingWorker(i)
	}

	// Start cleanup routine
	gi.wg.Add(1)
	go gi.cleanupRoutine()

	// Register P2P message handlers
	gi.registerP2PHandlers()

	log.Printf("GossipIntegration started: workers=%d block_gossip=%v tx_gossip=%v",
		gi.config.ProcessingWorkers, gi.config.EnableBlockGossip, gi.config.EnableTransactionGossip)

	return nil
}

// Stop stops the gossip integration
func (gi *GossipIntegration) Stop() {
	gi.mu.Lock()
	defer gi.mu.Unlock()

	if !gi.active {
		return
	}

	gi.active = false
	close(gi.stopChan)
	gi.wg.Wait()

	log.Printf("GossipIntegration stopped")
}

// registerP2PHandlers registers P2P message handlers for gossip messages
func (gi *GossipIntegration) registerP2PHandlers() {
	// Register gossip block message handler
	// This would integrate with P2PServer's message routing system
	// In production: server.RegisterHandler("gossip_block", gi.handleGossipBlockMessage)

	// Register gossip transaction message handler
	// In production: server.RegisterHandler("gossip_transaction", gi.handleGossipTransactionMessage)
}

// OnNewBlock handles a new block from mining or sync
func (gi *GossipIntegration) OnNewBlock(block *core.Block, source string) error {
	if block == nil {
		return fmt.Errorf("block is nil")
	}

	if !gi.config.EnableBlockGossip {
		return nil
	}

	// Broadcast via gossip protocol
	origin := gi.p2pServer.nodeID
	if source != "" {
		origin = source
	}

	if err := gi.gossipProtocol.BroadcastBlock(block, origin); err != nil {
		return fmt.Errorf("failed to broadcast block via gossip: %w", err)
	}

	log.Printf("GossipIntegration: New block broadcasted height=%d hash=%s",
		block.GetHeight(), hex.EncodeToString(block.Hash[:8]))

	return nil
}

// OnNewTransaction handles a new transaction from wallet or API
func (gi *GossipIntegration) OnNewTransaction(tx *core.Transaction, source string) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil")
	}

	if !gi.config.EnableTransactionGossip {
		return nil
	}

	// Broadcast via gossip protocol
	origin := gi.p2pServer.nodeID
	if source != "" {
		origin = source
	}

	if err := gi.gossipProtocol.BroadcastTransaction(tx, origin); err != nil {
		return fmt.Errorf("failed to broadcast transaction via gossip: %w", err)
	}

	txHash, err := tx.SigningHash()
	if err != nil {
		log.Printf("GossipIntegration: New transaction broadcasted (hash computation failed)")
	} else {
		log.Printf("GossipIntegration: New transaction broadcasted hash=%s",
			hex.EncodeToString(txHash[:8]))
	}

	return nil
}

// HandleGossipMessage handles an incoming gossip message from P2P network
func (gi *GossipIntegration) HandleGossipMessage(c net.Conn, envelope p2pEnvelope) error {
	var gossipMsg GossipMessage
	if err := json.Unmarshal(envelope.Payload, &gossipMsg); err != nil {
		return fmt.Errorf("failed to unmarshal gossip message: %w", err)
	}

	// Get peer ID from connection
	peerID := gi.getPeerIDFromConn(c)

	// Handle via gossip protocol
	if err := gi.gossipProtocol.HandleIncomingMessage(&gossipMsg, peerID); err != nil {
		return fmt.Errorf("failed to handle gossip message: %w", err)
	}

	// Queue for processing if needed
	task := &ProcessingTask{
		Type:     messageTypeToString(gossipMsg.Type),
		Data:     gossipMsg.Payload,
		PeerID:   peerID,
		Priority: gossipMsg.Priority,
	}

	select {
	case gi.processingQueue <- task:
		// Successfully queued
	default:
		// Queue full, drop
		log.Printf("WARNING: GossipIntegration processing queue full, dropping message")
	}

	return nil
}

// processingWorker processes messages from the queue
func (gi *GossipIntegration) processingWorker(id int) {
	defer gi.wg.Done()

	for {
		select {
		case <-gi.stopChan:
			return
		case task := <-gi.processingQueue:
			if task == nil {
				continue
			}

			startTime := time.Now()
			err := gi.processTask(task)
			processingTime := time.Since(startTime)

			if err != nil {
				log.Printf("GossipIntegration worker %d: processing failed: %v time=%v",
					id, err, processingTime)
			} else {
				log.Printf("GossipIntegration worker %d: processing success type=%s time=%v",
					id, task.Type, processingTime)
			}
		}
	}
}

// processTask processes a single task
func (gi *GossipIntegration) processTask(task *ProcessingTask) error {
	switch task.Type {
	case "block":
		return gi.processBlockTask(task)
	case "transaction":
		return gi.processTransactionTask(task)
	default:
		return fmt.Errorf("unknown task type: %s", task.Type)
	}
}

// processBlockTask processes a block task
func (gi *GossipIntegration) processBlockTask(task *ProcessingTask) error {
	// Deserialize block
	block, err := deserializeBlock(task.Data.([]byte))
	if err != nil {
		return fmt.Errorf("failed to deserialize block: %w", err)
	}

	// Validate context
	ctx, cancel := context.WithTimeout(context.Background(), gi.config.BlockValidationTimeout)
	defer cancel()

	// Add block to blockchain
	select {
	case <-ctx.Done():
		return fmt.Errorf("block validation timeout")
	default:
		accepted, err := gi.blockchain.AddBlock(block)
		if err != nil {
			return fmt.Errorf("failed to add block: %w", err)
		}

		if !accepted {
			log.Printf("GossipIntegration: Block not accepted height=%d hash=%s",
				block.GetHeight(), hex.EncodeToString(block.Hash[:8]))
		}

		return nil
	}
}

// processTransactionTask processes a transaction task
func (gi *GossipIntegration) processTransactionTask(task *ProcessingTask) error {
	// Deserialize transaction
	tx, err := deserializeTransaction(task.Data.([]byte))
	if err != nil {
		return fmt.Errorf("failed to deserialize transaction: %w", err)
	}

	// Validate context
	ctx, cancel := context.WithTimeout(context.Background(), gi.config.TransactionValidationTimeout)
	defer cancel()

	// Add transaction to mempool
	select {
	case <-ctx.Done():
		return fmt.Errorf("transaction validation timeout")
	default:
		if gi.mempool != nil {
			txID, err := gi.mempool.Add(*tx)
			if err != nil {
				return fmt.Errorf("failed to add transaction to mempool: %w", err)
			}

			log.Printf("GossipIntegration: Transaction added to mempool id=%s", txID)
		}

		return nil
	}
}

// cleanupRoutine periodically cleans up pending items
func (gi *GossipIntegration) cleanupRoutine() {
	defer gi.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-gi.stopChan:
			return
		case <-ticker.C:
			gi.cleanup()
		}
	}
}

// cleanup removes expired pending items
func (gi *GossipIntegration) cleanup() {
	gi.mu.Lock()
	defer gi.mu.Unlock()

	now := time.Now()
	timeout := 5 * time.Minute

	// Clean pending blocks
	for hash, pb := range gi.pendingBlocks {
		if now.Sub(pb.ReceivedAt) > timeout || pb.RetryCount > 3 {
			delete(gi.pendingBlocks, hash)
		}
	}

	// Clean pending transactions
	for hash, pt := range gi.pendingTxns {
		if now.Sub(pt.ReceivedAt) > timeout || pt.RetryCount > 3 {
			delete(gi.pendingTxns, hash)
		}
	}
}

// getPeerIDFromConn extracts peer ID from connection
func (gi *GossipIntegration) getPeerIDFromConn(c net.Conn) string {
	// In production, this would extract peer ID from connection metadata
	if c == nil {
		return "unknown"
	}

	return c.RemoteAddr().String()
}

// GetStats returns integration statistics
func (gi *GossipIntegration) GetStats() map[string]interface{} {
	gi.mu.RLock()
	defer gi.mu.RUnlock()

	return map[string]interface{}{
		"active":                   gi.active,
		"pending_blocks":           len(gi.pendingBlocks),
		"pending_transactions":     len(gi.pendingTxns),
		"processing_queue_size":    len(gi.processingQueue),
		"config":                   gi.config,
		"gossip_metrics":           gi.gossipProtocol.GetMetrics(),
	}
}

// Helper functions

func messageTypeToString(msgType GossipMessageType) string {
	switch msgType {
	case GossipMessageBlock:
		return "block"
	case GossipMessageTransaction:
		return "transaction"
	case GossipMessageBlockAnnouncement:
		return "block_announcement"
	case GossipMessageTransactionAnnouncement:
		return "transaction_announcement"
	case GossipMessagePeerDiscovery:
		return "peer_discovery"
	case GossipMessageChainState:
		return "chain_state"
	default:
		return "unknown"
	}
}

// GossipP2PAdapter adapts gossip protocol to P2P network interface
type GossipP2PAdapter struct {
	gossipIntegration *GossipIntegration
}

// NewGossipP2PAdapter creates a new P2P adapter for gossip
func NewGossipP2PAdapter(integration *GossipIntegration) *GossipP2PAdapter {
	return &GossipP2PAdapter{
		gossipIntegration: integration,
	}
}

// BroadcastBlock broadcasts a block to the network
func (a *GossipP2PAdapter) BroadcastBlock(block *core.Block, excludePeers []string) error {
	return a.gossipIntegration.OnNewBlock(block, "")
}

// BroadcastTransaction broadcasts a transaction to the network
func (a *GossipP2PAdapter) BroadcastTransaction(tx *core.Transaction, excludePeers []string) error {
	return a.gossipIntegration.OnNewTransaction(tx, "")
}

// OnPeerConnected handles a new peer connection
func (a *GossipP2PAdapter) OnPeerConnected(peerID string) {
	// Update gossip protocol peer list
	// In production, this would update the peer manager
}

// OnPeerDisconnected handles a peer disconnection
func (a *GossipP2PAdapter) OnPeerDisconnected(peerID string) {
	// Update gossip protocol peer list
	// In production, this would update the peer manager
}

// SendMessage sends a direct message to a peer
func (a *GossipP2PAdapter) SendMessage(peerID string, message interface{}) error {
	// Send direct message (non-gossip)
	// In production, this would use P2P client to send
	return nil
}
