// Package main provides environment variable handling for the NogoChain node.
//
// This file implements environment variable management including:
//   - Loading environment variables with prefix
//   - Type-safe parsing (string, int, bool, duration, list)
//   - Validation of environment variable values
//   - Default value support
//   - Setting and clearing environment variables
//
// All environment variables use the NOGO_ prefix for consistency.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultLogLevel = "info"

// EnvVars holds all environment variable values
type EnvVars struct {
	DataDir            string
	Network            string
	P2PPort            int
	P2PMaxPeers        int
	APIPort            int
	APICorsDomains     []string
	MiningEnabled      bool
	MiningThreads      int
	LogLevel           string
	MetricsEnabled     bool
	MetricsPort        int
	BootstrapNodes     []string
	GenesisFile        string
	DisableUPNP        bool
	ExternalIP         string
	Whitelist          []string
	MaxInboundPeers    int
	MaxOutboundPeers   int
	PeerConnectTimeout time.Duration
}

// LoadEnvVars loads environment variables with the given prefix
func LoadEnvVars(prefix string) (*EnvVars, error) {
	env := &EnvVars{
		DataDir:            getEnvString(prefix+"_DATADIR", ""),
		Network:            getEnvString(prefix+"_NETWORK", "mainnet"),
		P2PPort:            getEnvInt(prefix+"_P2P_PORT", 30303),
		P2PMaxPeers:        getEnvInt(prefix+"_P2P_MAX_PEERS", 50),
		APIPort:            getEnvInt(prefix+"_API_PORT", 8545),
		APICorsDomains:     getEnvStringList(prefix+"_API_CORS", []string{"*"}),
		MiningEnabled:      getEnvBool(prefix+"_MINING_ENABLED", false),
		MiningThreads:      getEnvInt(prefix+"_MINING_THREADS", 0),
		LogLevel:           getEnvString(prefix+"_LOG_LEVEL", defaultLogLevel),
		MetricsEnabled:     getEnvBool(prefix+"_METRICS_ENABLED", true),
		MetricsPort:        getEnvInt(prefix+"_METRICS_PORT", 9090),
		BootstrapNodes:     getEnvStringList(prefix+"_BOOTSTRAP_NODES", []string{}),
		GenesisFile:        getEnvString(prefix+"_GENESIS_FILE", ""),
		DisableUPNP:        getEnvBool(prefix+"_DISABLE_UPNP", false),
		ExternalIP:         getEnvString(prefix+"_EXTERNAL_IP", ""),
		Whitelist:          getEnvStringList(prefix+"_WHITELIST", []string{}),
		MaxInboundPeers:    getEnvInt(prefix+"_MAX_INBOUND_PEERS", 40),
		MaxOutboundPeers:   getEnvInt(prefix+"_MAX_OUTBOUND_PEERS", 10),
		PeerConnectTimeout: getEnvDuration(prefix+"_PEER_CONNECT_TIMEOUT", 10*time.Second),
	}

	if err := env.validate(); err != nil {
		return nil, fmt.Errorf("invalid environment variables: %w", err)
	}

	return env, nil
}

// validate validates the environment variables
func (e *EnvVars) validate() error {
	if e.P2PPort < 1 || e.P2PPort > 65535 {
		return fmt.Errorf("P2P port must be between 1 and 65535, got %d", e.P2PPort)
	}

	if e.APIPort < 1 || e.APIPort > 65535 {
		return fmt.Errorf("API port must be between 1 and 65535, got %d", e.APIPort)
	}

	if e.MetricsPort < 1 || e.MetricsPort > 65535 {
		return fmt.Errorf("Metrics port must be between 1 and 65535, got %d", e.MetricsPort)
	}

	if e.P2PMaxPeers < 1 {
		return fmt.Errorf("P2P max peers must be at least 1")
	}

	if e.MiningThreads < 0 {
		return fmt.Errorf("Mining threads cannot be negative")
	}

	validNetworks := map[string]bool{
		"mainnet": true,
		"testnet": true,
		"devnet":  true,
		"regtest": true,
	}

	if !validNetworks[e.Network] {
		return fmt.Errorf("invalid network: %s", e.Network)
	}

	return nil
}

