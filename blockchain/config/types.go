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

package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// Uint64String is a custom type for flexible uint64 JSON marshaling/unmarshaling
// It accepts both number and string formats in JSON
type Uint64String uint64

// UnmarshalJSON implements json.Unmarshaler interface
func (u *Uint64String) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		var val uint64
		_, err := fmt.Sscanf(s, "%d", &val)
		if err != nil {
			return err
		}
		*u = Uint64String(val)
		return nil
	}

	var val uint64
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}
	*u = Uint64String(val)
	return nil
}

// MarshalJSON implements json.Marshaler interface
func (u Uint64String) MarshalJSON() ([]byte, error) {
	return json.Marshal(uint64(u))
}

// Uint64 returns the underlying uint64 value
func (u Uint64String) Uint64() uint64 {
	return uint64(u)
}

// ConsensusParams defines the consensus parameters for the blockchain
type ConsensusParams struct {
	// ChainID is the unique identifier for the blockchain network
	ChainID uint64 `json:"chainId"`

	// DifficultyEnable indicates whether difficulty adjustment is enabled
	DifficultyEnable bool `json:"difficultyEnable"`

	// BlockTimeTargetSeconds is the target time between blocks in seconds
	BlockTimeTargetSeconds int64 `json:"blockTimeTargetSeconds"`

	// DifficultyAdjustmentInterval is the number of blocks between difficulty adjustments
	DifficultyAdjustmentInterval uint64 `json:"difficultyAdjustmentInterval"`

	// MaxBlockTimeDriftSeconds is the maximum allowed timestamp drift
	MaxBlockTimeDriftSeconds int64 `json:"maxBlockTimeDriftSeconds"`

	// MinDifficulty is the minimum difficulty level
	MinDifficulty uint32 `json:"minDifficulty"`

	// MaxDifficulty is the maximum difficulty level
	MaxDifficulty uint32 `json:"maxDifficulty"`

	// MinDifficultyBits is the minimum difficulty bits
	MinDifficultyBits uint32 `json:"minDifficultyBits"`

	// MaxDifficultyBits is the maximum difficulty bits
	MaxDifficultyBits uint32 `json:"maxDifficultyBits"`

	// MaxDifficultyChangePercent is the maximum difficulty change per adjustment
	MaxDifficultyChangePercent uint8 `json:"maxDifficultyChangePercent"`

	// MedianTimePastWindow is the window size for MTP calculation
	MedianTimePastWindow int `json:"medianTimePastWindow"`

	// MerkleEnable indicates whether Merkle root validation is enabled
	MerkleEnable bool `json:"merkleEnable"`

	// MerkleActivationHeight is the block height at which Merkle validation activates
	MerkleActivationHeight uint64 `json:"merkleActivationHeight"`

	// BinaryEncodingEnable indicates whether binary encoding is used for consensus
	BinaryEncodingEnable bool `json:"binaryEncodingEnable"`

	// BinaryEncodingActivationHeight is the block height at which binary encoding activates
	BinaryEncodingActivationHeight uint64 `json:"binaryEncodingActivationHeight"`

	// GenesisDifficultyBits is the difficulty bits for the genesis block
	GenesisDifficultyBits uint32 `json:"genesisDifficultyBits"`

	// MaxBlockSize is the maximum block size in bytes
	MaxBlockSize uint64 `json:"maxBlockSize"`

	// MaxTransactionsPerBlock is the maximum transactions per block
	MaxTransactionsPerBlock int `json:"maxTransactionsPerBlock"`

	// MonetaryPolicy defines the monetary policy parameters
	MonetaryPolicy MonetaryPolicy `json:"monetaryPolicy"`
}

