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

package core

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/index"
	"github.com/nogochain/nogo/blockchain/nogopow"
)

func (c *Chain) getPowEngine() *nogopow.NogopowEngine {
	c.powEngineOnce.Do(func() {
		if powModeCache.mode == "fake" {
			c.powEngine = nogopow.NewFaker()
		} else {
			powConfig := nogopow.DefaultConfig()
			powConfig.ConsensusParams = &c.consensus
			c.powEngine = nogopow.New(powConfig)
		}
	})
	return c.powEngine
}

// SetEventSink sets the event sink for publishing blockchain events
// Production-grade: enables WebSocket real-time notifications for new blocks
// Concurrency safety: safe to call before chain is used (during initialization)
func (c *Chain) SetEventSink(sink EventSink) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = sink
}

// SetMempool sets the mempool reference for automatic cleanup of confirmed transactions
// Production-grade: enables Chain to remove confirmed transactions from mempool
// Dependency injection: called during node initialization after mempool creation
// Thread-safety: uses mutex to ensure safe concurrent access
func (c *Chain) SetMempool(mp MempoolCleaner) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mempool = mp
}

// SetSyncNotifier sets the sync notifier for chain reorganization callbacks
// Production-grade: enables sync loop to re-evaluate state after fork rollback
// Dependency injection: called during node initialization
// Thread-safety: uses mutex to ensure safe concurrent access
func (c *Chain) SetSyncNotifier(notifier SyncNotifier) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.syncNotifier = notifier
}

// SetOnForkResolved sets the callback function to be called after fork rollback completes
// Production-grade: triggers immediate re-sync via BlockKeeper.forkResolvedCh
// CRITICAL: This callback must be non-blocking (use channel send with select+default)
// Dependency injection: called during node initialization after BlockKeeper creation
// Parameters:
//   - newHeight: the chain height after rollback
//   - rolledBack: number of blocks that were rolled back
func (c *Chain) SetOnForkResolved(callback func(newHeight uint64, rolledBack uint64)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onForkResolved = callback
}

// SetReorgExecutor sets the unified reorg executor for centralized fork resolution
// CRITICAL: Must be called during node initialization after ForkResolver is created
// When set, all reorg operations from Chain.AddBlock() will delegate to this executor
// This ensures:
//   - Global TryLock mutex prevents concurrent reorganizations
//   - Preventive timing (500ms for light forks) stops deep fork accumulation
//   - Single entry point for all reorg paths (network + consensus + chain)
func (c *Chain) SetReorgExecutor(executor ReorgExecutor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reorgExecutor = executor
	log.Printf("[Chain] ReorgExecutor set for unified fork resolution - all reorgs will go through centralized engine")
}

// SetCheckpointVoter assigns the checkpoint voter for multi-sig consensus.
func (c *Chain) SetCheckpointVoter(voter *CheckpointVoter) {
	c.checkpointVoteMu.Lock()
	defer c.checkpointVoteMu.Unlock()
	c.checkpointVoter = voter
	if voter != nil {
		voter.SetOnCheckpointFinalized(c.onCheckpointFinalized)
	}
}

// SetOnCheckpointBlock sets the callback for broadcasting checkpoint votes via P2P.
func (c *Chain) SetOnCheckpointBlock(callback func(height uint64, blockHash string, vote *CheckpointVote)) {
	c.checkpointVoteMu.Lock()
	defer c.checkpointVoteMu.Unlock()
	c.onCheckpointBlock = callback
}

func (c *Chain) onCheckpointFinalized(record *CheckpointRecord) {
	log.Printf("[Chain] Checkpoint finalized at h=%d (sigs=%d)", record.Height, len(record.Signatures))
}

// GetCheckpointVoter returns the checkpoint voter for sync queries.
func (c *Chain) GetCheckpointVoter() *CheckpointVoter {
	c.checkpointVoteMu.RLock()
	defer c.checkpointVoteMu.RUnlock()
	return c.checkpointVoter
}

// SetOnBlockAdded sets the callback function to be called when a block is added
// Production-grade: enables broadcasting of blocks added via API (e.g., from mining pool)
func (c *Chain) SetOnBlockAdded(callback func(*Block)) {
	c.onBlockMu.Lock()
	defer c.onBlockMu.Unlock()
	c.onBlockAdded = callback
}

