package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

type P2PPeerManager struct {
	peers  []string
	client *P2PClient
}

func ParseP2PPeersEnv(peersEnv string) []string {
	var peers []string
	for _, raw := range strings.Split(peersEnv, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		peers = append(peers, raw)
	}
	return peers
}

func NewP2PPeerManager(chainID uint64, rulesHash string, nodeID string, peers []string) *P2PPeerManager {
	return &P2PPeerManager{
		peers:  peers,
		client: NewP2PClient(chainID, rulesHash, nodeID),
	}
}

func (pm *P2PPeerManager) Peers() []string { return append([]string(nil), pm.peers...) }

func (pm *P2PPeerManager) AddPeer(addr string) {
	if addr == "" {
		return
	}
	for _, p := range pm.peers {
		if p == addr {
			return
		}
	}
	pm.peers = append(pm.peers, addr)
}

func (pm *P2PPeerManager) FetchChainInfo(ctx context.Context, peer string) (*chainInfo, error) {
	var out chainInfo
	if err := pm.client.do(ctx, peer, "chain_info_req", p2pChainInfoReq{}, &out, "chain_info"); err != nil {
		return nil, err
	}
	return &out, nil
}

func (pm *P2PPeerManager) FetchHeadersFrom(ctx context.Context, peer string, fromHeight uint64, count int) ([]BlockHeader, error) {
	var out []BlockHeader
	if err := pm.client.do(ctx, peer, "headers_from_req", p2pHeadersFromReq{From: fromHeight, Count: count}, &out, "headers"); err != nil {
		return nil, err
	}
	return out, nil
}

func (pm *P2PPeerManager) FetchBlockByHash(ctx context.Context, peer, hashHex string) (*Block, error) {
	var out Block
	err := pm.client.do(ctx, peer, "block_by_hash_req", p2pBlockByHashReq{HashHex: hashHex}, &out, "block")
	if err != nil {
		if err.Error() == "not found" {
			return nil, errors.New("not found")
		}
		return nil, err
	}
	return &out, nil
}

func (pm *P2PPeerManager) FetchAnyBlockByHash(ctx context.Context, hashHex string) (*Block, string, error) {
	var lastErr error
	for _, peer := range pm.peers {
		b, err := pm.FetchBlockByHash(ctx, peer, hashHex)
		if err == nil {
			return b, peer, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no peers configured")
	}
	return nil, "", lastErr
}

func (pm *P2PPeerManager) BroadcastTransaction(ctx context.Context, tx Transaction, _ int) {
	for _, peer := range pm.peers {
		go func(p string) {
			_, err := pm.client.BroadcastTransaction(ctx, p, tx)
			if err != nil {
				log.Printf("p2p broadcast tx to %s failed: %v", p, err)
			}
		}(peer)
	}
}

func (pm *P2PPeerManager) BroadcastBlock(ctx context.Context, block *Block) {
	for _, peer := range pm.peers {
		go func(p string) {
			_, err := pm.client.BroadcastBlock(ctx, p, block)
			if err != nil {
				log.Printf("p2p broadcast block to %s failed: %v", p, err)
			}
		}(peer)
	}
}

func (pm *P2PPeerManager) EnsureAncestors(ctx context.Context, bc *Blockchain, missingHashHex string) error {
	need := missingHashHex
	visited := map[string]struct{}{}
	for depth := 0; depth < 256; depth++ {
		if _, ok := bc.BlockByHash(need); ok {
			return nil
		}
		if _, ok := visited[need]; ok {
			return errors.New("ancestor fetch cycle")
		}
		visited[need] = struct{}{}

		b, _, err := pm.FetchAnyBlockByHash(ctx, need)
		if err != nil {
			return err
		}

		parentHex := fmt.Sprintf("%x", b.PrevHash)
		if len(b.PrevHash) != 0 {
			if _, ok := bc.BlockByHash(parentHex); !ok {
				if err := pm.EnsureAncestors(ctx, bc, parentHex); err != nil {
					return err
				}
			}
		}
		_, err = bc.AddBlock(b)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrUnknownParent) {
			continue
		}
		return err
	}
	return errors.New("max ancestor depth exceeded")
}
