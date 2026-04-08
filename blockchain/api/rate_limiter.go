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

package api

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// =============================================================================
// Configuration Constants
// =============================================================================
const (
	DefaultRPS               = 10
	DefaultBurst             = 20
	DefaultAPIKeyMultiplier  = 5.0
	DefaultTTL               = 10 * time.Minute
	DefaultCleanupInterval   = 1 * time.Minute
	MaxTokensPerRequest      = 1000
	MinRPS                   = 1
	MaxRPS                   = 10000
	MinBurst                 = 1
	MaxBurst                 = 100000
	MinAPIKeyMultiplier      = 1.0
	MaxAPIKeyMultiplier      = 100.0
)

// =============================================================================
// Prometheus Metrics
// =============================================================================
var (
	rateLimitRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nogo_rate_limit_requests_total",
			Help: "Total number of requests processed by rate limiter",
		},
		[]string{"endpoint", "result"}, // result: allowed, denied, bypassed
	)

	rateLimitTokensRemaining = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nogo_rate_limit_tokens_remaining",
			Help: "Current number of tokens remaining",
		},
		[]string{"endpoint", "identifier"},
	)

	rateLimitCurrentRPS = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nogo_rate_limit_current_rps",
			Help: "Current requests per second limit",
		},
		[]string{"endpoint", "identifier"},
	)

	rateLimitBypassesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "nogo_rate_limit_bypasses_total",
			Help: "Total number of rate limit bypasses (API key)",
		},
	)

	rateLimitEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nogo_rate_limit_events",
			Help: "Rate limiting events by type and reason",
		},
		[]string{"type", "reason", "endpoint"},
	)

	activeRateLimiters = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "nogo_active_rate_limiters",
			Help: "Number of active rate limiters in memory",
		},
	)
)

// RegisterRateLimitMetrics registers rate limiting metrics with Prometheus
func RegisterRateLimitMetrics() {
	prometheus.MustRegister(
		rateLimitRequestsTotal,
		rateLimitTokensRemaining,
		rateLimitCurrentRPS,
		rateLimitBypassesTotal,
		rateLimitEvents,
		activeRateLimiters,
	)
}

// =============================================================================
// Rate Limit Configuration Types
// =============================================================================

// EndpointRateLimitConfig represents rate limit configuration for a single endpoint
type EndpointRateLimitConfig struct {
	RequestsPerSecond float64 `json:"requests_per_second"`
	Burst             int     `json:"burst"`
}

// RateLimitConfig represents the complete rate limit configuration
type RateLimitConfig struct {
	Default           *EndpointRateLimitConfig            `json:"default"`
	Endpoints         map[string]*EndpointRateLimitConfig `json:"endpoints"`
	APIKeyMultiplier  float64                             `json:"api_key_multiplier"`
	Enabled           bool                                `json:"enabled"`
	ByIP              bool                                `json:"by_ip"`
	ByUser            bool                                `json:"by_user"`
	TrustProxy        bool                                `json:"trust_proxy"`
	CleanupInterval   time.Duration                       `json:"cleanup_interval"`
	StorageType       string                              `json:"storage_type"` // "memory" or "redis"
	RedisAddr         string                              `json:"redis_addr"`
	RedisPassword     string                              `json:"redis_password"`
	RedisDB           int                                 `json:"redis_db"`
}

// DefaultRateLimitConfig returns default rate limit configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: DefaultRPS,
			Burst:             DefaultBurst,
		},
		Endpoints:        make(map[string]*EndpointRateLimitConfig),
		APIKeyMultiplier: DefaultAPIKeyMultiplier,
		Enabled:          false,
		ByIP:             true,
		ByUser:           false,
		CleanupInterval:  DefaultCleanupInterval,
		StorageType:      "memory",
	}
}

