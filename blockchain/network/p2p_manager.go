package network

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
)

// ANSI Color codes for terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

// P2PPeerManager manages P2P peer connections with dynamic storage and cleanup
type P2PPeerManager struct {
	mu             sync.RWMutex
	peers          []string
	peerTimestamps map[string]int64
	peerFailCounts map[string]int // Track consecutive connection failures
	maxPeers       int
	client         *P2PClient
}

// Start starts the P2P manager
// Production-grade: initializes network listeners and peer discovery
func (pm *P2PPeerManager) Start(ctx context.Context) error {
	pm.mu.Lock()
	initialPeers := make([]string, len(pm.peers))
	copy(initialPeers, pm.peers)
	pm.mu.Unlock()

	log.Printf("P2P peer manager started with %d peers", len(initialPeers))

	// Perform initial peer discovery from bootstrap peers
	if len(initialPeers) > 0 {
		go func() {
			// Wait a short time for network to stabilize
			time.Sleep(5 * time.Second)

			discoverCtx, cancel := context.WithTimeout(ctx, time.Duration(PeerDiscoveryTimeoutSec)*time.Second)
			defer cancel()

			// Discover from first bootstrap peer
			pm.DiscoverPeersFromPeer(discoverCtx, initialPeers[0])
			log.Printf("P2P peer manager: initial discovery completed, total peers: %d", len(pm.Peers()))
		}()
	}

	return nil
}

// Stop stops the P2P manager
// Production-grade: gracefully closes all peer connections
func (pm *P2PPeerManager) Stop() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	log.Printf("P2P peer manager stopped")
	return nil
}

// ParseP2PPeersEnv parses a comma-separated list of peer addresses
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

// getMaxPeersFromEnv reads the P2P_MAX_PEERS environment variable
// Returns DefaultMaxPeers if not set or invalid
func getMaxPeersFromEnv() int {
	val := os.Getenv("P2P_MAX_PEERS")
	if val == "" {
		return DefaultMaxPeers
	}
	maxPeers, err := strconv.Atoi(val)
	if err != nil || maxPeers <= 0 {
		return DefaultMaxPeers
	}
	// Cap at reasonable maximum to prevent memory exhaustion
	if maxPeers > 10000 {
		return 10000
	}
	return maxPeers
}

// NewP2PPeerManager creates a new P2P peer manager with dynamic peer storage
func NewP2PPeerManager(chainID uint64, rulesHash string, nodeID string, peers []string) *P2PPeerManager {
	maxPeers := getMaxPeersFromEnv()
	pm := &P2PPeerManager{
		peers:          make([]string, 0, len(peers)),
		peerTimestamps: make(map[string]int64, len(peers)),
		peerFailCounts: make(map[string]int, len(peers)),
		maxPeers:       maxPeers,
		client:         NewP2PClient(chainID, rulesHash, nodeID),
	}

	// Initialize with configured peers (validate format only, allow private IPs)
	now := time.Now().Unix()
	for _, peer := range peers {
		if err := validatePeerAddressFormat(peer); err == nil {
			pm.peers = append(pm.peers, peer)
			pm.peerTimestamps[peer] = now
			pm.peerFailCounts[peer] = 0
		} else {
			log.Printf("P2P peer manager: skipping invalid peer %s: %v", peer, err)
		}
	}

	log.Printf("P2P peer manager initialized with maxPeers=%d, initialPeers=%d", maxPeers, len(pm.peers))
	return pm
}

// validatePublicIPFromAddr validates the IP portion of an address (host:port)
// Supports both IP addresses and domain names (with DNS resolution)
func validatePublicIPFromAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address format: %w", err)
	}

	// Try to parse as IP address first
	ip := net.ParseIP(host)
	if ip != nil {
		// It's already an IP address, validate it
		return validatePublicIP(host)
	}

	// It's a domain name, try to resolve it
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed: %w", err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("no IP addresses found for domain: %s", host)
	}

	// Validate the first resolved IP address
	for _, resolvedIP := range ips {
		// Try IPv4 first
		if ipv4 := resolvedIP.To4(); ipv4 != nil {
			return validatePublicIP(ipv4.String())
		}
	}

	// If no IPv4 found, try IPv6
	for _, resolvedIP := range ips {
		if resolvedIP.To4() == nil && resolvedIP.To16() != nil {
			// IPv6 address found (note: validatePublicIP may reject it)
			return validatePublicIP(resolvedIP.String())
		}
	}

	return fmt.Errorf("no valid IP addresses found for domain: %s", host)
}

