package main

import (
	"context"
	"encoding/hex"
	"errors"
	"log"
	"sync"
	"time"
)

type Miner struct {
	bc *Blockchain
	mp *Mempool
	pm PeerAPI

	maxTxPerBlock    int
	forceEmptyBlocks bool

	events EventSink

	mu              sync.RWMutex
	wakeCh          chan struct{}
	stopped         chan struct{}
	verificationCtx context.Context
	verifyCancel    context.CancelFunc
	verifyDoneCh    chan struct{}
}

func NewMiner(bc *Blockchain, mp *Mempool, pm PeerAPI, maxTxPerBlock int, forceEmptyBlocks bool) *Miner {
	if maxTxPerBlock <= 0 {
		maxTxPerBlock = 100
	}
	return &Miner{
		bc:               bc,
		mp:               mp,
		pm:               pm,
		maxTxPerBlock:    maxTxPerBlock,
		forceEmptyBlocks: forceEmptyBlocks,
		wakeCh:           make(chan struct{}, 1),
		stopped:          make(chan struct{}),
		verificationCtx:  nil, // Start with nil - not verifying
		verifyCancel:     nil, // Start with nil - not verifying
		verifyDoneCh:     make(chan struct{}, 1),
	}
}

func (m *Miner) SetEventSink(sink EventSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = sink
}

// OnBlockAdded is called when a block is added to the chain
// This pauses mining during verification using context-based control
func (m *Miner) OnBlockAdded() {
	m.StartVerification()

	// Start a goroutine that ends verification after a short delay
	// This simulates the time needed for block validation
	go func() {
		// Wait for a short period to allow block processing
		// In production, this should be tied to actual validation completion
		select {
		case <-time.After(500 * time.Millisecond):
			// Verification timeout - force end verification
			m.EndVerification()
		case <-m.stopped:
			// Miner stopped, exit goroutine gracefully
			return
		}
	}()
}

func (m *Miner) Wake() {
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}

// StartVerification signals that block verification is starting
// Mining will be paused until verification completes via context cancellation
// If already verifying, this function does nothing to prevent stacking
func (m *Miner) StartVerification() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Don't start a new verification if already verifying
	// This prevents stacking verifications that can never all be ended
	if m.verifyCancel != nil {
		log.Printf("miner: already verifying, skipping new verification start")
		return
	}

	// Create new verification context
	ctx, cancel := context.WithCancel(context.Background())
	m.verificationCtx = ctx
	m.verifyCancel = cancel

	log.Printf("miner: verification started, mining paused")
}

// EndVerification signals that block verification has completed
// Mining can now resume
func (m *Miner) EndVerification() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel verification context to signal completion
	if m.verifyCancel != nil {
		m.verifyCancel()
		m.verifyCancel = nil
	}
	m.verificationCtx = context.Background()

	// Signal verification completion (non-blocking)
	select {
	case m.verifyDoneCh <- struct{}{}:
	default:
	}

	log.Printf("miner: verification completed, mining resumed")
}

// IsVerifying returns true if verification is in progress
// Uses RWMutex for better concurrent read performance
func (m *Miner) IsVerifying() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.verifyCancel != nil
}

func (m *Miner) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	defer close(m.stopped)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Check if verification is in progress using context
			if m.isVerificationActive() {
				// Skip mining while verifying to prevent forks
				log.Printf("miner: skipping mining tick, verification in progress")
				continue
			}

			_, _ = m.MineOnce(ctx, false)
		case <-m.wakeCh:
			// Check if verification is in progress
			if m.isVerificationActive() {
				// Skip mining while verifying to prevent forks
				log.Printf("miner: skipping wake event, verification in progress")
				continue
			}

			_, _ = m.MineOnce(ctx, false)
		}
	}
}

// isVerificationActive checks if verification context is active
// Returns true if verification is ongoing, false otherwise
func (m *Miner) isVerificationActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.verificationCtx == nil {
		return false
	}

	// Check if context is done (verification completed)
	select {
	case <-m.verificationCtx.Done():
		// Context is done, verification complete
		return false
	default:
		// Context still active, verification in progress
		return true
	}
}

