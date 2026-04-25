package reactor

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/network/mconnection"
)

// Sync message type constants.
// Each message on the sync channel starts with a single byte indicating
// the message type, followed by JSON-encoded payload.
const (
	// SyncMsgGetHeaders requests block headers from a peer.
	// Payload: JSON-encoded getHeadersPayload {from: uint64, count: uint64}.
	SyncMsgGetHeaders byte = 0x01

	// SyncMsgHeaders is the response to GetHeaders, containing block headers.
	// Payload: JSON-encoded headersPayload {headers: []byte, hasMore: bool}.
	SyncMsgHeaders byte = 0x02

	// SyncMsgGetBlocks requests full block bodies from a peer.
	// Payload: JSON-encoded getBlocksPayload {heights: []uint64}.
	SyncMsgGetBlocks byte = 0x03

	// SyncMsgBlocks is the response to GetBlocks, containing full blocks.
	// Payload: JSON-encoded blocksPayload {blocks: []json.RawMessage}.
	SyncMsgBlocks byte = 0x04

	// SyncMsgGetBlockLocator requests a compact block locator from a peer.
	// Payload: JSON-encoded getBlockLocatorPayload {tipHeight: uint64}.
	SyncMsgGetBlockLocator byte = 0x05

	// SyncMsgBlockLocator is the response containing a block locator (sparse hash list).
	// Payload: JSON-encoded blockLocatorPayload {locators: [][]byte}.
	SyncMsgBlockLocator byte = 0x06

	// SyncMsgStatus broadcasts node status (height, work, latest hash) to all peers.
	// Payload: JSON-encoded statusPayload {height: uint64, work: string, latestHash: string}.
	SyncMsgStatus byte = 0x07

	// SyncMsgNotFound indicates requested data is unavailable.
	// Payload: JSON-encoded notFoundPayload {msgType: byte, ids: []string}.
	SyncMsgNotFound byte = 0xFF
)

// Minimum message size: 1 byte for message type.
const syncMinMsgSize = 1

// SyncHandler defines the interface for sync-related business logic.
// This allows injecting the actual chain/sync implementation without
// creating circular dependencies between the reactor and core packages.
type SyncHandler interface {
	// OnGetHeaders handles a request for block headers starting from height.
	OnGetHeaders(peerID string, from uint64, count uint64) error

	// OnHeaders handles received block headers from a peer.
	OnHeaders(peerID string, headers []byte, hasMore bool) error

	// OnGetBlocks handles a request for full block bodies.
	OnGetBlocks(peerID string, heights []uint64) error

	// OnBlocks handles received full blocks from a peer.
	OnBlocks(peerID string, blocks []byte) error

	// OnGetBlockLocator handles a request for a compact block locator.
	OnGetBlockLocator(peerID string, tipHeight uint64) error

	// OnBlockLocator handles a received block locator from a peer.
	OnBlockLocator(peerID string, locators [][]byte) error

	// OnNotFound handles a not-found response for a prior request.
	OnNotFound(peerID string, msgType byte, ids []string) error

	// OnStatus handles received node status broadcast from a peer.
	OnStatus(peerID string, height uint64, work string, latestHash string) error
}

// SyncReactor handles blockchain synchronization protocol messages.
//
// It operates on the sync channel (ChannelSync) and manages the message
// parsing, validation, and dispatching to the injected SyncHandler.
//
// Thread-safety: handler access is protected by sync.RWMutex.
type SyncReactor struct {
	BaseReactor
	mu      sync.RWMutex
	handler SyncHandler
	// peerHandshakeTimeout defines the maximum duration for peer handshake.
	peerHandshakeTimeout time.Duration
	// handshakeCheckInterval defines the interval for checking handshake status.
	handshakeCheckInterval time.Duration
}