// Peers returns a copy of the current peer list (thread-safe)
func (pm *P2PPeerManager) Peers() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return append([]string(nil), pm.peers...)
}

// AddPeer adds a new peer to the manager with timestamp tracking (thread-safe)
// Enforces maxPeers limit and validates IP addresses for configured peers
// Note: Does NOT validate public IP to allow local network peers with dynamic ports
func (pm *P2PPeerManager) AddPeer(addr string) {
	if addr == "" {
		return
	}

	// Validate address format (but allow private IPs for local network)
	if err := validatePeerAddressFormat(addr); err != nil {
		log.Printf("P2P peer manager: rejected peer %s: %v", addr, err)
		return
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if peer already exists
	for i, p := range pm.peers {
		if p == addr {
			// Update timestamp for existing peer
			pm.peerTimestamps[addr] = time.Now().Unix()
			// Reset failure count on successful reconnection
			pm.peerFailCounts[addr] = 0
			// Move to end of list (most recently seen)
			pm.peers = append(pm.peers[:i], pm.peers[i+1:]...)
			pm.peers = append(pm.peers, addr)
			return
		}
	}

	// Enforce maxPeers limit - remove oldest peer if at capacity
	if len(pm.peers) >= pm.maxPeers {
		// Remove oldest peer (first in list)
		oldestPeer := pm.peers[0]
		pm.peers = pm.peers[1:]
		delete(pm.peerTimestamps, oldestPeer)
		delete(pm.peerFailCounts, oldestPeer)
		log.Printf("P2P peer manager: removed oldest peer %s (maxPeers=%d reached)", oldestPeer, pm.maxPeers)
	}

	// Add new peer with current timestamp
	pm.peers = append(pm.peers, addr)
	pm.peerTimestamps[addr] = time.Now().Unix()
	pm.peerFailCounts[addr] = 0
	log.Printf("P2P peer manager: added peer %s (total=%d/%d)", addr, len(pm.peers), pm.maxPeers)
}

// validatePeerAddressFormat validates the format of a peer address (host:port)
// Unlike validatePublicIPFromAddr, this allows private IPs for local network peers
func validatePeerAddressFormat(addr string) error {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address format: %w", err)
	}

	// Validate port
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port: %s", portStr)
	}

	// Validate host (IP or domain name)
	ip := net.ParseIP(host)
	if ip == nil {
		// Not an IP, try to resolve as domain name
		ips, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("invalid host and not resolvable: %s", host)
		}
		if len(ips) == 0 {
			return fmt.Errorf("no IP addresses found for domain: %s", host)
		}
		// Domain resolved successfully, accept it
		return nil
	}

	// Valid IP format (including private IPs for local network)
	return nil
}

// RecordPeerSuccess records a successful connection to a peer
func (pm *P2PPeerManager) RecordPeerSuccess(addr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.peerFailCounts[addr]; exists {
		pm.peerFailCounts[addr] = 0
		pm.peerTimestamps[addr] = time.Now().Unix()
	}
}

// RecordPeerFailure records a failed connection to a peer.
// Implements exponential backoff and peer removal after threshold.
// Thread-safe implementation.
func (pm *P2PPeerManager) RecordPeerFailure(addr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Get current count or default to 0
	failCount := pm.peerFailCounts[addr]

	// Increment failure count
	newCount := failCount + 1
	pm.peerFailCounts[addr] = newCount

	// Log current failure count
	log.Printf("P2P peer manager: peer %s connection failed (count=%d)", addr, newCount)

	// Remove peer after MaxConsecutiveFailures consecutive failures
	if newCount >= MaxConsecutiveFailures {
		// Remove from peers list
		for i, p := range pm.peers {
			if p == addr {
				pm.peers = append(pm.peers[:i], pm.peers[i+1:]...)
				break
			}
		}
		delete(pm.peerTimestamps, addr)
		delete(pm.peerFailCounts, addr) // Delete BEFORE logging to avoid panic
		log.Printf("P2P peer manager: removed peer %s after %d consecutive failures",
			addr, newCount)
	}
}

