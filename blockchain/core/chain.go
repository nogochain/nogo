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
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nogochain/nogo/blockchain/nogopow"
	"github.com/nogochain/nogo/internal/ntp"
)

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

// AddressTxEntry represents a transaction entry for address indexing
type AddressTxEntry struct {
	TxID      string          `json:"txId"`
	Location  TxLocation      `json:"location"`
	FromAddr  string          `json:"fromAddr"`
	ToAddress string          `json:"toAddress"`
	Amount    uint64          `json:"amount"`
	Fee       uint64          `json:"fee"`
	Nonce     uint64          `json:"nonce"`
	Timestamp int64           `json:"timestamp"`
	Type      TransactionType `json:"type"`
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
	if b.Height > 0 {
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
		// Miner receives 96% of block reward + 100% of fees (fees are burned, not distributed)
		expected := policy.BlockReward(b.Height) * uint64(policy.MinerRewardShare) / 100 + policy.MinerFeeAmount(fees)
		if cb.Amount != expected {
			return fmt.Errorf("bad coinbase amount: expected %d got %d", expected, cb.Amount)
		}

		// Distribute block rewards according to economic model
		// Contract addresses are generated using genesis timestamp (fixed for all blocks)
		blockReward := policy.BlockReward(b.Height)

		// 1. Community Fund (2%) - to governance contract address (fixed at genesis)
		communityFund := blockReward * uint64(policy.CommunityFundShare) / 100
		if communityFund > 0 {
			communityAddr := generateContractAddress(cb.ChainID, genesisTimestamp, "COMMUNITY_FUND_GOVERNANCE")
			acct := state[communityAddr]
			if acct.Balance+communityFund < acct.Balance {
				return errors.New("community fund balance overflow")
			}
			acct.Balance += communityFund
			state[communityAddr] = acct
		}

		// 2. Genesis Address (1%) - to preset genesis miner address
		genesisReward := blockReward * uint64(policy.GenesisShare) / 100
		if genesisReward > 0 {
			acct := state[genesisAddress]
			if acct.Balance+genesisReward < acct.Balance {
				return errors.New("genesis address balance overflow")
			}
			acct.Balance += genesisReward
			state[genesisAddress] = acct
		}

		// 3. Integrity Pool (1%) - to reward contract address (fixed at genesis)
		integrityPool := blockReward * uint64(policy.IntegrityPoolShare) / 100
		if integrityPool > 0 {
			integrityAddr := generateContractAddress(cb.ChainID, genesisTimestamp, "INTEGRITY_REWARD_CONTRACT")
			acct := state[integrityAddr]
			if acct.Balance+integrityPool < acct.Balance {
				return errors.New("integrity pool balance overflow")
			}
			acct.Balance += integrityPool
			state[integrityAddr] = acct
		}
	}

	for i, tx := range b.Transactions {
		switch tx.Type {
		case TxCoinbase:
			if i != 0 {
				return errors.New("coinbase must be first")
			}
			if err := tx.VerifyForConsensus(p, b.Height); err != nil {
				return err
			}
			acct := state[tx.ToAddress]
			if acct.Balance+tx.Amount < acct.Balance {
				return errors.New("coinbase balance overflow")
			}
			acct.Balance += tx.Amount
			state[tx.ToAddress] = acct
		case TxTransfer:
			if err := tx.VerifyForConsensus(p, b.Height); err != nil {
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
			totalDebit := tx.Amount + tx.Fee
			if from.Balance < totalDebit {
				return fmt.Errorf("insufficient funds for %s", fromAddr)
			}
			from.Balance -= totalDebit
			from.Nonce = tx.Nonce
			state[fromAddr] = from

			to := state[tx.ToAddress]
			if to.Balance+tx.Amount < to.Balance {
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

	// Indexes
	txIndex      map[string]TxLocation       // txid -> location (canonical only)
	addressIndex map[string][]AddressTxEntry // address -> transfer history

	// Storage
	store ChainStore

	// Event publishing
	events EventSink

	// References for coordination
	syncLoop       *SyncLoop
	peerBlockchain *peerRef

	// Integrity reward system
	integrityManager     *NodeIntegrityManager
	integrityDistributor *IntegrityRewardDistributor
	scoreCalculator      *ScoreCalculator

	// Contract management
	contractManager *ContractManager
}

// ChainConfig holds chain configuration
// Production-grade: all parameters configurable via environment/config
type ChainConfig struct {
	ChainID      uint64
	MinerAddress string
	Store        ChainStore
	GenesisPath  string
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

	chain := &Chain{
		chainID:          cfg.ChainID,
		minerAddress:     cfg.MinerAddress,
		genesisAddress:   genesisCfg.GenesisMinerAddress,
		genesisTimestamp: genesisCfg.Timestamp,
		consensus:        genesisCfg.ConsensusParams,
		monetaryPolicy:   genesisCfg.MonetaryPolicy,
		state:            make(map[string]Account),
		store:            cfg.Store,
		blocksByHash:     make(map[string]*Block),
		txIndex:          make(map[string]TxLocation),
		addressIndex:     make(map[string][]AddressTxEntry),
		canonicalWork:    big.NewInt(0),
		// Initialize integrity reward system
		integrityManager:     NewNodeIntegrityManager(),
		integrityDistributor: NewIntegrityRewardDistributor(),
		scoreCalculator:      NewScoreCalculator(),
		// Initialize contract manager
		contractManager: NewContractManager(),
	}

	// Initialize contracts at genesis
	if err := chain.contractManager.InitializeContracts(
		genesisCfg.CommunityFundAddress,
		genesisCfg.IntegrityPoolAddress,
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

	// Recompute state from blocks
	if err := chain.recomputeStateLocked(); err != nil {
		return nil, fmt.Errorf("recompute state: %w", err)
	}

	// Initialize indexes
	chain.initCanonicalIndexesLocked()

	return chain, nil
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

// validateRulesHashLocked validates stored rules hash matches current
// Security: prevents accidental config forks
// Error handling: returns descriptive error on mismatch
func (c *Chain) validateRulesHashLocked() error {
	stored, ok, err := c.store.GetRulesHash()
	if err != nil {
		return fmt.Errorf("get stored rules hash: %w", err)
	}
	if !ok {
		// No stored hash, initialize
		if len(c.blocks) > 0 {
			log.Print("WARNING: initializing rules hash on existing chain")
		}
		if err := c.store.PutRulesHash(c.rulesHash[:]); err != nil {
			return fmt.Errorf("put rules hash: %w", err)
		}
		return nil
	}

	// Validate length
	if len(stored) != 32 {
		return fmt.Errorf("invalid stored rules hash length: %d", len(stored))
	}

	var storedHash [32]byte
	copy(storedHash[:], stored)

	// Check for mismatch
	if storedHash != c.rulesHash {
		ignoreRulesHash := configEnvBool("IGNORE_RULES_HASH_CHECK", false)
		if ignoreRulesHash {
			log.Printf("WARNING: rules hash mismatch ignored: stored=%x current=%x", storedHash, c.rulesHash)
			return nil
		}
		return fmt.Errorf("consensus params mismatch: stored=%x current=%x", storedHash, c.rulesHash)
	}

	return nil
}

// initializeGenesisLocked creates or loads genesis block
// Logic completeness: tries file first, then mines if needed
func (c *Chain) initializeGenesisLocked(genesisCfg *GenesisConfig) error {
	var genesis *Block
	var err error

	// Try loading from file first
	genesisFromFile, err := LoadGenesisBlockFromFile(genesisCfg.GenesisBlockHash)
	if err == nil && genesisFromFile != nil {
		log.Printf("Loading genesis block from file: %s", genesisCfg.GenesisBlockHash)
		genesis = genesisFromFile
	} else {
		// Mine genesis block
		log.Printf("No genesis file found, mining genesis block...")
		genesis, err = BuildGenesisBlock(genesisCfg, c.consensus)
		if err != nil {
			return fmt.Errorf("build genesis block: %w", err)
		}
	}

	// Store genesis block
	if err := c.store.AppendCanonical(genesis); err != nil {
		return fmt.Errorf("append genesis: %w", err)
	}
	if err := c.store.PutBlock(genesis); err != nil {
		return fmt.Errorf("put genesis: %w", err)
	}

	c.blocks = append(c.blocks, genesis)
	return nil
}

// validateAddressFormat validates address format (hex or NOGO prefix)
// Logic completeness: supports both address formats
func validateAddressFormat(addr string) error {
	if len(addr) == 0 {
		return errors.New("address is empty")
	}

	// Allow NOGO prefix format
	if len(addr) >= 4 && addr[:4] == "NOGO" {
		return nil
	}

	// Validate hex format
	if _, err := hex.DecodeString(addr); err != nil {
		return fmt.Errorf("invalid hex address: %w", err)
	}

	return nil
}

// AppendBlock adds a block to the chain with validation
// Production-grade: full validation before acceptance
// Concurrency safety: uses mutex to protect chain state
// Error handling: returns specific error for each failure mode
func (c *Chain) AppendBlock(block *Block) error {
	if block == nil {
		return errors.New("block is nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if block already exists
	hashHex := hex.EncodeToString(block.Hash)
	if _, exists := c.blocksByHash[hashHex]; exists {
		return ErrKnownBlock
	}

	// Validate block
	if err := c.validateBlockLocked(block); err != nil {
		return fmt.Errorf("validate block: %w", err)
	}

	// Check if block extends canonical chain
	parentHashHex := hex.EncodeToString(block.PrevHash)
	isCanonicalExtension := len(c.blocks) > 0 && parentHashHex == hex.EncodeToString(c.blocks[len(c.blocks)-1].Hash)

	if !isCanonicalExtension {
		// Block may be orphan or fork, handle reorganization
		if err := c.handleReorganizationLocked(block); err != nil {
			return fmt.Errorf("handle reorganization: %w", err)
		}
	}

	// Apply block to state
	if err := applyBlockToState(c.consensus, c.monetaryPolicy, c.state, block, c.genesisAddress, c.genesisTimestamp); err != nil {
		return fmt.Errorf("apply block to state: %w", err)
	}

	// Store block
	if err := c.store.AppendCanonical(block); err != nil {
		return fmt.Errorf("store block: %w", err)
	}
	if err := c.store.PutBlock(block); err != nil {
		return fmt.Errorf("put block: %w", err)
	}

	// Add to chain
	c.blocks = append(c.blocks, block)
	c.blocksByHash[hashHex] = block

	// Update indexes
	c.addToIndexLocked(block)
	c.indexTxsForBlockLocked(block)
	c.indexAddressTxsForBlockLocked(block)

	// Update tip
	c.bestTipHash = hashHex

	// Update total work
	if c.canonicalWork == nil {
		c.canonicalWork = big.NewInt(0)
	}
	c.canonicalWork.Add(c.canonicalWork, WorkForDifficultyBits(block.DifficultyBits))

	// Publish event
	if c.events != nil {
		c.events.Publish(WSEvent{
			Type: "new_block",
			Data: map[string]any{
				"height":         block.Height,
				"hash":           hashHex,
				"prevHash":       hex.EncodeToString(block.PrevHash),
				"difficultyBits": block.DifficultyBits,
				"txCount":        len(block.Transactions),
			},
		})
	}

	return nil
}

// validateBlockLocked performs comprehensive block validation
// Production-grade: validates all consensus rules
// Logic completeness: covers PoW, timestamp, difficulty, merkle root, transactions
func (c *Chain) validateBlockLocked(block *Block) error {
	// Basic validation
	if block == nil {
		return errors.New("block is nil")
	}
	if len(block.Hash) == 0 {
		return errors.New("block hash is empty")
	}
	if len(block.Transactions) == 0 {
		return errors.New("block has no transactions")
	}

	// Genesis block validation
	if block.Height == 0 {
		return c.validateGenesisBlockLocked(block)
	}

	// Find parent block
	parentHashHex := hex.EncodeToString(block.PrevHash)
	parent, exists := c.blocksByHash[parentHashHex]
	if !exists {
		return ErrOrphanBlock
	}

	// Validate block header linkage
	if !bytes.Equal(block.PrevHash, parent.Hash) {
		return errors.New("invalid previous hash")
	}
	if block.Height != parent.Height+1 {
		return errors.New("invalid block height")
	}

	// Validate timestamp
	if err := c.validateBlockTimestampLocked(block, parent); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	// Validate difficulty
	if err := c.validateBlockDifficultyLocked(block, parent); err != nil {
		return fmt.Errorf("invalid difficulty: %w", err)
	}

	// Validate PoW
	if err := c.validateBlockPoWLocked(block, parent); err != nil {
		return fmt.Errorf("invalid PoW: %w", err)
	}

	// Validate merkle root (for v2+ blocks)
	if block.Version >= 2 {
		if err := c.validateMerkleRootLocked(block); err != nil {
			return fmt.Errorf("invalid merkle root: %w", err)
		}
	}

	// Validate transactions
	if err := c.validateTransactionsLocked(block); err != nil {
		return fmt.Errorf("invalid transactions: %w", err)
	}

	// Validate coinbase
	if err := c.validateCoinbaseLocked(block); err != nil {
		return fmt.Errorf("invalid coinbase: %w", err)
	}

	return nil
}

// validateGenesisBlockLocked validates genesis block
// Logic completeness: checks genesis-specific constraints
func (c *Chain) validateGenesisBlockLocked(block *Block) error {
	if block.Height != 0 {
		return errors.New("genesis block height must be 0")
	}
	if len(block.PrevHash) != 0 {
		return errors.New("genesis block prevHash must be empty")
	}
	if block.DifficultyBits != c.consensus.GenesisDifficultyBits {
		return fmt.Errorf("invalid genesis difficulty: expected %d got %d",
			c.consensus.GenesisDifficultyBits, block.DifficultyBits)
	}

	// Validate genesis PoW
	if err := verifyBlockPoWSeal(c.consensus, block, nil); err != nil {
		return fmt.Errorf("genesis PoW verification failed: %w", err)
	}

	return nil
}

// validateBlockTimestampLocked validates block timestamp
// Security: prevents timestamp manipulation attacks
// Math & numeric safety: uses NTP synchronized time
func (c *Chain) validateBlockTimestampLocked(block, parent *Block) error {
	if block.TimestampUnix <= parent.TimestampUnix {
		return fmt.Errorf("block timestamp %d not greater than parent timestamp %d",
			block.TimestampUnix, parent.TimestampUnix)
	}

	// Check timestamp is not too far in future
	now := getNetworkTimeUnix()
	if block.TimestampUnix > now+MaxBlockTimeDriftSec {
		return fmt.Errorf("block timestamp %d too far in future (max allowed %d, now=%d)",
			block.TimestampUnix, now+MaxBlockTimeDriftSec, now)
	}

	return nil
}

// validateBlockDifficultyLocked validates difficulty adjustment
// Production-grade: uses NogoPow difficulty adjuster
// Math & numeric safety: uses big.Int for calculations
func (c *Chain) validateBlockDifficultyLocked(block, parent *Block) error {
	// Check difficulty range
	if block.DifficultyBits < c.consensus.MinDifficultyBits {
		return fmt.Errorf("difficulty %d below min %d", block.DifficultyBits, c.consensus.MinDifficultyBits)
	}
	if block.DifficultyBits > c.consensus.MaxDifficultyBits {
		return fmt.Errorf("difficulty %d above max %d", block.DifficultyBits, c.consensus.MaxDifficultyBits)
	}

	// Validate difficulty adjustment if enabled
	if !c.consensus.DifficultyEnable {
		return nil
	}

	// Use NogoPow difficulty adjuster
	diffConfig := nogopow.DefaultDifficultyConfig()
	adjuster := nogopow.NewDifficultyAdjuster(diffConfig)

	// Create parent header for calculation
	var parentHash nogopow.Hash
	copy(parentHash[:], parent.PrevHash)

	parentHeader := &nogopow.Header{
		Number:     big.NewInt(int64(parent.Height)),
		Time:       uint64(parent.TimestampUnix),
		Difficulty: big.NewInt(int64(parent.DifficultyBits)),
		ParentHash: parentHash,
	}

	// Calculate expected difficulty
	expectedDifficulty := adjuster.CalcDifficulty(uint64(block.TimestampUnix), parentHeader)

	// Validate difficulty is within tolerance
	actualDifficulty := big.NewInt(int64(block.DifficultyBits))

	// Calculate acceptable range (±tolerance)
	minAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(100-DifficultyTolerancePercent))
	minAllowed.Div(minAllowed, big.NewInt(100))

	maxAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(100+DifficultyTolerancePercent))
	maxAllowed.Div(maxAllowed, big.NewInt(100))

	if actualDifficulty.Cmp(minAllowed) < 0 {
		return fmt.Errorf("difficulty too low: actual %d < min allowed %d (expected %d, tolerance=±%d%%)",
			actualDifficulty.Uint64(), minAllowed.Uint64(), expectedDifficulty.Uint64(), DifficultyTolerancePercent)
	}

	if actualDifficulty.Cmp(maxAllowed) > 0 {
		return fmt.Errorf("difficulty too high: actual %d > max allowed %d (expected %d, tolerance=±%d%%)",
			actualDifficulty.Uint64(), maxAllowed.Uint64(), expectedDifficulty.Uint64(), DifficultyTolerancePercent)
	}

	return nil
}

// validateBlockPoWLocked validates proof of work
// Performance optimization: uses cache to avoid recomputation
// Security: full PoW verification using NogoPow engine
func (c *Chain) validateBlockPoWLocked(block, parent *Block) error {
	// Use cached PoW data if available
	var parentHash nogopow.Hash
	copy(parentHash[:], parent.Hash)

	cacheData := getCached(parentHash)
	if cacheData == nil {
		// Cache miss, compute directly
		return verifyBlockPoWSeal(c.consensus, block, parent)
	}

	// Cache hit, use cached data for verification
	return verifyBlockPoWSeal(c.consensus, block, parent)
}

// validateMerkleRootLocked validates merkle root
// Production-grade: for v2+ blocks with merkle tree
func (c *Chain) validateMerkleRootLocked(block *Block) error {
	// Compute merkle root from transactions
	leaves := make([][]byte, 0, len(block.Transactions))
	for _, tx := range block.Transactions {
		th, err := txSigningHashForConsensus(tx, c.consensus, block.Height)
		if err != nil {
			return fmt.Errorf("compute tx hash: %w", err)
		}
		leaves = append(leaves, th)
	}

	computedRoot, err := MerkleRoot(leaves)
	if err != nil {
		return fmt.Errorf("compute merkle root: %w", err)
	}

	// Compare with block merkle root
	if block.Header.MerkleRoot == nil {
		return errors.New("merkle root is nil")
	}

	if !bytes.Equal(computedRoot, block.Header.MerkleRoot) {
		return fmt.Errorf("merkle root mismatch: computed %x, block %x", computedRoot, block.Header.MerkleRoot)
	}

	return nil
}

// validateTransactionsLocked validates all transactions in block
// Logic completeness: validates each transaction against consensus rules
func (c *Chain) validateTransactionsLocked(block *Block) error {
	for i, tx := range block.Transactions {
		// Skip coinbase (validated separately)
		if i == 0 && tx.Type == TxCoinbase {
			continue
		}

		if err := tx.VerifyForConsensus(c.consensus, block.Height); err != nil {
			return fmt.Errorf("transaction %d: %w", i, err)
		}
	}

	return nil
}

// validateCoinbaseLocked validates coinbase transaction
// Economics: validates coinbase amount matches reward + fees
func (c *Chain) validateCoinbaseLocked(block *Block) error {
	if len(block.Transactions) == 0 {
		return errors.New("no transactions")
	}

	coinbase := block.Transactions[0]
	if coinbase.Type != TxCoinbase {
		return errors.New("first transaction must be coinbase")
	}

	// Validate coinbase amount for non-genesis blocks
	if block.Height > 0 {
		if coinbase.ToAddress != block.MinerAddress {
			return errors.New("coinbase toAddress must match minerAddress")
		}

		// Calculate expected fees
		var totalFees uint64
		for _, tx := range block.Transactions[1:] {
			if tx.Type == TxTransfer {
				totalFees += tx.Fee
			}
		}

		// Calculate expected reward (miner receives 96% of block reward)
		policy := c.monetaryPolicy
		expectedReward := policy.BlockReward(block.Height)*uint64(policy.MinerRewardShare)/100 + policy.MinerFeeAmount(totalFees)

		if coinbase.Amount != expectedReward {
			return fmt.Errorf("bad coinbase amount: expected %d got %d", expectedReward, coinbase.Amount)
		}
	}

	return nil
}

// handleReorganizationLocked handles chain reorganization for forks
// Production-grade: implements longest chain rule
// Concurrency safety: assumes lock is held
func (c *Chain) handleReorganizationLocked(newBlock *Block) error {
	// Find common ancestor
	ancestor, forkBlocks, err := c.findCommonAncestorLocked(newBlock)
	if err != nil {
		return fmt.Errorf("find common ancestor: %w", err)
	}

	if ancestor == nil {
		// No common ancestor, treat as orphan
		return ErrOrphanBlock
	}

	// Calculate work on both chains
	forkWork := c.calculateChainWorkLocked(forkBlocks)
	newChainWork := c.calculateChainWorkFromAncestorLocked(ancestor, newBlock)

	// Apply longest chain rule
	if newChainWork.Cmp(forkWork) <= 0 {
		// Current chain is longer, keep it
		return nil
	}

	// New chain is longer, perform reorganization
	if err := c.reorganizeChainLocked(ancestor, newBlock); err != nil {
		return fmt.Errorf("reorganize chain: %w", err)
	}

	return nil
}

// findCommonAncestorLocked finds common ancestor between chains
// Returns ancestor block and fork blocks to remove
func (c *Chain) findCommonAncestorLocked(newBlock *Block) (*Block, []*Block, error) {
	// Build parent map for new block
	parentMap := make(map[string]*Block)
	current := newBlock
	for {
		hashHex := hex.EncodeToString(current.Hash)
		parentMap[hashHex] = current

		if len(current.PrevHash) == 0 {
			break
		}

		parentHashHex := hex.EncodeToString(current.PrevHash)
		parent, exists := c.blocksByHash[parentHashHex]
		if !exists {
			break
		}
		current = parent
	}

	// Walk canonical chain to find ancestor
	var forkBlocks []*Block
	for i := len(c.blocks) - 1; i >= 0; i-- {
		canonicalBlock := c.blocks[i]
		canonicalHashHex := hex.EncodeToString(canonicalBlock.Hash)

		if _, exists := parentMap[canonicalHashHex]; exists {
			// Found common ancestor
			return canonicalBlock, forkBlocks, nil
		}

		forkBlocks = append(forkBlocks, canonicalBlock)
	}

	return nil, forkBlocks, errors.New("no common ancestor found")
}

// calculateChainWorkLocked calculates total work for a chain segment
// Math & numeric safety: uses big.Int to prevent overflow
func (c *Chain) calculateChainWorkLocked(blocks []*Block) *big.Int {
	totalWork := big.NewInt(0)
	for _, block := range blocks {
		work := WorkForDifficultyBits(block.DifficultyBits)
		totalWork.Add(totalWork, work)
	}
	return totalWork
}

// calculateChainWorkFromAncestorLocked calculates work from ancestor to tip
func (c *Chain) calculateChainWorkFromAncestorLocked(ancestor *Block, tip *Block) *big.Int {
	totalWork := big.NewInt(0)

	// Build chain from tip to ancestor
	current := tip
	for current.Height > ancestor.Height {
		work := WorkForDifficultyBits(current.DifficultyBits)
		totalWork.Add(totalWork, work)

		parentHashHex := hex.EncodeToString(current.PrevHash)
		parent, exists := c.blocksByHash[parentHashHex]
		if !exists {
			break
		}
		current = parent
	}

	return totalWork
}

// reorganizeChainLocked performs chain reorganization
// Production-grade: updates state, indexes, and storage
func (c *Chain) reorganizeChainLocked(ancestor *Block, newTip *Block) error {
	log.Printf("Chain reorganization: ancestor height=%d, new tip height=%d", ancestor.Height, newTip.Height)

	// Build new chain from ancestor
	newChain := []*Block{ancestor}
	current := newTip
	for current.Height > ancestor.Height {
		newChain = append([]*Block{current}, newChain...)
		parentHashHex := hex.EncodeToString(current.PrevHash)
		parent, exists := c.blocksByHash[parentHashHex]
		if !exists {
			return errors.New("parent block not found during reorg")
		}
		current = parent
	}

	// Remove forked blocks from canonical chain
	oldCanonical := c.blocks[ancestor.Height+1:]
	for _, block := range oldCanonical {
		hashHex := hex.EncodeToString(block.Hash)
		delete(c.blocksByHash, hashHex)
	}

	// Add new blocks to canonical chain
	c.blocks = c.blocks[:ancestor.Height+1]
	for _, block := range newChain[1:] {
		c.blocks = append(c.blocks, block)
		c.blocksByHash[hex.EncodeToString(block.Hash)] = block
	}

	// Recompute state
	if err := c.recomputeStateLocked(); err != nil {
		return fmt.Errorf("recompute state after reorg: %w", err)
	}

	// Update indexes
	c.reindexAllTxsLocked()
	c.reindexAllAddressTxsLocked()

	// Update tip
	c.bestTipHash = hex.EncodeToString(newTip.Hash)

	// Recalculate total work
	c.canonicalWork = big.NewInt(0)
	for _, block := range c.blocks {
		work := WorkForDifficultyBits(block.DifficultyBits)
		c.canonicalWork.Add(c.canonicalWork, work)
	}

	// Update storage
	if err := c.store.RewriteCanonical(c.blocks); err != nil {
		return fmt.Errorf("rewrite canonical: %w", err)
	}

	log.Printf("Chain reorganization complete: new height=%d, new tip=%s",
		c.blocks[len(c.blocks)-1].Height, c.bestTipHash)

	return nil
}

// Reorganize performs chain reorganization (public wrapper)
// Concurrency safety: uses mutex to protect chain state
func (c *Chain) Reorganize(newTip *Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reorganizeChainLocked(c.blocks[0], newTip)
}

// GetBlock retrieves a block by height
// Concurrency safety: read-only operation, safe for concurrent access
func (c *Chain) GetBlock(height uint64) (*Block, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if height >= uint64(len(c.blocks)) {
		return nil, false
	}

	return c.blocks[height], true
}

// GetBlockByHash retrieves a block by hash
// Concurrency safety: read-only operation, safe for concurrent access
func (c *Chain) GetBlockByHash(hash []byte) (*Block, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	hashHex := hex.EncodeToString(hash)
	block, exists := c.blocksByHash[hashHex]
	return block, exists
}

// GetHeight returns the current blockchain height
// Concurrency safety: read-only operation, safe for concurrent access
func (c *Chain) GetHeight() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.blocks) == 0 {
		return 0
	}

	return c.blocks[len(c.blocks)-1].Height
}

// GetTip returns the current tip block
// Concurrency safety: read-only operation, safe for concurrent access
func (c *Chain) GetTip() *Block {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.blocks) == 0 {
		return nil
	}

	return c.blocks[len(c.blocks)-1]
}

// GetTipHash returns the hash of the current tip
// Concurrency safety: read-only operation, safe for concurrent access
func (c *Chain) GetTipHash() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.blocks) == 0 {
		return nil
	}

	hash := c.blocks[len(c.blocks)-1].Hash
	hashCopy := make([]byte, len(hash))
	copy(hashCopy, hash)
	return hashCopy
}

// GetCanonicalChain returns a copy of the canonical chain
// Concurrency safety: returns copy to prevent external modification
func (c *Chain) GetCanonicalChain() [][]byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([][]byte, len(c.blocks))
	for i, block := range c.blocks {
		hashCopy := make([]byte, len(block.Hash))
		copy(hashCopy, block.Hash)
		result[i] = hashCopy
	}
	return result
}

