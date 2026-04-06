package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	p2pSecureVersion       = 1
	p2pSecureHandshakeSize = 4 + 32 + 64 + 8
	p2pMaxMessageSize      = 4 << 20
)

type p2pSecureHello struct {
	Version     uint8    `json:"version"`
	Protocol    uint32   `json:"protocol"`
	ChainID     uint64   `json:"chainId"`
	RulesHash   string   `json:"rulesHash"`
	NodeID      string   `json:"nodeId"`
	TimeUnix    int64    `json:"timeUnix"`
	PubKey      string   `json:"pubKey"`
	Signature   string   `json:"signature"`
	ListenAddrs []string `json:"listenAddrs,omitempty"`
}

type p2pSecureServer struct {
	bc         BlockchainInterface
	nodeID     string
	privKey    ed25519.PrivateKey
	pubKey     ed25519.PublicKey
	listenAddr string
	tlsConfig  *tls.Config

	maxConns int
	sem      chan struct{}
	peers    map[string]*securePeer
	peersMu  sync.RWMutex
}

type securePeer struct {
	NodeID   string
	PubKey   ed25519.PublicKey
	Addr     net.Addr
	Conn     net.Conn
	Score    float64
	LastSeen time.Time
	Latency  time.Duration
}

func NewSecureP2PServer(bc BlockchainInterface, nodeID string, privKey ed25519.PrivateKey, listenAddr string, tlsCert, tlsKey string) (*p2pSecureServer, error) {
	s := &p2pSecureServer{
		bc:         bc,
		nodeID:     nodeID,
		privKey:    privKey,
		pubKey:     privKey.Public().(ed25519.PublicKey),
		listenAddr: listenAddr,
		maxConns:   200,
		peers:      make(map[string]*securePeer),
	}

	if tlsCert != "" && tlsKey != "" {
		cert, err := tls.LoadX509KeyPair(tlsCert, tlsKey)
		if err != nil {
			return nil, fmt.Errorf("load TLS cert: %w", err)
		}
		s.tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
			ClientAuth:   tls.RequestClientCert,
		}
	}

	if s.maxConns <= 0 {
		s.maxConns = 200
	}
	s.sem = make(chan struct{}, s.maxConns)

	return s, nil
}

func (s *p2pSecureServer) Serve(ctx context.Context) error {
	network := "tcp"
	if s.tlsConfig != nil {
		network = "tcp4"
	}

	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, network, s.listenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Printf("Secure P2P listening on %s (nodeId=%s, tls=%v)", s.listenAddr, s.nodeID, s.tlsConfig != nil)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}

		select {
		case s.sem <- struct{}{}:
			go func() {
				defer func() { <-s.sem }()
				if err := s.handleConn(conn); err != nil {
					log.Printf("p2p secure server: handleConn error: %v", err)
				}
			}()
		default:
			if closeErr := conn.Close(); closeErr != nil {
				log.Printf("p2p secure server: failed to close connection: %v", closeErr)
			}
		}
	}
}

