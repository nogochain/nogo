package reactor

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/network/mconnection"
)

// =============================================================================
// Transaction Pre-Announcement Protocol
// Improves compact block mempool coverage by pre-sending transaction IDs
// to high-bandwidth peers before the block arrives.
// =============================================================================

const (
	// SyncMsgTxPreAnnounce pre-announces transaction IDs for an upcoming block.
	SyncMsgTxPreAnnounce byte = 0x12

	// maxPreAnnounceTxIDs limits the pre-announce message size.
	maxPreAnnounceTxIDs = 2000

	// preAnnounceRateLimit is the minimum interval between pre-announcements.
	preAnnounceRateLimit = 500 * time.Millisecond
)

// TxPreAnnounceMsg pre-announces transaction IDs that will appear in
// an upcoming block. Receiving peers can pre-fetch these transactions
// to improve compact block reconstruction hit rate.
type TxPreAnnounceMsg struct {
	TxIDs []string `json:"tx_ids"`
	Nonce uint64   `json:"nonce"` // deduplication nonce
}

// TxPreAnnouncer manages transaction pre-announcement to high-bandwidth peers.
type TxPreAnnouncer struct {
	mu        sync.RWMutex
	sw        Switch
	lastNonce uint64
	lastSent  time.Time
}

// NewTxPreAnnouncer creates a pre-announcer backed by a switch.
func NewTxPreAnnouncer(sw Switch) *TxPreAnnouncer {
	return &TxPreAnnouncer{sw: sw}
}

// PreAnnounce sends a list of transaction IDs to a peer before a block arrives.
// Rate-limited to one announcement per 500ms per instance.
func (pa *TxPreAnnouncer) PreAnnounce(peerID string, txIDs []string) error {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	// Rate limit: at most one pre-announcement per rateLimit interval
	if time.Since(pa.lastSent) < preAnnounceRateLimit {
		return nil
	}

	if len(txIDs) > maxPreAnnounceTxIDs {
		txIDs = txIDs[:maxPreAnnounceTxIDs]
	}

	pa.lastNonce++
	msg := TxPreAnnounceMsg{
		TxIDs: txIDs,
		Nonce: pa.lastNonce,
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal pre-announce: %w", err)
	}

	wireMsg := make([]byte, 1+len(payload))
	wireMsg[0] = SyncMsgTxPreAnnounce
	copy(wireMsg[1:], payload)

	if !pa.sw.Send(peerID, mconnection.ChannelSync, wireMsg) {
		return fmt.Errorf("send pre-announce to %s failed", peerID)
	}

	pa.lastSent = time.Now()
	log.Printf("[TxPreAnnounce] Sent %d tx IDs to peer %s (nonce=%d)", len(txIDs), peerID, pa.lastNonce)
	return nil
}

// ParseTxPreAnnounce parses a received pre-announce message payload.
func ParseTxPreAnnounce(data []byte) (*TxPreAnnounceMsg, error) {
	var msg TxPreAnnounceMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal pre-announce: %w", err)
	}
	return &msg, nil
}

// OnTxPreAnnounce handles a received transaction pre-announcement.
// Identifies transactions missing from the local mempool for potential
// pre-fetching before the compact block arrives.
func (h *SyncReactorHandler) OnTxPreAnnounce(peerID string, txIDs []string) error {
	if h.handlers == nil || h.handlers.mempool == nil {
		return fmt.Errorf("mempool not available for pre-announce")
	}

	var missing []string
	for _, txID := range txIDs {
		if !h.handlers.mempool.Contains(txID) {
			missing = append(missing, txID)
		}
	}

	if len(missing) > 0 {
		log.Printf("[SyncHandler] Pre-announce: %d/%d txs missing from mempool from %s",
			len(missing), len(txIDs), peerID)
	}

	return nil
}