func (m *Miner) MineOnce(ctx context.Context, force bool) (*Block, error) {
	if m.mp == nil {
		return nil, nil
	}

	// CRITICAL: Before mining, check if network has advanced
	// If so, pause mining and sync instead to prevent forks
	if m.pm != nil {
		localHeight := m.bc.LatestBlock().Height
		peerHeight := getPeerHeight(m.pm)

		if peerHeight > localHeight {
			log.Printf("miner: network advanced during mining attempt (local=%d, peer=%d) - aborting mining to sync", localHeight, peerHeight)
			// Abort mining - let sync loop handle it
			return nil, nil
		}

		// Additional safety: if peer height equals local height, wait a tiny bit
		// to ensure we're not competing with peer who just mined
		if peerHeight == localHeight {
			// Very short wait - just enough to let any pending block propagate
			time.Sleep(500 * time.Millisecond)

			// Re-check after short wait
			newPeerHeight := getPeerHeight(m.pm)
			if newPeerHeight > localHeight {
				log.Printf("miner: network advanced during wait (local=%d, peer=%d) - aborting mining", localHeight, newPeerHeight)
				return nil, nil
			}
		}
	}

	selected, selectedIDs, err := m.bc.SelectMempoolTxs(m.mp, m.maxTxPerBlock)
	if err != nil {
		log.Printf("miner: select txs failed: %v", err)
		return nil, err
	}

	mineEmpty := force || m.forceEmptyBlocks
	log.Printf("miner: force=%v, forceEmptyBlocks=%v, mineEmpty=%v, selected=%d", force, m.forceEmptyBlocks, mineEmpty, len(selected))
	if len(selected) == 0 && !mineEmpty {
		return nil, nil
	}

	log.Printf("miner: attempting to mine block with %d transactions", len(selected))
	b, err := m.bc.MineTransfers(selected)
	if err != nil {
		log.Printf("miner: mine failed: %v", err)
		return nil, err
	}
	log.Printf("miner: successfully mined block at height %d, diff=%d", b.Height, b.DifficultyBits)

	// CRITICAL: Validate the mined block before adding to chain and broadcasting
	// This ensures we don't propagate invalid blocks
	log.Printf("miner: validating mined block height=%d hash=%x", b.Height, b.Hash)

	// Get parent block for validation
	parent := m.bc.LatestBlock()
	if parent == nil {
		log.Printf("miner: no parent block for validation")
		return nil, errors.New("no parent block")
	}

	// Validate POW seal
	if err := validateBlockPoWNogoPow(m.bc.consensus, b, parent); err != nil {
		log.Printf("miner: POW validation failed for mined block: %v", err)
		// Don't return error - just don't broadcast this invalid block
		return nil, nil
	}

	// Add block to local chain
	accepted, err := m.bc.AddBlock(b)
	if err != nil {
		log.Printf("miner: failed to add mined block to chain: %v", err)
		return nil, err
	}
	if !accepted {
		log.Printf("miner: mined block was not accepted to chain")
		return nil, nil
	}

	log.Printf("miner: mined block validated and added to chain successfully")

	// Broadcast the new block to all peers
	go m.broadcastBlock(ctx, b)

	if len(selectedIDs) > 0 {
		m.mp.RemoveMany(selectedIDs)
		m.mu.Lock()
		sink := m.events
		m.mu.Unlock()
		if sink != nil {
			addrs := addressesForBlock(&Block{Transactions: selected})
			sink.Publish(WSEvent{
				Type: "mempool_removed",
				Data: map[string]any{
					"txIds":     selectedIDs,
					"reason":    "mined",
					"addresses": addrs,
				},
			})
		}
	}
	return b, nil
}

// broadcastBlock broadcasts the mined block to all connected peers
func (m *Miner) broadcastBlock(ctx context.Context, block *Block) {
	if m.pm == nil {
		log.Printf("miner: no peer manager available, skipping block broadcast")
		return
	}

	log.Printf("miner: broadcasting block %d (%s) to peers", block.Height, hex.EncodeToString(block.Hash))
	if pm, ok := m.pm.(*P2PPeerManager); ok {
		pm.BroadcastBlock(ctx, block)
		log.Printf("miner: block broadcast completed")
	} else {
		log.Printf("miner: peer manager does not support block broadcast")
	}
}
