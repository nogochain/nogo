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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAuditLoggerConfigValidation tests configuration validation
func TestAuditLoggerConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *AuditLoggerConfig
		wantErr bool
	}{
		{
			name: "valid default config",
			config: &AuditLoggerConfig{
				Enabled:         true,
				LogDir:          "logs/audit",
				MaxFileSize:     100 << 20,
				MaxRetention:    90,
				Compress:        true,
				AsyncWrite:      true,
				BufferSize:      1000,
				FlushInterval:   5 * time.Second,
				SensitiveFields: []string{"password", "token"},
			},
			wantErr: false,
		},
		{
			name: "empty log dir",
			config: &AuditLoggerConfig{
				Enabled:         true,
				LogDir:          "",
				MaxFileSize:     100 << 20,
				MaxRetention:    90,
				BufferSize:      1000,
				FlushInterval:   5 * time.Second,
				SensitiveFields: []string{"password"},
			},
			wantErr: true,
		},
		{
			name: "invalid max file size",
			config: &AuditLoggerConfig{
				Enabled:         true,
				LogDir:          "logs",
				MaxFileSize:     0,
				MaxRetention:    90,
				BufferSize:      1000,
				FlushInterval:   5 * time.Second,
				SensitiveFields: []string{"password"},
			},
			wantErr: true,
		},
		{
			name: "invalid retention days",
			config: &AuditLoggerConfig{
				Enabled:         true,
				LogDir:          "logs",
				MaxFileSize:     100 << 20,
				MaxRetention:    0,
				BufferSize:      1000,
				FlushInterval:   5 * time.Second,
				SensitiveFields: []string{"password"},
			},
			wantErr: true,
		},
		{
			name: "empty sensitive fields",
			config: &AuditLoggerConfig{
				Enabled:       true,
				LogDir:        "logs",
				MaxFileSize:   100 << 20,
				MaxRetention:  90,
				BufferSize:    1000,
				FlushInterval: 5 * time.Second,
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

// TestAuditLoggerCreation tests logger initialization
func TestAuditLoggerCreation(t *testing.T) {
	tmpDir := t.TempDir()
	
	config := &AuditLoggerConfig{
		Enabled:         true,
		LogDir:          tmpDir,
		MaxFileSize:     100 << 20,
		MaxRetention:    90,
		Compress:        false,
		AsyncWrite:      true,
		BufferSize:      100,
		FlushInterval:   100 * time.Millisecond,
		SensitiveFields: []string{"password", "token", "secret"},
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	if logger == nil {
		t.Fatal("Audit logger should not be nil")
	}

	if !logger.config.Enabled {
		t.Error("Audit logger should be enabled")
	}
}

// TestAuditLoggerDisabled tests disabled logger behavior
func TestAuditLoggerDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	
	config := &AuditLoggerConfig{
		Enabled:         false,
		LogDir:          tmpDir,
		MaxFileSize:     100 << 20,
		MaxRetention:    90,
		BufferSize:      1000,
		FlushInterval:   5 * time.Second,
		SensitiveFields: []string{"password"},
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create disabled audit logger: %v", err)
	}
	defer logger.Close()

	entry := &AuditLogEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Level:      LogLevelInfo,
		TraceID:    "test-trace-id",
		ClientIP:   "127.0.0.1",
		Method:     "GET",
		Path:       "/test",
		StatusCode: 200,
		DurationMs: 10,
		RequestID:  "test-request-id",
	}

	logger.Log(entry)

	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	if len(files) > 0 {
		t.Error("No log files should be created when logger is disabled")
	}
}

// TestAuditLogEntrySerialization tests JSON serialization
func TestAuditLogEntrySerialization(t *testing.T) {
	entry := &AuditLogEntry{
		Timestamp:  "2026-01-01T12:00:00Z",
		Level:      LogLevelInfo,
		TraceID:    "trace-123",
		ClientIP:   "192.168.1.1",
		Method:     "POST",
		Path:       "/api/tx",
		StatusCode: 200,
		DurationMs: 150,
		UserAgent:  "Mozilla/5.0",
		RequestID:  "req-456",
		RequestBody: map[string]any{
			"from":   "addr1",
			"to":     "addr2",
			"amount": 1000,
		},
		QueryParams: map[string]string{
			"limit":  "50",
			"cursor": "0",
		},
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal entry: %v", err)
	}

	var decoded AuditLogEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal entry: %v", err)
	}

	if decoded.Timestamp != entry.Timestamp {
		t.Errorf("Timestamp mismatch: got %s, want %s", decoded.Timestamp, entry.Timestamp)
	}
	if decoded.Level != entry.Level {
		t.Errorf("Level mismatch: got %s, want %s", decoded.Level, entry.Level)
	}
	if decoded.TraceID != entry.TraceID {
		t.Errorf("TraceID mismatch: got %s, want %s", decoded.TraceID, entry.TraceID)
	}
}

// TestSensitiveDataSanitization tests that sensitive data is redacted
func TestSensitiveDataSanitization(t *testing.T) {
	tmpDir := t.TempDir()
	
	config := &AuditLoggerConfig{
		Enabled:         true,
		LogDir:          tmpDir,
		MaxFileSize:     100 << 20,
		MaxRetention:    90,
		AsyncWrite:      false,
		BufferSize:      100,
		FlushInterval:   time.Second,
		SensitiveFields: []string{"password", "token", "secret", "api_key"},
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	entry := &AuditLogEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Level:      LogLevelInfo,
		TraceID:    "test-trace",
		ClientIP:   "127.0.0.1",
		Method:     "POST",
		Path:       "/api/login",
		StatusCode: 200,
		DurationMs: 50,
		RequestID:  "test-req",
		RequestBody: map[string]any{
			"username": "user123",
			"password": "secret123",
			"token":    "my-token",
			"data": map[string]any{
				"secret":    "hidden",
				"api_key":   "key-123",
				"public":    "visible",
			},
		},
	}

	logger.Log(entry)

	if entry.RequestBody["password"] != "[REDACTED]" {
		t.Errorf("Password should be redacted, got: %v", entry.RequestBody["password"])
	}
	if entry.RequestBody["token"] != "[REDACTED]" {
		t.Errorf("Token should be redacted, got: %v", entry.RequestBody["token"])
	}

	data := entry.RequestBody["data"].(map[string]any)
	if data["secret"] != "[REDACTED]" {
		t.Errorf("Nested secret should be redacted, got: %v", data["secret"])
	}
	if data["api_key"] != "[REDACTED]" {
		t.Errorf("Nested api_key should be redacted, got: %v", data["api_key"])
	}
	if data["public"] != "visible" {
		t.Errorf("Public field should remain visible, got: %v", data["public"])
	}
}

// TestAuditLoggerConcurrentSafety tests thread safety
func TestAuditLoggerConcurrentSafety(t *testing.T) {
	tmpDir := t.TempDir()
	
	config := &AuditLoggerConfig{
		Enabled:         true,
		LogDir:          tmpDir,
		MaxFileSize:     100 << 20,
		MaxRetention:    90,
		AsyncWrite:      true,
		BufferSize:      1000,
		FlushInterval:   100 * time.Millisecond,
		SensitiveFields: []string{"password"},
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	var wg sync.WaitGroup
	numGoroutines := 10
	entriesPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < entriesPerGoroutine; j++ {
				entry := &AuditLogEntry{
					Timestamp:  time.Now().UTC().Format(time.RFC3339),
					Level:      LogLevelInfo,
					TraceID:    "trace-" + string(rune(id)),
					ClientIP:   "127.0.0.1",
					Method:     "GET",
					Path:       "/test",
					StatusCode: 200,
					DurationMs: int64(j),
					RequestID:  "req-" + string(rune(id)) + "-" + string(rune(j)),
				}
				logger.Log(entry)
			}
		}(i)
	}

	wg.Wait()

	time.Sleep(500 * time.Millisecond)

	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	if len(files) == 0 {
		t.Error("Expected log files to be created")
	}
}