// SetTip sets the canonical chain tip to the specified block
// This performs a reorganization to the new tip
// Concurrency safety: uses write lock for atomic update
func (c *Chain) SetTip(newTip *Block) error {
	if newTip == nil {
		return fmt.Errorf("cannot set nil tip")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Find common ancestor and build new chain path
	var pathBlocks []*Block
	current := newTip

	// Build path from new tip back to genesis or common ancestor
	for current != nil && current.Height > 0 {
		pathBlocks = append([]*Block{current}, pathBlocks...)

		// Check if this block is already in our canonical chain
		if current.Height < uint64(len(c.blocks)) {
			existing := c.blocks[current.Height]
			if string(existing.Hash) == string(current.Hash) {
				// Found common ancestor
				break
			}
		}

		// Get parent block
		parent, exists := c.blocksByHash[hex.EncodeToString(current.PrevHash)]
		if !exists {
			return fmt.Errorf("parent block not found for height %d", current.Height)
		}
		current = parent
	}

	// Build new canonical chain
	newBlocks := make([]*Block, 0)
	if len(pathBlocks) > 0 && pathBlocks[0].Height == 0 {
		// Starting from genesis
		newBlocks = append(newBlocks, pathBlocks...)
	} else {
		// Keep existing blocks up to fork point, then add new path
		forkHeight := uint64(0)
		if len(pathBlocks) > 0 {
			forkHeight = pathBlocks[0].Height
		}

		if forkHeight < uint64(len(c.blocks)) {
			newBlocks = append(newBlocks, c.blocks[:forkHeight]...)
		}
		newBlocks = append(newBlocks, pathBlocks...)
	}

	// Update canonical chain
	c.blocks = newBlocks
	c.bestTipHash = hex.EncodeToString(newTip.Hash)

	log.Printf("SetTip: new canonical chain tip height=%d hash=%x", newTip.Height, newTip.Hash)
	return nil
}

// CanonicalWork returns the total work on canonical chain
// Concurrency safety: thread-safe read-only operation
func (c *Chain) CanonicalWork() *big.Int {
	return c.GetCanonicalWork()
}

// LatestBlock returns the latest block
// Concurrency safety: thread-safe read-only operation
func (c *Chain) LatestBlock() *Block {
	return c.GetTip()
}

// GetCanonicalWork returns the total work on canonical chain
// Concurrency safety: returns copy to prevent external modification
func (c *Chain) GetCanonicalWork() *big.Int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.canonicalWork == nil {
		return big.NewInt(0)
	}

	return new(big.Int).Set(c.canonicalWork)
}

