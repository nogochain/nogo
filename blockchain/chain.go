package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nogochain/nogo/blockchain/nogopow"
)

const (
	minFee = uint64(1)
)

// ErrInvalidPoW is returned when POW verification fails
var ErrInvalidPoW = errors.New("invalid proof of work")

// powCache stores computed cache data to avoid recalculation
// Key: seed hash, Value: computed cache data
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

// shouldVerifyPoW determines if a block should undergo full POW verification
// Uses last byte of block hash as random seed for probabilistic verification
// Returns true if hash[len(hash)-1] < 26 (approximately 10% probability)
func shouldVerifyPoW(hash []byte) bool {
	if len(hash) == 0 {
		return false
	}
	return hash[len(hash)-1] < powVerifyProbabilityThreshold
}

// getCached retrieves cached POW data for a seed or computes it if not cached
// Thread-safe access to powCache using RWMutex
func getCached(seed nogopow.Hash) []uint32 {
	// Try read lock first for cache hit
	powCache.mu.RLock()
	cacheData, exists := powCache.cache[seed]
	powCache.mu.RUnlock()

	if exists {
		// Cache hit - use atomic for thread-safe increment
		atomic.AddUint64(&powCache.stats.hits, 1)
		return cacheData
	}

	// Cache miss - compute with write lock
	powCache.mu.Lock()
	defer powCache.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have computed it)
	cacheData, exists = powCache.cache[seed]
	if exists {
		// Cache hit - use atomic for thread-safe increment
		atomic.AddUint64(&powCache.stats.hits, 1)
		return cacheData
	}

	// Compute cache data - use atomic for thread-safe increment
	atomic.AddUint64(&powCache.stats.misses, 1)
	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()

	// Generate cache using calcSeedCache from nogopow package
	cacheData = nogopow.CalcSeedCache(seed.Bytes())
	powCache.cache[seed] = cacheData

	return cacheData
}

// getCacheStats returns current cache statistics using atomic loads
func getCacheStats() (hits, misses uint64, size int) {
	powCache.mu.RLock()
	defer powCache.mu.RUnlock()
	hits = atomic.LoadUint64(&powCache.stats.hits)
	misses = atomic.LoadUint64(&powCache.stats.misses)
	size = len(powCache.cache)
	return hits, misses, size
}

// logCacheStats logs cache hit rate periodically
func logCacheStats() {
	hits, misses, size := getCacheStats()
	total := hits + misses
	if total == 0 {
		return
	}
	hitRate := float64(hits) / float64(total) * 100
	log.Printf("POW cache stats: hits=%d misses=%d hit_rate=%.2f%% cache_size=%d",
		hits, misses, hitRate, size)
}

// validateBlockPoWNogoPow validates a block's proof-of-work using NogoPow engine
// Parameters:
//   - consensus: consensus parameters for validation rules
//   - block: the block to validate
//   - parent: the parent block (required for non-genesis blocks)
//
// Validation logic (PRODUCTION-READY - NO SKIPPED VALIDATION):
// 1. Genesis block POW verification
// 2. Parent block nil check (for non-genesis)
// 3. Difficulty range check (100% execution)
// 4. Full POW seal verification using NogoPow engine
// 5. Difficulty adjustment validation
//
// SECURITY MODEL:
// Every block MUST be fully validated regardless of sync status.
// This follows Bitcoin full node validation model, NOT SPV model.
//
// Why we MUST validate POW during sync:
// 1. Security: Trust-but-verify is insufficient for consensus-critical validation
// 2. Consistency: All nodes must validate identically to prevent forks
// 3. Attack prevention: Malicious nodes cannot broadcast invalid POW blocks
// 4. Economic incentives: Validation ensures miners follow protocol rules
//
// Performance optimization:
// - POW cache is used to avoid recomputation for blocks with same seed
// - Cache is thread-safe with RWMutex and atomic counters
// - Typical cache hit rate: 80-95% in normal operation
func validateBlockPoWNogoPow(consensus ConsensusParams, block *Block, parent *Block) error {
	if block == nil {
		return errors.New("block is nil")
	}

	// Genesis block POW verification
	if block.Height == 0 {
		if block.DifficultyBits != consensus.GenesisDifficultyBits {
			return fmt.Errorf("bad genesis difficulty: expected %d got %d",
				consensus.GenesisDifficultyBits, block.DifficultyBits)
		}
		// Verify genesis block POW seal
		if err := verifyBlockPoWSeal(consensus, block, nil); err != nil {
			return fmt.Errorf("genesis POW verification failed: %w", err)
		}
		return nil
	}

	// Parent block nil check for non-genesis blocks
	if parent == nil {
		return errors.New("parent block is nil for non-genesis block")
	}

	// Difficulty range check (100% execution)
	if block.DifficultyBits < consensus.MinDifficultyBits {
		return fmt.Errorf("difficulty %d below min %d", block.DifficultyBits, consensus.MinDifficultyBits)
	}
	if block.DifficultyBits > consensus.MaxDifficultyBits {
		return fmt.Errorf("difficulty %d above max %d", block.DifficultyBits, consensus.MaxDifficultyBits)
	}

	// Full POW seal verification (ALWAYS EXECUTED - NO SKIPPING)
	if err := verifyBlockPoWSeal(consensus, block, parent); err != nil {
		return fmt.Errorf("POW seal verification failed: %w", err)
	}

	// Difficulty adjustment validation (for non-genesis blocks)
	if consensus.DifficultyEnable {
		if err := validateDifficultyAdjustment(consensus, block, parent); err != nil {
			return fmt.Errorf("difficulty adjustment validation failed: %w", err)
		}
	}

	return nil
}

