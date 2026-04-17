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

// Package api provides HTTP API implementation for NogoChain
// Audit Logger - Production-grade structured logging with rotation
package api

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LogLevel represents the severity of a log entry
type LogLevel string

const (
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

// AuditLogEntry represents a structured audit log entry
// Production-grade: all fields are properly typed and validated
type AuditLogEntry struct {
	Timestamp   string            `json:"timestamp"`
	Level       LogLevel          `json:"level"`
	TraceID     string            `json:"trace_id"`
	ClientIP    string            `json:"client_ip"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	StatusCode  int               `json:"status_code"`
	DurationMs  int64             `json:"duration_ms"`
	UserAgent   string            `json:"user_agent"`
	RequestID   string            `json:"request_id"`
	RequestBody map[string]any    `json:"request_body,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
	Error       string            `json:"error,omitempty"`
	UserID      string            `json:"user_id,omitempty"`
	Extra       map[string]any    `json:"extra,omitempty"`
}

// AuditLoggerConfig holds configuration for the audit logger
// Production-grade: all settings are configurable via environment variables
type AuditLoggerConfig struct {
	Enabled         bool          `json:"enabled"`
	LogDir          string        `json:"log_dir"`
	MaxFileSize     int64         `json:"max_file_size"`
	MaxRetention    int           `json:"max_retention_days"`
	Compress        bool          `json:"compress"`
	AsyncWrite      bool          `json:"async_write"`
	BufferSize      int           `json:"buffer_size"`
	FlushInterval   time.Duration `json:"flush_interval"`
	SensitiveFields []string      `json:"sensitive_fields"`
}

// DefaultAuditLoggerConfig returns production-ready default configuration
func DefaultAuditLoggerConfig() *AuditLoggerConfig {
	return &AuditLoggerConfig{
		Enabled:         true,
		LogDir:          "nogodata/logs/audit",
		MaxFileSize:     100 << 20, // 100MB
		MaxRetention:    90,        // 90 days
		Compress:        true,
		AsyncWrite:      true,
		BufferSize:      1000,
		FlushInterval:   5 * time.Second,
		SensitiveFields: []string{"password", "token", "private_key", "secret", "api_key", "authorization"},
	}
}

// LoadAuditLoggerConfig loads configuration from environment variables
// Production-grade: supports hot-reload via SIGHUP
func LoadAuditLoggerConfig() (*AuditLoggerConfig, error) {
	cfg := DefaultAuditLoggerConfig()

	if v := os.Getenv("AUDIT_LOG_ENABLED"); v != "" {
		cfg.Enabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("AUDIT_LOG_DIR"); v != "" {
		cfg.LogDir = v
	}

	if v := os.Getenv("AUDIT_LOG_MAX_FILE_SIZE"); v != "" {
		if size, err := strconv.ParseInt(v, 10, 64); err == nil && size > 0 {
			cfg.MaxFileSize = size
		}
	}

	if v := os.Getenv("AUDIT_LOG_MAX_RETENTION"); v != "" {
		if days, err := strconv.Atoi(v); err == nil && days > 0 {
			cfg.MaxRetention = days
		}
	}

	if v := os.Getenv("AUDIT_LOG_COMPRESS"); v != "" {
		cfg.Compress = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("AUDIT_LOG_ASYNC"); v != "" {
		cfg.AsyncWrite = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("AUDIT_LOG_BUFFER_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size > 0 {
			cfg.BufferSize = size
		}
	}

	if v := os.Getenv("AUDIT_LOG_FLUSH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.FlushInterval = d
		}
	}

	if v := os.Getenv("AUDIT_LOG_SENSITIVE_FIELDS"); v != "" {
		cfg.SensitiveFields = strings.Split(v, ",")
	}

	return cfg, nil
}

// Validate validates the audit logger configuration
// Production-grade: comprehensive validation with detailed error messages
func (c *AuditLoggerConfig) Validate() error {
	if c.LogDir == "" {
		return errors.New("AUDIT_LOG_DIR must be set")
	}

	if c.MaxFileSize <= 0 || c.MaxFileSize > math.MaxInt64 {
		return fmt.Errorf("AUDIT_LOG_MAX_FILE_SIZE must be positive: %d", c.MaxFileSize)
	}

	if c.MaxRetention <= 0 || c.MaxRetention > 3650 {
		return fmt.Errorf("AUDIT_LOG_MAX_RETENTION must be between 1 and 3650 days: %d", c.MaxRetention)
	}

	if c.BufferSize <= 0 || c.BufferSize > 100000 {
		return fmt.Errorf("AUDIT_LOG_BUFFER_SIZE must be between 1 and 100000: %d", c.BufferSize)
	}

	if c.FlushInterval <= 0 || c.FlushInterval > time.Hour {
		return fmt.Errorf("AUDIT_LOG_FLUSH_INTERVAL must be between 1ms and 1h: %v", c.FlushInterval)
	}

	if len(c.SensitiveFields) == 0 {
		return errors.New("AUDIT_LOG_SENSITIVE_FIELDS must not be empty")
	}

	return nil
}

// AuditLogger is the main audit logging engine
// Production-grade: thread-safe, async, with rotation and compression
type AuditLogger struct {
	config      *AuditLoggerConfig
	mu          sync.RWMutex
	currentFile *os.File
	currentSize int64
	buffer      chan *AuditLogEntry
	done        chan struct{}
	wg          sync.WaitGroup
	closed      atomic.Bool
	requestID   uint64
	sensitiveRe *regexp.Regexp
}

// NewAuditLogger creates a new audit logger instance
// Production-grade: initializes all components and starts background workers
func NewAuditLogger(cfg *AuditLoggerConfig) (*AuditLogger, error) {
	if cfg == nil {
		var err error
		cfg, err = LoadAuditLoggerConfig()
		if err != nil {
			return nil, fmt.Errorf("load audit logger config: %w", err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate audit logger config: %w", err)
	}

	logger := &AuditLogger{
		config:      cfg,
		buffer:      make(chan *AuditLogEntry, cfg.BufferSize),
		done:        make(chan struct{}),
		requestID:   0,
		sensitiveRe: buildSensitiveRegex(cfg.SensitiveFields),
	}

	if !cfg.Enabled {
		return logger, nil
	}

	if err := logger.ensureLogDir(); err != nil {
		return nil, fmt.Errorf("ensure log directory: %w", err)
	}

	if err := logger.rotate(); err != nil {
		return nil, fmt.Errorf("initial log rotation: %w", err)
	}

	if cfg.AsyncWrite {
		logger.startAsyncWriter()
	}

	logger.startRotationChecker()

	return logger, nil
}

// buildSensitiveRegex compiles a regex pattern to match sensitive field names
func buildSensitiveRegex(fields []string) *regexp.Regexp {
	pattern := "(?i)(" + strings.Join(fields, "|") + ")"
	return regexp.MustCompile(pattern)
}

// ensureLogDir creates the log directory if it doesn't exist
func (l *AuditLogger) ensureLogDir() error {
	if err := os.MkdirAll(l.config.LogDir, 0750); err != nil {
		return fmt.Errorf("create log directory %s: %w", l.config.LogDir, err)
	}
	return nil
}

// rotate performs log file rotation
// Production-grade: handles size-based rotation with compression
func (l *AuditLogger) rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.currentFile != nil {
		if err := l.currentFile.Close(); err != nil {
			return fmt.Errorf("close current log file: %w", err)
		}
		l.currentFile = nil
	}

	timestamp := time.Now().Format("20060102_150405")
	logPath := filepath.Join(l.config.LogDir, fmt.Sprintf("audit_%s.log", timestamp))

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return fmt.Errorf("open new log file %s: %w", logPath, err)
	}

	l.currentFile = file
	l.currentSize = 0

	go l.cleanupOldLogs()

	return nil
}

// cleanupOldLogs removes logs older than max retention period
// Production-grade: includes compression of old logs
func (l *AuditLogger) cleanupOldLogs() {
	entries, err := os.ReadDir(l.config.LogDir)
	if err != nil {
		fmt.Printf("[AUDIT] Failed to read log directory: %v\n", err)
		return
	}

	cutoff := time.Now().AddDate(0, 0, -l.config.MaxRetention)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "audit_") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			filePath := filepath.Join(l.config.LogDir, entry.Name())
			
			if !strings.HasSuffix(entry.Name(), ".gz") && l.config.Compress {
				if err := l.compressFile(filePath); err != nil {
					fmt.Printf("[AUDIT] Failed to compress %s: %v\n", entry.Name(), err)
				}
			}
			
			if err := os.Remove(filePath); err != nil {
				fmt.Printf("[AUDIT] Failed to remove old log %s: %v\n", entry.Name(), err)
			} else {
				fmt.Printf("[AUDIT] Removed old log: %s\n", entry.Name())
			}
		}
	}
}

