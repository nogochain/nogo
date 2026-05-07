// Package discover provides peer discovery for the NogoChain P2P network.
package discover

import (
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

// NATType classifies the NAT behavior.
type NATType int

const (
	NATTypeUnknown NATType = iota
	NATTypePublic          // No NAT — directly reachable
	NATTypeFullCone        // Full cone NAT — any external can send to internal
	NATTypeRestricted      // Restricted cone — only sent-to IP can reply
	NATTypePortRestricted  // Port-restricted cone — sent-to IP:port must reply
	NATTypeSymmetric       // Symmetric NAT — different mapping per destination
)

func (n NATType) String() string {
	switch n {
	case NATTypePublic:
		return "Public"
	case NATTypeFullCone:
		return "FullCone"
	case NATTypeRestricted:
		return "Restricted"
	case NATTypePortRestricted:
		return "PortRestricted"
	case NATTypeSymmetric:
		return "Symmetric"
	default:
		return "Unknown"
	}
}

// NATResult holds the result of NAT detection.
type NATResult struct {
	Type          NATType
	LocalIP       string
	LocalPort     int
	ExternalIP    string
	ExternalPort  int
	UPnPAvailable bool
	PortForwards  bool
}

// NATDetector performs NAT detection using STUN (RFC 5389).
type NATDetector struct {
	mu         sync.RWMutex
	tcpPort    int
	servers    []string
	lastResult NATResult
}

// NewNATDetector creates a new NAT detector.
func NewNATDetector(tcpPort int, stunServers []string) *NATDetector {
	if len(stunServers) == 0 {
		stunServers = DefaultSTUNServers()
	}
	return &NATDetector{
		tcpPort: tcpPort,
		servers: stunServers,
	}
}

// DefaultSTUNServers returns well-known public STUN servers.
func DefaultSTUNServers() []string {
	return []string{
		"stun.l.google.com:19302",
		"stun1.l.google.com:19302",
		"stun2.l.google.com:19302",
		"stun3.l.google.com:19302",
	}
}

// Detect performs NAT detection using STUN.
// It queries multiple STUN servers and classifies the NAT type.
func (nd *NATDetector) Detect() NATResult {
	localIP, localPort := nd.getLocalAddr()

	shuffled := make([]string, len(nd.servers))
	copy(shuffled, nd.servers)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	var (
		extIP        string
		extPort      int
		successCount int
	)

	for i, server := range shuffled {
		log.Printf("[STUN] Testing STUN server %d/%d: %s", i+1, len(shuffled), server)

		resp, err := nd.stunRequest(server)
		if err != nil {
			log.Printf("[STUN] Server %s failed: %v", server, err)
			continue
		}

		successCount++
		extIP = resp.mappedAddr.IP.String()
		extPort = int(resp.mappedAddr.Port)

		if i == 0 {
			break
		}
	}

	result := NATResult{
		LocalIP:      localIP,
		LocalPort:    localPort,
		ExternalIP:   extIP,
		ExternalPort: extPort,
	}

	if successCount == 0 {
		result.Type = NATTypeUnknown
		log.Printf("[STUN] All STUN servers failed — assuming NAT")
	} else if localIP == extIP && localPort == extPort {
		result.Type = NATTypePublic
		result.PortForwards = true
		log.Printf("[STUN] No NAT: %s:%d == %s:%d", localIP, localPort, extIP, extPort)
	} else if extPort != localPort {
		result.Type = NATTypeSymmetric
		result.PortForwards = false
		log.Printf("[STUN] Symmetric NAT: %s:%d -> %s:%d", localIP, localPort, extIP, extPort)
	} else {
		result.Type = NATTypePortRestricted
		result.PortForwards = false
		log.Printf("[STUN] NAT detected: %s:%d -> %s:%d (type: %s)", localIP, localPort, extIP, extPort, result.Type)
	}

	nd.mu.Lock()
	nd.lastResult = result
	nd.mu.Unlock()

	return result
}

// LastResult returns the most recent NAT detection result.
func (nd *NATDetector) LastResult() NATResult {
	nd.mu.RLock()
	defer nd.mu.RUnlock()
	return nd.lastResult
}

func (nd *NATDetector) stunRequest(server string) (*stunResponse, error) {
	addr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	msg := nd.buildBindingRequest()
	if _, err := conn.WriteToUDP(msg, addr); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	return nd.parseStunResponse(buf[:n])
}

type stunResponse struct {
	mappedAddr *net.UDPAddr
	xorAddr    *net.UDPAddr
	changedAddr *net.UDPAddr
}

// buildBindingRequest creates a STUN Binding Request (RFC 5389).
func (nd *NATDetector) buildBindingRequest() []byte {
	msg := make([]byte, 20)
	// Message type: 0x0001 = Binding Request
	msg[0] = 0x00
	msg[1] = 0x01
	// Message length (attributes only)
	msg[2] = 0x00
	msg[3] = 0x00
	// Magic cookie: 0x2112A442 (RFC 5389)
	msg[4] = 0x21
	msg[5] = 0x12
	msg[6] = 0xA4
	msg[7] = 0x42
	// Transaction ID: 12 random bytes
	rand.Read(msg[8:])
	return msg
}

// parseStunResponse parses a STUN Binding Response (RFC 5389).
func (nd *NATDetector) parseStunResponse(data []byte) (*stunResponse, error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("response too short: %d bytes", len(data))
	}
	// Check message type: 0x0101 = Binding Success Response
	if data[0] != 0x01 || data[1] != 0x01 {
		return nil, fmt.Errorf("not a STUN success response: 0x%02x%02x", data[0], data[1])
	}
	// Magic cookie must be 0x2112A442
	if data[4] != 0x21 || data[5] != 0x12 || data[6] != 0xA4 || data[7] != 0x42 {
		return nil, fmt.Errorf("invalid STUN magic cookie")
	}

	resp := &stunResponse{}
	msgLen := binary.BigEndian.Uint16(data[2:4])
	if int(msgLen)+20 > len(data) {
		return nil, fmt.Errorf("invalid message length")
	}

	offset := 20
	for offset < len(data) {
		if offset+4 > len(data) {
			break
		}
		attrType := binary.BigEndian.Uint16(data[offset:])
		attrLen := binary.BigEndian.Uint16(data[offset+2:])
		offset += 4

		if offset+int(attrLen) > len(data) {
			break
		}

		switch attrType {
		case 0x0001: // MAPPED-ADDRESS (RFC 3489)
			if attrLen >= 8 {
				port := binary.BigEndian.Uint16(data[offset+2:])
				ip := net.IPv4(data[offset+4], data[offset+5], data[offset+6], data[offset+7])
				resp.mappedAddr = &net.UDPAddr{IP: ip, Port: int(port)}
			}
		case 0x0020: // XOR-MAPPED-ADDRESS (RFC 5389)
			if attrLen >= 8 {
				xorPort := binary.BigEndian.Uint16(data[offset+2:]) ^ 0x2112
				xorIP := [4]byte{
					data[offset+4] ^ 0x21,
					data[offset+5] ^ 0x12,
					data[offset+6] ^ 0xA4,
					data[offset+7] ^ 0x42,
				}
				resp.xorAddr = &net.UDPAddr{IP: net.IP(xorIP[:]), Port: int(xorPort)}
			}
		case 0x0004: // CHANGED-ADDRESS
			if attrLen >= 8 {
				port := binary.BigEndian.Uint16(data[offset+2:]) ^ 0x2112
				ip := net.IPv4(
					data[offset+4]^0x21,
					data[offset+5]^0x12,
					data[offset+6]^0xA4,
					data[offset+7]^0x42,
				)
				resp.changedAddr = &net.UDPAddr{IP: ip, Port: int(port)}
			}
		}

		offset += int(attrLen)
		if attrLen%4 != 0 {
			offset += 4 - int(attrLen%4)
		}
	}

	// Prefer XOR-MAPPED-ADDRESS
	if resp.xorAddr != nil {
		resp.mappedAddr = resp.xorAddr
	}

	if resp.mappedAddr == nil {
		return nil, fmt.Errorf("no MAPPED-ADDRESS in response")
	}
	return resp, nil
}

func (nd *NATDetector) getLocalAddr() (string, int) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1", nd.tcpPort
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip := ipnet.IP.To4(); ip != nil {
				return ip.String(), nd.tcpPort
			}
		}
	}
	return "127.0.0.1", nd.tcpPort
}
