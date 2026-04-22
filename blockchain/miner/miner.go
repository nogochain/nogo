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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package miner

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/network"
)

// getBlockVersion calculates the appropriate block version based on consensus rules
func getBlockVersion(consensus config.ConsensusParams, height uint64) uint32 {
	// Check if Merkle Root feature is active
	if consensus.MerkleRootActive(height) {
		return 2
	}
	// Check if Binary Encoding feature is active
	if consensus.BinaryEncodingActive(height) {
		return 2
	}
	// Default version
	return 1
}

// ANSI color codes for colored logging
const (
	colorReset         = "\033[0m"
	colorRed           = "\033[31m"
	colorGreen         = "\033[32m"
	colorYellow        = "\033[33m"
	colorBlue          = "\033[34m"
	colorMagenta       = "\033[35m"
	colorCyan          = "\033[36m"
	colorBrightCyan    = "\033[96m"
	colorBrightGreen   = "\033[92m"
	colorBrightYellow  = "\033[93m"
	colorBrightMagenta = "\033[95m"
)

// logf prints colored log messages with emoji icons
// Supports both simple messages and formatted messages
// Usage: logf(color, icon) OR logf(color, icon, message) OR logf(color, icon, format, args...)
func logf(color string, icon string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	var message string

	if len(args) == 0 {
		// Only color and icon, no message
		message = ""
	} else if len(args) == 1 {
		// Only message string, no format arguments
		if msg, ok := args[0].(string); ok {
			message = msg
		} else {
			message = fmt.Sprint(args[0])
		}
	} else {
		// Format string with arguments - first arg after color/icon is format string
		if format, ok := args[0].(string); ok {
			// Use explicit switch for common cases to ensure type safety
			switch len(args) {
			case 2:
				message = fmt.Sprintf(format, args[1])
			case 3:
				message = fmt.Sprintf(format, args[1], args[2])
			case 4:
				message = fmt.Sprintf(format, args[1], args[2], args[3])
			case 5:
				message = fmt.Sprintf(format, args[1], args[2], args[3], args[4])
			default:
				message = fmt.Sprintf(format, args[1:]...)
			}
		} else {
			message = fmt.Sprint(args...)
		}
	}

	fmt.Fprintf(os.Stdout, "%s %s%s%s %s%s%s\n", timestamp, color, icon, colorReset, color, message, colorReset)
}

// Miner represents the mining engine for continuous block production
// Production-grade: implements continuous mining with proper concurrency control
type Miner struct {
	mu sync.RWMutex

	bc Blockchain
	mp Mempool
	pm PeerAPI

	maxTxPerBlock    int
	forceEmptyBlocks bool

	events EventSink

	wakeCh       chan struct{}
	stopped      chan struct{}
	isVerifying  bool
	verifyDoneCh chan struct{}

	miningCtx    context.Context
	miningCancel context.CancelFunc
	isMining     bool
	miningMu     sync.Mutex

	syncLoop SyncLoop

	minerAddress string
	chainID      uint64
}

// Blockchain defines the blockchain interface for mining
type Blockchain interface {
	LatestBlock() *core.Block
	SelectMempoolTxs(mp Mempool, maxTxPerBlock int) ([]core.Transaction, []string, error)
	MineTransfers(ctx context.Context, txs []core.Transaction) (*core.Block, error)
	CanonicalWork() *big.Int
	RollbackToHeight(height uint64) error
	GetConsensus() config.ConsensusParams
}

// Mempool defines the mempool interface
type Mempool interface {
	Size() int
	RemoveMany(txids []string)
	EntriesSortedByFeeDesc() []MempoolEntry
}

// PeerAPI defines the peer management API
type PeerAPI interface {
	Peers() []string
	FetchChainInfo(ctx context.Context, peer string) (*ChainInfo, error)
	BroadcastBlock(ctx context.Context, block *core.Block) error
}

// ChainInfo represents peer chain information (alias to network.ChainInfo)
type ChainInfo = network.ChainInfo

