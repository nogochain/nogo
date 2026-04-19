// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package security

import (
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"sync"
	"time"
)

const (
	envIPFilterConfig      = "NOGO_IP_FILTER_CONFIG"
	defaultBanTTL          = 24 * time.Hour
	DefaultPeerBanTTL      = 24 * time.Hour
	maxPeerBanEntries      = 10000
	peerBanCleanupInterval = 10 * time.Minute
	
	// Dynamic threshold configuration
	minBanThreshold        = uint32(50)   // Minimum threshold (lenient network)
	maxBanThreshold        = uint32(200)  // Maximum threshold (strict network)
	baseBanThreshold       = uint32(100)  // Base threshold
	thresholdAdjustInterval = 5 * time.Minute // How often to adjust threshold
)

var (
	ErrEmptyDataDir   = fmt.Errorf("security manager: data directory is empty")
	ErrPeerNotBanned  = fmt.Errorf("security manager: peer not in ban list")
)

// PeerBanEntry records why and when a peer was banned at the peerID level
type PeerBanEntry struct {
	PeerID  string
	Reason  string
	Since   time.Time
	Expires time.Time
}

// SecurityManager composes IP filtering, blacklisting, and dynamic ban scoring
// into a unified peer security gateway. It is the single authority for both
// IP-based and PeerID-based ban decisions.
type SecurityManager struct {
	banScores     map[string]*DynamicBanScore
	bannedPeers   map[string]struct{}
	blacklist     *Blacklist
	ipFilter      *IPFilter
	peerBans      map[string]*PeerBanEntry
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	dataDir       string
	onBanCallback func(peerID, ip, reason string)
	
	// Dynamic threshold management
	currentBanThreshold uint32
	lastAdjustTime      time.Time
	misbehaviorStats    map[string]uint32  // peerID -> misbehavior count
	networkQuality      float64            // 0.0 (bad) to 1.0 (good)
}

// NewSecurityManager initializes all security sub-components and loads
// IP filter rules from the environment when available.
func NewSecurityManager(dataDir string) (*SecurityManager, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("%w: data directory must not be empty", ErrEmptyDataDir)
	}

	bl, err := NewBlacklist(dataDir)
	if err != nil {
		return nil, fmt.Errorf("security manager: create blacklist: %w", err)
	}

	ipf := NewIPFilter()

	sm := &SecurityManager{
		banScores:           make(map[string]*DynamicBanScore),
		bannedPeers:         make(map[string]struct{}),
		blacklist:           bl,
		ipFilter:            ipf,
		peerBans:            make(map[string]*PeerBanEntry),
		dataDir:             dataDir,
		currentBanThreshold: baseBanThreshold,
		lastAdjustTime:      time.Now(),
		misbehaviorStats:    make(map[string]uint32),
		networkQuality:      1.0, // Start with perfect network quality
	}

	sm.loadIPFilterConfig()

	return sm, nil
}

// ShouldAcceptConnection checks whether an inbound connection should be
// accepted. It first validates the address, then applies IP filter rules,
// and finally checks the blacklist. Returns (true, "") on acceptance or
// (false, reason) on rejection.
func (sm *SecurityManager) ShouldAcceptConnection(addr string) (bool, string) {
	ipStr := extractIPFromAddr(addr)
	if ipStr == "" {
		return false, "invalid address: unable to extract IP"
	}

	parsedIP := net.ParseIP(ipStr)
	if parsedIP == nil {
		return false, fmt.Sprintf("invalid IP address: %s", ipStr)
	}

	if !sm.ipFilter.Allow(parsedIP) {
		return false, fmt.Sprintf("IP %s denied by IP filter", ipStr)
	}

	if entry, blacklisted := sm.blacklist.IsBlacklisted(ipStr); blacklisted {
		return false, fmt.Sprintf("IP %s is blacklisted: %s", ipStr, entry.Reason)
	}

	return true, ""
}

// OnPeerMisbehavior records a misbehavior event for the given peer.
// If the accumulated ban score reaches the dynamic threshold the peer is
// automatically banned via OnPeerBanned.
func (sm *SecurityManager) OnPeerMisbehavior(peerID string, persistent, transient uint32) {
	sm.mu.Lock()
	score, exists := sm.banScores[peerID]
	if !exists {
		score = NewDynamicBanScore()
		sm.banScores[peerID] = score
	}
	
	// Track misbehavior statistics for dynamic threshold adjustment
	sm.misbehaviorStats[peerID]++
	
	// Check if we need to adjust the threshold
	if time.Since(sm.lastAdjustTime) >= thresholdAdjustInterval {
		sm.adjustBanThresholdLocked()
	}
	
	sm.mu.Unlock()

	newScore := score.Increase(persistent, transient)

	// Use dynamic threshold instead of fixed BanThreshold
	sm.mu.RLock()
	threshold := sm.currentBanThreshold
	sm.mu.RUnlock()
	
	if newScore >= threshold {
		sm.mu.RLock()
		_, alreadyBanned := sm.bannedPeers[peerID]
		sm.mu.RUnlock()

		if !alreadyBanned {
			sm.OnPeerBanned(peerID, "", "ban score exceeded dynamic threshold")
		}
	}
}