// getEnvString gets a string environment variable with a default value
func getEnvString(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return strings.TrimSpace(value)
	}
	return defaultValue
}

// getEnvInt gets an integer environment variable with a default value
func getEnvInt(key string, defaultValue int) int {
	valueStr := getEnvString(key, "")
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

// getEnvBool gets a boolean environment variable with a default value
func getEnvBool(key string, defaultValue bool) bool {
	valueStr := getEnvString(key, "")
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

// getEnvStringList gets a comma-separated string list environment variable
func getEnvStringList(key string, defaultValue []string) []string {
	valueStr := getEnvString(key, "")
	if valueStr == "" {
		return defaultValue
	}

	parts := strings.Split(valueStr, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}

	if len(result) == 0 {
		return defaultValue
	}

	return result
}

// getEnvDuration gets a duration environment variable with a default value
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	valueStr := getEnvString(key, "")
	if valueStr == "" {
		return defaultValue
	}

	duration, err := time.ParseDuration(valueStr)
	if err != nil {
		return defaultValue
	}
	return duration
}

// SetEnvVars sets environment variables from the given struct
func SetEnvVars(prefix string, env *EnvVars) error {
	if err := os.Setenv(prefix+"_DATADIR", env.DataDir); err != nil {
		return err
	}
	if err := os.Setenv(prefix+"_NETWORK", env.Network); err != nil {
		return err
	}
	if err := os.Setenv(prefix+"_P2P_PORT", strconv.Itoa(env.P2PPort)); err != nil {
		return err
	}
	if err := os.Setenv(prefix+"_P2P_MAX_PEERS", strconv.Itoa(env.P2PMaxPeers)); err != nil {
		return err
	}
	if err := os.Setenv(prefix+"_API_PORT", strconv.Itoa(env.APIPort)); err != nil {
		return err
	}
	if err := os.Setenv(prefix+"_MINING_ENABLED", strconv.FormatBool(env.MiningEnabled)); err != nil {
		return err
	}
	if err := os.Setenv(prefix+"_MINING_THREADS", strconv.Itoa(env.MiningThreads)); err != nil {
		return err
	}
	if err := os.Setenv(prefix+"_LOG_LEVEL", env.LogLevel); err != nil {
		return err
	}
	if err := os.Setenv(prefix+"_METRICS_ENABLED", strconv.FormatBool(env.MetricsEnabled)); err != nil {
		return err
	}
	if err := os.Setenv(prefix+"_METRICS_PORT", strconv.Itoa(env.MetricsPort)); err != nil {
		return err
	}

	return nil
}

// ClearEnvVars clears all environment variables with the given prefix
func ClearEnvVars(prefix string) error {
	envVars := []string{
		"DATADIR", "NETWORK", "P2P_PORT", "P2P_MAX_PEERS",
		"API_PORT", "API_CORS", "MINING_ENABLED", "MINING_THREADS",
		"LOG_LEVEL", "METRICS_ENABLED", "METRICS_PORT",
		"BOOTSTRAP_NODES", "GENESIS_FILE", "DISABLE_UPNP",
		"EXTERNAL_IP", "WHITELIST", "MAX_INBOUND_PEERS",
		"MAX_OUTBOUND_PEERS", "PEER_CONNECT_TIMEOUT",
	}

	for _, name := range envVars {
		if err := os.Unsetenv(prefix + "_" + name); err != nil {
			return err
		}
	}

	return nil
}

// HasEnvVar checks if an environment variable is set
func HasEnvVar(key string) bool {
	_, exists := os.LookupEnv(key)
	return exists
}

// RequireEnvVar requires an environment variable to be set
func RequireEnvVar(key string) (string, error) {
	value, exists := os.LookupEnv(key)
	if !exists {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	if value == "" {
		return "", fmt.Errorf("required environment variable %s is empty", key)
	}
	return value, nil
}
