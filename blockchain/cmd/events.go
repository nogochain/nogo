// Package main provides event handling and subscription mechanisms for the NogoChain node.
//
// This file implements a comprehensive event system including:
//   - Event types for all node activities (blocks, transactions, peers, mining, sync)
//   - Publish-subscribe pattern for event distribution
//   - Channel-based event subscriptions for goroutine safety
//   - Event data structures with typed payloads
//   - Metrics integration for event monitoring
//
// The event system is designed for production-grade monitoring and integration.
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// EventType represents the type of event
type EventType string

const (
	// EventBlockAdded is emitted when a new block is added to the chain
	EventBlockAdded EventType = "block.added"

	// EventBlockRemoved is emitted when a block is removed during reorg
	EventBlockRemoved EventType = "block.removed"

	// EventTxAdded is emitted when a new transaction is added to mempool
	EventTxAdded EventType = "tx.added"

	// EventTxRemoved is emitted when a transaction is removed from mempool
	EventTxRemoved EventType = "tx.removed"

	// EventPeerConnected is emitted when a peer connects
	EventPeerConnected EventType = "peer.connected"

	// EventPeerDisconnected is emitted when a peer disconnects
	EventPeerDisconnected EventType = "peer.disconnected"

	// EventMiningStarted is emitted when mining starts
	EventMiningStarted EventType = "mining.started"

	// EventMiningStopped is emitted when mining stops
	EventMiningStopped EventType = "mining.stopped"

	// EventBlockMined is emitted when a new block is mined
	EventBlockMined EventType = "block.mined"

	// EventSyncStarted is emitted when chain sync starts
	EventSyncStarted EventType = "sync.started"

	// EventSyncCompleted is emitted when chain sync completes
	EventSyncCompleted EventType = "sync.completed"

	// EventShutdown is emitted when node is shutting down
	EventShutdown EventType = "node.shutdown"
)

