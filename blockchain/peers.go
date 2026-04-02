package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type PeerManager struct {
	peers []string

	client *http.Client

	maxAncestorDepth int
}

func ParsePeersEnv(peersEnv string) []string {
	var peers []string
	for _, raw := range strings.Split(peersEnv, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		raw = strings.TrimRight(raw, "/")
		if _, err := url.Parse(raw); err != nil {
			continue
		}
		peers = append(peers, raw)
	}
	return peers
}

func NewPeerManager(peers []string) *PeerManager {
	return &PeerManager{
		peers: peers,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		maxAncestorDepth: 256,
	}
}

func (pm *PeerManager) Peers() []string { return append([]string(nil), pm.peers...) }

func (pm *PeerManager) AddPeer(addr string) {
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

// GetActivePeers returns all peers (HTTP PeerManager doesn't track timestamps)
// This is a compatibility method for the PeerAPI interface
func (pm *PeerManager) GetActivePeers() []string {
	return pm.Peers()
}

type chainInfo struct {
	ChainID     uint64 `json:"chainId"`
	Height      uint64 `json:"height"`
	LatestHash  string `json:"latestHash"`
	RulesHash   string `json:"rulesHash"`
	GenesisHash string `json:"genesisHash"`
}

func (pm *PeerManager) FetchChainInfo(ctx context.Context, peer string) (*chainInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, peer+"/chain/info", nil)
	if err != nil {
		return nil, err
	}
	resp, err := pm.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chain/info status: %s", resp.Status)
	}
	var v chainInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (pm *PeerManager) FetchHeadersFrom(ctx context.Context, peer string, fromHeight uint64, count int) ([]BlockHeader, error) {
	u := fmt.Sprintf("%s/headers/from/%d?count=%d", peer, fromHeight, count)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := pm.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("headers status: %s", resp.Status)
	}
	var headers []BlockHeader
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&headers); err != nil {
		return nil, err
	}
	return headers, nil
}

func (pm *PeerManager) FetchBlockByHash(ctx context.Context, peer, hashHex string) (*Block, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, peer+"/blocks/hash/"+hashHex, nil)
	if err != nil {
		return nil, err
	}
	resp, err := pm.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("block status: %s", resp.Status)
	}
	var b Block
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&b); err != nil {
		return nil, err
	}
	return &b, nil
}

func (pm *PeerManager) FetchAnyBlockByHash(ctx context.Context, hashHex string) (*Block, string, error) {
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

const relayHopsHeader = "X-NogoChain-Relay-Hops"

func (pm *PeerManager) BroadcastTransaction(ctx context.Context, tx Transaction, hops int) {
	if pm == nil || len(pm.peers) == 0 || hops <= 0 {
		return
	}
	b, err := json.Marshal(tx)
	if err != nil {
		return
	}
	for _, peer := range pm.peers {
		peer := peer
		go func() {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, peer+"/tx", bytes.NewReader(b))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(relayHopsHeader, strconv.Itoa(hops))
			resp, err := pm.client.Do(req)
			if err != nil {
				return
			}
			_ = resp.Body.Close()
		}()
	}
}

// EnsureAncestors tries to fetch and add missing ancestor blocks so that a given parent hash is known locally.
func (pm *PeerManager) EnsureAncestors(ctx context.Context, bc *Blockchain, missingHashHex string) error {
	need := missingHashHex
	visited := map[string]struct{}{}
	for depth := 0; depth < pm.maxAncestorDepth; depth++ {
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

		// Ensure the parent first (if needed), then add this block.
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
			// retry loop
			continue
		}
		// It's possible the block is invalid for our chain; fail.
		return err
	}
	return errors.New("max ancestor depth exceeded")
}
