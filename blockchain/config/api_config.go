package config

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type HTTPAPIConfig struct {
	HTTPPort                  int                                `json:"http_port"`
	HTTPHost                  string                             `json:"http_host"`
	ReadTimeout               time.Duration                      `json:"read_timeout"`
	WriteTimeout              time.Duration                      `json:"write_timeout"`
	IdleTimeout               time.Duration                      `json:"idle_timeout"`
	MaxHeaderBytes            int                                `json:"max_header_bytes"`
	EnableGzip                bool                               `json:"enable_gzip"`
	GzipLevel                 int                                `json:"gzip_level"`
	EnableCORS                bool                               `json:"enable_cors"`
	CORSAllowOrigins          []string                           `json:"cors_allow_origins"`
	CORSAllowMethods          []string                           `json:"cors_allow_methods"`
	CORSAllowHeaders          []string                           `json:"cors_allow_headers"`
	CORSAllowCredentials      bool                               `json:"cors_allow_credentials"`
	CORSMaxAge                time.Duration                      `json:"cors_max_age"`
	RateLimitEnabled          bool                               `json:"rate_limit_enabled"`
	RateLimitRPS              int                                `json:"rate_limit_rps"`
	RateLimitBurst            int                                `json:"rate_limit_burst"`
	RateLimitByIP             bool                               `json:"rate_limit_by_ip"`
	RateLimitByAPIKey         bool                               `json:"rate_limit_by_api_key"`
	RateLimitAPIKeyMultiplier float64                            `json:"rate_limit_api_key_multiplier"`
	RateLimitEndpoints        map[string]EndpointRateLimitConfig `json:"rate_limit_endpoints"`
	TrustProxy                bool                               `json:"trust_proxy"`
	ProxyHeaders              []string                           `json:"proxy_headers"`
	TLSEnabled                bool                               `json:"tls_enabled"`
	TLSCertFile               string                             `json:"tls_cert_file"`
	TLSKeyFile                string                             `json:"tls_key_file"`
	TLSMinVersion             string                             `json:"tls_min_version"`
	TLSCipherSuites           []string                           `json:"tls_cipher_suites"`
	WebSocketEnabled          bool                               `json:"websocket_enabled"`
	WebSocketMaxConns         int                                `json:"websocket_max_conns"`
	WebSocketReadTimeout      time.Duration                      `json:"websocket_read_timeout"`
	WebSocketWriteTimeout     time.Duration                      `json:"websocket_write_timeout"`
	WebSocketMaxMsgSize       int                                `json:"websocket_max_msg_size"`
	APIKeyAuthEnabled         bool                               `json:"api_key_auth_enabled"`
	APIKeyHeader              string                             `json:"api_key_header"`
	APIKeyQuery               string                             `json:"api_key_query"`
	JWTEnabled                bool                               `json:"jwt_enabled"`
	JWTSecret                 string                             `json:"jwt_secret"`
	JWTExpiration             time.Duration                      `json:"jwt_expiration"`
	JWTIssuer                 string                             `json:"jwt_issuer"`
	LoggingEnabled            bool                               `json:"logging_enabled"`
	LoggingLevel              string                             `json:"logging_level"`
	LoggingFormat             string                             `json:"logging_format"`
	MetricsEnabled            bool                               `json:"metrics_enabled"`
	MetricsPath               string                             `json:"metrics_path"`
	MetricsPort               int                                `json:"metrics_port"`
	HealthCheckPath           string                             `json:"health_check_path"`
	HealthCheckEnabled        bool                               `json:"health_check_enabled"`
	PprofEnabled              bool                               `json:"pprof_enabled"`
	PprofPath                 string                             `json:"pprof_path"`
	AdminEnabled              bool                               `json:"admin_enabled"`
	AdminPath                 string                             `json:"admin_path"`
	AdminToken                string                             `json:"admin_token"`
	MaxRequestSize            int64                              `json:"max_request_size"`
	MaxResponseSize           int64                              `json:"max_response_size"`
	CompressionLevel          int                                `json:"compression_level"`
	CacheEnabled              bool                               `json:"cache_enabled"`
	CacheTTL                  time.Duration                      `json:"cache_ttl"`
	CacheMaxSize              int                                `json:"cache_max_size"`
	AuditLogEnabled           bool                               `json:"audit_log_enabled"`
	AuditLogDir               string                             `json:"audit_log_dir"`
	AuditLogMaxFileSize       int64                              `json:"audit_log_max_file_size"`
	AuditLogMaxRetention      int                                `json:"audit_log_max_retention_days"`
	AuditLogCompress          bool                               `json:"audit_log_compress"`
	AuditLogAsync             bool                               `json:"audit_log_async"`
	AuditLogBufferSize        int                                `json:"audit_log_buffer_size"`
	AuditLogFlushInterval     time.Duration                      `json:"audit_log_flush_interval"`
}