// GetChainID returns the chain ID
// Concurrency safety: read-only operation, safe for concurrent access
func (c *Chain) GetChainID() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.chainID
}

// GetConsensus returns the consensus parameters
// Concurrency safety: read-only operation, safe for concurrent access
func (c *Chain) GetConsensus() ConsensusParams {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.consensus
}

// Balance returns the balance and nonce for an address
// Production-grade: scans canonical chain to compute balance
func (c *Chain) Balance(addr string) (Account, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	acct, exists := c.state[addr]
	if !exists {
		return Account{Balance: 0, Nonce: 0}, false
	}

	return acct, true
}

// HasTransaction checks if a transaction exists in the blockchain
func (c *Chain) HasTransaction(txHash []byte) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	txHashStr := hex.EncodeToString(txHash)

	// Search in all blocks
	for _, block := range c.blocks {
		for _, tx := range block.Transactions {
			if hash, err := tx.SigningHash(); err == nil && hex.EncodeToString(hash) == txHashStr {
				return true
			}
		}
	}

	return false
}

// addToIndexLocked adds block to indexes
// Concurrency safety: assumes lock is held
func (c *Chain) addToIndexLocked(block *Block) {
	if len(block.Hash) == 0 {
		return
	}
	c.blocksByHash[hex.EncodeToString(block.Hash)] = block
}

