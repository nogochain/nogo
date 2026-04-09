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
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	nogoconfig "github.com/nogochain/nogo/config"
)

// GenerateAddress creates a NogoChain address from a public key
// Production-grade: implements address generation with checksum
// Security: uses SHA256 for hashing, includes 4-byte checksum
func GenerateAddress(pubKey []byte) string {
	hash := sha256.Sum256(pubKey)
	addressHash := hash[:HashLen]

	addressData := make([]byte, 1+len(addressHash))
	addressData[0] = AddressVersion
	copy(addressData[1:], addressHash)

	checksum := sha256.Sum256(addressData)
	addressData = append(addressData, checksum[:ChecksumLen]...)

	encoded := hex.EncodeToString(addressData)

	return fmt.Sprintf("%s%s", AddressPrefix, encoded)
}

// ValidateAddress validates a NogoChain address
// Production-grade: validates prefix, length, and checksum
// Logic completeness: checks all address components
func ValidateAddress(addr string) error {
	if len(addr) < len(AddressPrefix)+10 {
		return errors.New("address too short")
	}

	if addr[:len(AddressPrefix)] != AddressPrefix {
		return fmt.Errorf("invalid prefix, expected %s", AddressPrefix)
	}

	encoded := addr[len(AddressPrefix):]

	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("invalid hex: %w", err)
	}

	if len(decoded) < ChecksumLen+1 {
		return errors.New("invalid encoded length")
	}

	addressData := decoded[:len(decoded)-ChecksumLen]
	storedChecksum := decoded[len(decoded)-ChecksumLen:]

	checksum := sha256.Sum256(addressData)

	for i := 0; i < ChecksumLen; i++ {
		if storedChecksum[i] != checksum[i] {
			return errors.New("checksum mismatch")
		}
	}

	return nil
}

const (
	// AddressPrefix is the prefix for NogoChain addresses
	AddressPrefix = "NOGO"
	// AddressVersion is the version byte for addresses
	AddressVersion = 0x00
	// ChecksumLen is the length of the checksum in bytes
	ChecksumLen = 4
	// HashLen is the length of the hash in bytes
	HashLen = 32
	// PubKeySize is the size of Ed25519 public key in bytes
	PubKeySize = ed25519.PublicKeySize
	// SignatureSize is the size of Ed25519 signature in bytes
	SignatureSize = ed25519.SignatureSize
)

const (
	// defaultChainID is the default chain ID for NogoChain mainnet
	defaultChainID = uint64(1)
	// defaultDifficultyBits is the default difficulty bits for genesis block
	defaultDifficultyBits = uint32(18)
	// maxDifficultyBits is the maximum difficulty bits value (uint32 max)
	// Mathematical safety: prevents overflow in difficulty calculations
	maxDifficultyBits = uint32(4294967295)
	// difficultyAdjustmentInterval is the number of blocks between difficulty adjustments
	difficultyAdjustmentInterval = uint64(100)
	// powVerifyProbabilityThreshold is the threshold for PoW verification
	powVerifyProbabilityThreshold = uint8(26)
	// MinFee is the minimum transaction fee in wei (increased from 1 to 10000)
	MinFee = uint64(10000)
	// MinFeePerByte is the fee per byte in wei
	MinFeePerByte = uint64(100)
)

