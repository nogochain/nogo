package config

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	NodePort           int
	P2PPort            int
	DataDir            string
	MiningEnabled      bool
	MiningThreads      int
	MaxPeers           int
	LogLevel           string
	ChainID            uint64
	GenesisPath        string
	MinerAddress       string
	AdminToken         string
	P2PEnable          bool
	WSEnable           bool
	AIAuditorURL       string
	RPCPort            int
	EnableCORS         bool
	RateLimitRPS       int
	RateLimitBurst     int
	KeystoreDir        string
	TrustProxy         bool
	PruneDepth         int64
	StoreMode          string
	CheckpointInterval int64
	CacheMaxBlocks     int
	CacheMaxBalances   int
	CacheMaxProofs     int
	MempoolMaxSize     int
	MempoolMinFeeRate  int64
	MempoolTTL         time.Duration
	MaxPoolConns       int
	MaxConnsPerPeer    int
	SyncWorkers        int
	SyncBatchSize      int
	StratumEnabled     bool
	StratumAddr        string
	EnableProtobuf     bool
	MetricsEnabled     bool
	MetricsPort        int
	P2PSeeds           string
	// TLS configuration for production deployment
	TLSCertFile string
	TLSKeyFile  string
}

func LoadConfig() *Config {
	return &Config{
		NodePort:           getEnvInt("NODE_PORT", 8080),
		P2PPort:            getEnvInt("P2P_PORT", 9090),
		DataDir:            getEnvStr("DATA_DIR", "./data"),
		MiningEnabled:      getEnvBool("MINING_ENABLED", false),
		MiningThreads:      getEnvInt("MINING_THREADS", 1),
		MaxPeers:           getEnvInt("MAX_PEERS", 50),
		LogLevel:           getEnvStr("LOG_LEVEL", "info"),
		ChainID:            getEnvUint64("CHAIN_ID", 1),
		GenesisPath:        getEnvStr("GENESIS_PATH", "./genesis.json"),
		MinerAddress:       getEnvStr("MINER_ADDRESS", ""),
		AdminToken:         getEnvStr("ADMIN_TOKEN", ""),
		P2PEnable:          getEnvBool("P2P_ENABLE", true),
		WSEnable:           getEnvBool("WS_ENABLE", true),
		AIAuditorURL:       getEnvStr("AI_AUDITOR_URL", ""),
		RPCPort:            getEnvInt("RPC_PORT", 8080),
		EnableCORS:         getEnvBool("ENABLE_CORS", false),
		RateLimitRPS:       getEnvInt("RATE_LIMIT_REQUESTS", 0),
		RateLimitBurst:     getEnvInt("RATE_LIMIT_BURST", 0),
		KeystoreDir:        getEnvStr("KEYSTORE_DIR", "./keystore"),
		TrustProxy:         getEnvBool("TRUST_PROXY", false),
		PruneDepth:         getEnvInt64("PRUNE_DEPTH", 1000),
		StoreMode:          getEnvStr("STORE_MODE", "pruned"),
		CheckpointInterval: getEnvInt64("CHECKPOINT_INTERVAL", 100),
		CacheMaxBlocks:     getEnvInt("CACHE_MAX_BLOCKS", 10000),
		CacheMaxBalances:   getEnvInt("CACHE_MAX_BALANCES", 100000),
		CacheMaxProofs:     getEnvInt("CACHE_MAX_PROOFS", 10000),
		MempoolMaxSize:     getEnvInt("MEMPOOL_MAX_SIZE", 50000),
		MempoolMinFeeRate:  getEnvInt64("MEMPOOL_MIN_FEE_RATE", 1),
		MempoolTTL:         getEnvDuration("MEMPOOL_TTL", 24*time.Hour),
		MaxPoolConns:       getEnvInt("MAX_POOL_CONNS", 100),
		MaxConnsPerPeer:    getEnvInt("MAX_CONNS_PER_PEER", 3),
		SyncWorkers:        getEnvInt("SYNC_WORKERS", 8),
		SyncBatchSize:      getEnvInt("SYNC_BATCH_SIZE", 100),
		StratumEnabled:     getEnvBool("STRATUM_ENABLED", false),
		StratumAddr:        getEnvStr("STRATUM_ADDR", ":3333"),
		EnableProtobuf:     getEnvBool("ENABLE_PROTOBUF", true),
		MetricsEnabled:     getEnvBool("METRICS_ENABLED", true),
		MetricsPort:        getEnvInt("METRICS_PORT", 0),
		P2PSeeds:           getEnvStr("P2P_SEEDS", ""),
		TLSCertFile:        getEnvStr("TLS_CERT_FILE", ""),
		TLSKeyFile:         getEnvStr("TLS_KEY_FILE", ""),
	}
}

func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	return &cfg, nil
}

func LoadFromYAML(path string) (*Config, error) {
	return LoadFromFile(path)
}