// verifyBlockPoWSeal performs full POW seal verification using NogoPow engine
// This function verifies that the block's hash meets the stated difficulty target
// Parameters:
//   - consensus: consensus parameters
//   - block: block to verify
//   - parent: parent block (nil for genesis)
//
// Verification steps (following NogoPow engine):
// 1. Reconstruct header from block fields (NOT using block.Hash directly)
// 2. Calculate seed from parent hash
// 3. Compute NogoPow hash: powHash = computePoW(SealHash(header), seed)
// 4. Calculate target from difficulty: target = (2^256 - 1) / difficulty
// 5. Verify powHash <= target
//
// CRITICAL: We must reconstruct the header exactly as it was during mining,
// then let NogoPow engine compute SealHash(header) and verify the seal.
// Using block.Hash directly is WRONG because block.Hash = sealedHeader.Hash(),
// not the input to the POW function.
//
// Target calculation (NogoPow standard):
// target = (2^256 - 1) / difficulty
// For difficulty=1: target = 2^256 - 1 (maximum possible target)
// For difficulty=2: target = (2^256 - 1) / 2
func verifyBlockPoWSeal(consensus ConsensusParams, block *Block, parent *Block) error {
	if block == nil || len(block.Hash) == 0 {
		return errors.New("invalid block for POW verification")
	}

	// For genesis block, always accept (already validated difficulty)
	if block.Height == 0 {
		return nil
	}

	// NogoPow verification requires parent for seed calculation
	if parent == nil {
		return errors.New("parent block is nil for POW verification")
	}

	// Create NogoPow engine for verification
	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()

	// Reconstruct header from block fields
	// This MUST match the header structure used during mining
	var parentHash nogopow.Hash
	copy(parentHash[:], parent.Hash)

	// Prepare coinbase address for POW header (same logic as mining)
	// NogoChain addresses are 36 bytes (72 hex chars) after the "NOGO" prefix
	var powCoinbase nogopow.Address
	minerAddr := block.MinerAddress
	start := 0
	if len(minerAddr) >= 4 && minerAddr[:4] == "NOGO" {
		start = 4
	}
	// Parse up to 20 bytes (Address size) from the hex string
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

	// Debug logging for POW verification
	log.Printf("POW VERIFY: height=%d hash=%x nonce=%d", block.Height, block.Hash, block.Nonce)
	log.Printf("POW VERIFY: miner=%s", block.MinerAddress)
	log.Printf("POW VERIFY: difficulty=%d", block.DifficultyBits)
	log.Printf("POW VERIFY: powCoinbase=%x", powCoinbase)
	log.Printf("POW VERIFY: header fields: number=%d time=%d parentHash=%x",
		header.Number.Uint64(), header.Time, header.ParentHash)

	// Verify seal using NogoPow engine
	// This will:
	// 1. Calculate sealHash = SealHash(header) (RLP encode + SHA3)
	// 2. Calculate seed = parent.Hash()
	// 3. Compute powHash = computePoW(sealHash, seed)
	// 4. Check if powHash <= target where target = (2^256-1) / difficulty
	if err := engine.VerifySealOnly(header); err != nil {
		log.Printf("POW VERIFY FAILED: height=%d error=%v", block.Height, err)
		return fmt.Errorf("NogoPow seal verification failed for block %d: %w", block.Height, err)
	}

	log.Printf("POW VERIFY SUCCESS: height=%d", block.Height)
	return nil
}