// EndpointRateLimitConfig represents rate limit config for a single endpoint
type EndpointRateLimitConfig struct {
	RequestsPerSecond float64 `json:"requests_per_second"`
	Burst             int     `json:"burst"`
}

func DefaultHTTPAPIConfig() *HTTPAPIConfig {
	return &HTTPAPIConfig{
		HTTPPort:                  8080,
		HTTPHost:                  "0.0.0.0",
		ReadTimeout:               30 * time.Second,
		WriteTimeout:              30 * time.Second,
		IdleTimeout:               120 * time.Second,
		MaxHeaderBytes:            1 << 20,
		EnableGzip:                true,
		GzipLevel:                 6,
		EnableCORS:                false,
		CORSAllowOrigins:          []string{"*"},
		CORSAllowMethods:          []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		CORSAllowHeaders:          []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		CORSAllowCredentials:      false,
		CORSMaxAge:                24 * time.Hour,
		RateLimitEnabled:          false,
		RateLimitRPS:              100,
		RateLimitBurst:            200,
		RateLimitByIP:             true,
		RateLimitByAPIKey:         false,
		RateLimitAPIKeyMultiplier: 5.0,
		RateLimitEndpoints:        make(map[string]EndpointRateLimitConfig),
		TrustProxy:                false,
		ProxyHeaders:              []string{"X-Forwarded-For", "X-Real-IP"},
		TLSEnabled:                false,
		TLSMinVersion:             "TLS1.2",
		TLSCipherSuites:           []string{},
		WebSocketEnabled:          true,
		WebSocketMaxConns:         1000,
		WebSocketReadTimeout:      30 * time.Second,
		WebSocketWriteTimeout:     30 * time.Second,
		WebSocketMaxMsgSize:       1 << 20,
		APIKeyAuthEnabled:         false,
		APIKeyHeader:              "X-API-Key",
		APIKeyQuery:               "api_key",
		JWTEnabled:                false,
		JWTExpiration:             24 * time.Hour,
		JWTIssuer:                 "nogo-api",
		LoggingEnabled:            true,
		LoggingLevel:              "info",
		LoggingFormat:             "json",
		MetricsEnabled:            true,
		MetricsPath:               "/metrics",
		MetricsPort:               0,
		HealthCheckPath:           "/health",
		HealthCheckEnabled:        true,
		PprofEnabled:              false,
		PprofPath:                 "/debug/pprof",
		AdminEnabled:              false,
		AdminPath:                 "/admin",
		MaxRequestSize:            10 << 20,
		MaxResponseSize:           100 << 20,
		CompressionLevel:          6,
		CacheEnabled:              true,
		CacheTTL:                  5 * time.Minute,
		CacheMaxSize:              10000,
		AuditLogEnabled:           true,
		AuditLogDir:               "logs/audit",
		AuditLogMaxFileSize:       100 << 20,
		AuditLogMaxRetention:      90,
		AuditLogCompress:          true,
		AuditLogAsync:             true,
		AuditLogBufferSize:        1000,
		AuditLogFlushInterval:     5 * time.Second,
	}
}

func LoadHTTPAPIConfig() (*HTTPAPIConfig, error) {
	cfg := DefaultHTTPAPIConfig()

	if err := cfg.loadFromEnv(); err != nil {
		return nil, fmt.Errorf("load HTTP API config from env: %w", err)
	}

	return cfg, nil
}