func (c *Config) Merge(other *Config) *Config {
	if other == nil {
		return c
	}
	out := *c
	if other.NodePort != 0 {
		out.NodePort = other.NodePort
	}
	if other.P2PPort != 0 {
		out.P2PPort = other.P2PPort
	}
	if other.DataDir != "" {
		out.DataDir = other.DataDir
	}
	if other.GenesisPath != "" {
		out.GenesisPath = other.GenesisPath
	}
	if other.MinerAddress != "" {
		out.MinerAddress = other.MinerAddress
	}
	if other.AdminToken != "" {
		out.AdminToken = other.AdminToken
	}
	if other.AIAuditorURL != "" {
		out.AIAuditorURL = other.AIAuditorURL
	}
	if other.KeystoreDir != "" {
		out.KeystoreDir = other.KeystoreDir
	}
	if other.LogLevel != "" {
		out.LogLevel = other.LogLevel
	}
	out.MiningEnabled = out.MiningEnabled || other.MiningEnabled
	out.P2PEnable = out.P2PEnable || other.P2PEnable
	out.WSEnable = out.WSEnable || other.WSEnable
	out.EnableCORS = out.EnableCORS || other.EnableCORS
	out.TrustProxy = out.TrustProxy || other.TrustProxy
	if other.MiningThreads > 0 {
		out.MiningThreads = other.MiningThreads
	}
	if other.MaxPeers > 0 {
		out.MaxPeers = other.MaxPeers
	}
	if other.ChainID > 0 {
		out.ChainID = other.ChainID
	}
	if other.RPCPort > 0 {
		out.RPCPort = other.RPCPort
	}
	if other.RateLimitRPS > 0 {
		out.RateLimitRPS = other.RateLimitRPS
	}
	if other.RateLimitBurst > 0 {
		out.RateLimitBurst = other.RateLimitBurst
	}
	if other.PruneDepth > 0 {
		out.PruneDepth = other.PruneDepth
	}
	if other.StoreMode != "" {
		out.StoreMode = other.StoreMode
	}
	if other.CheckpointInterval > 0 {
		out.CheckpointInterval = other.CheckpointInterval
	}
	if other.MempoolMaxSize > 0 {
		out.MempoolMaxSize = other.MempoolMaxSize
	}
	if other.MempoolMinFeeRate > 0 {
		out.MempoolMinFeeRate = other.MempoolMinFeeRate
	}
	if other.MempoolTTL > 0 {
		out.MempoolTTL = other.MempoolTTL
	}
	return &out
}

const (
	AdminTokenMinLength = 16
	AddressMinLength    = 10
)

func (c *Config) Validate() error {
	if c.ChainID == 0 {
		return fmt.Errorf("CHAIN_ID must be set")
	}
	if c.NodePort < 0 || c.NodePort > 65535 {
		return fmt.Errorf("invalid NODE_PORT: %d", c.NodePort)
	}
	if c.P2PPort < 0 || c.P2PPort > 65535 {
		return fmt.Errorf("invalid P2P_PORT: %d", c.P2PPort)
	}
	if c.MaxPeers < 0 {
		return fmt.Errorf("invalid MAX_PEERS: %d", c.MaxPeers)
	}
	if c.RateLimitRPS < 0 {
		return fmt.Errorf("invalid RATE_LIMIT_REQUESTS: %d", c.RateLimitRPS)
	}
	if c.RateLimitBurst < 0 {
		return fmt.Errorf("invalid RATE_LIMIT_BURST: %d", c.RateLimitBurst)
	}
	if err := c.validateMinerAddress(); err != nil {
		return err
	}
	if err := c.validateAdminToken(); err != nil {
		return err
	}
	if err := c.validateDataDir(); err != nil {
		return err
	}
	return nil
}

func (c *Config) validateMinerAddress() error {
	if c.MinerAddress == "" {
		return nil
	}
	if !strings.HasPrefix(c.MinerAddress, "NOGO") {
		if _, err := hex.DecodeString(c.MinerAddress); err != nil {
			return fmt.Errorf("invalid MINER_ADDRESS: must be NOGO prefixed or valid hex")
		}
		if len(c.MinerAddress) < AddressMinLength {
			return fmt.Errorf("invalid MINER_ADDRESS: too short")
		}
	}
	return nil
}

func (c *Config) validateAdminToken() error {
	if c.AdminToken == "" {
		return nil
	}
	if len(c.AdminToken) < AdminTokenMinLength {
		return fmt.Errorf("invalid ADMIN_TOKEN: must be at least %d characters", AdminTokenMinLength)
	}
	return nil
}

func (c *Config) validateDataDir() error {
	if c.DataDir == "" {
		return fmt.Errorf("DATA_DIR must be set")
	}
	return nil
}

func getEnvStr(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvUint64(key string, defaultVal uint64) uint64 {
	if val := os.Getenv(key); val != "" {
		if uintVal, err := strconv.ParseUint(val, 10, 64); err == nil {
			return uintVal
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.ParseInt(val, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1" || strings.EqualFold(val, "yes")
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
