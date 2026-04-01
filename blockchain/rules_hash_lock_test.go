package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRulesHashMismatchOnRestart(t *testing.T) {
	store, err := OpenBoltStore(filepath.Join(t.TempDir(), "chain.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.db.Close() })

	wallet, err := NewWallet()
	if err != nil {
		t.Fatalf("new wallet: %v", err)
	}

	t.Setenv("MINER_ADDRESS", wallet.Address)

	consensus := defaultTestConsensusJSON()
	consensus.MaxBlockSize = 1_000_000
	genesisPath := writeTestGenesisFile(t, t.TempDir(), 1, wallet.Address, 1000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)

	if _, err := LoadBlockchain(1, wallet.Address, store, 1000); err != nil {
		t.Fatalf("first load: %v", err)
	}

	consensus.MaxBlockSize = 2_000_000
	_ = writeTestGenesisFile(t, filepath.Dir(genesisPath), 1, wallet.Address, 1000, consensus)

	if _, err := LoadBlockchain(1, wallet.Address, store, 1000); err == nil {
		t.Fatal("expected consensus params mismatch error")
	} else if !strings.Contains(err.Error(), "consensus params mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