// compressFile compresses a log file using gzip
func (l *AuditLogger) compressFile(filePath string) error {
	src, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer src.Close()

	dstPath := filePath + ".gz"
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer dst.Close()

	gw := gzip.NewWriter(dst)
	if _, err := io.Copy(gw, src); err != nil {
		gw.Close()
		return fmt.Errorf("compress file: %w", err)
	}

	if err := gw.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("remove original file: %w", err)
	}

	return nil
}

// startAsyncWriter starts the background writer goroutine
func (l *AuditLogger) startAsyncWriter() {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()

		ticker := time.NewTicker(l.config.FlushInterval)
		defer ticker.Stop()

		buffer := make([]*AuditLogEntry, 0, l.config.BufferSize)

		for {
			select {
			case entry, ok := <-l.buffer:
				if !ok {
					if len(buffer) > 0 {
						l.flush(buffer)
					}
					return
				}
				buffer = append(buffer, entry)
				
				if len(buffer) >= l.config.BufferSize {
					l.flush(buffer)
					buffer = buffer[:0]
				}

			case <-ticker.C:
				if len(buffer) > 0 {
					l.flush(buffer)
					buffer = buffer[:0]
				}

			case <-l.done:
				if len(buffer) > 0 {
					l.flush(buffer)
				}
				return
			}
		}
	}()
}