func LoadHTTPAPIConfigFromFile(path string) (*HTTPAPIConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read HTTP API config file: %w", err)
	}

	cfg := DefaultHTTPAPIConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse HTTP API config file: %w", err)
	}

	if err := cfg.loadFromEnv(); err != nil {
		return nil, fmt.Errorf("override HTTP API config from env: %w", err)
	}

	return cfg, nil
}

func (c *HTTPAPIConfig) loadFromEnv() error {
	if v := os.Getenv("API_HTTP_PORT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			c.HTTPPort = i
		}
	}

	if v := os.Getenv("API_HTTP_HOST"); v != "" {
		c.HTTPHost = v
	}

	if v := os.Getenv("API_READ_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.ReadTimeout = d
		}
	}

	if v := os.Getenv("API_WRITE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.WriteTimeout = d
		}
	}

	if v := os.Getenv("API_IDLE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.IdleTimeout = d
		}
	}

	if v := os.Getenv("API_MAX_HEADER_BYTES"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			c.MaxHeaderBytes = i
		}
	}

	if v := os.Getenv("API_ENABLE_GZIP"); v != "" {
		c.EnableGzip = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_GZIP_LEVEL"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i >= 1 && i <= 9 {
			c.GzipLevel = i
		}
	}

	if v := os.Getenv("API_ENABLE_CORS"); v != "" {
		c.EnableCORS = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_CORS_ALLOW_ORIGINS"); v != "" {
		c.CORSAllowOrigins = strings.Split(v, ",")
	}

	if v := os.Getenv("API_CORS_ALLOW_METHODS"); v != "" {
		c.CORSAllowMethods = strings.Split(v, ",")
	}

	if v := os.Getenv("API_CORS_ALLOW_HEADERS"); v != "" {
		c.CORSAllowHeaders = strings.Split(v, ",")
	}

	if v := os.Getenv("API_CORS_ALLOW_CREDENTIALS"); v != "" {
		c.CORSAllowCredentials = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_CORS_MAX_AGE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.CORSMaxAge = d
		}
	}

	if v := os.Getenv("API_RATE_LIMIT_ENABLED"); v != "" {
		c.RateLimitEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_RATE_LIMIT_RPS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			c.RateLimitRPS = i
		}
	}

	if v := os.Getenv("API_RATE_LIMIT_BURST"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			c.RateLimitBurst = i
		}
	}

	if v := os.Getenv("API_RATE_LIMIT_BY_IP"); v != "" {
		c.RateLimitByIP = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_RATE_LIMIT_BY_API_KEY"); v != "" {
		c.RateLimitByAPIKey = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_RATE_LIMIT_API_KEY_MULTIPLIER"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			c.RateLimitAPIKeyMultiplier = f
		}
	}

	if v := os.Getenv("API_TRUST_PROXY"); v != "" {
		c.TrustProxy = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_PROXY_HEADERS"); v != "" {
		c.ProxyHeaders = strings.Split(v, ",")
	}

	if v := os.Getenv("API_TLS_ENABLED"); v != "" {
		c.TLSEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_TLS_CERT_FILE"); v != "" {
		c.TLSCertFile = v
	}

	if v := os.Getenv("API_TLS_KEY_FILE"); v != "" {
		c.TLSKeyFile = v
	}

	if v := os.Getenv("API_TLS_MIN_VERSION"); v != "" {
		c.TLSMinVersion = v
	}

	if v := os.Getenv("API_TLS_CIPHER_SUITES"); v != "" {
		c.TLSCipherSuites = strings.Split(v, ",")
	}

	if v := os.Getenv("API_WEBSOCKET_ENABLED"); v != "" {
		c.WebSocketEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_WEBSOCKET_MAX_CONNS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			c.WebSocketMaxConns = i
		}
	}

	if v := os.Getenv("API_WEBSOCKET_READ_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.WebSocketReadTimeout = d
		}
	}

	if v := os.Getenv("API_WEBSOCKET_WRITE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.WebSocketWriteTimeout = d
		}
	}

	if v := os.Getenv("API_WEBSOCKET_MAX_MSG_SIZE"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			c.WebSocketMaxMsgSize = i
		}
	}

	if v := os.Getenv("API_KEY_AUTH_ENABLED"); v != "" {
		c.APIKeyAuthEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_KEY_HEADER"); v != "" {
		c.APIKeyHeader = v
	}

	if v := os.Getenv("API_KEY_QUERY"); v != "" {
		c.APIKeyQuery = v
	}

	if v := os.Getenv("API_JWT_ENABLED"); v != "" {
		c.JWTEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_JWT_SECRET"); v != "" {
		c.JWTSecret = v
	}

	if v := os.Getenv("API_JWT_EXPIRATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.JWTExpiration = d
		}
	}

	if v := os.Getenv("API_JWT_ISSUER"); v != "" {
		c.JWTIssuer = v
	}

	if v := os.Getenv("API_LOGGING_ENABLED"); v != "" {
		c.LoggingEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_LOGGING_LEVEL"); v != "" {
		c.LoggingLevel = v
	}

	if v := os.Getenv("API_LOGGING_FORMAT"); v != "" {
		c.LoggingFormat = v
	}

	if v := os.Getenv("API_METRICS_ENABLED"); v != "" {
		c.MetricsEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_METRICS_PATH"); v != "" {
		c.MetricsPath = v
	}

	if v := os.Getenv("API_METRICS_PORT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			c.MetricsPort = i
		}
	}

	if v := os.Getenv("API_HEALTH_CHECK_PATH"); v != "" {
		c.HealthCheckPath = v
	}

	if v := os.Getenv("API_HEALTH_CHECK_ENABLED"); v != "" {
		c.HealthCheckEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_PPROF_ENABLED"); v != "" {
		c.PprofEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_PPROF_PATH"); v != "" {
		c.PprofPath = v
	}

	if v := os.Getenv("API_ADMIN_ENABLED"); v != "" {
		c.AdminEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_ADMIN_PATH"); v != "" {
		c.AdminPath = v
	}

	if v := os.Getenv("API_ADMIN_TOKEN"); v != "" {
		c.AdminToken = v
	}

	if v := os.Getenv("API_MAX_REQUEST_SIZE"); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil && i > 0 {
			c.MaxRequestSize = i
		}
	}

	if v := os.Getenv("API_MAX_RESPONSE_SIZE"); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil && i > 0 {
			c.MaxResponseSize = i
		}
	}

	if v := os.Getenv("API_COMPRESSION_LEVEL"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i >= 1 && i <= 9 {
			c.CompressionLevel = i
		}
	}

	if v := os.Getenv("API_CACHE_ENABLED"); v != "" {
		c.CacheEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.CacheTTL = d
		}
	}

	if v := os.Getenv("API_CACHE_MAX_SIZE"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			c.CacheMaxSize = i
		}
	}

	if v := os.Getenv("API_AUDIT_LOG_ENABLED"); v != "" {
		c.AuditLogEnabled = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_AUDIT_LOG_DIR"); v != "" {
		c.AuditLogDir = v
	}

	if v := os.Getenv("API_AUDIT_LOG_MAX_FILE_SIZE"); v != "" {
		if size, err := strconv.ParseInt(v, 10, 64); err == nil && size > 0 {
			c.AuditLogMaxFileSize = size
		}
	}

	if v := os.Getenv("API_AUDIT_LOG_MAX_RETENTION"); v != "" {
		if days, err := strconv.Atoi(v); err == nil && days > 0 {
			c.AuditLogMaxRetention = days
		}
	}

	if v := os.Getenv("API_AUDIT_LOG_COMPRESS"); v != "" {
		c.AuditLogCompress = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_AUDIT_LOG_ASYNC"); v != "" {
		c.AuditLogAsync = v == "true" || v == "1" || strings.EqualFold(v, "yes")
	}

	if v := os.Getenv("API_AUDIT_LOG_BUFFER_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size > 0 {
			c.AuditLogBufferSize = size
		}
	}

	if v := os.Getenv("API_AUDIT_LOG_FLUSH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			c.AuditLogFlushInterval = d
		}
	}

	return nil
}

