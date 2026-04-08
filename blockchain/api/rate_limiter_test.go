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
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestTokenBucket_NewBucket(t *testing.T) {
	bucket := NewTokenBucket(10.0, 20)
	if bucket.tokens != 20.0 {
		t.Errorf("expected initial tokens to be 20.0, got %f", bucket.tokens)
	}
	if bucket.maxTokens != 20.0 {
		t.Errorf("expected max tokens to be 20.0, got %f", bucket.maxTokens)
	}
	if bucket.refillRate != 10.0 {
		t.Errorf("expected refill rate to be 10.0, got %f", bucket.refillRate)
	}
}

func TestTokenBucket_Allow(t *testing.T) {
	bucket := NewTokenBucket(10.0, 5)
	for i := 0; i < 5; i++ {
		if !bucket.Allow() {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	if bucket.Allow() {
		t.Error("6th request should be denied")
	}
}

func TestTokenBucket_AllowN(t *testing.T) {
	bucket := NewTokenBucket(10.0, 10)
	if !bucket.AllowN(5) {
		t.Error("request for 5 tokens should be allowed")
	}
	if !bucket.AllowN(5) {
		t.Error("request for 5 more tokens should be allowed")
	}
	if bucket.AllowN(1) {
		t.Error("request for 1 token should be denied")
	}
	if bucket.AllowN(0) {
		t.Error("request for 0 tokens should be denied")
	}
	if bucket.AllowN(-1) {
		t.Error("request for negative tokens should be denied")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	bucket := NewTokenBucket(100.0, 10)
	for i := 0; i < 10; i++ {
		bucket.Allow()
	}
	time.Sleep(50 * time.Millisecond)
	if !bucket.Allow() {
		t.Error("should have refilled at least one token")
	}
}

func TestTokenBucket_TimeUntilTokens(t *testing.T) {
	bucket := NewTokenBucket(10.0, 10)
	for i := 0; i < 10; i++ {
		bucket.Allow()
	}
	waitTime := bucket.TimeUntilTokens(1)
	if waitTime <= 0 {
		t.Error("should need to wait for tokens")
	}
	time.Sleep(waitTime + 10*time.Millisecond)
	if !bucket.Allow() {
		t.Error("should have tokens after waiting")
	}
}

func TestTokenBucket_Reset(t *testing.T) {
	bucket := NewTokenBucket(10.0, 10)
	for i := 0; i < 10; i++ {
		bucket.Allow()
	}
	bucket.Reset()
	if bucket.Tokens() != 10.0 {
		t.Errorf("expected 10.0 tokens after reset, got %f", bucket.Tokens())
	}
}

func TestTokenBucket_Concurrent(t *testing.T) {
	bucket := NewTokenBucket(1000.0, 100)
	var wg sync.WaitGroup
	allowed := make(chan bool, 200)
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- bucket.Allow()
		}()
	}
	wg.Wait()
	close(allowed)
	count := 0
	for ok := range allowed {
		if ok {
			count++
		}
	}
	if count < 95 || count > 105 {
		t.Errorf("expected ~100 allowed requests, got %d", count)
	}
}

func TestEnhancedRateLimiter_NewLimiter(t *testing.T) {
	cfg := DefaultRateLimitConfig()
	cfg.Enabled = true
	cfg.StorageType = "memory"
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	stats := rl.GetStats()
	if stats["enabled"] != true {
		t.Error("rate limiter should be enabled")
	}
}

func TestEnhancedRateLimiter_Allow(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 10.0,
			Burst:             5,
		},
		Endpoints:        make(map[string]*EndpointRateLimitConfig),
		APIKeyMultiplier: 5.0,
		Enabled:          true,
		ByIP:             true,
		CleanupInterval:  DefaultCleanupInterval,
		StorageType:      "memory",
	}
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	endpoint := "/test"
	identifier := "192.168.1.1"
	for i := 0; i < 5; i++ {
		allowed, _, limit, remaining := rl.Allow(endpoint, identifier, "")
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
		if limit != 5.0 {
			t.Errorf("expected limit 5.0, got %f", limit)
		}
		if remaining < 0 {
			t.Errorf("expected remaining >= 0, got %f", remaining)
		}
	}
	allowed, retryAfter, _, _ := rl.Allow(endpoint, identifier, "")
	if allowed {
		t.Error("6th request should be denied")
	}
	if retryAfter <= 0 {
		t.Error("retryAfter should be positive when denied")
	}
}