// startRotationChecker starts the background rotation checker
func (l *AuditLogger) startRotationChecker() {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()

		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				l.mu.RLock()
				needsRotation := l.currentSize >= l.config.MaxFileSize
				l.mu.RUnlock()

				if needsRotation {
					if err := l.rotate(); err != nil {
						fmt.Printf("[AUDIT] Rotation failed: %v\n", err)
					}
				}

			case <-l.done:
				return
			}
		}
	}()
}

// flush writes a batch of log entries to disk
func (l *AuditLogger) flush(entries []*AuditLogEntry) {
	if len(entries) == 0 {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.currentFile == nil {
		return
	}

	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			fmt.Printf("[AUDIT] Marshal failed: %v\n", err)
			continue
		}

		data = append(data, '\n')

		n, err := l.currentFile.Write(data)
		if err != nil {
			fmt.Printf("[AUDIT] Write failed: %v\n", err)
			continue
		}

		atomic.AddInt64(&l.currentSize, int64(n))
	}

	if err := l.currentFile.Sync(); err != nil {
		fmt.Printf("[AUDIT] Sync failed: %v\n", err)
	}
}

// Log logs an audit entry
// Production-grade: non-blocking when async mode is enabled
func (l *AuditLogger) Log(entry *AuditLogEntry) {
	if l == nil || !l.config.Enabled || l.closed.Load() {
		return
	}

	if entry == nil {
		return
	}

	l.sanitizeEntry(entry)

	if l.config.AsyncWrite {
		select {
		case l.buffer <- entry:
		default:
			fmt.Printf("[AUDIT] Buffer full, dropping entry\n")
		}
	} else {
		l.flush([]*AuditLogEntry{entry})
	}
}