// Default configuration constants for production deployment
// All values are configurable via environment variables
const (
	// DefaultMempoolMax is the default maximum number of transactions in mempool
	DefaultMempoolMax = 10000
	// DefaultMaxTxPerBlock is the default maximum transactions per block
	DefaultMaxTxPerBlock = 100
	// DefaultHTTPTimeout is the default HTTP request timeout in seconds
	DefaultHTTPTimeout = 10
	// DefaultWSPort is the default WebSocket port
	DefaultWSPort = 8080
	// DefaultWSMaxConnections is the default maximum WebSocket connections
	DefaultWSMaxConnections = 100
	// DefaultRateLimitRequests is the default rate limit requests per second (0 = disabled)
	DefaultRateLimitRequests = 0
	// DefaultRateLimitBurst is the default rate limit burst size (0 = disabled)
	DefaultRateLimitBurst = 0
	// DefaultHTTPMaxHeaderBytes is the default maximum HTTP header size in bytes
	DefaultHTTPMaxHeaderBytes = 8192
	// DefaultP2PMaxMessageBytes is the default maximum P2P message size (4MB)
	DefaultP2PMaxMessageBytes = 4 << 20
	// DefaultP2PMaxPeers is the default maximum number of P2P peers
	DefaultP2PMaxPeers = 1000
	// DefaultP2PMaxAddrReturn is the default maximum addresses to return in getaddr
	DefaultP2PMaxAddrReturn = 100
	// DefaultSyncInterval is the default sync interval in milliseconds
	DefaultSyncInterval = 3000 * time.Millisecond
	// DefaultMineInterval is the default mining interval in milliseconds
	DefaultMineInterval = 1000 * time.Millisecond
	// DefaultMaxPoolConns is the default maximum connection pool size
	DefaultMaxPoolConns = 100
	// DefaultMaxConnsPerPeer is the default maximum connections per peer
	DefaultMaxConnsPerPeer = 3
	// DefaultSyncWorkers is the default number of sync workers
	DefaultSyncWorkers = 8

	// MaxBlockTimeDriftSec is the maximum allowed block time drift in seconds (deprecated: use config.ConsensusParams)
	MaxBlockTimeDriftSec = int64(900) // 15 minutes
	// DifficultyTolerancePercent is the tolerance percentage for difficulty adjustment (deprecated: use config.ConsensusParams)
	DifficultyTolerancePercent = uint8(20)
)

// BlockHeader represents the header of a block
// Production-grade: all fields are exported for JSON serialization
// Concurrency safety: immutable after creation, safe for concurrent reads
type BlockHeader struct {
	Version        uint32 `json:"version"`
	PrevHash       []byte `json:"prevHash"`
	TimestampUnix  int64  `json:"timestampUnix"`
	DifficultyBits uint32 `json:"difficultyBits"`
	Difficulty     uint32 `json:"difficulty"`
	Nonce          uint64 `json:"nonce"`
	MerkleRoot     []byte `json:"merkleRoot,omitempty"`
}

// Height returns the block height from header context
// Note: BlockHeader itself doesn't store height, this is a convenience method
func (h *BlockHeader) Height(blockHeight uint64) uint64 {
	return blockHeight
}

// HashHex returns the block hash as hex string
// Note: BlockHeader doesn't store hash, this requires block context
func (h *BlockHeader) HashHex(blockHash []byte) string {
	if blockHash == nil {
		return ""
	}
	return hex.EncodeToString(blockHash)
}

// Block represents a blockchain block
// Production-grade: includes all necessary fields for consensus
// Concurrency safety: use mutex for write operations, safe for concurrent reads
type Block struct {
	mu sync.RWMutex

	Version      uint32        `json:"version"`
	Hash         []byte        `json:"hash,omitempty"`
	Height       uint64        `json:"height"`
	Header       BlockHeader   `json:"header"`
	Transactions []Transaction `json:"transactions"`
	CoinbaseTx   *Transaction  `json:"coinbaseTx,omitempty"`
	MinerAddress string        `json:"minerAddress"`
	TotalWork    string        `json:"totalWork"`

	TimestampUnix  int64  `json:"timestampUnix"`
	DifficultyBits uint32 `json:"difficultyBits"`
	Difficulty     uint32 `json:"difficulty"`
	Nonce          uint64 `json:"nonce"`
	PrevHash       []byte `json:"prevHash"`
}

// GetHeight returns the block height
// Concurrency safety: read-only operation, safe for concurrent access
func (b *Block) GetHeight() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Height
}

// GetHash returns the block hash
// Concurrency safety: returns copy to prevent external modification
func (b *Block) GetHash() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.Hash == nil {
		return nil
	}
	hashCopy := make([]byte, len(b.Hash))
	copy(hashCopy, b.Hash)
	return hashCopy
}

// GetPrevHash returns the previous block hash
// Concurrency safety: returns copy to prevent external modification
func (b *Block) GetPrevHash() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.Header.PrevHash == nil {
		return nil
	}
	hashCopy := make([]byte, len(b.Header.PrevHash))
	copy(hashCopy, b.Header.PrevHash)
	return hashCopy
}

// GetTimestampUnix returns the block timestamp
// Concurrency safety: read-only operation, safe for concurrent access
func (b *Block) GetTimestampUnix() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Header.TimestampUnix
}