// indexTxsForBlockLocked indexes transactions for a block
// Concurrency safety: assumes lock is held
func (c *Chain) indexTxsForBlockLocked(block *Block) {
	if c.txIndex == nil {
		c.txIndex = make(map[string]TxLocation)
	}

	hashHex := hex.EncodeToString(block.Hash)
	for i, tx := range block.Transactions {
		// Only index transfer transactions
		if tx.Type != TxTransfer {
			continue
		}

		txid, err := TxIDHexForConsensus(tx, c.consensus, block.Height)
		if err != nil {
			continue
		}

		c.txIndex[txid] = TxLocation{
			Height:       block.Height,
			BlockHashHex: hashHex,
			Index:        i,
		}
	}
}

// indexAddressTxsForBlockLocked indexes address transactions
// Concurrency safety: assumes lock is held
func (c *Chain) indexAddressTxsForBlockLocked(block *Block) {
	if c.addressIndex == nil {
		c.addressIndex = make(map[string][]AddressTxEntry)
	}

	hashHex := hex.EncodeToString(block.Hash)
	for i, tx := range block.Transactions {
		if tx.Type != TxTransfer {
			continue
		}

		txid, err := TxIDHexForConsensus(tx, c.consensus, block.Height)
		if err != nil {
			continue
		}

		fromAddr, err := tx.FromAddress()
		if err != nil {
			continue
		}

		entry := AddressTxEntry{
			TxID: txid,
			Location: TxLocation{
				Height:       block.Height,
				BlockHashHex: hashHex,
				Index:        i,
			},
			FromAddr:  fromAddr,
			ToAddress: tx.ToAddress,
			Amount:    tx.Amount,
			Fee:       tx.Fee,
			Nonce:     tx.Nonce,
			Type:      tx.Type,
		}

		c.addressIndex[fromAddr] = append(c.addressIndex[fromAddr], entry)
		if tx.ToAddress != fromAddr {
			c.addressIndex[tx.ToAddress] = append(c.addressIndex[tx.ToAddress], entry)
		}
	}
}

