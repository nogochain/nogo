package main

import (
	"encoding/hex"
	"path/filepath"
	"testing"
)

func TestGenesisDeterministicWithGenesisToAddress(t *testing.T) {
	genesisWallet, err := NewWallet()
	if err != nil {
		t.Fatalf("new genesis wallet: %v", err)
	}

	miner1, err := NewWallet()
	if err != nil {
		t.Fatalf("new miner1 wallet: %v", err)
	}
	miner2, err := NewWallet()
	if err != nil {
		t.Fatalf("new miner2 wallet: %v", err)
	}

	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 2
	genesisPath := writeTestGenesisFile(t, t.TempDir(), 42, genesisWallet.Address, 1000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)

	store1, err := OpenGobStore(filepath.Join(t.TempDir(), "blocks.gob"))
	if err != nil {
		t.Fatalf("open store1: %v", err)
	}
	bc1, err := LoadBlockchain(42, miner1.Address, store1, 1000)
	if err != nil {
		t.Fatalf("load bc1: %v", err)
	}

	store2, err := OpenGobStore(filepath.Join(t.TempDir(), "blocks.gob"))
	if err != nil {
		t.Fatalf("open store2: %v", err)
	}
	bc2, err := LoadBlockchain(42, miner2.Address, store2, 1000)
	if err != nil {
		t.Fatalf("load bc2: %v", err)
	}

	g1, ok := bc1.BlockByHeight(0)
	if !ok {
		t.Fatal("missing genesis on bc1")
	}
	g2, ok := bc2.BlockByHeight(0)
	if !ok {
		t.Fatal("missing genesis on bc2")
	}

	if g1.TimestampUnix != 1700000000 {
		t.Fatalf("unexpected genesis timestamp: %d", g1.TimestampUnix)
	}
	if g1.MinerAddress != genesisWallet.Address {
		t.Fatalf("unexpected genesis miner address: %s", g1.MinerAddress)
	}
	if len(g1.Transactions) != 1 || g1.Transactions[0].ToAddress != genesisWallet.Address {
		t.Fatalf("unexpected genesis allocation")
	}

	h1 := hex.EncodeToString(g1.Hash)
	h2 := hex.EncodeToString(g2.Hash)
	if h1 != h2 {
		t.Fatalf("genesis mismatch: bc1=%s bc2=%s", h1, h2)
	}
}
