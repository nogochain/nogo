package networking

import (
	"sync"
	"time"
)

type TokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
	mu         sync.Mutex
}

func NewTokenBucket(maxTokens float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}
	return false
}

type RateLimiter struct {
	limiters map[string]*TokenBucket
	mu       sync.RWMutex
	config   RateLimiterConfig
}

type RateLimiterConfig struct {
	MaxMessagesPerSecond   float64
	BurstSize              float64
	MaxBytesPerSecond      float64
	MaxBurstBytes          float64
	CleanupInterval        time.Duration
}

func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		MaxMessagesPerSecond: 100.0,
		BurstSize:            200.0,
		MaxBytesPerSecond:    1024 * 1024,
		MaxBurstBytes:        2 * 1024 * 1024,
		CleanupInterval:      5 * time.Minute,
	}
}

func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	if config.MaxMessagesPerSecond <= 0 {
		config.MaxMessagesPerSecond = 100.0
	}
	if config.BurstSize <= 0 {
		config.BurstSize = 200.0
	}
	if config.MaxBytesPerSecond <= 0 {
		config.MaxBytesPerSecond = 1024 * 1024
	}
	if config.MaxBurstBytes <= 0 {
		config.MaxBurstBytes = 2 * 1024 * 1024
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Minute
	}

	rl := &RateLimiter{
		limiters: make(map[string]*TokenBucket),
		config:   config,
	}

	go rl.cleanupLoop()

	return rl
}

func (rl *RateLimiter) AllowMessage(addr string) bool {
	rl.mu.RLock()
	limiter, ok := rl.limiters[addr]
	rl.mu.RUnlock()

	if !ok {
		rl.mu.Lock()
		limiter, ok = rl.limiters[addr]
		if !ok {
			limiter = NewTokenBucket(rl.config.BurstSize, rl.config.MaxMessagesPerSecond)
			rl.limiters[addr] = limiter
		}
		rl.mu.Unlock()
	}

	return limiter.Allow()
}

func (rl *RateLimiter) AllowBytes(addr string, bytes int) bool {
	rl.mu.RLock()
	limiter, ok := rl.limiters[addr]
	rl.mu.RUnlock()

	if !ok {
		rl.mu.Lock()
		limiter, ok = rl.limiters[addr]
		if !ok {
			limiter = NewTokenBucket(rl.config.MaxBurstBytes, rl.config.MaxBytesPerSecond)
			rl.limiters[addr] = limiter
		}
		rl.mu.Unlock()
	}

	return limiter.AllowN(bytes)
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanup()
	}
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for addr, limiter := range rl.limiters {
		limiter.mu.Lock()
		if limiter.tokens <= 0 {
			delete(rl.limiters, addr)
		}
		limiter.mu.Unlock()
	}
}
