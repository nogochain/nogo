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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestIntegration_RateLimiting_Basic(t *testing.T) {
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
		fmt.Fprint(w, "OK")
	})
	middleware := RateLimitMiddleware(
		rl,
		func(r *http.Request) string { return "192.168.1.1" },
		func(r *http.Request) string { return "" },
		nil,
	)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", rr.Code)
	}
	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if response["error"] != "rate_limit_exceeded" {
		t.Errorf("expected error 'rate_limit_exceeded', got '%v'", response["error"])
	}
}

func TestIntegration_RateLimiting_PerEndpoint(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 10.0,
			Burst:             5,
		},
		Endpoints: map[string]*EndpointRateLimitConfig{
			"/api/tx": {
				RequestsPerSecond: 50.0,
				Burst:             20,
			},
			"/api/balance": {
				RequestsPerSecond: 20.0,
				Burst:             10,
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := RateLimitMiddleware(
		rl,
		func(r *http.Request) string { return "192.168.1.1" },
		func(r *http.Request) string { return "" },
		nil,
	)
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/tx", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("/api/tx request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/tx", nil)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("/api/tx: expected status 429, got %d", rr.Code)
	}
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/balance", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("/api/balance request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
	req = httptest.NewRequest(http.MethodGet, "/api/balance", nil)
	rr = httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("/api/balance: expected status 429, got %d", rr.Code)
	}
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/other", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("/api/other request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
}

func TestIntegration_RateLimiting_APIKeyBypass(t *testing.T) {
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
	apiKey := "test-api-key-premium"
	err = rl.RegisterAPIKey(apiKey, "premium-user", "premium", 5.0, time.Time{})
	if err != nil {
		t.Fatalf("failed to register API key: %v", err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := RateLimitMiddleware(
		rl,
		func(r *http.Request) string { return "192.168.1.1" },
		func(r *http.Request) string {
			return r.Header.Get("X-API-Key")
		},
		nil,
	)
	requestCount := 0
	for i := 0; i < 30; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("X-API-Key", apiKey)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
		if rr.Code == http.StatusOK {
			requestCount++
		}
	}
	if requestCount < 20 || requestCount > 30 {
		t.Errorf("expected ~25 allowed requests with API key, got %d", requestCount)
	}
	requestCount = 0
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
		if rr.Code == http.StatusOK {
			requestCount++
		}
	}
	if requestCount != 5 {
		t.Errorf("expected 5 allowed requests without API key, got %d", requestCount)
	}
}

func TestIntegration_RateLimiting_MultipleIPs(t *testing.T) {
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
	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}
	for _, ip := range ips {
		currentIP := ip
		middleware := RateLimitMiddleware(
			rl,
			func(r *http.Request) string { return currentIP },
			func(r *http.Request) string { return "" },
			nil,
		)
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			rr := httptest.NewRecorder()
			middleware(handler).ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("IP %s request %d: expected status 200, got %d", ip, i+1, rr.Code)
			}
		}
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
		if rr.Code != http.StatusTooManyRequests {
			t.Errorf("IP %s: expected status 429, got %d", ip, rr.Code)
		}
	}
}

func TestIntegration_RateLimiting_TokenRefill(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 100.0,
			Burst:             10,
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := RateLimitMiddleware(
		rl,
		func(r *http.Request) string { return "192.168.1.1" },
		func(r *http.Request) string { return "" },
		nil,
	)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
	}
	time.Sleep(50 * time.Millisecond)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 after token refill, got %d", rr.Code)
	}
}

func TestIntegration_RateLimiting_Concurrent(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 1000.0,
			Burst:             100,
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
	var wg sync.WaitGroup
	successCount := 0
	rateLimitedCount := 0
	var mu sync.Mutex
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			rr := httptest.NewRecorder()
			middleware(handler).ServeHTTP(rr, req)
			mu.Lock()
			defer mu.Unlock()
			if rr.Code == http.StatusOK {
				successCount++
			} else if rr.Code == http.StatusTooManyRequests {
				rateLimitedCount++
			}
		}()
	}
	wg.Wait()
	if successCount < 90 || successCount > 110 {
		t.Errorf("expected ~100 successful requests, got %d", successCount)
	}
	if rateLimitedCount < 90 || rateLimitedCount > 110 {
		t.Errorf("expected ~100 rate limited requests, got %d", rateLimitedCount)
	}
}

