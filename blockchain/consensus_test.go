package main

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/nogopow"
)

func TestBlockVersionForHeight(t *testing.T) {
	p := ConsensusParams{MerkleEnable: false, MerkleActivationHeight: 0}
	if got := blockVersionForHeight(p, 0); got != 1 {
		t.Fatalf("got %d", got)
	}
	p = ConsensusParams{MerkleEnable: true, MerkleActivationHeight: 10}
	if got := blockVersionForHeight(p, 9); got != 1 {
		t.Fatalf("got %d", got)
	}
	if got := blockVersionForHeight(p, 10); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := blockVersionForHeight(p, 11); got != 2 {
		t.Fatalf("got %d", got)
	}
}

func testMonetaryPolicy() MonetaryPolicy {
	return MonetaryPolicy{
		InitialBlockReward: 50,
		HalvingInterval:    210000,
		MinerFeeShare:      100,
	}
}

func TestApplyBlockToState_CoinbaseEconomics(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	minerAddr := GenerateAddress(pub)

	_, recipPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	recipPub := recipPriv.Public().(ed25519.PublicKey)
	recipAddr := GenerateAddress(recipPub)

	state := map[string]Account{}
	policy := testMonetaryPolicy()
	p := ConsensusParams{MonetaryPolicy: policy}
	genesis := &Block{
		Version:        1,
		Height:         0,
		DifficultyBits: 1,
		MinerAddress:   minerAddr,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   defaultChainID,
			ToAddress: minerAddr,
			Amount:    1000000,
			Data:      "genesis",
		}},
	}
	if err := applyBlockToState(p, state, genesis); err != nil {
		t.Fatalf("apply genesis: %v", err)
	}

	tx := Transaction{
		Type:       TxTransfer,
		ChainID:    defaultChainID,
		FromPubKey: pub,
		ToAddress:  recipAddr,
		Amount:     10,
		Fee:        1,
		Nonce:      1,
	}
	h, err := tx.SigningHash()
	if err != nil {
		t.Fatal(err)
	}
	tx.Signature = ed25519.Sign(priv, h)
	if err := tx.Verify(); err != nil {
		t.Fatalf("tx verify: %v", err)
	}

	bad := &Block{
		Version:        1,
		Height:         1,
		DifficultyBits: 1,
		MinerAddress:   minerAddr,
		Transactions: []Transaction{
			{
				Type:      TxCoinbase,
				ChainID:   defaultChainID,
				ToAddress: minerAddr,
				Amount:    policy.BlockReward(1), // should be blockReward + fee
				Data:      "block reward + fees (height=1)",
			},
			tx,
		},
	}
	if err := applyBlockToState(p, state, bad); err == nil {
		t.Fatalf("expected coinbase economics error")
	}

	good := &Block{
		Version:        1,
		Height:         1,
		DifficultyBits: 1,
		MinerAddress:   minerAddr,
		Transactions: []Transaction{
			{
				Type:      TxCoinbase,
				ChainID:   defaultChainID,
				ToAddress: minerAddr,
				Amount:    policy.BlockReward(1) + 1,
				Data:      "block reward + fees (height=1)",
			},
			tx,
		},
	}
	if err := applyBlockToState(p, state, good); err != nil {
		t.Fatalf("apply good: %v", err)
	}
}

func TestValidateBlockTime(t *testing.T) {
	p := ConsensusParams{MedianTimePastWindow: 3, MaxTimeDrift: 100}
	path := []*Block{
		{Height: 0, TimestampUnix: 10},
		{Height: 1, TimestampUnix: 20},
		{Height: 2, TimestampUnix: 30},
	}
	// Next must be > prev and > MTP(heights 0..2) = 20.
	ok := &Block{Height: 3, TimestampUnix: 31}
	path = append(path, ok)
	if err := validateBlockTime(p, path, 3); err != nil {
		t.Fatalf("expected ok: %v", err)
	}

	// Too old: <= MTP which is 20 for endIdx=2.
	path[3].TimestampUnix = 20
	if err := validateBlockTime(p, path, 3); err == nil {
		t.Fatalf("expected mtp error")
	}
}

func TestWorkForDifficultyBitsMonotonic(t *testing.T) {
	a := WorkForDifficultyBits(10)
	b := WorkForDifficultyBits(11)
	if b.Cmp(a) <= 0 {
		t.Fatalf("expected work to increase with bits")
	}
}

func TestNextDifficultyBits_DisabledCarriesParent(t *testing.T) {
	// This test is deprecated - difficulty calculation now uses nogopow engine
	// The old nextDifficultyBitsFromPath function has been removed
	// Difficulty adjustment is now handled by nogopow.DifficultyAdjuster
	t.Skip("deprecated - difficulty calculation moved to nogopow package")
}

