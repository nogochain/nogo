package config

import (
	"flag"
	"os"
	"strings"
)

type Flags struct {
	ConfigPath     string
	NodePort       int
	P2PPort        int
	DataDir        string
	MiningEnabled  bool
	MiningThreads  int
	MaxPeers       int
	LogLevel       string
	ChainID        uint64
	GenesisPath    string
	MinerAddress   string
	AdminToken     string
	P2PEnable      bool
	WSEnable       bool
	WSMaxConns     int
	AIAuditorURL   string
	RPCPort        int
	EnableCORS     bool
	RateLimitRPS   int
	RateLimitBurst int
	KeystoreDir    string
	TrustProxy     bool
}

func ParseFlags() *Flags {
	f := &Flags{}

	flag.StringVar(&f.ConfigPath, "config", "", "Path to YAML config file")
	flag.IntVar(&f.NodePort, "port", 0, "HTTP server port (default: from env or 8080)")
	flag.IntVar(&f.P2PPort, "p2p-port", 0, "P2P server port (default: from env or 9090)")
	flag.StringVar(&f.DataDir, "data-dir", "", "Data directory for blockchain storage")
	flag.BoolVar(&f.MiningEnabled, "mining", false, "Enable mining")
	flag.IntVar(&f.MiningThreads, "mining-threads", 0, "Number of mining threads")
	flag.IntVar(&f.MaxPeers, "max-peers", 0, "Maximum number of P2P peers")
	flag.StringVar(&f.LogLevel, "log-level", "", "Log level (debug, info, warn, error)")
	flag.Uint64Var(&f.ChainID, "chain-id", 0, "Chain ID")
	flag.StringVar(&f.GenesisPath, "genesis", "", "Path to genesis JSON file")
	flag.StringVar(&f.MinerAddress, "miner-address", "", "Miner address")
	flag.StringVar(&f.AdminToken, "admin-token", "", "Admin authentication token")
	flag.BoolVar(&f.P2PEnable, "p2p", false, "Enable P2P networking")
	flag.BoolVar(&f.WSEnable, "ws", false, "Enable WebSocket server")
	flag.IntVar(&f.WSMaxConns, "ws-max-conns", 100, "Maximum WebSocket connections")
	flag.StringVar(&f.AIAuditorURL, "ai-auditor-url", "", "AI auditor URL for transaction policy checking")
	flag.IntVar(&f.RPCPort, "rpc-port", 0, "RPC server port")
	flag.BoolVar(&f.EnableCORS, "cors", false, "Enable CORS")
	flag.IntVar(&f.RateLimitRPS, "rate-limit-rps", 0, "Rate limit (requests per second)")
	flag.IntVar(&f.RateLimitBurst, "rate-limit-burst", 0, "Rate limit burst size")
	flag.StringVar(&f.KeystoreDir, "keystore-dir", "", "Directory for wallet keystore files")
	flag.BoolVar(&f.TrustProxy, "trust-proxy", false, "Trust X-Forwarded-For headers")

	flag.Parse()

	return f
}

func (f *Flags) OverrideEnv() {
	if f.AdminToken == "" {
		f.AdminToken = os.Getenv("ADMIN_TOKEN")
	}
	if f.MinerAddress == "" {
		f.MinerAddress = os.Getenv("MINER_ADDRESS")
	}
	if f.AIAuditorURL == "" {
		f.AIAuditorURL = os.Getenv("AI_AUDITOR_URL")
	}
	if f.ChainID == 0 {
		if raw := strings.TrimSpace(os.Getenv("CHAIN_ID")); raw != "" {
			var id uint64
			if _, err := parseUint64(raw); err == nil {
				f.ChainID = id
			}
		}
	}
}

func (f *Flags) ApplyTo(cfg *Config) *Config {
	if f.ConfigPath != "" {
		yamlCfg, err := LoadFromFile(f.ConfigPath)
		if err == nil {
			cfg = cfg.Merge(yamlCfg)
		}
	}
	if f.NodePort > 0 {
		cfg.NodePort = f.NodePort
	}
	if f.P2PPort > 0 {
		cfg.P2PPort = f.P2PPort
	}
	if f.DataDir != "" {
		cfg.DataDir = f.DataDir
	}
	cfg.MiningEnabled = cfg.MiningEnabled || f.MiningEnabled
	if f.MiningThreads > 0 {
		cfg.MiningThreads = f.MiningThreads
	}
	if f.MaxPeers > 0 {
		cfg.MaxPeers = f.MaxPeers
	}
	if f.LogLevel != "" {
		cfg.LogLevel = f.LogLevel
	}
	if f.ChainID > 0 {
		cfg.ChainID = f.ChainID
	}
	if f.GenesisPath != "" {
		cfg.GenesisPath = f.GenesisPath
	}
	if f.MinerAddress != "" {
		cfg.MinerAddress = f.MinerAddress
	}
	if f.AdminToken != "" {
		cfg.AdminToken = f.AdminToken
	}
	cfg.P2PEnable = cfg.P2PEnable || f.P2PEnable
	cfg.WSEnable = cfg.WSEnable || f.WSEnable
	if f.AIAuditorURL != "" {
		cfg.AIAuditorURL = f.AIAuditorURL
	}
	if f.RPCPort > 0 {
		cfg.RPCPort = f.RPCPort
	}
	cfg.EnableCORS = cfg.EnableCORS || f.EnableCORS
	cfg.TrustProxy = cfg.TrustProxy || f.TrustProxy
	if f.RateLimitRPS > 0 {
		cfg.RateLimitRPS = f.RateLimitRPS
	}
	if f.RateLimitBurst > 0 {
		cfg.RateLimitBurst = f.RateLimitBurst
	}
	if f.KeystoreDir != "" {
		cfg.KeystoreDir = f.KeystoreDir
	}
	return cfg
}

func parseUint64(s string) (uint64, error) {
	var v uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, &parseError{s}
		}
		v = v*10 + uint64(c-'0')
	}
	return v, nil
}

type parseError struct {
	s string
}

func (e *parseError) Error() string { return "invalid uint64: " + e.s }
