package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"
)

type P2PClient struct {
	chainID   uint64
	rulesHash string
	nodeID    string

	dialTimeout time.Duration
	ioTimeout   time.Duration
	maxMsgBytes int
}

func NewP2PClient(chainID uint64, rulesHash string, nodeID string) *P2PClient {
	if strings.TrimSpace(nodeID) == "" {
		nodeID = "unknown"
	}
	return &P2PClient{
		chainID:     chainID,
		rulesHash:   strings.TrimSpace(rulesHash),
		nodeID:      nodeID,
		dialTimeout: 5 * time.Second,
		ioTimeout:   10 * time.Second,
		maxMsgBytes: 4 << 20,
	}
}

func (c *P2PClient) do(ctx context.Context, peer string, reqType string, reqPayload any, resp any, expectedRespType string) error {
	peer = strings.TrimSpace(peer)
	if peer == "" {
		return errors.New("empty peer")
	}

	d := net.Dialer{Timeout: c.dialTimeout}
	conn, err := d.DialContext(ctx, "tcp", peer)
	if err != nil {
		return err
	}
	defer conn.Close()

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

func (c *P2PClient) RequestTransaction(ctx context.Context, peer string, tx Transaction) (string, error) {
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

func (c *P2PClient) BroadcastTransaction(ctx context.Context, peer string, tx Transaction) (string, error) {
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

func (c *P2PClient) BroadcastBlock(ctx context.Context, peer string, block *Block) (string, error) {
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

func (c *P2PClient) RequestBlock(ctx context.Context, peer string, hashHex string) (*Block, error) {
	var block Block
	err := c.do(ctx, peer, "block_req", p2pBlockReq{HashHex: hashHex}, &block, "block")
	if err != nil {
		return nil, err
	}
	return &block, nil
}
