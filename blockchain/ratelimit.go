package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// =============================================================================
// Configuration Constants (所有数值通过配置注入，此处仅为默认值)
// =============================================================================
const (
	DefaultRateLimitConnectionsPerSecond = 10  // 每秒连接数限制
	DefaultRateLimitMessagesPerSecond    = 100 // 每秒消息数限制
	DefaultRateLimitBanDuration          = 300 // 封禁时长（秒）
	DefaultRateLimitViolationsThreshold  = 10  // 违规阈值
	DefaultTokenBucketMaxTokens          = 100 // 令牌桶最大容量
	DefaultTokenBucketRefillRate         = 10  // 令牌补充速率（个/秒）
	DefaultBlacklistCleanupInterval      = 60  // 黑名单清理间隔（秒）
)

// =============================================================================
// Prometheus Metrics
// =============================================================================
var (
	rateLimitEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nogo_rate_limit_events",
			Help: "Number of rate limiting events by reason",
		},
		[]string{"reason", "type"}, // reason: blacklist, connection_limit, message_limit; type: block, throttle
	)

	blacklistedIPs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "nogo_blacklisted_ips",
			Help: "Current number of blacklisted IPs",
		},
	)
)

// RegisterMetrics registers rate limiting metrics with Prometheus
func RegisterMetrics() {
	prometheus.MustRegister(rateLimitEvents, blacklistedIPs)
}

// =============================================================================
// RateLimitConfig 限流配置结构
// =============================================================================
type RateLimitConfig struct {
	ConnectionsPerSecond     int           `json:"connections_per_second"`     // 每秒连接数限制
	MessagesPerSecond        int           `json:"messages_per_second"`        // 每秒消息数限制
	BanDuration              time.Duration `json:"ban_duration"`               // 封禁时长
	ViolationsThreshold      int           `json:"violations_threshold"`       // 违规阈值
	TokenBucketMaxTokens     int           `json:"token_bucket_max_tokens"`    // 令牌桶最大容量
	TokenBucketRefillRate    int           `json:"token_bucket_refill_rate"`   // 令牌补充速率
	BlacklistCleanupInterval time.Duration `json:"blacklist_cleanup_interval"` // 黑名单清理间隔
}

// DefaultRateLimitConfig returns default rate limit configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		ConnectionsPerSecond:     DefaultRateLimitConnectionsPerSecond,
		MessagesPerSecond:        DefaultRateLimitMessagesPerSecond,
		BanDuration:              time.Duration(DefaultRateLimitBanDuration) * time.Second,
		ViolationsThreshold:      DefaultRateLimitViolationsThreshold,
		TokenBucketMaxTokens:     DefaultTokenBucketMaxTokens,
		TokenBucketRefillRate:    DefaultTokenBucketRefillRate,
		BlacklistCleanupInterval: time.Duration(DefaultBlacklistCleanupInterval) * time.Second,
	}
}

// =============================================================================
// TokenBucket 令牌桶算法实现
// =============================================================================
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64   // 当前令牌数
	maxTokens  float64   // 最大令牌容量
	refillRate float64   // 令牌补充速率（个/秒）
	lastRefill time.Time // 上次补充时间
}

// NewTokenBucket creates a new token bucket
func NewTokenBucket(maxTokens, refillRate int) *TokenBucket {
	return &TokenBucket{
		tokens:     float64(maxTokens),
		maxTokens:  float64(maxTokens),
		refillRate: float64(refillRate),
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed and consumes a token if so
// Thread-safe with mutex protection
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate

	// Cap tokens at maximum
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}

	tb.lastRefill = now

	// Check if we have tokens available
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

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate

	// Cap tokens at maximum
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}

	tb.lastRefill = now

	// Check if we have enough tokens
	tokensNeeded := float64(n)
	if tb.tokens >= tokensNeeded {
		tb.tokens -= tokensNeeded
		return true
	}

	return false
}