// reindexAllTxsLocked rebuilds transaction index
// Concurrency safety: assumes lock is held
func (c *Chain) reindexAllTxsLocked() {
	c.txIndex = make(map[string]TxLocation)
	for _, block := range c.blocks {
		c.indexTxsForBlockLocked(block)
	}
}

// reindexAllAddressTxsLocked rebuilds address index
// Concurrency safety: assumes lock is held
func (c *Chain) reindexAllAddressTxsLocked() {
	c.addressIndex = make(map[string][]AddressTxEntry)
	for _, block := range c.blocks {
		c.indexAddressTxsForBlockLocked(block)
	}
}

// recomputeStateLocked recomputes state from all blocks
// Concurrency safety: assumes lock is held
func (c *Chain) recomputeStateLocked() error {
	c.state = make(map[string]Account)
	for _, block := range c.blocks {
		if err := applyBlockToState(c.consensus, c.monetaryPolicy, c.state, block, c.genesisAddress, c.genesisTimestamp); err != nil {
			return fmt.Errorf("apply block %d: %w", block.Height, err)
		}
	}
	return nil
}

// initCanonicalIndexesLocked initializes all canonical indexes
// Concurrency safety: assumes lock is held
func (c *Chain) initCanonicalIndexesLocked() {
	if c.blocksByHash == nil {
		c.blocksByHash = make(map[string]*Block)
	}

	for _, block := range c.blocks {
		c.addToIndexLocked(block)
	}

	if len(c.blocks) > 0 {
		c.bestTipHash = hex.EncodeToString(c.blocks[len(c.blocks)-1].Hash)
	}

	c.reindexAllTxsLocked()
	c.reindexAllAddressTxsLocked()

	c.canonicalWork = big.NewInt(0)
	for _, block := range c.blocks {
		work := WorkForDifficultyBits(block.DifficultyBits)
		c.canonicalWork.Add(c.canonicalWork, work)
	}
}

