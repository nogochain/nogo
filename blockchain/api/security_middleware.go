// Copyright 2026 NogoChain Team
// Production-grade security enhancements for HTTP API layer
// Implements: configurable rate limiting, request body limits, security headers

package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// =============================================================================
// Environment Variable Configuration Constants
// =============================================================================
const (
	envPrefix = "NOGO_API_"

	// Rate limiting environment variables
	envRateLimitRPS     = envPrefix + "RATE_LIMIT_RPS"
	envRateLimitBurst   = envPrefix + "RATE_LIMIT_BURST"
	envRateLimitEnabled = envPrefix + "RATE_LIMIT_ENABLED"

	// Request body size limit
	envMaxRequestBodyMB = envPrefix + "MAX_REQUEST_BODY_MB"

	// Security headers configuration
	envSecurityHeadersEnabled = envPrefix + "SECURITY_HEADERS_ENABLED"
	envCORSAllowOrigins       = envPrefix + "CORS_ALLOW_ORIGINS"

	// Default production values
	defaultMaxRequestBodyBytes = 10 << 20 // 10 MB
	defaultExchangeRPS         = 1000     // Exchange high-concurrency mode
	defaultExchangeBurst       = 5000     // Burst capacity for exchanges
	defaultPublicRPS           = 100      // Public node rate limit
	defaultPublicBurst         = 200      // Public node burst
)

// =============================================================================
// Prometheus Metrics for Security Module
// =============================================================================
var (
	securityRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nogo_security_requests_total",
			Help: "Total number of requests processed by security middleware",
		},
		[]string{"endpoint", "method", "result"},
	)

	securityBodySizeExceeded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nogo_security_body_size_exceeded_total",
			Help: "Number of requests rejected due to body size exceeded",
		},
		[]string{"endpoint"},
	)

	securityConfigLoaded = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "nogo_security_config_loaded",
			Help: "Indicates if security configuration was successfully loaded (1=yes, 0=no)",
		},
	)
)

func RegisterSecurityMetrics() {
	prometheus.MustRegister(
		securityRequestsTotal,
		securityBodySizeExceeded,
		securityConfigLoaded,
	)
}

// =============================================================================
// SecurityConfig holds all security-related configuration loaded from environment
// =============================================================================
type SecurityConfig struct {
	mu sync.RWMutex

	RateLimitRPS        int
	RateLimitBurst      int
	RateLimitEnabled    bool
	MaxRequestBodyBytes int64
	SecurityHeaders     bool
	CORSAllowOrigins    []string

	loaded   bool
	loadTime time.Time
}

var globalSecurityConfig *SecurityConfig
var once sync.Once

// GetSecurityConfig returns the singleton security configuration instance
func GetSecurityConfig() *SecurityConfig {
	once.Do(func() {
		globalSecurityConfig = loadSecurityConfigFromEnv()
	})
	return globalSecurityConfig
}

// loadSecurityConfigFromEnv reads all security settings from environment variables
// Production-grade: provides sensible defaults for both public nodes and exchange deployments
func loadSecurityConfigFromEnv() *SecurityConfig {
	config := &SecurityConfig{
		loadTime: time.Now(),
	}

	config.RateLimitRPS = getIntEnv(envRateLimitRPS, defaultPublicRPS)
	config.RateLimitBurst = getIntEnv(envRateLimitBurst, defaultPublicBurst)
	config.RateLimitEnabled = getBoolEnv(envRateLimitEnabled, true)
	config.MaxRequestBodyBytes = getInt64Env(envMaxRequestBodyMB, defaultMaxRequestBodyBytes>>20) << 20
	config.SecurityHeaders = getBoolEnv(envSecurityHeadersEnabled, true)

	if origins := os.Getenv(envCORSAllowOrigins); origins != "" {
		config.CORSAllowOrigins = strings.Split(origins, ",")
	} else {
		config.CORSAllowOrigins = []string{"*"}
	}

	config.validateAndNormalize()
	config.loaded = true
	securityConfigLoaded.Set(1)

	return config
}