// SyncLoop defines the sync loop interface
type SyncLoop interface {
	IsSyncing() bool
	IsSynced() bool
}

// EventSink defines the event sink interface
type EventSink interface {
	Publish(event WSEvent)
}

// WSEvent represents a WebSocket event
type WSEvent struct {
	Type string
	Data interface{}
}

// mempoolEntry represents a mempool transaction entry
type mempoolEntry struct {
	tx       core.Transaction
	txIDHex  string
	received time.Time
}

// Tx returns the transaction
func (e mempoolEntry) Tx() core.Transaction {
	return e.tx
}

// TxID returns the transaction ID
func (e mempoolEntry) TxID() string {
	return e.txIDHex
}

// Received returns the received time
func (e mempoolEntry) Received() time.Time {
	return e.received
}

// MempoolEntry is the exported interface for mempool entries
type MempoolEntry interface {
	Tx() core.Transaction
	TxID() string
	Received() time.Time
}

// NewMiner creates a new miner instance
// Production-grade: initializes all fields with proper defaults
func NewMiner(
	bc Blockchain,
	mp Mempool,
	pm PeerAPI,
	maxTxPerBlock int,
	forceEmptyBlocks bool,
	minerAddress string,
	chainID uint64,
) *Miner {
	if maxTxPerBlock <= 0 {
		maxTxPerBlock = config.DefaultMaxTransactionsPerBlock
	}

	miner := &Miner{
		bc:               bc,
		mp:               mp,
		pm:               pm,
		maxTxPerBlock:    maxTxPerBlock,
		forceEmptyBlocks: forceEmptyBlocks,
		wakeCh:           make(chan struct{}, 1),
		stopped:          make(chan struct{}),
		isVerifying:      false,
		verifyDoneCh:     nil,
		minerAddress:     minerAddress,
		chainID:          chainID,
	}

	return miner
}

// SetEventSink sets the event sink for publishing mining events
func (m *Miner) SetEventSink(sink EventSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = sink
}

// SetSyncLoop sets the sync loop for mining coordination
// CRITICAL: This must be called before mining starts to prevent forks
func (m *Miner) SetSyncLoop(syncLoop SyncLoop) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncLoop = syncLoop
}

// OnBlockAdded is called when a block is added to the chain
// Pauses mining during verification
// CRITICAL FIX: Use very short verification time (1 second) to avoid blocking mining
// Block propagation typically completes within 1-2 seconds in healthy networks
func (m *Miner) OnBlockAdded() {
	m.mu.Lock()
	// Cancel any existing verification timeout
	if m.verifyDoneCh != nil {
		close(m.verifyDoneCh)
	}

	// Start new verification with very short timeout
	m.isVerifying = true
	doneCh := make(chan struct{})
	m.verifyDoneCh = doneCh
	m.mu.Unlock()

	logf(colorBrightCyan, "🔍 ", fmt.Sprintf("OnBlockAdded: verification started (isVerifying=true)"))

	// Single timeout goroutine with 1 second timeout (not 5 seconds!)
	go func() {
		select {
		case <-time.After(1000 * time.Millisecond): // 1 second, not 5!
			m.mu.Lock()
			// Only end if this is still the active done channel
			if m.verifyDoneCh == doneCh && m.isVerifying {
				m.isVerifying = false
				m.verifyDoneCh = nil
				m.mu.Unlock()
				logf(colorBrightCyan, "🔍 ", "OnBlockAdded: verification timeout (isVerifying=false)")
			} else {
				m.mu.Unlock()
				logf(colorYellow, "⚠️ ", "OnBlockAdded: verification already ended")
			}
		case <-doneCh:
			// Channel closed by newer block
			return
		case <-m.stopped:
			return
		}
	}()
}

// Wake triggers an immediate mining attempt
func (m *Miner) Wake() {
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}

// IsVerifying returns true if verification is in progress
func (m *Miner) IsVerifying() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isVerifying
}

