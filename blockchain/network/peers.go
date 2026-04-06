package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
)

type PeerEntry struct {
	Address     string
	LastSuccess time.Time
	LastFailure time.Time
	FailCount   int
	IsActive    bool
}

type PeerManager struct {
	peers   []string
	peerMap map[string]*PeerEntry
	peersMu sync.RWMutex

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
	pm := &PeerManager{
		peers:   make([]string, 0, len(peers)),
		peerMap: make(map[string]*PeerEntry),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		maxAncestorDepth: 256,
	}

	// Initialize peer entries
	for _, addr := range peers {
		pm.addPeerInternal(addr)
	}

	return pm
}

// addPeerInternal adds a peer without locking (caller must hold lock)
func (pm *PeerManager) addPeerInternal(addr string) {
	if addr == "" {
		return
	}

	// Check if already exists
	if _, exists := pm.peerMap[addr]; exists {
		return
	}

	pm.peers = append(pm.peers, addr)
	pm.peerMap[addr] = &PeerEntry{
		Address:  addr,
		IsActive: true,
	}
}

func (pm *PeerManager) Peers() []string {
	pm.peersMu.RLock()
	defer pm.peersMu.RUnlock()

	// Return only active peers
	activePeers := make([]string, 0, len(pm.peers))
	for _, addr := range pm.peers {
		if entry, exists := pm.peerMap[addr]; exists && entry.IsActive {
			activePeers = append(activePeers, addr)
		}
	}
	return activePeers
}

func (pm *PeerManager) AddPeer(addr string) {
	pm.peersMu.Lock()
	defer pm.peersMu.Unlock()

	pm.addPeerInternal(addr)
}

// RecordPeerSuccess records a successful connection to a peer
func (pm *PeerManager) RecordPeerSuccess(addr string) {
	pm.peersMu.Lock()
	defer pm.peersMu.Unlock()

	if entry, exists := pm.peerMap[addr]; exists {
		entry.LastSuccess = time.Now()
		entry.IsActive = true
		entry.FailCount = 0
	}
}

// RecordPeerFailure records a failed connection to a peer
func (pm *PeerManager) RecordPeerFailure(addr string) {
	pm.peersMu.Lock()
	defer pm.peersMu.Unlock()

	if entry, exists := pm.peerMap[addr]; exists {
		entry.LastFailure = time.Now()
		entry.FailCount++

		// Mark peer as inactive after 5 consecutive failures
		if entry.FailCount >= 5 {
			entry.IsActive = false
			log.Printf("peer_manager: marked peer %s as inactive (failures=%d)", addr, entry.FailCount)
		}
	}
}

// CleanupStalePeers removes peers that have been inactive for too long
func (pm *PeerManager) CleanupStalePeers() {
	pm.peersMu.Lock()
	defer pm.peersMu.Unlock()

	now := time.Now()
	cleanupThreshold := 24 * time.Hour // Remove peers inactive for 24 hours

	for addr, entry := range pm.peerMap {
		if !entry.IsActive && entry.LastFailure.Before(now.Add(-cleanupThreshold)) {
			// Remove from peers list
			for i, peerAddr := range pm.peers {
				if peerAddr == addr {
					pm.peers = append(pm.peers[:i], pm.peers[i+1:]...)
					break
				}
			}
			delete(pm.peerMap, addr)
			log.Printf("peer_manager: removed stale peer %s", addr)
		}
	}
}

// GetActivePeers returns all active peers
func (pm *PeerManager) GetActivePeers() []string {
	pm.peersMu.RLock()
	defer pm.peersMu.RUnlock()

	activePeers := make([]string, 0, len(pm.peers))
	for _, entry := range pm.peerMap {
		if entry.IsActive {
			activePeers = append(activePeers, entry.Address)
		}
	}
	return activePeers
}

type chainInfo struct {
	ChainID     uint64   `json:"chainId"`
	Height      uint64   `json:"height"`
	LatestHash  string   `json:"latestHash"`
	RulesHash   string   `json:"rulesHash"`
	GenesisHash string   `json:"genesisHash"`
	Work        *big.Int `json:"-"` // Ignore in automatic decoding
}

// UnmarshalJSON implements custom JSON unmarshaling for chainInfo
// This is needed to properly decode big.Int from string
func (ci *chainInfo) UnmarshalJSON(data []byte) error {
	// Use alias to avoid infinite recursion
	type chainInfoAlias chainInfo
	aux := &struct {
		WorkStr string `json:"work"`
		*chainInfoAlias
	}{
		chainInfoAlias: (*chainInfoAlias)(ci),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Decode work from string to big.Int
	if aux.WorkStr != "" {
		ci.Work = new(big.Int)
		if _, ok := ci.Work.SetString(aux.WorkStr, 10); !ok {
			return fmt.Errorf("invalid work value: %s", aux.WorkStr)
		}
	} else {
		ci.Work = big.NewInt(0)
	}

	return nil
}

func (pm *PeerManager) FetchChainInfo(ctx context.Context, peer string) (*chainInfo, error) {
	// Convert P2P address (port 9090) to HTTP address (port 8080)
	httpPeer := convertP2PToHTTP(peer)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpPeer+"/chain/info", nil)
	if err != nil {
		return nil, fmt.Errorf("create chain info request: %w", err)
	}
	resp, err := pm.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch chain info: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("peer manager: failed to close chain info response body: %v", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chain/info status: %s", resp.Status)
	}
	var v chainInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&v); err != nil {
		return nil, fmt.Errorf("decode chain info: %w", err)
	}
	return &v, nil
}

