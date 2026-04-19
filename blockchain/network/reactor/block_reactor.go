package reactor

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/network/mconnection"
)

// Block message type constants.
// Each message on the block channel starts with a single byte indicating
// the message type, followed by JSON-encoded payload.
const (
	// BlockMsgInv announces block hashes available at the sender.
	// Payload: JSON-encoded blockInvPayload {blockHashes: []string}.
	BlockMsgInv byte = 0x01

	// BlockMsgGet requests specific blocks by hash.
	// Payload: JSON-encoded blockGetPayload {blockHashes: []string}.
	BlockMsgGet byte = 0x02

	// BlockMsgBlock delivers one or more full blocks.
	// Payload: JSON-encoded blockBlockPayload {blocks: []json.RawMessage}.
	BlockMsgBlock byte = 0x03
)

// Minimum message size: 1 byte for message type.
const blockMinMsgSize = 1

// BlockHandler defines the interface for block-related business logic.
// This allows injecting the actual chain/block implementation without
// creating circular dependencies between the reactor and core packages.
type BlockHandler interface {
	// OnInvBlock handles an inventory announcement of available blocks.
	// Block hashes are provided as hex-encoded strings.
	OnInvBlock(peerID string, blockHashes []string) error

	// OnGetBlock handles a request for specific blocks.
	// Block hashes are provided as hex-encoded strings.
	OnGetBlock(peerID string, blockHashes []string) error

	// OnBlock handles received full blocks from a peer.
	// The blocks have been parsed and validated at the JSON level.
	OnBlock(peerID string, blocks []*core.Block) error
}

// BlockReactor handles block propagation protocol messages.
//
// It operates on the block channel (ChannelBlock) and manages message parsing,
// validation, and dispatching to the injected BlockHandler.
//
// Thread-safety: handler access is protected by sync.RWMutex.
type BlockReactor struct {
	BaseReactor
	mu      sync.RWMutex
	handler BlockHandler
}

// NewBlockReactor creates a new BlockReactor with the given handler.
// Returns an error if handler is nil.
func NewBlockReactor(handler BlockHandler) (*BlockReactor, error) {
	if handler == nil {
		return nil, fmt.Errorf("block reactor: handler must not be nil")
	}

	chs := []*mconnection.ChannelDescriptor{
		{
			ID:                  mconnection.ChannelBlock,
			Priority:            5,
			SendQueueCapacity:   256,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	r := &BlockReactor{
		handler: handler,
	}
	r.SetChannels(chs)

	return r, nil
}

// SetHandler replaces the block handler.
func (br *BlockReactor) SetHandler(handler BlockHandler) error {
	if handler == nil {
		return fmt.Errorf("block reactor: handler must not be nil")
	}
	br.mu.Lock()
	defer br.mu.Unlock()
	br.handler = handler
	return nil
}

// AddPeer is called when a new peer connects on the block channel.
// Currently no-op; override if peer-specific block state is needed.
func (br *BlockReactor) AddPeer(_ string, _ map[string]string) error {
	return nil
}

// RemovePeer is called when a peer disconnects.
// Currently no-op; override if peer-specific cleanup is needed.
func (br *BlockReactor) RemovePeer(_ string, _ interface{}) {
}

// Receive parses and dispatches incoming block channel messages.
//
// Message format: [1 byte msgType][JSON payload]
// The first byte determines the message type, and the remaining bytes
// are JSON-encoded data specific to that message type.
func (br *BlockReactor) Receive(chID byte, peerID string, msgBytes []byte) {
	if peerID == "" {
		return
	}
	if len(msgBytes) < blockMinMsgSize {
		return
	}

	msgType := msgBytes[0]
	payload := msgBytes[blockMinMsgSize:]

	br.mu.RLock()
	handler := br.handler
	br.mu.RUnlock()

	if handler == nil {
		return
	}

	// CRITICAL: Skip processing response messages that should be handled by sendAndWait
	// BlockMsgBlock is a response to BlockMsgGet requests, not a broadcast
	// It should be routed to pending request channels, not processed here
	if msgType == BlockMsgBlock {
		// This is a response message, skip processing
		// It will be handled by the sendAndWait mechanism in switch.go
		return
	}

	br.dispatch(msgType, peerID, payload, handler)
}

// dispatch routes the parsed message to the appropriate handler method.
func (br *BlockReactor) dispatch(msgType byte, peerID string, payload []byte, handler BlockHandler) {
	switch msgType {
	case BlockMsgInv:
		br.handleInvBlock(peerID, payload, handler)
	case BlockMsgGet:
		br.handleGetBlock(peerID, payload, handler)
	case BlockMsgBlock:
		br.handleBlock(peerID, payload, handler)
	default:
		// Unknown message type - silently ignore.
	}
}

// handleInvBlock parses and dispatches an InvBlock message.
func (br *BlockReactor) handleInvBlock(peerID string, payload []byte, handler BlockHandler) {
	if len(payload) == 0 {
		return
	}

	var req blockInvPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return
	}

	if req.BlockHashes == nil {
		req.BlockHashes = []string{}
	}

	for _, hashHex := range req.BlockHashes {
		if _, err := hex.DecodeString(hashHex); err != nil {
			return
		}
	}

	if err := handler.OnInvBlock(peerID, req.BlockHashes); err != nil {
		return
	}
}

// handleGetBlock parses and dispatches a GetBlock message.
func (br *BlockReactor) handleGetBlock(peerID string, payload []byte, handler BlockHandler) {
	if len(payload) == 0 {
		return
	}

	var req blockGetPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return
	}

	if req.BlockHashes == nil {
		req.BlockHashes = []string{}
	}

	for _, hashHex := range req.BlockHashes {
		if _, err := hex.DecodeString(hashHex); err != nil {
			return
		}
	}

	if err := handler.OnGetBlock(peerID, req.BlockHashes); err != nil {
		return
	}
}