// ResetPeerFailureCount resets the failure count for a peer after successful connection.
// Call this when a peer successfully reconnects.
func (pm *P2PPeerManager) ResetPeerFailureCount(addr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.peerFailCounts[addr]; exists {
		pm.peerFailCounts[addr] = 0
		pm.peerTimestamps[addr] = time.Now().Unix()
		log.Printf("P2P peer manager: reset failure count for peer %s", addr)
	}
}

// CleanupStalePeers removes peers that haven't been seen in PeerExpiryDuration (thread-safe)
// Should be called periodically (e.g., every hour)
func (pm *P2PPeerManager) CleanupStalePeers() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now().Unix()
	expiryThreshold := now - int64(PeerExpiryDuration.Seconds())

	var activePeers []string
	removedCount := 0

	for _, peer := range pm.peers {
		timestamp, exists := pm.peerTimestamps[peer]
		if !exists || timestamp < expiryThreshold {
			// Peer is stale - remove it
			delete(pm.peerTimestamps, peer)
			removedCount++
			log.Printf("P2P peer manager: removed stale peer %s (last seen: %s)", peer, time.Unix(timestamp, 0).Format(time.RFC3339))
			continue
		}
		activePeers = append(activePeers, peer)
	}

	pm.peers = activePeers
	if removedCount > 0 {
		log.Printf("P2P peer manager: cleanup completed - removed %d stale peers, %d active peers remaining", removedCount, len(pm.peers))
	}
}

// GetActivePeers returns only peers with recent timestamps (< PeerExpiryDuration) (thread-safe)
// Used by handleGetAddr instead of returning all peers
func (pm *P2PPeerManager) GetActivePeers() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	now := time.Now().Unix()
	expiryThreshold := now - int64(PeerExpiryDuration.Seconds())

	var activePeers []string
	for _, peer := range pm.peers {
		timestamp, exists := pm.peerTimestamps[peer]
		if exists && timestamp >= expiryThreshold {
			activePeers = append(activePeers, peer)
		}
	}

	return activePeers
}

// GetPeerTimestamp returns the last seen timestamp for a peer (thread-safe)
func (pm *P2PPeerManager) GetPeerTimestamp(addr string) (int64, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	ts, exists := pm.peerTimestamps[addr]
	return ts, exists
}

// GetPeerCount returns the current number of peers (thread-safe)
func (pm *P2PPeerManager) GetPeerCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.peers)
}

// runPeerCleanupLoop runs periodic cleanup of stale peers
// Should be started as a goroutine with a cancellable context
func runPeerCleanupLoop(ctx context.Context, pm *P2PPeerManager) {
	if pm == nil {
		return
	}

	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()

	log.Printf("P2P peer manager: starting cleanup loop (interval=%v)", CleanupInterval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("P2P peer manager: cleanup loop stopped")
			return
		case <-ticker.C:
			pm.CleanupStalePeers()
		}
	}
}

// FetchChainInfo fetches chain information from a peer
func (pm *P2PPeerManager) FetchChainInfo(ctx context.Context, peer string) (*ChainInfo, error) {
	var out ChainInfo
	if err := pm.client.do(ctx, peer, "chain_info_req", p2pChainInfoReq{}, &out, "chain_info"); err != nil {
		return nil, err
	}
	return &out, nil
}

// DiscoverPeersFromPeer connects to a peer and requests their peer list via getaddr
func (pm *P2PPeerManager) DiscoverPeersFromPeer(ctx context.Context, peer string) {
	log.Printf("P2P peer discovery: requesting peer list from %s", peer)

	var addrResp struct {
		Addresses []peerAddr `json:"addresses"`
	}
	if err := pm.client.do(ctx, peer, "getaddr", nil, &addrResp, "addr"); err != nil {
		log.Printf("P2P peer discovery: failed to get addresses from %s: %v", peer, err)
		return
	}

	addedCount := 0
	for _, a := range addrResp.Addresses {
		addr := fmt.Sprintf("%s:%d", a.IP, a.Port)
		if addr != "" && addr != ":" {
			pm.AddPeer(addr)
			addedCount++
		}
	}
	log.Printf("P2P peer discovery: added %d peers from %s", addedCount, peer)
}