func TestEnhancedRateLimiter_APIKeyMultiplier(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 10.0,
			Burst:             5,
		},
		Endpoints:        make(map[string]*EndpointRateLimitConfig),
		APIKeyMultiplier: 5.0,
		Enabled:          true,
		ByIP:             true,
		CleanupInterval:  DefaultCleanupInterval,
		StorageType:      "memory",
	}
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	apiKey := "test-api-key"
	err = rl.RegisterAPIKey(apiKey, "test-user", "premium", 5.0, time.Time{})
	if err != nil {
		t.Fatalf("failed to register API key: %v", err)
	}
	endpoint := "/test"
	identifier := "192.168.1.1"
	allowedCount := 0
	for i := 0; i < 30; i++ {
		allowed, _, _, _ := rl.Allow(endpoint, identifier, apiKey)
		if allowed {
			allowedCount++
		}
	}
	if allowedCount < 20 || allowedCount > 30 {
		t.Errorf("expected ~25 allowed requests with API key, got %d", allowedCount)
	}
}

func TestEnhancedRateLimiter_APIKeyExpiry(t *testing.T) {
	cfg := DefaultRateLimitConfig()
	cfg.Enabled = true
	cfg.StorageType = "memory"
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	apiKey := "test-api-key"
	expiresAt := time.Now().Add(100 * time.Millisecond)
	err = rl.RegisterAPIKey(apiKey, "test-user", "basic", 2.0, expiresAt)
	if err != nil {
		t.Fatalf("failed to register API key: %v", err)
	}
	_, _, _, _ = rl.Allow("/test", "192.168.1.1", apiKey)
	time.Sleep(150 * time.Millisecond)
	allowed, _, _, _ := rl.Allow("/test", "192.168.1.1", apiKey)
	if !allowed {
	}
}

func TestEnhancedRateLimiter_APIKeyRevoke(t *testing.T) {
	cfg := DefaultRateLimitConfig()
	cfg.Enabled = true
	cfg.StorageType = "memory"
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	apiKey := "test-api-key"
	err = rl.RegisterAPIKey(apiKey, "test-user", "basic", 2.0, time.Time{})
	if err != nil {
		t.Fatalf("failed to register API key: %v", err)
	}
	err = rl.RevokeAPIKey(apiKey)
	if err != nil {
		t.Fatalf("failed to revoke API key: %v", err)
	}
	_, err = rl.GetAPIKeyInfo(apiKey)
	if err == nil {
		t.Error("should error on revoked key")
	}
}

func TestEnhancedRateLimiter_EndpointConfig(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 10.0,
			Burst:             10,
		},
		Endpoints: map[string]*EndpointRateLimitConfig{
			"/api/tx": {
				RequestsPerSecond: 50.0,
				Burst:             100,
			},
		},
		APIKeyMultiplier: 5.0,
		Enabled:          true,
		ByIP:             true,
		CleanupInterval:  DefaultCleanupInterval,
		StorageType:      "memory",
	}
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	allowed, _, limit, _ := rl.Allow("/default", "192.168.1.1", "")
	if !allowed {
		t.Error("default endpoint should allow first request")
	}
	if limit != 10.0 {
		t.Errorf("expected default limit 10.0, got %f", limit)
	}
	allowed, _, limit, _ = rl.Allow("/api/tx", "192.168.1.1", "")
	if !allowed {
		t.Error("/api/tx endpoint should allow first request")
	}
	if limit != 100.0 {
		t.Errorf("expected /api/tx limit 100.0, got %f", limit)
	}
}

