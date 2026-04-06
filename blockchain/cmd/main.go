package main

import (
	"fmt"
	"os"
	"strings"
)

var (
	version   = "NogoChain"
	buildTime = "unknown"
	gitCommit string
	appName   = "nogo"

	mainnetConfigHardcoded = NodeConfig{
		ChainID:              1,
		HTTPAddr:             "0.0.0.0:8080",
		P2PListenAddr:        "0.0.0.0:9090",
		P2PPeers:             "main.nogochain.org:9090",
		P2PAdvertiseSelf:     true,
		P2PMaxPeers:          1000,
		P2PMaxConnections:    50,
		SyncEnable:           true,
		MineForceEmptyBlocks: true,
		MaxTxPerBlock:        10000,
		MineIntervalMs:       17000,
		MetricsEnabled:       true,
		MetricsAddr:          "0.0.0.0:9100",
		DataDir:              "./data",
		RateLimitReqs:        100,
		RateLimitBurst:       50,
	}

	testnetConfigHardcoded = NodeConfig{
		ChainID:              2,
		HTTPAddr:             "0.0.0.0:8080",
		P2PListenAddr:        "0.0.0.0:9090",
		P2PPeers:             "test.nogochain.org:9090",
		P2PAdvertiseSelf:     true,
		P2PMaxPeers:          1000,
		P2PMaxConnections:    50,
		SyncEnable:           true,
		MineForceEmptyBlocks: true,
		MaxTxPerBlock:        10000,
		MineIntervalMs:       15000,
		MetricsEnabled:       true,
		MetricsAddr:          "0.0.0.0:9100",
		DataDir:              "./data-testnet",
		RateLimitReqs:        100,
		RateLimitBurst:       50,
	}
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "server":
		handleServerCommand()
	default:
		if err := runCLI(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			usage()
			os.Exit(2)
		}
	}
}

func handleServerCommand() {
	SetGlobalFormatter(true)
	log := GetGlobalFormatter()

	if len(os.Args) < 3 {
		log.Error("Usage: %s server <miner_address> [mine] [test]", os.Args[0])
		log.Error("Example: %s server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine test", os.Args[0])
		os.Exit(1)
	}

	miner := strings.TrimSpace(os.Args[2])

	autoMine := false
	isTestnet := false

	for i := 3; i < len(os.Args); i++ {
		arg := strings.ToLower(strings.TrimSpace(os.Args[i]))
		if arg == "mine" {
			autoMine = true
		} else if arg == "test" {
			isTestnet = true
		}
	}

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		adminToken = miner
	}

	var cfg NodeConfig
	if isTestnet {
		cfg = testnetConfigHardcoded
	} else {
		cfg = mainnetConfigHardcoded
	}

	node := NewNode(cfg, miner, adminToken, autoMine, isTestnet)

	if err := node.Start(); err != nil {
		log.Error("Node failed: %v", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <command> [arguments]\n", appName)
	fmt.Fprintf(os.Stderr, "\nCommands:\n")
	fmt.Fprintf(os.Stderr, "  server <miner_address> [mine] [test]  Start the blockchain node\n")
	fmt.Fprintf(os.Stderr, "  wallet create                       Create a new wallet\n")
	fmt.Fprintf(os.Stderr, "  wallet sign                         Sign a transaction\n")
	fmt.Fprintf(os.Stderr, "  tx submit                           Submit a transaction\n")
	fmt.Fprintf(os.Stderr, "  block get                           Get block information\n")
	fmt.Fprintf(os.Stderr, "  balance <address>                   Get balance for an address\n")
	fmt.Fprintf(os.Stderr, "\nFlags:\n")
	fmt.Fprintf(os.Stderr, "  mine    Enable automatic mining\n")
	fmt.Fprintf(os.Stderr, "  test    Use testnet instead of mainnet\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  %s server NOGO... mine              Start node with mining on mainnet\n", appName)
	fmt.Fprintf(os.Stderr, "  %s server NOGO... mine test         Start node with mining on testnet\n", appName)
	fmt.Fprintf(os.Stderr, "  %s server NOGO...                   Start node without mining (sync only)\n", appName)
	fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
	fmt.Fprintf(os.Stderr, "  ADMIN_TOKEN    Admin authentication token (defaults to miner address)\n")
	fmt.Fprintf(os.Stderr, "\nFor more information, visit https://nogochain.org\n")
}

func getGitCommit() string {
	return gitCommit
}