// TestAuditMiddleware tests HTTP middleware integration
func TestAuditMiddleware(t *testing.T) {
	tmpDir := t.TempDir()
	
	config := &AuditLoggerConfig{
		Enabled:         true,
		LogDir:          tmpDir,
		MaxFileSize:     100 << 20,
		MaxRetention:    90,
		AsyncWrite:      false,
		BufferSize:      100,
		FlushInterval:   time.Second,
		SensitiveFields: []string{"password"},
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	auditedHandler := logger.AuditMiddleware(handler)

	req := httptest.NewRequest("GET", "/test/path?param=value", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("User-Agent", "TestAgent/1.0")

	rr := httptest.NewRecorder()
	auditedHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	time.Sleep(100 * time.Millisecond)

	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	if len(files) == 0 {
		t.Error("Expected log file to be created by middleware")
	}
}

// TestRequestIDGeneration tests unique ID generation
func TestRequestIDGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	
	config := &AuditLoggerConfig{
		Enabled:         true,
		LogDir:          tmpDir,
		MaxFileSize:     100 << 20,
		MaxRetention:    90,
		BufferSize:      100,
		FlushInterval:   time.Second,
		SensitiveFields: []string{"password"},
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	ids := make(map[string]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := logger.GenerateRequestID()
			mu.Lock()
			defer mu.Unlock()
			if ids[id] {
				t.Errorf("Duplicate request ID generated: %s", id)
			}
			ids[id] = true
		}()
	}

	wg.Wait()

	if len(ids) != 100 {
		t.Errorf("Expected 100 unique IDs, got %d", len(ids))
	}
}

// TestLogRotation tests log file rotation
func TestLogRotation(t *testing.T) {
	tmpDir := t.TempDir()
	
	config := &AuditLoggerConfig{
		Enabled:         true,
		LogDir:          tmpDir,
		MaxFileSize:     1024,
		MaxRetention:    90,
		AsyncWrite:      false,
		BufferSize:      10,
		FlushInterval:   time.Millisecond,
		SensitiveFields: []string{"password"},
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	for i := 0; i < 100; i++ {
		entry := &AuditLogEntry{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Level:      LogLevelInfo,
			TraceID:    "trace-rotation",
			ClientIP:   "127.0.0.1",
			Method:     "GET",
			Path:       "/test",
			StatusCode: 200,
			DurationMs: 10,
			RequestID:  "req-" + string(rune(i)),
			Extra: map[string]any{
				"data": strings.Repeat("x", 100),
			},
		}
		logger.Log(entry)
	}

	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	if len(files) < 1 {
		t.Error("Expected at least one log file after rotation")
	}
}

// TestClientIPExtraction tests IP extraction from headers
func TestClientIPExtraction(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		remote   string
		expected string
	}{
		{
			name: "X-Forwarded-For single IP",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.1",
			},
			remote:   "10.0.0.1:12345",
			expected: "192.168.1.1",
		},
		{
			name: "X-Forwarded-For multiple IPs",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.1, 10.0.0.2, 172.16.0.1",
			},
			remote:   "10.0.0.1:12345",
			expected: "192.168.1.1",
		},
		{
			name: "X-Real-IP",
			headers: map[string]string{
				"X-Real-IP": "192.168.1.2",
			},
			remote:   "10.0.0.1:12345",
			expected: "192.168.1.2",
		},
		{
			name:    "RemoteAddr only",
			headers: map[string]string{},
			remote:   "192.168.1.3:12345",
			expected: "192.168.1.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remote
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			ip := getClientIP(req)
			if ip != tt.expected {
				t.Errorf("getClientIP() = %s, want %s", ip, tt.expected)
			}
		})
	}
}