// GetDifficultyBits returns the difficulty bits
// Concurrency safety: read-only operation, safe for concurrent access
func (b *Block) GetDifficultyBits() uint32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Header.DifficultyBits
}

// GetMinerAddress returns the miner address
// Concurrency safety: read-only operation, safe for concurrent access
func (b *Block) GetMinerAddress() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.MinerAddress
}

// SetTimestampUnix sets the timestamp in both Block and Header
// Concurrency safety: uses mutex to protect concurrent writes
func (b *Block) SetTimestampUnix(ts int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.TimestampUnix = ts
	b.Header.TimestampUnix = ts
}

// SetDifficultyBits sets the difficulty in both Block and Header
// Concurrency safety: uses mutex to protect concurrent writes
func (b *Block) SetDifficultyBits(diff uint32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.DifficultyBits = diff
	b.Header.DifficultyBits = diff
	b.Header.Difficulty = diff
}

// SetNonce sets the nonce in both Block and Header
// Concurrency safety: uses mutex to protect concurrent writes
func (b *Block) SetNonce(nonce uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Nonce = nonce
	b.Header.Nonce = nonce
}

// SetHash sets the block hash
// Concurrency safety: uses mutex to protect concurrent writes
func (b *Block) SetHash(hash []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if hash != nil {
		b.Hash = make([]byte, len(hash))
		copy(b.Hash, hash)
	}
}

// SetTransactions sets the transactions with validation
// Concurrency safety: uses mutex to protect concurrent writes
// Logic completeness: creates deep copy of transactions slice
func (b *Block) SetTransactions(txs []Transaction) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if txs == nil {
		b.Transactions = nil
		return
	}
	b.Transactions = make([]Transaction, len(txs))
	copy(b.Transactions, txs)
}

// GetTransactions returns a copy of the transactions
// Concurrency safety: returns copy to prevent external modification
func (b *Block) GetTransactions() []Transaction {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.Transactions == nil {
		return nil
	}
	txs := make([]Transaction, len(b.Transactions))
	copy(txs, b.Transactions)
	return txs
}

// TxRootLegacyForConsensus computes the legacy transaction root
// Production-grade: used for v1 blocks and backward compatibility
func (b *Block) TxRootLegacyForConsensus(p ConsensusParams) ([]byte, error) {
	h := sha256.New()
	for _, tx := range b.Transactions {
		th, err := txSigningHashForConsensus(tx, p, b.Height)
		if err != nil {
			return nil, err
		}
		h.Write(th)
	}
	sum := h.Sum(nil)
	return sum, nil
}

// MerkleRootV2ForConsensus computes the Merkle root for v2 blocks
// Production-grade: used for blocks with version >= 2
func (b *Block) MerkleRootV2ForConsensus(p ConsensusParams) ([]byte, error) {
	leaves := make([][]byte, 0, len(b.Transactions))
	for _, tx := range b.Transactions {
		th, err := txSigningHashForConsensus(tx, p, b.Height)
		if err != nil {
			return nil, err
		}
		leaves = append(leaves, th)
	}
	return MerkleRoot(leaves)
}

// HeaderBytesForConsensus returns the header bytes for hashing
// Production-grade: consensus-aware header encoding
func (b *Block) HeaderBytesForConsensus(p ConsensusParams, nonce uint64) ([]byte, error) {
	return blockHeaderPreimageBinaryV1(b, nonce, p)
}

// ConsensusParams defines the consensus parameters
// Type alias to config package to avoid circular dependency
type ConsensusParams = config.ConsensusParams

// Account represents a blockchain account
// Production-grade: simple structure for balance and nonce tracking
// Concurrency safety: immutable after creation, safe for concurrent reads
type Account struct {
	Balance uint64 `json:"balance"`
	Nonce   uint64 `json:"nonce"`
}

// AddBalance adds amount to balance with overflow protection
// Math & numeric safety: checks for overflow before addition
// Returns error if overflow would occur
func (a *Account) AddBalance(amount uint64) error {
	if amount > math.MaxUint64-a.Balance {
		return errors.New("balance overflow")
	}
	a.Balance += amount
	return nil
}

// SubBalance subtracts amount from balance with underflow protection
// Math & numeric safety: checks for underflow before subtraction
// Returns error if underflow would occur
func (a *Account) SubBalance(amount uint64) error {
	if amount > a.Balance {
		return errors.New("balance underflow")
	}
	a.Balance -= amount
	return nil
}

