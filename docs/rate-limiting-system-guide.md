# Enhanced Rate Limiting System Usage Guide

## Overview

The enhanced rate limiting system is implemented based on the token bucket algorithm, supporting per-endpoint independent configuration, IP-level and user-level rate limiting, as well as limit elevation after API key authentication.

## Key Features

### 1. Token Bucket Algorithm
- **Token Generation**: Tokens generated at a fixed rate (RequestsPerSecond per second)
- **Burst Support**: Allows burst traffic (Burst capacity)
- **Auto-Replenish**: Tokens automatically replenish over time
- **Concurrency Safe**: Uses mutex locks for thread safety

### 2. Per-Endpoint Independent Configuration
Different API endpoints can be configured with different rate limits:
```json
{
  "rate_limits": {
    "default": {
      "requests_per_second": 10,
      "burst": 20
    },
    "/api/tx": {
      "requests_per_second": 50,
      "burst": 100
    },
    "/api/balance": {
      "requests_per_second": 20,
      "burst": 40
    }
  }
}
```

### 3. API Key Authentication Elevation
- **Default Multiplier**: 5x elevation (configurable)
- **Expiration Support**: API keys can have expiration times
- **Revocation Support**: API keys can be revoked at any time
- **Tier Support**: Supports different tiers (basic, premium, enterprise)

### 4. Rate Limit Headers
All responses include the following headers:
- `X-RateLimit-Limit`: Maximum requests (bucket capacity)
- `X-RateLimit-Remaining`: Remaining requests (current tokens)
- `X-RateLimit-Reset`: Reset time (Unix timestamp)
- `Retry-After`: Retry wait time in seconds (only returned when rate limited)

### 5. Prometheus Monitoring Metrics
- `nogo_rate_limit_requests_total`: Total requests (classified by endpoint and result)
- `nogo_rate_limit_tokens_remaining`: Remaining tokens
- `nogo_rate_limit_current_rps`: Current RPS limit
- `nogo_rate_limit_bypasses_total`: API key bypass count
- `nogo_rate_limit_events`: Rate limiting events
- `nogo_active_rate_limiters`: Active rate limiter count

## Configuration Example

### Basic Configuration
```go
cfg := &RateLimitConfig{
    Default: &EndpointRateLimitConfig{
        RequestsPerSecond: 10.0,  // 10 requests per second
        Burst:             20,     // Burst capacity 20
    },
    Endpoints: map[string]*EndpointRateLimitConfig{
        "/api/tx": {
            RequestsPerSecond: 50.0,
            Burst:             100,
        },
    },
    APIKeyMultiplier: 5.0,        // API key elevation 5x
    Enabled:          true,       // Enable rate limiting
    ByIP:             true,       // Rate limit by IP
    ByUser:           false,      // Do not rate limit by user
    StorageType:      "memory",   // Memory storage
    CleanupInterval:  time.Minute, // Cleanup interval
}
```

### Environment Variable Configuration
```bash
# Enable rate limiting
export API_RATE_LIMIT_ENABLED=true

# Default RPS and burst
export API_RATE_LIMIT_RPS=10
export API_RATE_LIMIT_BURST=20

# API key elevation multiplier
export API_RATE_LIMIT_API_KEY_MULTIPLIER=5.0

# Trust proxy (get real IP)
export API_TRUST_PROXY=true
```

## Usage Examples

### 1. Create Rate Limiter
```go
import "github.com/nogochain/nogo/blockchain/api"

// Create configuration
cfg := api.DefaultRateLimitConfig()
cfg.Enabled = true
cfg.StorageType = "memory"

// Create rate limiter
rateLimiter, err := api.NewEnhancedRateLimiter(cfg)
if err != nil {
    log.Fatalf("Failed to create rate limiter: %v", err)
}
defer rateLimiter.Stop()
```

### 2. Register API Key
```go
// Register API key
apiKey := "your-api-key-here"
err := rateLimiter.RegisterAPIKey(
    apiKey,           // API key
    "user123",        // Owner
    "premium",        // Tier
    5.0,              // Elevation multiplier
    time.Time{},      // Expiration time (zero means never expires)
)
if err != nil {
    log.Fatalf("Failed to register API key: %v", err)
}
```

### 3. Integrate into HTTP Middleware
```go
// Create HTTP middleware
middleware := api.RateLimitMiddleware(
    rateLimiter,
    // Get client IP
    func(r *http.Request) string {
        return api.ExtractClientIP(r, true) // Trust proxy
    },
    // Get API key
    func(r *http.Request) string {
        return api.ExtractAPIKey(r, "X-API-Key", "api_key")
    },
    // Get user ID (optional)
    func(r *http.Request) string {
        return r.Header.Get("X-User-ID")
    },
)

// Apply to HTTP server
handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    fmt.Fprint(w, "Success")
})

httpServer := &http.Server{
    Addr:    ":8080",
    Handler: middleware(handler),
}
```

### 4. Use Rate Limiter Directly
```go
// Check if request is allowed
endpoint := "/api/tx"
identifier := "192.168.1.1" // IP or user ID
apiKey := "optional-api-key"

allowed, retryAfter, limit, remaining := rateLimiter.Allow(endpoint, identifier, apiKey)

if !allowed {
    // Return 429 Too Many Requests
    w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
    w.WriteHeader(http.StatusTooManyRequests)
    return
}

// Set rate limit headers
w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(limit, 'f', 0, 64))
w.Header().Set("X-RateLimit-Remaining", strconv.FormatFloat(remaining, 'f', 0, 64))
```