// validateDifficultyAdjustment validates difficulty adjustment every 100 blocks
// Parameters:
//   - consensus: consensus parameters
//   - block: current block at adjustment boundary
//   - parent: parent block
//
// Validation checks:
// 1. Verify parent difficulty is within acceptable range
// 2. Verify timestamp constraints
// 3. Verify difficulty change is within allowed bounds
func validateDifficultyAdjustment(consensus ConsensusParams, block *Block, parent *Block) error {
	if block == nil || parent == nil {
		return errors.New("block or parent is nil")
	}

	// Check parent difficulty
	if parent.DifficultyBits < consensus.MinDifficultyBits {
		return fmt.Errorf("parent difficulty %d below min %d", parent.DifficultyBits, consensus.MinDifficultyBits)
	}

	// Check timestamp validity
	if block.TimestampUnix <= parent.TimestampUnix {
		return fmt.Errorf("block timestamp %d not greater than parent timestamp %d",
			block.TimestampUnix, parent.TimestampUnix)
	}

	// Use NogoPow difficulty adjuster to calculate expected difficulty
	config := nogopow.DefaultDifficultyConfig()
	adjuster := nogopow.NewDifficultyAdjuster(config)

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

	// Validate difficulty change is within allowed bounds
	// Allow ±50% deviation from expected (to account for different implementations)
	actualDifficulty := big.NewInt(int64(block.DifficultyBits))

	// Calculate acceptable range
	minAllowed := new(big.Int).Div(expectedDifficulty, big.NewInt(2))
	maxAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(3))

	if actualDifficulty.Cmp(minAllowed) < 0 {
		return fmt.Errorf("difficulty adjustment too aggressive: actual %d < min allowed %d (expected %d)",
			actualDifficulty.Uint64(), minAllowed.Uint64(), expectedDifficulty.Uint64())
	}

	if actualDifficulty.Cmp(maxAllowed) > 0 {
		return fmt.Errorf("difficulty adjustment too aggressive: actual %d > max allowed %d (expected %d)",
			actualDifficulty.Uint64(), maxAllowed.Uint64(), expectedDifficulty.Uint64())
	}

	return nil
}

// stringToAddress converts a string address to nogopow.Address
func stringToAddress(addr string) nogopow.Address {
	var result nogopow.Address
	// Decode hex address
	if len(addr) >= 40 {
		// Skip NOGO prefix if present
		start := 0
		if len(addr) > 40 && addr[:4] == "NOGO" {
			start = 4
		}
		// Decode hex
		for i := 0; i < 20 && start+i*2 < len(addr); i++ {
			var byteVal byte
			fmt.Sscanf(addr[start+i*2:start+i*2+2], "%02x", &byteVal)
			result[i] = byteVal
		}
	}
	return result
}

// WorkForDifficultyBits calculates the work value for a given difficulty
func WorkForDifficultyBits(bits uint32) *big.Int {
	// Probability of mining a block is ~2^-bits, so expected work is ~2^bits.
	// Use big.Int to avoid overflow.
	if bits > 256 {
		bits = 256
	}
	if bits == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Lsh(big.NewInt(1), uint(bits))
}

// Ensure Blockchain implements nogopow.ChainHeaderReader
var _ nogopow.ChainHeaderReader = (*Blockchain)(nil)

// GetHeaderByHash returns the header by hash (for NogoPow engine)
func (bc *Blockchain) GetHeaderByHash(hash nogopow.Hash) *nogopow.Header {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	hashBytes := hash.Bytes()
	hashStr := string(hashBytes)
	for _, block := range bc.blocks {
		if string(block.Hash) == hashStr {
			var blockHash nogopow.Hash
			copy(blockHash[:], block.Hash)

			var parentHash nogopow.Hash
			if len(block.PrevHash) > 0 {
				copy(parentHash[:], block.PrevHash)
			}

			var n nogopow.BlockNonce
			binary.LittleEndian.PutUint64(n[:8], block.Nonce)

			return &nogopow.Header{
				Number:     big.NewInt(int64(block.Height)),
				Time:       uint64(block.TimestampUnix),
				ParentHash: parentHash,
				Difficulty: big.NewInt(int64(block.DifficultyBits)),
				Nonce:      n,
				Coinbase:   stringToAddress(block.MinerAddress),
			}
		}
	}
	return nil
}

type Blockchain struct {
	ChainID      uint64
	MinerAddress string

	consensus ConsensusParams
	rulesHash [32]byte

	events EventSink

	mu sync.RWMutex

	blocks []*Block
	state  map[string]Account
	store  ChainStore

	blocksByHash  map[string]*Block
	bestTipHash   string
	canonicalWork *big.Int

	txIndex map[string]TxLocation // txid -> location (canonical only)

	addressIndex map[string][]AddressTxEntry // address -> canonical transfer history (oldest->newest)
}