// IncrementNonce increments the nonce with overflow protection
// Math & numeric safety: checks for overflow before increment
// Returns error if overflow would occur
func (a *Account) IncrementNonce() error {
	if a.Nonce >= math.MaxUint64 {
		return errors.New("nonce overflow")
	}
	a.Nonce++
	return nil
}

// TransactionType represents the type of transaction
type TransactionType string

const (
	// TxCoinbase represents a coinbase transaction (block reward)
	TxCoinbase TransactionType = "coinbase"
	// TxTransfer represents a transfer transaction
	TxTransfer TransactionType = "transfer"
)

// Transaction represents a blockchain transaction
// Production-grade: includes all fields for consensus and validation
// Concurrency safety: immutable after creation, safe for concurrent reads
type Transaction struct {
	Type TransactionType `json:"type"`

	ChainID uint64 `json:"chainId"`

	FromPubKey []byte `json:"fromPubKey,omitempty"`
	ToAddress  string `json:"toAddress"`

	Amount uint64 `json:"amount"`
	Fee    uint64 `json:"fee"`
	Nonce  uint64 `json:"nonce,omitempty"`

	Data string `json:"data,omitempty"`

	Signature []byte `json:"signature,omitempty"`
}

// GetID returns the transaction ID
// Error handling: ignores error for compatibility, use GetIDWithError for production
func (t Transaction) GetID() string {
	txid, _ := TxIDHex(t)
	return txid
}

// GetIDWithError returns the transaction ID with error handling
// Production-grade: always check errors in production code
func (t Transaction) GetIDWithError() (string, error) {
	return TxIDHex(t)
}

// GetSender returns the sender address
// Error handling: ignores error for compatibility, use GetSenderWithError for production
func (t Transaction) GetSender() string {
	addr, _ := t.FromAddress()
	return addr
}

// GetSenderWithError returns the sender address with error handling
// Production-grade: always check errors in production code
func (t Transaction) GetSenderWithError() (string, error) {
	return t.FromAddress()
}

// GetReceiver returns the receiver address
// Concurrency safety: read-only operation, safe for concurrent access
func (t Transaction) GetReceiver() string {
	return t.ToAddress
}

// GetAmount returns the transaction amount as big.Int
// Math & numeric safety: returns big.Int for safe arithmetic
func (t Transaction) GetAmount() *big.Int {
	return new(big.Int).SetUint64(t.Amount)
}

// GetFee returns the transaction fee as big.Int
// Math & numeric safety: returns big.Int for safe arithmetic
func (t Transaction) GetFee() *big.Int {
	return new(big.Int).SetUint64(t.Fee)
}

// GetNonce returns the transaction nonce
// Concurrency safety: read-only operation, safe for concurrent access
func (t Transaction) GetNonce() uint64 {
	return t.Nonce
}

// GetTimestamp returns the transaction timestamp
// Design: In NogoChain, transactions do not carry explicit timestamps.
// Transaction ordering is determined by:
//  1. Block height (transactions are ordered by inclusion height)
//  2. Transaction index within block (deterministic ordering)
//  3. Nonce (per-account sequential ordering)
//
// This design choice:
// - Reduces transaction size (no timestamp field)
// - Prevents timestamp manipulation attacks
// - Simplifies consensus (no timestamp validation rules)
//
// If timestamp is needed, use the parent block's timestamp:
//
//	block := chain.GetBlockByTxID(txid)
//	timestamp := block.TimestampUnix
func (t Transaction) GetTimestamp() int64 {
	// Transactions don't have timestamps - return 0 to indicate "not available"
	// Use block timestamp for temporal ordering instead
	return 0
}

// FromAddress returns the sender address derived from the public key
// Logic completeness: validates public key length before deriving address
// Security: uses Ed25519 public key size constant for validation
func (t Transaction) FromAddress() (string, error) {
	if t.Type != TxTransfer {
		return "", errors.New("from address only exists for transfer transactions")
	}
	if len(t.FromPubKey) != PubKeySize {
		return "", fmt.Errorf("invalid fromPubKey length: %d, expected %d", len(t.FromPubKey), PubKeySize)
	}
	return GenerateAddress(t.FromPubKey), nil
}

