package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

type P2PServer struct {
	bc *Blockchain
	pm PeerAPI
	mp *Mempool

	listenAddr string
	nodeID     string

	maxConns    int
	maxMsgSize  int
	sem         chan struct{}
	blockRecvCB func(*Block)
}

func NewP2PServer(bc *Blockchain, pm PeerAPI, mp *Mempool, listenAddr string, nodeID string) *P2PServer {
	if strings.TrimSpace(listenAddr) == "" {
		listenAddr = ":9090"
	}
	if strings.TrimSpace(nodeID) == "" {
		nodeID = bc.MinerAddress
	}
	s := &P2PServer{
		bc:         bc,
		pm:         pm,
		mp:         mp,
		listenAddr: listenAddr,
		nodeID:     nodeID,
		maxConns:   envInt("P2P_MAX_CONNECTIONS", 200),
		maxMsgSize: envInt("P2P_MAX_MESSAGE_BYTES", 4<<20),
	}
	if s.maxConns <= 0 {
		s.maxConns = 200
	}
	if s.maxMsgSize <= 0 {
		s.maxMsgSize = 4 << 20
	}
	s.sem = make(chan struct{}, s.maxConns)
	return s
}

func (s *P2PServer) ListenAddr() string { return s.listenAddr }

func (s *P2PServer) Serve(ctx context.Context) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", s.listenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Printf("P2P listening on %s (nodeId=%s)", s.listenAddr, s.nodeID)

	for {
		c, err := ln.Accept()
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
				_ = s.handleConn(c)
			}()
		default:
			_ = c.Close()
		}
	}
}

func (s *P2PServer) handleConn(c net.Conn) error {
	defer c.Close()

	_ = c.SetDeadline(time.Now().Add(15 * time.Second))

	// Expect hello first.
	raw, err := p2pReadJSON(c, 1<<20)
	if err != nil {
		return err
	}
	var env p2pEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}
	if env.Type != "hello" {
		return errors.New("expected hello")
	}
	var hello p2pHello
	if err := json.Unmarshal(env.Payload, &hello); err != nil {
		return err
	}
	if hello.Protocol != 1 || hello.ChainID != s.bc.ChainID {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "wrong_chain_or_protocol"})})
		return errors.New("wrong chain/protocol")
	}
	if strings.TrimSpace(hello.RulesHash) == "" || hello.RulesHash != s.bc.RulesHashHex() {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "rules_hash_mismatch"})})
		return errors.New("rules hash mismatch")
	}

	// Reply hello.
	_ = p2pWriteJSON(c, p2pEnvelope{Type: "hello", Payload: mustJSON(newP2PHello(s.bc.ChainID, s.bc.RulesHashHex(), s.nodeID))})

	// One request per connection (simple and safe).
	raw, err = p2pReadJSON(c, s.maxMsgSize)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}

	_ = c.SetDeadline(time.Now().Add(30 * time.Second))

	switch env.Type {
	case "chain_info_req":
		return s.writeChainInfo(c)
	case "headers_from_req":
		var req p2pHeadersFromReq
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return err
		}
		return s.writeHeadersFrom(c, req.From, req.Count)
	case "block_by_hash_req":
		var req p2pBlockByHashReq
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return err
		}
		return s.writeBlockByHash(c, req.HashHex)
	case "tx_req":
		return s.handleTransactionReq(c, env.Payload)
	case "tx_broadcast":
		return s.handleTransactionBroadcast(c, env.Payload)
	case "block_broadcast":
		return s.handleBlockBroadcast(c, env.Payload)
	case "block_req":
		return s.handleBlockReq(c, env.Payload)
	case "getaddr":
		return s.handleGetAddr(c)
	case "addr":
		return s.handleAddr(c, env.Payload)
	default:
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "unknown_type"})})
		return nil
	}
}

func (s *P2PServer) writeChainInfo(w io.Writer) error {
	latest := s.bc.LatestBlock()
	genesis, _ := s.bc.BlockByHeight(0)
	peersCount := 0
	if s.pm != nil {
		peersCount = len(s.pm.Peers())
	}
	out := map[string]any{
		"chainId":              s.bc.ChainID,
		"rulesHash":            s.bc.RulesHashHex(),
		"height":               latest.Height,
		"latestHash":           fmt.Sprintf("%x", latest.Hash),
		"genesisHash":          fmt.Sprintf("%x", genesis.Hash),
		"genesisTimestampUnix": genesis.TimestampUnix,
		"peersCount":           peersCount,
	}
	return p2pWriteJSON(w, p2pEnvelope{Type: "chain_info", Payload: mustJSON(out)})
}

func (s *P2PServer) writeHeadersFrom(w io.Writer, from uint64, count int) error {
	if count <= 0 || count > 500 {
		count = 100
	}
	headers := s.bc.HeadersFrom(from, count)
	return p2pWriteJSON(w, p2pEnvelope{Type: "headers", Payload: mustJSON(headers)})
}