// FetchHeadersFrom fetches block headers from a peer starting at a specific height
// Uses dedicated connection per request for safe concurrent access
func (pm *P2PPeerManager) FetchHeadersFrom(ctx context.Context, peer string, fromHeight uint64, count int) ([]core.BlockHeader, error) {
	var out []core.BlockHeader
	// Use doWithNewConnection for concurrent safety - each request gets its own connection
	if err := pm.client.doWithNewConnection(ctx, peer, "headers_from_req", p2pHeadersFromReq{From: fromHeight, Count: count}, &out, "headers"); err != nil {
		return nil, err
	}
	return out, nil
}

// FetchBlockByHash fetches a specific block by its hash from a peer
// Uses dedicated connection per request for safe concurrent access
func (pm *P2PPeerManager) FetchBlockByHash(ctx context.Context, peer string, hashHex string) (*core.Block, error) {
	var out core.Block
	// Use doWithNewConnection for concurrent safety - each request gets its own connection
	err := pm.client.doWithNewConnection(ctx, peer, "block_by_hash_req", p2pBlockByHashReq{HashHex: hashHex}, &out, "block")
	if err != nil {
		if err.Error() == "not found" {
			return nil, errors.New("not found")
		}
		return nil, err
	}
	return &out, nil
}

// FetchBlockByHeight fetches a block by its height from a specific peer
// Uses dedicated connection per request for safe concurrent access
func (pm *P2PPeerManager) FetchBlockByHeight(ctx context.Context, peer string, height uint64) (*core.Block, error) {
	var out core.Block
	// Use doWithNewConnection for concurrent safety - each request gets its own connection
	err := pm.client.doWithNewConnection(ctx, peer, "block_by_height_req", p2pBlockByHeightReq{Height: height}, &out, "block")
	if err != nil {
		if err.Error() == "not found" {
			return nil, errors.New("not found")
		}
		return nil, err
	}
	return &out, nil
}

// FetchBlocksByHeightRange fetches multiple blocks by height range in a single connection.
// This is more efficient than calling FetchBlockByHeight multiple times because it reuses
// the same TCP connection for all requests, avoiding the overhead of repeated handshakes.
func (pm *P2PPeerManager) FetchBlocksByHeightRange(ctx context.Context, peer string, startHeight, count uint64) ([]*core.Block, error) {
	return pm.client.FetchBlocksByHeightRange(ctx, peer, startHeight, count)
}

// FetchAnyBlockByHash attempts to fetch a block from any available peer
func (pm *P2PPeerManager) FetchAnyBlockByHash(ctx context.Context, hashHex string) (*core.Block, string, error) {
	// Use active peers only for block fetching
	activePeers := pm.GetActivePeers()
	if len(activePeers) == 0 {
		return nil, "", errors.New("no active peers available")
	}

	var lastErr error
	for _, peer := range activePeers {
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

// BroadcastTransaction broadcasts a transaction to all peers concurrently
func (pm *P2PPeerManager) BroadcastTransaction(ctx context.Context, tx core.Transaction, _ int) {
	peers := pm.GetActivePeers()
	if len(peers) == 0 {
		log.Printf("P2P peer manager: no active peers to broadcast transaction")
		return
	}

	// Print transaction broadcast header with box and color
	txHash, err := tx.SigningHash()
	txHashStr := "unknown"
	if err == nil {
		txHashStr = hex.EncodeToString(txHash[:8])
	}
	fmt.Printf("\n")
	fmt.Printf(ColorCyan + ColorBold + "╔═══════════════════════════════════════════════════════════╗\n" + ColorReset)
	fmt.Printf(ColorCyan + "║" + ColorReset + ColorWhite + "  📤 TRANSACTION BROADCAST                                    " + ColorReset + ColorCyan + "║\n" + ColorReset)
	fmt.Printf(ColorCyan + "╠═══════════════════════════════════════════════════════════╣\n" + ColorReset)
	fmt.Printf(ColorCyan+"║"+ColorReset+ColorYellow+"  Hash: %-54s"+ColorReset+ColorCyan+"║\n"+ColorReset, txHashStr+"...")
	fmt.Printf(ColorCyan+"║"+ColorReset+ColorYellow+"  To:   %-54s"+ColorReset+ColorCyan+"║\n"+ColorReset, tx.ToAddress)
	fmt.Printf(ColorCyan+"║"+ColorReset+ColorYellow+"  Amount: %-52d NOGO"+ColorReset+ColorCyan+"║\n"+ColorReset, tx.Amount)
	fmt.Printf(ColorCyan+"║"+ColorReset+ColorGreen+"  Peers: %-53d"+ColorReset+ColorCyan+"║\n"+ColorReset, len(peers))
	fmt.Printf(ColorCyan + "╚═══════════════════════════════════════════════════════════╝\n" + ColorReset)
	fmt.Printf("\n")

	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			_, err := pm.client.BroadcastTransaction(ctx, p, tx)
			if err != nil {
				log.Printf("p2p broadcast tx to %s failed: %v", p, err)
			}
		}(peer)
	}
	wg.Wait()
}

