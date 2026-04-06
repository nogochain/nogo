package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/utils"
)

const (
	// defaultConfigName is the default configuration file name
	defaultConfigName = "config.json"

	// envVarPrefix is the prefix for environment variables
	envVarPrefix = "NOGO"
)

// CLI represents the command-line interface
type CLI struct {
	configPath string
	dataDir    string
	network    string
	verbose    bool
	jsonOutput bool
}

// NewCLI creates a new CLI instance
func NewCLI() *CLI {
	return &CLI{
		configPath: getDefaultConfigPath(),
		dataDir:    utils.DefaultDataDir(),
		network:    "mainnet",
		verbose:    false,
		jsonOutput: false,
	}
}

// getDefaultConfigPath returns the default configuration file path
func getDefaultConfigPath() string {
	configDir := getConfigDir()
	return filepath.Join(configDir, defaultConfigName)
}

// getConfigDir returns the configuration directory
func getConfigDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "NogoChain")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(homeDir, ".nogochain")
}

// ParseFlags parses command-line flags
func (cli *CLI) ParseFlags(args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			continue
		}

		flag := strings.TrimPrefix(arg, "-")
		var value string

		if idx := strings.Index(flag, "="); idx != -1 {
			value = flag[idx+1:]
			flag = flag[:idx]
		} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			value = args[i+1]
			i++
		}

		if err := cli.setFlag(flag, value); err != nil {
			return fmt.Errorf("invalid flag %s: %w", arg, err)
		}
	}

	return nil
}

// setFlag sets a single flag value
func (cli *CLI) setFlag(flag, value string) error {
	switch flag {
	case "config", "c":
		cli.configPath = value
	case "datadir":
		cli.dataDir = value
	case "network":
		cli.network = value
	case "verbose", "v":
		cli.verbose = true
	case "json":
		cli.jsonOutput = true
	case "version":
		printVersion()
		os.Exit(0)
	case "help", "h":
		printHelp()
		os.Exit(0)
	default:
		return fmt.Errorf("unknown flag: %s", flag)
	}
	return nil
}

// LoadConfig loads configuration from file and environment
func (cli *CLI) LoadConfig() (*config.Config, error) {
	cfg, err := config.LoadConfigFromFile(cli.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if err := cli.applyOverrides(cfg); err != nil {
		return nil, fmt.Errorf("failed to apply overrides: %w", err)
	}

	return cfg, nil
}

// applyOverrides applies configuration overrides
func (cli *CLI) applyOverrides(cfg *config.Config) error {
	if cli.dataDir != "" {
		cfg.DataDir = cli.dataDir
	}

	if cli.network != "" {
		cfg.Network.Name = cli.network
	}

	config.ApplyEnvOverrides(cfg)

	return nil
}

// PrintInfo prints node information
func (cli *CLI) PrintInfo(cfg *config.Config) {
	if cli.jsonOutput {
		cli.printInfoJSON(cfg)
	} else {
		cli.printInfoText(cfg)
	}
}

// printInfoText prints node information in text format
func (cli *CLI) printInfoText(cfg *config.Config) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NogoChain Node Information")
	fmt.Fprintln(w, "========================")
	fmt.Fprintf(w, "Version:\t%s\n", version)
	fmt.Fprintf(w, "Go Version:\t%s\n", runtime.Version())
	fmt.Fprintf(w, "OS/Arch:\t%s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(w, "Network:\t%s\n", cfg.Network.Name)
	fmt.Fprintf(w, "Data Directory:\t%s\n", cfg.DataDir)
	fmt.Fprintf(w, "P2P Port:\t%d\n", cfg.P2P.Port)
	fmt.Fprintf(w, "API Port:\t%d\n", cfg.API.HTTPPort)
	fmt.Fprintf(w, "Mining Enabled:\t%v\n", cfg.Mining.Enabled)
	fmt.Fprintf(w, "Max Peers:\t%d\n", cfg.P2P.MaxPeers)
	w.Flush()
}

// printInfoJSON prints node information in JSON format
func (cli *CLI) printInfoJSON(cfg *config.Config) {
	info := map[string]interface{}{
		"version":       version,
		"go_version":    runtime.Version(),
		"os_arch":       fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		"network":       cfg.Network.Name,
		"data_dir":      cfg.DataDir,
		"p2p_port":      cfg.P2P.Port,
		"api_port":      cfg.API.HTTPPort,
		"mining":        cfg.Mining.Enabled,
		"max_peers":     cfg.P2P.MaxPeers,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(info)
}

// printVersion prints the version information
func printVersion() {
	fmt.Printf("%s v%s (%s %s/%s)\n",
		appName,
		version,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	)
}

// printHelp prints the help message
func printHelp() {
	fmt.Printf("%s - Decentralized Blockchain Node\n\n", appName)
	fmt.Println("Usage: nogo [options]")
	fmt.Println("\nOptions:")
	fmt.Println("  -config, -c <path>    Configuration file path")
	fmt.Println("  -datadir <path>       Data directory path")
	fmt.Println("  -network <name>       Network name (mainnet/testnet/devnet)")
	fmt.Println("  -verbose, -v          Enable verbose output")
	fmt.Println("  -json                 Output in JSON format")
	fmt.Println("  -version              Print version information")
	fmt.Println("  -help, -h             Print this help message")
	fmt.Println("\nEnvironment Variables:")
	fmt.Printf("  %s_DATADIR              Data directory\n", envVarPrefix)
	fmt.Printf("  %s_NETWORK              Network name\n", envVarPrefix)
	fmt.Printf("  %s_P2P_PORT             P2P listening port\n", envVarPrefix)
	fmt.Printf("  %s_API_PORT             API listening port\n", envVarPrefix)
}

// InitConfig creates a default configuration file
func (cli *CLI) InitConfig() error {
	cfg := config.DefaultConfig()

	configDir := filepath.Dir(cli.configPath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(cli.configPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Configuration file created at %s\n", cli.configPath)
	return nil
}

// ValidateConfig validates the configuration
func (cli *CLI) ValidateConfig(cfg *config.Config) error {
	if cfg.DataDir == "" {
		return fmt.Errorf("data directory is required")
	}

	if cfg.P2P.Port < 1 || cfg.P2P.Port > 65535 {
		return fmt.Errorf("P2P port must be between 1 and 65535")
	}

	if cfg.API.HTTPPort < 1 || cfg.API.HTTPPort > 65535 {
		return fmt.Errorf("API port must be between 1 and 65535")
	}

	if cfg.P2P.MaxPeers < 1 {
		return fmt.Errorf("max peers must be at least 1")
	}

	return nil
}

// runCLI handles CLI command execution
func runCLI(args []string) error {
	cli := NewCLI()

	if err := cli.ParseFlags(args); err != nil {
		return err
	}

	if len(args) > 0 && args[0] == "init" {
		return cli.InitConfig()
	}

	if len(args) > 0 && args[0] == "info" {
		cfg, err := cli.LoadConfig()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		if err := cli.ValidateConfig(cfg); err != nil {
			return fmt.Errorf("validate config: %w", err)
		}

		cli.PrintInfo(cfg)
		return nil
	}

	return nil
}