// GetOnBlockAdded returns the current callback function
func (c *Chain) GetOnBlockAdded() func(*Block) {
	c.onBlockMu.RLock()
	defer c.onBlockMu.RUnlock()
	return c.onBlockAdded
}

// SetOnMissingBlock sets the callback function to be called when an orphan block is received
// Production-grade: enables automatic request of missing parent blocks from peers
func (c *Chain) SetOnMissingBlock(callback func(parentHash []byte, height uint64)) {
	c.onMissingMu.Lock()
	defer c.onMissingMu.Unlock()
	c.onMissingBlock = callback
}

// GetOnMissingBlock returns the current callback function
func (c *Chain) GetOnMissingBlock() func(parentHash []byte, height uint64) {
	c.onMissingMu.RLock()
	defer c.onMissingMu.RUnlock()
	return c.onMissingBlock
}

// CalcNextDifficulty calculates the difficulty for the next block
// Production-grade: uses PI controller from consensus engine for accurate difficulty adjustment
// Parameters:
//   - latest: the parent block (latest block in the chain)
//   - currentTime: Unix timestamp for the new block
//
// Returns:
//   - uint32: difficulty bits for the next block
func (c *Chain) CalcNextDifficulty(latest *Block, currentTime int64) uint32 {
	// Guard clause: no parent block, return minimum difficulty
	if latest == nil {
		c.mu.RLock()
		defer c.mu.RUnlock()
		return uint32(c.consensus.MinDifficulty)
	}

	// Difficulty cache: only recalculate when parent block height changes.
	// Fixes "difficulty death spiral": the pool polls /block/template every 3
	// seconds. Previously each call created a fresh PI controller with time.Now(),
	// causing timeDiff to grow and difficulty to spiral down continuously.
	// Now difficulty is computed once per block height and cached — matching the
	// node's own mining pattern where the persistent diffAdjuster is called
	// once per Prepare().
	c.diffCacheMu.Lock()
	if latest.GetHeight() == c.diffCacheHeight {
		cached := c.diffCacheValue
		c.diffCacheMu.Unlock()
		return cached
	}
	c.diffCacheMu.Unlock()

	// Cache miss: calculate next difficulty using deterministic PI controller
	c.mu.RLock()
	params := &config.ConsensusParams{
		BlockTimeTargetSeconds:     c.consensus.BlockTimeTargetSeconds,
		MaxDifficultyChangePercent: c.consensus.MaxDifficultyChangePercent,
		MinDifficulty:              c.consensus.MinDifficulty,
	}
	c.mu.RUnlock()

	calc := nogopow.NewDifficultyCalculator(params)

	// Set ancestor function for deterministic difficulty calculation.
	// This ensures all nodes compute the exact same difficulty from the
	// same chain state, preventing validator state contamination.
	calc.SetAncestorFunc(func(height uint64) *nogopow.Header {
		block, ok := c.GetBlock(height)
		if !ok || block == nil {
			return nil
		}
		var prevHash nogopow.Hash
		copy(prevHash[:], block.Header.PrevHash)
		return &nogopow.Header{
			Number:     big.NewInt(int64(block.GetHeight())),
			Time:       uint64(block.Header.TimestampUnix),
			Difficulty: big.NewInt(int64(block.Header.DifficultyBits)),
			ParentHash: prevHash,
		}
	})

	// Convert block to BlockHeader format
	parentHeader := &nogopow.BlockHeader{
		Height:         latest.GetHeight(),
		TimestampUnix:  latest.Header.TimestampUnix,
		DifficultyBits: latest.Header.DifficultyBits,
		PrevHash:       latest.Header.PrevHash,
		Hash:           latest.Hash,
	}

	// Calculate next difficulty using PI controller
	nextDifficulty := calc.CalcNextDifficulty(parentHeader, uint64(currentTime))

	// Cache the result for subsequent calls at same height
	c.diffCacheMu.Lock()
	c.diffCacheHeight = latest.GetHeight()
	c.diffCacheValue = nextDifficulty
	c.diffCacheMu.Unlock()

	return nextDifficulty
}