func (s *P2PServer) writeBlockByHash(w io.Writer, hashHex string) error {
	hashHex = strings.TrimSpace(hashHex)
	if hashHex == "" {
		return p2pWriteJSON(w, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "missing_hash"})})
	}
	b, ok := s.bc.BlockByHash(hashHex)
	if !ok {
		return p2pWriteJSON(w, p2pEnvelope{Type: "not_found", Payload: mustJSON(map[string]any{"hashHex": hashHex})})
	}
	return p2pWriteJSON(w, p2pEnvelope{Type: "block", Payload: mustJSON(b)})
}

func (s *P2PServer) handleTransactionReq(c net.Conn, payload json.RawMessage) error {
	var req p2pTransactionReq
	if err := json.Unmarshal(payload, &req); err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_payload"})})
		return err
	}

	var tx Transaction
	if err := json.Unmarshal([]byte(req.TxHex), &tx); err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_json"})})
		return err
	}

	txid, err := TxIDHex(tx)
	if err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_tx"})})
		return err
	}

	if s.mp != nil {
		_, _ = s.mp.Add(tx)
	}

	return p2pWriteJSON(c, p2pEnvelope{Type: "tx_ack", Payload: mustJSON(map[string]any{"txid": txid})})
}

func (s *P2PServer) handleTransactionBroadcast(c net.Conn, payload json.RawMessage) error {
	var broadcast p2pTransactionBroadcast
	if err := json.Unmarshal(payload, &broadcast); err != nil {
		return err
	}

	var tx Transaction
	if err := json.Unmarshal([]byte(broadcast.TxHex), &tx); err != nil {
		return err
	}

	txid, err := TxIDHex(tx)
	if err != nil {
		return err
	}

	if s.mp != nil {
		_, _ = s.mp.Add(tx)
	}

	return p2pWriteJSON(c, p2pEnvelope{Type: "tx_broadcast_ack", Payload: mustJSON(map[string]any{"txid": txid})})
}

func (s *P2PServer) handleBlockBroadcast(c net.Conn, payload json.RawMessage) error {
	var broadcast p2pBlockBroadcast
	if err := json.Unmarshal(payload, &broadcast); err != nil {
		return err
	}

	var block Block
	if err := json.Unmarshal([]byte(broadcast.BlockHex), &block); err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_block_json"})})
		return err
	}

	if s.bc != nil {
		_, err := s.bc.AddBlock(&block)
		if err != nil {
			log.Printf("p2p block broadcast add result: %v", err)
		}
	}

	if s.blockRecvCB != nil {
		s.blockRecvCB(&block)
	}

	return p2pWriteJSON(c, p2pEnvelope{Type: "block_broadcast_ack", Payload: mustJSON(map[string]any{"hash": fmt.Sprintf("%x", block.Hash)})})
}

func (s *P2PServer) handleBlockReq(c net.Conn, payload json.RawMessage) error {
	var req p2pBlockReq
	if err := json.Unmarshal(payload, &req); err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_payload"})})
		return err
	}

	b, ok := s.bc.BlockByHash(req.HashHex)
	if !ok {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "not_found", Payload: mustJSON(map[string]any{"hashHex": req.HashHex})})
		return nil
	}

	blockJSON, err := json.Marshal(b)
	if err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "marshal_failed"})})
		return err
	}

	return p2pWriteJSON(c, p2pEnvelope{Type: "block", Payload: blockJSON})
}

func (s *P2PServer) handleGetAddr(c net.Conn) error {
	if s.pm == nil {
		return p2pWriteJSON(c, p2pEnvelope{Type: "addr", Payload: mustJSON(map[string]any{"addresses": []struct{}{}})})
	}
	type peerAddr struct {
		IP        string `json:"ip"`
		Port      int    `json:"port"`
		Timestamp int64  `json:"timestamp"`
	}
	var peerAddrs []peerAddr
	for _, addr := range s.pm.Peers() {
		host, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			continue
		}
		var port int
		fmt.Sscanf(portStr, "%d", &port)
		peerAddrs = append(peerAddrs, peerAddr{
			IP:        host,
			Port:      port,
			Timestamp: time.Now().Unix(),
		})
	}
	return p2pWriteJSON(c, p2pEnvelope{Type: "addr", Payload: mustJSON(map[string]any{"addresses": peerAddrs})})
}

func (s *P2PServer) handleAddr(c net.Conn, payload json.RawMessage) error {
	if s.pm == nil {
		return nil
	}
	type addrMsg struct {
		Addresses []struct {
			IP        string `json:"ip"`
			Port      int    `json:"port"`
			Timestamp int64  `json:"timestamp"`
		} `json:"addresses"`
	}
	var msg addrMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil
	}
	for _, a := range msg.Addresses {
		addr := fmt.Sprintf("%s:%d", a.IP, a.Port)
		if addr != "" && addr != ":" {
			s.pm.AddPeer(addr)
		}
	}
	return p2pWriteJSON(c, p2pEnvelope{Type: "addr_ack", Payload: nil})
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