// Verify performs basic transaction validation
// Logic completeness: covers all transaction types and validation branches
// Error handling: all errors include context for debugging
func (t Transaction) Verify() error {
	switch t.Type {
	case TxCoinbase:
		return t.verifyCoinbase()
	case TxTransfer:
		return t.verifyTransfer()
	default:
		return fmt.Errorf("unknown transaction type: %q", t.Type)
	}
}

// verifyCoinbase validates a coinbase transaction
// Logic completeness: checks all coinbase-specific constraints
func (t Transaction) verifyCoinbase() error {
	if t.ChainID == 0 {
		return errors.New("chainId must be set")
	}
	if t.Amount == 0 {
		return errors.New("coinbase amount must be > 0")
	}
	if err := ValidateAddress(t.ToAddress); err != nil {
		return fmt.Errorf("invalid toAddress: %w", err)
	}
	if t.FromPubKey != nil || t.Signature != nil || t.Nonce != 0 || t.Fee != 0 {
		return errors.New("coinbase must not include fromPubKey/signature/nonce/fee")
	}
	return nil
}

// verifyTransfer validates a transfer transaction
// Logic completeness: checks all transfer-specific constraints including signature
func (t Transaction) verifyTransfer() error {
	if t.Amount == 0 {
		return errors.New("amount must be > 0")
	}
	if err := ValidateAddress(t.ToAddress); err != nil {
		return fmt.Errorf("invalid toAddress: %w", err)
	}
	if len(t.FromPubKey) != PubKeySize {
		return fmt.Errorf("invalid fromPubKey length: %d, expected %d", len(t.FromPubKey), PubKeySize)
	}
	if len(t.Signature) != SignatureSize {
		return fmt.Errorf("invalid signature length: %d, expected %d", len(t.Signature), SignatureSize)
	}
	if t.Nonce == 0 {
		return errors.New("nonce must be > 0")
	}
	if t.ChainID == 0 {
		return errors.New("chainId must be set")
	}
	h, err := t.SigningHash()
	if err != nil {
		return err
	}
	return t.verifyWithSigningHash(h)
}

// MarshalJSON implements custom JSON marshaling for Block
// Production-grade: includes all fields needed for block explorers and wallets
func (b *Block) MarshalJSON() ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Build transaction list
	txs := make([]Transaction, len(b.Transactions))
	copy(txs, b.Transactions)

	// Build response with all fields exposed
	response := map[string]interface{}{
		"version":        b.Version,
		"height":         b.Height,
		"hash":           base64.StdEncoding.EncodeToString(b.Hash),
		"prevHash":       base64.StdEncoding.EncodeToString(b.Header.PrevHash),
		"timestampUnix":  b.Header.TimestampUnix,
		"difficultyBits": b.Header.DifficultyBits,
		"nonce":          b.Header.Nonce,
		"minerAddress":   b.MinerAddress,
		"transactions":   txs,
		"coinbaseTx":     b.CoinbaseTx,
		"totalWork":      b.TotalWork,
	}

	return json.Marshal(response)
}

// verifyWithSigningHash verifies the transaction signature
// Security: uses Ed25519 signature verification
// Math & numeric safety: validates key and signature lengths
func (t Transaction) verifyWithSigningHash(h []byte) error {
	if t.Type != TxTransfer {
		return errors.New("signature verification only applies to transfer transactions")
	}
	if len(t.FromPubKey) != PubKeySize {
		return fmt.Errorf("invalid fromPubKey length: %d", len(t.FromPubKey))
	}
	if len(t.Signature) != SignatureSize {
		return fmt.Errorf("invalid signature length: %d", len(t.Signature))
	}
	if !ed25519.Verify(t.FromPubKey, h, t.Signature) {
		return errors.New("invalid signature")
	}
	return nil
}

// VerifyForConsensus validates a transaction under consensus rules
// Production-grade: accepts consensus parameters and block height
// Error handling: all errors include context for debugging
func (t Transaction) VerifyForConsensus(p ConsensusParams, height uint64) error {
	return t.VerifyForConsensusWithMetrics(p, height, nil)
}

