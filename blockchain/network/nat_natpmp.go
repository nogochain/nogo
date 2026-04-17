// Copyright 2026 NogoChain Team
// NAT-PMP (NAT Port Mapping Protocol) Implementation
// Implements RFC 6886 for Apple NAT-PMP devices

package network

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"time"
)

// NAT-PMP constants (RFC 6886)
const (
	NATPMPVersion      = 0
	NATPMPClientPort   = 5351
	NATPMPServerPort  = 5350

	// NAT-PMP Opcodes
	NATPMPOpcodeExternalAddr = 0
	NATPMPOpcodeMapUDP      = 1
	NATPMPOpcodeMapTCP      = 2

	// NAT-PMP Result Codes
	NATPMPResultSuccess            = 128
	NATPMPResultUnsupportedVersion = 129
	NATPMPResultRefused            = 130
	NATPMPResultFailure           = 131

	// NAT-PMP Default TTL
	NATPMPDefaultTTL = 7200 // 2 hours in seconds
)

// NATPMPRequest represents a NAT-PMP request
type NATPMPRequest struct {
	Version  uint8 // Must be 0
	OpCode  uint8 // 0=ExternalAddr, 1=MapUDP, 2=MapTCP
	Reserved uint16
}

// NATPMPResponse represents a NAT-PMP response
type NATPMPResponse struct {
	Version   uint8
	OpCode    uint8
	ResultCode uint8
	Epoch     uint16
	Reserved   [2]uint16
	Addresses []byte
}

// MapPortRequest represents a port mapping request
type MapPortRequest struct {
	PrivatePort uint16
	PublicPort  uint16 // 0 = any port
	TTL        uint32
	Reserved   [3]uint16
}

// MapPortResponse represents a port mapping response
type MapPortResponse struct {
	PrivatePort uint16
	PublicPort  uint16
	TTL         uint32
	Reserved    [3]uint16
}

// NATPMPClient implements a NAT-PMP client
type NATPMPClient struct {
	Gateway   net.IP
	LastEpoch uint16
}

// NewNATPMPClient creates a new NAT-PMP client
func NewNATPMPClient(gateway net.IP) *NATPMPClient {
	return &NATPMPClient{
		Gateway:   gateway,
		LastEpoch: 0,
	}
}

// GetExternalAddress requests the external IP address
func (c *NATPMPClient) GetExternalAddress() (net.IP, uint16, error) {
	log.Printf("[NAT-PMP] Requesting external address...")

	// Build request
	req := NATPMPRequest{
		Version:  NATPMPVersion,
		OpCode:  NATPMPOpcodeExternalAddr,
		Reserved: 0,
	}

	data := make([]byte, 2)
	data[0] = req.Version
	data[1] = req.OpCode

	// Send request
	conn, err := net.Dial("udp", net.JoinHostPort(c.Gateway.String(), fmt.Sprintf("%d", NATPMPClientPort)))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to connect to NAT-PMP gateway: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(data)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send NAT-PMP request: %w", err)
	}

	// Read response
	response := make([]byte, 16)
	n, err := conn.Read(response)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read NAT-PMP response: %w", err)
	}

	if n < 8 {
		return nil, 0, errors.New("response too short")
	}

	// Parse response
	resp := &NATPMPResponse{
		Version:   response[0],
		OpCode:    response[1],
		ResultCode: response[2],
		Epoch:     binary.BigEndian.Uint16(response[4:6]),
		Addresses: response[8:16],
	}

	// Check result
	if resp.ResultCode != NATPMPResultSuccess {
		return nil, 0, fmt.Errorf("NAT-PMP error: %d", resp.ResultCode)
	}

	// Update epoch
	c.LastEpoch = resp.Epoch

	// Extract IP (IPv4)
	extIP := net.IP(resp.Addresses[:4])

	log.Printf("[NAT-PMP] External address: %s (epoch=%d)", extIP.String(), resp.Epoch)

	return extIP, resp.Epoch, nil
}

