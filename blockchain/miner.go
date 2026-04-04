package main

import (
	"context"
	"encoding/hex"
	"errors"
	"log"
	"reflect"
	"sync"
	"time"
)

// isPeerAPIValid checks if a PeerAPI interface is truly valid (not nil or containing nil)
// This is necessary because in Go, an interface can contain a nil pointer but still not be nil itself
func isPeerAPIValid(pm PeerAPI) bool {
	if pm == nil {
		return false
	}
	// Use reflection to check if the underlying value is nil
	v := reflect.ValueOf(pm)
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return !v.IsNil()
	default:
		return true
	}
}

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

	// CRITICAL: Mining interruption support for fast chain switching
	miningCtx    context.Context
	miningCancel context.CancelFunc
	isMining     bool
	miningMu     sync.Mutex
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
		case <-time.After(VerificationTimeoutMs * time.Millisecond):
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

// InterruptMining stops any ongoing mining operation
// This is called when a new block is received to ensure fast chain switching
func (m *Miner) InterruptMining() {
	m.miningMu.Lock()
	defer m.miningMu.Unlock()

	if m.isMining && m.miningCancel != nil {
		log.Printf("miner: interrupting ongoing mining operation")
		m.miningCancel()
		m.isMining = false
	}
}

// ResumeMining allows new mining operations after interruption
func (m *Miner) ResumeMining() {
	m.miningMu.Lock()
	defer m.miningMu.Unlock()

	m.miningCtx, m.miningCancel = context.WithCancel(context.Background())
	m.isMining = true
	log.Printf("miner: mining resumed")
}

// isMiningActive checks if mining context is active
func (m *Miner) isMiningActive() bool {
	m.miningMu.Lock()
	defer m.miningMu.Unlock()

	if !m.isMining || m.miningCtx == nil {
		return false
	}

	select {
	case <-m.miningCtx.Done():
		m.isMining = false
		return false
	default:
		return true
	}
}