// InterruptMining stops any ongoing mining operation
// Called when a new block is received for fast chain switching
func (m *Miner) InterruptMining() {
	m.miningMu.Lock()
	defer m.miningMu.Unlock()

	if m.isMining && m.miningCancel != nil {
		logf(colorYellow, "⚠️ ", "InterruptMining: cancelling mining context")
		m.miningCancel()
		m.isMining = false
		logf(colorYellow, "⚠️ ", "InterruptMining: mining context cancelled, isMining=false")
	} else {
		logf(colorYellow, "⚠️ ", "InterruptMining: no active mining to interrupt")
	}
}

// ResumeMining allows new mining operations after interruption
func (m *Miner) ResumeMining() {
	m.miningMu.Lock()
	defer m.miningMu.Unlock()

	m.miningCtx, m.miningCancel = context.WithCancel(context.Background())
	m.isMining = true
	logf(colorBrightGreen, "✅ ", "ResumeMining: mining context recreated, isMining=true")
}

// isMiningActive checks if mining context is active
func (m *Miner) isMiningActive() bool {
	m.miningMu.Lock()
	defer m.miningMu.Unlock()

	if !m.isMining || m.miningCtx == nil {
		logf(colorYellow, "⚠️ ", fmt.Sprintf("isMiningActive: false (isMining=%v, miningCtx=%v)", m.isMining, m.miningCtx != nil))
		return false
	}

	select {
	case <-m.miningCtx.Done():
		m.isMining = false
		logf(colorYellow, "⚠️ ", "isMiningActive: context done, returning false")
		return false
	default:
		return true
	}
}

// isVerificationActive checks if verification is in progress
func (m *Miner) isVerificationActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := m.isVerifying
	if active {
		logf(colorBrightCyan, "🔍 ", "isVerificationActive: true")
	}
	return active
}

// Run starts the continuous mining loop
// Production-grade: implements all coordination logic for fork prevention
func (m *Miner) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Duration(config.DefaultMiningIntervalSec) * time.Second
	}

	t := time.NewTicker(interval)
	defer t.Stop()
	defer close(m.stopped)

	m.ResumeMining()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// CRITICAL: Pause mining during sync to prevent forks
			// Check if sync loop is available and currently syncing
			if m.syncLoop != nil && m.syncLoop.IsSyncing() {
				continue // Skip mining tick, wait for next ticker
			}

			// CRITICAL: Wait for chain stability after sync completes
			// This prevents mining on unstable chain
			if m.syncLoop != nil && !m.syncLoop.IsSynced() {
				continue // Skip mining tick, wait for next ticker
			}

			m.handleMiningTick(ctx, false)
		case <-m.wakeCh:
			m.handleMiningTick(ctx, false)
		}
	}
}

// Stop stops the miner
// Production-grade: gracefully stops mining operations
func (m *Miner) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// End any ongoing verification
	m.isVerifying = false
	m.verifyDoneCh = nil

	m.miningMu.Lock()
	if m.miningCancel != nil {
		m.miningCancel()
		m.isMining = false
	}
	m.miningMu.Unlock()

	select {
	case m.stopped <- struct{}{}:
	default:
	}

	logf(colorCyan, "ℹ️  Miner stopped")
	return nil
}