// Validate validates consensus parameters
func (p *ConsensusParams) Validate() error {
	if p.ChainID == 0 {
		return fmt.Errorf("chainId must be > 0")
	}

	if p.BlockTimeTargetSeconds <= 0 {
		return fmt.Errorf("blockTimeTargetSeconds must be > 0")
	}

	if p.DifficultyAdjustmentInterval == 0 {
		return fmt.Errorf("difficultyAdjustmentInterval must be > 0")
	}

	if p.MaxBlockTimeDriftSeconds <= 0 {
		return fmt.Errorf("maxBlockTimeDriftSeconds must be > 0")
	}

	if p.MinDifficulty == 0 {
		return fmt.Errorf("minDifficulty must be > 0")
	}

	if p.MaxDifficulty == 0 {
		return fmt.Errorf("maxDifficulty must be > 0")
	}

	if p.MinDifficulty > p.MaxDifficulty {
		return fmt.Errorf("minDifficulty cannot exceed maxDifficulty")
	}

	if p.MaxDifficultyChangePercent == 0 || p.MaxDifficultyChangePercent > 100 {
		return fmt.Errorf("maxDifficultyChangePercent must be between 1 and 100")
	}

	if p.MedianTimePastWindow <= 0 || p.MedianTimePastWindow%2 == 0 {
		return fmt.Errorf("medianTimePastWindow must be positive and odd")
	}

	if p.GenesisDifficultyBits < p.MinDifficultyBits || p.GenesisDifficultyBits > p.MaxDifficultyBits {
		return fmt.Errorf("genesisDifficultyBits must be between min and max difficulty bits")
	}

	if err := p.MonetaryPolicy.Validate(); err != nil {
		return fmt.Errorf("monetary policy validation failed: %w", err)
	}

	return nil
}

// BinaryEncodingActive returns true if binary encoding is active at the given height
func (p *ConsensusParams) BinaryEncodingActive(height uint64) bool {
	return p.BinaryEncodingEnable && height >= p.BinaryEncodingActivationHeight
}

// MerkleRootActive returns true if Merkle root validation is active at the given height
func (p *ConsensusParams) MerkleRootActive(height uint64) bool {
	return p.MerkleEnable && height >= p.MerkleActivationHeight
}

// NetworkConfig defines network configuration parameters
type NetworkConfig struct {
	// Name is the network name (e.g., "mainnet", "testnet")
	Name string `json:"name"`

	// ChainID is the chain identifier
	ChainID uint64 `json:"chainId"`

	// GenesisHash is the hash of the genesis block
	GenesisHash string `json:"genesisHash"`

	// BootNodes is the list of bootstrap node addresses
	BootNodes []string `json:"bootNodes"`

	// DNSDiscovery is the list of DNS discovery domains
	DNSDiscovery []string `json:"dnsDiscovery"`

	// P2PPort is the default P2P listening port
	P2PPort uint16 `json:"p2pPort"`

	// HTTPPort is the default HTTP RPC port
	HTTPPort uint16 `json:"httpPort"`

	// WSPort is the default WebSocket port
	WSPort uint16 `json:"wsPort"`

	// EnableWS indicates if WebSocket is enabled by default
	EnableWS bool `json:"enableWS"`

	// MaxPeers is the maximum number of P2P peers
	MaxPeers int `json:"maxPeers"`

	// MaxConnections is the maximum number of connections
	MaxConnections int `json:"maxConnections"`
}

// MiningConfig defines mining configuration parameters
type MiningConfig struct {
	// Enabled indicates if mining is enabled
	Enabled bool `json:"enabled"`

	// MinerAddress is the address to receive mining rewards
	MinerAddress string `json:"minerAddress"`

	// MineInterval is the interval between mining attempts
	MineInterval time.Duration `json:"mineInterval"`

	// MaxTxPerBlock is the maximum transactions per block
	MaxTxPerBlock int `json:"maxTxPerBlock"`

	// ForceEmptyBlocks forces mining empty blocks when no transactions
	ForceEmptyBlocks bool `json:"forceEmptyBlocks"`

	// ConvergenceBaseDelayMs is the base delay for mining convergence
	ConvergenceBaseDelayMs int64 `json:"convergenceBaseDelayMs"`

	// ConvergenceVariableDelayMs is the maximum variable delay
	ConvergenceVariableDelayMs int64 `json:"convergenceVariableDelayMs"`
}

