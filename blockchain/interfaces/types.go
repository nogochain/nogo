package interfaces

import (
	"context"
	"math/big"
	"time"
)

// Block represents a blockchain block
type Block interface {
	GetHeight() uint64
	GetHash() []byte
	GetPrevHash() []byte
	GetTimestampUnix() int64
	GetDifficultyBits() uint32
	GetMinerAddress() string
}

// PeerAPI defines the interface for peer management
type PeerAPI interface {
	Peers() []string
	AddPeer(addr string) bool
	RemovePeer(addr string)
	GetActivePeers() []string
	FetchChainInfo(ctx context.Context, peer string) (*ChainInfo, error)
	BroadcastBlock(ctx context.Context, block Block)
}

// ChainInfo represents blockchain state summary
type ChainInfo struct {
	ChainID uint64
	Height  uint64
	Hash    string
	Work    *big.Int
}

// MinerAPI defines the interface for miner operations
type MinerAPI interface {
	InterruptMining()
	ResumeMining()
	IsVerifying() bool
	OnBlockAdded()
}

// MempoolAPI defines the interface for mempool operations
type MempoolAPI interface {
	GetTxIDs() []string
}

// SyncLoopAPI defines the interface for sync loop coordination
type SyncLoopAPI interface {
	IsSyncing() bool
	IsSynced() bool
	TriggerBlockEvent(block Block)
}

// ConnectionStats represents connection statistics
type ConnectionStats struct {
	TotalConnections   int64
	ActiveConnections  int
	PeakConnections    int
	TotalBytesReceived int64
	TotalBytesSent     int64
	AverageLatencyMs   float64
}

// RateLimiterAPI defines interface for rate limiting
type RateLimiterAPI interface {
	AllowConnection(ip string) bool
	AllowMessage(nodeID, ip string) bool
	IsBanned(ip string) bool
}

// PeerScorerAPI defines interface for peer scoring
type PeerScorerAPI interface {
	RecordSuccess(peer string)
	RecordFailure(peer string)
	GetScore(peer string) float64
}

// HealthChecker defines interface for node health monitoring
type HealthChecker interface {
	IsHealthy() bool
	GetUptime() time.Duration
	GetLastBlockTime() time.Time
}