// handleMiningTick handles a single mining tick
func (m *Miner) handleMiningTick(ctx context.Context, force bool) {
	if m.syncLoop != nil && m.syncLoop.IsSyncing() {
		logf(colorYellow, "⚠️ ", "handleMiningTick: sync in progress, skipping")
		time.Sleep(time.Second)
		return
	}

	if m.syncLoop != nil && !m.syncLoop.IsSynced() {
		logf(colorYellow, "⚠️ ", "handleMiningTick: not synced, skipping")
		time.Sleep(time.Second)
		return
	}

	if m.isVerificationActive() {
		// CRITICAL: Do NOT skip mining during verification!
		// Verification is just a short delay to allow block propagation.
		// Mining should continue to maintain block production rate.
		// If there's a fork, the sync loop will handle it.
		logf(colorBrightCyan, "🔍 ", "handleMiningTick: verification active but continuing mining")
		// Fall through to continue mining
	}

	if !m.isMiningActive() {
		logf(colorRed, "❌ ", "handleMiningTick: mining not active, skipping")
		return
	}

	logf(colorBrightCyan, "🔍 ", "handleMiningTick: starting mining...")
	block, err := m.MineOnce(ctx, force)
	if err != nil {
		logf(colorRed, "❌ ", fmt.Sprintf("Mine once failed: %v", err))
		return
	}

	if block != nil {
		m.handleMinedBlock(ctx, block)
	} else {
		logf(colorYellow, "⚠️ ", "handleMiningTick: no block mined (empty mempool or no force)")
	}
}

// handleMinedBlock handles a successfully mined block
// Fixed: Broadcast block before checking network work
func (m *Miner) handleMinedBlock(ctx context.Context, block *core.Block) {
	logf(colorBrightGreen, "✅ ", fmt.Sprintf("Block #%d mined successfully, hash=%x", block.GetHeight(), block.Hash))

	// Broadcast block to network (non-blocking)
	if m.syncLoop != nil && m.pm != nil {
		m.broadcastBlockAsync(ctx, block)
		// Check network work after broadcast
		m.checkNetworkWork(ctx, block)
	}
}

// broadcastBlockAsync broadcasts block asynchronously to prevent blocking mining
func (m *Miner) broadcastBlockAsync(ctx context.Context, block *core.Block) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Miner] broadcastBlockAsync recovered from panic: %v", r)
			}
		}()

		// Broadcast block to peers via P2P manager
		if m.pm != nil {
			if err := m.pm.BroadcastBlock(ctx, block); err != nil {
				log.Printf("[Miner] failed to broadcast block: %v", err)
			}
		}
	}()
}

// checkNetworkWork checks if network has more work after broadcasting block
// CRITICAL FIX: After mining a block, we should NOT interrupt mining just because
// network appears to have more work. The 刚 mined block hasn't propagated yet,
// so peer work values are temporarily stale. The sync loop will detect if we're
// actually behind and trigger sync if needed. Mining should continue uninterrupted.
func (m *Miner) checkNetworkWork(ctx context.Context, block *core.Block) {
	logf(colorBrightCyan, "🔍 ", fmt.Sprintf("checkNetworkWork: block #%d", block.GetHeight()))

	networkMaxWork := m.getNetworkMaxWork(ctx)
	localWork := m.bc.CanonicalWork()

	logf(colorBrightCyan, "🔍 ", fmt.Sprintf("checkNetworkWork: localWork=%v, networkMaxWork=%v", localWork, networkMaxWork))

	if networkMaxWork != nil && networkMaxWork.Cmp(localWork) > 0 {
		logf(colorBrightYellow, "⚠️ ", fmt.Sprintf("Network has more work (local=%v, network=%v) - but continuing mining (刚 mined block not yet propagated)", localWork, networkMaxWork))
		// CRITICAL: Do NOT interrupt mining here!
		// The 刚 mined block hasn't propagated to peers yet, so their work values
		// are temporarily higher. The sync loop will detect if we're actually
		// behind and trigger sync if needed.
		// Interrupting mining here causes a deadlock: mining stops, but sync
		// never starts because we're not significantly behind.
	} else {
		logf(colorBrightGreen, "✅ ", "Local chain has most work, continuing mining")
	}

	// CRITICAL: Always resume mining to ensure continuous block production
	// This is safe because:
	// 1. If we're actually behind, sync loop will set isSyncing=true and stop mining
	// 2. If we're synced, mining should continue
	// 3. If we just mined a block, mining should continue for next block
	logf(colorBrightCyan, "🔍 ", "checkNetworkWork: calling ResumeMining()")
	m.ResumeMining()
	logf(colorBrightCyan, "🔍 ", "checkNetworkWork: ResumeMining() completed")
}