### 5. Wait Until Allowed
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := rateLimiter.WaitUntilAllowed(ctx, "/api/tx", "192.168.1.1", "")
if err != nil {
    // Timeout or other error
    log.Printf("Wait failed: %v", err)
    return
}

// Now can send request
```

### 6. Batch Token Acquisition
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Try to acquire 10 tokens at once
err := rateLimiter.AcquireTokens(ctx, "/api/batch", "192.168.1.1", 10)
if err != nil {
    log.Printf("Failed to acquire tokens: %v", err)
    return
}

// Execute batch operations
```

### 7. Manage API Keys
```go
// Get API key information
info, err := rateLimiter.GetAPIKeyInfo(apiKey)
if err != nil {
    log.Printf("Invalid API key: %v", err)
}

// Revoke API key
err = rateLimiter.RevokeAPIKey(apiKey)
if err != nil {
    log.Printf("Failed to revoke API key: %v", err)
}

// Generate new API key
newKey, err := api.GenerateAPIKey()
if err != nil {
    log.Fatalf("Failed to generate API key: %v", err)
}
```

### 8. Get Statistics
```go
stats := rateLimiter.GetStats()
fmt.Printf("Enabled: %v\n", stats["enabled"])
fmt.Printf("Active Endpoints: %v\n", stats["active_endpoints"])
fmt.Printf("API Keys Count: %v\n", stats["api_keys_count"])
fmt.Printf("Default RPS: %v\n", stats["default_rps"])
fmt.Printf("Default Burst: %v\n", stats["default_burst"])
```

## Prometheus Integration

### Register Metrics
```go
import "github.com/prometheus/client_golang/prometheus"

// Register rate limiting metrics
api.RegisterRateLimitMetrics()

// Expose at /metrics endpoint
http.Handle("/metrics", promhttp.Handler())
```

### Query Examples
```promql
# Total requests
sum(rate(nogo_rate_limit_requests_total[5m]))

# Rate limit rejection rate
sum(rate(nogo_rate_limit_requests_total{result="denied"}[5m])) 
/ 
sum(rate(nogo_rate_limit_requests_total[5m]))

# API key usage rate
sum(rate(nogo_rate_limit_bypasses_total[5m]))

# Token remaining by endpoint
nogo_rate_limit_tokens_remaining
```

## Best Practices

### 1. Configuration Recommendations
- **Default Endpoints**: Set lower default limits (e.g., 10 RPS)
- **High-Frequency Endpoints**: Set higher limits for query endpoints (e.g., 50-100 RPS)
- **Write Operation Endpoints**: Set lower limits for write endpoints (e.g., 5-10 RPS)
- **Burst Capacity**: Set to 2-5 times RPS

### 2. API Key Management
- **Regular Rotation**: Recommended to rotate every 3-6 months
- **Least Privilege**: Assign appropriate elevation multipliers based on user needs
- **Monitor Usage**: Monitor API key usage through Prometheus metrics

### 3. Monitoring and Alerts
```yaml
# Prometheus alert rules example
groups:
- name: rate_limiting
  rules:
  - alert: HighRateLimitRejection
    expr: sum(rate(nogo_rate_limit_requests_total{result="denied"}[5m])) > 100
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High rate limit rejection rate"
      
  - alert: RateLimiterMemoryHigh
    expr: nogo_active_rate_limiters > 10000
    for: 10m
    labels:
      severity: info
    annotations:
      summary: "Too many active rate limiters"
```

### 4. Performance Optimization
- **Memory Storage**: Use memory storage by default for best performance (< 1ms overhead)
- **Redis Storage**: Can choose Redis for distributed scenarios (to be implemented)
- **Cleanup Interval**: Adjust cleanup interval based on memory usage
- **Concurrency Control**: Rate limiter itself is optimized for concurrency

## Testing

### Run Unit Tests
```bash
cd blockchain/api
go test -v rate_limiter.go rate_limiter_test.go -run "TestTokenBucket|TestEnhancedRateLimiter"
```

### Run Integration Tests
```bash
cd blockchain/api
go test -v rate_limiter.go rate_limiter_integration_test.go -run "TestIntegration"
```

### Performance Benchmark Tests
```bash
cd blockchain/api
go test -bench=Benchmark rate_limiter.go rate_limiter_test.go
```

## Important Notes

1. **Concurrency Safety**: Rate limiter internally handles concurrency, safe to use in multiple goroutines
2. **Memory Management**: Regularly clean up inactive rate limit buckets (default 10-minute TTL)
3. **Time Precision**: Token replenishment based on timestamp calculation, precision in nanoseconds
4. **Overflow Protection**: All mathematical operations have overflow checks
5. **Error Handling**: All errors include context information, support errors.Is/As

## Troubleshooting

### Problem: Rate limiter not working
- Check if `cfg.Enabled` is set to `true`
- Confirm middleware is correctly applied to HTTP server
- Check if configuration passes validation (`cfg.Validate()`)

### Problem: API key not taking effect
- Confirm API key is correctly registered
- Check if API key is expired or revoked
- Verify request header name is correct (default `X-API-Key`)

### Problem: High memory usage
- Reduce `CleanupInterval` interval
- Lower `DefaultTTL` time
- Monitor `nogo_active_rate_limiters` metric

## Future Extensions

1. **Redis Storage**: Support distributed rate limiting
2. **Sliding Window**: Optional sliding window algorithm
3. **Dynamic Configuration**: Support runtime dynamic configuration adjustment
4. **Rate Limiting Strategies**: Support more rate limiting strategies (e.g., leaky bucket, fixed window, etc.)