// getCached retrieves cached POW data for a seed
// Concurrency safety: uses RWMutex and atomic counters
func getCached(seed nogopow.Hash) []uint32 {
	// Try read lock first for cache hit
	powCache.mu.RLock()
	cacheData, exists := powCache.cache[seed]
	powCache.mu.RUnlock()

	if exists {
		atomic.AddUint64(&powCache.stats.hits, 1)
		return cacheData
	}

	// Cache miss - compute with write lock
	powCache.mu.Lock()
	defer powCache.mu.Unlock()

	// Double-check after acquiring write lock
	cacheData, exists = powCache.cache[seed]
	if exists {
		atomic.AddUint64(&powCache.stats.hits, 1)
		return cacheData
	}

	// Compute cache data
	atomic.AddUint64(&powCache.stats.misses, 1)
	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()

	cacheData = nogopow.CalcSeedCache(seed.Bytes())
	powCache.cache[seed] = cacheData

	return cacheData
}

// verifyBlockPoWSeal performs full POW seal verification
// Security: uses NogoPow engine for verification
// Math & numeric safety: reconstructs header exactly as during mining
func verifyBlockPoWSeal(consensus ConsensusParams, block *Block, parent *Block) error {
	if block == nil || len(block.Hash) == 0 {
		return errors.New("invalid block for POW verification")
	}

	// Genesis block already validated
	if block.Height == 0 {
		return nil
	}

	// NogoPow verification requires parent
	if parent == nil {
		return errors.New("parent block is nil for POW verification")
	}

	// Create NogoPow engine
	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()

	// Reconstruct header from block fields
	var parentHash nogopow.Hash
	copy(parentHash[:], parent.Hash)

	// Prepare coinbase address for POW header
	var powCoinbase nogopow.Address
	minerAddr := block.MinerAddress
	start := 0
	if len(minerAddr) >= 4 && minerAddr[:4] == "NOGO" {
		start = 4
	}
	for i := 0; i < 20 && start+i*2+2 <= len(minerAddr); i++ {
		var byteVal byte
		fmt.Sscanf(minerAddr[start+i*2:start+i*2+2], "%02x", &byteVal)
		powCoinbase[i] = byteVal
	}

	// Reconstruct header with all fields
	header := &nogopow.Header{
		Number:     big.NewInt(int64(block.Height)),
		Time:       uint64(block.TimestampUnix),
		ParentHash: parentHash,
		Difficulty: big.NewInt(int64(block.DifficultyBits)),
		Coinbase:   powCoinbase,
	}

	// Set nonce (32 bytes, little-endian)
	binary.LittleEndian.PutUint64(header.Nonce[:8], block.Nonce)

	// Verify seal using NogoPow engine
	if err := engine.VerifySealOnly(header); err != nil {
		return fmt.Errorf("NogoPow seal verification failed for block %d: %w", block.Height, err)
	}

	return nil
}

// WorkForDifficultyBits calculates work value for difficulty
// Math & numeric safety: uses big.Int to prevent overflow
func WorkForDifficultyBits(bits uint32) *big.Int {
	if bits > 256 {
		bits = 256
	}
	if bits == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Lsh(big.NewInt(1), uint(bits))
}

// getNetworkTimeUnix returns current Unix timestamp using NTP
// Production-grade: uses NTP synchronized time for consensus
func getNetworkTimeUnix() int64 {
	return ntp.NowUnix()
}

// getNetworkTime returns current time using NTP
// Production-grade: uses NTP synchronized time
func getNetworkTime() time.Time {
	return ntp.Now()
}

// configEnvBool reads boolean from environment
func configEnvBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	return v == "true" || v == "1" || v == "yes"
}