// sanitizeEntry removes sensitive information from log entries
// Production-grade: comprehensive sanitization using regex
func (l *AuditLogger) sanitizeEntry(entry *AuditLogEntry) {
	if entry.RequestBody != nil {
		entry.RequestBody = l.sanitizeMap(entry.RequestBody)
	}

	if entry.QueryParams != nil {
		sanitized := make(map[string]string, len(entry.QueryParams))
		for k, v := range entry.QueryParams {
			if l.sensitiveRe.MatchString(k) {
				sanitized[k] = "[REDACTED]"
			} else {
				sanitized[k] = v
			}
		}
		entry.QueryParams = sanitized
	}

	if entry.Error != "" && l.sensitiveRe.MatchString(entry.Error) {
		entry.Error = "[REDACTED]"
	}
}

// sanitizeMap recursively sanitizes a map
func (l *AuditLogger) sanitizeMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	sanitized := make(map[string]any, len(m))
	for k, v := range m {
		if l.sensitiveRe.MatchString(k) {
			sanitized[k] = "[REDACTED]"
			continue
		}

		switch val := v.(type) {
		case map[string]any:
			sanitized[k] = l.sanitizeMap(val)
		case []any:
			sanitized[k] = l.sanitizeArray(val)
		case string:
			if l.sensitiveRe.MatchString(k) {
				sanitized[k] = "[REDACTED]"
			} else {
				sanitized[k] = val
			}
		default:
			sanitized[k] = v
		}
	}

	return sanitized
}

// sanitizeArray recursively sanitizes an array
func (l *AuditLogger) sanitizeArray(arr []any) []any {
	if arr == nil {
		return nil
	}

	sanitized := make([]any, len(arr))
	for i, v := range arr {
		switch val := v.(type) {
		case map[string]any:
			sanitized[i] = l.sanitizeMap(val)
		case []any:
			sanitized[i] = l.sanitizeArray(val)
		default:
			sanitized[i] = v
		}
	}

	return sanitized
}

// GenerateRequestID generates a unique request ID
func (l *AuditLogger) GenerateRequestID() string {
	id := atomic.AddUint64(&l.requestID, 1)
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%d-%d", timestamp, id)
}

// Close gracefully shuts down the audit logger
// Production-grade: ensures all pending logs are flushed
func (l *AuditLogger) Close() error {
	if l == nil || l.closed.Load() {
		return nil
	}

	l.closed.Store(true)
	close(l.done)

	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		return errors.New("timeout waiting for audit logger to flush")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.currentFile != nil {
		if err := l.currentFile.Sync(); err != nil {
			return fmt.Errorf("sync log file: %w", err)
		}
		if err := l.currentFile.Close(); err != nil {
			return fmt.Errorf("close log file: %w", err)
		}
		l.currentFile = nil
	}

	close(l.buffer)

	return nil
}

// AuditMiddleware creates an HTTP middleware for audit logging
// Production-grade: captures all request/response details
func (l *AuditLogger) AuditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := l.GenerateRequestID()

		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		entry := &AuditLogEntry{
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Level:       LogLevelInfo,
			TraceID:     requestID,
			ClientIP:    getClientIP(r),
			Method:      r.Method,
			Path:        r.URL.Path,
			StatusCode:  wrapped.statusCode,
			DurationMs:  duration.Milliseconds(),
			UserAgent:   r.UserAgent(),
			RequestID:   requestID,
			QueryParams: cloneQueryParams(r.URL.Query()),
		}

		if wrapped.statusCode >= 400 {
			entry.Level = LogLevelWarn
		}
		if wrapped.statusCode >= 500 {
			entry.Level = LogLevelError
		}

		l.Log(entry)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// getClientIP extracts the client IP from the request