type memChainStore struct {
	blocksByHash map[string]*Block
	canonical    []*Block
	rulesHash    []byte
	genesisHash  []byte
}

func newMemChainStore() *memChainStore {
	return &memChainStore{
		blocksByHash: map[string]*Block{},
	}
}

func (s *memChainStore) ReadCanonical() ([]*Block, error) {
	return append([]*Block(nil), s.canonical...), nil
}
func (s *memChainStore) ReadAllBlocks() (map[string]*Block, error) {
	out := map[string]*Block{}
	for k, v := range s.blocksByHash {
		out[k] = v
	}
	return out, nil
}
func (s *memChainStore) PutBlock(b *Block) error {
	if b == nil || len(b.Hash) == 0 {
		return nil
	}
	s.blocksByHash[hex.EncodeToString(b.Hash)] = b
	return nil
}
func (s *memChainStore) AppendCanonical(b *Block) error {
	if err := s.PutBlock(b); err != nil {
		return err
	}
	s.canonical = append(s.canonical, b)
	return nil
}
func (s *memChainStore) RewriteCanonical(blocks []*Block) error {
	s.canonical = append([]*Block(nil), blocks...)
	for _, b := range blocks {
		_ = s.PutBlock(b)
	}
	return nil
}

func (s *memChainStore) GetRulesHash() ([]byte, bool, error) {
	if len(s.rulesHash) == 0 {
		return nil, false, nil
	}
	return append([]byte(nil), s.rulesHash...), true, nil
}

func (s *memChainStore) PutRulesHash(hash []byte) error {
	s.rulesHash = append([]byte(nil), hash...)
	return nil
}

func (s *memChainStore) GetGenesisHash() ([]byte, bool, error) {
	if len(s.genesisHash) == 0 {
		return nil, false, nil
	}
	return append([]byte(nil), s.genesisHash...), true, nil
}

func (s *memChainStore) PutGenesisHash(hash []byte) error {
	s.genesisHash = append([]byte(nil), hash...)
	return nil
}

func mineTestBlock(t *testing.T, p ConsensusParams, b *Block) {
	t.Helper()
	
	// Mine using NogoPow engine
	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()
	
	header := &nogopow.Header{
		ParentHash: nogopow.BytesToHash(b.PrevHash),
		Coinbase:   stringToAddress(b.MinerAddress),
		Number:     big.NewInt(int64(b.Height)),
		Time:       uint64(b.TimestampUnix),
		Difficulty: big.NewInt(int64(b.DifficultyBits)),
	}
	
	block := nogopow.NewBlock(header, nil, nil, nil)
	stop := make(chan struct{})
	resultCh := make(chan *nogopow.Block, 1)
	
	if err := engine.Seal(nil, block, resultCh, stop); err != nil {
		t.Fatal(err)
	}
	
	result, ok := <-resultCh
	if !ok {
		close(stop)
		t.Fatal("mining failed: channel closed")
	}
	
	sealedHeader := result.Header()
	b.Nonce = binary.LittleEndian.Uint64(sealedHeader.Nonce[:8])
	b.Hash = sealedHeader.Hash().Bytes()
}

func TestForkChoicePrefersMoreWork(t *testing.T) {
	store := newMemChainStore()
	miner := TestAddressMiner

	// Build a chain with low difficulty so mining is fast/deterministic.
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 2
	genesisPath := writeTestGenesisFile(t, t.TempDir(), defaultChainID, miner, 1000000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)
	bc, err := LoadBlockchain(defaultChainID, miner, store, 1000000)
	if err != nil {
		t.Fatal(err)
	}
	// Force consensus params (tests shouldn't depend on env beyond bootstrap).
	policy := testMonetaryPolicy()
	bc.consensus = ConsensusParams{
		DifficultyEnable:      false,
		GenesisDifficultyBits: 2,
		MinDifficultyBits:     1,
		MaxDifficultyBits:     255,
		MerkleEnable:          false,
		MonetaryPolicy:        policy,
	}

	// Mine canonical block 1 and 2 with difficulty 4.
	b1, err := bc.MineTransfers(nil)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := bc.MineTransfers(nil)
	if err != nil {
		t.Fatal(err)
	}
	if b2.Height != 2 || b1.Height != 1 {
		t.Fatalf("unexpected heights")
	}

	// Create an alternative block at height 2 with higher difficulty -> more work.
	alt := &Block{
		Version:        1,
		Height:         2,
		TimestampUnix:  b1.TimestampUnix + 2,
		PrevHash:       append([]byte(nil), b1.Hash...),
		DifficultyBits: 6,
		MinerAddress:   bc.MinerAddress,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: bc.MinerAddress,
			Amount:    policy.BlockReward(2),
			Data:      "block reward + fees (height=2)",
		}},
	}
	mineTestBlock(t, bc.consensus, alt)

	reorg, err := bc.AddBlock(alt)
	if err != nil {
		t.Fatal(err)
	}
	if !reorg {
		t.Fatalf("expected reorg to higher-work fork")
	}
	latest := bc.LatestBlock()
	if hex.EncodeToString(latest.Hash) != hex.EncodeToString(alt.Hash) {
		t.Fatalf("expected alt to be tip")
	}
	_ = b2
}

