// Package main provides feature flags and capability management for the NogoChain node.
//
// This file implements a comprehensive feature flag system including:
//   - Predefined feature flags for all node capabilities
//   - Runtime feature enable/disable with callbacks
//   - Feature stability tracking (stable vs experimental)
//   - Feature import/export for configuration management
//   - Feature statistics and monitoring
//
// Feature flags allow gradual rollout and A/B testing of new functionality.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// FeatureFlag represents a feature flag
type FeatureFlag struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	Stable      bool      `json:"stable"`
	Since       string    `json:"since"`
	Deprecated  string    `json:"deprecated,omitempty"`
}

// FeatureManager manages feature flags
type FeatureManager struct {
	mu       sync.RWMutex
	features map[string]*FeatureFlag
	callbacks map[string][]func(bool)
}

// Feature names
const (
	FeatureP2Pv2           = "p2p_v2"
	FeatureFastSync        = "fast_sync"
	FeatureWarpSync        = "warp_sync"
	FeatureLightClient     = "light_client"
	FeatureArchiveNode     = "archive_node"
	FeaturePruning         = "pruning"
	FeatureMempoolIndex    = "mempool_index"
	FeatureTxIndex         = "tx_index"
	FeatureAddrIndex       = "addr_index"
	FeatureWSAPI           = "ws_api"
	FeatureHTTPAPI         = "http_api"
	FeatureMetrics         = "metrics"
	FeatureMining          = "mining"
	FeatureAutoPeering     = "auto_peering"
	FeatureUPNP            = "upnp"
	FeatureNATPMP          = "natpmp"
	FeatureDNSDiscovery    = "dns_discovery"
	FeatureMDNSDiscovery   = "mdns_discovery"
	FeaturePeerScoring     = "peer_scoring"
	FeatureRateLimiting    = "rate_limiting"
	FeatureDDoSProtection  = "ddos_protection"
	FeatureCompression     = "compression"
	FeatureEncryption      = "encryption"
	FeatureBatchVerify     = "batch_verify"
	FeatureParallelVerify  = "parallel_verify"
	FeatureCheckpointSync  = "checkpoint_sync"
	FeatureStateSync       = "state_sync"
	FeatureSnapshotSync    = "snapshot_sync"
	FeatureCompactBlocks   = "compact_blocks"
	FeatureCompactFilters  = "compact_filters"
	FeatureBloomFilters    = "bloom_filters"
	FeatureTxLookup        = "tx_lookup"
	FeatureLogsIndex       = "logs_index"
	FeatureDebugAPI        = "debug_api"
	FeatureAdminAPI        = "admin_api"
	FeaturePersonalAPI     = "personal_api"
	FeatureNetAPI          = "net_api"
	FeatureWeb3API         = "web3_api"
	FeatureEthAPI          = "eth_api"
	FeatureTxPoolAPI       = "txpool_api"
	FeatureMinerAPI        = "miner_api"
	FeatureCliqueAPI       = "clique_api"
)

