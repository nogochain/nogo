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
	"crypto/rand"
	"fmt"
	"log"
	"math"
	"net"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/utils"
)

// RetryStrategy defines retry configuration
type RetryStrategy struct {
	MaxRetries      int           // Maximum number of retry attempts
	InitialDelay    time.Duration // Initial delay before first retry
	MaxDelay        time.Duration // Maximum delay cap
	Multiplier      float64       // Exponential backoff multiplier
	Jitter          float64       // Random jitter factor (0-1)
	Timeout         time.Duration // Per-attempt timeout
	RetryableErrors []error       // Errors that should trigger retry
}

// DefaultRetryStrategy returns production-grade default retry strategy
// 3 retries with exponential backoff: 1s, 2s, 4s
// Timeout is set to 5 minutes to allow sufficient time for block downloads
func DefaultRetryStrategy() *RetryStrategy {
	return &RetryStrategy{
		MaxRetries:   3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		Timeout:      5 * time.Minute, // 5 minutes for block downloads
		RetryableErrors: []error{
			utils.ErrTimeout,
			utils.ErrConnectionClosed,
			utils.ErrBroadcastFailed,
			utils.ErrHTTPRequest,
			utils.ErrHTTPResponse,
		},
	}
}

// RetryResult contains retry operation results
type RetryResult struct {
	Success       bool
	Attempts      int
	TotalDuration time.Duration
	LastErr       error
	Errors        []error // All errors encountered
	FinalPeer     string  // Peer that succeeded (if any)
	SwitchedPeer  bool    // Whether peer was switched during retry
}

// RetryExecutor handles retry logic with peer switching
type RetryExecutor struct {
	mu                sync.RWMutex
	strategy          *RetryStrategy
	scorer            *AdvancedPeerScorer
	metrics           *RetryMetrics
	peerSwitchEnabled bool
	minScoreThreshold float64
}

// RetryMetrics tracks retry statistics
type RetryMetrics struct {
	mu                sync.RWMutex
	TotalRetries      uint64
	SuccessfulRetries uint64
	FailedRetries     uint64
	PeerSwitches      uint64
	TotalAttempts     uint64
	AvgAttempts       float64
	LastErrorTypes    map[string]uint64
}

// NewRetryExecutor creates a new retry executor
func NewRetryExecutor(strategy *RetryStrategy, scorer *AdvancedPeerScorer) *RetryExecutor {
	if strategy == nil {
		strategy = DefaultRetryStrategy()
	}

	return &RetryExecutor{
		strategy:          strategy,
		scorer:            scorer,
		peerSwitchEnabled: true,
		minScoreThreshold: 40.0,
		metrics: &RetryMetrics{
			LastErrorTypes: make(map[string]uint64),
		},
	}
}

// ExecuteWithRetry executes a function with retry logic
func (re *RetryExecutor) ExecuteWithRetry(
	ctx context.Context,
	operation func(context.Context, string) error,
	initialPeer string,
) *RetryResult {

	result := &RetryResult{
		Errors: make([]error, 0),
	}

	startTime := time.Now()
	currentPeer := initialPeer

	for attempt := 0; attempt <= re.strategy.MaxRetries; attempt++ {
		result.Attempts = attempt + 1

		// Check context cancellation
		select {
		case <-ctx.Done():
			result.LastErr = ctx.Err()
			return result
		default:
		}

		// Create timeout context for this attempt
		attemptCtx, cancel := context.WithTimeout(ctx, re.strategy.Timeout)
		
		// Execute operation
		err := operation(attemptCtx, currentPeer)
		
		cancel()

		if err == nil {
			// Success
			result.Success = true
			result.FinalPeer = currentPeer
			result.TotalDuration = time.Since(startTime)
			
			// Update metrics
			re.updateMetrics(true, result)
			
			// Record success with scorer
			if re.scorer != nil {
				latencyMs := result.TotalDuration.Milliseconds()
				re.scorer.RecordSuccess(currentPeer, latencyMs)
			}
			
			return result
		}

		// Record failure
		result.LastErr = err
		result.Errors = append(result.Errors, err)
		re.metrics.TotalAttempts++

		// Record failure with scorer
		if re.scorer != nil {
			re.scorer.RecordFailure(currentPeer)
		}

		// Check if error is retryable
		if !re.isRetryableError(err) {
			log.Printf("retry_executor: non-retryable error: %v", err)
			break
		}

		// Log retry attempt
		log.Printf("retry_executor: attempt %d/%d failed for peer %s: %v",
			attempt+1, re.strategy.MaxRetries+1, currentPeer, err)

		// Check if we should switch peers
		if re.peerSwitchEnabled && attempt < re.strategy.MaxRetries {
			newPeer := re.selectBestPeer(currentPeer)
			if newPeer != "" && newPeer != currentPeer {
				currentPeer = newPeer
				result.SwitchedPeer = true
				re.metrics.PeerSwitches++
				log.Printf("retry_executor: switched to peer %s (score=%.2f)",
					currentPeer, re.scorer.GetPeerScore(currentPeer))
			}
		}

		// Apply backoff delay before next retry
		if attempt < re.strategy.MaxRetries {
			delay := re.calculateBackoff(attempt)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				result.LastErr = ctx.Err()
				return result
			}
		}
	}

	// All retries exhausted
	result.TotalDuration = time.Since(startTime)
	re.updateMetrics(false, result)

	log.Printf("retry_executor: all %d attempts failed, last error: %v",
		result.Attempts, result.LastErr)

	return result
}