func TestIntegration_RateLimiting_Headers(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	headers := rr.Header()
	limit := headers.Get("X-RateLimit-Limit")
	remaining := headers.Get("X-RateLimit-Remaining")
	if limit == "" {
		t.Error("X-RateLimit-Limit header missing")
	}
	if remaining == "" {
		t.Error("X-RateLimit-Remaining header missing")
	}
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr = httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	headers = rr.Header()
	retryAfter := headers.Get("Retry-After")
	resetTime := headers.Get("X-RateLimit-Reset")
	if retryAfter == "" {
		t.Error("Retry-After header missing when rate limited")
	}
	if resetTime == "" {
		t.Error("X-RateLimit-Reset header missing when rate limited")
	}
}

func TestIntegration_RateLimiting_Disabled(t *testing.T) {
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
	for i := 0; i < 1000; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
}

func TestIntegration_RateLimiting_Stats(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 10.0,
			Burst:             5,
		},
		Endpoints: map[string]*EndpointRateLimitConfig{
			"/api/tx": {
				RequestsPerSecond: 50.0,
				Burst:             10,
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
	for i := 0; i < 5; i++ {
		rl.Allow("/api/tx", "192.168.1.1", "")
	}
	stats := rl.GetStats()
	if stats["enabled"] != true {
		t.Error("stats should show enabled")
	}
	if stats["active_endpoints"].(int) < 1 {
		t.Error("should have at least 1 active endpoint")
	}
	if stats["default_rps"].(float64) != 10.0 {
		t.Errorf("expected default_rps 10.0, got %v", stats["default_rps"])
	}
	if stats["default_burst"].(int) != 5 {
		t.Errorf("expected default_burst 5, got %v", stats["default_burst"])
	}
}

func TestIntegration_RateLimiting_AutoCleanup(t *testing.T) {
	cfg := DefaultRateLimitConfig()
	cfg.Enabled = true
	cfg.CleanupInterval = 50 * time.Millisecond
	cfg.StorageType = "memory"
	rl, err := NewEnhancedRateLimiter(cfg)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer rl.Stop()
	rl.Allow("/endpoint1", "ip1", "")
	rl.Allow("/endpoint2", "ip2", "")
	stats := rl.GetStats()
	initialEndpoints := stats["active_endpoints"].(int)
	if initialEndpoints != 2 {
		t.Errorf("expected 2 active endpoints, got %d", initialEndpoints)
	}
	time.Sleep(100 * time.Millisecond)
	rl.Allow("/endpoint1", "ip1", "")
	time.Sleep(100 * time.Millisecond)
	rl.Allow("/endpoint2", "ip2", "")
	stats = rl.GetStats()
	if stats["active_endpoints"].(int) != 2 {
		t.Errorf("expected 2 active endpoints, got %d", stats["active_endpoints"])
	}
}

func TestIntegration_RateLimiting_UserLevel(t *testing.T) {
	cfg := &RateLimitConfig{
		Default: &EndpointRateLimitConfig{
			RequestsPerSecond: 10.0,
			Burst:             5,
		},
		Endpoints:        make(map[string]*EndpointRateLimitConfig),
		APIKeyMultiplier: 5.0,
		Enabled:          true,
		ByIP:             false,
		ByUser:           true,
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
	users := []string{"user1", "user2"}
	for _, userID := range users {
		currentUser := userID
		middleware := RateLimitMiddleware(
			rl,
			func(r *http.Request) string { return "192.168.1.1" },
			func(r *http.Request) string { return "" },
			func(r *http.Request) string { return currentUser },
		)
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			rr := httptest.NewRecorder()
			middleware(handler).ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("user %s request %d: expected status 200, got %d", userID, i+1, rr.Code)
			}
		}
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
		if rr.Code != http.StatusTooManyRequests {
			t.Errorf("user %s: expected status 429, got %d", userID, rr.Code)
		}
	}
}

func TestIntegration_RateLimiting_MixedAuth(t *testing.T) {
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
	apiKey := "test-key"
	err = rl.RegisterAPIKey(apiKey, "user", "basic", 5.0, time.Time{})
	if err != nil {
		t.Fatalf("failed to register API key: %v", err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := RateLimitMiddleware(
		rl,
		func(r *http.Request) string { return "192.168.1.1" },
		func(r *http.Request) string {
			return r.Header.Get("X-API-Key")
		},
		nil,
	)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rr := httptest.NewRecorder()
		middleware(handler).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("unauthenticated request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("unauthenticated: expected status 429, got %d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-API-Key", apiKey)
	rr = httptest.NewRecorder()
	middleware(handler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("authenticated: expected status 200, got %d", rr.Code)
	}
}