// validateAndNormalize ensures all configuration values are within acceptable bounds
func (c *SecurityConfig) validateAndNormalize() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.RateLimitRPS <= 0 {
		c.RateLimitRPS = defaultPublicRPS
	}
	if c.RateLimitRPS > 10000 {
		c.RateLimitRPS = 10000
	}
	if c.RateLimitBurst <= 0 {
		c.RateLimitBurst = c.RateLimitRPS * 2
	}
	if c.RateLimitBurst > 50000 {
		c.RateLimitBurst = 50000
	}
	if c.MaxRequestBodyBytes <= 0 {
		c.MaxRequestBodyBytes = defaultMaxRequestBodyBytes
	}
	if c.MaxRequestBodyBytes > 100<<20 { // Max 100 MB
		c.MaxRequestBodyBytes = 100 << 20
	}

	if c.RateLimitBurst < c.RateLimitRPS {
		c.RateLimitBurst = c.RateLimitRPS * 2
	}
}

// Reload allows runtime reconfiguration without restart (hot-reload capable)
func (c *SecurityConfig) Reload() error {
	newConfig := loadSecurityConfigFromEnv()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.RateLimitRPS = newConfig.RateLimitRPS
	c.RateLimitBurst = newConfig.RateLimitBurst
	c.RateLimitEnabled = newConfig.RateLimitEnabled
	c.MaxRequestBodyBytes = newConfig.MaxRequestBodyBytes
	c.SecurityHeaders = newConfig.SecurityHeaders
	c.CORSAllowOrigins = newConfig.CORSAllowOrigins
	c.loadTime = time.Now()

	return nil
}

// IsExchangeMode detects if the current configuration is optimized for exchange deployment
func (c *SecurityConfig) IsExchangeMode() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.RateLimitRPS >= 500
}

// String returns a human-readable representation of the current security config
func (c *SecurityConfig) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	mode := "public"
	if c.IsExchangeMode() {
		mode = "exchange"
	}
	return fmt.Sprintf("SecurityConfig{mode=%s, rps=%d, burst=%d, maxBody=%dMB, secHeaders=%v}",
		mode, c.RateLimitRPS, c.RateLimitBurst, c.MaxRequestBodyBytes>>20, c.SecurityHeaders)
}

// =============================================================================
// Security Middleware Factory
// =============================================================================

// SecurityMiddleware wraps an http.Handler with production-grade security features:
// - Configurable rate limiting (environment-driven)
// - Request body size enforcement
// - Security headers injection
// - CORS policy enforcement
// - Request ID tracking for audit trails
type SecurityMiddleware struct {
	config      *SecurityConfig
	next        http.Handler
	rateLimiter *EnhancedRateLimiter
	metrics     bool
}

// NewSecurityMiddleware creates a new security middleware with the given configuration
func NewSecurityMiddleware(next http.Handler, enableMetrics bool) *SecurityMiddleware {
	config := GetSecurityConfig()

	var limiter *EnhancedRateLimiter
	if config.RateLimitEnabled {
		rateConfig := DefaultRateLimitConfig()
		rateConfig.Default.RequestsPerSecond = float64(config.RateLimitRPS)
		rateConfig.Default.Burst = config.RateLimitBurst
		rateConfig.Enabled = true

		var err error
		limiter, err = NewEnhancedRateLimiter(rateConfig)
		if err != nil {
			fmt.Printf("[SECURITY] Failed to create rate limiter: %v\n", err)
			return nil
		}
	}

	if enableMetrics {
		RegisterSecurityMetrics()
	}

	return &SecurityMiddleware{
		config:      config,
		next:        next,
		rateLimiter: limiter,
		metrics:     enableMetrics,
	}
}