// calculateBackoff computes exponential backoff with jitter
// Security: Uses crypto/rand for jitter to prevent timing-based attacks
func (re *RetryExecutor) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: initialDelay * multiplier^attempt
	delay := float64(re.strategy.InitialDelay) * math.Pow(re.strategy.Multiplier, float64(attempt))
	
	// Cap at max delay
	if delay > float64(re.strategy.MaxDelay) {
		delay = float64(re.strategy.MaxDelay)
	}

	// Add jitter to prevent thundering herd
	// Security: crypto/rand instead of math/rand for unpredictable jitter
	jitterRange := delay * re.strategy.Jitter
	var jitter float64
	jitterBytes := make([]byte, 8)
	if _, err := rand.Read(jitterBytes); err != nil {
		// Fallback to zero jitter on crypto failure (degraded but safe)
		jitter = 0
	} else {
		// Convert random bytes to float64 in [0, 1)
		randVal := float64(uint64(jitterBytes[0])<<56|uint64(jitterBytes[1])<<48|
			uint64(jitterBytes[2])<<40|uint64(jitterBytes[3])<<32|
			uint64(jitterBytes[4])<<24|uint64(jitterBytes[5])<<16|
			uint64(jitterBytes[6])<<8|uint64(jitterBytes[7])) / float64(math.MaxUint64)
		jitter = (randVal * 2 * jitterRange) - jitterRange
	}
	delay += jitter

	// Ensure non-negative delay
	if delay < 0 {
		delay = float64(100 * time.Millisecond)
	}

	return time.Duration(delay)
}

// isRetryableError checks if error should trigger retry
func (re *RetryExecutor) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	
	// Check against retryable errors list
	for _, retryableErr := range re.strategy.RetryableErrors {
		if err == retryableErr || errStr == retryableErr.Error() {
			return true
		}
	}

	// Check for network-related errors
	if isNetworkError(err) {
		return true
	}

	return false
}

// isNetworkError identifies network-related errors
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for timeout
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	// Check for temporary network errors
	if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
		return true
	}

	// Check for connection errors
	errStr := err.Error()
	networkErrorKeywords := []string{
		"connection refused",
		"connection reset",
		"connection closed",
		"network is unreachable",
		"no route to host",
		"i/o timeout",
		"broken pipe",
	}

	for _, keyword := range networkErrorKeywords {
		if containsIgnoreCase(errStr, keyword) {
			return true
		}
	}

	return false
}

// containsIgnoreCase checks if string contains substring (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
		findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// selectBestPeer selects the best available peer