// NewChain creates a new blockchain instance
// Production-grade: initializes all indexes and loads from storage
// Error handling: returns error on initialization failure
func NewChain(cfg ChainConfig) (*Chain, error) {
	if cfg.Store == nil {
		return nil, errors.New("chain store is required")
	}

	// Load consensus parameters from genesis config
	genesisCfg, err := LoadGenesisConfigWithChainID(cfg.GenesisPath, cfg.ChainID)
	if err != nil {
		return nil, fmt.Errorf("load genesis config: %w", err)
	}

	// Validate chain ID match
	if cfg.ChainID != 0 && genesisCfg.ChainID != cfg.ChainID {
		return nil, fmt.Errorf("genesis chainId mismatch: env=%d genesis=%d", cfg.ChainID, genesisCfg.ChainID)
	}
	cfg.ChainID = genesisCfg.ChainID

	// Validate miner address format
	if cfg.MinerAddress != "" {
		if err := validateAddressFormat(cfg.MinerAddress); err != nil {
			return nil, fmt.Errorf("invalid miner address: %w", err)
		}
	}

	// Create context for background goroutines
	ctx, cancel := context.WithCancel(context.Background())

	chain := &Chain{
		chainID:                 cfg.ChainID,
		minerAddress:            cfg.MinerAddress,
		genesisAddress:          genesisCfg.GenesisMinerAddress,
		genesisTimestamp:        genesisCfg.Timestamp,
		consensus:               genesisCfg.ConsensusParams,
		monetaryPolicy:          genesisCfg.MonetaryPolicy,
		state:                   make(map[string]Account),
		store:                   cfg.Store,
		blocksByHash:            make(map[string]*Block),
		blocks:                  make([]*Block, 0),
		forkBlocks:              make(map[uint64][]*Block),
		forkBlocksByHash:        make(map[string]*Block),
		workCache:               make(map[string]*big.Int),
		txIndex:                 make(map[string]TxLocation),
		addressIndex:            make(map[string][]AddressTxEntry),
		indexPath:               cfg.IndexPath,
		canonicalWork:           big.NewInt(0),
		pendingAncestorRequests: make(map[string]time.Time),
		ctx:                     ctx,
		cancel:                  cancel,
		// Initialize contract manager
		contractManager: NewContractManager(),
	}

	// Initialize contracts at genesis
	if err := chain.contractManager.InitializeContracts(
		genesisCfg.CommunityFundAddress,
	); err != nil {
		return nil, fmt.Errorf("initialize contracts: %w", err)
	}

	// Initialize rules hash for consensus validation
	curRulesHash := chain.consensus.MustRulesHash()
	chain.rulesHash = curRulesHash

	// Load blocks from storage
	blocks, err := cfg.Store.ReadCanonical()
	if err != nil {
		return nil, fmt.Errorf("read canonical chain: %w", err)
	}
	chain.blocks = blocks

	// Load all blocks (including orphans)
	allBlocks, err := cfg.Store.ReadAllBlocks()
	if err != nil {
		return nil, fmt.Errorf("read all blocks: %w", err)
	}
	if len(allBlocks) > 0 {
		chain.blocksByHash = allBlocks
	}

	// Validate rules hash consistency
	if err := chain.validateRulesHashLocked(); err != nil {
		return nil, err
	}

	// Initialize genesis block if needed
	if len(chain.blocks) == 0 {
		if err := chain.initializeGenesisLocked(genesisCfg); err != nil {
			return nil, fmt.Errorf("initialize genesis: %w", err)
		}
	} else {
		// Validate existing genesis block
		if err := ValidateGenesisBlock(chain.blocks[0], genesisCfg, chain.consensus); err != nil {
			return nil, fmt.Errorf("validate genesis: %w", err)
		}
	}

	// Try loading state snapshot first (P0-1 Fix: state persistence)
	// This is O(1) vs O(n) for recomputing from all blocks
	if chain.store != nil {
		snapshotHeight, stateRoot, snapshotState, err := chain.store.LoadSnapshot(chain.currentHeight())
		if err == nil && snapshotState != nil && len(snapshotState) > 0 {
			chain.state = snapshotState
			log.Printf("[Chain] Loaded state snapshot at height %d (root=%x, accounts=%d)", snapshotHeight, stateRoot[:8], len(snapshotState))

			currentH := chain.currentHeight()
			if snapshotHeight < currentH {
				blocksToApply := currentH - snapshotHeight
				for i := snapshotHeight + 1; i <= currentH; i++ {
					block := chain.blocks[i]
					if err := applyBlockToState(chain.consensus, chain.monetaryPolicy, chain.state, block, chain.genesisAddress, chain.genesisTimestamp); err != nil {
						return nil, fmt.Errorf("apply post-snapshot block %d: %w", block.GetHeight(), err)
					}
				}
				log.Printf("[Chain] Applied %d blocks after snapshot (heights %d -> %d, final accounts=%d)", blocksToApply, snapshotHeight+1, currentH, len(chain.state))

				updatedRoot, sErr := chain.store.CalculateStateRoot(chain.state)
				if sErr == nil {
					if snapErr := chain.store.Snapshot(currentH, updatedRoot, chain.state); snapErr != nil {
						log.Printf("[Chain] WARNING: failed to save updated snapshot at height %d: %v", currentH, snapErr)
					} else {
						log.Printf("[Chain] Saved updated state snapshot at height %d (root=%x)", currentH, updatedRoot[:8])
					}
				}
			}
		} else {
			// Snapshot not found or failed, recompute from blocks - O(n)
			if err := chain.recomputeStateLocked(); err != nil {
				return nil, fmt.Errorf("recompute state: %w", err)
			}
			log.Printf("[Chain] Recomputed state from %d blocks (no snapshot available)", len(chain.blocks))

			// Save snapshot after initial recomputation to avoid O(n) rebuild on next restart.
			// This is critical: without this, every restart recomputes all state from genesis.
			if len(chain.blocks) > 0 {
				currentH := chain.currentHeight()
				stateRoot, sErr := chain.store.CalculateStateRoot(chain.state)
				if sErr == nil {
					if snapErr := chain.store.Snapshot(currentH, stateRoot, chain.state); snapErr != nil {
						log.Printf("[Chain] WARNING: failed to save initial snapshot at height %d: %v", currentH, snapErr)
					} else {
						log.Printf("[Chain] Saved initial state snapshot at height %d (root=%x, next restart will be fast)", currentH, stateRoot[:8])
					}
				}
			}
		}
	} else {
		// No persistent store available, recompute from blocks - O(n)
		if err := chain.recomputeStateLocked(); err != nil {
			return nil, fmt.Errorf("recompute state: %w", err)
		}
	}

	// Initialize indexes
	chain.initCanonicalIndexesLocked()

	// Initialize BoltDB address index
	if err := chain.initAddressIndexLocked(); err != nil {
		return nil, fmt.Errorf("init address index: %w", err)
	}

	// CRITICAL: Recalculate canonical work and set TotalWork on all blocks
	// After loading from storage, canonicalWork=0 and blocks have empty TotalWork.
	// Without this, the node reports work=0 to peers, causing TieBreaker to always
	// prefer other chains and permanently lose fork resolution competitions.
	// This is the same logic as reorganizeChainLocked (line 2496-2501).
	chain.canonicalWork = big.NewInt(0)
	for _, block := range chain.blocks {
		work := WorkForDifficultyBits(block.Header.DifficultyBits)
		chain.canonicalWork.Add(chain.canonicalWork, work)
		block.TotalWork = chain.canonicalWork.String()
	}
	log.Printf("[Chain] Initialized chain work from %d blocks: totalWork=%s", len(chain.blocks), chain.canonicalWork.String())

	// Process any orphan blocks loaded from storage
	// This connects blocks that were downloaded but not yet added to canonical chain
	chain.processLoadedOrphansLocked()

	return chain, nil
}