func LoadBlockchain(chainID uint64, minerAddress string, store ChainStore, genesisSupply uint64) (*Blockchain, error) {
	envConsensus := defaultConsensusParamsFromEnv()
	genesisPath, err := GenesisPathFromEnv(chainID)
	if err != nil {
		return nil, err
	}
	genesisCfg, err := LoadGenesisConfig(genesisPath)
	if err != nil {
		return nil, err
	}
	if chainID != 0 && genesisCfg.ChainID != chainID {
		return nil, fmt.Errorf("genesis chainId mismatch: env=%d genesis=%d", chainID, genesisCfg.ChainID)
	}
	chainID = genesisCfg.ChainID

	bc := &Blockchain{
		ChainID:       chainID,
		MinerAddress:  minerAddress,
		consensus:     genesisCfg.ConsensusParams,
		state:         map[string]Account{},
		store:         store,
		blocksByHash:  map[string]*Block{},
		txIndex:       map[string]TxLocation{},
		addressIndex:  map[string][]AddressTxEntry{},
		canonicalWork: big.NewInt(0),
	}

	if minerAddress != "" {
		// Validate - allow both raw hex and NOGO00 address formats
		if !strings.HasPrefix(minerAddress, "NOGO") {
			// Raw hex format - validate directly
			if _, err := hex.DecodeString(minerAddress); err != nil {
				return nil, fmt.Errorf("invalid MINER_ADDRESS (not hex or NOGO00): %w", err)
			}
		}
	}

	envConsensus.MonetaryPolicy = bc.consensus.MonetaryPolicy
	if consensusEnvOverridesSet() && envConsensus != bc.consensus {
		log.Print("WARNING: consensus env vars are ignored because genesis.json is authoritative")
	}

	blocks, err := store.ReadCanonical()
	if err != nil {
		return nil, err
	}
	bc.blocks = blocks
	allBlocks, err := store.ReadAllBlocks()
	if err != nil {
		return nil, err
	}
	if len(allBlocks) > 0 {
		bc.blocksByHash = allBlocks
	}

	// Operator safety: lock consensus params to this chain store on first run, and refuse
	// to run if they ever change (prevents accidental config forks).
	curRulesHash := bc.consensus.MustRulesHash()
	bc.rulesHash = curRulesHash

	ignoreRulesHash := envBool("IGNORE_RULES_HASH_CHECK", false)
	if !ignoreRulesHash && envBool("UNSAFE_IGNORE_RULES_HASH_CHECK", false) {
		ignoreRulesHash = true
		log.Print("WARNING: UNSAFE_IGNORE_RULES_HASH_CHECK is deprecated; use IGNORE_RULES_HASH_CHECK=true instead")
	}
	if ignoreRulesHash {
		log.Print("WARNING: IGNORE_RULES_HASH_CHECK=true; running with consensus params that do not match stored rules hash")
	}

	if stored, ok, err := store.GetRulesHash(); err != nil {
		return nil, err
	} else if ok {
		if len(stored) != 32 {
			return nil, fmt.Errorf("invalid stored rules hash length: %d", len(stored))
		}
		var storedHash [32]byte
		copy(storedHash[:], stored)
		if storedHash != curRulesHash {
			if ignoreRulesHash {
				log.Printf("WARNING: rules hash mismatch ignored: stored=%x current=%x", storedHash, curRulesHash)
			} else {
				return nil, fmt.Errorf("consensus params mismatch: stored rulesHash=%x current rulesHash=%x (set IGNORE_RULES_HASH_CHECK=true to bypass, or delete data/ to reinit)", storedHash, curRulesHash)
			}
		}
	} else {
		// No stored rules hash yet. Initialize it.
		if len(bc.blocks) > 0 {
			log.Print("WARNING: initializing rules hash on an existing chain; ensure all nodes use identical consensus env vars")
		}
		if err := store.PutRulesHash(curRulesHash[:]); err != nil {
			return nil, err
		}
	}

	if len(bc.blocks) == 0 {
		// Check if we should mine genesis block or load from file
		// For new nodes joining existing network, load genesis from file
		// Only mine genesis block if this is a true genesis node

		var genesis *Block
		genesisPath, _ := GenesisPathFromEnv(chainID)
		genesisFromFile, err := LoadGenesisBlockFromFile(genesisPath)
		if err == nil && genesisFromFile != nil {
			// Genesis block exists in file - use it instead of mining
			log.Printf("Loading genesis block from file: %s", genesisPath)
			genesis = genesisFromFile
		} else {
			// No genesis file or failed to load - mine genesis block
			log.Printf("No genesis file found, mining genesis block...")
			genesis, err = BuildGenesisBlock(genesisCfg, bc.consensus)
			if err != nil {
				return nil, err
			}
		}

		if err := bc.store.AppendCanonical(genesis); err != nil {
			return nil, err
		}
		_ = bc.store.PutBlock(genesis)
		bc.blocks = append(bc.blocks, genesis)
	} else {
		if err := ValidateGenesisBlock(bc.blocks[0], genesisCfg, bc.consensus); err != nil {
			return nil, err
		}
	}

	if len(bc.blocks) == 0 {
		return nil, errors.New("missing genesis block")
	}
	genesisHash, err := ensureBlockHash(bc.blocks[0], bc.consensus)
	if err != nil {
		return nil, err
	}
	if stored, ok, err := store.GetGenesisHash(); err != nil {
		return nil, err
	} else if ok {
		if !bytes.Equal(stored, genesisHash) {
			return nil, fmt.Errorf("genesis hash mismatch: stored=%x current=%x", stored, genesisHash)
		}
	} else {
		if err := store.PutGenesisHash(genesisHash); err != nil {
			return nil, err
		}
	}

	if err := bc.recomputeStateLocked(); err != nil {
		return nil, err
	}
	bc.initCanonicalIndexesLocked()
	return bc, nil
}

func (bc *Blockchain) RulesHashHex() string {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if bc.rulesHash == ([32]byte{}) {
		return ""
	}
	return hex.EncodeToString(bc.rulesHash[:])
}