// MapPort requests a port mapping
func (c *NATPMPClient) MapPort(protocol string, internalPort, externalPort int, ttl uint32) (uint16, error) {
	var opcode uint8 = NATPMPOpcodeMapTCP
	if protocol == "UDP" {
		opcode = NATPMPOpcodeMapUDP
	}

	log.Printf("[NAT-PMP] Requesting port mapping: protocol=%s, internal port=%d, external port=%d",
		protocol, internalPort, externalPort)

	// Build request
	mapReq := MapPortRequest{
		PrivatePort: uint16(internalPort),
		PublicPort:  uint16(externalPort),
		TTL:        ttl,
		Reserved:   [3]uint16{0, 0, 0},
	}

	// Serialize request
	reqHeader := NATPMPRequest{
		Version:  NATPMPVersion,
		OpCode:  opcode,
		Reserved: 0,
	}

	reqData := make([]byte, 12)
	reqData[0] = reqHeader.Version
	reqData[1] = reqHeader.OpCode
	binary.BigEndian.PutUint16(reqData[2:4], reqHeader.Reserved)
	binary.BigEndian.PutUint16(reqData[4:6], mapReq.PrivatePort)
	binary.BigEndian.PutUint16(reqData[6:8], mapReq.PublicPort)
	binary.BigEndian.PutUint32(reqData[8:12], mapReq.TTL)

	// Send request
	conn, err := net.Dial("udp", net.JoinHostPort(c.Gateway.String(), fmt.Sprintf("%d", NATPMPClientPort)))
	if err != nil {
		return 0, fmt.Errorf("failed to connect to NAT-PMP gateway: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(reqData)
	if err != nil {
		return 0, fmt.Errorf("failed to send NAT-PMP map request: %w", err)
	}

	// Read response
	response := make([]byte, 16)
	n, err := conn.Read(response)
	if err != nil {
		return 0, fmt.Errorf("failed to read NAT-PMP map response: %w", err)
	}

	if n < 16 {
		return 0, errors.New("map response too short")
	}

	// Parse response
	respHeader := &NATPMPResponse{
		Version:   response[0],
		OpCode:    response[1],
		ResultCode: response[2],
		Epoch:     binary.BigEndian.Uint16(response[4:6]),
	}

	// Parse mapping response
	if n >= 16 {
		mapResp := MapPortResponse{
			PrivatePort: binary.BigEndian.Uint16(response[8:10]),
			PublicPort:  binary.BigEndian.Uint16(response[10:12]),
			TTL:         binary.BigEndian.Uint32(response[12:16]),
		}

		// Check result
		if respHeader.ResultCode != NATPMPResultSuccess {
			return 0, fmt.Errorf("NAT-PMP map failed: result=%d", respHeader.ResultCode)
		}

		// Update epoch
		c.LastEpoch = respHeader.Epoch

		log.Printf("[NAT-PMP] ✓ Port mapping successful!")
		log.Printf("[NAT-PMP]   External port: %d", mapResp.PublicPort)
		log.Printf("[NAT-PMP]   Internal port: %d", mapResp.PrivatePort)
		log.Printf("[NAT-PMP]   TTL: %d seconds", mapResp.TTL)

		return mapResp.PublicPort, nil
	}

	return 0, fmt.Errorf("invalid response length")
}

// DeletePortMapping removes a port mapping (by setting TTL to 0)
func (c *NATPMPClient) DeletePortMapping(protocol string, internalPort int) error {
	log.Printf("[NAT-PMP] Deleting port mapping: protocol=%s, internal port=%d",
		protocol, internalPort)

	// Delete by requesting mapping with TTL=0
	_, err := c.MapPort(protocol, internalPort, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to delete NAT-PMP mapping: %w", err)
	}

	log.Printf("[NAT-PMP] Port mapping deleted successfully")
	return nil
}