// Start launches background goroutines for chain maintenance
// P1 Issue 1.5.1: Orphan pool unlimited growth
// This function starts periodic orphan cleanup to prevent memory exhaustion attacks
func (c *Chain) Start() {
	// Start orphan cleanup loop
	go c.startOrphanCleanupLoop()
	log.Printf("[Chain] Started background maintenance goroutines")
}

// Stop cancels all background goroutines
func (c *Chain) Stop() {
	if c.cancel != nil {
		c.cancel()
		log.Printf("[Chain] Stopped background maintenance goroutines")
	}
}

// startOrphanCleanupLoop periodically cleans up expired orphan blocks
// P1 Issue 1.5.1: Orphan pool unlimited growth
// This prevents memory exhaustion attacks by removing old orphan blocks
func (c *Chain) startOrphanCleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	log.Printf("[Chain] Orphan cleanup loop started (interval=5min, maxAge=%v, maxSize=%d)",
		MaxOrphanPoolAge, MaxOrphanPoolSize)

	for {
		select {
		case <-c.ctx.Done():
			log.Printf("[Chain] Orphan cleanup loop stopped")
			return
		case <-ticker.C:
			c.mu.Lock()
			orphanCount := len(c.orphanPool)
			c.cleanupExpiredOrphansLocked()
			newOrphanCount := len(c.orphanPool)
			c.mu.Unlock()

			if orphanCount != newOrphanCount {
				log.Printf("[Chain] Orphan cleanup: removed %d orphans (before=%d, after=%d)",
					orphanCount-newOrphanCount, orphanCount, newOrphanCount)
			}
		}
	}
}

