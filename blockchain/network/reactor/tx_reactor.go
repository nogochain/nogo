package reactor

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/network/mconnection"
)

// Tx message type constants.
// Each message on the tx channel starts with a single byte indicating
// the message type, followed by JSON-encoded payload.
const (
	// TxMsgInv announces transaction IDs available at the sender.
	// Payload: JSON-encoded txInvPayload {txIDs: []string}.
	TxMsgInv byte = 0x01

	// TxMsgGet requests specific transactions by ID.
	// Payload: JSON-encoded txGetPayload {txIDs: []string}.
	TxMsgGet byte = 0x02

	// TxMsgTx delivers one or more full transactions.
	// Payload: JSON-encoded txTxPayload {txs: []json.RawMessage}.
	TxMsgTx byte = 0x03
)

// Minimum message size: 1 byte for message type.
const txMinMsgSize = 1

// TxHandler defines the interface for transaction-related business logic.
// This allows injecting the actual mempool/tx implementation without
// creating circular dependencies between the reactor and core packages.
type TxHandler interface {
	// OnInvTx handles an inventory announcement of available transactions.
	OnInvTx(peerID string, txIDs []string) error

	// OnGetTx handles a request for specific transactions.
	OnGetTx(peerID string, txIDs []string) error

	// OnTx handles received full transactions from a peer.
	// The transactions have been parsed and validated at the JSON level.
	OnTx(peerID string, txs []core.Transaction) error
}

// TxReactor handles transaction propagation protocol messages.
//
// It operates on the tx channel (ChannelTx) and manages message parsing,
// validation, and dispatching to the injected TxHandler.
//
// Thread-safety: handler access is protected by sync.RWMutex.
type TxReactor struct {
	BaseReactor
	mu      sync.RWMutex
	handler TxHandler
}

// NewTxReactor creates a new TxReactor with the given handler.
// Returns an error if handler is nil.
func NewTxReactor(handler TxHandler) (*TxReactor, error) {
	if handler == nil {
		return nil, fmt.Errorf("tx reactor: handler must not be nil")
	}

	chs := []*mconnection.ChannelDescriptor{
		{
			ID:                  mconnection.ChannelTx,
			Priority:            3,
			SendQueueCapacity:   256,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	r := &TxReactor{
		handler: handler,
	}
	r.SetChannels(chs)

	return r, nil
}

// SetHandler replaces the transaction handler.
func (tr *TxReactor) SetHandler(handler TxHandler) error {
	if handler == nil {
		return fmt.Errorf("tx reactor: handler must not be nil")
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.handler = handler
	return nil
}

// AddPeer is called when a new peer connects on the tx channel.
// Currently no-op; override if peer-specific tx state is needed.
func (tr *TxReactor) AddPeer(_ string, _ map[string]string) error {
	return nil
}

// RemovePeer is called when a peer disconnects.
// Currently no-op; override if peer-specific cleanup is needed.
func (tr *TxReactor) RemovePeer(_ string, _ interface{}) {
}

// Receive parses and dispatches incoming tx channel messages.
//
// Message format: [1 byte msgType][JSON payload]
// The first byte determines the message type, and the remaining bytes
// are JSON-encoded data specific to that message type.
func (tr *TxReactor) Receive(chID byte, peerID string, msgBytes []byte) {
	if peerID == "" {
		return
	}
	if len(msgBytes) < txMinMsgSize {
		return
	}

	msgType := msgBytes[0]
	payload := msgBytes[txMinMsgSize:]

	tr.mu.RLock()
	handler := tr.handler
	tr.mu.RUnlock()

	if handler == nil {
		return
	}

	tr.dispatch(msgType, peerID, payload, handler)
}

// dispatch routes the parsed message to the appropriate handler method.
func (tr *TxReactor) dispatch(msgType byte, peerID string, payload []byte, handler TxHandler) {
	switch msgType {
	case TxMsgInv:
		tr.handleInvTx(peerID, payload, handler)
	case TxMsgGet:
		tr.handleGetTx(peerID, payload, handler)
	case TxMsgTx:
		tr.handleTx(peerID, payload, handler)
	default:
		// Unknown message type - silently ignore.
	}
}

// handleInvTx parses and dispatches an InvTx message.
func (tr *TxReactor) handleInvTx(peerID string, payload []byte, handler TxHandler) {
	if len(payload) == 0 {
		return
	}

	var req txInvPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return
	}

	if req.TxIDs == nil {
		req.TxIDs = []string{}
	}

	if err := handler.OnInvTx(peerID, req.TxIDs); err != nil {
		return
	}
}

// handleGetTx parses and dispatches a GetTx message.
func (tr *TxReactor) handleGetTx(peerID string, payload []byte, handler TxHandler) {
	if len(payload) == 0 {
		return
	}

	var req txGetPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return
	}

	if req.TxIDs == nil {
		req.TxIDs = []string{}
	}

	if err := handler.OnGetTx(peerID, req.TxIDs); err != nil {
		return
	}
}

// handleTx parses and dispatches a Tx message containing full transactions.
func (tr *TxReactor) handleTx(peerID string, payload []byte, handler TxHandler) {
	if len(payload) == 0 {
		return
	}

	var req txTxPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return
	}

	if req.Txs == nil {
		req.Txs = []json.RawMessage{}
	}

	txs := make([]core.Transaction, 0, len(req.Txs))
	for _, raw := range req.Txs {
		var tx core.Transaction
		if err := json.Unmarshal(raw, &tx); err != nil {
			// Skip malformed transaction, continue processing remaining.
			continue
		}
		txs = append(txs, tx)
	}

	if len(txs) == 0 {
		return
	}

	if err := handler.OnTx(peerID, txs); err != nil {
		return
	}
}