// getNetworkMaxWork gets the maximum work from all peers
func (m *Miner) getNetworkMaxWork(ctx context.Context) *big.Int {
	var networkMaxWork *big.Int
	localWork := m.bc.CanonicalWork()

	for _, peer := range m.pm.Peers() {
		info, err := m.pm.FetchChainInfo(ctx, peer)
		if err != nil {
			continue
		}
		if info.Work != nil && (networkMaxWork == nil || info.Work.Cmp(networkMaxWork) > 0) {
			networkMaxWork = info.Work
		}
	}

	_ = localWork
	return networkMaxWork
}

// MineOnce attempts to mine a single block
// Production-grade: implements complete mining logic with validation
func (m *Miner) MineOnce(ctx context.Context, force bool) (*core.Block, error) {
	if m.mp == nil {
		return nil, nil
	}

	// CRITICAL: Ensure mining context is valid before attempting to mine
	// This fixes the issue where mining stops after the first block
	m.miningMu.Lock()
	if !m.isMining || m.miningCtx == nil {
		logf(colorBrightCyan, "🔍 ", "MineOnce: recreating mining context (was not active)")
		m.miningCtx, m.miningCancel = context.WithCancel(context.Background())
		m.isMining = true
	} else {
		// Check if context is already done
		select {
		case <-m.miningCtx.Done():
			logf(colorBrightCyan, "🔍 ", "MineOnce: context was done, recreating")
			m.miningCtx, m.miningCancel = context.WithCancel(context.Background())
			m.isMining = true
		default:
			// Context is still valid, just ensure isMining is true
			m.isMining = true
		}
	}
	m.miningMu.Unlock()

	selected, selectedIDs, err := m.bc.SelectMempoolTxs(m.mp, m.maxTxPerBlock)
	if err != nil {
		logf(colorRed, "❌ ", fmt.Sprintf("Select txs failed: %v", err))
		return nil, err
	}

	mineEmpty := force || m.forceEmptyBlocks
	if len(selected) == 0 && !mineEmpty {
		return nil, nil
	}

	parentAtMineTime := m.bc.LatestBlock()
	if parentAtMineTime == nil {
		logf(colorRed, "❌ ", "No parent block at mining time")
		return nil, errors.New("no parent block")
	}

	b, err := m.bc.MineTransfers(ctx, selected)
	if err != nil {
		logf(colorRed, "❌ ", fmt.Sprintf("Mine failed: %v", err))
		return nil, err
	}

	if b == nil {
		return nil, nil
	}

	propagationDelay := calculateAdaptivePropagationDelay(m.pm)
	time.Sleep(time.Duration(propagationDelay) * time.Millisecond)

	go m.broadcastBlock(ctx, b)

	if len(selectedIDs) > 0 {
		m.mp.RemoveMany(selectedIDs)
		m.publishRemoveEvent(selectedIDs)
	}

	return b, nil
}

// publishRemoveEvent publishes mempool removal events
func (m *Miner) publishRemoveEvent(txids []string) {
	m.mu.RLock()
	sink := m.events
	m.mu.RUnlock()

	if sink != nil {
		sink.Publish(WSEvent{
			Type: "mempool_removed",
			Data: map[string]interface{}{
				"txIds":  txids,
				"reason": "mined",
			},
		})
	}
}

// broadcastBlock broadcasts the mined block to all peers
func (m *Miner) broadcastBlock(ctx context.Context, block *core.Block) {
	if !isPeerAPIValid(m.pm) {
		return
	}

	// Broadcast block to peers via P2P manager
	if m.pm != nil {
		if err := m.pm.BroadcastBlock(ctx, block); err != nil {
			log.Printf("[Miner] failed to broadcast block: %v", err)
		}
	}
}