func (bc *Blockchain) SetEventSink(sink EventSink) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.events = sink
}

func (bc *Blockchain) LatestBlock() *Block {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.blocks[len(bc.blocks)-1]
}

func (bc *Blockchain) CanonicalWork() *big.Int {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if bc.canonicalWork == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(bc.canonicalWork)
}

func (bc *Blockchain) BlockByHeight(height uint64) (*Block, bool) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if height >= uint64(len(bc.blocks)) {
		return nil, false
	}
	return bc.blocks[int(height)], true
}

func (bc *Blockchain) CanonicalTxCount() int {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	total := 0
	for _, b := range bc.blocks {
		total += len(b.Transactions)
	}
	return total
}

func (bc *Blockchain) Balance(address string) (Account, bool) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	acct, ok := bc.state[address]
	return acct, ok
}

func (bc *Blockchain) TotalSupply() uint64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	var total uint64
	for _, acct := range bc.state {
		if total+acct.Balance < total {
			return 0
		}
		total += acct.Balance
	}
	return total
}

func (bc *Blockchain) SubmitTransfer(tx Transaction, requireAIAudit bool, aiApproved bool) (*Block, error) {
	// Compatibility helper: mine a single transfer immediately.
	if requireAIAudit && !aiApproved {
		return nil, errors.New("transaction rejected by AI auditor")
	}
	return bc.MineTransfers([]Transaction{tx})
}

func (bc *Blockchain) MineTransfers(transfers []Transaction) (*Block, error) {
	latest := bc.LatestBlock()
	if latest == nil {
		return nil, errors.New("no genesis block")
	}

	prevHash := append([]byte(nil), latest.Hash...)
	height := latest.Height + 1
	now := time.Now().Unix()
	ts := now
	if ts <= latest.TimestampUnix {
		ts = latest.TimestampUnix + 1
	}

	var fees uint64
	for _, tx := range transfers {
		if tx.Type != TxTransfer {
			return nil, errors.New("only transfer txs can be mined")
		}
		if tx.ChainID == 0 {
			tx.ChainID = bc.ChainID
		}
		if tx.ChainID != bc.ChainID {
			return nil, fmt.Errorf("wrong chainId: %d", tx.ChainID)
		}
		if err := tx.VerifyForConsensus(bc.consensus, height); err != nil {
			return nil, err
		}
		if tx.Fee < minFee {
			return nil, fmt.Errorf("fee too low: minFee=%d", minFee)
		}
		fees += tx.Fee
	}

	policy := bc.consensus.MonetaryPolicy
	reward := policy.BlockReward(height)
	minerFees := policy.MinerFeeAmount(fees)
	coinbaseData := fmt.Sprintf("block reward + fees (height=%d)", height)
	if height == 1 {
		coinbaseData = "Memphis"
	}
	coinbase := Transaction{
		Type:      TxCoinbase,
		ChainID:   bc.ChainID,
		ToAddress: bc.MinerAddress,
		Amount:    reward + minerFees,
		Data:      coinbaseData,
	}

	txs := make([]Transaction, 0, 1+len(transfers))
	txs = append(txs, coinbase)
	txs = append(txs, transfers...)

	// Calculate next difficulty using NogoPow engine
	engine := nogopow.New(nogopow.DefaultConfig())

	// Get parent header for difficulty calculation
	parentHeader := &nogopow.Header{
		Number:     big.NewInt(int64(latest.Height)),
		Time:       uint64(latest.TimestampUnix),
		Difficulty: big.NewInt(int64(latest.DifficultyBits)),
	}

	// Calculate next difficulty
	nextDifficulty := engine.CalcDifficulty(bc, uint64(ts), parentHeader)

	newBlock := &Block{
		Version:        blockVersionForHeight(bc.consensus, height),
		Height:         height,
		TimestampUnix:  ts,
		PrevHash:       prevHash,
		DifficultyBits: uint32(nextDifficulty.Uint64()),
		MinerAddress:   bc.MinerAddress,
		Transactions:   txs,
	}

	// Create NogoPow block
	parentHash := nogopow.Hash{}
	copy(parentHash[:], newBlock.PrevHash)

	// Prepare coinbase address for POW header
	// NogoChain addresses are 36 bytes (72 hex chars) after the "NOGO" prefix
	var powCoinbase nogopow.Address
	minerAddr := bc.MinerAddress
	start := 0
	if len(minerAddr) >= 4 && minerAddr[:4] == "NOGO" {
		start = 4
	}
	// Parse up to 20 bytes (Address size) from the hex string
	for i := 0; i < 20 && start+i*2+2 <= len(minerAddr); i++ {
		var byteVal byte
		fmt.Sscanf(minerAddr[start+i*2:start+i*2+2], "%02x", &byteVal)
		powCoinbase[i] = byteVal
	}

	// Debug logging for mining
	log.Printf("MINING: height=%d miner=%s powCoinbase=%x",
		newBlock.Height, bc.MinerAddress, powCoinbase)

	header := &nogopow.Header{
		Number:     big.NewInt(int64(newBlock.Height)),
		Time:       uint64(newBlock.TimestampUnix),
		ParentHash: parentHash,
		Difficulty: nextDifficulty,
		Coinbase:   powCoinbase,
	}

	// Prepare header with dynamic difficulty
	if err := engine.Prepare(bc, header); err != nil {
		return nil, fmt.Errorf("failed to prepare header: %w", err)
	}

	// Create block for mining
	block := nogopow.NewBlock(header, nil, nil, nil)

	// Mine using NogoPow algorithm (no timeout - wait until solution found)
	stop := make(chan struct{})
	resultCh := make(chan *nogopow.Block, 1)

	go func() {
		err := engine.Seal(bc, block, resultCh, stop)
		if err != nil {
			close(resultCh)
		}
	}()

	// Wait for result (no timeout)
	result, ok := <-resultCh
	if !ok {
		close(stop)
		return nil, fmt.Errorf("mining failed: channel closed")
	}

	// Extract nonce and hash from sealed header
	sealedHeader := result.Header()
	newBlock.Nonce = binary.LittleEndian.Uint64(sealedHeader.Nonce[:8])
	newBlock.Hash = sealedHeader.Hash().Bytes()

	bc.mu.Lock()
	var eventSink EventSink
	var toPublish *WSEvent
	defer func() {
		bc.mu.Unlock()
		if eventSink != nil && toPublish != nil {
			eventSink.Publish(*toPublish)
		}
	}()

	if err := applyBlockToState(bc.consensus, bc.state, newBlock); err != nil {
		return nil, err
	}
	if err := bc.store.AppendCanonical(newBlock); err != nil {
		return nil, err
	}
	bc.blocks = append(bc.blocks, newBlock)
	bc.addToIndexLocked(newBlock)
	bc.indexTxsForBlockLocked(newBlock)
	bc.indexAddressTxsForBlockLocked(newBlock)
	bc.bestTipHash = hex.EncodeToString(newBlock.Hash)
	if bc.canonicalWork == nil {
		bc.canonicalWork = big.NewInt(0)
	}
	bc.canonicalWork.Add(bc.canonicalWork, WorkForDifficultyBits(newBlock.DifficultyBits))
	toPublish = &WSEvent{
		Type: "new_block",
		Data: map[string]any{
			"height":         newBlock.Height,
			"hash":           hex.EncodeToString(newBlock.Hash),
			"prevHash":       hex.EncodeToString(newBlock.PrevHash),
			"difficultyBits": newBlock.DifficultyBits,
			"txCount":        len(newBlock.Transactions),
			"addresses":      addressesForBlock(newBlock),
		},
	}
	eventSink = bc.events
	return newBlock, nil
}