func (m *Miner) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	defer close(m.stopped)

	// Initialize mining context
	m.ResumeMining()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// CRITICAL: Pause mining during sync to prevent forks
			// Check if sync loop is available and currently syncing
			if m.bc.syncLoop != nil && m.bc.syncLoop.IsSyncing() {
				log.Printf("miner: pausing mining during sync to prevent forks")
				continue
			}

			// Check if verification is in progress using context
			if m.isVerificationActive() {
				// Skip mining while verifying to prevent forks
				log.Printf("miner: skipping mining tick, verification in progress")
				continue
			}

			// Check if mining was interrupted
			if !m.isMiningActive() {
				log.Printf("miner: mining interrupted, skipping tick")
				continue
			}

			block, err := m.MineOnce(ctx, false)

			// CRITICAL: After successfully mining a block, wait for network to sync
			// This prevents forks caused by rapid consecutive block mining
			if block != nil && err == nil {
				log.Printf("miner: block mined at height %d, waiting %dms for network sync before next mining...", block.Height, NetworkSyncCheckDelayMs)
				time.Sleep(NetworkSyncCheckDelayMs * time.Millisecond)

				// Re-check network height after wait
				// Thread-safe: use local pm variable
				pmLocal := m.pm
				if isPeerAPIValid(pmLocal) {
					peerHeight := getPeerHeight(pmLocal)
					localHeight := m.bc.LatestBlock().Height
					if peerHeight > localHeight {
						log.Printf("miner: network advanced during sync wait (local=%d, peer=%d) - resuming sync", localHeight, peerHeight)
						continue
					}
				}
			}
		case <-m.wakeCh:
			// Check if verification is in progress
			if m.isVerificationActive() {
				// Skip mining while verifying to prevent forks
				log.Printf("miner: skipping wake event, verification in progress")
				continue
			}

			// Check if mining was interrupted
			if !m.isMiningActive() {
				log.Printf("miner: mining interrupted, skipping wake event")
				continue
			}

			block, err := m.MineOnce(ctx, false)

			// CRITICAL: After successfully mining a block, wait for network to sync
			// This prevents forks caused by rapid consecutive block mining
			if block != nil && err == nil {
				log.Printf("miner: block mined at height %d, waiting %dms for network sync before next mining...", block.Height, NetworkSyncCheckDelayMs)
				time.Sleep(NetworkSyncCheckDelayMs * time.Millisecond)

				// Re-check network height after wait
				// Thread-safe: use local pm variable
				pmLocal := m.pm
				if isPeerAPIValid(pmLocal) {
					peerHeight := getPeerHeight(pmLocal)
					localHeight := m.bc.LatestBlock().Height
					if peerHeight > localHeight {
						log.Printf("miner: network advanced during sync wait (local=%d, peer=%d) - resuming sync", localHeight, peerHeight)
						continue
					}
				}
			}
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

	// Mark mining as active
	m.miningMu.Lock()
	m.isMining = true
	m.miningMu.Unlock()

	// CRITICAL: Check if P2P is configured
	// Thread-safe: store pm in local variable immediately
	pm := m.pm

	// If P2P is configured, perform network checks
	// Note: pm is a PeerAPI interface, check if it's truly nil
	if isPeerAPIValid(pm) {
		localHeight := m.bc.LatestBlock().Height
		peerHeight := getPeerHeight(pm)

		if peerHeight > localHeight {
			log.Printf("miner: network advanced during mining attempt (local=%d, peer=%d) - aborting mining to sync", localHeight, peerHeight)
			// Abort mining - let sync loop handle it
			return nil, nil
		}

		// CRITICAL: Implement true convergence mechanism
		// If peer height equals local height OR peer height is unknown (0), use deterministic delay
		// Strategy: Use deterministic delay based on miner address to avoid collisions
		if peerHeight == localHeight || peerHeight == 0 {
			// Calculate deterministic delay based on miner address
			// This ensures different miners wait different amounts of time
			minerAddr := m.bc.MinerAddress
			hashSuffix := 0
			if len(minerAddr) >= 4 {
				// Use last 2 hex chars of miner address for delay (0-255ms)
				for i := len(minerAddr) - 1; i >= 0 && i >= len(minerAddr)-2; i-- {
					c := minerAddr[i]
					var val int
					if c >= '0' && c <= '9' {
						val = int(c - '0')
					} else if c >= 'a' && c <= 'f' {
						val = int(c - 'a' + 10)
					} else if c >= 'A' && c <= 'F' {
						val = int(c - 'A' + 10)
					}
					hashSuffix = (hashSuffix << 4) | val
				}
			}

			// Wait baseDelay + variableDelay based on miner address
			// This spreads out mining attempts to avoid collisions
			// CRITICAL: Always wait even if peerHeight is 0 (unknown peer state)
			baseDelay := time.Duration(MinerConvergenceBaseDelayMs) * time.Millisecond
			variableDelay := time.Duration(hashSuffix) * time.Millisecond
			totalDelay := baseDelay + variableDelay

			log.Printf("miner: detected same height or unknown peer (local=%d, peer=%d), waiting %v to avoid competition (miner=%s)", localHeight, peerHeight, totalDelay, minerAddr[:16])
			time.Sleep(totalDelay)

			// Re-check after delay - if peer mined a block, abort
			// Thread-safe: pm is local variable, safe to use
			newPeerHeight := getPeerHeight(pm)
			if newPeerHeight > localHeight {
				log.Printf("miner: network advanced during convergence wait (local=%d, peer=%d) - aborting mining", localHeight, newPeerHeight)
				return nil, nil
			}

			// Final check: ensure we're still at the same height
			newLocalHeight := m.bc.LatestBlock().Height
			if newLocalHeight != localHeight {
				log.Printf("miner: local height changed during wait (old=%d, new=%d) - aborting mining", localHeight, newLocalHeight)
				return nil, nil
			}
		}
	} else {
		// No P2P configured, log and proceed with mining
		log.Printf("miner: no P2P configured, mining without network sync checks")
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

	// CRITICAL: Save parent block BEFORE mining (create a deep copy to prevent modification)
	// This ensures we have a consistent view of the parent block throughout the mining process
	parentAtMineTime := m.bc.LatestBlock()
	if parentAtMineTime == nil {
		log.Printf("miner: no parent block at mining time")
		return nil, errors.New("no parent block")
	}

	// Create a deep copy of the parent block to preserve its state
	parentCopy := &Block{
		Height:         parentAtMineTime.Height,
		Hash:           append([]byte(nil), parentAtMineTime.Hash...),
		PrevHash:       append([]byte(nil), parentAtMineTime.PrevHash...),
		TimestampUnix:  parentAtMineTime.TimestampUnix,
		DifficultyBits: parentAtMineTime.DifficultyBits,
		MinerAddress:   parentAtMineTime.MinerAddress,
	}

	log.Printf("miner: created parentCopy BEFORE mining - height=%d, hash=%x, timestamp=%d, diff=%d",
		parentCopy.Height, parentCopy.Hash, parentCopy.TimestampUnix, parentCopy.DifficultyBits)

	b, err := m.bc.MineTransfers(selected)
	if err != nil {
		log.Printf("miner: mine failed: %v", err)
		return nil, err
	}
	log.Printf("miner: successfully mined block at height %d, diff=%d", b.Height, b.DifficultyBits)

	// Use saved parent copy for timestamp adjustment and validation
	// CRITICAL: Do NOT re-fetch parent here, as MineTransfers already added the block to chain
	log.Printf("miner: before timestamp check - block.Time=%d, parentCopy.Time=%d, condition=%v",
		b.TimestampUnix, parentCopy.TimestampUnix, b.TimestampUnix <= parentCopy.TimestampUnix)
	if b.TimestampUnix <= parentCopy.TimestampUnix {
		oldTs := b.TimestampUnix
		b.TimestampUnix = parentCopy.TimestampUnix + 1
		log.Printf("miner: adjusted block timestamp from %d to %d (parent time was %d, parentCopy ptr=%p)",
			oldTs, b.TimestampUnix, parentCopy.TimestampUnix, parentCopy)
	}

	// CRITICAL: Validate the mined block before adding to chain and broadcasting
	// This ensures we don't propagate invalid blocks
	log.Printf("miner: validating mined block height=%d hash=%x", b.Height, b.Hash)

	// Validate POW seal using saved parent copy (we already verified it hasn't changed)
	log.Printf("miner: starting POW validation height=%d hash=%x, block.Time=%d, parentCopy.Time=%d",
		b.Height, b.Hash, b.TimestampUnix, parentCopy.TimestampUnix)
	if err := validateBlockPoWNogoPow(m.bc.consensus, b, parentCopy); err != nil {
		log.Printf("miner: POW validation failed for mined block: %v", err)

		// CRITICAL: Check if network has advanced and we need to rollback
		// Thread-safe: use local pm variable
		pmForValidation := m.pm
		peerHeight := getPeerHeight(pmForValidation)
		localHeight := m.bc.LatestBlock().Height

		if peerHeight > localHeight {
			log.Printf("miner: validation failed but network has advanced (local=%d, peer=%d) - triggering rollback and sync",
				localHeight, peerHeight)

			// Find the fork point and rollback
			// Calculate how many blocks to rollback
			rollbackDepth := peerHeight - localHeight
			if rollbackDepth < 1 {
				rollbackDepth = 1
			}
			if rollbackDepth > 10 {
				rollbackDepth = 10 // Safety limit
			}

			targetHeight := localHeight
			if rollbackDepth > 0 {
				targetHeight = localHeight - uint64(rollbackDepth)
			}

			log.Printf("miner: rolling back %d blocks to height %d", rollbackDepth, targetHeight)
			if rollbackErr := m.bc.RollbackToHeight(targetHeight); rollbackErr != nil {
				log.Printf("miner: rollback failed: %v", rollbackErr)
				return nil, rollbackErr
			}

			log.Printf("miner: rollback completed, returning to sync loop")
			return nil, nil
		}

		// Don't return error - just don't broadcast this invalid block
		return nil, nil
	}

	// CRITICAL: MineTransfers already added the block to chain, so we don't need to call AddBlock again
	// Just verify that the block was actually added by checking if it's the latest block
	latest := m.bc.LatestBlock()
	latestHeight := uint64(0)
	if latest != nil {
		latestHeight = latest.Height
	}
	if latest == nil || latest.Hash == nil || string(latest.Hash) != string(b.Hash) {
		log.Printf("miner: mined block was not added to chain (latest=%d, expected=%d)", latestHeight, b.Height)
		return nil, nil
	}

	log.Printf("miner: mined block validated and added to chain successfully (height=%d, hash=%x)", b.Height, b.Hash)

	// Broadcast the new block to all peers
	// CRITICAL: Wait for network sync before next mining to prevent forks
	// Use adaptive delay based on network conditions
	// Thread-safe: pm is local variable, safe to use
	propagationDelay := calculateAdaptivePropagationDelay(pm)
	log.Printf("miner: block mined at height %d, waiting %dms for network sync before next mining...", b.Height, propagationDelay)
	time.Sleep(time.Duration(propagationDelay) * time.Millisecond)

	// Get peer count for logging (safe check)
	peerCount := 0
	if isPeerAPIValid(pm) {
		peerCount = len(pm.Peers())
	}
	log.Printf("miner: starting block broadcast height=%d hash=%x peers=%d", b.Height, b.Hash, peerCount)
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
	if !isPeerAPIValid(m.pm) {
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

// calculateAdaptivePropagationDelay calculates adaptive propagation delay based on network conditions
// Returns delay in milliseconds
func calculateAdaptivePropagationDelay(pm PeerAPI) int {
	if !isPeerAPIValid(pm) {
		return BlockPropagationDelayMs
	}

	peers := pm.Peers()
	if len(peers) == 0 {
		// No peers, use minimum delay
		return BlockPropagationDelayMs / 2
	}

	// Base delay on number of peers and network topology
	// More peers = longer propagation time needed
	baseDelay := BlockPropagationDelayMs

	// Add delay for each additional peer (diminishing returns)
	peerFactor := 0
	if len(peers) > 1 {
		// Logarithmic scaling: each additional peer adds less delay
		peerFactor = int(50 * float64(len(peers)-1) / float64(len(peers)))
	}

	adaptiveDelay := baseDelay + peerFactor

	// Cap at reasonable maximum (3 seconds)
	if adaptiveDelay > 3000 {
		adaptiveDelay = 3000
	}

	log.Printf("miner: calculated adaptive propagation delay=%dms (peers=%d, base=%d, factor=%d)",
		adaptiveDelay, len(peers), baseDelay, peerFactor)

	return adaptiveDelay
}
