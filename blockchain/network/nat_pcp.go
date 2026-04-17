// Copyright 2026 NogoChain Team
// Port Control Protocol (PCP) Implementation
// Implements RFC 6887 for NAT port mapping with better security than UPnP

package network

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"time"
)

// PCP constants (RFC 6887)
const (
	PCPVersion              = 2
	PCPPort               = 5351
	PCPMinPacketSize      = 24  // Minimum PCP packet size
	PCPMaxPacketSize      = 1100 // Maximum PCP packet size
	PCPDefaultLifetime    = 120 * 60 // 120 minutes in seconds

	// PCP OpCodes
	PCPOpcodeAnnounce    = 0
	PCPOpcodeMap         = 1
	PCPOpcodePeer      = 2

	// PCP Options
	PCPOptionThirdParty          = 1
	PCPOptionPrefrence         = 2
	PCPOptionFilterPrefix       = 3
	// Option 4-6 are reserved
	PCPOptionFQDN          = 8
	PCPOptionFQDNv6        = 9
)

// MappingError codes (RFC 6887)
type MappingError uint8

const (
	Success                 MappingError = 0
	UnsuppVersion           MappingError = 1
	NotAuthorized           MappingError = 2
	MalformedRequest       MappingError = 3
	UnsuppOpcode          MappingError = 4
	UnsuppOption          MappingError = 5
	MalformedOption        MappingError = 6
	NetworkFailure         MappingError = 7
	NoResources           MappingError = 8
	UnsuppProtocol        MappingError = 9
	UserExQuota            MappingError = 10
	CannotProvideExternal   MappingError = 11
	AddressMismatch        MappingError = 12
	ExcessiveRemotePeers  MappingError = 13
)

func (e MappingError) String() string {
	switch e {
	case Success:
		return "Success"
	case UnsuppVersion:
		return "Unsupported Version"
	case NotAuthorized:
		return "Not Authorized"
	case MalformedRequest:
		return "Malformed Request"
	case UnsuppOpcode:
		return "Unsupported Opcode"
	case UnsuppOption:
		return "Unsupported Option"
	case MalformedOption:
		return "Malformed Option"
	case NetworkFailure:
		return "Network Failure"
	case NoResources:
		return "No Resources"
	case UnsuppProtocol:
		return "Unsupported Protocol"
	case UserExQuota:
		return "User Exceeded Quota"
	case CannotProvideExternal:
		return "Cannot Provide External Address"
	case AddressMismatch:
		return "Address Mismatch"
	case ExcessiveRemotePeers:
		return "Excessive Remote Peers"
	default:
		return fmt.Sprintf("Unknown Error (%d)", e)
	}
}

// PCPPacket represents a PCP packet (RFC 6887)
type PCPPacket struct {
	Version     uint8  // Must be 2
	OpCode      uint8  // 0=Announce, 1=Map, 2=Peer
	Reserved    uint8  // MUST be zero on send
	RequestTime uint32 // Timestamp (epoch)
	Options     []PCPOption
}

// PCPOption represents a PCP option
type PCPOption struct {
	Code    uint8
	Length  uint8
	Payload []byte
}

// PCPMapOption represents the MAP option
type PCPMapOption struct {
	Nonce      [12]byte  // Random nonce
	Protocol   uint8      // 6=TCP, 17=UDP
	IntPort    uint16     // Internal port
	ExtPort    uint16     // External port (suggested)
	ExtIP      net.IP     // External IP address
}

// ParsePCPPacket parses a PCP packet from bytes
func ParsePCPPacket(data []byte) (*PCPPacket, error) {
	if len(data) < PCPMinPacketSize {
		return nil, errors.New("packet too small")
	}

	packet := &PCPPacket{
		Version:     data[0],
		OpCode:      data[1],
		Reserved:    data[2],
		RequestTime: binary.BigEndian.Uint32(data[4:8]),
	}

	// Parse options
	offset := 8
	for offset < len(data) {
		if offset+4 > len(data) {
			break
		}

		option := &PCPOption{
			Code:   data[offset],
			Length: data[offset+1],
		}

		if offset+4+int(option.Length) > len(data) {
			return nil, fmt.Errorf("option %d extends beyond packet", option.Code)
		}

		option.Payload = data[offset+4 : offset+4+int(option.Length)]
		packet.Options = append(packet.Options, *option)
		offset += 4 + int(option.Length)
	}

	return packet, nil
}

// Serialize converts a PCPPacket to bytes
func (p *PCPPacket) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Header
	buf.WriteByte(p.Version)
	buf.WriteByte(p.OpCode)
	buf.WriteByte(p.Reserved)
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, p.RequestTime)
	buf.Write(tmp)

	// Options
	for _, opt := range p.Options {
		buf.WriteByte(opt.Code)
		buf.WriteByte(opt.Length)
		buf.Write(opt.Payload)

		// Pad to 32-bit boundary
		for (opt.Length+4)%4 != 0 {
			buf.WriteByte(0)
		}
	}

	return buf.Bytes(), nil
}