func TestDifficultyEnforcedWhenEnabled(t *testing.T) {
	store := newMemChainStore()
	miner := TestAddressMiner
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 2
	genesisPath := writeTestGenesisFile(t, t.TempDir(), defaultChainID, miner, 1000000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)
	bc, err := LoadBlockchain(defaultChainID, miner, store, 1000000)
	if err != nil {
		t.Fatal(err)
	}
	policy := testMonetaryPolicy()
	bc.consensus = ConsensusParams{
		DifficultyEnable:      true,
		TargetBlockTime:       10 * time.Second,
		DifficultyWindow:      1,
		DifficultyMaxStep:     1,
		MinDifficultyBits:     1,
		MaxDifficultyBits:     255,
		GenesisDifficultyBits: 2,
		MedianTimePastWindow:  1,
		MaxTimeDrift:          100000,
		MonetaryPolicy:        policy,
	}

	gen := bc.blocks[0]
	gen.TimestampUnix = 100
	gen.DifficultyBits = 2
	mineTestBlock(t, bc.consensus, gen)
	_ = store.RewriteCanonical([]*Block{gen})
	bc.blocks = []*Block{gen}
	bc.blocksByHash = map[string]*Block{hex.EncodeToString(gen.Hash): gen}
	bc.initCanonicalIndexesLocked()

	// Calculate expected difficulty for block 1 using nogopow engine
	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()
	
	parentHeader := &nogopow.Header{
		Number:     big.NewInt(int64(gen.Height)),
		Time:       uint64(gen.TimestampUnix),
		Difficulty: big.NewInt(int64(gen.DifficultyBits)),
	}
	expectedB1Difficulty := engine.CalcDifficulty(nil, 101, parentHeader)
	expectedB1Bits := uint32(expectedB1Difficulty.Uint64())
	
	// Block 1 uses calculated difficulty
	b1 := &Block{
		Version:        1,
		Height:         1,
		TimestampUnix:  101,
		PrevHash:       append([]byte(nil), gen.Hash...),
		DifficultyBits: expectedB1Bits,
		MinerAddress:   bc.MinerAddress,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: bc.MinerAddress,
			Amount:    policy.BlockReward(1),
			Data:      "block reward + fees (height=1)",
		}},
	}
	mineTestBlock(t, bc.consensus, b1)
	_, err = bc.AddBlock(b1)
	if err != nil {
		t.Fatal(err)
	}

	// Calculate expected difficulty for block 2 using nogopow engine (reuse engine from above)
	parentHeader2 := &nogopow.Header{
		Number:     big.NewInt(int64(b1.Height)),
		Time:       uint64(b1.TimestampUnix),
		Difficulty: big.NewInt(int64(b1.DifficultyBits)),
	}
	expectedDifficulty := engine.CalcDifficulty(nil, 102, parentHeader2)
	expectedBits := uint32(expectedDifficulty.Uint64())
	
	b2Wrong := &Block{
		Version:        1,
		Height:         2,
		TimestampUnix:  102,
		PrevHash:       append([]byte(nil), b1.Hash...),
		DifficultyBits: 2, // wrong
		MinerAddress:   bc.MinerAddress,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: bc.MinerAddress,
			Amount:    policy.BlockReward(2),
			Data:      "block reward + fees (height=2)",
		}},
	}
	mineTestBlock(t, bc.consensus, b2Wrong)
	_, err = bc.AddBlock(b2Wrong)
	if err == nil {
		t.Fatalf("expected difficulty mismatch error")
	}

	b2 := &Block{
		Version:        1,
		Height:         2,
		TimestampUnix:  102,
		PrevHash:       append([]byte(nil), b1.Hash...),
		DifficultyBits: expectedBits,
		MinerAddress:   bc.MinerAddress,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: bc.MinerAddress,
			Amount:    policy.BlockReward(2),
			Data:      "block reward + fees (height=2)",
		}},
	}
	mineTestBlock(t, bc.consensus, b2)
	_, err = bc.AddBlock(b2)
	if err != nil {
		t.Fatalf("expected accept: %v", err)
	}
}
