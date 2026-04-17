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
	"time"

	"github.com/nogochain/nogo/config"
)

// =============================================================================
// P2P NETWORK CONSTANTS
// =============================================================================

const (
	// DefaultMaxPeers is the default maximum number of P2P peers
	DefaultMaxPeers = config.DefaultP2PMaxPeers

	// DefaultP2PMaxConnections is the default maximum concurrent P2P connections
	DefaultP2PMaxConnections = 100

	// DefaultP2PMaxMessageBytes is the default maximum P2P message size (16MB)
	// Increased to handle batch block responses (up to 50 blocks with transactions)
	DefaultP2PMaxMessageBytes = 16 << 20

	// DefaultP2PMaxAddrReturn is the default maximum addresses to return in getaddr
	DefaultP2PMaxAddrReturn = 100

	// ProtocolVersion is the current P2P protocol version
	ProtocolVersion = 1

	// MinProtocolVersion is the minimum supported protocol version
	MinProtocolVersion = 1
)

// =============================================================================
// SYNC CONSTANTS
// =============================================================================

const (
	// SyncBatchSize is the default number of blocks to sync in one batch
	// Optimized for Bitcoin-style fast sync
	SyncBatchSize = 500

	// MaxSyncRange is the maximum number of blocks to sync in one request
	MaxSyncRange = 1000

	// SyncWorkers is the default number of parallel sync workers
	SyncWorkers = 16

	// SyncTimeout is the default timeout for sync operations
	SyncTimeout = 30 * time.Second

	// BlockPropagationDelayMs is the delay to wait for block propagation
	// Optimized for fast consensus: reduced from 3000ms to 100ms
	// Bitcoin-style: minimal delay, relies on work comparison instead of waiting
	BlockPropagationDelayMs = 100 // 100ms - sufficient for network propagation

	// HeaderSyncBatchSize is the number of headers to sync in one batch
	HeaderSyncBatchSize = 1000

	// MaxAncestorDepth is the maximum depth for ancestor block fetching
	MaxAncestorDepth = 256
)

// =============================================================================
// PEER DISCOVERY CONSTANTS
// =============================================================================

const (
	// PeerDiscoveryIntervalSec is the interval between peer discovery rounds
	PeerDiscoveryIntervalSec = 30 // 30 seconds

	// PeerDiscoveryTimeoutSec is the timeout for peer discovery requests
	PeerDiscoveryTimeoutSec = 10 // 10 seconds

	// MaxPeersDiscoverPerRound is the maximum peers to query per discovery round
	MaxPeersDiscoverPerRound = 5

	// PeerExpiryDuration is the duration after which a peer is considered stale
	PeerExpiryDuration = 24 * time.Hour

	// CleanupInterval is the interval at which stale peers are cleaned up
	CleanupInterval = 1 * time.Hour

	// MaxPeerFailures is the maximum consecutive failures before peer removal
	MaxPeerFailures = 10

	// MaxConsecutiveFailures is the threshold for removing a peer after consecutive failures
	// Aligned with MaxPeerFailures for consistency
	MaxConsecutiveFailures = MaxPeerFailures
)

// =============================================================================
// ORPHAN POOL CONSTANTS
// =============================================================================

const (
	// DefaultOrphanPoolSize is the default maximum number of orphan blocks
	DefaultOrphanPoolSize = 100

	// DefaultOrphanTTL is the default time-to-live for orphan blocks
	DefaultOrphanTTL = 24 * time.Hour
)

// =============================================================================
// INSTANT VALIDATOR CONSTANTS
// =============================================================================

const (
	// DefaultValidatorQueueSize is the default size of validation queue
	DefaultValidatorQueueSize = 1000

	// DefaultValidationTimeout is the default timeout for block validation
	DefaultValidationTimeout = 10 * time.Second
)

// =============================================================================
// RATE LIMITING CONSTANTS
// =============================================================================

const (
	// DefaultRateLimitWindow is the default rate limit window duration
	DefaultRateLimitWindow = time.Minute

	// DefaultRateLimitMaxRequests is the default maximum requests per window
	DefaultRateLimitMaxRequests = 100

	// DefaultRateLimitBurst is the default burst size for rate limiting
	DefaultRateLimitBurst = 20
)

// =============================================================================
// NETWORK TIMING CONSTANTS
// =============================================================================

const (
	// ConnectionTimeout is the default timeout for establishing connections
	ConnectionTimeout = 10 * time.Second

	// ReadTimeout is the default timeout for read operations
	ReadTimeout = 30 * time.Second

	// WriteTimeout is the default timeout for write operations
	WriteTimeout = 30 * time.Second

	// HandshakeTimeout is the default timeout for P2P handshake
	HandshakeTimeout = 15 * time.Second

	// KeepAliveInterval is the interval for keep-alive messages
	KeepAliveInterval = 5 * time.Minute

	// PingTimeout is the timeout for ping/pong responses
	PingTimeout = 30 * time.Second
)

// =============================================================================
// SECURITY CONSTANTS
// =============================================================================

const (
	// MaxMessageSize is the absolute maximum P2P message size
	MaxMessageSize = 16 << 20 // 16MB

	// ChallengeExpiry is the time after which auth challenges expire
	ChallengeExpiry = 5 * time.Minute

	// MinChallengeLength is the minimum length for auth challenges
	MinChallengeLength = 32

	// MaxChallengeLength is the maximum length for auth challenges
	MaxChallengeLength = 256
)

// =============================================================================
// PEER SCORING CONSTANTS (imported from config)
// =============================================================================

const (
	// DefaultPeerScorerMinScore is the minimum score to keep a peer
	DefaultPeerScorerMinScore = config.DefaultPeerScorerMinScore

	// DefaultPeerScorerDecayFactor is the decay factor for inactive peers
	DefaultPeerScorerDecayFactor = config.DefaultPeerScorerDecayFactor

	// DefaultPeerScorerTrustWeight is the weight for trust in scoring
	DefaultPeerScorerTrustWeight = config.DefaultPeerScorerTrustWeight

	// DefaultPeerScorerLatencyWeight is the weight for latency in scoring
	DefaultPeerScorerLatencyWeight = config.DefaultPeerScorerLatencyWeight

	// DefaultPeerScorerSuccessWeight is the weight for success rate in scoring
	DefaultPeerScorerSuccessWeight = config.DefaultPeerScorerSuccessWeight

	// DefaultPeerScorerMaxConsecutiveFails is max failures before ban
	DefaultPeerScorerMaxConsecutiveFails = config.DefaultPeerScorerMaxConsecutiveFails

	// DefaultPeerScorerTrustDecayRate is the trust decay on failure
	DefaultPeerScorerTrustDecayRate = config.DefaultPeerScorerTrustDecayRate

	// DefaultPeerScorerTrustGrowthRate is the trust growth on success
	DefaultPeerScorerTrustGrowthRate = config.DefaultPeerScorerTrustGrowthRate

	// DefaultPeerScorerMinimumSamples is min interactions before scoring
	DefaultPeerScorerMinimumSamples = config.DefaultPeerScorerMinimumSamples

	// DefaultPeerScorerHourlyDecayFactor is the hourly decay factor
	DefaultPeerScorerHourlyDecayFactor = config.DefaultPeerScorerHourlyDecayFactor
)