// Tokens returns current token count (for monitoring)
func (tb *TokenBucket) Tokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.tokens
}

// Reset resets the token bucket to full capacity
func (tb *TokenBucket) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.tokens = tb.maxTokens
	tb.lastRefill = time.Now()
}

// =============================================================================
// IPBlacklist IP 黑名单管理
// =============================================================================
type blacklistEntry struct {
	ip         string
	expiryTime time.Time
	reason     string
}

type IPBlacklist struct {
	mu       sync.RWMutex
	entries  map[string]*blacklistEntry
	cleanup  chan struct{}
	interval time.Duration
}

// NewIPBlacklist creates a new IP blacklist with automatic cleanup
func NewIPBlacklist(cleanupInterval time.Duration) *IPBlacklist {
	bl := &IPBlacklist{
		entries:  make(map[string]*blacklistEntry),
		cleanup:  make(chan struct{}),
		interval: cleanupInterval,
	}

	// Start background cleanup goroutine
	go bl.cleanupLoop()

	return bl
}

// cleanupLoop periodically removes expired entries
func (bl *IPBlacklist) cleanupLoop() {
	ticker := time.NewTicker(bl.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bl.cleanupExpired()
		case <-bl.cleanup:
			return
		}
	}
}

// cleanupExpired removes all expired entries from the blacklist
func (bl *IPBlacklist) cleanupExpired() {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	now := time.Now()
	for ip, entry := range bl.entries {
		if now.After(entry.expiryTime) {
			delete(bl.entries, ip)
		}
	}

	// Update metrics
	blacklistedIPs.Set(float64(len(bl.entries)))
}

// IsBlacklisted checks if an IP is currently blacklisted
func (bl *IPBlacklist) IsBlacklisted(ip string) bool {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	entry, exists := bl.entries[ip]
	if !exists {
		return false
	}

	// Check if entry has expired (should be cleaned up automatically, but double-check)
	if time.Now().After(entry.expiryTime) {
		return false
	}

	return true
}

// AddToBlacklist adds an IP to the blacklist for a specified duration
func (bl *IPBlacklist) AddToBlacklist(ip, reason string, duration time.Duration) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	expiryTime := time.Now().Add(duration)
	bl.entries[ip] = &blacklistEntry{
		ip:         ip,
		expiryTime: expiryTime,
		reason:     reason,
	}

	// Update metrics
	blacklistedIPs.Set(float64(len(bl.entries)))

	// Record event
	rateLimitEvents.WithLabelValues("blacklist", "block").Inc()
}

// RemoveFromBlacklist removes an IP from the blacklist
func (bl *IPBlacklist) RemoveFromBlacklist(ip string) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	delete(bl.entries, ip)

	// Update metrics
	blacklistedIPs.Set(float64(len(bl.entries)))
}

// GetReason returns the reason for blacklisting (empty if not blacklisted)
func (bl *IPBlacklist) GetReason(ip string) string {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	entry, exists := bl.entries[ip]
	if !exists {
		return ""
	}

	return entry.reason
}

// Count returns the number of currently blacklisted IPs
func (bl *IPBlacklist) Count() int {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	return len(bl.entries)
}

// Stop stops the background cleanup goroutine
func (bl *IPBlacklist) Stop() {
	close(bl.cleanup)
}

// =============================================================================
// PeerRateLimiter Per-peer rate limiter
// =============================================================================
type PeerRateLimiter struct {
	mu         sync.Mutex
	peers      map[string]*TokenBucket
	config     *RateLimitConfig
	violations map[string]int // Track violations per peer
}

// NewPeerRateLimiter creates a new per-peer rate limiter
func NewPeerRateLimiter(config *RateLimitConfig) *PeerRateLimiter {
	return &PeerRateLimiter{
		peers:      make(map[string]*TokenBucket),
		config:     config,
		violations: make(map[string]int),
	}
}