// convertP2PToHTTP converts P2P address (port 9090) to HTTP address (port 8080)
// e.g., "<host>:9090" -> "http://<host>:8080"
func convertP2PToHTTP(peer string) string {
	// If already has http:// or https://, return as is
	if strings.HasPrefix(peer, "http://") || strings.HasPrefix(peer, "https://") {
		return peer
	}

	// Parse host and port
	host, port, err := net.SplitHostPort(peer)
	if err != nil {
		// No port specified, assume HTTP port 8080
		return "http://" + peer + ":8080"
	}

	// Convert common P2P ports to HTTP ports
	switch port {
	case "9090", "9091", "9092":
		// P2P port, convert to HTTP port
		httpPort := "8080"
		if port == "9091" {
			httpPort = "8081"
		} else if port == "9092" {
			httpPort = "8082"
		}
		return "http://" + host + ":" + httpPort
	default:
		// Unknown port, use as is (assume it's already HTTP port)
		return "http://" + peer
	}
}

func (pm *PeerManager) FetchHeadersFrom(ctx context.Context, peer string, fromHeight uint64, count int) ([]core.BlockHeader, error) {
	u := fmt.Sprintf("%s/headers/from/%d?count=%d", peer, fromHeight, count)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create headers request: %w", err)
	}
	resp, err := pm.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch headers: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("peer manager: failed to close headers response body: %v", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("headers status: %s", resp.Status)
	}
	var headers []core.BlockHeader
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&headers); err != nil {
		return nil, fmt.Errorf("decode headers: %w", err)
	}
	return headers, nil
}

func (pm *PeerManager) FetchBlockByHash(ctx context.Context, peer, hashHex string) (*core.Block, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, peer+"/blocks/hash/"+hashHex, nil)
	if err != nil {
		return nil, fmt.Errorf("create block request: %w", err)
	}
	resp, err := pm.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch block: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("peer manager: failed to close block response body: %v", closeErr)
		}
	}()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("block status: %s", resp.Status)
	}
	var b core.Block
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&b); err != nil {
		return nil, fmt.Errorf("decode block: %w", err)
	}
	return &b, nil
}

func (pm *PeerManager) FetchAnyBlockByHash(ctx context.Context, hashHex string) (*core.Block, string, error) {
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

func (pm *PeerManager) BroadcastTransaction(ctx context.Context, tx core.Transaction, hops int) {
	if pm == nil || len(pm.peers) == 0 || hops <= 0 {
		return
	}
	b, err := json.Marshal(tx)
	if err != nil {
		log.Printf("peer manager: failed to marshal transaction: %v", err)
		return
	}
	for _, peer := range pm.peers {
		peer := peer
		go func(p string) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, p+"/tx", bytes.NewReader(b))
			if err != nil {
				log.Printf("peer manager: failed to create broadcast request to %s: %v", p, err)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(relayHopsHeader, strconv.Itoa(hops))
			resp, err := pm.client.Do(req)
			if err != nil {
				log.Printf("peer manager: failed to broadcast transaction to %s: %v", p, err)
				return
			}
			defer func() {
				if closeErr := resp.Body.Close(); closeErr != nil {
					log.Printf("peer manager: failed to close broadcast response body to %s: %v", p, closeErr)
				}
			}()
			if resp.StatusCode != http.StatusOK {
				log.Printf("peer manager: broadcast transaction to %s returned non-OK status: %d", p, resp.StatusCode)
			}
		}(peer)
	}
}

// EnsureAncestors tries to fetch and add missing ancestor blocks so that a given parent hash is known locally.
func (pm *PeerManager) EnsureAncestors(ctx context.Context, bc BlockchainInterface, missingHashHex string) error {
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
		if errors.Is(err, consensus.ErrUnknownParent) {
			// retry loop
			continue
		}
		// It's the block is invalid for our chain; fail.
		return err
	}
	return errors.New("max ancestor depth exceeded")
}

// getPeerHeight returns the maximum chain height among all peers
func getPeerHeight(pm PeerAPI) uint64 {
	if pm == nil {
		return 0
	}

	var maxHeight uint64
	var bestPeer string
	for _, peer := range pm.Peers() {
		info, err := pm.FetchChainInfo(context.Background(), peer)
		if err != nil {
			continue
		}
		if info.Height > maxHeight {
			maxHeight = info.Height
			bestPeer = peer
		}
	}
	if maxHeight > 0 && bestPeer != "" {
		log.Printf("miner: getPeerHeight returns height=%d from peer=%s (total peers=%d)", maxHeight, bestPeer, len(pm.Peers()))
	}
	return maxHeight
}