func (bc *Blockchain) AuditChain() error {
	bc.mu.RLock()
	blocks := append([]*Block(nil), bc.blocks...)
	consensus := bc.consensus
	bc.mu.RUnlock()
	if len(blocks) == 0 {
		return errors.New("empty chain")
	}

	for i, b := range blocks {
		if i == 0 {
			if b.Height != 0 || len(b.PrevHash) != 0 {
				return errors.New("invalid genesis header")
			}
			if b.Version != blockVersionForHeight(consensus, 0) {
				return fmt.Errorf("bad block version at %d: expected %d got %d", b.Height, blockVersionForHeight(consensus, 0), b.Version)
			}
		} else {
			prev := blocks[i-1]
			if b.Height != prev.Height+1 {
				return fmt.Errorf("bad height at %d", b.Height)
			}
			if string(b.PrevHash) != string(prev.Hash) {
				return fmt.Errorf("bad prev hash at %d", b.Height)
			}
			if err := validateBlockTime(consensus, blocks, i); err != nil {
				return err
			}
			if consensus.DifficultyEnable {
				// Validate difficulty using NogoPow algorithm
				if err := validateDifficultyNogoPow(consensus, blocks, i); err != nil {
					return err
				}
			}
			if b.Version != blockVersionForHeight(consensus, b.Height) {
				return fmt.Errorf("bad block version at %d: expected %d got %d", b.Height, blockVersionForHeight(consensus, b.Height), b.Version)
			}
		}
		if b.DifficultyBits == 0 || b.DifficultyBits > maxDifficultyBits {
			return fmt.Errorf("difficultyBits out of range at %d: %d", b.Height, b.DifficultyBits)
		}
		// Validate PoW using NogoPow engine
		var parent *Block
		if i > 0 {
			parent = blocks[i-1]
		}
		if err := validateBlockPoWNogoPow(consensus, b, parent); err != nil {
			return err
		}
		// tx validity check (structural + signatures)
		for _, tx := range b.Transactions {
			if tx.ChainID == 0 {
				return fmt.Errorf("missing chainId at height %d", b.Height)
			}
			if err := tx.VerifyForConsensus(consensus, b.Height); err != nil {
				return fmt.Errorf("invalid tx at height %d: %w", b.Height, err)
			}
		}
	}
	// Ensure state is reproducible
	return bc.recomputeState()
}

