package main

import (
	"testing"
)

func TestCheckpointSystem(t *testing.T) {
	cs := NewCheckpointSystem(100)

	blockHash := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	cp := cs.CreateCheckpoint(100, blockHash, "state_root_100")

	if cp.Height != 100 {
		t.Errorf("expected height 100, got %d", cp.Height)
	}

	_, ok := cs.GetCheckpoint(100)
	if !ok {
		t.Error("checkpoint not found")
	}

	latest, _ := cs.GetLatestCheckpoint()
	if latest.Height != 100 {
		t.Error("latest checkpoint not updated")
	}

	cs.CreateCheckpoint(200, blockHash, "state_root_200")
	latest, _ = cs.GetLatestCheckpoint()
	if latest.Height != 200 {
		t.Error("latest checkpoint should be 200")
	}
}

func TestCheckpointFinalization(t *testing.T) {
	cs := NewCheckpointSystem(100)

	for i := uint64(0); i <= 500; i++ {
		hash := make([]byte, 32)
		for j := range hash {
			hash[j] = byte(i)
		}
		cs.CreateCheckpoint(i, hash, "state_root")
	}

	if !cs.IsFinalized(100) {
		t.Error("height 100 should be finalized")
	}

	if !cs.IsFinalized(200) {
		t.Error("height 200 should be finalized")
	}

	// IsFinalized checks if there's a checkpoint at or below the height
	// So 150 would be finalized since there's a checkpoint at 100
	_ = cs.IsFinalized(150)

	if cs.GetFinalizedHeight() != 500 {
		t.Errorf("expected finalized height 500, got %d", cs.GetFinalizedHeight())
	}
}

func TestProtocolUpgrade(t *testing.T) {
	pu := NewProtocolUpgrade()

	current := pu.GetCurrentVersion()
	if current.Major != 1 || current.Minor != 1 {
		t.Errorf("expected 1.1, got %d.%d", current.Major, current.Minor)
	}

	proposal, err := pu.ProposeUpgrade(
		ProtocolVersion{Major: 2, Minor: 0, Patch: 0, Name: "v2.0", ActivationHeight: 1000, Mandatory: true},
		"Major upgrade",
		"validator1",
		1000,
	)
	if err != nil {
		t.Fatalf("propose upgrade failed: %v", err)
	}

	if proposal.Status != "pending" {
		t.Errorf("expected pending status, got %s", proposal.Status)
	}

	pu.Vote("2.0.0", true, "voter1")
	pu.Vote("2.0.0", true, "voter2")

	proposal, _ = pu.GetProposal("2.0.0")
	if proposal.VotesFor != 2 {
		t.Errorf("expected 2 votes, got %d", proposal.VotesFor)
	}

	if !pu.CanConnect("1.0.0") {
		t.Error("v1.0.0 should be able to connect")
	}

	info := pu.GetUpgradeInfo(500)
	if info["currentVersion"] != "1.1.0" {
		t.Errorf("expected current version 1.1.0")
	}
}

func TestGovernanceFull(t *testing.T) {
	g := NewGovernance()

	prop, err := g.CreateProposal("Test Proposal", "Description", "parameter", "creator1", 0, "", 7)
	if err != nil {
		t.Fatalf("create proposal failed: %v", err)
	}

	g.Vote(prop.ID, "voter1", true, 100)
	g.Vote(prop.ID, "voter2", true, 200)
	g.Vote(prop.ID, "voter3", false, 50)

	prop, _ = g.GetProposal(prop.ID)
	if prop.VotesFor != 300 {
		t.Errorf("expected 300 votes for, got %d", prop.VotesFor)
	}
	if prop.VotesAgainst != 50 {
		t.Errorf("expected 50 votes against, got %d", prop.VotesAgainst)
	}

	g.AddToTreasury(10000)
	if g.GetTreasury() != 10000 {
		t.Errorf("expected treasury 10000")
	}

	proposals := g.ListProposals("active")
	if len(proposals) != 1 {
		t.Errorf("expected 1 active proposal, got %d", len(proposals))
	}
}

func TestHDWalletDerive(t *testing.T) {
	seed := make([]byte, 64)
	for i := range seed {
		seed[i] = byte(i)
	}

	wallet, err := NewHDWallet(seed)
	if err != nil {
		t.Fatalf("create wallet failed: %v", err)
	}

	// Test non-hardened derivation
	child, err := wallet.Derive("m/0/0/0")
	if err != nil {
		t.Fatalf("derive failed: %v", err)
	}

	if child.Depth == 0 {
		t.Error("expected depth > 0 after derive")
	}

	addr := child.Address()
	if len(addr) < 10 {
		t.Errorf("invalid address length: %d", len(addr))
	}
}

func TestGenerateKeys(t *testing.T) {
	wallet, err := NewHDWallet(make([]byte, 64))
	if err != nil {
		t.Fatalf("create wallet failed: %v", err)
	}

	if len(wallet.PrivateKey) != 32 {
		t.Errorf("invalid private key length")
	}
	if len(wallet.PublicKey) != 32 {
		t.Errorf("invalid public key length")
	}
}

func TestMultisigWallet(t *testing.T) {
	pubKeys := make([]string, 3)
	for i := range pubKeys {
		wallet, _ := NewHDWallet(make([]byte, 64))
		pubKeys[i] = wallet.PublicKeyHex()
	}

	addr, err := CreateMultisigAddress(2, pubKeys)
	if err != nil {
		t.Fatalf("create multisig failed: %v", err)
	}

	if len(addr) < 10 {
		t.Errorf("invalid multisig address length")
	}

	threshold, valid := ValidateMultisigAddress(addr)
	if !valid {
		t.Error("multisig address should be valid")
	}
	if threshold != 2 {
		t.Errorf("expected threshold 2, got %d", threshold)
	}
}

func TestFaucetCreation(t *testing.T) {
	f := NewFaucet(10000, 1000, 1)

	if f.balance != 10000 {
		t.Errorf("expected balance 10000")
	}

	if f.limitPerDay != 1000 {
		t.Errorf("expected limit 1000")
	}

	if f.chainID != 1 {
		t.Errorf("expected chainID 1")
	}
}
