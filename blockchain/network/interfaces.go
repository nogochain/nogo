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
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
)

// HeaderLocator is re-exported from core package for network layer convenience
// Actual definition: core.HeaderLocator (core/types.go)
type HeaderLocator = core.HeaderLocator

// BlockchainInterface defines the blockchain interface for P2P operations
// Interface-based design for decoupling and testability
type BlockchainInterface interface {
	// Chain metadata
	LatestBlock() *core.Block
	BlockByHeight(height uint64) (*core.Block, bool)
	BlockByHash(hashHex string) (*core.Block, bool)
	HeadersFrom(from uint64, count uint64) []*core.BlockHeader
	BlocksFrom(from uint64, count uint64) []*core.Block
	Blocks() []*core.Block
	CanonicalWork() *big.Int
	RulesHashHex() string

	// Header-based chain access for block locator (Bitcoin-style sparse hash list)
	BestBlockHeader() (*HeaderLocator, error)
	GetHeaderByHeight(height uint64) (*HeaderLocator, error)

	// Chain state
	GetChainID() uint64
	GetMinerAddress() string
	TotalSupply() uint64
	GetConsensus() config.ConsensusParams

	// Block operations
	AddBlock(block *core.Block) (bool, error)
	RollbackToHeight(height uint64) error

	// Missing block callback (for fast sync from genesis)
	SetOnMissingBlock(callback func(parentHash []byte, height uint64))

	// Block retrieval (for fork resolution - matches core.BlockProvider)
	GetBlockByHash(hash []byte) (*core.Block, bool)
	GetBlockByHashBytes(hash []byte) (*core.Block, bool)
	GetAllBlocks() ([]*core.Block, error)

	// Mempool operations
	SelectMempoolTxs(mp Mempool, maxTxPerBlock int) ([]core.Transaction, []string, error)

	// Mining operations
	MineTransfers(ctx context.Context, txs []core.Transaction) (*core.Block, error)
	CalcNextDifficulty(latest *core.Block, currentTime int64) uint32

	// Chain audit
	AuditChain() error

	// Reorg protection
	IsReorgInProgress() bool

	// Transaction queries
	TxByID(txid string) (*core.Transaction, *core.TxLocation, bool)
	AddressTxs(addr string, limit, cursor int) ([]core.AddressTxEntry, int, bool)
	Balance(addr string) (core.Account, bool)
	HasTransaction(txHash []byte) bool

	// Contract management
	GetContractManager() *core.ContractManager

	// Sync loop coordination
	SyncLoop() SyncLoopInterface
}

// Miner defines the miner interface for P2P coordination
// Production-grade: enables real-time fork detection via P2P broadcasts
type Miner interface {
	// Mining control
	InterruptMining()
	ResumeMining()
	IsVerifying() bool
	OnBlockAdded()

	// Verification coordination - CRITICAL for P2P block processing
	// StartVerification signals that block verification is starting (pauses mining)
	// EndVerification signals that verification is complete (resumes mining)
	StartVerification()
	EndVerification()

	// P2P broadcast coordination
	// CRITICAL: Called when P2P receives block broadcast from peers
	// Enables miner to pause mining and verify forks in real-time
	OnPeerBlockBroadcast(block *core.Block)
}

// PeerBanChecker is the interface for checking whether a peer is banned.
// SecurityManager implements this interface, allowing AdvancedPeerScorer
// to delegate ban checks to the unified security gateway.
type PeerBanChecker interface {
	IsPeerBanned(peerID string) bool
}

// Mempool defines the mempool interface for transaction management
type Mempool interface {
	// Transaction queries
	Contains(txID string) bool
	GetTx(txID string) (*core.Transaction, bool)
	GetTxIDs() []string

	// Transaction operations
	Add(tx core.Transaction) (string, error)
	AddWithoutSignatureValidation(tx core.Transaction) (string, error)
	Remove(txID string)
	RemoveMany(txids []string)

	// Mempool state
	Size() int
	EntriesSortedByFeeDesc() []MempoolEntry

	// Update methods for P2P received transactions
	UpdateHeight(height uint64)
	UpdateConsensus(consensus config.ConsensusParams)
}