func TestEnhancedRateLimiter_Disabled(t *testing.T) {
	cfg := DefaultRateLimitConfig()
	cfg.Enabled = false
	cfg.StorageType = "memory"
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	for i := 0; i < 100; i++ {
		allowed, _, _, _ := rl.Allow("/test", "192.168.1.1", "")
		if !allowed {
			t.Error("should always allow when disabled")
		}
	}
}

func TestEnhancedRateLimiter_WaitUntilAllowed(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 100.0,
			Burst:             5,
		},
		Endpoints:        make(map[string]*EndpointRateLimitConfig),
		APIKeyMultiplier: 1.0,
		Enabled:          true,
		ByIP:             true,
		CleanupInterval:  DefaultCleanupInterval,
		StorageType:      "memory",
	}
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	for i := 0; i < 5; i++ {
		rl.Allow("/test", "192.168.1.1", "")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err = rl.WaitUntilAllowed(ctx, "/test", "192.168.1.1", "")
	if err != nil {
		t.Errorf("should have been allowed within timeout: %v", err)
	}
}

func TestEnhancedRateLimiter_Cleanup(t *testing.T) {
	cfg := DefaultRateLimitConfig()
	cfg.Enabled = true
	cfg.CleanupInterval = 100 * time.Millisecond
	cfg.StorageType = "memory"
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	rl.Allow("/endpoint1", "ip1", "")
	rl.Allow("/endpoint2", "ip2", "")
	stats := rl.GetStats()
	if stats["active_endpoints"].(int) != 2 {
		t.Errorf("expected 2 active endpoints, got %v", stats["active_endpoints"])
	}
	time.Sleep(150 * time.Millisecond)
	stats = rl.GetStats()
	if stats["active_endpoints"].(int) != 2 {
		t.Errorf("expected 2 active endpoints after cleanup, got %v", stats["active_endpoints"])
	}
}

func TestRateLimitMiddleware_Allow(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 10.0,
			Burst:             10,
		},
		Endpoints:        make(map[string]*EndpointRateLimitConfig),
		APIKeyMultiplier: 5.0,
		Enabled:          true,
		ByIP:             true,
		CleanupInterval:  DefaultCleanupInterval,
		StorageType:      "memory",
	}
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := RateLimitMiddleware(
		rl,
		func(r *http.Request) string { return "192.168.1.1" },
		func(r *http.Request) string { return "" },
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if rr.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("X-RateLimit-Limit header should be set")
	}
	if rr.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("X-RateLimit-Remaining header should be set")
	}
}

func TestRateLimitMiddleware_Deny(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 10.0,
			Burst:             2,
		},
		Endpoints:        make(map[string]*EndpointRateLimitConfig),
		APIKeyMultiplier: 5.0,
		Enabled:          true,
		ByIP:             true,
		CleanupInterval:  DefaultCleanupInterval,
		StorageType:      "memory",
	}
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := RateLimitMiddleware(
		rl,
		func(r *http.Request) string { return "192.168.1.1" },
		func(r *http.Request) string { return "" },
		nil,
	)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
	}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header should be set")
	}
	if rr.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("X-RateLimit-Reset header should be set")
	}
}

