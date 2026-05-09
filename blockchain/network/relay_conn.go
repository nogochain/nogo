// Package network provides P2P networking for NogoChain.
package network

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/p2p/discover"
)

const (
	relayConnReadTimeout  = 60 * time.Second
	relayConnWriteTimeout = 30 * time.Second
)

// relayConn adapts a relay tunnel session to the net.Conn interface.
// It allows the Switch to treat relay-tunneled connections identically
// to direct TCP connections for P2P message exchange.
type relayConn struct {
	client    *discover.RelayClient
	sessionID [8]byte
	peerID    string

	readCh  chan []byte
	closeCh chan struct{}

	readDeadline  time.Time
	writeDeadline time.Time
	deadlineMu    sync.RWMutex

	closed bool
	mu     sync.Mutex
}

// newRelayConn creates a relay connection wrapper for a relay tunnel session.
func newRelayConn(client *discover.RelayClient, sessionID [8]byte, peerID string) *relayConn {
	return &relayConn{
		client:    client,
		sessionID: sessionID,
		peerID:    peerID,
		readCh:    make(chan []byte, 64),
		closeCh:   make(chan struct{}),
	}
}

// relayDataDemuxer reads from the relay client's data channel and routes
// messages to the appropriate relayConn instances.
type relayDataDemuxer struct {
	client  *discover.RelayClient
	conns   map[[8]byte]*relayConn
	connsMu sync.RWMutex
	quit    chan struct{}
}

// newRelayDataDemuxer creates a demuxer that routes incoming relay data
// to the correct relay tunnel connection.
func newRelayDataDemuxer(client *discover.RelayClient, quit chan struct{}) *relayDataDemuxer {
	return &relayDataDemuxer{
		client: client,
		conns:  make(map[[8]byte]*relayConn),
		quit:   quit,
	}
}

// register adds a relayConn to the demuxer for receiving data.
func (d *relayDataDemuxer) register(sessionID [8]byte, rc *relayConn) {
	d.connsMu.Lock()
	d.conns[sessionID] = rc
	d.connsMu.Unlock()
}

// unregister removes a relayConn from the demuxer.
func (d *relayDataDemuxer) unregister(sessionID [8]byte) {
	d.connsMu.Lock()
	delete(d.conns, sessionID)
	d.connsMu.Unlock()
}

// run starts the demux loop, reading from the relay data channel
// and dispatching to registered relay connections.
func (d *relayDataDemuxer) run() {
	dataCh := d.client.DataChannel()
	for {
		select {
		case <-d.quit:
			return
		case msg, ok := <-dataCh:
			if !ok {
				return
			}
			if msg == nil {
				continue
			}
			d.connsMu.RLock()
			rc, exists := d.conns[msg.SessionID]
			d.connsMu.RUnlock()
			if !exists {
				continue
			}
			select {
			case rc.readCh <- msg.Data:
			case <-d.quit:
				return
			default:
				log.Printf("[RelayConn] read buffer full for session %x, dropping packet", msg.SessionID[:4])
			}
		}
	}
}

// Read implements net.Conn.Read by consuming data from the relay tunnel.
func (rc *relayConn) Read(b []byte) (int, error) {
	rc.mu.Lock()
	if rc.closed {
		rc.mu.Unlock()
		return 0, errors.New("relay connection closed")
	}
	rc.mu.Unlock()

	var timeout <-chan time.Time
	rc.deadlineMu.RLock()
	if !rc.readDeadline.IsZero() {
		timeout = time.After(time.Until(rc.readDeadline))
	}
	rc.deadlineMu.RUnlock()

	select {
	case <-rc.closeCh:
		return 0, errors.New("relay connection closed")
	case data, ok := <-rc.readCh:
		if !ok {
			return 0, errors.New("relay connection closed")
		}
		n := copy(b, data)
		return n, nil
	case <-timeout:
		return 0, fmt.Errorf("relay read timeout")
	}
}

// Write implements net.Conn.Write by sending data through the relay tunnel.
func (rc *relayConn) Write(b []byte) (int, error) {
	rc.mu.Lock()
	if rc.closed {
		rc.mu.Unlock()
		return 0, errors.New("relay connection closed")
	}
	rc.mu.Unlock()

	rc.deadlineMu.RLock()
	if !rc.writeDeadline.IsZero() && time.Now().After(rc.writeDeadline) {
		rc.deadlineMu.RUnlock()
		return 0, fmt.Errorf("relay write timeout")
	}
	rc.deadlineMu.RUnlock()

	if err := rc.client.SendRelayData(rc.sessionID, b); err != nil {
		return 0, fmt.Errorf("relay write: %w", err)
	}
	return len(b), nil
}

// Close implements net.Conn.Close by closing the relay tunnel session.
func (rc *relayConn) Close() error {
	rc.mu.Lock()
	if rc.closed {
		rc.mu.Unlock()
		return nil
	}
	rc.closed = true
	close(rc.closeCh)
	rc.mu.Unlock()

	rc.client.CloseSession(rc.sessionID)
	return nil
}

// LocalAddr implements net.Conn.LocalAddr.
func (rc *relayConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

// RemoteAddr implements net.Conn.RemoteAddr.
func (rc *relayConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

// SetDeadline implements net.Conn.SetDeadline.
func (rc *relayConn) SetDeadline(t time.Time) error {
	rc.deadlineMu.Lock()
	defer rc.deadlineMu.Unlock()
	rc.readDeadline = t
	rc.writeDeadline = t
	return nil
}

// SetReadDeadline implements net.Conn.SetReadDeadline.
func (rc *relayConn) SetReadDeadline(t time.Time) error {
	rc.deadlineMu.Lock()
	defer rc.deadlineMu.Unlock()
	rc.readDeadline = t
	return nil
}

// SetWriteDeadline implements net.Conn.SetWriteDeadline.
func (rc *relayConn) SetWriteDeadline(t time.Time) error {
	rc.deadlineMu.Lock()
	defer rc.deadlineMu.Unlock()
	rc.writeDeadline = t
	return nil
}