// calculateAdaptivePropagationDelay calculates adaptive delay based on network conditions
func calculateAdaptivePropagationDelay(pm PeerAPI) int {
	if !isPeerAPIValid(pm) {
		return config.DefaultBlockPropagationDelayMs
	}

	peers := pm.Peers()
	if len(peers) == 0 {
		return config.DefaultBlockPropagationDelayMs / 2
	}

	baseDelay := config.DefaultBlockPropagationDelayMs
	peerFactor := 0
	if len(peers) > 1 {
		peerFactor = int(50 * float64(len(peers)-1) / float64(len(peers)))
	}

	adaptiveDelay := baseDelay + peerFactor
	if adaptiveDelay > 3000 {
		adaptiveDelay = 3000
	}

	return adaptiveDelay
}

// isPeerAPIValid checks if PeerAPI is valid (not nil)
func isPeerAPIValid(pm PeerAPI) bool {
	if pm == nil {
		return false
	}
	v := reflect.ValueOf(pm)
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return !v.IsNil()
	default:
		return true
	}
}

// getPeerHeight gets the maximum peer height
func getPeerHeight(pm PeerAPI) uint64 {
	var maxPeerHeight uint64
	for _, peer := range pm.Peers() {
		info, err := pm.FetchChainInfo(context.Background(), peer)
		if err != nil || info == nil {
			continue
		}
		if info.Height > maxPeerHeight {
			maxPeerHeight = info.Height
		}
	}
	return maxPeerHeight
}

// validateBlockPoW validates the proof of work for a block
// Note: NogoPow engine already validates PoW during Seal, this just validates block structure
func validateBlockPoW(consensus config.ConsensusParams, block, parent *core.Block) error {
	if block == nil {
		return errors.New("block is nil")
	}
	if parent == nil {
		return errors.New("parent is nil")
	}

	if block.GetHeight() != parent.GetHeight()+1 {
		return fmt.Errorf("invalid block height: expected %d, got %d", parent.GetHeight()+1, block.GetHeight())
	}

	if string(block.Header.PrevHash) != string(parent.Hash) {
		return errors.New("invalid previous hash")
	}

	// PoW already validated by NogoPow engine during Seal
	// No need to re-validate here
	return nil
}

// CreateCoinbaseTx creates a coinbase transaction for the block reward
// Production-grade: implements proper coinbase transaction structure
func CreateCoinbaseTx(minerAddress string, height uint64, totalFees uint64, chainID uint64, consensus config.ConsensusParams) (*core.Transaction, error) {
	if minerAddress == "" {
		return nil, errors.New("miner address is required")
	}

	if err := core.ValidateAddress(minerAddress); err != nil {
		return nil, fmt.Errorf("invalid miner address: %w", err)
	}

	blockReward := consensus.MonetaryPolicy.BlockReward(height)
	// Miner receives MinerRewardShare% of block reward
	// Transaction fees are burned (MinerFeeShare=0) to create deflationary pressure
	minerReward := blockReward * uint64(consensus.MonetaryPolicy.MinerRewardShare) / 100
	minerFee := consensus.MonetaryPolicy.MinerFeeAmount(totalFees)
	totalAmount := minerReward + minerFee

	if totalAmount > math.MaxUint64 {
		return nil, errors.New("coinbase amount overflow")
	}

	coinbase := &core.Transaction{
		Type:      core.TxCoinbase,
		ChainID:   chainID,
		ToAddress: minerAddress,
		Amount:    totalAmount,
		Fee:       0,
		Nonce:     0,
		Data:      fmt.Sprintf("height=%d", height),
	}

	if err := coinbase.Verify(); err != nil {
		return nil, fmt.Errorf("invalid coinbase transaction: %w", err)
	}

	return coinbase, nil
}