// MempoolEntry represents a mempool transaction entry
type MempoolEntry struct {
	Tx       core.Transaction
	TxIDHex  string
	Received interface{} // time.Time in actual implementation
}

// SyncLoopInterface defines the sync loop interface for coordination
type SyncLoopInterface interface {
	IsSyncing() bool
	IsSynced() bool
	TriggerBlockEvent(block *core.Block)
}

// PeerManagerInterface defines the peer management interface
type PeerManagerInterface interface {
	Peers() []string
	AddPeer(addr string) bool
	RemovePeer(addr string)
	GetActivePeers() []string
	FetchChainInfo(ctx context.Context, peer string) (*ChainInfo, error)
}

// ChainInfo represents peer chain information
type ChainInfo struct {
	ChainID              uint64   `json:"chainId"`
	RulesHash            string   `json:"rulesHash"`
	Height               uint64   `json:"height"`
	LatestHash           string   `json:"latestHash"`
	GenesisHash          string   `json:"genesisHash"`
	GenesisTimestampUnix int64    `json:"genesisTimestampUnix"`
	PeersCount           int      `json:"peersCount"`
	Work                 *big.Int `json:"work"`
}

// chainInfoJSON is an intermediate struct for JSON parsing
// Handles both string and number formats for Work field
type chainInfoJSON struct {
	ChainID              uint64 `json:"chainId"`
	RulesHash            string `json:"rulesHash"`
	Height               uint64 `json:"height"`
	LatestHash           string `json:"latestHash"`
	GenesisHash          string `json:"genesisHash"`
	GenesisTimestampUnix int64  `json:"genesisTimestampUnix"`
	PeersCount           int    `json:"peersCount"`
	Work                 string `json:"work"`
}

// UnmarshalJSON implements custom JSON unmarshaling for ChainInfo
// Supports both string and numeric work values
func (c *ChainInfo) UnmarshalJSON(data []byte) error {
	var tmp chainInfoJSON
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	c.ChainID = tmp.ChainID
	c.RulesHash = tmp.RulesHash
	c.Height = tmp.Height
	c.LatestHash = tmp.LatestHash
	c.GenesisHash = tmp.GenesisHash
	c.GenesisTimestampUnix = tmp.GenesisTimestampUnix
	c.PeersCount = tmp.PeersCount

	// Parse Work field - handle both string and number formats
	if tmp.Work != "" {
		// Remove quotes if present (handles "\"22\"" -> "22")
		workStr := strings.Trim(tmp.Work, "\"")
		var ok bool
		c.Work, ok = new(big.Int).SetString(workStr, 10)
		if !ok {
			return fmt.Errorf("invalid work value: %s", tmp.Work)
		}
	} else {
		c.Work = big.NewInt(0)
	}

	return nil
}

// MarshalJSON implements custom JSON marshaling for ChainInfo
func (c *ChainInfo) MarshalJSON() ([]byte, error) {
	workStr := "0"
	if c.Work != nil {
		workStr = c.Work.String()
	}

	return json.Marshal(&chainInfoJSON{
		ChainID:              c.ChainID,
		RulesHash:            c.RulesHash,
		Height:               c.Height,
		LatestHash:           c.LatestHash,
		GenesisHash:          c.GenesisHash,
		GenesisTimestampUnix: c.GenesisTimestampUnix,
		PeersCount:           c.PeersCount,
		Work:                 workStr,
	})
}

// RateLimiterConfig holds rate limiter configuration
type RateLimiterConfig struct {
	Window      time.Duration
	MaxRequests int
	Burst       int
}