// GetBucket gets or creates a token bucket for a peer
func (prl *PeerRateLimiter) GetBucket(peerID string) *TokenBucket {
	prl.mu.Lock()
	defer prl.mu.Unlock()

	bucket, exists := prl.peers[peerID]
	if !exists {
		bucket = NewTokenBucket(
			prl.config.TokenBucketMaxTokens,
			prl.config.TokenBucketRefillRate,
		)
		prl.peers[peerID] = bucket
	}

	return bucket
}

// AllowMessage checks if a message from a peer is allowed
func (prl *PeerRateLimiter) AllowMessage(peerID string) bool {
	bucket := prl.GetBucket(peerID)

	if !bucket.Allow() {
		prl.recordViolation(peerID)
		return false
	}

	return true
}

// recordViolation records a rate limit violation for a peer
func (prl *PeerRateLimiter) recordViolation(peerID string) {
	prl.mu.Lock()
	defer prl.mu.Unlock()

	prl.violations[peerID]++
}

// GetViolations returns the number of violations for a peer
func (prl *PeerRateLimiter) GetViolations(peerID string) int {
	prl.mu.Lock()
	defer prl.mu.Unlock()

	return prl.violations[peerID]
}

// ResetViolations resets violation count for a peer
func (prl *PeerRateLimiter) ResetViolations(peerID string) {
	prl.mu.Lock()
	defer prl.mu.Unlock()

	delete(prl.violations, peerID)
}

// RemovePeer removes a peer from the rate limiter
func (prl *PeerRateLimiter) RemovePeer(peerID string) {
	prl.mu.Lock()
	defer prl.mu.Unlock()

	delete(prl.peers, peerID)
	delete(prl.violations, peerID)
}

// =============================================================================
// RateLimiter Unified rate limiter (main entry point)
// =============================================================================
type RateLimiter struct {
	config           *RateLimitConfig
	connectionBucket *TokenBucket
	peerLimiter      *PeerRateLimiter
	blacklist        *IPBlacklist
	mu               sync.RWMutex
}

// NewRateLimiter creates a new unified rate limiter
func NewRateLimiter(config *RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		config:           config,
		connectionBucket: NewTokenBucket(config.ConnectionsPerSecond*2, config.ConnectionsPerSecond),
		peerLimiter:      NewPeerRateLimiter(config),
		blacklist:        NewIPBlacklist(config.BlacklistCleanupInterval),
	}

	return rl
}

// AllowConnection checks if a new connection from an IP is allowed
func (rl *RateLimiter) AllowConnection(ip string) bool {
	// Check blacklist first
	if rl.blacklist.IsBlacklisted(ip) {
		rateLimitEvents.WithLabelValues("blacklist", "block").Inc()
		return false
	}

	// Check connection rate limit
	if !rl.connectionBucket.Allow() {
		rateLimitEvents.WithLabelValues("connection_limit", "throttle").Inc()
		return false
	}

	return true
}

// AllowMessage checks if a message from a peer is allowed
func (rl *RateLimiter) AllowMessage(peerID, ip string) bool {
	// Check blacklist
	if rl.blacklist.IsBlacklisted(ip) {
		rateLimitEvents.WithLabelValues("blacklist", "block").Inc()
		return false
	}

	// Check per-peer rate limit
	if !rl.peerLimiter.AllowMessage(peerID) {
		violations := rl.peerLimiter.GetViolations(peerID)

		// Check if peer should be blacklisted
		if violations >= rl.config.ViolationsThreshold {
			rl.blacklist.AddToBlacklist(ip, "message_rate_limit_violations", rl.config.BanDuration)
			rateLimitEvents.WithLabelValues("message_limit", "block").Inc()
			return false
		}

		rateLimitEvents.WithLabelValues("message_limit", "throttle").Inc()
		return false
	}

	return true
}