// CreateBlockTemplate creates a block template for mining
// Production-grade: prepares all block fields except nonce
func CreateBlockTemplate(
	parent *core.Block,
	txs []core.Transaction,
	minerAddress string,
	chainID uint64,
	consensus config.ConsensusParams,
) (*core.Block, error) {
	if parent == nil {
		return nil, errors.New("parent block is required")
	}

	if minerAddress == "" {
		return nil, errors.New("miner address is required")
	}

	totalFees := uint64(0)
	for _, tx := range txs {
		if totalFees > math.MaxUint64-tx.Fee {
			return nil, errors.New("total fees overflow")
		}
		totalFees += tx.Fee
	}

	coinbase, err := CreateCoinbaseTx(minerAddress, parent.GetHeight()+1, totalFees, chainID, consensus)
	if err != nil {
		return nil, fmt.Errorf("create coinbase: %w", err)
	}

	allTxs := append([]core.Transaction{*coinbase}, txs...)

	// CRITICAL: Use consensus-aware merkle root calculation
	// This ensures the merkle root matches what the node validates during AddBlock
	merkleRoot, err := computeMerkleRootForConsensus(allTxs, consensus, parent.GetHeight()+1)
	if err != nil {
		return nil, fmt.Errorf("compute merkle root: %w", err)
	}

	// Calculate block version based on consensus rules
	blockVersion := getBlockVersion(consensus, parent.GetHeight()+1)

	template := &core.Block{
		Height:       parent.GetHeight() + 1,
		MinerAddress: minerAddress,
		Transactions: allTxs,
		CoinbaseTx:   coinbase,
		Header: core.BlockHeader{
			Version:        blockVersion,
			PrevHash:       append([]byte(nil), parent.Hash...),
			TimestampUnix:  time.Now().Unix(),
			DifficultyBits: parent.Header.DifficultyBits,
			MerkleRoot:     merkleRoot,
		},
	}

	if template.Header.TimestampUnix <= parent.Header.TimestampUnix {
		template.Header.TimestampUnix = parent.Header.TimestampUnix + 1
	}

	return template, nil
}

// computeMerkleRootForConsensus computes merkle root using consensus-aware transaction hash
// Production-grade: matches validateMerkleRootLocked implementation in chain.go
func computeMerkleRootForConsensus(txs []core.Transaction, consensus config.ConsensusParams, height uint64) ([]byte, error) {
	if len(txs) == 0 {
		return make([]byte, 32), nil
	}

	leaves := make([][]byte, len(txs))
	for i, tx := range txs {
		th, err := core.TxSigningHashForConsensus(tx, consensus, height)
		if err != nil {
			return nil, fmt.Errorf("compute tx hash: %w", err)
		}
		leaves[i] = th
	}

	return core.ComputeMerkleRoot(leaves)
}

// computeMerkleRoot computes the Merkle root of transactions (legacy format)
// Deprecated: use computeMerkleRootForConsensus for consensus-critical operations
func computeMerkleRoot(txs []core.Transaction) ([]byte, error) {
	if len(txs) == 0 {
		return make([]byte, 32), nil
	}

	leaves := make([][]byte, len(txs))
	for i, tx := range txs {
		hash := sha256.Sum256([]byte(fmt.Sprintf("%v", tx)))
		leaves[i] = hash[:]
	}

	return computeMerkleTree(leaves), nil
}

// computeMerkleTree computes the Merkle root from leaves
func computeMerkleTree(leaves [][]byte) []byte {
	if len(leaves) == 0 {
		return make([]byte, 32)
	}

	if len(leaves) == 1 {
		hash := sha256.Sum256(leaves[0])
		return hash[:]
	}

	for len(leaves) > 1 {
		nextLevel := make([][]byte, (len(leaves)+1)/2)
		for i := 0; i < len(leaves); i += 2 {
			var combined []byte
			if i+1 < len(leaves) {
				combined = append(leaves[i], leaves[i+1]...)
			} else {
				combined = append(leaves[i], leaves[i]...)
			}
			hash := sha256.Sum256(combined)
			nextLevel[i/2] = hash[:]
		}
		leaves = nextLevel
	}

	return leaves[0]
}
