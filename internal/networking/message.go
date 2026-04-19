package networking

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	MaxMessageSize = 16 << 20
)

type MessageType uint8

// Wire protocol message type codes.
// Canonical types (active): Ping, Pong, GetBlocks, GetHeaders, Headers, Block,
// GetBlock, GetChainInfo, ChainInfo, NotFound, Error, Version, VerAck, Addr,
// Inv, GetData, Reject, MemPool, SendHeaders, SendCompactBlocks, CompactBlock.
//
// Deprecated aliases (reserved for backward compatibility, do not use in new code):
//   MessageTypeBlocks(0x04)      -> use Inv+GetData flow
//   MessageTypeGetTX(0x05)       -> use MessageTypeGetData(0x14)
//   MessageTypeTX(0x06)          -> use MessageTypeTransaction(0x0B)
//   MessageTypeTransaction(0x0B) -> canonical for single transaction relay
//   MessageTypePingExtended(0x1B)-> reserved, use MessageTypePing(0x01)
//   MessageTypePongExtended(0x1C)-> reserved, use MessageTypePong(0x02)
//
// Handshake protocol uses 0x80-0x8F range (see bootstrap_peer_manager.go).
const (
	MessageTypePing              MessageType = 0x01
	MessageTypePong              MessageType = 0x02
	MessageTypeGetBlocks         MessageType = 0x03
	MessageTypeBlocks            MessageType = 0x04
	MessageTypeGetTX             MessageType = 0x05
	MessageTypeTX                MessageType = 0x06
	MessageTypeGetHeaders        MessageType = 0x07
	MessageTypeHeaders           MessageType = 0x08
	MessageTypeBlock             MessageType = 0x09
	MessageTypeGetBlock          MessageType = 0x0A
	MessageTypeTransaction       MessageType = 0x0B
	MessageTypeGetChainInfo      MessageType = 0x0C
	MessageTypeChainInfo         MessageType = 0x0D
	MessageTypeNotFound          MessageType = 0x0E
	MessageTypeError             MessageType = 0x0F
	MessageTypeVersion           MessageType = 0x10
	MessageTypeVerAck            MessageType = 0x11
	MessageTypeAddr              MessageType = 0x12
	MessageTypeInv               MessageType = 0x13
	MessageTypeGetData           MessageType = 0x14
	MessageTypeReject            MessageType = 0x15
	MessageTypeMemPool           MessageType = 0x16
	MessageTypeFilterLoad        MessageType = 0x17
	MessageTypeFilterAdd         MessageType = 0x18
	MessageTypeFilterClear       MessageType = 0x19
	MessageTypeMerkleBlock       MessageType = 0x1A
	MessageTypePingExtended      MessageType = 0x1B
	MessageTypePongExtended      MessageType = 0x1C
	MessageTypeSendHeaders       MessageType = 0x1D
	MessageTypeSendCompactBlocks MessageType = 0x1E
	MessageTypeCompactBlock      MessageType = 0x1F
	MessageTypeGetBlockTxn       MessageType = 0x20
	MessageTypeBlockTxn          MessageType = 0x21
)

var ErrMessageTooLarge = errors.New("message too large")
var ErrInvalidChecksum = errors.New("invalid checksum")
var ErrInvalidMessage = errors.New("invalid message")

type Message struct {
	Type    MessageType `json:"type"`
	Payload []byte      `json:"payload"`
}

type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type Hello struct {
	Protocol  uint32 `json:"protocol"`
	ChainID   uint64 `json:"chainId"`
	RulesHash string `json:"rulesHash"`
	NodeID    string `json:"nodeId"`
	TimeUnix  int64  `json:"timeUnix"`
	Port      uint16 `json:"port,omitempty"`
	Services  uint64 `json:"services,omitempty"`
}

func NewHello(chainID uint64, rulesHash, nodeID string) Hello {
	return Hello{
		Protocol:  ProtocolVersionNumber,
		ChainID:   chainID,
		RulesHash: rulesHash,
		NodeID:    nodeID,
		TimeUnix:  0,
	}
}

type ChainInfo struct {
	ChainID     uint64 `json:"chainId"`
	Height      uint64 `json:"height"`
	LatestHash  string `json:"latestHash"`
	GenesisHash string `json:"genesisHash,omitempty"`
	RulesHash   string `json:"rulesHash,omitempty"`
}

type HeadersRequest struct {
	From  uint64 `json:"from"`
	Count int    `json:"count"`
}

type HeadersResponse struct {
	Headers []BlockHeader `json:"headers"`
}

type BlockHeader struct {
	Version        uint32 `json:"version"`
	Height         uint64 `json:"height"`
	TimestampUnix  int64  `json:"timestampUnix"`
	PrevHash       []byte `json:"prevHash"`
	Hash           []byte `json:"hash"`
	DifficultyBits uint32 `json:"difficultyBits"`
	MerkleRoot     []byte `json:"merkleRoot,omitempty"`
}