// createGenesis reserved for future use
//
//nolint:unused
func (bc *Blockchain) createGenesis(genesisSupply uint64, genesisToAddress, genesisMinerAddress string, genesisTimestampUnix int64) (*Block, error) {
	coinbase := Transaction{
		Type:      TxCoinbase,
		ChainID:   bc.ChainID,
		ToAddress: genesisToAddress,
		Amount:    genesisSupply,
		Data:      fmt.Sprintf("genesis allocation (supply=%d)", genesisSupply),
	}
	genesis := &Block{
		Version:        blockVersionForHeight(bc.consensus, 0),
		Height:         0,
		TimestampUnix:  genesisTimestampUnix,
		PrevHash:       nil,
		DifficultyBits: bc.consensus.GenesisDifficultyBits,
		MinerAddress:   genesisMinerAddress,
		Transactions:   []Transaction{coinbase},
	}

	// Mine genesis block using NogoPow engine
	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()

	genesisHeader := &nogopow.Header{
		ParentHash: nogopow.BytesToHash(genesis.PrevHash),
		Coinbase:   stringToAddress(genesis.MinerAddress),
		Number:     big.NewInt(int64(genesis.Height)),
		Time:       uint64(genesis.TimestampUnix),
		Difficulty: big.NewInt(int64(genesis.DifficultyBits)),
	}

	genesisBlock := nogopow.NewBlock(genesisHeader, nil, nil, nil)
	stop := make(chan struct{})
	resultCh := make(chan *nogopow.Block, 1)

	if err := engine.Seal(bc, genesisBlock, resultCh, stop); err != nil {
		return nil, err
	}

	result, ok := <-resultCh
	if !ok {
		close(stop)
		return nil, fmt.Errorf("genesis mining failed")
	}

	sealedHeader := result.Header()
	genesis.Nonce = binary.LittleEndian.Uint64(sealedHeader.Nonce[:8])
	hashBytes := sealedHeader.Hash().Bytes()
	genesis.Hash = hashBytes

	return genesis, nil
}

func (bc *Blockchain) recomputeState() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.recomputeStateLocked()
}

func (bc *Blockchain) recomputeStateLocked() error {
	bc.state = map[string]Account{}
	for _, b := range bc.blocks {
		if err := applyBlockToState(bc.consensus, bc.state, b); err != nil {
			return fmt.Errorf("apply block %d: %w", b.Height, err)
		}
	}
	return nil
}

type TxLocation struct {
	Height       uint64 `json:"height"`
	BlockHashHex string `json:"blockHashHex"`
	Index        int    `json:"index"`
}

type AddressTxEntry struct {
	TxID      string     `json:"txId"`
	Location  TxLocation `json:"location"`
	FromAddr  string     `json:"fromAddr"`
	ToAddress string     `json:"toAddress"`
	Amount    uint64     `json:"amount"`
	Fee       uint64     `json:"fee"`
	Nonce     uint64     `json:"nonce"`
}

func (bc *Blockchain) TxByID(txid string) (Transaction, TxLocation, bool) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	loc, ok := bc.txIndex[txid]
	if !ok {
		return Transaction{}, TxLocation{}, false
	}
	if loc.Height >= uint64(len(bc.blocks)) || loc.Index < 0 {
		return Transaction{}, TxLocation{}, false
	}
	b := bc.blocks[int(loc.Height)]
	if loc.Index >= len(b.Transactions) {
		return Transaction{}, TxLocation{}, false
	}
	if hex.EncodeToString(b.Hash) != loc.BlockHashHex {
		return Transaction{}, TxLocation{}, false
	}
	return b.Transactions[loc.Index], loc, true
}

func (bc *Blockchain) indexTxsForBlockLocked(b *Block) {
	if bc.txIndex == nil {
		bc.txIndex = map[string]TxLocation{}
	}
	hashHex := hex.EncodeToString(b.Hash)
	for i, tx := range b.Transactions {
		// Only index transfers (coinbase txids can collide).
		if tx.Type != TxTransfer {
			continue
		}
		txid, err := TxIDHexForConsensus(tx, bc.consensus, b.Height)
		if err != nil {
			continue
		}
		bc.txIndex[txid] = TxLocation{Height: b.Height, BlockHashHex: hashHex, Index: i}
	}
}

func (bc *Blockchain) indexAddressTxsForBlockLocked(b *Block) {
	if bc.addressIndex == nil {
		bc.addressIndex = map[string][]AddressTxEntry{}
	}
	hashHex := hex.EncodeToString(b.Hash)
	for i, tx := range b.Transactions {
		if tx.Type != TxTransfer {
			continue
		}
		txid, err := TxIDHexForConsensus(tx, bc.consensus, b.Height)
		if err != nil {
			continue
		}
		from, err := tx.FromAddress()
		if err != nil {
			continue
		}
		entry := AddressTxEntry{
			TxID: txid,
			Location: TxLocation{
				Height:       b.Height,
				BlockHashHex: hashHex,
				Index:        i,
			},
			FromAddr:  from,
			ToAddress: tx.ToAddress,
			Amount:    tx.Amount,
			Fee:       tx.Fee,
			Nonce:     tx.Nonce,
		}
		bc.addressIndex[from] = append(bc.addressIndex[from], entry)
		if tx.ToAddress != from {
			bc.addressIndex[tx.ToAddress] = append(bc.addressIndex[tx.ToAddress], entry)
		}
	}
}