// Validate validates the rate limit configuration
func (c *RateLimitConfig) Validate() error {
	if c.Default != nil {
		if c.Default.RequestsPerSecond < MinRPS || c.Default.RequestsPerSecond > MaxRPS {
			return fmt.Errorf("default RPS must be between %d and %d", MinRPS, MaxRPS)
		}
		if c.Default.Burst < MinBurst || c.Default.Burst > MaxBurst {
			return fmt.Errorf("default burst must be between %d and %d", MinBurst, MaxBurst)
		}
	}

	if c.APIKeyMultiplier < MinAPIKeyMultiplier || c.APIKeyMultiplier > MaxAPIKeyMultiplier {
		return fmt.Errorf("API key multiplier must be between %.1f and %.1f", MinAPIKeyMultiplier, MaxAPIKeyMultiplier)
	}

	for endpoint, cfg := range c.Endpoints {
		if cfg.RequestsPerSecond < MinRPS || cfg.RequestsPerSecond > MaxRPS {
			return fmt.Errorf("endpoint %s RPS must be between %d and %d", endpoint, MinRPS, MaxRPS)
		}
		if cfg.Burst < MinBurst || cfg.Burst > MaxBurst {
			return fmt.Errorf("endpoint %s burst must be between %d and %d", endpoint, MinBurst, MaxBurst)
		}
	}

	if c.StorageType != "memory" && c.StorageType != "redis" {
		return fmt.Errorf("storage type must be 'memory' or 'redis'")
	}

	return nil
}

// GetEndpointConfig returns the rate limit configuration for a specific endpoint
func (c *RateLimitConfig) GetEndpointConfig(endpoint string) *EndpointRateLimitConfig {
	if cfg, ok := c.Endpoints[endpoint]; ok {
		return cfg
	}
	return c.Default
}

// =============================================================================
// Token Bucket Implementation
// =============================================================================

// TokenBucket implements the token bucket algorithm for rate limiting
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
}

// NewTokenBucket creates a new token bucket
func NewTokenBucket(refillRate float64, burst int) *TokenBucket {
	return &TokenBucket{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed and consumes one token if so
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refillLocked()

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

// AllowN checks if n requests are allowed and consumes n tokens if so
func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if n <= 0 || n > MaxTokensPerRequest {
		return false
	}

	tb.refillLocked()

	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}
	return false
}

// refillLocked refills tokens based on elapsed time (must be called with lock held)
func (tb *TokenBucket) refillLocked() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	if elapsed > 0 {
		tb.tokens += elapsed * tb.refillRate
		if tb.tokens > tb.maxTokens {
			tb.tokens = tb.maxTokens
		}
		tb.lastRefill = now
	}
}

// Tokens returns current token count (for monitoring)
func (tb *TokenBucket) Tokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refillLocked()
	return tb.tokens
}

// MaxTokens returns maximum token capacity
func (tb *TokenBucket) MaxTokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.maxTokens
}

// RefillRate returns current refill rate
func (tb *TokenBucket) RefillRate() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.refillRate
}

// Reset resets the token bucket to full capacity
func (tb *TokenBucket) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.tokens = tb.maxTokens
	tb.lastRefill = time.Now()
}

// TimeUntilTokens returns time until n tokens are available
func (tb *TokenBucket) TimeUntilTokens(n int) time.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refillLocked()

	if tb.tokens >= float64(n) {
		return 0
	}

	needed := float64(n) - tb.tokens
	seconds := needed / tb.refillRate

	if seconds < 0 {
		seconds = 0
	}

	return time.Duration(seconds * float64(time.Second))
}

// =============================================================================
// Rate Limit Bucket (per identifier)
// =============================================================================

// RateLimitBucket represents a rate limiter for a specific identifier (IP or user)
type RateLimitBucket struct {
	bucket     *TokenBucket
	config     *EndpointRateLimitConfig
	lastAccess time.Time
	hits       int64
	misses     int64
}

// =============================================================================
// Enhanced Rate Limiter (Main Implementation)
// =============================================================================

// EnhancedRateLimiter provides per-endpoint, per-identifier rate limiting
type EnhancedRateLimiter struct {
	mu              sync.RWMutex
	config          *RateLimitConfig
	buckets         map[string]map[string]*RateLimitBucket // map[endpoint][identifier]
	apiKeys         map[string]*APIKeyInfo                 // map[apiKey]APIKeyInfo
	lastCleanup     time.Time
	cleanupInterval time.Duration
	stopCleanup     chan struct{}
}