// BanIP manually adds an IP to the blacklist
func (rl *RateLimiter) BanIP(ip, reason string, duration time.Duration) {
	rl.blacklist.AddToBlacklist(ip, reason, duration)
}

// UnbanIP removes an IP from the blacklist
func (rl *RateLimiter) UnbanIP(ip string) {
	rl.blacklist.RemoveFromBlacklist(ip)
}

// IsBanned checks if an IP is banned
func (rl *RateLimiter) IsBanned(ip string) bool {
	return rl.blacklist.IsBlacklisted(ip)
}

// GetPeerViolations returns violation count for a peer
func (rl *RateLimiter) GetPeerViolations(peerID string) int {
	return rl.peerLimiter.GetViolations(peerID)
}

// RemovePeer removes a peer from rate limiting
func (rl *RateLimiter) RemovePeer(peerID string) {
	rl.peerLimiter.RemovePeer(peerID)
}

// Stop stops the rate limiter and all background goroutines
func (rl *RateLimiter) Stop() {
	rl.blacklist.Stop()
}

// GetStats returns current rate limiter statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return map[string]interface{}{
		"connection_tokens": rl.connectionBucket.Tokens(),
		"blacklisted_ips":   rl.blacklist.Count(),
		"config":            rl.config,
	}
}

// =============================================================================
// Integration Helpers (P2P and HTTP)
// =============================================================================

// P2PConnectionHandler wraps a connection handler with rate limiting
type P2PConnectionHandler struct {
	rateLimiter *RateLimiter
	handler     func(ip string, conn interface{}) error
}

// NewP2PConnectionHandler creates a new P2P connection handler with rate limiting
func NewP2PConnectionHandler(rl *RateLimiter, handler func(ip string, conn interface{}) error) *P2PConnectionHandler {
	return &P2PConnectionHandler{
		rateLimiter: rl,
		handler:     handler,
	}
}

// HandleConnection handles incoming connections with rate limiting
func (h *P2PConnectionHandler) HandleConnection(ip string, conn interface{}) error {
	if !h.rateLimiter.AllowConnection(ip) {
		return ErrRateLimitExceeded
	}

	return h.handler(ip, conn)
}

// P2PMessageHandler wraps a message handler with per-peer rate limiting
type P2PMessageHandler struct {
	rateLimiter *RateLimiter
	handler     func(peerID string, msg []byte) error
}

// NewP2PMessageHandler creates a new P2P message handler with rate limiting
func NewP2PMessageHandler(rl *RateLimiter, handler func(peerID string, msg []byte) error) *P2PMessageHandler {
	return &P2PMessageHandler{
		rateLimiter: rl,
		handler:     handler,
	}
}

// HandleMessage handles incoming messages with rate limiting
func (h *P2PMessageHandler) HandleMessage(peerID, ip string, msg []byte) error {
	if !h.rateLimiter.AllowMessage(peerID, ip) {
		return ErrRateLimitExceeded
	}

	return h.handler(peerID, msg)
}

// RateLimitMiddleware creates HTTP middleware for rate limiting
func RateLimitMiddleware(rl *RateLimiter, getClientIP func() string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP()

			if rl.IsBanned(ip) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			if !rl.AllowConnection(ip) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// =============================================================================
// Errors
// =============================================================================
var (
	ErrRateLimitExceeded = &rateLimitError{"rate limit exceeded"}
)

type rateLimitError struct {
	message string
}

func (e *rateLimitError) Error() string {
	return e.message
}

// =============================================================================
// Context-based Rate Limiter (for advanced usage)
// =============================================================================

// Wait waits until a token is available or context is cancelled
func (tb *TokenBucket) Wait(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if tb.Allow() {
				return nil
			}
		}
	}
}

// Acquire attempts to acquire n tokens with context support
func (tb *TokenBucket) Acquire(ctx context.Context, n int) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if tb.AllowN(n) {
				return nil
			}
		}
	}
}
