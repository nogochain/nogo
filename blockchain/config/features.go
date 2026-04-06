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
	"sync"
	"time"
)

// FeatureFlags defines all feature flags for the blockchain
type FeatureFlags struct {
	mu sync.RWMutex

	// EnableAIAuditor enables AI-powered block auditing
	EnableAIAuditor bool `json:"enableAIAuditor"`

	// EnableDNSRegistry enables decentralized DNS registry
	EnableDNSRegistry bool `json:"enableDNSRegistry"`

	// EnableGovernance enables on-chain governance
	EnableGovernance bool `json:"enableGovernance"`

	// EnablePriceOracle enables price oracle feeds
	EnablePriceOracle bool `json:"enablePriceOracle"`

	// EnableSocialRecovery enables social recovery for wallets
	EnableSocialRecovery bool `json:"enableSocialRecovery"`

	// EnableAnomalyDetection enables network anomaly detection
	EnableAnomalyDetection bool `json:"enableAnomalyDetection"`

	// EnableSpamDetection enables P2P spam detection
	EnableSpamDetection bool `json:"enableSpamDetection"`

	// EnableFeeEstimation enables dynamic fee estimation
	EnableFeeEstimation bool `json:"enableFeeEstimation"`

	// EnableNodeHealth enables node health monitoring
	EnableNodeHealth bool `json:"enableNodeHealth"`

	// EnableWalletAnalysis enables wallet behavior analysis
	EnableWalletAnalysis bool `json:"enableWalletAnalysis"`
}

// FeatureManager manages runtime feature flag toggling
type FeatureManager struct {
	mu         sync.RWMutex
	features   FeatureFlags
	callbacks  map[string][]func(bool)
	enabledAt  map[string]time.Time
	disabledAt map[string]time.Time
}

// NewFeatureManager creates a new feature manager instance
// Production-grade: properly initializes feature flags without copying locks
func NewFeatureManager() *FeatureManager {
	fm := &FeatureManager{
		features:   FeatureFlags{},
		callbacks:  make(map[string][]func(bool)),
		enabledAt:  make(map[string]time.Time),
		disabledAt: make(map[string]time.Time),
	}
	return fm
}

// GetFeatures returns current feature flags
func (m *FeatureManager) GetFeatures() *FeatureFlags {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return &m.features
}

// IsEnabled checks if a specific feature is enabled
func (m *FeatureManager) IsEnabled(feature string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isFeatureEnabled(feature)
}

// isFeatureEnabled checks if feature is enabled (internal, no lock)
func (m *FeatureManager) isFeatureEnabled(feature string) bool {
	switch feature {
	case "ai_auditor":
		return m.features.EnableAIAuditor
	case "dns_registry":
		return m.features.EnableDNSRegistry
	case "governance":
		return m.features.EnableGovernance
	case "price_oracle":
		return m.features.EnablePriceOracle
	case "social_recovery":
		return m.features.EnableSocialRecovery
	case "anomaly_detection":
		return m.features.EnableAnomalyDetection
	case "spam_detection":
		return m.features.EnableSpamDetection
	case "fee_estimation":
		return m.features.EnableFeeEstimation
	case "node_health":
		return m.features.EnableNodeHealth
	case "wallet_analysis":
		return m.features.EnableWalletAnalysis
	default:
		return false
	}
}

// Enable enables a feature at runtime
func (m *FeatureManager) Enable(feature string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isFeatureEnabled(feature) {
		return nil
	}

	m.setFeature(feature, true)
	m.enabledAt[feature] = time.Now()
	m.notifyCallbacks(feature, true)

	return nil
}

// Disable disables a feature at runtime
func (m *FeatureManager) Disable(feature string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isFeatureEnabled(feature) {
		return nil
	}

	m.setFeature(feature, false)
	m.disabledAt[feature] = time.Now()
	m.notifyCallbacks(feature, false)

	return nil
}

// Toggle toggles a feature at runtime
func (m *FeatureManager) Toggle(feature string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	enabled := !m.isFeatureEnabled(feature)
	m.setFeature(feature, enabled)

	if enabled {
		m.enabledAt[feature] = time.Now()
	} else {
		m.disabledAt[feature] = time.Now()
	}

	m.notifyCallbacks(feature, enabled)
	return nil
}

// setFeature sets a feature flag (internal, no lock)
func (m *FeatureManager) setFeature(feature string, enabled bool) {
	switch feature {
	case "ai_auditor":
		m.features.EnableAIAuditor = enabled
	case "dns_registry":
		m.features.EnableDNSRegistry = enabled
	case "governance":
		m.features.EnableGovernance = enabled
	case "price_oracle":
		m.features.EnablePriceOracle = enabled
	case "social_recovery":
		m.features.EnableSocialRecovery = enabled
	case "anomaly_detection":
		m.features.EnableAnomalyDetection = enabled
	case "spam_detection":
		m.features.EnableSpamDetection = enabled
	case "fee_estimation":
		m.features.EnableFeeEstimation = enabled
	case "node_health":
		m.features.EnableNodeHealth = enabled
	case "wallet_analysis":
		m.features.EnableWalletAnalysis = enabled
	}
}

// RegisterCallback registers a callback for feature changes
func (m *FeatureManager) RegisterCallback(feature string, callback func(bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks[feature] = append(m.callbacks[feature], callback)
}

// notifyCallbacks notifies all registered callbacks
func (m *FeatureManager) notifyCallbacks(feature string, enabled bool) {
	for _, cb := range m.callbacks[feature] {
		go cb(enabled)
	}
}

// GetEnabledAt returns when a feature was last enabled
func (m *FeatureManager) GetEnabledAt(feature string) (time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.enabledAt[feature]
	return t, ok
}

// GetDisabledAt returns when a feature was last disabled
func (m *FeatureManager) GetDisabledAt(feature string) (time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.disabledAt[feature]
	return t, ok
}

// GetFeatureStatus returns detailed status for a feature
func (m *FeatureManager) GetFeatureStatus(feature string) map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	enabled := m.isFeatureEnabled(feature)
	status := map[string]interface{}{
		"feature": feature,
		"enabled": enabled,
	}

	if enabled {
		if t, ok := m.enabledAt[feature]; ok {
			status["enabledAt"] = t
		}
	} else {
		if t, ok := m.disabledAt[feature]; ok {
			status["disabledAt"] = t
		}
	}

	return status
}

// ListFeatures returns a list of all features with their status
func (m *FeatureManager) ListFeatures() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]bool{
		"ai_auditor":        m.features.EnableAIAuditor,
		"dns_registry":      m.features.EnableDNSRegistry,
		"governance":        m.features.EnableGovernance,
		"price_oracle":      m.features.EnablePriceOracle,
		"social_recovery":   m.features.EnableSocialRecovery,
		"anomaly_detection": m.features.EnableAnomalyDetection,
		"spam_detection":    m.features.EnableSpamDetection,
		"fee_estimation":    m.features.EnableFeeEstimation,
		"node_health":       m.features.EnableNodeHealth,
		"wallet_analysis":   m.features.EnableWalletAnalysis,
	}
}