func (c *HTTPAPIConfig) Validate() error {
	if c.HTTPPort < 0 || c.HTTPPort > 65535 {
		return fmt.Errorf("invalid HTTP_PORT: %d", c.HTTPPort)
	}

	if c.HTTPHost == "" {
		return fmt.Errorf("HTTP_HOST must be set")
	}

	if c.ReadTimeout <= 0 {
		return fmt.Errorf("READ_TIMEOUT must be positive")
	}

	if c.WriteTimeout <= 0 {
		return fmt.Errorf("WRITE_TIMEOUT must be positive")
	}

	if c.IdleTimeout <= 0 {
		return fmt.Errorf("IDLE_TIMEOUT must be positive")
	}

	if c.MaxHeaderBytes <= 0 {
		return fmt.Errorf("MAX_HEADER_BYTES must be positive")
	}

	if c.EnableGzip && (c.GzipLevel < 1 || c.GzipLevel > 9) {
		return fmt.Errorf("GZIP_LEVEL must be between 1 and 9")
	}

	if c.EnableCORS {
		if len(c.CORSAllowOrigins) == 0 {
			return fmt.Errorf("CORS_ALLOW_ORIGINS must be set when CORS is enabled")
		}
		if len(c.CORSAllowMethods) == 0 {
			return fmt.Errorf("CORS_ALLOW_METHODS must be set when CORS is enabled")
		}
		if c.CORSMaxAge <= 0 {
			return fmt.Errorf("CORS_MAX_AGE must be positive")
		}
	}

	if c.RateLimitEnabled {
		if c.RateLimitRPS <= 0 {
			return fmt.Errorf("RATE_LIMIT_RPS must be positive when rate limiting is enabled")
		}
		if c.RateLimitBurst <= 0 {
			return fmt.Errorf("RATE_LIMIT_BURST must be positive when rate limiting is enabled")
		}
		if c.RateLimitAPIKeyMultiplier <= 0 || c.RateLimitAPIKeyMultiplier > 100.0 {
			return fmt.Errorf("RATE_LIMIT_API_KEY_MULTIPLIER must be between 0 and 100")
		}
		if !c.RateLimitByIP && !c.RateLimitByAPIKey {
			return fmt.Errorf("at least one of RATE_LIMIT_BY_IP or RATE_LIMIT_BY_API_KEY must be enabled")
		}
		// Validate per-endpoint rate limits
		for endpoint, cfg := range c.RateLimitEndpoints {
			if cfg.RequestsPerSecond <= 0 {
				return fmt.Errorf("endpoint %s requests_per_second must be positive", endpoint)
			}
			if cfg.Burst <= 0 {
				return fmt.Errorf("endpoint %s burst must be positive", endpoint)
			}
		}
	}

	if c.TLSEnabled {
		if c.TLSCertFile == "" {
			return fmt.Errorf("TLS_CERT_FILE must be set when TLS is enabled")
		}
		if c.TLSKeyFile == "" {
			return fmt.Errorf("TLS_KEY_FILE must be set when TLS is enabled")
		}
		if err := c.validateTLSVersion(); err != nil {
			return err
		}
		if err := c.validateTLSCipherSuites(); err != nil {
			return err
		}
		if _, err := os.Stat(c.TLSCertFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS_CERT_FILE does not exist: %s", c.TLSCertFile)
		}
		if _, err := os.Stat(c.TLSKeyFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS_KEY_FILE does not exist: %s", c.TLSKeyFile)
		}
		if _, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile); err != nil {
			return fmt.Errorf("invalid TLS certificate/key pair: %w", err)
		}
	}

	if c.WebSocketEnabled {
		if c.WebSocketMaxConns <= 0 {
			return fmt.Errorf("WEBSOCKET_MAX_CONNS must be positive")
		}
		if c.WebSocketReadTimeout <= 0 {
			return fmt.Errorf("WEBSOCKET_READ_TIMEOUT must be positive")
		}
		if c.WebSocketWriteTimeout <= 0 {
			return fmt.Errorf("WEBSOCKET_WRITE_TIMEOUT must be positive")
		}
		if c.WebSocketMaxMsgSize <= 0 {
			return fmt.Errorf("WEBSOCKET_MAX_MSG_SIZE must be positive")
		}
	}

	if c.JWTEnabled {
		if c.JWTSecret == "" {
			return fmt.Errorf("JWT_SECRET must be set when JWT is enabled")
		}
		if len(c.JWTSecret) < 32 {
			return fmt.Errorf("JWT_SECRET must be at least 32 characters")
		}
		if c.JWTExpiration <= 0 {
			return fmt.Errorf("JWT_EXPIRATION must be positive")
		}
		if c.JWTIssuer == "" {
			return fmt.Errorf("JWT_ISSUER must be set when JWT is enabled")
		}
	}

	if c.LoggingEnabled {
		if c.LoggingLevel == "" {
			return fmt.Errorf("LOGGING_LEVEL must be set when logging is enabled")
		}
		if c.LoggingFormat != "json" && c.LoggingFormat != "text" {
			return fmt.Errorf("LOGGING_FORMAT must be either 'json' or 'text'")
		}
	}

	if c.MetricsEnabled {
		if c.MetricsPath == "" {
			return fmt.Errorf("METRICS_PATH must be set when metrics is enabled")
		}
	}

	if c.HealthCheckEnabled {
		if c.HealthCheckPath == "" {
			return fmt.Errorf("HEALTH_CHECK_PATH must be set when health check is enabled")
		}
	}

	if c.AdminEnabled {
		if c.AdminPath == "" {
			return fmt.Errorf("ADMIN_PATH must be set when admin is enabled")
		}
		if c.AdminToken == "" {
			return fmt.Errorf("ADMIN_TOKEN must be set when admin is enabled")
		}
		if len(c.AdminToken) < 16 {
			return fmt.Errorf("ADMIN_TOKEN must be at least 16 characters")
		}
	}

	if c.MaxRequestSize <= 0 {
		return fmt.Errorf("MAX_REQUEST_SIZE must be positive")
	}

	if c.MaxResponseSize <= 0 {
		return fmt.Errorf("MAX_RESPONSE_SIZE must be positive")
	}

	if c.CompressionLevel < 1 || c.CompressionLevel > 9 {
		return fmt.Errorf("COMPRESSION_LEVEL must be between 1 and 9")
	}

	if c.CacheEnabled {
		if c.CacheTTL <= 0 {
			return fmt.Errorf("CACHE_TTL must be positive")
		}
		if c.CacheMaxSize <= 0 {
			return fmt.Errorf("CACHE_MAX_SIZE must be positive")
		}
	}

	if c.AuditLogEnabled {
		if c.AuditLogDir == "" {
			return fmt.Errorf("AUDIT_LOG_DIR must be set when audit logging is enabled")
		}
		if c.AuditLogMaxFileSize <= 0 {
			return fmt.Errorf("AUDIT_LOG_MAX_FILE_SIZE must be positive when audit logging is enabled")
		}
		if c.AuditLogMaxRetention <= 0 || c.AuditLogMaxRetention > 3650 {
			return fmt.Errorf("AUDIT_LOG_MAX_RETENTION must be between 1 and 3650 days when audit logging is enabled")
		}
		if c.AuditLogBufferSize <= 0 || c.AuditLogBufferSize > 100000 {
			return fmt.Errorf("AUDIT_LOG_BUFFER_SIZE must be between 1 and 100000 when audit logging is enabled")
		}
		if c.AuditLogFlushInterval <= 0 || c.AuditLogFlushInterval > time.Hour {
			return fmt.Errorf("AUDIT_LOG_FLUSH_INTERVAL must be between 1ms and 1h when audit logging is enabled")
		}
	}

	return nil
}