// OnPeerBanned adds the IP to the blacklist with the default TTL,
// records a PeerID-level ban entry, and invokes the registered ban
// callback so the caller can disconnect the peer.
func (sm *SecurityManager) OnPeerBanned(peerID, ip, reason string) {
	if ip != "" {
		if err := sm.blacklist.Add(ip, reason, defaultBanTTL); err != nil {
			log.Printf("[SecurityManager] failed to add IP %s to blacklist: %v", ip, err)
		}
	}

	if peerID != "" {
		sm.BanPeer(peerID, reason, DefaultPeerBanTTL)

		sm.mu.Lock()
		sm.bannedPeers[peerID] = struct{}{}
		sm.mu.Unlock()
	}

	sm.mu.RLock()
	cb := sm.onBanCallback
	sm.mu.RUnlock()

	if cb != nil {
		cb(peerID, ip, reason)
	}
}

// SetOnBanCallback registers a callback that is invoked whenever a peer
// is banned. The callback receives the peerID, IP, and reason.
func (sm *SecurityManager) SetOnBanCallback(cb func(peerID, ip, reason string)) {
	sm.mu.Lock()
	sm.onBanCallback = cb
	sm.mu.Unlock()
}

// GetBanScore returns the DynamicBanScore for the given peer, or nil if
// the peer has no recorded score.
func (sm *SecurityManager) GetBanScore(peerID string) *DynamicBanScore {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.banScores[peerID]
}

// adjustBanThresholdLocked adjusts the ban threshold based on network quality.
// Caller must hold sm.mu lock.
func (sm *SecurityManager) adjustBanThresholdLocked() {
	// Calculate network quality based on misbehavior rate
	totalPeers := len(sm.banScores)
	totalMisbehavior := uint32(0)
	for _, count := range sm.misbehaviorStats {
		totalMisbehavior += count
	}
	
	// Network quality = 1.0 - (misbehavior rate)
	// Higher misbehavior rate = lower quality = stricter threshold
	var misbehaviorRate float64
	if totalPeers > 0 {
		misbehaviorRate = float64(totalMisbehavior) / float64(totalPeers)
	}
	
	// Quality ranges from 0.0 (bad) to 1.0 (good)
	// Misbehavior rate of 0% = quality 1.0
	// Misbehavior rate of 50%+ = quality 0.0
	sm.networkQuality = 1.0 - math.Min(misbehaviorRate*2, 1.0)
	
	// Adjust threshold based on network quality
	// Good network (quality > 0.8): lenient threshold (50-100)
	// Normal network (quality 0.5-0.8): standard threshold (100)
	// Bad network (quality < 0.5): strict threshold (100-200)
	var newThreshold uint32
	if sm.networkQuality > 0.8 {
		// Lenient: interpolate between min and base
		qualityInRange := (sm.networkQuality - 0.8) / 0.2 // 0.0 to 1.0
		newThreshold = minBanThreshold + uint32(float64(baseBanThreshold-minBanThreshold)*qualityInRange)
	} else if sm.networkQuality > 0.5 {
		// Normal: use base threshold
		newThreshold = baseBanThreshold
	} else {
		// Strict: interpolate between base and max
		qualityInRange := (0.5 - sm.networkQuality) / 0.5 // 0.0 to 1.0
		newThreshold = baseBanThreshold + uint32(float64(maxBanThreshold-baseBanThreshold)*qualityInRange)
	}
	
	// Clamp to valid range
	if newThreshold < minBanThreshold {
		newThreshold = minBanThreshold
	}
	if newThreshold > maxBanThreshold {
		newThreshold = maxBanThreshold
	}
	
	oldThreshold := sm.currentBanThreshold
	sm.currentBanThreshold = newThreshold
	sm.lastAdjustTime = time.Now()
	
	if oldThreshold != newThreshold {
		log.Printf("[SecurityManager] Ban threshold adjusted: %d -> %d (network quality=%.2f, misbehavior rate=%.2f%%)",
			oldThreshold, newThreshold, sm.networkQuality, misbehaviorRate*100)
	}
}

// GetBanThreshold returns the current dynamic ban threshold.
func (sm *SecurityManager) GetBanThreshold() uint32 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.currentBanThreshold
}

// GetNetworkQuality returns the current network quality score (0.0-1.0).
func (sm *SecurityManager) GetNetworkQuality() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.networkQuality
}

// RemovePeer cleans up the ban score entry for a disconnected peer.
func (sm *SecurityManager) RemovePeer(peerID string) {
	sm.mu.Lock()
	delete(sm.banScores, peerID)
	delete(sm.bannedPeers, peerID)
	delete(sm.misbehaviorStats, peerID)
	sm.mu.Unlock()
}