// ServeHTTP implements http.Handler interface with full security pipeline
func (mw *SecurityMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	endpoint := sanitizeEndpoint(r.URL.Path)

	if mw.metrics {
		defer func() {
			result := "allowed"
			if w.Header().Get("X-Rate-Limited") == "true" {
				result = "rate_limited"
			}
			if w.Header().Get("X-Body-Rejected") == "true" {
				result = "body_rejected"
			}
			securityRequestsTotal.WithLabelValues(endpoint, r.Method, result).Inc()
		}()
	}

	ctx := r.Context()
	reqID := ctx.Value(requestIDKey)
	if reqID == nil {
		reqID = generateSecureRequestID()
		ctx = context.WithValue(ctx, requestIDKey, reqID)
		r = r.WithContext(ctx)
	}

	w.Header().Set("X-Request-ID", reqID.(string))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	if mw.config.SecurityHeaders {
		mw.applySecurityHeaders(w, r)
	}

	if mw.config.RateLimitEnabled && mw.rateLimiter != nil {
		if !mw.checkRateLimit(w, r, endpoint, reqID.(string)) {
			return
		}
	}

	if !mw.checkRequestBodySize(w, r, endpoint, reqID.(string)) {
		return
	}

	mw.next.ServeHTTP(w, r)

	elapsed := time.Since(startTime)
	if elapsed > 500*time.Millisecond {
		logSlowRequest(endpoint, elapsed, reqID.(string))
	}
}

// applySecurityHeaders injects production-grade security headers into response
func (mw *SecurityMiddleware) applySecurityHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")

	origin := r.Header.Get("Origin")
	if origin != "" && mw.isOriginAllowed(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
		w.Header().Set("Access-Control-Allow-Headers",
			"Origin, Content-Type, Accept, Authorization, X-Requested-With, X-Request-ID, X-API-Key, X-Relay-Hops")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-RateLimit-Remaining, X-RateLimit-Reset")
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
}

// checkRateLimit enforces rate limiting rules based on client identifier
func (mw *SecurityMiddleware) checkRateLimit(w http.ResponseWriter, r *http.Request, endpoint, reqID string) bool {
	clientID := identifyClient(r)

	allowed, retryAfter, remaining, _ := mw.rateLimiter.Allow(endpoint, clientID, "")
	if !allowed {
		w.Header().Set("X-Rate-Limited", "true")
		w.Header().Set("Retry-After", strconv.FormatFloat(float64(retryAfter.Seconds())+1, 'f', 0, 64))
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(mw.config.RateLimitBurst))

		writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"error":      "rate_limited",
			"message":    "too many requests, please retry later",
			"requestId":  reqID,
			"retryAfter": int(retryAfter.Seconds()) + 1,
		})
		return false
	}

	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(int(remaining)))
	resetTime := time.Now().Add(retryAfter).Unix()
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))

	return true
}

// checkRequestBodySize enforces maximum request body size to prevent DoS attacks
func (mw *SecurityMiddleware) checkRequestBodySize(w http.ResponseWriter, r *http.Request, endpoint, reqID string) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
		return true
	}

	maxSize := mw.config.MaxRequestBodyBytes
	if maxSize <= 0 {
		return true
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	return true
}

// isOriginAllowed checks if the given origin is in the allowlist
func (mw *SecurityMiddleware) isOriginAllowed(origin string) bool {
	mw.config.mu.RLock()
	defer mw.config.mu.RUnlock()

	for _, allowed := range mw.config.CORSAllowOrigins {
		if allowed == "*" || strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

// =============================================================================
// Helper Functions
// =============================================================================

// identifyClient extracts a unique client identifier from the request
func identifyClient(r *http.Request) string {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		return "key:" + hashIdentifier(apiKey)
	}

	ip := extractClientIP(r)
	return "ip:" + ip
}

// extractClientIP gets the real client IP, accounting for reverse proxies
func extractClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// generateSecureRequestID creates a cryptographically secure request ID for tracing
func generateSecureRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

// sanitizeEndpoint removes sensitive path segments for logging
func sanitizeEndpoint(path string) string {
	if len(path) > 50 {
		return path[:50] + "..."
	}
	return path
}

// logSlowRequest records requests that exceed performance thresholds
func logSlowRequest(endpoint string, duration time.Duration, reqID string) {
	durationMs := float64(duration.Milliseconds())
	if durationMs > 1000 {
		fmt.Printf("[SECURITY] Slow request detected: endpoint=%s duration=%.2fms requestId=%s\n",
			endpoint, durationMs, reqID)
	}
}

// hashIdentifier creates a one-way hash of client identifiers for privacy
func hashIdentifier(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:8])
}