func TestRateLimitMiddleware_Disabled(t *testing.T) {
	cfg := DefaultRateLimitConfig()
	cfg.Enabled = false
	cfg.StorageType = "memory"
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := RateLimitMiddleware(rl, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key1, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("failed to generate API key: %v", err)
	}
	key2, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("failed to generate API key: %v", err)
	}
	if key1 == key2 {
		t.Error("generated API keys should be unique")
	}
	if len(key1) != 64 {
		t.Errorf("expected API key length 64, got %d", len(key1))
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/api/tx/", "/api/tx"},
		{"/api/tx", "/api/tx"},
		{"/api/tx//", "/api/tx"},
		{"/", ""},
		{"", ""},
	}
	for _, tt := range tests {
		result := normalizeEndpoint(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		trustProxy bool
		expected   string
	}{
		{
			name:       "no proxy",
			remoteAddr: "192.168.1.1:12345",
			headers:    map[string]string{},
			trustProxy: false,
			expected:   "192.168.1.1",
		},
		{
			name:       "proxy trusted with X-Forwarded-For",
			remoteAddr: "10.0.0.1:12345",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.1, 10.0.0.2",
			},
			trustProxy: true,
			expected:   "192.168.1.1",
		},
		{
			name:       "proxy trusted with X-Real-IP",
			remoteAddr: "10.0.0.1:12345",
			headers: map[string]string{
				"X-Real-IP": "192.168.1.1",
			},
			trustProxy: true,
			expected:   "192.168.1.1",
		},
		{
			name:       "proxy not trusted",
			remoteAddr: "192.168.1.1:12345",
			headers: map[string]string{
				"X-Forwarded-For": "10.0.0.1",
			},
			trustProxy: false,
			expected:   "192.168.1.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			result := ExtractClientIP(req, tt.trustProxy)
			if result != tt.expected {
				t.Errorf("expected IP %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractAPIKey(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		query      map[string]string
		headerName string
		queryParam string
		expected   string
	}{
		{
			name: "from header",
			headers: map[string]string{
				"X-API-Key": "test-key-123",
			},
			headerName: "X-API-Key",
			queryParam: "api_key",
			expected:   "test-key-123",
		},
		{
			name: "from query",
			query: map[string]string{
				"api_key": "test-key-456",
			},
			headerName: "X-API-Key",
			queryParam: "api_key",
			expected:   "test-key-456",
		},
		{
			name: "header takes precedence",
			headers: map[string]string{
				"X-API-Key": "header-key",
			},
			query: map[string]string{
				"api_key": "query-key",
			},
			headerName: "X-API-Key",
			queryParam: "api_key",
			expected:   "header-key",
		},
		{
			name:       "not found",
			headers:    map[string]string{},
			query:      map[string]string{},
			headerName: "X-API-Key",
			queryParam: "api_key",
			expected:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			if len(tt.query) > 0 {
				q := req.URL.Query()
				for k, v := range tt.query {
					q.Set(k, v)
				}
				req.URL.RawQuery = q.Encode()
			}
			result := ExtractAPIKey(req, tt.headerName, tt.queryParam)
			if result != tt.expected {
				t.Errorf("expected API key %q, got %q", tt.expected, result)
			}
		})
	}
}

func BenchmarkTokenBucket_Allow(b *testing.B) {
	bucket := NewTokenBucket(1000.0, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bucket.Allow()
		bucket.refillLocked()
	}
}

func BenchmarkEnhancedRateLimiter_Allow(b *testing.B) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 1000.0,
			Burst:             1000,
		},
		Endpoints:        make(map[string]*EndpointRateLimitConfig),
		APIKeyMultiplier: 5.0,
		Enabled:          true,
		ByIP:             true,
		CleanupInterval:  DefaultCleanupInterval,
		StorageType:      "memory",
	}
	rl, _ := NewEnhancedRateLimiter(cfg)
	defer rl.Stop()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow("/test", "192.168.1.1", "")
	}
}

func BenchmarkEnhancedRateLimiter_Concurrent(b *testing.B) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 10000.0,
			Burst:             1000,
		},
		Endpoints:        make(map[string]*EndpointRateLimitConfig),
		APIKeyMultiplier: 5.0,
		Enabled:          true,
		ByIP:             true,
		CleanupInterval:  DefaultCleanupInterval,
		StorageType:      "memory",
	}
	rl, _ := NewEnhancedRateLimiter(cfg)
	defer rl.Stop()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rl.Allow("/test", "192.168.1.1", "")
		}
	})
}

func TestRateLimitConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *RateLimitConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &RateLimitConfig{
				Default: &EndpointRateLimitConfig{
					RequestsPerSecond: 10.0,
					Burst:             20,
				},
				APIKeyMultiplier: 5.0,
				StorageType:      "memory",
			},
			wantErr: false,
		},
		{
			name: "invalid RPS",
			config: &RateLimitConfig{
				Default: &EndpointRateLimitConfig{
					RequestsPerSecond: 0,
					Burst:             20,
				},
				StorageType: "memory",
			},
			wantErr: true,
		},
		{
			name: "invalid burst",
			config: &RateLimitConfig{
				Default: &EndpointRateLimitConfig{
					RequestsPerSecond: 10.0,
					Burst:             0,
				},
				StorageType: "memory",
			},
			wantErr: true,
		},
		{
			name: "invalid multiplier",
			config: &RateLimitConfig{
				Default: &EndpointRateLimitConfig{
					RequestsPerSecond: 10.0,
					Burst:             20,
				},
				APIKeyMultiplier: 101.0,
				StorageType:      "memory",
			},
			wantErr: true,
		},
		{
			name: "invalid storage type",
			config: &RateLimitConfig{
				Default: &EndpointRateLimitConfig{
					RequestsPerSecond: 10.0,
					Burst:             20,
				},
				StorageType: "invalid",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRateLimitConfig_GetEndpointConfig(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 10.0,
			Burst:             20,
		},
		Endpoints: map[string]*EndpointRateLimitConfig{
			"/api/tx": {
				RequestsPerSecond: 50.0,
				Burst:             100,
			},
		},
	}
	epCfg := cfg.GetEndpointConfig("/api/tx")
	if epCfg.RequestsPerSecond != 50.0 {
		t.Errorf("expected 50.0 RPS, got %f", epCfg.RequestsPerSecond)
	}
	if epCfg.Burst != 100 {
		t.Errorf("expected burst 100, got %d", epCfg.Burst)
	}
	epCfg = cfg.GetEndpointConfig("/unknown")
	if epCfg.RequestsPerSecond != 10.0 {
		t.Errorf("expected 10.0 RPS, got %f", epCfg.RequestsPerSecond)
	}
	if epCfg.Burst != 20 {
		t.Errorf("expected burst 20, got %d", epCfg.Burst)
	}
}

func TestRateLimitHeaders(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 10.0,
			Burst:             5,
		},
		Endpoints:        make(map[string]*EndpointRateLimitConfig),
		APIKeyMultiplier: 5.0,
		Enabled:          true,
		ByIP:             true,
		CleanupInterval:  DefaultCleanupInterval,
		StorageType:      "memory",
	}
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := RateLimitMiddleware(
		rl,
		func(r *http.Request) string { return "192.168.1.1" },
		func(r *http.Request) string { return "" },
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	limit := rr.Header().Get("X-RateLimit-Limit")
	remaining := rr.Header().Get("X-RateLimit-Remaining")
	if limit == "" {
		t.Error("X-RateLimit-Limit header should be present")
	}
	if remaining == "" {
		t.Error("X-RateLimit-Remaining header should be present")
	}
	limitVal, err := strconv.ParseFloat(limit, 64)
	if err != nil {
		t.Errorf("X-RateLimit-Limit should be numeric: %v", err)
	} else if limitVal != 5.0 {
		t.Errorf("expected limit 5.0, got %f", limitVal)
	}
	remainingVal, err := strconv.ParseFloat(remaining, 64)
	if err != nil {
		t.Errorf("X-RateLimit-Remaining should be numeric: %v", err)
	} else if remainingVal < 0 || remainingVal > 5.0 {
		t.Errorf("expected remaining between 0 and 5, got %f", remainingVal)
	}
}
