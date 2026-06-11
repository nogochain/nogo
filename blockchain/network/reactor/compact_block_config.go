package reactor

import (
	"sync"
	"time"
)

// =============================================================================
// BIP152 High/Low Bandwidth Mode Configuration
// =============================================================================

const (
	// MaxHighBandwidthPeers limits the number of peers accepted as
	// high-bandwidth compact block relays (BIP152 section 3: <= 3).
	// Economic rationale: marginal propagation benefit drops below
	// 0.5%/MB after the 3rd high-bandwidth peer.
	MaxHighBandwidthPeers = 3

	// CompactBlockVersion is the BIP152 version this node supports.
	// Version 2 uses SipHash-2-4 with salt-based O(M) matching.
	CompactBlockVersion = 2
)

// PeerCompactPreference stores a peer's compact block negotiation result.
type PeerCompactPreference struct {
	Version   uint8     // negotiated compact block version
	HighBW    bool      // true = high-bandwidth, false = low-bandwidth
	UpdatedAt time.Time // last negotiation timestamp
}

// PeerCompactPreferences manages per-peer compact block preferences.
// Thread-safe, supports high/low bandwidth peer tracking.
type PeerCompactPreferences struct {
	mu    sync.RWMutex
	prefs map[string]*PeerCompactPreference // peerID -> preference
}

// NewPeerCompactPreferences creates a new preference store.
func NewPeerCompactPreferences() *PeerCompactPreferences {
	return &PeerCompactPreferences{
		prefs: make(map[string]*PeerCompactPreference),
	}
}

// Set stores a peer's compact block preference.
func (p *PeerCompactPreferences) Set(peerID string, pref *PeerCompactPreference) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prefs[peerID] = pref
}

// Get retrieves a peer's compact block preference.
func (p *PeerCompactPreferences) Get(peerID string) (*PeerCompactPreference, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pref, ok := p.prefs[peerID]
	return pref, ok
}

// Remove cleans up a disconnected peer's preferences.
func (p *PeerCompactPreferences) Remove(peerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.prefs, peerID)
}

// HighBandwidthCount returns the current count of high-bandwidth peers.
func (p *PeerCompactPreferences) HighBandwidthCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	count := 0
	for _, pref := range p.prefs {
		if pref.HighBW {
			count++
		}
	}
	return count
}

// IsHighBandwidthPeer checks if a peer is in high-bandwidth mode.
func (p *PeerCompactPreferences) IsHighBandwidthPeer(peerID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pref, ok := p.prefs[peerID]
	return ok && pref.HighBW
}

// HasStaleHighBandwidthPeer checks if any high-bandwidth peer has not
// re-negotiated within the given duration. Used for priority eviction.
// P2-3 fix: enables graceful replacement of inactive high-bandwidth peers.
func (p *PeerCompactPreferences) HasStaleHighBandwidthPeer(staleThreshold time.Duration) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	now := time.Now()
	for _, pref := range p.prefs {
		if pref.HighBW && now.Sub(pref.UpdatedAt) > staleThreshold {
			return true
		}
	}
	return false
}

// HighBandwidthPeers returns a list of all high-bandwidth peer IDs.
func (p *PeerCompactPreferences) HighBandwidthPeers() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var peers []string
	for id, pref := range p.prefs {
		if pref.HighBW {
			peers = append(peers, id)
		}
	}
	return peers
}

// LowBandwidthPeers returns compact-block-capable peers in low-bandwidth mode.
func (p *PeerCompactPreferences) LowBandwidthPeers() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var peers []string
	for id, pref := range p.prefs {
		if !pref.HighBW && pref.Version >= 2 {
			peers = append(peers, id)
		}
	}
	return peers
}
