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
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/index"
	"github.com/nogochain/nogo/blockchain/nogopow"
)

const (
	// MaxOrphanPoolSize is the maximum number of orphan blocks to store
	MaxOrphanPoolSize = 2048

	// MaxOrphanPoolAge is the maximum time an orphan can stay in the pool
	MaxOrphanPoolAge = 10 * time.Minute

	// MaxBlocksByHashSize limits the total number of blocks in blocksByHash map
	MaxBlocksByHashSize = 100000

	// MaxForkBlocksPerHeight limits the number of fork blocks per height
	MaxForkBlocksPerHeight = 16

	// SnapshotInterval is the block interval for state snapshot creation.
	// Aligned to power-of-2 for efficient modulo and consistent disk I/O scheduling.
	SnapshotInterval = 256

	// MaxWorkCacheSize limits the number of cumulative work cache entries
	MaxWorkCacheSize = 100000

	// MaxForkBlocksTotal limits the total number of fork blocks across all heights
	MaxForkBlocksTotal = 50000

	// MaxReorgDepth limits how far back findBestChainTipLocked scans fork blocks
	// Prevents infinite oscillation by ignoring forks too deep below canonical tip
	MaxReorgDepth = 100

	// MinAutoReorgInterval is the minimum time between auto-reorg checks.
	// Debounces autoReorgIfNeededLocked to prevent O(N) scans on every fork block.
	MinAutoReorgInterval = 200 * time.Millisecond
)

// SyncNotifier defines the interface for notifying sync loop about chain events
// Production-grade: enables coordination between chain and sync loop
type SyncNotifier interface {
	// OnChainReorganized is called after chain reorganization completes
	// Allows sync loop to re-evaluate chain state and trigger sync if needed
	OnChainReorganized(newTip *Block)
}

var (
	// ErrInvalidPoW is returned when POW verification fails
	ErrInvalidPoW = errors.New("invalid proof of work")
	// ErrInvalidBlock is returned when block validation fails
	ErrInvalidBlock = errors.New("invalid block")
	// ErrOrphanBlock is returned when block parent is not found
	ErrOrphanBlock = errors.New("orphan block")
	// ErrKnownBlock is returned when block already exists in chain
	ErrKnownBlock = errors.New("block already known")
	// ErrInvalidMerkleRoot is returned when merkle root verification fails
	ErrInvalidMerkleRoot = errors.New("invalid merkle root")
	// ErrInvalidTimestamp is returned when timestamp validation fails
	ErrInvalidTimestamp = errors.New("invalid timestamp")
	// ErrInvalidDifficulty is returned when difficulty validation fails
	ErrInvalidDifficulty = errors.New("invalid difficulty")
	// ErrInvalidCoinbase is returned when coinbase transaction is invalid
	ErrInvalidCoinbase = errors.New("invalid coinbase transaction")
	// ErrInvalidTransaction is returned when transaction validation fails
	ErrInvalidTransaction = errors.New("invalid transaction")
)

// powCache stores computed cache data to avoid recalculation
// Key: seed hash, Value: computed cache data
// Concurrency safety: protected by RWMutex and atomic counters
var powCache = struct {
	mu    sync.RWMutex
	cache map[nogopow.Hash][]uint32
	stats struct {
		hits   uint64 // atomic
		misses uint64 // atomic
	}
}{
	cache: make(map[nogopow.Hash][]uint32),
}

// TxLocation represents transaction location in the chain
type TxLocation struct {
	Height       uint64 `json:"height"`
	BlockHashHex string `json:"blockHashHex"`
	Index        int    `json:"index"`
}