func (re *RetryExecutor) selectBestPeer(excludePeer string) string {
	if re.scorer == nil {
		return ""
	}

	// Get top scoring peers
	topPeers := re.scorer.GetTopPeersByScore(5)
	
	for _, peer := range topPeers {
		if peer == excludePeer {
			continue
		}

		// Check if peer meets minimum score threshold
		score := re.scorer.GetPeerScore(peer)
		if score >= re.minScoreThreshold {
			return peer
		}
	}

	return ""
}

// updateMetrics updates retry metrics
func (re *RetryExecutor) updateMetrics(success bool, result *RetryResult) {
	re.metrics.mu.Lock()
	defer re.metrics.mu.Unlock()

	re.metrics.TotalRetries++
	
	if success {
		re.metrics.SuccessfulRetries++
	} else {
		re.metrics.FailedRetries++
	}

	// Update average attempts
	total := float64(re.metrics.SuccessfulRetries + re.metrics.FailedRetries)
	re.metrics.AvgAttempts = (re.metrics.AvgAttempts*(total-1) + float64(result.Attempts)) / total

	// Track error types
	if result.LastErr != nil {
		errType := fmt.Sprintf("%T", result.LastErr)
		re.metrics.LastErrorTypes[errType]++
	}
}

// GetMetrics returns retry metrics
func (re *RetryExecutor) GetMetrics() map[string]interface{} {
	re.metrics.mu.RLock()
	defer re.metrics.mu.RUnlock()

	return map[string]interface{}{
		"total_retries":       re.metrics.TotalRetries,
		"successful_retries":  re.metrics.SuccessfulRetries,
		"failed_retries":      re.metrics.FailedRetries,
		"peer_switches":       re.metrics.PeerSwitches,
		"total_attempts":      re.metrics.TotalAttempts,
		"avg_attempts":        re.metrics.AvgAttempts,
		"success_rate":        re.calculateSuccessRate(),
		"last_error_types":    re.metrics.LastErrorTypes,
		"timestamp":           time.Now().UTC().Format(time.RFC3339),
	}
}

// calculateSuccessRate computes retry success rate
func (re *RetryExecutor) calculateSuccessRate() float64 {
	total := re.metrics.SuccessfulRetries + re.metrics.FailedRetries
	if total == 0 {
		return 0
	}
	return float64(re.metrics.SuccessfulRetries) / float64(total)
}

// SetPeerSwitchEnabled enables/disables peer switching
func (re *RetryExecutor) SetPeerSwitchEnabled(enabled bool) {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.peerSwitchEnabled = enabled
}

// SetMinScoreThreshold sets minimum peer score threshold
func (re *RetryExecutor) SetMinScoreThreshold(threshold float64) {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.minScoreThreshold = threshold
}

// ClassifyError categorizes error type
func ClassifyError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()

	// Network errors
	if isNetworkError(err) {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return "network_timeout"
		}
		return "network_error"
	}

	// Validation errors
	validationKeywords := []string{"invalid", "validation", "verification"}
	for _, keyword := range validationKeywords {
		if containsIgnoreCase(errStr, keyword) {
			return "validation_error"
		}
	}

	// Timeout errors
	timeoutKeywords := []string{"timeout", "timed out", "deadline exceeded"}
	for _, keyword := range timeoutKeywords {
		if containsIgnoreCase(errStr, keyword) {
			return "timeout"
		}
	}

	// Not found errors
	if containsIgnoreCase(errStr, "not found") {
		return "not_found"
	}

	// Unknown error
	return "unknown"
}

// RetryDecorator creates a retry wrapper for any function
// Note: Uses interface{} for generic-like behavior due to Go 1.21 compatibility
func RetryDecorator(
	executor *RetryExecutor,
	fn func(context.Context, string) (interface{}, error),
	initialPeer string,
) func(context.Context) (interface{}, *RetryResult, error) {
	
	return func(ctx context.Context) (interface{}, *RetryResult, error) {
		result := executor.ExecuteWithRetry(ctx, func(ctx context.Context, peer string) error {
			_, err := fn(ctx, peer)
			return err
		}, initialPeer)

		if !result.Success {
			return nil, result, result.LastErr
		}

		// Call function one more time to get result
		res, err := fn(ctx, result.FinalPeer)
		if err != nil {
			return nil, result, err
		}

		return res, result, nil
	}
}