func (c *HTTPAPIConfig) validateTLSVersion() error {
	validVersions := map[string]bool{
		"TLS1.0": false,
		"TLS1.1": false,
		"TLS1.2": true,
		"TLS1.3": true,
	}

	if !validVersions[c.TLSMinVersion] {
		return fmt.Errorf("invalid TLS_MIN_VERSION: %s (TLS1.0 and TLS1.1 are deprecated)", c.TLSMinVersion)
	}

	return nil
}

func (c *HTTPAPIConfig) validateTLSCipherSuites() error {
	if len(c.TLSCipherSuites) == 0 {
		return nil
	}

	validCiphers := map[string]bool{
		"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256":       true,
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":         true,
		"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384":       true,
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":         true,
		"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256": true,
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256":   true,
		"TLS_AES_128_GCM_SHA256":                        true,
		"TLS_AES_256_GCM_SHA384":                        true,
		"TLS_CHACHA20_POLY1305_SHA256":                  true,
	}

	for _, cipher := range c.TLSCipherSuites {
		if !validCiphers[cipher] {
			return fmt.Errorf("invalid TLS cipher suite: %s", cipher)
		}
	}

	return nil
}

func (c *HTTPAPIConfig) Merge(other *HTTPAPIConfig) *HTTPAPIConfig {
	if other == nil {
		return c
	}

	out := *c

	if other.HTTPPort != 0 {
		out.HTTPPort = other.HTTPPort
	}
	if other.HTTPHost != "" {
		out.HTTPHost = other.HTTPHost
	}
	if other.ReadTimeout > 0 {
		out.ReadTimeout = other.ReadTimeout
	}
	if other.WriteTimeout > 0 {
		out.WriteTimeout = other.WriteTimeout
	}
	if other.IdleTimeout > 0 {
		out.IdleTimeout = other.IdleTimeout
	}
	if other.MaxHeaderBytes > 0 {
		out.MaxHeaderBytes = other.MaxHeaderBytes
	}
	if other.GzipLevel >= 1 && other.GzipLevel <= 9 {
		out.GzipLevel = other.GzipLevel
	}
	if other.RateLimitRPS > 0 {
		out.RateLimitRPS = other.RateLimitRPS
	}
	if other.RateLimitBurst > 0 {
		out.RateLimitBurst = other.RateLimitBurst
	}
	if other.RateLimitAPIKeyMultiplier > 0 {
		out.RateLimitAPIKeyMultiplier = other.RateLimitAPIKeyMultiplier
	}
	if len(other.RateLimitEndpoints) > 0 {
		out.RateLimitEndpoints = other.RateLimitEndpoints
	}
	if other.WebSocketMaxConns > 0 {
		out.WebSocketMaxConns = other.WebSocketMaxConns
	}
	if other.WebSocketMaxMsgSize > 0 {
		out.WebSocketMaxMsgSize = other.WebSocketMaxMsgSize
	}
	if other.MaxRequestSize > 0 {
		out.MaxRequestSize = other.MaxRequestSize
	}
	if other.MaxResponseSize > 0 {
		out.MaxResponseSize = other.MaxResponseSize
	}
	if other.CacheMaxSize > 0 {
		out.CacheMaxSize = other.CacheMaxSize
	}

	out.EnableGzip = out.EnableGzip || other.EnableGzip
	out.EnableCORS = out.EnableCORS || other.EnableCORS
	out.RateLimitEnabled = out.RateLimitEnabled || other.RateLimitEnabled
	out.RateLimitByIP = out.RateLimitByIP || other.RateLimitByIP
	out.RateLimitByAPIKey = out.RateLimitByAPIKey || other.RateLimitByAPIKey
	out.TrustProxy = out.TrustProxy || other.TrustProxy
	out.TLSEnabled = out.TLSEnabled || other.TLSEnabled
	out.WebSocketEnabled = out.WebSocketEnabled || other.WebSocketEnabled
	out.APIKeyAuthEnabled = out.APIKeyAuthEnabled || other.APIKeyAuthEnabled
	out.JWTEnabled = out.JWTEnabled || other.JWTEnabled
	out.LoggingEnabled = out.LoggingEnabled || other.LoggingEnabled
	out.MetricsEnabled = out.MetricsEnabled || other.MetricsEnabled
	out.HealthCheckEnabled = out.HealthCheckEnabled || other.HealthCheckEnabled
	out.PprofEnabled = out.PprofEnabled || other.PprofEnabled
	out.AdminEnabled = out.AdminEnabled || other.AdminEnabled
	out.CacheEnabled = out.CacheEnabled || other.CacheEnabled

	if len(other.CORSAllowOrigins) > 0 {
		out.CORSAllowOrigins = other.CORSAllowOrigins
	}
	if len(other.CORSAllowMethods) > 0 {
		out.CORSAllowMethods = other.CORSAllowMethods
	}
	if len(other.CORSAllowHeaders) > 0 {
		out.CORSAllowHeaders = other.CORSAllowHeaders
	}
	if len(other.ProxyHeaders) > 0 {
		out.ProxyHeaders = other.ProxyHeaders
	}
	if len(other.TLSCipherSuites) > 0 {
		out.TLSCipherSuites = other.TLSCipherSuites
	}
	if other.TLSMinVersion != "" {
		out.TLSMinVersion = other.TLSMinVersion
	}
	if other.TLSCertFile != "" {
		out.TLSCertFile = other.TLSCertFile
	}
	if other.TLSKeyFile != "" {
		out.TLSKeyFile = other.TLSKeyFile
	}
	if other.APIKeyHeader != "" {
		out.APIKeyHeader = other.APIKeyHeader
	}
	if other.APIKeyQuery != "" {
		out.APIKeyQuery = other.APIKeyQuery
	}
	if other.JWTSecret != "" {
		out.JWTSecret = other.JWTSecret
	}
	if other.JWTExpiration > 0 {
		out.JWTExpiration = other.JWTExpiration
	}
	if other.JWTIssuer != "" {
		out.JWTIssuer = other.JWTIssuer
	}
	if other.LoggingLevel != "" {
		out.LoggingLevel = other.LoggingLevel
	}
	if other.LoggingFormat != "" {
		out.LoggingFormat = other.LoggingFormat
	}
	if other.MetricsPath != "" {
		out.MetricsPath = other.MetricsPath
	}
	if other.MetricsPort != 0 {
		out.MetricsPort = other.MetricsPort
	}
	if other.HealthCheckPath != "" {
		out.HealthCheckPath = other.HealthCheckPath
	}
	if other.PprofPath != "" {
		out.PprofPath = other.PprofPath
	}
	if other.AdminPath != "" {
		out.AdminPath = other.AdminPath
	}
	if other.AdminToken != "" {
		out.AdminToken = other.AdminToken
	}
	if other.CacheTTL > 0 {
		out.CacheTTL = other.CacheTTL
	}
	if other.CompressionLevel >= 1 && other.CompressionLevel <= 9 {
		out.CompressionLevel = other.CompressionLevel
	}
	if other.AuditLogDir != "" {
		out.AuditLogDir = other.AuditLogDir
	}
	if other.AuditLogMaxFileSize > 0 {
		out.AuditLogMaxFileSize = other.AuditLogMaxFileSize
	}
	if other.AuditLogMaxRetention > 0 {
		out.AuditLogMaxRetention = other.AuditLogMaxRetention
	}
	if other.AuditLogBufferSize > 0 {
		out.AuditLogBufferSize = other.AuditLogBufferSize
	}
	if other.AuditLogFlushInterval > 0 {
		out.AuditLogFlushInterval = other.AuditLogFlushInterval
	}

	out.AuditLogEnabled = out.AuditLogEnabled || other.AuditLogEnabled
	out.AuditLogCompress = out.AuditLogCompress || other.AuditLogCompress
	out.AuditLogAsync = out.AuditLogAsync || other.AuditLogAsync

	return &out
}

func (c *HTTPAPIConfig) ToJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

func (c *HTTPAPIConfig) SaveToFile(path string) error {
	data, err := c.ToJSON()
	if err != nil {
		return fmt.Errorf("marshal HTTP API config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write HTTP API config file: %w", err)
	}

	return nil
}

func (c *HTTPAPIConfig) SyncFromLegacy(cfg *APIConfig) {
	if cfg == nil {
		return
	}

	if cfg.HTTPPort != 0 && c.HTTPPort == 0 {
		c.HTTPPort = cfg.HTTPPort
	}

	if cfg.Enabled && !c.LoggingEnabled {
		c.LoggingEnabled = true
	}

	if len(cfg.CORS) > 0 && len(c.CORSAllowOrigins) == 0 {
		c.CORSAllowOrigins = cfg.CORS
		c.EnableCORS = true
	}
}