// applyBlockToState applies a block to the state
// Production-grade: validates and updates account balances
func applyBlockToState(p ConsensusParams, mp MonetaryPolicy, state map[string]Account, b *Block, genesisAddress string, genesisTimestamp int64) error {
	if p.MaxBlockSize > 0 {
		size, err := blockSizeForConsensus(b)
		if err != nil {
			return err
		}
		if uint64(size) > p.MaxBlockSize {
			return fmt.Errorf("block too large: %d bytes (max %d)", size, p.MaxBlockSize)
		}
	}
	if len(b.Transactions) == 0 {
		return errors.New("block has no transactions")
	}
	// Enforce coinbase position
	if b.Transactions[0].Type != TxCoinbase {
		return errors.New("first tx must be coinbase")
	}

	// Consensus economics: for non-genesis blocks, coinbase must pay subsidy + miner fee share
	// to the block's declared miner address.
	if b.GetHeight() > 0 {
		if err := validateAddress(b.MinerAddress); err != nil {
			return fmt.Errorf("invalid minerAddress: %w", err)
		}
		var fees uint64
		for _, tx := range b.Transactions[1:] {
			if tx.Type != TxTransfer {
				continue
			}
			fees += tx.Fee
		}
		cb := b.Transactions[0]
		if cb.ToAddress != b.MinerAddress {
			return errors.New("coinbase toAddress must match minerAddress")
		}
		policy := mp
		// Miner receives MinerRewardShare% of block reward
		// Transaction fees are burned (MinerFeeShare=0) to create deflationary pressure
		expected := policy.BlockReward(b.GetHeight())*uint64(policy.MinerRewardShare)/100 + policy.MinerFeeAmount(fees)
		if cb.Amount != expected {
			return fmt.Errorf("bad coinbase amount: expected %d got %d", expected, cb.Amount)
		}

		// Distribute block rewards according to economic model.
		// Reward allocation: 99% MinerRewardShare + 1% GenesisShare.
		// CommunityFundShare(0%) and IntegrityPoolShare(0%) are permanently disabled.
		// Transaction fees are burned (MinerFeeShare=0%).
		blockReward := policy.BlockReward(b.GetHeight())

		// Genesis Address (1%) - to preset genesis miner address
		genesisReward := blockReward * uint64(policy.GenesisShare) / 100
		if genesisReward > 0 {
			acct := state[genesisAddress]
			if acct.Balance > math.MaxUint64-genesisReward {
				return errors.New("genesis address balance overflow")
			}
			acct.Balance += genesisReward
			state[genesisAddress] = acct
		}
	}

	for i, tx := range b.Transactions {
		switch tx.Type {
		case TxCoinbase:
			if i != 0 {
				return errors.New("coinbase must be first")
			}
			if err := tx.VerifyForConsensus(p, b.GetHeight()); err != nil {
				return err
			}
			acct := state[tx.ToAddress]
			// Overflow check: ensure balance + amount doesn't overflow
			if acct.Balance > math.MaxUint64-tx.Amount {
				return errors.New("coinbase balance overflow")
			}
			acct.Balance += tx.Amount
			state[tx.ToAddress] = acct
		case TxTransfer:
			if err := tx.VerifyForConsensus(p, b.GetHeight()); err != nil {
				return err
			}
			fromAddr, err := tx.FromAddress()
			if err != nil {
				return err
			}
			from := state[fromAddr]
			// Nonce must increase sequentially per account
			if from.Nonce+1 != tx.Nonce {
				return fmt.Errorf("bad nonce for %s: expected %d got %d", fromAddr, from.Nonce+1, tx.Nonce)
			}
			// Overflow check for totalDebit
			if tx.Amount > math.MaxUint64-tx.Fee {
				return errors.New("transaction amount + fee overflow")
			}
			totalDebit := tx.Amount + tx.Fee
			if from.Balance < totalDebit {
				return fmt.Errorf("insufficient funds for %s", fromAddr)
			}
			from.Balance -= totalDebit
			from.Nonce = tx.Nonce
			state[fromAddr] = from

			to := state[tx.ToAddress]
			// Overflow check: ensure balance + amount doesn't overflow
			if to.Balance > math.MaxUint64-tx.Amount {
				return errors.New("transfer balance overflow")
			}
			to.Balance += tx.Amount
			state[tx.ToAddress] = to
		default:
			return fmt.Errorf("unknown tx type: %q", tx.Type)
		}
	}
	return nil
}

