package main

import (
	"crypto/ed25519"
	"testing"
)

func TestAddressIndexUpdatesOnMineAndReorg(t *testing.T) {
	store := newMemChainStore()

	_, minerPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	minerPub := minerPriv.Public().(ed25519.PublicKey)
	minerAddr := GenerateAddress(minerPub)

	_, recipPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	recipPub := recipPriv.Public().(ed25519.PublicKey)
	recipAddr := GenerateAddress(recipPub)

	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 1
	genesisPath := writeTestGenesisFile(t, t.TempDir(), defaultChainID, minerAddr, 1000000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)

	bc, err := LoadBlockchain(defaultChainID, minerAddr, store, 1000000)
	if err != nil {
		t.Fatal(err)
	}
	bc.consensus = ConsensusParams{
		DifficultyEnable:      false,
		GenesisDifficultyBits: 1,
		MinDifficultyBits:     1,
		MaxDifficultyBits:     255,
		MerkleEnable:          false,
		MonetaryPolicy:        testMonetaryPolicy(),
	}

	tx := Transaction{
		Type:       TxTransfer,
		ChainID:    defaultChainID,
		FromPubKey: minerPub,
		ToAddress:  recipAddr,
		Amount:     10,
		Fee:        1,
		Nonce:      1,
	}
	h, err := tx.SigningHash()
	if err != nil {
		t.Fatal(err)
	}
	tx.Signature = ed25519.Sign(minerPriv, h)
	if err := tx.Verify(); err != nil {
		t.Fatalf("tx verify: %v", err)
	}

	b1, err := bc.MineTransfers([]Transaction{tx})
	if err != nil {
		t.Fatal(err)
	}
	if b1.Height != 1 {
		t.Fatalf("expected height=1 got %d", b1.Height)
	}

	minerTxs, _, _ := bc.AddressTxs(minerAddr, 10, 0)
	if len(minerTxs) != 1 {
		t.Fatalf("expected miner address to have 1 tx after mining")
	}
	recipTxs, _, _ := bc.AddressTxs(recipAddr, 10, 0)
	if len(recipTxs) != 1 {
		t.Fatalf("expected recipient address to have 1 tx after mining")
	}

	// Build a higher-work alternative block at height 1 with no transfer txs.
	gen := bc.blocks[0]
	alt := &Block{
		Version:        1,
		Height:         1,
		TimestampUnix:  gen.TimestampUnix + 1,
		PrevHash:       append([]byte(nil), gen.Hash...),
		DifficultyBits: 2,
		MinerAddress:   bc.MinerAddress,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: bc.MinerAddress,
			Amount:    bc.consensus.MonetaryPolicy.BlockReward(1),
			Data:      "block reward + fees (height=1)",
		}},
	}
	mineTestBlock(t, bc.consensus, alt)

	reorg, err := bc.AddBlock(alt)
	if err != nil {
		t.Fatal(err)
	}
	if !reorg {
		t.Fatalf("expected reorg to higher-work alt block")
	}

	minerTxs, _, _ = bc.AddressTxs(minerAddr, 10, 0)
	if len(minerTxs) != 0 {
		t.Fatalf("expected miner address to have no canonical transfer txs after reorg")
	}
	recipTxs, _, _ = bc.AddressTxs(recipAddr, 10, 0)
	if len(recipTxs) != 0 {
		t.Fatalf("expected recipient address to have no canonical transfer txs after reorg")
	}
}
