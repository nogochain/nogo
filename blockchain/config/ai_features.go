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
	"errors"
	"sync"
	"time"
)

// AIFeatures defines AI-specific feature configuration
type AIFeatures struct {
	mu sync.RWMutex

	// AuditorEnabled enables AI-powered block auditing
	AuditorEnabled bool `json:"auditorEnabled"`

	// AuditorModelPath is the path to the AI auditor model
	AuditorModelPath string `json:"auditorModelPath"`

	// AuditorThreshold is the confidence threshold for flagging anomalies (0.0-1.0)
	AuditorThreshold float64 `json:"auditorThreshold"`

	// AnomalyDetectionEnabled enables network anomaly detection
	AnomalyDetectionEnabled bool `json:"anomalyDetectionEnabled"`

	// AnomalyWindowSize is the number of blocks to analyze for anomalies
	AnomalyWindowSize int `json:"anomalyWindowSize"`

	// AnomalyThresholdStd is the standard deviation threshold for anomaly detection
	AnomalyThresholdStd float64 `json:"anomalyThresholdStd"`

	// SpamDetectionEnabled enables P2P spam detection
	SpamDetectionEnabled bool `json:"spamDetectionEnabled"`

	// SpamWindowDuration is the time window for spam detection
	SpamWindowDuration time.Duration `json:"spamWindowDuration"`

	// SpamMaxRequests is the maximum requests per window before flagging as spam
	SpamMaxRequests int `json:"spamMaxRequests"`

	// SpamMaxBytes is the maximum bytes per window before flagging as spam
	SpamMaxBytes int64 `json:"spamMaxBytes"`

	// FeeEstimationEnabled enables AI-powered fee estimation
	FeeEstimationEnabled bool `json:"feeEstimationEnabled"`

	// FeeEstimationWindowSize is the number of blocks to analyze for fee estimation
	FeeEstimationWindowSize int `json:"feeEstimationWindowSize"`

	// WalletAnalysisEnabled enables wallet behavior analysis
	WalletAnalysisEnabled bool `json:"walletAnalysisEnabled"`

	// WalletAnalysisWindowSize is the number of transactions to analyze
	WalletAnalysisWindowSize int `json:"walletAnalysisWindowSize"`
}

// DefaultAIFeatures returns AI features with default values
func DefaultAIFeatures() *AIFeatures {
	return &AIFeatures{
		AuditorEnabled:           false,
		AuditorModelPath:         "",
		AuditorThreshold:         0.8,
		AnomalyDetectionEnabled:  true,
		AnomalyWindowSize:        100,
		AnomalyThresholdStd:      3.0,
		SpamDetectionEnabled:     true,
		SpamWindowDuration:       time.Minute,
		SpamMaxRequests:          100,
		SpamMaxBytes:             10 * 1024 * 1024,
		FeeEstimationEnabled:     true,
		FeeEstimationWindowSize:  100,
		WalletAnalysisEnabled:    true,
		WalletAnalysisWindowSize: 1000,
	}
}

// Validate validates AI feature configuration
func (a *AIFeatures) Validate() error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.AuditorThreshold < 0 || a.AuditorThreshold > 1 {
		return ErrInvalidAuditorThreshold
	}

	if a.AnomalyWindowSize <= 0 {
		return ErrInvalidAnomalyWindowSize
	}

	if a.AnomalyThresholdStd <= 0 {
		return ErrInvalidAnomalyThreshold
	}

	if a.SpamWindowDuration <= 0 {
		return ErrInvalidSpamWindow
	}

	if a.SpamMaxRequests <= 0 {
		return ErrInvalidSpamMaxRequests
	}

	if a.SpamMaxBytes <= 0 {
		return ErrInvalidSpamMaxBytes
	}

	if a.FeeEstimationWindowSize <= 0 {
		return ErrInvalidFeeWindowSize
	}

	if a.WalletAnalysisWindowSize <= 0 {
		return ErrInvalidWalletWindowSize
	}

	return nil
}

// IsAuditorEnabled returns true if AI auditor is enabled
func (a *AIFeatures) IsAuditorEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.AuditorEnabled
}

// IsAnomalyDetectionEnabled returns true if anomaly detection is enabled
func (a *AIFeatures) IsAnomalyDetectionEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.AnomalyDetectionEnabled
}

// IsSpamDetectionEnabled returns true if spam detection is enabled
func (a *AIFeatures) IsSpamDetectionEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.SpamDetectionEnabled
}

// IsFeeEstimationEnabled returns true if fee estimation is enabled
func (a *AIFeatures) IsFeeEstimationEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.FeeEstimationEnabled
}

// IsWalletAnalysisEnabled returns true if wallet analysis is enabled
func (a *AIFeatures) IsWalletAnalysisEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.WalletAnalysisEnabled
}

// GetAuditorThreshold returns the auditor confidence threshold
func (a *AIFeatures) GetAuditorThreshold() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.AuditorThreshold
}

// GetAnomalyWindowSize returns the anomaly detection window size
func (a *AIFeatures) GetAnomalyWindowSize() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.AnomalyWindowSize
}

// GetAnomalyThresholdStd returns the anomaly detection threshold
func (a *AIFeatures) GetAnomalyThresholdStd() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.AnomalyThresholdStd
}

// GetSpamConfig returns spam detection configuration
func (a *AIFeatures) GetSpamConfig() (time.Duration, int, int64) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.SpamWindowDuration, a.SpamMaxRequests, a.SpamMaxBytes
}

// GetFeeEstimationWindowSize returns the fee estimation window size
func (a *AIFeatures) GetFeeEstimationWindowSize() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.FeeEstimationWindowSize
}

// GetWalletAnalysisWindowSize returns the wallet analysis window size
func (a *AIFeatures) GetWalletAnalysisWindowSize() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.WalletAnalysisWindowSize
}

// Error definitions for AI features validation
var (
	ErrInvalidAuditorThreshold  = errors.New("auditor threshold must be between 0 and 1")
	ErrInvalidAnomalyWindowSize = errors.New("anomaly window size must be > 0")
	ErrInvalidAnomalyThreshold  = errors.New("anomaly threshold must be > 0")
	ErrInvalidSpamWindow        = errors.New("spam window duration must be > 0")
	ErrInvalidSpamMaxRequests   = errors.New("spam max requests must be > 0")
	ErrInvalidSpamMaxBytes      = errors.New("spam max bytes must be > 0")
	ErrInvalidFeeWindowSize     = errors.New("fee estimation window size must be > 0")
	ErrInvalidWalletWindowSize  = errors.New("wallet analysis window size must be > 0")
)