// Chain represents the blockchain with thread-safe access
// Production-grade: implements full chain management with proper concurrency control
// Fork support: stores alternative blocks at same height for automatic reorganization
type Chain struct {
	mu sync.RWMutex

	// Chain metadata
	chainID          uint64
	minerAddress     string
	genesisAddress   string // Preset genesis miner address for 1% reward
	genesisTimestamp int64  // Genesis block timestamp for contract address generation
	consensus        ConsensusParams
	monetaryPolicy   MonetaryPolicy
	rulesHash        [32]byte

	// Chain state
	blocks        []*Block           // Canonical chain
	blocksByHash  map[string]*Block  // All blocks (canonical + orphans)
	state         map[string]Account // Current state
	bestTipHash   string             // Hash of best tip
	canonicalWork *big.Int           // Total work on canonical chain

	// Fork management - store alternative blocks at same height
	// Key: height, Value: list of blocks at that height (including canonical)
	forkBlocks map[uint64][]*Block

	// Fork block hash index for O(1) duplicate detection
	// Maps hash to block for fork blocks only (not canonical)
	forkBlocksByHash map[string]*Block

	// Cumulative work cache: block hash hex -> cumulative work
	// Avoids O(N) recalculation for frequently accessed fork blocks
	workCache map[string]*big.Int

	// Orphan pool - store blocks whose parent is not yet known
	orphanPool       map[string]*Block    // hash -> block
	orphanByParent   map[string][]string  // parent hash -> list of orphan hashes
	orphanTimestamps map[string]time.Time // hash -> insertion time for TTL cleanup
	orphanOrder      []string             // insertion order for LRU eviction

	// Indexes
	txIndex          map[string]TxLocation       // txid -> location (canonical only)
	addressIndex     map[string][]AddressTxEntry // address -> transfer history (in-memory)
	addressIndexBolt *index.AddressIndex         // address -> transactions (BoltDB persistent)
	indexPath        string                      // path to index database

	// Storage
	store ChainStore

	// Event publishing
	events EventSink

	// References for coordination
	peerBlockchain *peerRef

	// Block added callback - called when block is added to canonical chain
	// Used for broadcasting blocks added via API (e.g., from mining pool)
	onBlockAdded func(*Block)
	onBlockMu    sync.RWMutex

	// Missing block callback - called when orphan block is received
	// Used to request missing parent blocks from peers
	onMissingBlock func(parentHash []byte, height uint64)
	onMissingMu    sync.RWMutex

	// pendingAncestorRequests tracks in-flight parent fetches to prevent
	// request storms when batch orphans share the same missing parent.
	pendingAncestorRequests map[string]time.Time

	// skipLogCooldown suppresses duplicate "already on canonical chain" log spam
	skipLogCooldown   time.Time
	skipLogCooldownMu sync.Mutex

	// Context for background goroutines (orphan cleanup, etc.)
	ctx    context.Context
	cancel context.CancelFunc

	// Reorg protection - prevent template generation during reorg
	reorgInProgress   bool
	reorgMu           sync.Mutex
	lastReorgTime     time.Time
	lastAutoReorgTime time.Time

	// Contract management
	contractManager *ContractManager

	// Mempool reference for cleanup on block confirmation
	// Production-grade: enables automatic removal of confirmed transactions
	// Thread-safety: MempoolCleaner implementation must be thread-safe
	mempool MempoolCleaner

	// Sync notifier - called after chain reorganization to trigger sync check
	// CRITICAL: Enables sync loop to re-evaluate chain state after fork rollback
	syncNotifier SyncNotifier

	// Fork resolved callback - called after rollback completes with new height and rolled back count
	// CRITICAL: Triggers immediate re-sync via BlockKeeper.forkResolvedCh
	// Thread-safety: callback is invoked under mutex lock, must not block
	onForkResolved func(newHeight uint64, rolledBack uint64)

	// External reorg state checker removed - heaviest chain rule is deterministic
	// No external coordination needed for fork resolution

	// UNIFIED FORK RESOLUTION: ReorgExecutor for centralized reorg management
	// When set, all reorg operations from Chain will delegate to this executor
	// This ensures global mutex (TryLock) and preventive timing (500ms/2s/1s) are applied
	// CRITICAL: Must be set during node initialization via SetReorgExecutor()
	reorgExecutor ReorgExecutor

	// Shared PoW engine for mining and verification.
	// Single engine instance guarantees DAG cache, matrix pool, and
	// diffAdjuster state are identical between mining and validation paths.
	powEngine     *nogopow.NogopowEngine
	powEngineOnce sync.Once

	// Difficulty cache for block template API
	// Prevents PI controller recalculation on every template poll
	// Only recalculates when parent block height changes (once per block)
	diffCacheHeight uint64
	diffCacheValue  uint32
	diffCacheMu     sync.Mutex

	// Checkpoint voter for multi-sig checkpoint consensus
	checkpointVoter   *CheckpointVoter
	checkpointVoteMu  sync.RWMutex
	onCheckpointBlock func(height uint64, blockHash string, vote *CheckpointVote) // callback to broadcast vote
}

// ReorgExecutor interface for unified fork resolution
// Implementations: network/forkresolution.ForkResolver (via adapter)
type ReorgExecutor interface {
	RequestReorg(block *Block, source string) error
	IsReorgInProgress() bool
}

// ChainConfig holds chain configuration
// Production-grade: all parameters configurable via environment/config
type ChainConfig struct {
	ChainID      uint64
	MinerAddress string
	Store        ChainStore
	GenesisPath  string
	IndexPath    string // path to address index database
}