// BroadcastBlock broadcasts a block to all peers concurrently
func (pm *P2PPeerManager) BroadcastBlock(ctx context.Context, block *core.Block) {
	pm.mu.RLock()
	peers := append([]string(nil), pm.peers...)
	pm.mu.RUnlock()

	if len(peers) == 0 {
		log.Printf("P2P peer manager: no peers to broadcast block height=%d hash=%s", block.GetHeight(), hex.EncodeToString(block.Hash))
		return
	}

	// Print block broadcast header with box and color
	blockHashStr := hex.EncodeToString(block.Hash[:8])
	fmt.Printf("\n")
	fmt.Printf(ColorGreen + ColorBold + "╔═══════════════════════════════════════════════════════════╗\n" + ColorReset)
	fmt.Printf(ColorGreen + "║" + ColorReset + ColorWhite + "  ⛏️  BLOCK BROADCAST                                          " + ColorReset + ColorGreen + "║\n" + ColorReset)
	fmt.Printf(ColorGreen + "╠═══════════════════════════════════════════════════════════╣\n" + ColorReset)
	fmt.Printf(ColorGreen+"║"+ColorReset+ColorYellow+"  Height: %-52d"+ColorReset+ColorGreen+"║\n"+ColorReset, block.GetHeight())
	fmt.Printf(ColorGreen+"║"+ColorReset+ColorYellow+"  Hash: %-54s"+ColorReset+ColorGreen+"║\n"+ColorReset, blockHashStr+"...")
	fmt.Printf(ColorGreen+"║"+ColorReset+ColorYellow+"  Tx Count: %-50d"+ColorReset+ColorGreen+"║\n"+ColorReset, len(block.Transactions))
	fmt.Printf(ColorGreen+"║"+ColorReset+ColorYellow+"  Timestamp: %-49s"+ColorReset+ColorGreen+"║\n"+ColorReset, time.Unix(block.Header.TimestampUnix, 0).Format("2006-01-02 15:04:05"))
	fmt.Printf(ColorGreen+"║"+ColorReset+ColorCyan+"  Peers: %-53d"+ColorReset+ColorGreen+"║\n"+ColorReset, len(peers))
	fmt.Printf(ColorGreen + "╚═══════════════════════════════════════════════════════════╝\n" + ColorReset)
	fmt.Printf("\n")

	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			_, err := pm.client.BroadcastBlock(ctx, p, block)
			if err != nil {
				log.Printf("p2p broadcast block to %s failed: %v", p, err)
			}
		}(peer)
	}
	wg.Wait()
	log.Printf("P2P peer manager: block broadcast completed height=%d hash=%s", block.GetHeight(), hex.EncodeToString(block.Hash))
}

// formatAddress formats an address for display
func formatAddress(addr []byte) string {
	if len(addr) == 0 {
		return "N/A"
	}
	return hex.EncodeToString(addr[:20])
}

// EnsureAncestors recursively fetches ancestor blocks to ensure chain continuity
func (pm *P2PPeerManager) EnsureAncestors(ctx context.Context, bc BlockchainInterface, missingHashHex string) error {
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

		parentHex := fmt.Sprintf("%x", b.Header.PrevHash)
		if len(b.Header.PrevHash) != 0 {
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
			continue
		}
		return err
	}
	return errors.New("max ancestor depth exceeded")
}