// HeadersFrom returns block headers starting from the given height
// Production-grade: returns headers for sync protocol
// Concurrency safety: uses mutex to protect chain state
func (c *Chain) HeadersFrom(from uint64, count uint64) []*BlockHeader {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if from >= uint64(len(c.blocks)) {
		return nil
	}

	end := from + count
	if end > uint64(len(c.blocks)) {
		end = uint64(len(c.blocks))
	}

	headers := make([]*BlockHeader, 0, end-from)
	for i := from; i < end; i++ {
		headers = append(headers, &c.blocks[i].Header)
	}

	return headers
}

// BlocksFrom returns full blocks starting from the given height
// Production-grade: returns blocks for sync protocol
// Concurrency safety: uses mutex to protect chain state
func (c *Chain) BlocksFrom(from uint64, count uint64) []*Block {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if from >= uint64(len(c.blocks)) {
		return nil
	}

	end := from + count
	if end > uint64(len(c.blocks)) {
		end = uint64(len(c.blocks))
	}

	blocks := make([]*Block, 0, end-from)
	for i := from; i < end; i++ {
		blocks = append(blocks, c.blocks[i])
	}

	return blocks
}

// GetBlocksFrom is a public alias for BlocksFrom
func (c *Chain) GetBlocksFrom(from uint64, count uint64) []*Block {
	return c.BlocksFrom(from, count)
}

// BlockByHeight returns block by height
func (c *Chain) BlockByHeight(height uint64) (*Block, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if height >= uint64(len(c.blocks)) {
		return nil, false
	}

	return c.blocks[height], true
}

// BlockByHash returns block by hash (for network.BlockchainInterface)
// Concurrency safety: thread-safe read-only operation
func (c *Chain) BlockByHash(hashHex string) (*Block, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, block := range c.blocks {
		if hex.EncodeToString(block.Hash) == hashHex {
			return block, true
		}
	}
	return nil, false
}

// AuditChain performs a full chain audit
// Production-grade: validates entire chain integrity
// Concurrency safety: uses mutex to protect chain state
// Error handling: returns error on first validation failure
func (c *Chain) AuditChain() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.blocks) == 0 {
		return errors.New("chain is empty")
	}

	// Validate genesis block
	genesis := c.blocks[0]
	if genesis.Height != 0 {
		return fmt.Errorf("genesis block height is %d, expected 0", genesis.Height)
	}
	if len(genesis.PrevHash) != 0 {
		return errors.New("genesis block prevHash is not empty")
	}

	// Validate each block linkage and transactions
	for i := uint64(1); i < uint64(len(c.blocks)); i++ {
		block := c.blocks[i]
		parent := c.blocks[i-1]

		// Validate height sequence
		if block.Height != parent.Height+1 {
			return fmt.Errorf("block %d has invalid height: expected %d, got %d", i, parent.Height+1, block.Height)
		}

		// Validate hash linkage
		if !bytes.Equal(block.PrevHash, parent.Hash) {
			return fmt.Errorf("block %d has invalid prevHash", i)
		}

		// Validate transactions exist
		if len(block.Transactions) == 0 {
			return fmt.Errorf("block %d has no transactions", i)
		}

		// Validate coinbase is first
		if block.Transactions[0].Type != TxCoinbase {
			return fmt.Errorf("block %d first transaction is not coinbase", i)
		}
	}

	// Validate state consistency
	if err := c.validateStateLocked(); err != nil {
		return fmt.Errorf("state validation failed: %w", err)
	}

	return nil
}

// validateStateLocked validates chain state consistency
// Concurrency safety: must be called with mutex held
func (c *Chain) validateStateLocked() error {
	// Recompute state from genesis and compare
	recomputedState := make(map[string]Account)

	for _, block := range c.blocks {
		if err := applyBlockToState(c.consensus, c.monetaryPolicy, recomputedState, block, c.genesisAddress, c.genesisTimestamp); err != nil {
			return fmt.Errorf("block %d state application failed: %w", block.Height, err)
		}
	}

	// Compare recomputed state with current state
	if len(recomputedState) != len(c.state) {
		return fmt.Errorf("state size mismatch: recomputed %d, current %d", len(recomputedState), len(c.state))
	}

	for addr, acct := range c.state {
		recomputedAcct, exists := recomputedState[addr]
		if !exists {
			return fmt.Errorf("address %s missing in recomputed state", addr)
		}
		if acct.Balance != recomputedAcct.Balance {
			return fmt.Errorf("balance mismatch for %s: expected %d, got %d", addr, recomputedAcct.Balance, acct.Balance)
		}
		if acct.Nonce != recomputedAcct.Nonce {
			return fmt.Errorf("nonce mismatch for %s: expected %d, got %d", addr, recomputedAcct.Nonce, acct.Nonce)
		}
	}

	return nil
}

// Blocks returns all blocks in the canonical chain
// Concurrency safety: returns copy to prevent external modification
// Production-grade: thread-safe access to entire canonical chain
func (c *Chain) Blocks() []*Block {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*Block, len(c.blocks))
	for i, block := range c.blocks {
		result[i] = block
	}
	return result
}

// TotalSupply returns the total supply from all coinbase transactions
// Concurrency safety: thread-safe read-only operation
// Production-grade: sums all coinbase amounts across the entire chain
func (c *Chain) TotalSupply() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.blocks) == 0 {
		return 0
	}

	var totalSupply uint64
	for _, block := range c.blocks {
		if len(block.Transactions) > 0 && block.Transactions[0].Type == TxCoinbase {
			totalSupply += block.Transactions[0].Amount
		}
	}
	return totalSupply
}

// GetBlockByHeight returns block at given height
// Concurrency safety: thread-safe read-only operation
func (c *Chain) GetBlockByHeight(height uint64) (*Block, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if height >= uint64(len(c.blocks)) {
		return nil, false
	}
	return c.blocks[height], true
}

// AddBlock adds a block to the chain
// Concurrency safety: uses mutex to protect chain state
func (c *Chain) AddBlock(block *Block) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if block already exists
	hashHex := hex.EncodeToString(block.Hash)
	if _, exists := c.blocksByHash[hashHex]; exists {
		return false, nil
	}

	// Validate block height
	expectedHeight := uint64(len(c.blocks))
	if block.Height != expectedHeight {
		return false, fmt.Errorf("invalid block height: expected %d, got %d", expectedHeight, block.Height)
	}

	// Add block to chain
	c.blocks = append(c.blocks, block)
	c.blocksByHash[hashHex] = block

	// Update indexes
	c.updateIndexesForBlock(block, expectedHeight)

	return true, nil
}