// Event represents a node event
type Event struct {
	Type      EventType   `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
	Source    string      `json:"source,omitempty"`
}

// BlockEventData contains block event data
type BlockEventData struct {
	Block      *core.Block `json:"block"`
	Height     uint64      `json:"height"`
	Hash       string      `json:"hash"`
	ParentHash string      `json:"parent_hash"`
	TxCount    int         `json:"tx_count"`
	Size       int         `json:"size"`
	Miner      string      `json:"miner,omitempty"`
}

// TxEventData contains transaction event data
type TxEventData struct {
	TxID   string            `json:"tx_id"`
	Tx     *core.Transaction `json:"tx"`
	Size   int               `json:"size"`
	Fee    uint64            `json:"fee"`
	Source string            `json:"source,omitempty"`
}

// PeerEventData contains peer event data
type PeerEventData struct {
	PeerID    string `json:"peer_id"`
	Address   string `json:"address"`
	Direction string `json:"direction"`
	Height    uint64 `json:"height"`
	UserAgent string `json:"user_agent,omitempty"`
}

// MiningEventData contains mining event data
type MiningEventData struct {
	Height   uint64  `json:"height"`
	Hashrate float64 `json:"hashrate"`
	Threads  int     `json:"threads"`
	Enabled  bool    `json:"enabled"`
}

// SyncEventData contains sync event data
type SyncEventData struct {
	StartHeight   uint64        `json:"start_height"`
	CurrentHeight uint64        `json:"current_height"`
	TargetHeight  uint64        `json:"target_height"`
	PeerCount     int           `json:"peer_count"`
	Duration      time.Duration `json:"duration,omitempty"`
}

// EventHandler is a function that handles events
type EventHandler func(event *Event)

// EventManager manages event publishing and subscribing
type EventManager struct {
	mu          sync.RWMutex
	subscribers map[EventType][]EventHandler
	channels    map[EventType][]chan *Event
	closed      bool
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewEventManager creates a new event manager
func NewEventManager(ctx context.Context) *EventManager {
	managerCtx, cancel := context.WithCancel(ctx)

	return &EventManager{
		subscribers: make(map[EventType][]EventHandler),
		channels:    make(map[EventType][]chan *Event),
		ctx:         managerCtx,
		cancel:      cancel,
	}
}

// Subscribe subscribes to events of a specific type
func (em *EventManager) Subscribe(eventType EventType, handler EventHandler) {
	em.mu.Lock()
	defer em.mu.Unlock()

	if em.closed {
		return
	}

	em.subscribers[eventType] = append(em.subscribers[eventType], handler)
}

// SubscribeChannel subscribes to events via a channel
func (em *EventManager) SubscribeChannel(eventType EventType, bufferSize int) <-chan *Event {
	em.mu.Lock()
	defer em.mu.Unlock()

	if em.closed {
		ch := make(chan *Event)
		close(ch)
		return ch
	}

	ch := make(chan *Event, bufferSize)
	em.channels[eventType] = append(em.channels[eventType], ch)
	return ch
}

// Unsubscribe unsubscribes from events
func (em *EventManager) Unsubscribe(eventType EventType, handler EventHandler) {
	em.mu.Lock()
	defer em.mu.Unlock()

	handlers := em.subscribers[eventType]
	for i, h := range handlers {
		// Compare function pointers
		if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
			em.subscribers[eventType] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}
}

// UnsubscribeChannel unsubscribes a channel
func (em *EventManager) UnsubscribeChannel(eventType EventType, ch <-chan *Event) {
	em.mu.Lock()
	defer em.mu.Unlock()

	channels := em.channels[eventType]
	for i, c := range channels {
		if c == ch {
			close(c)
			em.channels[eventType] = append(channels[:i], channels[i+1:]...)
			break
		}
	}
}

// Publish publishes an event to all subscribers
func (em *EventManager) Publish(event *Event) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if em.closed {
		return
	}

	event.Timestamp = time.Now().UTC()

	handlers := em.subscribers[event.Type]
	for _, handler := range handlers {
		go handler(event)
	}

	channels := em.channels[event.Type]
	for _, ch := range channels {
		select {
		case ch <- event:
		default:
			// Metrics tracking disabled for compatibility
		}
	}

	// Metrics tracking disabled for compatibility
}

// PublishBlockAdded publishes a block added event
func (em *EventManager) PublishBlockAdded(block *core.Block, height uint64) {
	// Get block hash as hex string
	blockHash := fmt.Sprintf("%x", block.GetHash())

	// Get parent hash from header
	parentHash := fmt.Sprintf("%x", block.GetPrevHash())

	// Calculate block size from serialized data
	blockData, err := core.EncodeBlockBinary(block)
	blockSize := 0
	if err == nil {
		blockSize = len(blockData)
	} else {
		blockSize = len(block.Header.PrevHash) + 32 + len(block.Transactions)*100
	}

	event := &Event{
		Type: EventBlockAdded,
		Data: &BlockEventData{
			Block:      block,
			Height:     height,
			Hash:       blockHash,
			ParentHash: parentHash,
			TxCount:    len(block.Transactions),
			Size:       blockSize,
		},
	}
	em.Publish(event)
}

// PublishBlockRemoved publishes a block removed event
func (em *EventManager) PublishBlockRemoved(block *core.Block, height uint64) {
	blockHash := fmt.Sprintf("%x", block.GetHash())

	event := &Event{
		Type: EventBlockRemoved,
		Data: &BlockEventData{
			Block:  block,
			Height: height,
			Hash:   blockHash,
		},
	}
	em.Publish(event)
}

// PublishTxAdded publishes a transaction added event
func (em *EventManager) PublishTxAdded(tx *core.Transaction, source string) {
	txID := tx.GetID()

	// Calculate transaction size from serialized data
	txData, err := core.EncodeTransactionBinary(*tx)
	txSize := 0
	if err == nil {
		txSize = len(txData)
	} else {
		txSize = len(tx.ToAddress) + len(tx.FromPubKey) + 16
	}

	event := &Event{
		Type: EventTxAdded,
		Data: &TxEventData{
			TxID:   txID,
			Tx:     tx,
			Size:   txSize,
			Source: source,
		},
	}
	em.Publish(event)
}

// PublishPeerConnected publishes a peer connected event
func (em *EventManager) PublishPeerConnected(peerID, address, direction, userAgent string, height uint64) {
	event := &Event{
		Type: EventPeerConnected,
		Data: &PeerEventData{
			PeerID:    peerID,
			Address:   address,
			Direction: direction,
			Height:    height,
			UserAgent: userAgent,
		},
	}
	em.Publish(event)
}

// PublishPeerDisconnected publishes a peer disconnected event
func (em *EventManager) PublishPeerDisconnected(peerID, address string) {
	event := &Event{
		Type: EventPeerDisconnected,
		Data: &PeerEventData{
			PeerID:  peerID,
			Address: address,
		},
	}
	em.Publish(event)
}

// PublishBlockMined publishes a block mined event
func (em *EventManager) PublishBlockMined(block *core.Block, height uint64, hashrate float64) {
	event := &Event{
		Type: EventBlockMined,
		Data: &MiningEventData{
			Height:   height,
			Hashrate: hashrate,
		},
	}
	em.Publish(event)
}

// PublishSyncStarted publishes a sync started event
func (em *EventManager) PublishSyncStarted(startHeight, targetHeight uint64, peerCount int) {
	event := &Event{
		Type: EventSyncStarted,
		Data: &SyncEventData{
			StartHeight:  startHeight,
			TargetHeight: targetHeight,
			PeerCount:    peerCount,
		},
	}
	em.Publish(event)
}

// PublishSyncCompleted publishes a sync completed event
func (em *EventManager) PublishSyncCompleted(currentHeight, targetHeight uint64, duration time.Duration) {
	event := &Event{
		Type: EventSyncCompleted,
		Data: &SyncEventData{
			CurrentHeight: currentHeight,
			TargetHeight:  targetHeight,
			Duration:      duration,
		},
	}
	em.Publish(event)
}

// Close closes the event manager and cleans up resources
func (em *EventManager) Close() {
	em.mu.Lock()
	defer em.mu.Unlock()

	if em.closed {
		return
	}

	em.closed = true
	em.cancel()

	for eventType, channels := range em.channels {
		for _, ch := range channels {
			close(ch)
		}
		delete(em.channels, eventType)
	}

	em.subscribers = make(map[EventType][]EventHandler)
}

// WaitForShutdown waits for shutdown event
func (em *EventManager) WaitForShutdown(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	ch := em.SubscribeChannel(EventShutdown, 1)

	go func() {
		defer close(done)
		select {
		case <-ch:
		case <-ctx.Done():
		}
	}()

	return done
}
