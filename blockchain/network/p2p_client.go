package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

type P2PClient struct {
	chainID       uint64
	rulesHash     string
	nodeID        string
	publicIP      string
	advertiseSelf bool

	dialTimeout time.Duration
	ioTimeout   time.Duration
	maxMsgBytes int
}

func NewP2PClient(chainID uint64, rulesHash string, nodeID string) *P2PClient {
	if strings.TrimSpace(nodeID) == "" {
		nodeID = "unknown"
	}
	publicIP, _ := detectPublicIP()
	advertiseSelf := envBool("P2P_ADVERTISE_SELF", true)
	return &P2PClient{
		chainID:       chainID,
		rulesHash:     strings.TrimSpace(rulesHash),
		nodeID:        nodeID,
		publicIP:      publicIP,
		advertiseSelf: advertiseSelf,
		dialTimeout:   5 * time.Second,
		ioTimeout:     10 * time.Second,
		maxMsgBytes:   4 << 20,
	}
}

func (c *P2PClient) do(ctx context.Context, peer string, reqType string, reqPayload any, resp any, expectedRespType string) error {
	peer = strings.TrimSpace(peer)
	if peer == "" {
		return errors.New("empty peer")
	}

	log.Printf("P2P client: dialing peer %s", peer)
	d := net.Dialer{Timeout: c.dialTimeout}
	conn, err := d.DialContext(ctx, "tcp", peer)
	if err != nil {
		log.Printf("P2P client: failed to dial %s: %v", peer, err)
		// Note: Connection failure is recorded by caller if they have access to PeerManager
		return err
	}
	defer conn.Close()
	log.Printf("P2P client: connected to %s (local addr: %s)", peer, conn.LocalAddr().String())

	_ = conn.SetDeadline(time.Now().Add(c.ioTimeout))

	// hello -> hello
	if err := p2pWriteJSON(conn, p2pEnvelope{Type: "hello", Payload: mustJSON(newP2PHello(c.chainID, c.rulesHash, c.nodeID))}); err != nil {
		return err
	}
	raw, err := p2pReadJSON(conn, 1<<20)
	if err != nil {
		return err
	}
	var env p2pEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if env.Type != "hello" {
		return errors.New("bad hello response")
	}
	var hello p2pHello
	if err := json.Unmarshal(env.Payload, &hello); err != nil {
		return err
	}
	if hello.Protocol != 1 || hello.ChainID != c.chainID {
		return errors.New("wrong chain/protocol")
	}
	if c.rulesHash != "" && hello.RulesHash != c.rulesHash {
		return errors.New("rules hash mismatch")
	}

	// CRITICAL FIX: Do NOT send addr message here
	// The addr message should only be sent in response to getaddr requests
	// Sending it here disrupts the request-response flow for sync operations
	// The P2P server will still advertise peers via handleGetAddr

	// request -> response
	var payload json.RawMessage
	if reqPayload != nil {
		payload = mustJSON(reqPayload)
	}
	if err := p2pWriteJSON(conn, p2pEnvelope{Type: reqType, Payload: payload}); err != nil {
		return err
	}
	raw, err = p2pReadJSON(conn, c.maxMsgBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if expectedRespType != "" && env.Type != expectedRespType {
		if env.Type == "not_found" {
			return errors.New("not found")
		}
		return errors.New("unexpected response type: " + env.Type)
	}
	if resp != nil {
		return json.Unmarshal(env.Payload, resp)
	}
	return nil
}

type p2pTxResponse struct {
	TxID string `json:"txid"`
}

type p2pBlockResponse struct {
	Hash string `json:"hash"`
}

func (c *P2PClient) RequestTransaction(ctx context.Context, peer string, tx core.Transaction) (string, error) {
	txJSON, err := json.Marshal(tx)
	if err != nil {
		return "", err
	}
	var resp p2pTxResponse
	err = c.do(ctx, peer, "tx_req", p2pTransactionReq{TxHex: string(txJSON)}, &resp, "tx_ack")
	if err != nil {
		return "", err
	}
	return resp.TxID, nil
}

func (c *P2PClient) BroadcastTransaction(ctx context.Context, peer string, tx core.Transaction) (string, error) {
	txJSON, err := json.Marshal(tx)
	if err != nil {
		return "", err
	}
	var resp p2pTxResponse
	err = c.do(ctx, peer, "tx_broadcast", p2pTransactionBroadcast{TxHex: string(txJSON)}, &resp, "tx_broadcast_ack")
	if err != nil {
		return "", err
	}
	return resp.TxID, nil
}

func (c *P2PClient) BroadcastBlock(ctx context.Context, peer string, block *core.Block) (string, error) {
	blockJSON, err := json.Marshal(block)
	if err != nil {
		return "", err
	}
	var resp p2pBlockResponse
	err = c.do(ctx, peer, "block_broadcast", p2pBlockBroadcast{BlockHex: string(blockJSON)}, &resp, "block_broadcast_ack")
	if err != nil {
		return "", err
	}
	return resp.Hash, nil
}

func (c *P2PClient) RequestBlock(ctx context.Context, peer string, hashHex string) (*core.Block, error) {
	var block core.Block
	err := c.do(ctx, peer, "block_req", p2pBlockReq{HashHex: hashHex}, &block, "block")
	if err != nil {
		return nil, err
	}
	return &block, nil
}

type peerAddr struct {
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	Timestamp int64  `json:"timestamp"`
}

func (c *P2PClient) sendAddrMessage(conn net.Conn) error {
	host, portStr, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		log.Printf("P2P client failed to parse local address: %v", err)
		return err
	}
	if host == "" || host == "0.0.0.0" {
		host = c.publicIP
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil || port <= 0 {
		port = 9090
	}
	addrMsg := map[string]any{
		"addresses": []peerAddr{
			{
				IP:        c.publicIP,
				Port:      port,
				Timestamp: time.Now().Unix(),
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_ = conn.SetDeadline(time.Now().Add(1 * time.Second))
		done <- p2pWriteJSON(conn, p2pEnvelope{Type: "addr", Payload: mustJSON(addrMsg)})
	}()
	select {
	case err := <-done:
		if err != nil {
			log.Printf("P2P client failed to send addr message: %v", err)
		}
		return err
	case <-ctx.Done():
		log.Printf("P2P client addr message send timeout")
		return ctx.Err()
	}
}
