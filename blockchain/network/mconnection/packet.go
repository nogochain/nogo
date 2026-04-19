package mconnection

import (
	"encoding/binary"
	"fmt"
	"io"
)

// maxMsgPacketPayloadSize is the maximum payload size per packet fragment.
// Messages larger than this are split into multiple packets.
const maxMsgPacketPayloadSize = 1024

// msgPacketHeaderSize is the size of the packet header:
// 1 byte channelID + 2 bytes length + 1 byte EOF flag = 4 bytes
const msgPacketHeaderSize = 4

// msgPacket represents a fragmented message packet on the wire.
// Large messages are split into multiple packets with EOF marking the last one.
// Frame format: [1 byte channelID][2 bytes length][data][1 byte EOF flag]
type msgPacket struct {
	ChannelID byte
	Data      []byte
	EOF       bool
}

// Serialize encodes the packet into wire format.
// Format: [1 byte channelID][2 bytes big-endian length][data][1 byte EOF flag]
func (p *msgPacket) Serialize() ([]byte, error) {
	payloadLen := len(p.Data)
	if payloadLen > maxMsgPacketPayloadSize {
		return nil, fmt.Errorf("payload size %d exceeds maximum %d", payloadLen, maxMsgPacketPayloadSize)
	}

	buf := make([]byte, msgPacketHeaderSize+payloadLen)
	buf[0] = p.ChannelID
	binary.BigEndian.PutUint16(buf[1:3], uint16(payloadLen))
	copy(buf[3:3+payloadLen], p.Data)
	if p.EOF {
		buf[3+payloadLen] = 0x01
	} else {
		buf[3+payloadLen] = 0x00
	}
	return buf, nil
}

// SerializeTo writes the packet directly to an io.Writer.
// Returns the number of bytes written and any error encountered.
func (p *msgPacket) SerializeTo(w io.Writer) (int, error) {
	payloadLen := len(p.Data)
	if payloadLen > maxMsgPacketPayloadSize {
		return 0, fmt.Errorf("payload size %d exceeds maximum %d", payloadLen, maxMsgPacketPayloadSize)
	}

	header := make([]byte, msgPacketHeaderSize)
	header[0] = p.ChannelID
	binary.BigEndian.PutUint16(header[1:3], uint16(payloadLen))
	if p.EOF {
		header[3] = 0x01
	} else {
		header[3] = 0x00
	}

	n, err := w.Write(header)
	if err != nil {
		return n, fmt.Errorf("write packet header: %w", err)
	}

	if payloadLen > 0 {
		dataN, err := w.Write(p.Data)
		n += dataN
		if err != nil {
			return n, fmt.Errorf("write packet payload: %w", err)
		}
	}

	return n, nil
}

// Deserialize decodes a packet from wire format reader.
// Reads exactly the packet frame and returns the decoded msgPacket.
func DeserializePacket(r io.Reader) (*msgPacket, error) {
	header := make([]byte, msgPacketHeaderSize)
	_, err := io.ReadFull(r, header)
	if err != nil {
		return nil, fmt.Errorf("read packet header: %w", err)
	}

	channelID := header[0]
	payloadLen := int(binary.BigEndian.Uint16(header[1:3]))
	eofFlag := header[3]

	if payloadLen > maxMsgPacketPayloadSize {
		return nil, fmt.Errorf("payload length %d exceeds maximum %d", payloadLen, maxMsgPacketPayloadSize)
	}

	var data []byte
	if payloadLen > 0 {
		data = make([]byte, payloadLen)
		_, err := io.ReadFull(r, data)
		if err != nil {
			return nil, fmt.Errorf("read packet payload: %w", err)
		}
	} else {
		data = make([]byte, 0)
	}

	eof := eofFlag == 0x01

	return &msgPacket{
		ChannelID: channelID,
		Data:      data,
		EOF:       eof,
	}, nil
}

// String returns a human-readable representation of the packet for debugging.
func (p *msgPacket) String() string {
	eofStr := "false"
	if p.EOF {
		eofStr = "true"
	}
	return fmt.Sprintf("msgPacket{chID:0x%02x, len:%d, eof:%s}", p.ChannelID, len(p.Data), eofStr)
}
