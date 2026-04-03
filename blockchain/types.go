package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	defaultChainID    = uint64(1)
	maxDifficultyBits = uint32(256)

	difficultyAdjustmentInterval = uint64(100)
	powVerifyProbabilityThreshold = uint8(26)
)

// Default configuration constants for production deployment
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
	
	// DefaultP2PMaxConnections is the default maximum P2P connections
	DefaultP2PMaxConnections = 200
	
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
	
	// DefaultSyncBatchSize is the default sync batch size
	DefaultSyncBatchSize = 100
)

// ConsensusParams defines the consensus parameters for the blockchain
type ConsensusParams struct {
	DifficultyEnable bool

	TargetBlockTime   time.Duration
	DifficultyWindow  int
	DifficultyMaxStep uint32

	MinDifficultyBits uint32
	MaxDifficultyBits uint32

	GenesisDifficultyBits uint32

	MedianTimePastWindow int
	MaxTimeDrift         int64
	MaxBlockSize         uint64

	// MerkleEnable gates a new block header commitment scheme (v2 blocks).
	// When enabled, blocks at height >= MerkleActivationHeight must use Version=2 and commit to a Merkle root.
	MerkleEnable           bool
	MerkleActivationHeight uint64

	// BinaryEncodingEnable switches consensus-critical hashing away from JSON serialization
	// (tx signing hash / txid, and PoW header hashing).
	//
	// If enabled, blocks at height >= BinaryEncodingActivationHeight must use the binary
	// encoding scheme for:
	// - Transaction signing hash (and therefore txid)
	// - Block header hashing (PoW)
	BinaryEncodingEnable           bool
	BinaryEncodingActivationHeight uint64

	// MonetaryPolicy defines block subsidy + fee allocation rules.
	MonetaryPolicy MonetaryPolicy
}

// BinaryEncodingActive returns true if binary encoding is active at the given height
func (p ConsensusParams) BinaryEncodingActive(height uint64) bool {
	return p.BinaryEncodingEnable && height >= p.BinaryEncodingActivationHeight
}

type Account struct {
	Balance uint64 `json:"balance"`
	Nonce   uint64 `json:"nonce"`
}

type TransactionType string

const (
	TxCoinbase TransactionType = "coinbase"
	TxTransfer TransactionType = "transfer"
)

type Transaction struct {
	Type TransactionType `json:"type"`

	ChainID uint64 `json:"chainId"`

	FromPubKey []byte `json:"fromPubKey,omitempty"` // base64 in JSON
	ToAddress  string `json:"toAddress"`

	Amount uint64 `json:"amount"`
	Fee    uint64 `json:"fee"`
	Nonce  uint64 `json:"nonce,omitempty"`

	Data string `json:"data,omitempty"`

	Signature []byte `json:"signature,omitempty"` // base64 in JSON
}

func validateAddress(addr string) error {
	return ValidateAddress(addr)
}

func (t Transaction) FromAddress() (string, error) {
	if t.Type != TxTransfer {
		return "", errors.New("from address only exists for transfer transactions")
	}
	if len(t.FromPubKey) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid fromPubKey length: %d", len(t.FromPubKey))
	}
	return GenerateAddress(t.FromPubKey), nil
}

// signingHashLegacyJSON is the legacy signing hash.
// It is NOT suitable for cross-language consensus unless every implementation
// exactly matches the JSON canonicalization performed by Go's encoding/json.
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

// SigningHash returns the legacy JSON-based signing hash.
// Prefer using consensus-aware helpers instead of calling this directly.
func (t Transaction) SigningHash() ([]byte, error) {
	return t.signingHashLegacyJSON()
}

func (t Transaction) verifyWithSigningHash(h []byte) error {
	if t.Type != TxTransfer {
		return errors.New("signature verification only applies to transfer transactions")
	}
	if len(t.FromPubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid fromPubKey length: %d", len(t.FromPubKey))
	}
	if len(t.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length: %d", len(t.Signature))
	}
	if !ed25519.Verify(t.FromPubKey, h, t.Signature) {
		return errors.New("invalid signature")
	}
	return nil
}

func (t Transaction) Verify() error {
	switch t.Type {
	case TxCoinbase:
		if t.ChainID == 0 {
			return errors.New("chainId must be set")
		}
		if t.Amount == 0 {
			return errors.New("coinbase amount must be > 0")
		}
		if err := validateAddress(t.ToAddress); err != nil {
			return fmt.Errorf("invalid toAddress: %w", err)
		}
		if t.FromPubKey != nil || t.Signature != nil || t.Nonce != 0 || t.Fee != 0 {
			return errors.New("coinbase must not include fromPubKey/signature/nonce/fee")
		}
		return nil
	case TxTransfer:
		if t.Amount == 0 {
			return errors.New("amount must be > 0")
		}
		if err := validateAddress(t.ToAddress); err != nil {
			return fmt.Errorf("invalid toAddress: %w", err)
		}
		if len(t.FromPubKey) != ed25519.PublicKeySize {
			return fmt.Errorf("invalid fromPubKey length: %d", len(t.FromPubKey))
		}
		if len(t.Signature) != ed25519.SignatureSize {
			return fmt.Errorf("invalid signature length: %d", len(t.Signature))
		}
		if t.Nonce == 0 {
			return errors.New("nonce must be > 0")
		}
		if t.ChainID == 0 {
			return errors.New("chainId must be set")
		}
		h, err := t.signingHashLegacyJSON()
		if err != nil {
			return err
		}
		return t.verifyWithSigningHash(h)
	default:
		return fmt.Errorf("unknown transaction type: %q", t.Type)
	}
}