// APIKeyInfo contains information about an API key
type APIKeyInfo struct {
	Key        string
	Owner      string
	Tier       string // "basic", "premium", "enterprise"
	Multiplier float64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	Revoked    bool
}

// NewEnhancedRateLimiter creates a new enhanced rate limiter
func NewEnhancedRateLimiter(cfg *RateLimitConfig) (*EnhancedRateLimiter, error) {
	if cfg == nil {
		cfg = DefaultRateLimitConfig()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid rate limit config: %w", err)
	}

	rl := &EnhancedRateLimiter{
		config:          cfg,
		buckets:         make(map[string]map[string]*RateLimitBucket),
		apiKeys:         make(map[string]*APIKeyInfo),
		cleanupInterval: cfg.CleanupInterval,
		stopCleanup:     make(chan struct{}),
		lastCleanup:     time.Now(),
	}

	// Start background cleanup goroutine
	go rl.cleanupLoop()

	return rl, nil
}

// cleanupLoop periodically removes stale buckets
func (rl *EnhancedRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCleanup:
			return
		}
	}
}

// cleanup removes stale buckets (not accessed in TTL duration)
func (rl *EnhancedRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	rl.lastCleanup = now

	for endpoint, buckets := range rl.buckets {
		for id, bucket := range buckets {
			if now.Sub(bucket.lastAccess) > DefaultTTL {
				delete(buckets, id)
			}
		}
		if len(buckets) == 0 {
			delete(rl.buckets, endpoint)
		}
	}

	activeRateLimiters.Set(float64(len(rl.buckets)))
}

// getOrCreateBucket gets or creates a bucket for an endpoint and identifier
func (rl *EnhancedRateLimiter) getOrCreateBucket(endpoint, identifier string, isAPIKey bool) *RateLimitBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.buckets[endpoint] == nil {
		rl.buckets[endpoint] = make(map[string]*RateLimitBucket)
	}

	bucket, exists := rl.buckets[endpoint][identifier]
	if !exists {
		cfg := rl.config.GetEndpointConfig(endpoint)

		// Apply API key multiplier if authenticated
		refillRate := cfg.RequestsPerSecond
		burst := cfg.Burst
		if isAPIKey {
			refillRate *= rl.config.APIKeyMultiplier
			burst = int(float64(burst) * rl.config.APIKeyMultiplier)
		}

		bucket = &RateLimitBucket{
			bucket:     NewTokenBucket(refillRate, burst),
			config:     cfg,
			lastAccess: time.Now(),
		}
		rl.buckets[endpoint][identifier] = bucket
	} else {
		bucket.lastAccess = time.Now()
	}

	return bucket
}

// Allow checks if a request is allowed for the given endpoint and identifier
func (rl *EnhancedRateLimiter) Allow(endpoint, identifier string, apiKey string) (bool, time.Duration, float64, float64) {
	if !rl.config.Enabled {
		return true, 0, 0, 0
	}

	// Check if API key is valid and not revoked
	isAPIKey := false
	if apiKey != "" {
		if info, ok := rl.apiKeys[apiKey]; ok && !info.Revoked {
			if info.ExpiresAt.IsZero() || time.Now().Before(info.ExpiresAt) {
				isAPIKey = true
				rateLimitBypassesTotal.Inc()
				rateLimitRequestsTotal.WithLabelValues(endpoint, "bypassed").Inc()
				return true, 0, info.Multiplier, 0
			}
		}
	}

	// Get or create bucket
	bucket := rl.getOrCreateBucket(endpoint, identifier, isAPIKey)

	// Check if request is allowed
	allowed := bucket.bucket.Allow()

	// Update metrics
	bucket.lastAccess = time.Now()
	if allowed {
		bucket.hits++
		rateLimitRequestsTotal.WithLabelValues(endpoint, "allowed").Inc()
	} else {
		bucket.misses++
		rateLimitRequestsTotal.WithLabelValues(endpoint, "denied").Inc()
		rateLimitEvents.WithLabelValues("rate_limit", "exceeded", endpoint).Inc()
	}

	// Update Prometheus gauges
	rateLimitTokensRemaining.WithLabelValues(endpoint, identifier).Set(bucket.bucket.Tokens())
	rateLimitCurrentRPS.WithLabelValues(endpoint, identifier).Set(bucket.bucket.RefillRate())

	// Calculate retry time if denied
	var retryAfter time.Duration
	if !allowed {
		retryAfter = bucket.bucket.TimeUntilTokens(1)
	}

	// Calculate limit and remaining for headers
	limit := bucket.bucket.MaxTokens()
	remaining := bucket.bucket.Tokens()

	return allowed, retryAfter, limit, remaining
}