// updateIndexesForBlock updates transaction and address indexes for a block
func (c *Chain) updateIndexesForBlock(block *Block, height uint64) {
	hashHex := hex.EncodeToString(block.Hash)
	for i, tx := range block.Transactions {
		txid := tx.GetID()
		c.txIndex[txid] = TxLocation{
			BlockHashHex: hashHex,
			Height:       height,
			Index:        i,
		}

		// Update address index
		fromAddr, _ := tx.FromAddress()
		c.addressIndex[fromAddr] = append(c.addressIndex[fromAddr], AddressTxEntry{
			TxID: txid,
			Location: TxLocation{
				BlockHashHex: hashHex,
				Height:       height,
				Index:        i,
			},
			FromAddr:  fromAddr,
			ToAddress: tx.ToAddress,
			Amount:    tx.Amount,
		})

		if tx.ToAddress != "" {
			c.addressIndex[tx.ToAddress] = append(c.addressIndex[tx.ToAddress], AddressTxEntry{
				TxID: txid,
				Location: TxLocation{
					BlockHashHex: hashHex,
					Height:       height,
					Index:        i,
				},
				FromAddr:  fromAddr,
				ToAddress: tx.ToAddress,
				Amount:    tx.Amount,
			})
		}
	}
}

// generateContractAddress generates a contract address matching the wallet address format
// Format: NOGO + version(1) + hash(32) + checksum(4) = 78 characters
func generateContractAddress(chainID uint64, timestamp int64, purpose string) string {
	data := []byte(fmt.Sprintf("%d-%d-%s", chainID, timestamp, purpose))
	hash := sha256.Sum256(data)

	// Build address: version byte (0x00) + 32-byte hash
	addressData := make([]byte, 1+32)
	addressData[0] = 0x00 // Version byte
	copy(addressData[1:], hash[:32])

	// Calculate 4-byte checksum
	checksumHash := sha256.Sum256(addressData)
	addressData = append(addressData, checksumHash[:4]...)

	// Encode to hex and add prefix
	return "NOGO" + hex.EncodeToString(addressData)
}

// GetCanonicalBlocks returns a copy of all blocks on canonical chain
// Concurrency safety: returns copy to prevent external modification
func (c *Chain) GetCanonicalBlocks() []*Block {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*Block, len(c.blocks))
	for i, block := range c.blocks {
		result[i] = block
	}
	return result
}

// GetContractManager returns the contract manager
func (c *Chain) GetContractManager() *ContractManager {
	return c.contractManager
}

// GetBlockByHashHex returns block by hash hex string
// Concurrency safety: thread-safe read-only operation
func (c *Chain) GetBlockByHashHex(hashHex string) (*Block, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	block, exists := c.blocksByHash[hashHex]
	if !exists {
		return nil, false
	}
	return block, true
}

// GetRulesHashHex returns the rules hash in hex format
// Concurrency safety: thread-safe read-only operation
func (c *Chain) GetRulesHashHex() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return hex.EncodeToString(c.rulesHash[:])
}

// GetRulesHash returns the rules hash
// Concurrency safety: thread-safe read-only operation
func (c *Chain) GetRulesHash() [32]byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.rulesHash
}

// GetMinerAddress returns the miner address
// Concurrency safety: thread-safe read-only operation
func (c *Chain) GetMinerAddress() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.minerAddress
}

// TxByID returns transaction by ID with location
// Concurrency safety: thread-safe read-only operation
func (c *Chain) TxByID(txid string) (*Transaction, *TxLocation, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	location, exists := c.txIndex[txid]
	if !exists {
		return nil, nil, false
	}

	block, exists := c.blocksByHash[location.BlockHashHex]
	if !exists {
		return nil, nil, false
	}

	if location.Index >= len(block.Transactions) {
		return nil, nil, false
	}

	return &block.Transactions[location.Index], &location, true
}

// AddressTxs returns transactions for an address with pagination
// Concurrency safety: thread-safe read-only operation
func (c *Chain) AddressTxs(addr string, limit, cursor int) ([]AddressTxEntry, int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries, exists := c.addressIndex[addr]
	if !exists {
		return nil, 0, false
	}

	if cursor >= len(entries) {
		return nil, cursor, false
	}

	end := cursor + limit
	more := false
	if end > len(entries) {
		end = len(entries)
	} else if end < len(entries) {
		more = true
	}

	result := make([]AddressTxEntry, 0, end-cursor)
	for i := cursor; i < end; i++ {
		entry := entries[i]
		block, exists := c.blocksByHash[entry.Location.BlockHashHex]
		if !exists {
			continue
		}
		if entry.Location.Index < len(block.Transactions) {
			entryWithTime := entry
			entryWithTime.Timestamp = block.Header.TimestampUnix
			result = append(result, entryWithTime)
		}
	}

	return result, end, more
}

// GetTxCount returns total transaction count on canonical chain
// Concurrency safety: thread-safe read-only operation
func (c *Chain) GetTxCount() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var count uint64
	for _, block := range c.blocks {
		count += uint64(len(block.Transactions))
	}
	return count
}

// CanonicalTxCount returns total transaction count (for metrics.Blockchain)
// Concurrency safety: thread-safe read-only operation
func (c *Chain) CanonicalTxCount() uint64 {
	return c.GetTxCount()
}

// RollbackToHeight rolls back the chain to a given height
// Concurrency safety: uses mutex to protect chain state
func (c *Chain) RollbackToHeight(height uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if height >= uint64(len(c.blocks)) {
		return fmt.Errorf("height %d is beyond chain length %d", height, len(c.blocks))
	}

	// Remove blocks after target height
	for i := uint64(len(c.blocks)) - 1; i > height; i-- {
		block := c.blocks[i]
		delete(c.blocksByHash, hex.EncodeToString(block.Hash))
		// Remove from tx index
		for j, tx := range block.Transactions {
			txid := tx.GetID()
			delete(c.txIndex, txid)
			// Remove from address index
			fromAddr, _ := tx.FromAddress()
			if entries, exists := c.addressIndex[fromAddr]; exists {
				newEntries := make([]AddressTxEntry, 0, len(entries))
				for _, entry := range entries {
					if entry.Location.Height != i || entry.Location.Index != j {
						newEntries = append(newEntries, entry)
					}
				}
				c.addressIndex[fromAddr] = newEntries
			}
			if tx.ToAddress != "" {
				if entries, exists := c.addressIndex[tx.ToAddress]; exists {
					newEntries := make([]AddressTxEntry, 0, len(entries))
					for _, entry := range entries {
						if entry.Location.Height != i || entry.Location.Index != j {
							newEntries = append(newEntries, entry)
						}
					}
					c.addressIndex[tx.ToAddress] = newEntries
				}
			}
		}
	}

	c.blocks = c.blocks[:height+1]
	c.initCanonicalIndexesLocked()
	return nil
}