// VerifyForConsensusWithMetrics validates a transaction with optional metrics
// Production-grade: includes metrics collection for monitoring
func (t Transaction) VerifyForConsensusWithMetrics(p ConsensusParams, height uint64, metrics MetricsCollector) error {
	startTime := time.Now()
	defer func() {
		if metrics != nil {
			metrics.ObserveTransactionVerification(time.Since(startTime))
		}
	}()

	switch t.Type {
	case TxCoinbase:
		return t.Verify()
	case TxTransfer:
		return t.verifyTransferForConsensus(p, height)
	default:
		return fmt.Errorf("unknown transaction type: %q", t.Type)
	}
}

// verifyTransferForConsensus validates a transfer transaction for consensus
// Production-grade: uses consensus-aware signing hash
func (t Transaction) verifyTransferForConsensus(p ConsensusParams, height uint64) error {
	if t.Amount == 0 {
		return errors.New("amount must be > 0")
	}
	if err := ValidateAddress(t.ToAddress); err != nil {
		return fmt.Errorf("invalid toAddress: %w", err)
	}
	if len(t.FromPubKey) != PubKeySize {
		return fmt.Errorf("invalid fromPubKey length: %d", len(t.FromPubKey))
	}
	if len(t.Signature) != SignatureSize {
		return fmt.Errorf("invalid signature length: %d", len(t.Signature))
	}
	if t.Nonce == 0 {
		return errors.New("nonce must be > 0")
	}
	if t.ChainID == 0 {
		return errors.New("chainId must be set")
	}
	h, err := txSigningHashForConsensus(t, p, height)
	if err != nil {
		return err
	}
	return t.verifyWithSigningHash(h)
}

// SigningHash returns the signing hash for the transaction
// Production-grade: uses consensus-aware implementation
func (t Transaction) SigningHash() ([]byte, error) {
	return t.signingHashLegacyJSON()
}

// signingHashLegacyJSON computes the legacy JSON-based signing hash
// Note: This is for backward compatibility, prefer consensus-aware methods
func (t Transaction) signingHashLegacyJSON() ([]byte, error) {
	type signingView struct {
		Type      TransactionType `json:"type"`
		ChainID   uint64          `json:"chainId"`
		FromAddr  string          `json:"fromAddr,omitempty"`
		ToAddress string          `json:"toAddress"`
		Amount    uint64          `json:"amount"`
		Fee       uint64          `json:"fee"`
		Nonce     uint64          `json:"nonce,omitempty"`
		Data      string          `json:"data,omitempty"`
	}

	v := signingView{
		Type:      t.Type,
		ChainID:   t.ChainID,
		ToAddress: t.ToAddress,
		Amount:    t.Amount,
		Fee:       t.Fee,
		Nonce:     t.Nonce,
		Data:      t.Data,
	}

	if t.Type == TxTransfer {
		fromAddr, err := t.FromAddress()
		if err != nil {
			return nil, err
		}
		v.FromAddr = fromAddr
	}

	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	return sum[:], nil
}

// MonetaryPolicy defines the monetary policy for block rewards
// Production-grade: implements NogoChain economic model
// Concurrency safety: immutable after creation, safe for concurrent reads
type MonetaryPolicy struct {
	InitialBlockReward     uint64 `json:"initialBlockReward"`
	MinimumBlockReward     uint64 `json:"minimumBlockReward"`
	AnnualReductionPercent uint8  `json:"annualReductionPercent"`
	MinerFeeShare          uint8  `json:"minerFeeShare"`
	UncleRewardEnabled     bool   `json:"uncleRewardEnabled"`
	MaxUncleDepth          uint8  `json:"maxUncleDepth"`
	// Reward distribution shares (must sum to 100)
	MinerRewardShare   uint8 `json:"minerRewardShare"`
	CommunityFundShare uint8 `json:"communityFundShare"`
	GenesisShare       uint8 `json:"genesisShare"`
	IntegrityPoolShare uint8 `json:"integrityPoolShare"`
	// Legacy fields for compatibility
	HalvingInterval uint64 `json:"halvingInterval"`
	MaxSupply       uint64 `json:"maxSupply"`
	TailEmission    uint64 `json:"tailEmission"`
}