// VerifyForConsensus validates a transaction under the consensus rules active at the given height.
// Height is the block height the transaction is being validated for (i.e. its containing block height
// when validating blocks, or the next block height when validating a mempool submission).
func (t Transaction) VerifyForConsensus(p ConsensusParams, height uint64) error {
	switch t.Type {
	case TxCoinbase:
		// Coinbase does not have a signature. Structural checks are the same across encodings.
		return t.Verify()
	case TxTransfer:
		// Re-run structural checks, but verify signature against the consensus-selected signing hash.
		if t.Amount == 0 {
			return errors.New("amount must be > 0")
		}
		if err := validateAddress(t.ToAddress); err != nil {
			return fmt.Errorf("invalid toAddress: %w", err)
		}
		if len(t.FromPubKey) != ed25519.PublicKeySize {
			return fmt.Errorf("invalid fromPubKey length: %d", len(t.FromPubKey))
		}
		if len(t.Signature) != ed25519.SignatureSize {
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
	default:
		return fmt.Errorf("unknown transaction type: %q", t.Type)
	}
}

type Block struct {
	Version        uint32        `json:"version"`
	Height         uint64        `json:"height"`
	TimestampUnix  int64         `json:"timestampUnix"`
	PrevHash       []byte        `json:"prevHash"` // base64 in JSON
	Nonce          uint64        `json:"nonce"`
	DifficultyBits uint32        `json:"difficultyBits"`
	MinerAddress   string        `json:"minerAddress"`
	Transactions   []Transaction `json:"transactions"`
	Hash           []byte        `json:"hash"` // base64 in JSON
}

// TxRootLegacy is the original v1 commitment: SHA256(concat(txSigningHash)).
func (b *Block) TxRootLegacy() ([]byte, error) {
	return b.TxRootLegacyForConsensus(defaultConsensusParamsFromEnv())
}

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

// MerkleRootV2 returns the Merkle root commitment for v2 blocks.
// Leaves are the transaction signing hashes (32 bytes each).
func (b *Block) MerkleRootV2() ([]byte, error) {
	return b.MerkleRootV2ForConsensus(defaultConsensusParamsFromEnv())
}

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

func (b *Block) CommitmentRootHex() (string, error) {
	var root []byte
	var err error
	switch b.Version {
	case 2:
		root, err = b.MerkleRootV2()
	default:
		root, err = b.TxRootLegacy()
	}
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(root), nil
}

func (b *Block) HeaderBytes(nonce uint64) ([]byte, error) {
	return b.HeaderBytesForConsensus(defaultConsensusParamsFromEnv(), nonce)
}

func (b *Block) HeaderBytesForConsensus(p ConsensusParams, nonce uint64) ([]byte, error) {
	if p.BinaryEncodingActive(b.Height) {
		return blockHeaderPreimageBinaryV1(b, nonce, p)
	}
	switch b.Version {
	case 2:
		root, err := b.MerkleRootV2ForConsensus(p)
		if err != nil {
			return nil, err
		}
		type headerV2 struct {
			Version        uint32 `json:"version"`
			Height         uint64 `json:"height"`
			TimestampUnix  int64  `json:"timestampUnix"`
			PrevHashB64    string `json:"prevHashB64"`
			MerkleRootHex  string `json:"merkleRootHex"`
			DifficultyBits uint32 `json:"difficultyBits"`
			MinerAddress   string `json:"minerAddress"`
			Nonce          uint64 `json:"nonce"`
		}
		v := headerV2{
			Version:        b.Version,
			Height:         b.Height,
			TimestampUnix:  b.TimestampUnix,
			PrevHashB64:    base64.StdEncoding.EncodeToString(b.PrevHash),
			MerkleRootHex:  hex.EncodeToString(root),
			DifficultyBits: b.DifficultyBits,
			MinerAddress:   b.MinerAddress,
			Nonce:          nonce,
		}
		return json.Marshal(v)
	default:
		// IMPORTANT: keep v1 header encoding stable for backwards compatibility.
		root, err := b.TxRootLegacyForConsensus(p)
		if err != nil {
			return nil, err
		}
		type headerV1 struct {
			Version        uint32 `json:"version"`
			Height         uint64 `json:"height"`
			TimestampUnix  int64  `json:"timestampUnix"`
			PrevHashB64    string `json:"prevHashB64"`
			TxRootHex      string `json:"txRootHex"`
			DifficultyBits uint32 `json:"difficultyBits"`
			MinerAddress   string `json:"minerAddress"`
			Nonce          uint64 `json:"nonce"`
		}
		v := headerV1{
			Version:        b.Version,
			Height:         b.Height,
			TimestampUnix:  b.TimestampUnix,
			PrevHashB64:    base64.StdEncoding.EncodeToString(b.PrevHash),
			TxRootHex:      hex.EncodeToString(root),
			DifficultyBits: b.DifficultyBits,
			MinerAddress:   b.MinerAddress,
			Nonce:          nonce,
		}
		return json.Marshal(v)
	}
}