// TestQueryLogs tests log querying functionality
func TestQueryLogs(t *testing.T) {
	tmpDir := t.TempDir()
	
	config := &AuditLoggerConfig{
		Enabled:         true,
		LogDir:          tmpDir,
		MaxFileSize:     100 << 20,
		MaxRetention:    90,
		AsyncWrite:      false,
		BufferSize:      100,
		FlushInterval:   time.Second,
		SensitiveFields: []string{"password"},
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	now := time.Now()
	entries := []*AuditLogEntry{
		{
			Timestamp:  now.Add(-2 * time.Hour).Format(time.RFC3339),
			Level:      LogLevelInfo,
			TraceID:    "trace-1",
			ClientIP:   "192.168.1.1",
			Method:     "GET",
			Path:       "/api/test1",
			StatusCode: 200,
			DurationMs: 50,
			RequestID:  "req-1",
		},
		{
			Timestamp:  now.Add(-1 * time.Hour).Format(time.RFC3339),
			Level:      LogLevelWarn,
			TraceID:    "trace-2",
			ClientIP:   "192.168.1.2",
			Method:     "POST",
			Path:       "/api/test2",
			StatusCode: 400,
			DurationMs: 100,
			RequestID:  "req-2",
		},
		{
			Timestamp:  now.Format(time.RFC3339),
			Level:      LogLevelError,
			TraceID:    "trace-3",
			ClientIP:   "192.168.1.1",
			Method:     "GET",
			Path:       "/api/test3",
			StatusCode: 500,
			DurationMs: 200,
			RequestID:  "req-3",
		},
	}

	for _, entry := range entries {
		logger.Log(entry)
	}

	query := QueryLogEntry{
		StartTime: now.Add(-3 * time.Hour),
		EndTime:   now,
		Limit:     10,
		Offset:    0,
	}

	results, total, err := logger.QueryLogs(query)
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}

	if total != len(entries) {
		t.Errorf("Expected %d total results, got %d", len(entries), total)
	}

	if len(results) != len(entries) {
		t.Errorf("Expected %d results, got %d", len(entries), len(results))
	}
}