// BanPeer adds a PeerID-level ban entry with the given TTL.
// This is the unified entry point for all PeerID-based bans.
func (sm *SecurityManager) BanPeer(peerID, reason string, ttl time.Duration) {
	if peerID == "" {
		return
	}

	sm.mu.Lock()
	sm.peerBans[peerID] = &PeerBanEntry{
		PeerID:  peerID,
		Reason:  reason,
		Since:   time.Now(),
		Expires: time.Now().Add(ttl),
	}
	if len(sm.peerBans) > maxPeerBanEntries {
		sm.cleanupPeerBansLocked()
	}
	sm.mu.Unlock()

	log.Printf("[SecurityManager] banned peer %s (reason=%s, ttl=%v)", peerID, reason, ttl)
}

// IsPeerBanned checks whether a peerID is currently banned.
// Expired entries are lazily removed on read.
func (sm *SecurityManager) IsPeerBanned(peerID string) bool {
	sm.mu.RLock()
	entry, exists := sm.peerBans[peerID]
	sm.mu.RUnlock()

	if !exists {
		return false
	}

	if !entry.Expires.IsZero() && time.Now().After(entry.Expires) {
		sm.mu.Lock()
		if e, ok := sm.peerBans[peerID]; ok && !e.Expires.IsZero() && time.Now().After(e.Expires) {
			delete(sm.peerBans, peerID)
			delete(sm.bannedPeers, peerID)
		}
		sm.mu.Unlock()
		return false
	}

	return true
}

// UnbanPeer removes a PeerID-level ban entry.
func (sm *SecurityManager) UnbanPeer(peerID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.peerBans[peerID]; !exists {
		return fmt.Errorf("%w: %s", ErrPeerNotBanned, peerID)
	}

	delete(sm.peerBans, peerID)
	delete(sm.bannedPeers, peerID)
	log.Printf("[SecurityManager] unbanned peer %s", peerID)
	return nil
}

// GetPeerBanInfo returns ban details for a peer, or nil if not banned.
func (sm *SecurityManager) GetPeerBanInfo(peerID string) map[string]interface{} {
	sm.mu.RLock()
	entry, exists := sm.peerBans[peerID]
	sm.mu.RUnlock()

	if !exists {
		return nil
	}

	return map[string]interface{}{
		"peer_id":    entry.PeerID,
		"reason":     entry.Reason,
		"since":      entry.Since,
		"expires":    entry.Expires,
		"is_expired": !entry.Expires.IsZero() && time.Now().After(entry.Expires),
	}
}

// GetPeerBanCount returns the number of currently banned peers.
func (sm *SecurityManager) GetPeerBanCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.peerBans)
}

// cleanupPeerBansLocked removes expired peer ban entries.
// Caller must hold sm.mu write lock.
func (sm *SecurityManager) cleanupPeerBansLocked() {
	now := time.Now()
	for peerID, entry := range sm.peerBans {
		if !entry.Expires.IsZero() && now.After(entry.Expires) {
			delete(sm.peerBans, peerID)
			delete(sm.bannedPeers, peerID)
		}
	}
}

// Start launches background maintenance goroutines (blacklist cleanup,
// peer ban cleanup) and loads IP filter configuration from the environment.
func (sm *SecurityManager) Start(ctx context.Context) error {
	smCtx, cancel := context.WithCancel(ctx)
	sm.ctx = smCtx
	sm.cancel = cancel

	sm.loadIPFilterConfig()
	sm.blacklist.Start(smCtx)
	go sm.peerBanCleanupLoop(smCtx)

	return nil
}

// Stop cancels the background context and halts all maintenance goroutines.
// The blacklist cleanup goroutine exits via context cancellation; its deferred
// cleanup stops the internal ticker, so we do not call Blacklist.Stop() here
// to avoid a race between ticker swap-to-nil and the goroutine's select.
func (sm *SecurityManager) Stop() {
	if sm.cancel != nil {
		sm.cancel()
	}
}

// loadIPFilterConfig reads IP filter rules from the NOGO_IP_FILTER_CONFIG
// environment variable. Invalid or missing configuration is silently ignored
// so that the node can start with default (permissive) settings.
func (sm *SecurityManager) loadIPFilterConfig() {
	configJSON := os.Getenv(envIPFilterConfig)
	if configJSON == "" {
		return
	}

	if err := sm.ipFilter.ParseConfig([]byte(configJSON)); err != nil {
		log.Printf("[SecurityManager] failed to parse IP filter config from env: %v", err)
	}
}

// extractIPFromAddr splits "host:port" and returns the host portion.
// If the input is already a bare IP address it is returned as-is;
// otherwise an empty string signals an unparseable address.
func extractIPFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		if net.ParseIP(addr) != nil {
			return addr
		}
		return ""
	}
	return host
}

// peerBanCleanupLoop periodically removes expired peer ban entries.
func (sm *SecurityManager) peerBanCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(peerBanCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.mu.Lock()
			sm.cleanupPeerBansLocked()
			sm.mu.Unlock()
		}
	}
}