// RegisterAPIKey registers a new API key
func (rl *EnhancedRateLimiter) RegisterAPIKey(key, owner, tier string, multiplier float64, expiresAt time.Time) error {
	if key == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	if multiplier < MinAPIKeyMultiplier || multiplier > MaxAPIKeyMultiplier {
		return fmt.Errorf("multiplier must be between %.1f and %.1f", MinAPIKeyMultiplier, MaxAPIKeyMultiplier)
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.apiKeys[key] = &APIKeyInfo{
		Key:        key,
		Owner:      owner,
		Tier:       tier,
		Multiplier: multiplier,
		CreatedAt:  time.Now(),
		ExpiresAt:  expiresAt,
		Revoked:    false,
	}

	return nil
}

// RevokeAPIKey revokes an API key
func (rl *EnhancedRateLimiter) RevokeAPIKey(key string) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if _, ok := rl.apiKeys[key]; !ok {
		return fmt.Errorf("API key not found")
	}

	rl.apiKeys[key].Revoked = true
	return nil
}

// GetAPIKeyInfo returns information about an API key
func (rl *EnhancedRateLimiter) GetAPIKeyInfo(key string) (*APIKeyInfo, error) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	info, ok := rl.apiKeys[key]
	if !ok {
		return nil, fmt.Errorf("API key not found")
	}

	if info.Revoked {
		return nil, fmt.Errorf("API key has been revoked")
	}

	if !info.ExpiresAt.IsZero() && time.Now().After(info.ExpiresAt) {
		return nil, fmt.Errorf("API key has expired")
	}

	return info, nil
}

// GetStats returns rate limiter statistics
func (rl *EnhancedRateLimiter) GetStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	stats := map[string]interface{}{
		"enabled":           rl.config.Enabled,
		"active_endpoints":  len(rl.buckets),
		"api_keys_count":    len(rl.apiKeys),
		"cleanup_interval":  rl.cleanupInterval.String(),
		"default_rps":       rl.config.Default.RequestsPerSecond,
		"default_burst":     rl.config.Default.Burst,
		"api_key_multiplier": rl.config.APIKeyMultiplier,
	}

	endpoints := make(map[string]interface{})
	for endpoint, buckets := range rl.buckets {
		endpointStats := map[string]interface{}{
			"active_identifiers": len(buckets),
		}
		endpoints[endpoint] = endpointStats
	}
	stats["endpoints"] = endpoints

	return stats
}

// Stop stops the rate limiter and cleanup goroutine
func (rl *EnhancedRateLimiter) Stop() {
	close(rl.stopCleanup)
}

// =============================================================================
// HTTP Middleware
// =============================================================================