// DefaultRateLimitConfig returns the default rate limit configuration
func DefaultRateLimitConfig() RateLimiterConfig {
	return RateLimiterConfig{
		Window:      DefaultRateLimitWindow,
		MaxRequests: DefaultRateLimitMaxRequests,
		Burst:       DefaultRateLimitBurst,
	}
}

// RateLimiter implements rate limiting for DDoS protection
type RateLimiter struct {
	mu       sync.RWMutex
	config   RateLimiterConfig
	requests map[string][]time.Time // IP -> request timestamps
	banned   map[string]time.Time   // IP -> ban expiry
}

// NewRateLimiter creates a new rate limiter with the given configuration
func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	return &RateLimiter{
		config:   config,
		requests: make(map[string][]time.Time),
		banned:   make(map[string]time.Time),
	}
}

// AllowConnection checks if a new connection from the given IP is allowed
func (rl *RateLimiter) AllowConnection(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Check if IP is banned
	if expiry, ok := rl.banned[ip]; ok {
		if time.Now().Before(expiry) {
			return false
		}
		// Ban expired, remove it
		delete(rl.banned, ip)
	}

	// Clean old requests
	now := time.Now()
	windowStart := now.Add(-rl.config.Window)
	if timestamps, ok := rl.requests[ip]; ok {
		valid := make([]time.Time, 0, len(timestamps))
		for _, ts := range timestamps {
			if ts.After(windowStart) {
				valid = append(valid, ts)
			}
		}
		rl.requests[ip] = valid
	}

	// Check if within limit
	if len(rl.requests[ip]) >= rl.config.MaxRequests {
		return false
	}

	// Record this connection
	rl.requests[ip] = append(rl.requests[ip], now)
	return true
}

// AllowMessage checks if a message from the given node/IP is allowed
func (rl *RateLimiter) AllowMessage(nodeID, ip string) bool {
	// For now, use the same logic as AllowConnection
	// Can be extended with node-specific rate limiting
	return rl.AllowConnection(ip)
}

// IsBanned checks if an IP is currently banned
func (rl *RateLimiter) IsBanned(ip string) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if expiry, ok := rl.banned[ip]; ok {
		return time.Now().Before(expiry)
	}
	return false
}

// BanIP bans an IP address for the specified duration
func (rl *RateLimiter) BanIP(ip string, duration time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.banned[ip] = time.Now().Add(duration)
}

// LogFormatter provides structured logging
type LogFormatter interface {
	P2P(format string, args ...any)
	Connection(format string, args ...any)
	BlockProduced(format string, args ...any)
	Success(format string, args ...any)
	Error(format string, args ...any)
	Security(format string, args ...any)
	Sync(format string, args ...any)
	Peer(format string, args ...any)
}

// GetGlobalFormatter returns the global log formatter
// This function should be implemented in the logging package
func GetGlobalFormatter() LogFormatter {
	// Default implementation - should be overridden in production
	return &defaultLogFormatter{}
}

// P2PManager is an alias for Switch for backward compatibility.
// Production-grade: redirects legacy references to the new Switch architecture.
type P2PManager = Switch

type defaultLogFormatter struct{}

func (f *defaultLogFormatter) P2P(format string, args ...any)           { logPrintf(format, args...) }
func (f *defaultLogFormatter) Connection(format string, args ...any)    { logPrintf(format, args...) }
func (f *defaultLogFormatter) BlockProduced(format string, args ...any) { logPrintf(format, args...) }
func (f *defaultLogFormatter) Success(format string, args ...any)       { logPrintf(format, args...) }
func (f *defaultLogFormatter) Error(format string, args ...any)         { logPrintf(format, args...) }
func (f *defaultLogFormatter) Security(format string, args ...any)      { logPrintf(format, args...) }
func (f *defaultLogFormatter) Sync(format string, args ...any)          { logPrintf(format, args...) }
func (f *defaultLogFormatter) Peer(format string, args ...any)          { logPrintf(format, args...) }

// logPrintf is a helper function for logging
func logPrintf(format string, args ...any) {
	log.Printf(format, args...)
}