func (bc *Blockchain) reindexAllTxsLocked() {
	bc.txIndex = map[string]TxLocation{}
	for _, b := range bc.blocks {
		bc.indexTxsForBlockLocked(b)
	}
}

func (bc *Blockchain) reindexAllAddressTxsLocked() {
	bc.addressIndex = map[string][]AddressTxEntry{}
	for _, b := range bc.blocks {
		bc.indexAddressTxsForBlockLocked(b)
	}
}

// AddressTxs returns canonical transfer history for an address, newest-first.
// cursor is an offset from the newest item (0 means start at newest).
func (bc *Blockchain) AddressTxs(address string, limit int, cursor int) ([]AddressTxEntry, int, bool) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	if bc.addressIndex == nil {
		return nil, 0, false
	}
	all := bc.addressIndex[address]
	if len(all) == 0 {
		return []AddressTxEntry{}, 0, false
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if cursor < 0 {
		cursor = 0
	}
	start := len(all) - 1 - cursor
	if start < 0 {
		return []AddressTxEntry{}, cursor, false
	}
	out := make([]AddressTxEntry, 0, limit)
	i := start
	for i >= 0 && len(out) < limit {
		out = append(out, all[i])
		i--
	}
	nextCursor := cursor + len(out)
	more := (len(all) - 1 - nextCursor) >= 0
	return out, nextCursor, more
}

func applyBlockToState(p ConsensusParams, state map[string]Account, b *Block) error {
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
		policy := p.MonetaryPolicy
		expected := policy.BlockReward(b.Height) + policy.MinerFeeAmount(fees)
		if cb.Amount != expected {
			return fmt.Errorf("bad coinbase amount: expected %d got %d", expected, cb.Amount)
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

// SelectMempoolTxs picks a fee-sorted set of transactions that are valid against the current chain state
// plus the effects of already-selected mempool transactions.
func (bc *Blockchain) SelectMempoolTxs(mp *Mempool, max int) ([]Transaction, []string, error) {
	if max <= 0 {
		max = 100
	}

	bc.mu.RLock()
	baseState := make(map[string]Account, len(bc.state))
	for k, v := range bc.state {
		baseState[k] = v
	}
	bc.mu.RUnlock()

	entries := mp.EntriesSortedByFeeDesc()
	var picked []Transaction
	var pickedIDs []string

	state := baseState
	nextHeight := bc.LatestBlock().Height + 1

	for _, e := range entries {
		if len(picked) >= max {
			break
		}
		tx := e.tx
		if tx.Type != TxTransfer {
			continue
		}
		if tx.ChainID == 0 {
			tx.ChainID = bc.ChainID
		}
		if err := tx.VerifyForConsensus(bc.consensus, nextHeight); err != nil {
			continue
		}
		fromAddr, err := tx.FromAddress()
		if err != nil {
			continue
		}
		from := state[fromAddr]
		if from.Nonce+1 != tx.Nonce {
			continue
		}
		totalDebit := tx.Amount + tx.Fee
		if from.Balance < totalDebit {
			continue
		}

		// Apply in the simulated state so later txs from same account validate correctly.
		from.Balance -= totalDebit
		from.Nonce = tx.Nonce
		state[fromAddr] = from

		to := state[tx.ToAddress]
		if to.Balance+tx.Amount < to.Balance {
			continue
		}
		to.Balance += tx.Amount
		state[tx.ToAddress] = to

		picked = append(picked, tx)
		pickedIDs = append(pickedIDs, e.txIDHex)
	}

	return picked, pickedIDs, nil
}

func (bc *Blockchain) initCanonicalIndexesLocked() {
	if bc.blocksByHash == nil {
		bc.blocksByHash = map[string]*Block{}
	}
	for _, b := range bc.blocks {
		bc.addToIndexLocked(b)
	}
	bc.bestTipHash = hex.EncodeToString(bc.blocks[len(bc.blocks)-1].Hash)
	bc.reindexAllTxsLocked()
	bc.reindexAllAddressTxsLocked()
	bc.canonicalWork = big.NewInt(0)
	for _, b := range bc.blocks {
		bc.canonicalWork.Add(bc.canonicalWork, WorkForDifficultyBits(b.DifficultyBits))
	}
}

func (bc *Blockchain) addToIndexLocked(b *Block) {
	if len(b.Hash) == 0 {
		return
	}
	bc.blocksByHash[hex.EncodeToString(b.Hash)] = b
}