// BuildTxInvMsg serializes a transaction inventory announcement message.
func BuildTxInvMsg(txIDs []string) ([]byte, error) {
	if txIDs == nil {
		txIDs = []string{}
	}

	req := txInvPayload{TxIDs: txIDs}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("build txInv message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = TxMsgInv
	copy(msg[1:], payload)
	return msg, nil
}

// BuildTxGetMsg serializes a transaction request message.
func BuildTxGetMsg(txIDs []string) ([]byte, error) {
	if txIDs == nil {
		txIDs = []string{}
	}

	req := txGetPayload{TxIDs: txIDs}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("build txGet message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = TxMsgGet
	copy(msg[1:], payload)
	return msg, nil
}

// BuildTxMsg serializes a full transaction delivery message.
func BuildTxMsg(txs []core.Transaction) ([]byte, error) {
	if txs == nil {
		txs = []core.Transaction{}
	}

	rawTxs := make([]json.RawMessage, 0, len(txs))
	for _, tx := range txs {
		raw, err := json.Marshal(tx)
		if err != nil {
			return nil, fmt.Errorf("build tx message: marshal transaction: %w", err)
		}
		rawTxs = append(rawTxs, raw)
	}

	req := txTxPayload{Txs: rawTxs}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("build tx message: %w", err)
	}

	msg := make([]byte, 1+len(payload))
	msg[0] = TxMsgTx
	copy(msg[1:], payload)
	return msg, nil
}

// ParseTxMessageType extracts the message type from a raw tx message.
// Returns an error if the message is too short.
func ParseTxMessageType(msgBytes []byte) (byte, error) {
	if len(msgBytes) < txMinMsgSize {
		return 0, fmt.Errorf("tx message too short: %d bytes", len(msgBytes))
	}
	return msgBytes[0], nil
}

// Internal payload structures for JSON serialization.

type txInvPayload struct {
	TxIDs []string `json:"txIDs"`
}

type txGetPayload struct {
	TxIDs []string `json:"txIDs"`
}

type txTxPayload struct {
	Txs []json.RawMessage `json:"txs"`
}