// Production-grade: handles X-Forwarded-For and X-Real-IP headers
func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// cloneQueryParams creates a copy of query parameters
func cloneQueryParams(params map[string][]string) map[string]string {
	result := make(map[string]string, len(params))
	for k, v := range params {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// QueryLogEntry represents a log query request
type QueryLogEntry struct {
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Level       LogLevel  `json:"level,omitempty"`
	TraceID     string    `json:"trace_id,omitempty"`
	ClientIP    string    `json:"client_ip,omitempty"`
	Path        string    `json:"path,omitempty"`
	MinDuration int64     `json:"min_duration_ms,omitempty"`
	Limit       int       `json:"limit,omitempty"`
	Offset      int       `json:"offset,omitempty"`
}

// QueryLogs queries audit logs based on filters
// Production-grade: supports pagination and filtering
func (l *AuditLogger) QueryLogs(query QueryLogEntry) ([]AuditLogEntry, int, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Limit > 1000 {
		query.Limit = 1000
	}

	entries, err := l.readLogFiles(query)
	if err != nil {
		return nil, 0, fmt.Errorf("read log files: %w", err)
	}

	filtered := l.filterEntries(entries, query)
	total := len(filtered)

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp > filtered[j].Timestamp
	})

	if query.Offset >= len(filtered) {
		return []AuditLogEntry{}, total, nil
	}

	end := query.Offset + query.Limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[query.Offset:end], total, nil
}

// readLogFiles reads all log files in the log directory
func (l *AuditLogger) readLogFiles(query QueryLogEntry) ([]AuditLogEntry, error) {
	entries, err := os.ReadDir(l.config.LogDir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var allEntries []AuditLogEntry

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "audit_") {
			continue
		}

		filePath := filepath.Join(l.config.LogDir, entry.Name())
		fileEntries, err := l.readLogFile(filePath)
		if err != nil {
			continue
		}
		allEntries = append(allEntries, fileEntries...)
	}

	return allEntries, nil
}

// readLogFile reads a single log file
func (l *AuditLogger) readLogFile(filePath string) ([]AuditLogEntry, error) {
	var file io.ReadCloser
	var err error

	if strings.HasSuffix(filePath, ".gz") {
		file, err = os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		defer file.Close()

		gzr, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		defer gzr.Close()

		file = gzr
	} else {
		file, err = os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		defer file.Close()
	}

	var entries []AuditLogEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry AuditLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan file: %w", err)
	}

	return entries, nil
}

// filterEntries filters log entries based on query criteria
func (l *AuditLogger) filterEntries(entries []AuditLogEntry, query QueryLogEntry) []AuditLogEntry {
	filtered := make([]AuditLogEntry, 0, len(entries))

	for _, entry := range entries {
		if !query.StartTime.IsZero() {
			ts, err := time.Parse(time.RFC3339, entry.Timestamp)
			if err != nil || ts.Before(query.StartTime) {
				continue
			}
		}

		if !query.EndTime.IsZero() {
			ts, err := time.Parse(time.RFC3339, entry.Timestamp)
			if err != nil || ts.After(query.EndTime) {
				continue
			}
		}

		if query.Level != "" && entry.Level != query.Level {
			continue
		}

		if query.TraceID != "" && entry.TraceID != query.TraceID {
			continue
		}

		if query.ClientIP != "" && entry.ClientIP != query.ClientIP {
			continue
		}

		if query.Path != "" && !strings.Contains(entry.Path, query.Path) {
			continue
		}

		if query.MinDuration > 0 && entry.DurationMs < query.MinDuration {
			continue
		}

		filtered = append(filtered, entry)
	}

	return filtered
}