// GenerateRandomNonce generates a random nonce for PCP mapping
func GenerateRandomNonce() ([]byte, error) {
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate random nonce: %w", err)
	}
	return nonce, nil
}

// PCPClient implements a PCP client
type PCPClient struct {
	Gateway   net.IP
	Protocol  uint8 // 6=TCP, 17=UDP
	LocalIP   net.IP
}

// NewPCPClient creates a new PCP client
func NewPCPClient(gateway, localIP net.IP, protocol uint8) *PCPClient {
	return &PCPClient{
		Gateway:  gateway,
		LocalIP:   localIP,
		Protocol:  protocol,
	}
}

// RequestPortMapping sends a MAP request via PCP
func (c *PCPClient) RequestPortMapping(internalPort int, lifetime int) (uint16, error) {
	// Generate nonce
	nonce, err := GenerateRandomNonce()
	if err != nil {
		return 0, err
	}

	log.Printf("[PCP] Requesting port mapping: internal port=%d, protocol=%d", internalPort, c.Protocol)

	// Build MAP option payload
	mapPayload := new(bytes.Buffer)
	mapPayload.Write(nonce[:12])                // Nonce
	mapPayload.WriteByte(c.Protocol)              // Protocol
	tmp := make([]byte, 2)
	binary.BigEndian.PutUint16(tmp, uint16(internalPort))
	mapPayload.Write(tmp)                      // Internal port
	tmp = make([]byte, 2)
	binary.BigEndian.PutUint16(tmp, uint16(0))
	mapPayload.Write(tmp)                         // External port (0 = any)
	tmp = make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, uint32(0))
	mapPayload.Write(tmp)                           // Internal IP (0 = auto)

	mapOption := PCPOption{
		Code:    PCPOpcodeMap,
		Length:   uint8(mapPayload.Len()),
		Payload: mapPayload.Bytes(),
	}

	// Build packet
	requestTime := uint32(time.Now().Unix())
	packet := &PCPPacket{
		Version:     PCPVersion,
		OpCode:      PCPOpcodeMap,
		Reserved:    0,
		RequestTime: requestTime,
		Options:     []PCPOption{mapOption},
	}

	data, err := packet.Serialize()
	if err != nil {
		return 0, fmt.Errorf("failed to serialize packet: %w", err)
	}

	// Send to gateway
	conn, err := net.Dial("udp", net.JoinHostPort(c.Gateway.String(), fmt.Sprintf("%d", PCPPort)))
	if err != nil {
		return 0, fmt.Errorf("failed to connect to PCP server: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))
	_, err = conn.Write(data)
	if err != nil {
		return 0, fmt.Errorf("failed to send PCP request: %w", err)
	}

	// Read response
	response := make([]byte, PCPMaxPacketSize)
	n, err := conn.Read(response)
	if err != nil {
		return 0, fmt.Errorf("failed to read PCP response: %w", err)
	}

	// Parse response
	respPacket, err := ParsePCPPacket(response[:n])
	if err != nil {
		return 0, fmt.Errorf("failed to parse PCP response: %w", err)
	}

	// Check response
	if respPacket.OpCode != PCPOpcodeMap {
		return 0, fmt.Errorf("unexpected response opcode: %d", respPacket.OpCode)
	}

	// Parse MAP response option
	if len(respPacket.Options) < 1 {
		return 0, errors.New("no options in response")
	}

	mapResp := respPacket.Options[0]
	if mapResp.Code != PCPOpcodeMap {
		return 0, fmt.Errorf("unexpected response option code: %d", mapResp.Code)
	}

	if mapResp.Length < 24 {
		return 0, errors.New("MAP option too short")
	}

	// Parse external IP and port from response
	extPort := binary.BigEndian.Uint16(mapResp.Payload[14:16])
	extIP := net.IP(mapResp.Payload[16:20])

	// Check result code (last byte)
	resultCode := mapResp.Payload[23]
	mappingErr := MappingError(resultCode)

	if mappingErr != Success {
		return 0, fmt.Errorf("PCP mapping failed: %s", mappingErr.String())
	}

	log.Printf("[PCP] ✓ Port mapping successful!")
	log.Printf("[PCP]   External: %s:%d", extIP.String(), extPort)
	log.Printf("[PCP]   Internal: %s:%d", c.LocalIP.String(), internalPort)
	log.Printf("[PCP]   Lifetime: %d seconds", lifetime)

	return extPort, nil
}

// DeletePortMapping removes a PCP port mapping
func (c *PCPClient) DeletePortMapping(internalPort int) error {
	log.Printf("[PCP] Deleting port mapping: internal port=%d", internalPort)

	// For PCP, we can request a mapping with lifetime=0 to delete
	_, err := c.RequestPortMapping(internalPort, 0)
	if err != nil {
		return fmt.Errorf("failed to delete PCP mapping: %w", err)
	}

	log.Printf("[PCP] Port mapping deleted successfully")
	return nil
}