// RateLimitMiddleware creates HTTP middleware for enhanced rate limiting
func RateLimitMiddleware(rl *EnhancedRateLimiter, getClientIP func(r *http.Request) string, getAPIKey func(r *http.Request) string, getUserID func(r *http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rl == nil || !rl.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Determine identifier (user ID > IP)
			identifier := getClientIP(r)
			if rl.config.ByUser && getUserID != nil {
				if userID := getUserID(r); userID != "" {
					identifier = userID
				}
			}

			// Get API key if present
			apiKey := ""
			if getAPIKey != nil {
				apiKey = getAPIKey(r)
			}

			// Normalize endpoint (remove trailing slashes and query params)
			endpoint := normalizeEndpoint(r.URL.Path)

			// Check rate limit
			allowed, retryAfter, limit, remaining := rl.Allow(endpoint, identifier, apiKey)

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(limit, 'f', 0, 64))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatFloat(remaining, 'f', 0, 64))

			if !allowed {
				resetTime := time.Now().Add(retryAfter).Unix()
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, `{"error":"rate_limit_exceeded","message":"Too many requests","retryAfter":%d}`, int(retryAfter.Seconds())+1)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// normalizeEndpoint normalizes an endpoint path for rate limiting
func normalizeEndpoint(path string) string {
	// Remove trailing slash
	path = strings.TrimSuffix(path, "/")

	// Remove query parameters (already handled by URL.Path)
	return path
}

// =============================================================================
// Helper Functions
// =============================================================================

// GenerateAPIKey generates a new random API key
func GenerateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// ExtractClientIP extracts client IP from request, respecting proxy headers if trusted
func ExtractClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		// Check X-Forwarded-For header
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				ip := strings.TrimSpace(parts[0])
				if netIP := net.ParseIP(ip); netIP != nil {
					return ip
				}
			}
		}

		// Check X-Real-IP header
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if netIP := net.ParseIP(xri); netIP != nil {
				return xri
			}
		}
	}

	// Fallback to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}

	return r.RemoteAddr
}

// ExtractAPIKey extracts API key from request headers or query parameters
func ExtractAPIKey(r *http.Request, headerName, queryParam string) string {
	// Check header first
	if headerName != "" {
		if key := r.Header.Get(headerName); key != "" {
			return key
		}
	}

	// Check query parameter
	if queryParam != "" {
		if key := r.URL.Query().Get(queryParam); key != "" {
			return key
		}
	}

	return ""
}

// ExtractUserID extracts user ID from request context or headers
func ExtractUserID(r *http.Request, headerName string) string {
	if headerName != "" {
		return r.Header.Get(headerName)
	}
	return ""
}



// =============================================================================
// Context Helpers
// =============================================================================

// WaitUntilAllowed waits until a request is allowed or context is cancelled
func (rl *EnhancedRateLimiter) WaitUntilAllowed(ctx context.Context, endpoint, identifier, apiKey string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			allowed, _, _, _ := rl.Allow(endpoint, identifier, apiKey)
			if allowed {
				return nil
			}
		}
	}
}

// AcquireTokens attempts to acquire n tokens at once
func (rl *EnhancedRateLimiter) AcquireTokens(ctx context.Context, endpoint, identifier string, n int) error {
	if n <= 0 || n > MaxTokensPerRequest {
		return fmt.Errorf("invalid token count: %d", n)
	}

	bucket := rl.getOrCreateBucket(endpoint, identifier, false)

	if bucket.bucket.AllowN(n) {
		return nil
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if bucket.bucket.AllowN(n) {
				return nil
			}
		}
	}
}

// =============================================================================
// Math Helper Functions (Overflow Protection)
// =============================================================================

// safeMultiplyFloat64 performs safe float64 multiplication with overflow check
func safeMultiplyFloat64(a, b float64) (float64, error) {
	if math.IsInf(a, 0) || math.IsInf(b, 0) {
		return 0, fmt.Errorf("infinite value")
	}
	if math.IsNaN(a) || math.IsNaN(b) {
		return 0, fmt.Errorf("NaN value")
	}

	result := a * b
	if math.IsInf(result, 0) {
		return 0, fmt.Errorf("overflow")
	}
	return result, nil
}

// safeAddFloat64 performs safe float64 addition with overflow check
func safeAddFloat64(a, b float64) (float64, error) {
	if math.IsInf(a, 0) || math.IsInf(b, 0) {
		return 0, fmt.Errorf("infinite value")
	}
	if math.IsNaN(a) || math.IsNaN(b) {
		return 0, fmt.Errorf("NaN value")
	}

	result := a + b
	if math.IsInf(result, 0) {
		return 0, fmt.Errorf("overflow")
	}
	return result, nil
}