// DefaultFeatures returns the default feature configuration
func DefaultFeatures() map[string]*FeatureFlag {
	return map[string]*FeatureFlag{
		FeatureP2Pv2: {
			Name:        FeatureP2Pv2,
			Description: "P2P protocol version 2 with improved efficiency",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureFastSync: {
			Name:        FeatureFastSync,
			Description: "Fast synchronization mode",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureWarpSync: {
			Name:        FeatureWarpSync,
			Description: "Warp synchronization for instant sync",
			Enabled:     false,
			Stable:      false,
			Since:       "1.0.0",
		},
		FeatureLightClient: {
			Name:        FeatureLightClient,
			Description: "Light client mode with reduced storage",
			Enabled:     false,
			Stable:      false,
			Since:       "1.0.0",
		},
		FeatureArchiveNode: {
			Name:        FeatureArchiveNode,
			Description: "Archive node mode with full history",
			Enabled:     false,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeaturePruning: {
			Name:        FeaturePruning,
			Description: "State pruning to reduce disk usage",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureMempoolIndex: {
			Name:        FeatureMempoolIndex,
			Description: "Mempool indexing for faster lookups",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureTxIndex: {
			Name:        FeatureTxIndex,
			Description: "Transaction index for historical queries",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureAddrIndex: {
			Name:        FeatureAddrIndex,
			Description: "Address index for balance queries",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureWSAPI: {
			Name:        FeatureWSAPI,
			Description: "WebSocket API for real-time updates",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureHTTPAPI: {
			Name:        FeatureHTTPAPI,
			Description: "HTTP JSON-RPC API",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureMetrics: {
			Name:        FeatureMetrics,
			Description: "Prometheus metrics export",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureMining: {
			Name:        FeatureMining,
			Description: "Block mining support",
			Enabled:     false,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureAutoPeering: {
			Name:        FeatureAutoPeering,
			Description: "Automatic peer discovery and management",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureUPNP: {
			Name:        FeatureUPNP,
			Description: "UPnP port forwarding",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeaturePeerScoring: {
			Name:        FeaturePeerScoring,
			Description: "Peer quality scoring system",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureRateLimiting: {
			Name:        FeatureRateLimiting,
			Description: "API rate limiting",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureDDoSProtection: {
			Name:        FeatureDDoSProtection,
			Description: "DDoS attack protection",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureCompression: {
			Name:        FeatureCompression,
			Description: "Message compression for P2P",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureEncryption: {
			Name:        FeatureEncryption,
			Description: "P2P message encryption",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureBatchVerify: {
			Name:        FeatureBatchVerify,
			Description: "Batch signature verification",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureParallelVerify: {
			Name:        FeatureParallelVerify,
			Description: "Parallel transaction verification",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureCheckpointSync: {
			Name:        FeatureCheckpointSync,
			Description: "Checkpoint-based synchronization",
			Enabled:     true,
			Stable:      true,
			Since:       "1.0.0",
		},
		FeatureCompactBlocks: {
			Name:        FeatureCompactBlocks,
			Description: "Compact block relay protocol",
			Enabled:     false,
			Stable:      false,
			Since:       "1.0.0",
		},
		FeatureDebugAPI: {
			Name:        FeatureDebugAPI,
			Description: "Debug API for development",
			Enabled:     false,
			Stable:      false,
			Since:       "1.0.0",
		},
		FeatureAdminAPI: {
			Name:        FeatureAdminAPI,
			Description: "Admin API for node management",
			Enabled:     false,
			Stable:      true,
			Since:       "1.0.0",
		},
	}
}

// NewFeatureManager creates a new feature manager
func NewFeatureManager() *FeatureManager {
	return &FeatureManager{
		features:  DefaultFeatures(),
		callbacks: make(map[string][]func(bool)),
	}
}

// IsEnabled checks if a feature is enabled
func (fm *FeatureManager) IsEnabled(name string) bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	feature, exists := fm.features[name]
	if !exists {
		return false
	}

	return feature.Enabled
}

// Enable enables a feature
func (fm *FeatureManager) Enable(name string) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	feature, exists := fm.features[name]
	if !exists {
		return fmt.Errorf("feature %s does not exist", name)
	}

	if feature.Deprecated != "" {
		return fmt.Errorf("feature %s is deprecated: %s", name, feature.Deprecated)
	}

	if !feature.Enabled {
		feature.Enabled = true
		fm.notifyCallbacks(name, true)
	}

	return nil
}

// Disable disables a feature
func (fm *FeatureManager) Disable(name string) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	feature, exists := fm.features[name]
	if !exists {
		return fmt.Errorf("feature %s does not exist", name)
	}

	if feature.Enabled {
		feature.Enabled = false
		fm.notifyCallbacks(name, false)
	}

	return nil
}

// Toggle toggles a feature
func (fm *FeatureManager) Toggle(name string) (bool, error) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	feature, exists := fm.features[name]
	if !exists {
		return false, fmt.Errorf("feature %s does not exist", name)
	}

	feature.Enabled = !feature.Enabled
	fm.notifyCallbacks(name, feature.Enabled)

	return feature.Enabled, nil
}

// List lists all features
func (fm *FeatureManager) List() []*FeatureFlag {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	features := make([]*FeatureFlag, 0, len(fm.features))
	for _, feature := range fm.features {
		features = append(features, feature)
	}

	return features
}

// Get gets a feature by name
func (fm *FeatureManager) Get(name string) *FeatureFlag {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	return fm.features[name]
}

// Subscribe registers a callback for feature changes
func (fm *FeatureManager) Subscribe(name string, callback func(bool)) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	fm.callbacks[name] = append(fm.callbacks[name], callback)
}

// Unsubscribe removes a callback
func (fm *FeatureManager) Unsubscribe(name string, callback func(bool)) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	callbacks := fm.callbacks[name]
	for i, cb := range callbacks {
		// Compare function pointers
		if fmt.Sprintf("%p", cb) == fmt.Sprintf("%p", callback) {
			fm.callbacks[name] = append(callbacks[:i], callbacks[i+1:]...)
			break
		}
	}
}

// notifyCallbacks notifies all callbacks for a feature change
func (fm *FeatureManager) notifyCallbacks(name string, enabled bool) {
	for _, callback := range fm.callbacks[name] {
		go callback(enabled)
	}
}

// Export exports features to a file
func (fm *FeatureManager) Export(path string) error {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	data, err := json.MarshalIndent(fm.features, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal features: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write features file: %w", err)
	}

	return nil
}

// Import imports features from a file
func (fm *FeatureManager) Import(path string) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read features file: %w", err)
	}

	var features map[string]*FeatureFlag
	if err := json.Unmarshal(data, &features); err != nil {
		return fmt.Errorf("failed to unmarshal features: %w", err)
	}

	for name, feature := range features {
		if existing, exists := fm.features[name]; exists {
			existing.Enabled = feature.Enabled
		}
	}

	return nil
}

// GetEnabledFeatures returns a list of enabled feature names
func (fm *FeatureManager) GetEnabledFeatures() []string {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	enabled := make([]string, 0)
	for name, feature := range fm.features {
		if feature.Enabled {
			enabled = append(enabled, name)
		}
	}

	return enabled
}

// GetStableFeatures returns a list of stable feature names
func (fm *FeatureManager) GetStableFeatures() []string {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	stable := make([]string, 0)
	for name, feature := range fm.features {
		if feature.Stable {
			stable = append(stable, name)
		}
	}

	return stable
}

// GetExperimentalFeatures returns a list of experimental feature names
func (fm *FeatureManager) GetExperimentalFeatures() []string {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	experimental := make([]string, 0)
	for name, feature := range fm.features {
		if !feature.Stable {
			experimental = append(experimental, name)
		}
	}

	return experimental
}

// Stats returns feature statistics
func (fm *FeatureManager) Stats() map[string]interface{} {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	total := len(fm.features)
	enabled := 0
	stable := 0
	experimental := 0

	for _, feature := range fm.features {
		if feature.Enabled {
			enabled++
		}
		if feature.Stable {
			stable++
		} else {
			experimental++
		}
	}

	return map[string]interface{}{
		"total":        total,
		"enabled":      enabled,
		"disabled":     total - enabled,
		"stable":       stable,
		"experimental": experimental,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	}
}