// cleanupExpiredOrphansLocked removes orphans that exceed MaxOrphanPoolAge
// This is called periodically by startOrphanCleanupLoop and during orphan addition
func (c *Chain) cleanupExpiredOrphansLocked() {
	if len(c.orphanTimestamps) == 0 {
		return
	}

	now := time.Now()
	expiredCount := 0

	for hashHex, timestamp := range c.orphanTimestamps {
		if now.Sub(timestamp) > MaxOrphanPoolAge {
			c.removeOrphanLocked(hashHex)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		log.Printf("[Chain] Cleaned up %d expired orphans (maxAge=%v)", expiredCount, MaxOrphanPoolAge)
	}
}

// NewBlockchain creates a new blockchain instance (alias for NewChain for compatibility)
// Production-grade: wrapper function for backward compatibility with backup code
func NewBlockchain(store interface{}, cfg interface{}) (*Chain, error) {
	// Extract chain config from interface
	var chainCfg ChainConfig

	if c, ok := cfg.(*ChainConfig); ok {
		chainCfg = *c
	} else if c, ok := cfg.(ChainConfig); ok {
		chainCfg = c
	} else {
		// Default config for compatibility
		chainCfg = ChainConfig{
			ChainID: 1,
		}
	}

	return NewChain(chainCfg)
}

// initAddressIndexLocked initializes the BoltDB address index
// Production-grade: creates index database and builds from existing blocks
// Concurrency safety: must be called with mutex held
func (c *Chain) initAddressIndexLocked() error {
	if c.indexPath == "" {
		c.indexPath = "nogodata/blockchain_data"
	}

	indexDir := filepath.Join(c.indexPath, "address_index")
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		return fmt.Errorf("create index dir: %w", err)
	}

	indexDBPath := filepath.Join(indexDir, "address.db")
	addrIndex, err := index.NewAddressIndex(indexDBPath)
	if err != nil {
		return fmt.Errorf("create address index: %w", err)
	}

	c.addressIndexBolt = addrIndex

	if len(c.blocks) == 0 {
		return nil
	}

	currentHeight := uint64(len(c.blocks) - 1)
	lastIndexed, _ := c.addressIndexBolt.GetIndexedHeight()

	if lastIndexed >= currentHeight {
		log.Printf("[Chain] Address index already up to date (height=%d, blocks=%d), skipping rebuild", lastIndexed, len(c.blocks))
		return nil
	}

	startFrom := lastIndexed + 1
	log.Printf("[Chain] Incremental address index build: height %d -> %d (%d new blocks)...", startFrom-1, currentHeight, currentHeight-lastIndexed+1)

	for i := startFrom; i <= currentHeight; i++ {
		block := c.blocks[i]
		entries := make([]index.AddressIndexEntry, 0, len(block.Transactions))
		for _, tx := range block.Transactions {
			if tx.Type != TxTransfer {
				continue
			}
			fromAddr, err := tx.FromAddress()
			if err != nil {
				continue
			}
			txID, err := tx.GetIDWithError()
			if err != nil {
				continue
			}
			entries = append(entries, index.AddressIndexEntry{
				TxID:      txID,
				FromAddr:  fromAddr,
				ToAddress: tx.ToAddress,
				Amount:    tx.Amount,
				Fee:       tx.Fee,
				Nonce:     tx.Nonce,
				Type:      index.TransactionType(tx.Type),
			})
		}
		if err := c.addressIndexBolt.IndexBlockSimple(block.Hash, block.GetHeight(), block.Header.TimestampUnix, entries); err != nil {
			log.Printf("WARNING: index block %d: %v", block.GetHeight(), err)
		}
	}

	c.addressIndexBolt.SetIndexedHeight(currentHeight)
	log.Printf("[Chain] Address index built successfully (height=%d)", currentHeight)

	return nil
}

// Close closes the address index database
// Resource management: properly closes BoltDB connection
func (c *Chain) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.addressIndexBolt != nil {
		if err := c.addressIndexBolt.Close(); err != nil {
			log.Printf("WARNING: close address index: %v", err)
		}
	}

	return nil
}