// TestExportLogs tests log export functionality
func TestExportLogs(t *testing.T) {
	tmpDir := t.TempDir()
	
	config := &AuditLoggerConfig{
		Enabled:         true,
		LogDir:          tmpDir,
		MaxFileSize:     100 << 20,
		MaxRetention:    90,
		AsyncWrite:      false,
		BufferSize:      100,
		FlushInterval:   time.Second,
		SensitiveFields: []string{"password"},
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	entry := &AuditLogEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Level:      LogLevelInfo,
		TraceID:    "trace-export",
		ClientIP:   "192.168.1.1",
		Method:     "GET",
		Path:       "/api/export",
		StatusCode: 200,
		DurationMs: 50,
		RequestID:  "req-export",
	}
	logger.Log(entry)

	time.Sleep(100 * time.Millisecond)

	exportPath := filepath.Join(tmpDir, "export.json")
	query := QueryLogEntry{
		Limit:  10,
		Offset: 0,
	}

	err = logger.ExportLogs(query, "json", exportPath)
	if err != nil {
		t.Fatalf("ExportLogs failed: %v", err)
	}

	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("Failed to read exported file: %v", err)
	}

	if len(data) == 0 {
		t.Error("Exported file is empty")
	}

	var exported []AuditLogEntry
	if err := json.Unmarshal(data, &exported); err != nil {
		t.Fatalf("Failed to unmarshal exported JSON: %v", err)
	}

	if len(exported) == 0 {
		t.Error("No entries in exported file")
	}
}

// TestGetStats tests statistics generation
func TestGetStats(t *testing.T) {
	tmpDir := t.TempDir()
	
	config := &AuditLoggerConfig{
		Enabled:         true,
		LogDir:          tmpDir,
		MaxFileSize:     100 << 20,
		MaxRetention:    90,
		AsyncWrite:      false,
		BufferSize:      100,
		FlushInterval:   time.Second,
		SensitiveFields: []string{"password"},
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	entries := []*AuditLogEntry{
		{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Level:      LogLevelInfo,
			TraceID:    "trace-1",
			ClientIP:   "192.168.1.1",
			Method:     "GET",
			Path:       "/test",
			StatusCode: 200,
			DurationMs: 50,
			RequestID:  "req-1",
		},
		{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Level:      LogLevelWarn,
			TraceID:    "trace-2",
			ClientIP:   "192.168.1.2",
			Method:     "POST",
			Path:       "/test",
			StatusCode: 400,
			DurationMs: 100,
			RequestID:  "req-2",
		},
		{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Level:      LogLevelError,
			TraceID:    "trace-3",
			ClientIP:   "192.168.1.3",
			Method:     "GET",
			Path:       "/test",
			StatusCode: 500,
			DurationMs: 150,
			RequestID:  "req-3",
		},
	}

	for _, entry := range entries {
		logger.Log(entry)
	}

	time.Sleep(100 * time.Millisecond)

	stats, err := logger.GetStats()
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats["total_entries"] != 3 {
		t.Errorf("Expected 3 total entries, got %v", stats["total_entries"])
	}

	byLevel := stats["by_level"].(map[LogLevel]int)
	if byLevel[LogLevelInfo] != 1 {
		t.Errorf("Expected 1 INFO entry, got %d", byLevel[LogLevelInfo])
	}
	if byLevel[LogLevelWarn] != 1 {
		t.Errorf("Expected 1 WARN entry, got %d", byLevel[LogLevelWarn])
	}
	if byLevel[LogLevelError] != 1 {
		t.Errorf("Expected 1 ERROR entry, got %d", byLevel[LogLevelError])
	}
}

// TestLoggerClose tests graceful shutdown
func TestLoggerClose(t *testing.T) {
	tmpDir := t.TempDir()
	
	config := &AuditLoggerConfig{
		Enabled:         true,
		LogDir:          tmpDir,
		MaxFileSize:     100 << 20,
		MaxRetention:    90,
		AsyncWrite:      true,
		BufferSize:      100,
		FlushInterval:   100 * time.Millisecond,
		SensitiveFields: []string{"password"},
	}

	logger, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}

	for i := 0; i < 10; i++ {
		entry := &AuditLogEntry{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Level:      LogLevelInfo,
			TraceID:    "trace-close",
			ClientIP:   "127.0.0.1",
			Method:     "GET",
			Path:       "/test",
			StatusCode: 200,
			DurationMs: 10,
			RequestID:  "req-" + string(rune(i)),
		}
		logger.Log(entry)
	}

	err = logger.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	if len(files) == 0 {
		t.Error("Expected log file to exist after close")
	}
}

// TestLogLevelValues tests log level constants
func TestLogLevelValues(t *testing.T) {
	if LogLevelInfo != "INFO" {
		t.Errorf("LogLevelInfo = %s, want INFO", LogLevelInfo)
	}
	if LogLevelWarn != "WARN" {
		t.Errorf("LogLevelWarn = %s, want WARN", LogLevelWarn)
	}
	if LogLevelError != "ERROR" {
		t.Errorf("LogLevelError = %s, want ERROR", LogLevelError)
	}
}