// handleBlock parses and dispatches a Block message containing full blocks.
func (br *BlockReactor) handleBlock(peerID string, payload []byte, handler BlockHandler) {
	if len(payload) == 0 {
		return
	}

	var req blockBlockPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return
	}

	if req.Blocks == nil {
		req.Blocks = []json.RawMessage{}
	}

	blocks := make([]*core.Block, 0, len(req.Blocks))
	for _, raw := range req.Blocks {
		var block core.Block
		if err := json.Unmarshal(raw, &block); err != nil {
			// Skip malformed block, continue processing remaining.
			continue
		}
		blocks = append(blocks, &block)
	}

	if len(blocks) == 0 {
		return
	}

	if err := handler.OnBlock(peerID, blocks); err != nil {
		return
	}
}

// BuildBlockInvMsg serializes a block inventory announcement message.
func BuildBlockInvMsg(blockHashes []string) ([]byte, error) {
	if blockHashes == nil {
		blockHashes = []string{}
	}

	req := blockInvPayload{BlockHashes: blockHashes}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("build blockInv message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = BlockMsgInv
	copy(msg[1:], payload)
	return msg, nil
}

// BuildBlockGetMsg serializes a block request message.
func BuildBlockGetMsg(blockHashes []string) ([]byte, error) {
	if blockHashes == nil {
		blockHashes = []string{}
	}

	req := blockGetPayload{BlockHashes: blockHashes}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("build blockGet message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = BlockMsgGet
	copy(msg[1:], payload)
	return msg, nil
}

// BuildBlockMsg serializes a full block delivery message.
func BuildBlockMsg(blocks []*core.Block) ([]byte, error) {
	if blocks == nil {
		blocks = []*core.Block{}
	}

	rawBlocks := make([]json.RawMessage, 0, len(blocks))
	for _, block := range blocks {
		raw, err := json.Marshal(block)
		if err != nil {
			return nil, fmt.Errorf("build block message: marshal block: %w", err)
		}
		rawBlocks = append(rawBlocks, raw)
	}

	req := blockBlockPayload{Blocks: rawBlocks}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("build block message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = BlockMsgBlock
	copy(msg[1:], payload)
	return msg, nil
}

// ParseBlockMessageType extracts the message type from a raw block message.
// Returns an error if the message is too short.
func ParseBlockMessageType(msgBytes []byte) (byte, error) {
	if len(msgBytes) < blockMinMsgSize {
		return 0, fmt.Errorf("block message too short: %d bytes", len(msgBytes))
	}
	return msgBytes[0], nil
}

// Internal payload structures for JSON serialization.

type blockInvPayload struct {
	BlockHashes []string `json:"blockHashes"`
}

type blockGetPayload struct {
	BlockHashes []string `json:"blockHashes"`
}

type blockBlockPayload struct {
	Blocks []json.RawMessage `json:"blocks"`
}