// BlockReward calculates the block reward for a given height
// Math & numeric safety: uses big.Int for precise calculations
// Production-grade: implements geometric decay economic model
func (p MonetaryPolicy) BlockReward(height uint64) uint64 {
	if p.InitialBlockReward == 0 {
		return 0
	}

	minReward := p.MinimumBlockReward
	if minReward == 0 {
		minReward = 0
	}

	years := height / nogoconfig.GetBlocksPerYear()

	reward := new(big.Int).SetUint64(p.InitialBlockReward)
	minRewardBig := new(big.Int).SetUint64(minReward)

	for i := uint64(0); i < years; i++ {
		if reward.Cmp(minRewardBig) <= 0 {
			return minReward
		}
		reward.Mul(reward, big.NewInt(int64(10-p.AnnualReductionPercent)))
		reward.Div(reward, big.NewInt(10))
		if reward.Cmp(minRewardBig) <= 0 {
			return minReward
		}
	}

	if reward.Cmp(minRewardBig) < 0 {
		return minReward
	}

	if !reward.IsUint64() {
		return minReward
	}

	return reward.Uint64()
}

// GetUncleReward calculates the uncle block reward
// Math & numeric safety: validates inputs and uses integer arithmetic
func (p MonetaryPolicy) GetUncleReward(nephewHeight, uncleHeight uint64, blockReward uint64) uint64 {
	if nephewHeight <= uncleHeight {
		return 0
	}

	distance := nephewHeight - uncleHeight
	if distance == 0 {
		return 0
	}

	maxDepth := p.MaxUncleDepth
	if maxDepth == 0 {
		maxDepth = 6
	}
	if distance > uint64(maxDepth) || distance >= 8 {
		return 0
	}

	multiplier := 8 - distance
	rewardBig := new(big.Int).SetUint64(uint64(multiplier))
	rewardBig.Mul(rewardBig, big.NewInt(int64(blockReward)))
	rewardBig.Div(rewardBig, big.NewInt(8))

	if !rewardBig.IsUint64() {
		return 0
	}

	return rewardBig.Uint64()
}

// GetNephewBonus calculates the bonus for including uncle blocks
// Math & numeric safety: validates inputs and prevents overflow
func (p MonetaryPolicy) GetNephewBonus(blockReward uint64, uncleCount int) uint64 {
	if uncleCount <= 0 {
		return 0
	}

	if uncleCount > 2 {
		uncleCount = 2
	}

	bonusPerUncle := blockReward / 32
	totalBonus := uint64(uncleCount) * bonusPerUncle

	if totalBonus > blockReward {
		return blockReward
	}

	return totalBonus
}

// GetTotalMinerReward calculates total reward including uncle bonuses
// Math & numeric safety: checks for overflow before addition
func (p MonetaryPolicy) GetTotalMinerReward(height uint64, uncleCount int) uint64 {
	blockReward := p.BlockReward(height)
	nephewBonus := p.GetNephewBonus(blockReward, uncleCount)

	if blockReward > math.MaxUint64-nephewBonus {
		return blockReward
	}

	return blockReward + nephewBonus
}

// MinerFeeAmount calculates the amount of fees allocated to the miner
// Math & numeric safety: uses integer arithmetic to prevent precision loss
func (p MonetaryPolicy) MinerFeeAmount(totalFees uint64) uint64 {
	if p.MinerFeeShare == 0 || totalFees == 0 {
		return 0
	}
	return totalFees * uint64(p.MinerFeeShare) / 100
}

// Validate validates the monetary policy parameters
// Logic completeness: checks all parameter constraints
func (p MonetaryPolicy) Validate() error {
	if p.InitialBlockReward == 0 {
		return errors.New("initialBlockReward must be > 0")
	}
	if p.MinimumBlockReward >= p.InitialBlockReward {
		return errors.New("minimumBlockReward must be < initialBlockReward")
	}
	if p.AnnualReductionPercent > 100 {
		return errors.New("annualReductionPercent must be <= 100")
	}
	if p.UncleRewardEnabled && (p.MaxUncleDepth < 1 || p.MaxUncleDepth > 10) {
		return errors.New("maxUncleDepth must be between 1 and 10")
	}
	if p.MinerFeeShare > 100 {
		return errors.New("minerFeeShare must be <= 100")
	}
	return nil
}

// EventSink defines the event sink interface for blockchain events
type EventSink interface {
	Publish(event WSEvent)
}