func (s *p2pSecureServer) handleConn(conn net.Conn) error {
	defer conn.Close()

	var rawConn net.Conn = conn
	if s.tlsConfig != nil {
		tlsConn := conn.(*tls.Conn)
		if err := tlsConn.Handshake(); err != nil {
			return fmt.Errorf("TLS handshake: %w", err)
		}
		rawConn = tlsConn
	}

	_ = rawConn.SetDeadline(time.Now().Add(15 * time.Second))

	helloBytes, err := p2pSecureRead(rawConn)
	if err != nil {
		return fmt.Errorf("read hello: %w", err)
	}

	var hello p2pSecureHello
	if err := json.Unmarshal(helloBytes, &hello); err != nil {
		return fmt.Errorf("parse hello: %w", err)
	}

	if err := s.validateHello(&hello); err != nil {
		_ = p2pSecureWrite(rawConn, []byte(`{"error":"`+err.Error()+`"}`))
		return fmt.Errorf("validate hello: %w", err)
	}

	peerPubKey, err := base64.StdEncoding.DecodeString(hello.PubKey)
	if err != nil {
		return fmt.Errorf("decode pubkey: %w", err)
	}

	sig, err := base64.StdEncoding.DecodeString(hello.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	sigData := fmt.Sprintf("%d|%d|%s|%s|%d", hello.Version, hello.ChainID, hello.RulesHash, hello.NodeID, hello.TimeUnix)
	if !ed25519.Verify(peerPubKey, []byte(sigData), sig) {
		return fmt.Errorf("invalid signature")
	}

	respHello := s.newHello()
	respBytes, _ := json.Marshal(respHello)
	_ = p2pSecureWrite(rawConn, respBytes)

	_ = rawConn.SetDeadline(time.Now().Add(30 * time.Second))

	peer := &securePeer{
		NodeID:   hello.NodeID,
		PubKey:   peerPubKey,
		Addr:     conn.RemoteAddr(),
		Conn:     rawConn,
		Score:    1.0,
		LastSeen: time.Now(),
	}

	s.addPeer(peer)
	defer s.removePeer(peer.NodeID)

	return s.handlePeerMessages(rawConn, peer)
}

func (s *p2pSecureServer) validateHello(h *p2pSecureHello) error {
	if h.Version != p2pSecureVersion {
		return fmt.Errorf("unsupported version")
	}
	if h.Protocol != 1 {
		return fmt.Errorf("unsupported protocol")
	}
	if h.ChainID != s.bc.GetChainID() {
		return fmt.Errorf("wrong chain")
	}
	if strings.TrimSpace(h.RulesHash) == "" || h.RulesHash != s.bc.RulesHashHex() {
		return fmt.Errorf("rules hash mismatch")
	}
	if time.Now().Unix()-h.TimeUnix > 300 {
		return fmt.Errorf("stale hello")
	}
	return nil
}

func (s *p2pSecureServer) newHello() p2pSecureHello {
	sigData := fmt.Sprintf("%d|%d|%s|%s|%d", p2pSecureVersion, s.bc.GetChainID(), s.bc.RulesHashHex(), s.nodeID, time.Now().Unix())
	sig := ed25519.Sign(s.privKey, []byte(sigData))

	return p2pSecureHello{
		Version:   p2pSecureVersion,
		Protocol:  1,
		ChainID:   s.bc.GetChainID(),
		RulesHash: s.bc.RulesHashHex(),
		NodeID:    s.nodeID,
		TimeUnix:  time.Now().Unix(),
		PubKey:    base64.StdEncoding.EncodeToString(s.pubKey),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}
}

func (s *p2pSecureServer) addPeer(p *securePeer) {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()
	s.peers[p.NodeID] = p
}

func (s *p2pSecureServer) removePeer(nodeID string) {
	s.peersMu.Lock()
	defer s.peersMu.Unlock()
	delete(s.peers, nodeID)
}

func (s *p2pSecureServer) handlePeerMessages(conn net.Conn, peer *securePeer) error {
	for {
		data, err := p2pSecureRead(conn)
		if err != nil {
			return err
		}

		var msg map[string]json.RawMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		peer.LastSeen = time.Now()

		if typ, ok := msg["type"]; ok {
			var typeStr string
			json.Unmarshal(typ, &typeStr)
			switch typeStr {
			case "chain_info_req":
				latest := s.bc.LatestBlock()
				info := map[string]any{
					"type":       "chain_info",
					"chainId":    s.bc.GetChainID(),
					"height":     latest.Height,
					"latestHash": fmt.Sprintf("%x", latest.Hash),
					"rulesHash":  s.bc.RulesHashHex(),
				}
				infoBytes, _ := json.Marshal(info)
				_ = p2pSecureWrite(conn, infoBytes)
			case "ping":
				_ = p2pSecureWrite(conn, []byte(`{"type":"pong"}`))
			}
		}
	}
}

func p2pSecureRead(conn net.Conn) ([]byte, error) {
	var size [4]byte
	if _, err := conn.Read(size[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(size[:])
	if n > p2pMaxMessageSize {
		return nil, fmt.Errorf("message too large")
	}
	buf := make([]byte, n)
	if _, err := conn.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func p2pSecureWrite(conn net.Conn, data []byte) error {
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(data)))
	if _, err := conn.Write(size[:]); err != nil {
		return err
	}
	_, err := conn.Write(data)
	return err
}

func GenerateP2PKey() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return privKey, pubKey, nil
}
