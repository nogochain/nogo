package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type p2pSecureClient struct {
	nodeID    string
	privKey   ed25519.PrivateKey
	pubKey    ed25519.PublicKey
	tlsConfig *tls.Config

	conn     net.Conn
	mu       sync.Mutex
	peerInfo *securePeerInfo
}

type securePeerInfo struct {
	NodeID  string
	PubKey  ed25519.PublicKey
	Addr    string
	Latency time.Duration
	Score   float64
}

func NewSecureP2PClient(nodeID string, privKey ed25519.PrivateKey, tlsInsecure bool) *p2pSecureClient {
	c := &p2pSecureClient{
		nodeID:  nodeID,
		privKey: privKey,
		pubKey:  privKey.Public().(ed25519.PublicKey),
	}
	if tlsInsecure {
		c.tlsConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return c
}

func (c *p2pSecureClient) Connect(ctx context.Context, addr string) error {
	network := "tcp"
	if c.tlsConfig != nil {
		network = "tcp4"
	}

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	var rawConn net.Conn = conn
	if c.tlsConfig != nil {
		tlsConn := conn.(*tls.Conn)
		if err := tlsConn.Handshake(); err != nil {
			return fmt.Errorf("TLS handshake: %w", err)
		}
		rawConn = tlsConn
	}

	c.conn = rawConn
	return c.handshake()
}

func (c *p2pSecureClient) handshake() error {
	_ = c.conn.SetDeadline(time.Now().Add(15 * time.Second))

	hello := c.newHello()
	helloBytes, _ := json.Marshal(hello)
	_ = p2pSecureWrite(c.conn, helloBytes)

	respBytes, err := p2pSecureRead(c.conn)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if errMsg, ok := resp["error"]; ok {
		return fmt.Errorf("server error: %v", errMsg)
	}

	c.peerInfo = &securePeerInfo{
		Addr:    c.conn.RemoteAddr().String(),
		Latency: time.Since(time.Now().Add(-15 * time.Second)),
		Score:   1.0,
	}

	_ = c.conn.SetDeadline(time.Now().Add(30 * time.Second))
	return nil
}

func (c *p2pSecureClient) newHello() p2pSecureHello {
	sigData := fmt.Sprintf("%d|%d|%s|%s|%d", p2pSecureVersion, 1, "", c.nodeID, time.Now().Unix())
	sig := ed25519.Sign(c.privKey, []byte(sigData))

	return p2pSecureHello{
		Version:   p2pSecureVersion,
		Protocol:  1,
		ChainID:   1,
		RulesHash: "",
		NodeID:    c.nodeID,
		TimeUnix:  time.Now().Unix(),
		PubKey:    base64.StdEncoding.EncodeToString(c.pubKey),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}
}

func (c *p2pSecureClient) RequestChainInfo() (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := map[string]any{"type": "chain_info_req"}
	reqBytes, _ := json.Marshal(req)
	_ = p2pSecureWrite(c.conn, reqBytes)

	respBytes, err := p2pSecureRead(c.conn)
	if err != nil {
		return nil, err
	}

	var resp map[string]any
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *p2pSecureClient) RequestHeaders(from uint64, count int) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := map[string]any{
		"type":  "headers_from_req",
		"from":  from,
		"count": count,
	}
	reqBytes, _ := json.Marshal(req)
	_ = p2pSecureWrite(c.conn, reqBytes)

	return p2pSecureRead(c.conn)
}

func (c *p2pSecureClient) RequestBlock(hashHex string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := map[string]any{
		"type":    "block_by_hash_req",
		"hashHex": hashHex,
	}
	reqBytes, _ := json.Marshal(req)
	_ = p2pSecureWrite(c.conn, reqBytes)

	return p2pSecureRead(c.conn)
}

func (c *p2pSecureClient) BroadcastTx(txHex string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	msg := map[string]any{
		"type":  "tx_broadcast",
		"txHex": txHex,
	}
	msgBytes, _ := json.Marshal(msg)
	_ = p2pSecureWrite(c.conn, msgBytes)
	return nil
}

func (c *p2pSecureClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *p2pSecureClient) Latency() time.Duration {
	if c.peerInfo != nil {
		return c.peerInfo.Latency
	}
	return 0
}

func (c *p2pSecureClient) Score() float64 {
	if c.peerInfo != nil {
		return c.peerInfo.Score
	}
	return 0
}

func GenerateP2PKeys() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}

var _ io.Closer = (*p2pSecureClient)(nil)