// SyncConfig defines synchronization configuration parameters
type SyncConfig struct {
	// BatchSize is the number of blocks to sync in a single batch
	BatchSize int `json:"batchSize"`

	// MaxRollbackDepth is the maximum blocks to rollback during reorg
	MaxRollbackDepth int `json:"maxRollbackDepth"`

	// LongForkThreshold is the threshold for detecting long forks
	LongForkThreshold int `json:"longForkThreshold"`

	// MaxSyncRange is the maximum blocks to sync in one operation
	MaxSyncRange int `json:"maxSyncRange"`

	// PeerHeightPollIntervalMs is the polling interval for peer height
	PeerHeightPollIntervalMs int64 `json:"peerHeightPollIntervalMs"`

	// NetworkSyncCheckDelayMs is the delay before checking network state
	NetworkSyncCheckDelayMs int64 `json:"networkSyncCheckDelayMs"`
}

// SecurityConfig defines security configuration parameters
type SecurityConfig struct {
	// AdminToken is the admin authentication token
	AdminToken string `json:"-"`

	// RateLimitReqs is the number of requests per rate limit window
	RateLimitReqs int `json:"rateLimitReqs"`

	// RateLimitBurst is the burst size for rate limiting
	RateLimitBurst int `json:"rateLimitBurst"`

	// TrustProxy indicates if proxy headers should be trusted
	TrustProxy bool `json:"trustProxy"`

	// TLSEnabled indicates if TLS is enabled
	TLSEnabled bool `json:"tlsEnabled"`

	// TLSCertFile is the path to TLS certificate file
	TLSCertFile string `json:"tlsCertFile"`

	// TLSKeyFile is the path to TLS private key file
	TLSKeyFile string `json:"tlsKeyFile"`
}

// NTPConfig defines NTP synchronization configuration
type NTPConfig struct {
	// Enabled indicates if NTP synchronization is enabled
	Enabled bool `json:"enabled"`

	// Servers is the list of NTP servers
	Servers []string `json:"servers"`

	// SyncInterval is the interval between NTP syncs
	SyncInterval time.Duration `json:"syncInterval"`

	// MaxDrift is the maximum allowed clock drift
	MaxDrift time.Duration `json:"maxDrift"`
}

// GovernanceConfig defines governance configuration parameters
type GovernanceConfig struct {
	// MinQuorum is the minimum number of votes for quorum
	MinQuorum uint64 `json:"minQuorum"`

	// ApprovalThreshold is the approval threshold (0.0-1.0)
	ApprovalThreshold float64 `json:"approvalThreshold"`

	// VotingPeriodDays is the voting period in days
	VotingPeriodDays int `json:"votingPeriodDays"`

	// ProposalDeposit is the deposit required to create a proposal
	ProposalDeposit uint64 `json:"proposalDeposit"`

	// ExecutionDelayBlocks is the delay before proposal execution
	ExecutionDelayBlocks uint64 `json:"executionDelayBlocks"`
}

// P2PConfig defines P2P network configuration
type P2PConfig struct {
	// Port is the P2P listening port
	Port int `json:"port"`

	// MaxPeers is the maximum number of peers
	MaxPeers int `json:"maxPeers"`

	// Peers is the list of initial peer addresses
	Peers []string `json:"peers"`

	// EnableNAT indicates if NAT traversal is enabled
	EnableNAT bool `json:"enableNAT"`
}

// APIConfig defines API server configuration
type APIConfig struct {
	// HTTPPort is the HTTP API port
	HTTPPort int `json:"httpPort"`

	// WSPort is the WebSocket API port
	WSPort int `json:"wsPort"`

	// Enabled indicates if the API is enabled
	Enabled bool `json:"enabled"`

	// CORS is the list of allowed CORS origins
	CORS []string `json:"cors"`
}

// BlockReader defines a minimal interface for block time validation
type BlockReader interface {
	GetHeight() uint64
	GetTimestamp() int64
}