// NewSyncReactor creates a new SyncReactor with the given handler.
// Returns an error if handler is nil.
func NewSyncReactor(handler SyncHandler) (*SyncReactor, error) {
	if handler == nil {
		return nil, fmt.Errorf("sync reactor: handler must not be nil")
	}

	chs := []*mconnection.ChannelDescriptor{
		{
			ID:                  mconnection.ChannelSync,
			Priority:            5,
			SendQueueCapacity:   256,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	r := &SyncReactor{
		handler:                handler,
		peerHandshakeTimeout:   10 * time.Second,
		handshakeCheckInterval: 500 * time.Millisecond,
	}
	r.SetChannels(chs)

	return r, nil
}

// SetHandler replaces the sync handler.
// Use with caution; not safe during active message processing.
func (sr *SyncReactor) SetHandler(handler SyncHandler) error {
	if handler == nil {
		return fmt.Errorf("sync reactor: handler must not be nil")
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.handler = handler
	return nil
}

// AddPeer is called when a new peer connects on the sync channel.
// It performs a handshake with a 10-second timeout and 500ms check interval.
// Returns an error if handshake times out or fails.
func (sr *SyncReactor) AddPeer(peerID string, metadata map[string]string) error {
	if peerID == "" {
		return fmt.Errorf("sync reactor: peerID must not be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), sr.peerHandshakeTimeout)
	defer cancel()

	ticker := time.NewTicker(sr.handshakeCheckInterval)
	defer ticker.Stop()

	handshakeComplete := make(chan struct{})
	handshakeErr := make(chan error, 1)

	go func() {
		defer close(handshakeComplete)
		if err := sr.performHandshake(peerID, metadata); err != nil {
			select {
			case handshakeErr <- err:
			default:
			}
			return
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("sync reactor: peer handshake timeout after %v", sr.peerHandshakeTimeout)
		case <-ticker.C:
			select {
			case <-handshakeComplete:
				return nil
			default:
			}
		case err := <-handshakeErr:
			return fmt.Errorf("sync reactor: handshake failed: %w", err)
		}
	}
}

// performHandshake executes the actual handshake logic for a peer.
// This is a placeholder for the actual handshake implementation.
func (sr *SyncReactor) performHandshake(peerID string, metadata map[string]string) error {
	time.Sleep(100 * time.Millisecond)
	return nil
}

// RemovePeer is called when a peer disconnects.
// Currently no-op; override in subclass if peer-specific cleanup is needed.
func (sr *SyncReactor) RemovePeer(_ string, _ interface{}) {
}

// Receive parses and dispatches incoming sync channel messages.
//
// Message format: [1 byte msgType][JSON payload]
// The first byte determines the message type, and the remaining bytes
// are JSON-encoded data specific to that message type.
//
// Thread-safety: handler is accessed under read lock.
func (sr *SyncReactor) Receive(chID byte, peerID string, msgBytes []byte) {
	if peerID == "" {
		log.Printf("[SyncReactor] Receive: empty peerID, chID=%d, msgLen=%d", chID, len(msgBytes))
		return
	}
	if len(msgBytes) < syncMinMsgSize {
		log.Printf("[SyncReactor] Receive: message too short from peer %s, len=%d, minSize=%d", peerID, len(msgBytes), syncMinMsgSize)
		return
	}

	msgType := msgBytes[0]
	payload := msgBytes[syncMinMsgSize:]

	log.Printf("[SyncReactor] Receive: peer=%s, chID=%d, msgType=%d, payloadLen=%d", peerID, chID, msgType, len(payload))

	sr.mu.RLock()
	handler := sr.handler
	sr.mu.RUnlock()

	if handler == nil {
		log.Printf("[SyncReactor] Receive: handler is nil for peer %s", peerID)
		return
	}

	sr.dispatch(msgType, peerID, payload, handler)
}

// dispatch routes the parsed message to the appropriate handler method.
func (sr *SyncReactor) dispatch(msgType byte, peerID string, payload []byte, handler SyncHandler) {
	switch msgType {
	case SyncMsgGetHeaders:
		sr.handleGetHeaders(peerID, payload, handler)
	case SyncMsgHeaders:
		sr.handleHeaders(peerID, payload, handler)
	case SyncMsgGetBlocks:
		sr.handleGetBlocks(peerID, payload, handler)
	case SyncMsgBlocks:
		sr.handleBlocks(peerID, payload, handler)
	case SyncMsgGetBlockLocator:
		sr.handleGetBlockLocator(peerID, payload, handler)
	case SyncMsgBlockLocator:
		sr.handleBlockLocator(peerID, payload, handler)
	case SyncMsgStatus:
		sr.handleStatus(peerID, payload, handler)
	case SyncMsgNotFound:
		sr.handleNotFound(peerID, payload, handler)
	default:
		// Unknown message type - silently ignore to avoid amplifying
		// malformed or malicious traffic.
	}
}

// handleGetHeaders parses and dispatches a GetHeaders request.
func (sr *SyncReactor) handleGetHeaders(peerID string, payload []byte, handler SyncHandler) {
	if len(payload) == 0 {
		log.Printf("[SyncReactor] handleGetHeaders: empty payload from peer %s", peerID)
		return
	}

	var req getHeadersPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Printf("[SyncReactor] handleGetHeaders: failed to unmarshal payload from peer %s: %v", peerID, err)
		return
	}

	log.Printf("[SyncReactor] handleGetHeaders: peer=%s, from=%d, count=%d", peerID, req.From, req.Count)

	if err := handler.OnGetHeaders(peerID, req.From, req.Count); err != nil {
		log.Printf("[SyncReactor] handleGetHeaders: OnGetHeaders failed for peer %s: %v", peerID, err)
		return
	}
}

// handleHeaders parses and dispatches a Headers response.
func (sr *SyncReactor) handleHeaders(peerID string, payload []byte, handler SyncHandler) {
	if len(payload) == 0 {
		return
	}

	var resp headersPayload
	if err := json.Unmarshal(payload, &resp); err != nil {
		return
	}

	if resp.Headers == nil {
		resp.Headers = []byte{}
	}

	if err := handler.OnHeaders(peerID, resp.Headers, resp.HasMore); err != nil {
		return
	}
}

// handleGetBlocks parses and dispatches a GetBlocks request.
func (sr *SyncReactor) handleGetBlocks(peerID string, payload []byte, handler SyncHandler) {
	if len(payload) == 0 {
		return
	}

	var req getBlocksPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return
	}

	if req.Heights == nil {
		req.Heights = []uint64{}
	}

	handler.OnGetBlocks(peerID, req.Heights)
}

// handleBlocks parses and dispatches a Blocks response.
func (sr *SyncReactor) handleBlocks(peerID string, payload []byte, handler SyncHandler) {
	if len(payload) == 0 {
		return
	}

	handler.OnBlocks(peerID, payload)
}

// handleGetBlockLocator parses and dispatches a GetBlockLocator request.
func (sr *SyncReactor) handleGetBlockLocator(peerID string, payload []byte, handler SyncHandler) {
	if len(payload) == 0 {
		return
	}

	var req getBlockLocatorPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return
	}

	handler.OnGetBlockLocator(peerID, req.TipHeight)
}

// handleBlockLocator parses and dispatches a BlockLocator response.
func (sr *SyncReactor) handleBlockLocator(peerID string, payload []byte, handler SyncHandler) {
	if len(payload) == 0 {
		return
	}

	var resp blockLocatorPayload
	if err := json.Unmarshal(payload, &resp); err != nil {
		return
	}

	handler.OnBlockLocator(peerID, resp.Locators)
}

// handleNotFound parses and dispatches a NotFound response.
func (sr *SyncReactor) handleNotFound(peerID string, payload []byte, handler SyncHandler) {
	if len(payload) == 0 {
		return
	}

	var resp notFoundPayload
	if err := json.Unmarshal(payload, &resp); err != nil {
		return
	}

	if resp.IDs == nil {
		resp.IDs = []string{}
	}

	handler.OnNotFound(peerID, resp.MsgType, resp.IDs)
}

// handleStatus parses and dispatches a Status broadcast message.
func (sr *SyncReactor) handleStatus(peerID string, payload []byte, handler SyncHandler) {
	if len(payload) == 0 {
		return
	}

	var status struct {
		Height     uint64 `json:"height"`
		Work       string `json:"work"`
		LatestHash string `json:"latestHash"`
	}
	if err := json.Unmarshal(payload, &status); err != nil {
		return
	}

	handler.OnStatus(peerID, status.Height, status.Work, status.LatestHash)
}

// BuildGetHeadersMsg serializes a GetHeaders request message.
func BuildGetHeadersMsg(from uint64, count uint64) ([]byte, error) {
	req := getHeadersPayload{From: from, Count: count}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("build getHeaders message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = SyncMsgGetHeaders
	copy(msg[1:], payload)
	return msg, nil
}

// BuildHeadersMsg serializes a Headers response message.
func BuildHeadersMsg(headers []byte, hasMore bool) ([]byte, error) {
	resp := headersPayload{Headers: headers, HasMore: hasMore}
	payload, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("build headers message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = SyncMsgHeaders
	copy(msg[1:], payload)
	return msg, nil
}

// BuildGetBlocksMsg serializes a GetBlocks request message.
func BuildGetBlocksMsg(heights []uint64) ([]byte, error) {
	req := getBlocksPayload{Heights: heights}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("build getBlocks message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = SyncMsgGetBlocks
	copy(msg[1:], payload)
	return msg, nil
}

// BuildBlocksMsg serializes a Blocks response message.
func BuildBlocksMsg(blocks []byte) ([]byte, error) {
	if blocks == nil {
		blocks = []byte{}
	}

	msg := make([]byte, 1+len(blocks))
	msg[0] = SyncMsgBlocks
	copy(msg[1:], blocks)
	return msg, nil
}

// BuildGetBlockLocatorMsg serializes a GetBlockLocator request message.
func BuildGetBlockLocatorMsg(tipHeight uint64) ([]byte, error) {
	req := getBlockLocatorPayload{TipHeight: tipHeight}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("build getBlockLocator message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = SyncMsgGetBlockLocator
	copy(msg[1:], payload)
	return msg, nil
}

// BuildBlockLocatorMsg serializes a BlockLocator response message.
func BuildBlockLocatorMsg(locators [][]byte) ([]byte, error) {
	resp := blockLocatorPayload{Locators: locators}
	payload, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("build blockLocator message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = SyncMsgBlockLocator
	copy(msg[1:], payload)
	return msg, nil
}

// BuildNotFoundMsg serializes a NotFound response message.
func BuildNotFoundMsg(msgType byte, ids []string) ([]byte, error) {
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

// BuildStatusMsg serializes a Status broadcast message.
func BuildStatusMsg(height uint64, work string, latestHash string) ([]byte, error) {
	status := struct {
		Height     uint64 `json:"height"`
		Work       string `json:"work"`
		LatestHash string `json:"latestHash"`
	}{
		Height:     height,
		Work:       work,
		LatestHash: latestHash,
	}
	payload, err := json.Marshal(status)
	if err != nil {
		return nil, fmt.Errorf("build status message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = SyncMsgStatus
	copy(msg[1:], payload)
	return msg, nil
}

// Internal payload structures for JSON serialization.

type getHeadersPayload struct {
	From  uint64 `json:"from"`
	Count uint64 `json:"count"`
}

type headersPayload struct {
	Headers []byte `json:"headers"`
	HasMore bool   `json:"hasMore"`
}

type getBlocksPayload struct {
	Heights []uint64 `json:"heights"`
}

type getBlockLocatorPayload struct {
	TipHeight uint64 `json:"tipHeight"`
}

type blockLocatorPayload struct {
	Locators [][]byte `json:"locators"`
}

type notFoundPayload struct {
	MsgType byte     `json:"msgType"`
	IDs     []string `json:"ids"`
}

// ParseSyncMessageType extracts the message type from a raw sync message.
// Returns an error if the message is too short.
func ParseSyncMessageType(msgBytes []byte) (byte, error) {
	if len(msgBytes) < syncMinMsgSize {
		return 0, fmt.Errorf("sync message too short: %d bytes", len(msgBytes))
	}
	return msgBytes[0], nil
}

// DecodeUint64FromBytes decodes an 8-byte little-endian uint64.
// Returns an error if the byte slice is too short.
func DecodeUint64FromBytes(b []byte) (uint64, error) {
	if len(b) < 8 {
		return 0, fmt.Errorf("insufficient bytes for uint64: got %d, need 8", len(b))
	}
	return binary.LittleEndian.Uint64(b[:8]), nil
}

// EncodeUint64ToBytes encodes a uint64 to 8-byte little-endian format.
func EncodeUint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	return b
}