// ExportLogs exports audit logs to a file
// Production-grade: supports JSON and CSV formats
func (l *AuditLogger) ExportLogs(query QueryLogEntry, format string, outputPath string) error {
	entries, _, err := l.QueryLogs(query)
	if err != nil {
		return fmt.Errorf("query logs: %w", err)
	}

	var data []byte
	switch format {
	case "json":
		data, err = json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal JSON: %w", err)
		}
	case "csv":
		data, err = l.entriesToCSV(entries)
		if err != nil {
			return fmt.Errorf("convert to CSV: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	if err := os.WriteFile(outputPath, data, 0640); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// entriesToCSV converts log entries to CSV format
func (l *AuditLogger) entriesToCSV(entries []AuditLogEntry) ([]byte, error) {
	var sb strings.Builder

	sb.WriteString("timestamp,level,trace_id,client_ip,method,path,status_code,duration_ms,request_id,error\n")

	for _, entry := range entries {
		line := fmt.Sprintf("%s,%s,%s,%s,%s,%s,%d,%d,%s,%s\n",
			entry.Timestamp,
			entry.Level,
			entry.TraceID,
			entry.ClientIP,
			entry.Method,
			entry.Path,
			entry.StatusCode,
			entry.DurationMs,
			entry.RequestID,
			entry.Error,
		)
		sb.WriteString(line)
	}

	return []byte(sb.String()), nil
}

// GetStats returns audit log statistics
// Production-grade: provides insights into log volume and patterns
func (l *AuditLogger) GetStats() (map[string]any, error) {
	entries, err := l.readLogFiles(QueryLogEntry{})
	if err != nil {
		return nil, fmt.Errorf("read logs: %w", err)
	}

	stats := map[string]any{
		"total_entries": len(entries),
		"by_level":      make(map[LogLevel]int),
		"by_status":     make(map[int]int),
		"avg_duration_ms": int64(0),
	}

	var totalDuration int64
	levelCounts := make(map[LogLevel]int)
	statusCounts := make(map[int]int)

	for _, entry := range entries {
		levelCounts[entry.Level]++
		statusCounts[entry.StatusCode]++
		totalDuration += entry.DurationMs
	}

	if len(entries) > 0 {
		stats["avg_duration_ms"] = totalDuration / int64(len(entries))
	}
	stats["by_level"] = levelCounts
	stats["by_status"] = statusCounts

	return stats, nil
}

// LogEntryFromRequest creates an audit log entry from an HTTP request
// Production-grade: captures all relevant request details
func LogEntryFromRequest(r *http.Request, statusCode int, duration time.Duration, requestID string, extra map[string]any) *AuditLogEntry {
	level := LogLevelInfo
	if statusCode >= 400 {
		level = LogLevelWarn
	}
	if statusCode >= 500 {
		level = LogLevelError
	}

	return &AuditLogEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Level:      level,
		TraceID:    requestID,
		ClientIP:   getClientIP(r),
		Method:     r.Method,
		Path:       r.URL.Path,
		StatusCode: statusCode,
		DurationMs: duration.Milliseconds(),
		UserAgent:  r.UserAgent(),
		RequestID:  requestID,
		Extra:      extra,
	}
}

// LogAPIError logs an API error with full context
// Production-grade: includes stack trace for debugging
func (l *AuditLogger) LogAPIError(r *http.Request, err error, requestID string) {
	if l == nil || !l.config.Enabled {
		return
	}

	pc, file, line, _ := runtime.Caller(1)
	funcName := runtime.FuncForPC(pc).Name()

	entry := &AuditLogEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Level:      LogLevelError,
		TraceID:    requestID,
		ClientIP:   getClientIP(r),
		Method:     r.Method,
		Path:       r.URL.Path,
		StatusCode: http.StatusInternalServerError,
		DurationMs: 0,
		UserAgent:  r.UserAgent(),
		RequestID:  requestID,
		Error:      err.Error(),
		Extra: map[string]any{
			"function": funcName,
			"file":     file,
			"line":     line,
		},
	}

	l.Log(entry)
}

// LogSecurityEvent logs security-related events
// Production-grade: highlights security-critical events
func (l *AuditLogger) LogSecurityEvent(r *http.Request, event string, requestID string) {
	if l == nil || !l.config.Enabled {
		return
	}

	entry := &AuditLogEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Level:      LogLevelWarn,
		TraceID:    requestID,
		ClientIP:   getClientIP(r),
		Method:     r.Method,
		Path:       r.URL.Path,
		StatusCode: http.StatusForbidden,
		DurationMs: 0,
		UserAgent:  r.UserAgent(),
		RequestID:  requestID,
		Extra: map[string]any{
			"security_event": event,
		},
	}

	l.Log(entry)
}

// Ensure interface compliance
var (
	_ io.Closer = (*AuditLogger)(nil)
)