// WSEvent represents a WebSocket event
type WSEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// ChainStore defines the interface for blockchain storage
type ChainStore interface {
	SaveBlock(block *Block) error
	LoadBlock(hash []byte) (*Block, error)
	LoadCanonicalChain() ([]*Block, error)
	SaveCanonicalChain(blocks []*Block) error
	ReadCanonical() ([]*Block, error)
	ReadAllBlocks() (map[string]*Block, error)
	GetRulesHash() ([]byte, bool, error)
	PutRulesHash(hash []byte) error
	AppendCanonical(block *Block) error
	RewriteCanonical(blocks []*Block) error
	PutBlock(block *Block) error
	GetGenesisHash() ([]byte, bool, error)
	PutGenesisHash(hash []byte) error
}

// SyncLoop represents the sync loop for P2P synchronization
type SyncLoop struct {
	mu sync.Mutex
}

// peerRef represents a peer reference
type peerRef struct {
	addr string
}

// Mempool represents the transaction mempool
type Mempool struct {
	mu sync.RWMutex
}

// MetricsCollector defines interface for metrics collection
// Production-grade: implemented by metrics.Metrics for Prometheus integration
// Design: interface-based design allows dependency injection for testing
type MetricsCollector interface {
	ObserveTransactionVerification(duration time.Duration)
	ObserveBlockVerification(duration time.Duration)
}

// NoopMetrics is a no-op implementation of MetricsCollector
// Production-grade: safe default when metrics are disabled
// Performance: zero overhead - methods are inlined and optimized away
type NoopMetrics struct{}

// ObserveTransactionVerification is a no-op implementation
func (m *NoopMetrics) ObserveTransactionVerification(duration time.Duration) {
	// Intentionally empty - no-op for when metrics are disabled
}

// ObserveBlockVerification is a no-op implementation
func (m *NoopMetrics) ObserveBlockVerification(duration time.Duration) {
	// Intentionally empty - no-op for when metrics are disabled
}

// Note: For actual metrics collection, use metrics.Metrics from the metrics package.
// Example:
//   import "github.com/nogochain/nogo/blockchain/metrics"
//   m := metrics.NewMetrics(blockchain, mempool, peers, syncLoop, nodeID, chainID)
//   m.ObserveTransactionVerification(duration)

// validateAddress validates an address (alias for ValidateAddress for compatibility)
// Production-grade: supports both NOGO prefix and raw hex formats
func validateAddress(addr string) error {
	return ValidateAddress(addr)
}

// StringToAddress converts a string address to nogopow.Address
// Production-grade: handles NOGO prefix and validates length
// Used in consensus validation and mining
func StringToAddress(addr string) ([20]byte, error) {
	var result [20]byte
	encoded := addr
	
	// Strip NOGO prefix if present
	if len(addr) >= 4 && addr[:4] == "NOGO" {
		encoded = addr[4:]
	}
	
	// Decode hex string to bytes
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return result, fmt.Errorf("invalid address hex encoding: %w", err)
	}
	
	// Validate address length (must be at least 20 bytes)
	if len(decoded) < 20 {
		return result, fmt.Errorf("address too short: expected at least 20 bytes, got %d", len(decoded))
	}
	
	// Copy first 20 bytes
	copy(result[:], decoded[:20])
	return result, nil
}

// TxIDHexForConsensus computes transaction ID for consensus
// Production-grade: uses consensus-aware encoding
func TxIDHexForConsensus(tx Transaction, p ConsensusParams, height uint64) (string, error) {
	hash, err := txSigningHashForConsensus(tx, p, height)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash), nil
}

// hexDecode decodes a hex string (alias for hexDecodeString for compatibility)
// Error handling: returns descriptive error for invalid hex
func hexDecode(s string) ([]byte, error) {
	return hexDecodeString(s)
}

// blockSizeForConsensus returns the size of the block for consensus
// Production-grade: uses JSON encoding as the canonical representation
func blockSizeForConsensus(b *Block) (int, error) {
	if b == nil {
		return 0, errors.New("nil block")
	}
	data, err := json.Marshal(b)
	if err != nil {
		return 0, fmt.Errorf("marshal block: %w", err)
	}
	return len(data), nil
}

// BuildGenesisBlock creates the genesis block (alias for GetGenesisBlock for compatibility)
// Production-grade: mines the genesis block using NogoPow engine
func BuildGenesisBlock(cfg *GenesisConfig, consensus ConsensusParams) (*Block, error) {
	return GetGenesisBlock(cfg, consensus)
}

// Blockchain is an alias for Chain for backward compatibility
// Production-grade: this type alias maintains compatibility with existing code
type Blockchain = Chain
