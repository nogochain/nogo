package discover

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/p2p/discover/dht"
)

const (
	mdnsService         = "_nogochain._tcp"
	mdnsGroup           = "224.0.0.251"
	mdnsPort            = 5353
	mdnsQueryInterval   = 60 * time.Second
	mdnsBufferSize      = 512
	mdnsAnnounceTTL     = 120
)

// mDNS handles LAN peer discovery using a simplified multicast DNS protocol.
type mDNS struct {
	mu          sync.Mutex
	localNode   *dht.Node
	peerCh      chan *dht.Node
	conn        *net.UDPConn
	quit        chan struct{}
	running     bool
}

// newMDNS creates a new mDNS instance bound to the given local node.
func newMDNS(localNode *dht.Node, peerCh chan *dht.Node) (*mDNS, error) {
	addr := &net.UDPAddr{
		IP:   net.IPv4(224, 0, 0, 251),
		Port: mdnsPort,
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("mdns: listen multicast: %w", err)
	}

	return &mDNS{
		localNode: localNode,
		peerCh:    peerCh,
		conn:      conn,
		quit:      make(chan struct{}),
	}, nil
}

// start begins the mDNS query and announce loops.
func (m *mDNS) start() {
	m.running = true
	go m.queryLoop()
	go m.announceLoop()
	log.Printf("[mDNS] LAN peer discovery started (service=%s)", mdnsService)
}

// stop shuts down the mDNS service.
func (m *mDNS) stop() {
	if !m.running {
		return
	}
	close(m.quit)
	m.conn.Close()
	m.running = false
}

// queryLoop sends periodic mDNS queries to discover LAN peers.
func (m *mDNS) queryLoop() {
	ticker := time.NewTicker(mdnsQueryInterval)
	defer ticker.Stop()

	query := buildMDNSQuery(mdnsService)

	for {
		select {
		case <-m.quit:
			return
		case <-ticker.C:
			m.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if _, err := m.conn.Write(query); err != nil {
				log.Printf("[mDNS] query send: %v", err)
				continue
			}
			m.readResponses()
		}
	}
}

// announceLoop periodically broadcasts the local node's presence on the LAN.
func (m *mDNS) announceLoop() {
	ticker := time.NewTicker(mdnsQueryInterval / 2)
	defer ticker.Stop()

	announce := buildMDNSAnnounce(m.localNode, mdnsService)
	for {
		select {
		case <-m.quit:
			return
		case <-ticker.C:
			m.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			m.conn.Write(announce)
		}
	}
}

// readResponses reads incoming mDNS responses and extracts peer nodes.
func (m *mDNS) readResponses() {
	buf := make([]byte, mdnsBufferSize)
	m.conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	for {
		n, from, err := m.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			continue
		}
		// Skip own announcements
		if from.IP.Equal(m.localNode.IP) && int(from.Port) == int(m.localNode.TCP) {
			continue
		}
		node := parseMDNSResponse(buf[:n], from)
		if node != nil {
			select {
			case m.peerCh <- node:
			default:
			}
		}
	}
}

// buildMDNSQuery constructs a simple mDNS query for the service.
func buildMDNSQuery(service string) []byte {
	// Simplified mDNS query: 12-byte header + question
	msg := make([]byte, 12+len(service)+6)
	msg[0] = 0x00 // transaction ID high
	msg[1] = 0x00 // transaction ID low
	msg[2] = 0x00 // flags
	msg[5] = 0x01 // 1 question

	offset := 12
	// Encode service name as labels
	for _, part := range strings.Split(service, ".") {
		msg[offset] = byte(len(part))
		offset++
		copy(msg[offset:], []byte(part))
		offset += len(part)
	}
	msg[offset] = 0x00 // terminate name
	offset++
	msg[offset] = 0x00 // QTYPE PTR high
	msg[offset+1] = 0x0c
	msg[offset+2] = 0x00 // QCLASS IN high
	msg[offset+3] = 0x01

	return msg
}

// buildMDNSAnnounce creates an mDNS announcement for the local node.
func buildMDNSAnnounce(node *dht.Node, service string) []byte {
	enode := node.Enode()
	msg := make([]byte, 12+len(service)+len(enode)+10)
	msg[2] = 0x84 // response + authoritative
	offset := 12

	for _, part := range strings.Split(service, ".") {
		msg[offset] = byte(len(part))
		offset++
		copy(msg[offset:], []byte(part))
		offset += len(part)
	}
	msg[offset] = 0x00
	offset++
	msg[offset] = 0x10 // TXT record
	msg[offset+1] = 0x00
	msg[offset+2] = 0x00
	msg[offset+3] = 0x01 // IN
	msg[offset+4] = 0x00
	msg[offset+5] = byte(mdnsAnnounceTTL >> 24)
	msg[offset+6] = byte(mdnsAnnounceTTL >> 16)
	msg[offset+7] = byte(mdnsAnnounceTTL >> 8)
	msg[offset+8] = byte(mdnsAnnounceTTL)
	msg[offset+9] = byte(len(enode))
	msg[offset+10] = byte(len(enode))
	offset += 11
	copy(msg[offset:], []byte(enode))

	return msg
}

// parseMDNSResponse extracts a Node from an mDNS response packet.
func parseMDNSResponse(data []byte, from *net.UDPAddr) *dht.Node {
	// Search for enode:// prefix in the response data
	str := string(data)
	idx := strings.Index(str, "enode://")
	if idx < 0 {
		return nil
	}

	enodeEnd := idx
	for enodeEnd < len(str) && str[enodeEnd] != 0 && str[enodeEnd] > 32 {
		enodeEnd++
	}
	enode := str[idx:enodeEnd]
	return parseEnode(enode)
}