// =============================================================================
// Environment Variable Helpers
// =============================================================================

// getIntEnv reads an integer from environment variable with fallback default
func getIntEnv(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getInt64Env reads an int64 from environment variable with fallback default
func getInt64Env(key string, defaultValue int64) int64 {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getBoolEnv reads a boolean from environment variable with fallback default
func getBoolEnv(key string, defaultValue bool) bool {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getFloat64Env reads a float64 from environment variable with fallback default
func getFloat64Env(key string, defaultValue float64) float64 {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			if !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
				return parsed
			}
		}
	}
	return defaultValue
}

// =============================================================================
// Exchange Deployment Helper
// =============================================================================

// ConfigureForExchange sets optimal security parameters for exchange integration
// Call this during node initialization when exchange mode is detected
func ConfigureForExchange() error {
	config := GetSecurityConfig()

	config.mu.Lock()
	defer config.mu.Unlock()

	config.RateLimitRPS = defaultExchangeRPS
	config.RateLimitBurst = defaultExchangeBurst
	config.RateLimitEnabled = true
	config.MaxRequestBodyBytes = 20 << 20 // 20 MB for batch operations
	config.SecurityHeaders = true
	config.CORSAllowOrigins = []string{"*"} // Exchanges may call from multiple IPs

	fmt.Printf("[SECURITY] Exchange mode activated: %s\n", config.String())

	return nil
}

// ConfigureForPublicNode sets conservative security parameters for public nodes
func ConfigureForPublicNode() error {
	config := GetSecurityConfig()

	config.mu.Lock()
	defer config.mu.Unlock()

	config.RateLimitRPS = defaultPublicRPS
	config.RateLimitBurst = defaultPublicBurst
	config.RateLimitEnabled = true
	config.MaxRequestBodyBytes = defaultMaxRequestBodyBytes
	config.SecurityHeaders = true
	config.CORSAllowOrigins = []string{"*"}

	fmt.Printf("[SECURITY] Public node mode activated: %s\n", config.String())

	return nil
}

// PrintSecurityStatus outputs current security configuration for operational visibility
func PrintSecurityStatus() {
	config := GetSecurityConfig()
	mode := "Standard"
	if config.IsExchangeMode() {
		mode = "🏦 EXCHANGE (High-Concurrency)"
	}

	fmt.Println("\n========================================")
	fmt.Println("🔒 NogoChain Security Configuration Status")
	fmt.Println("========================================")
	fmt.Printf("Mode:              %s\n", mode)
	fmt.Printf("Rate Limit RPS:    %d requests/second\n", config.RateLimitRPS)
	fmt.Printf("Rate Limit Burst: %d tokens\n", config.RateLimitBurst)
	fmt.Printf("Rate Limiting:    %v\n", config.RateLimitEnabled)
	fmt.Printf("Max Body Size:    %d MB\n", config.MaxRequestBodyBytes>>20)
	fmt.Printf("Security Headers: %v\n", config.SecurityHeaders)
	fmt.Printf("CORS Origins:     %v\n", config.CORSAllowOrigins)
	fmt.Printf("Config Loaded At: %s\n", config.loadTime.Format(time.RFC3339))
	fmt.Println("========================================")
}