type GetBlocksRequest struct {
	BlockLocator [][]byte `json:"blockLocator"`
	HashStop     []byte   `json:"hashStop"`
}

type BlocksResponse struct {
	Blocks []Block `json:"blocks"`
}

type GetTXRequest struct {
	TxID string `json:"txId"`
}

type TXResponse struct {
	TxHex string `json:"txHex"`
}

type BlockResponse struct {
	BlockHex string `json:"blockHex"`
}

type TransactionBroadcast struct {
	TxHex string `json:"txHex"`
}

type BlockBroadcast struct {
	BlockHex string `json:"blockHex"`
}

type GetDataRequest struct {
	InvType  uint8    `json:"invType"`
	Hash     []byte   `json:"hash"`
	BlockLoc [][]byte `json:"blockLocator,omitempty"`
	HashStop []byte   `json:"hashStop,omitempty"`
}

// Inventory type codes for GetData requests.
// Must align with blockchain/network.InventoryType values:
//   0 = Error, 1 = TX, 2 = Block, 3 = Filtered Block, 4 = Compact Block
const (
	InvTypeError        uint8 = 0
	InvTypeMSG_TX       uint8 = 1
	InvTypeMSG_BLOCK    uint8 = 2
	InvTypeMSG_FILTERED uint8 = 3
	InvTypeMSG_COMPACT  uint8 = 4
)

type InvMessage struct {
	Items []InvItem `json:"items"`
}

type InvItem struct {
	Type uint8 `json:"type"`
	Hash []byte `json:"hash"`
}

type Address struct {
	Addresses []NetworkAddress `json:"addresses"`
}

type NetworkAddress struct {
	Timestamp int64  `json:"timestamp,omitempty"`
	Services  uint64 `json:"services,omitempty"`
	Address   string `json:"address"`
	Port      uint16 `json:"port"`
}

type RejectMessage struct {
	Message string `json:"message"`
	CCode   uint8  `json:"ccode"`
	Reason  string `json:"reason"`
}

type Codec struct {
	maxMessageSize int
}

func NewCodec(maxMessageSize int) *Codec {
	if maxMessageSize <= 0 {
		maxMessageSize = MaxMessageSize
	}
	return &Codec{maxMessageSize: maxMessageSize}
}

type framedMessage struct {
	Version  uint8
	Length   uint32
	Checksum [4]byte
	Type     MessageType
	Payload  []byte
}

func (c *Codec) Encode(w io.Writer, msg *Message) error {
	payloadLen := uint32(len(msg.Payload))
	if payloadLen > uint32(c.maxMessageSize) {
		return ErrMessageTooLarge
	}

	checksum := sha256.Sum256(msg.Payload)
	checksum = sha256.Sum256(checksum[:])

	frame := framedMessage{
		Version:  1,
		Length:   payloadLen,
		Checksum: [4]byte{checksum[0], checksum[1], checksum[2], checksum[3]},
		Type:     msg.Type,
		Payload:  msg.Payload,
	}

	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}

	var header [9]byte
	header[0] = frame.Version
	binary.BigEndian.PutUint32(header[1:5], frame.Length)
	copy(header[5:9], frame.Checksum[:])

	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}

	return nil
}

func (c *Codec) Decode(r io.Reader) (*Message, error) {
	var header [9]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}

	version := header[0]
	if version != 1 {
		return nil, fmt.Errorf("unsupported protocol version: %d", version)
	}

	length := binary.BigEndian.Uint32(header[1:5])
	if length > uint32(c.maxMessageSize) {
		return nil, ErrMessageTooLarge
	}

	expectedChecksum := [4]byte{header[5], header[6], header[7], header[8]}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	checksum := sha256.Sum256(payload)
	checksum = sha256.Sum256(checksum[:])
	if checksum[0] != expectedChecksum[0] || checksum[1] != expectedChecksum[1] ||
		checksum[2] != expectedChecksum[2] || checksum[3] != expectedChecksum[3] {
		return nil, ErrInvalidChecksum
	}

	var frame framedMessage
	if err := json.Unmarshal(payload, &frame); err != nil {
		return nil, err
	}

	return &Message{
		Type:    frame.Type,
		Payload: frame.Payload,
	}, nil
}

func (c *Codec) WriteJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(b)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func (c *Codec) ReadJSON(r io.Reader, maxBytes int) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint32(lenBuf[:]))
	if n <= 0 {
		return nil, io.ErrUnexpectedEOF
	}
	if maxBytes > 0 && n > maxBytes {
		_, _ = io.CopyN(io.Discard, r, int64(n))
		return nil, ErrMessageTooLarge
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

type Block = interface{ Block() }
type Transaction = interface{ Transaction() }
