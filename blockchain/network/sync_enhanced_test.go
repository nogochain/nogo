package network

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
)

type fastSyncMockBlock struct {
	hash   []byte
	height uint64
	header core.BlockHeader
}

type fastSyncMockBC struct {
	blocks     []*fastSyncMockBlock
	added      []*core.Block
	rejectNext bool
}

func newFastSyncMockBC(blockCount int) *fastSyncMockBC {
	bc := &fastSyncMockBC{
		blocks: make([]*fastSyncMockBlock, blockCount),
	}
	prevHash := make([]byte, 32)
	for i := 0; i < blockCount; i++ {
		hdr := core.BlockHeader{
			Version:        1,
			PrevHash:       append([]byte{}, prevHash...),
			TimestampUnix:  1775044800 + int64(i)*10,
			DifficultyBits: uint32(1 << 20),
			Difficulty:     1 << 20,
			Nonce:          uint64(i),
			Height:         uint64(i),
			MinerAddress:   "test_miner",
		}
		hash, hashErr := computeHeaderHash(&hdr, uint64(i), "test_miner")
		if hashErr != nil {
			panic(fmt.Sprintf("computeHeaderHash failed at block %d: %v", i, hashErr))
		}
		bc.blocks[i] = &fastSyncMockBlock{
			hash:   hash,
			height: uint64(i),
			header: hdr,
		}
		prevHash = hash
	}
	return bc
}

func (m *fastSyncMockBC) LatestBlock() *core.Block {
	if len(m.blocks) == 0 {
		return nil
	}
	b := m.blocks[len(m.blocks)-1]
	return &core.Block{Hash: b.hash, Height: b.height, Header: b.header}
}

func (m *fastSyncMockBC) BlockByHeight(height uint64) (*core.Block, bool) {
	if height >= uint64(len(m.blocks)) {
		return nil, false
	}
	b := m.blocks[height]
	return &core.Block{Hash: b.hash, Height: b.height, Header: b.header}, true
}

func (m *fastSyncMockBC) BlockByHash(hashHex string) (*core.Block, bool) {
	for _, b := range m.blocks {
		if hex.EncodeToString(b.hash) == hashHex {
			return &core.Block{Hash: b.hash, Height: b.height, Header: b.header}, true
		}
	}
	return nil, false
}

func (m *fastSyncMockBC) HeadersFrom(from uint64, count uint64) []*core.BlockHeader {
	result := make([]*core.BlockHeader, 0)
	end := from + count
	if end > uint64(len(m.blocks)) {
		end = uint64(len(m.blocks))
	}
	for i := from; i < end; i++ {
		h := m.blocks[i].header
		result = append(result, &h)
	}
	return result
}

func (m *fastSyncMockBC) BlocksFrom(from uint64, count uint64) []*core.Block {
	result := make([]*core.Block, 0)
	end := from + count
	if end > uint64(len(m.blocks)) {
		end = uint64(len(m.blocks))
	}
	for i := from; i < end; i++ {
		b := m.blocks[i]
		result = append(result, &core.Block{Hash: b.hash, Height: b.height, Header: b.header})
	}
	return result
}

func (m *fastSyncMockBC) Blocks() []*core.Block {
	result := make([]*core.Block, len(m.blocks))
	for i, b := range m.blocks {
		result[i] = &core.Block{Hash: b.hash, Height: b.height, Header: b.header}
	}
	return result
}

func (m *fastSyncMockBC) CanonicalWork() *big.Int { return big.NewInt(int64(len(m.blocks))) }
func (m *fastSyncMockBC) RulesHashHex() string    { return "abc123" }

func (m *fastSyncMockBC) BestBlockHeader() (*HeaderLocator, error) {
	if len(m.blocks) == 0 {
		return nil, nil
	}
	tip := m.blocks[len(m.blocks)-1]
	hCopy := tip.header
	return &HeaderLocator{Header: &hCopy, Height: tip.height}, nil
}

func (m *fastSyncMockBC) GetHeaderByHeight(height uint64) (*HeaderLocator, error) {
	if height >= uint64(len(m.blocks)) {
		return nil, nil
	}
	b := m.blocks[height]
	hCopy := b.header
	return &HeaderLocator{Header: &hCopy, Height: b.height}, nil
}

func (m *fastSyncMockBC) GetChainID() uint64      { return 1 }
func (m *fastSyncMockBC) GetMinerAddress() string { return "NOGO_TEST_MINER" }
func (m *fastSyncMockBC) TotalSupply() uint64     { return 0 }
func (m *fastSyncMockBC) GetConsensus() config.ConsensusParams {
	return config.DefaultConfig().Consensus
}
func (m *fastSyncMockBC) RollbackToHeight(height uint64) error                { return nil }
func (m *fastSyncMockBC) GetBlockByHash(hash []byte) (*core.Block, bool)      { return nil, false }
func (m *fastSyncMockBC) GetBlockByHashBytes(hash []byte) (*core.Block, bool) { return nil, false }
func (m *fastSyncMockBC) GetAllBlocks() ([]*core.Block, error)                { return nil, nil }
func (m *fastSyncMockBC) SelectMempoolTxs(mp Mempool, maxTxPerBlock int) ([]core.Transaction, []string, error) {
	return nil, nil, nil
}
func (m *fastSyncMockBC) MineTransfers(ctx context.Context, txs []core.Transaction) (*core.Block, error) {
	return nil, nil
}
func (m *fastSyncMockBC) CalcNextDifficulty(latest *core.Block, currentTime int64) uint32 {
	return 1 << 20
}
func (m *fastSyncMockBC) AuditChain() error       { return nil }
func (m *fastSyncMockBC) IsReorgInProgress() bool { return false }
func (m *fastSyncMockBC) SetOnMissingBlock(cb func(hash []byte, height uint64)) {}
func (m *fastSyncMockBC) TxByID(txid string) (*core.Transaction, *core.TxLocation, bool) {
	return nil, nil, false
}
func (m *fastSyncMockBC) AddressTxs(addr string, limit, cursor int) ([]core.AddressTxEntry, int, bool) {
	return nil, 0, false
}
func (m *fastSyncMockBC) Balance(addr string) (core.Account, bool)  { return core.Account{}, false }
func (m *fastSyncMockBC) HasTransaction(hash []byte) bool           { return false }
func (m *fastSyncMockBC) GetContractManager() *core.ContractManager { return nil }
func (m *fastSyncMockBC) SyncLoop() SyncLoopInterface               { return nil }

func (m *fastSyncMockBC) AddBlock(block *core.Block) (bool, error) {
	if m.rejectNext {
		m.rejectNext = false
		return false, fmt.Errorf("rejected for test")
	}
	m.added = append(m.added, block)
	m.blocks = append(m.blocks, &fastSyncMockBlock{
		hash:   block.Hash,
		height: block.Height,
		header: block.Header,
	})
	return true, nil
}

type fastSyncMockPM struct {
	peers       []string
	chainHeight uint64
	blocks      map[uint64]*core.Block
	headers     map[uint64]core.BlockHeader
	chainInfo   *ChainInfo
	failFetch   bool
}

func newFastSyncMockPM(peerCount int, chainHeight uint64, bc *fastSyncMockBC) *fastSyncMockPM {
	pm := &fastSyncMockPM{
		chainHeight: chainHeight,
		blocks:      make(map[uint64]*core.Block),
		headers:     make(map[uint64]core.BlockHeader),
	}

	for i := 0; i < peerCount; i++ {
		pm.peers = append(pm.peers, fmt.Sprintf("peer-%d", i))
	}

	tipHash := ""
	if len(bc.blocks) > 0 {
		tipHash = hex.EncodeToString(bc.blocks[len(bc.blocks)-1].hash)
	}
	pm.chainInfo = &ChainInfo{
		ChainID:    1,
		Height:     chainHeight,
		LatestHash: tipHash,
		Work:       big.NewInt(int64(chainHeight)),
	}

	return pm
}

func (m *fastSyncMockPM) Peers() []string { return m.peers }
func (m *fastSyncMockPM) AddPeer(addr string) {
	m.peers = append(m.peers, addr)
}
func (m *fastSyncMockPM) GetActivePeers() []string {
	if m.failFetch {
		return nil
	}
	return m.peers
}

func (m *fastSyncMockPM) FetchChainInfo(ctx context.Context, peer string) (*ChainInfo, error) {
	if m.failFetch {
		return nil, fmt.Errorf("mock fetch failed")
	}
	infoCopy := *m.chainInfo
	return &infoCopy, nil
}

func (m *fastSyncMockPM) FetchHeadersFrom(ctx context.Context, peer string, fromHeight uint64, count int) ([]core.BlockHeader, error) {
	if m.failFetch {
		return nil, fmt.Errorf("mock fetch failed")
	}
	result := make([]core.BlockHeader, 0)
	for i := uint64(0); i < uint64(count); i++ {
		h := fromHeight + i
		if hdr, ok := m.headers[h]; ok {
			result = append(result, hdr)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no headers at height %d", fromHeight)
	}
	return result, nil
}

func (m *fastSyncMockPM) FetchBlockByHash(ctx context.Context, peer, hashHex string) (*core.Block, error) {
	if m.failFetch {
		return nil, fmt.Errorf("mock fetch failed")
	}
	for _, blk := range m.blocks {
		if hex.EncodeToString(blk.Hash) == hashHex {
			return blk, nil
		}
	}
	return nil, fmt.Errorf("block not found: %s", hashHex)
}

func (m *fastSyncMockPM) FetchBlockByHeight(ctx context.Context, peer string, height uint64) (*core.Block, error) {
	if m.failFetch {
		return nil, fmt.Errorf("mock fetch failed")
	}
	if blk, ok := m.blocks[height]; ok {
		return blk, nil
	}
	return nil, fmt.Errorf("block not found at height %d", height)
}

func (m *fastSyncMockPM) FetchAnyBlockByHash(ctx context.Context, hashHex string) (*core.Block, string, error) {
	return nil, "", fmt.Errorf("fetch block by hash not used in this test scenario")
}

func (m *fastSyncMockPM) FetchBlocksByHeightRange(ctx context.Context, peer string, startHeight, count uint64) ([]*core.Block, error) {
	if m.failFetch {
		return nil, fmt.Errorf("mock fetch failed")
	}
	result := make([]*core.Block, 0)
	for i := uint64(0); i < count; i++ {
		h := startHeight + i
		if blk, ok := m.blocks[h]; ok {
			result = append(result, blk)
		}
	}
	return result, nil
}

func (m *fastSyncMockPM) BroadcastTransaction(ctx context.Context, tx core.Transaction, hops int) {}

func (m *fastSyncMockPM) BroadcastBlock(ctx context.Context, block *core.Block) error { return nil }

func (m *fastSyncMockPM) BroadcastNewStatus(ctx context.Context, height uint64, work *big.Int, latestHash string) {
}

func (m *fastSyncMockPM) EnsureAncestors(ctx context.Context, bc BlockchainInterface, missingHashHex string) error {
	return nil
}

func buildPeerChain(localBlockCount, peerBlockCount int) (*fastSyncMockBC, *fastSyncMockPM) {
	localBC := newFastSyncMockBC(localBlockCount)
	pm := newFastSyncMockPM(3, uint64(peerBlockCount), localBC)

	prevHash := make([]byte, 32)
	if localBlockCount > 0 {
		prevHash = make([]byte, len(localBC.blocks[localBlockCount-1].hash))
		copy(prevHash, localBC.blocks[localBlockCount-1].hash)
	}

	for i := localBlockCount; i < peerBlockCount; i++ {
		hdr := core.BlockHeader{
			Version:        1,
			PrevHash:       append([]byte{}, prevHash...),
			TimestampUnix:  1775044800 + int64(i)*10,
			DifficultyBits: uint32(1 << 20),
			Difficulty:     1 << 20,
			Nonce:          uint64(i),
			Height:         uint64(i),
			MinerAddress:   "test_miner",
		}
		hash, hashErr := computeHeaderHash(&hdr, uint64(i), "test_miner")
		if hashErr != nil {
			panic(fmt.Sprintf("computeHeaderHash failed at block %d: %v", i, hashErr))
		}
		blk := &core.Block{
			Hash:   hash,
			Height: uint64(i),
			Header: hdr,
		}
		pm.blocks[uint64(i)] = blk
		pm.headers[uint64(i)] = hdr
		prevHash = hash
	}

	return localBC, pm
}

func TestCheckFastSyncEligible_LocalBehindCheckpoint(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)
	validHash := make([]byte, 32)
	validHash[0] = 0x01

	if !engine.CheckFastSyncEligible(50, 100, validHash) {
		t.Error("expected eligible when localHeight=50 < checkpointHeight=100")
	}
	if !engine.CheckFastSyncEligible(0, 1, validHash) {
		t.Error("expected eligible when localHeight=0 < checkpointHeight=1")
	}
	if !engine.CheckFastSyncEligible(99, 100, validHash) {
		t.Error("expected eligible when localHeight=99 < checkpointHeight=100")
	}
}

func TestCheckFastSyncEligible_LocalAtCheckpoint(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)
	validHash := make([]byte, 32)
	validHash[0] = 0x01

	if engine.CheckFastSyncEligible(100, 100, validHash) {
		t.Error("expected NOT eligible when localHeight=100 == checkpointHeight=100")
	}
}

func TestCheckFastSyncEligible_LocalAheadOfCheckpoint(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)
	validHash := make([]byte, 32)
	validHash[0] = 0x01

	if engine.CheckFastSyncEligible(200, 100, validHash) {
		t.Error("expected NOT eligible when localHeight=200 > checkpointHeight=100")
	}
	if engine.CheckFastSyncEligible(101, 100, validHash) {
		t.Error("expected NOT eligible when localHeight=101 > checkpointHeight=100")
	}
}

func TestCheckFastSyncEligible_ZeroHeights(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)
	validHash := make([]byte, 32)
	validHash[0] = 0x01

	if engine.CheckFastSyncEligible(0, 0, validHash) {
		t.Error("expected NOT eligible when both heights are 0")
	}
}

func TestCheckFastSyncEligible_EmptyCheckpointHash(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)

	if engine.CheckFastSyncEligible(50, 100, nil) {
		t.Error("expected NOT eligible when checkpointHash is nil")
	}
	if engine.CheckFastSyncEligible(50, 100, []byte{}) {
		t.Error("expected NOT eligible when checkpointHash is empty")
	}
}

func TestCheckFastSyncEligible_LargeHeights(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)
	validHash := make([]byte, 32)
	validHash[0] = 0x01

	if !engine.CheckFastSyncEligible(1_000_000, 2_000_000, validHash) {
		t.Error("expected eligible for large height difference")
	}
	if engine.CheckFastSyncEligible(2_000_000, 1_000_000, validHash) {
		t.Error("expected NOT eligible when local ahead")
	}
}

func TestVerifyHeaderChain_ValidChain(t *testing.T) {
	bc := newFastSyncMockBC(10)
	engine := NewFastSyncEngine(bc, 100)

	tip := bc.LatestBlock()

	headers := make([]*core.BlockHeader, 5)
	prevHash := make([]byte, len(tip.Hash))
	copy(prevHash, tip.Hash)
	for i := 0; i < 5; i++ {
		hdr := core.BlockHeader{
			Version:        1,
			PrevHash:       append([]byte{}, prevHash...),
			TimestampUnix:  1775044800 + int64(10+i)*10,
			DifficultyBits: uint32(1 << 20),
			Difficulty:     1 << 20,
			Nonce:          uint64(10 + i),
			Height:         uint64(10 + i),
			MinerAddress:   "test_miner",
		}
		hash, hashErr := computeHeaderHash(&hdr, uint64(10+i), "test_miner")
		if hashErr != nil {
			t.Fatalf("computeHeaderHash failed at index %d: %v", i, hashErr)
		}
		headers[i] = &hdr
		prevHash = hash
	}

	if err := engine.verifyHeaderChain(headers, tip); err != nil {
		t.Errorf("expected valid header chain, got error: %v", err)
	}
}

func TestVerifyHeaderChain_EmptyHeaders(t *testing.T) {
	bc := newFastSyncMockBC(10)
	engine := NewFastSyncEngine(bc, 100)
	tip := bc.LatestBlock()

	if err := engine.verifyHeaderChain(nil, tip); err == nil {
		t.Error("expected error for empty header list")
	}
}

func TestVerifyHeaderChain_FirstHeaderPrevHashMismatch(t *testing.T) {
	bc := newFastSyncMockBC(10)
	engine := NewFastSyncEngine(bc, 100)
	tip := bc.LatestBlock()

	headers := make([]*core.BlockHeader, 3)
	for i := 0; i < 3; i++ {
		headers[i] = &bc.blocks[i+5].header
	}
	headers[0] = &core.BlockHeader{
		Version:        1,
		PrevHash:       make([]byte, 32),
		TimestampUnix:  1775044900,
		DifficultyBits: 1 << 20,
		Difficulty:     1 << 20,
		Nonce:          999,
	}

	if err := engine.verifyHeaderChain(headers, tip); err == nil {
		t.Error("expected error for first header prevHash mismatch")
	}
}

func TestVerifyHeaderChain_NonMonotonicTimestamp(t *testing.T) {
	bc := newFastSyncMockBC(10)
	engine := NewFastSyncEngine(bc, 100)
	tip := bc.LatestBlock()

	headers := make([]*core.BlockHeader, 3)
	for i := 0; i < 3; i++ {
		headers[i] = &bc.blocks[i+5].header
	}

	originalTs := headers[1].TimestampUnix
	headers[1] = &core.BlockHeader{
		Version:        headers[1].Version,
		PrevHash:       headers[1].PrevHash,
		TimestampUnix:  headers[2].TimestampUnix + 100,
		DifficultyBits: headers[1].DifficultyBits,
		Difficulty:     headers[1].Difficulty,
		Nonce:          headers[1].Nonce,
	}

	_ = originalTs

	if err := engine.verifyHeaderChain(headers, tip); err == nil {
		t.Error("expected error for non-monotonic timestamp")
	}
}

func TestVerifyHeaderChain_ZeroDifficulty(t *testing.T) {
	bc := newFastSyncMockBC(10)
	engine := NewFastSyncEngine(bc, 100)
	tip := bc.LatestBlock()

	headers := make([]*core.BlockHeader, 3)
	for i := 0; i < 3; i++ {
		headers[i] = &bc.blocks[i+5].header
	}

	headers[2] = &core.BlockHeader{
		Version:        headers[2].Version,
		PrevHash:       headers[2].PrevHash,
		TimestampUnix:  headers[2].TimestampUnix,
		DifficultyBits: 0,
		Difficulty:     0,
		Nonce:          headers[2].Nonce,
	}

	if err := engine.verifyHeaderChain(headers, tip); err == nil {
		t.Error("expected error for zero difficulty")
	}
}

func TestVerifyHeaderChain_EmptyPrevHash(t *testing.T) {
	bc := newFastSyncMockBC(10)
	engine := NewFastSyncEngine(bc, 100)
	tip := bc.LatestBlock()

	headers := make([]*core.BlockHeader, 3)
	for i := 0; i < 3; i++ {
		headers[i] = &bc.blocks[i+5].header
	}

	headers[2] = &core.BlockHeader{
		Version:        headers[2].Version,
		PrevHash:       nil,
		TimestampUnix:  headers[2].TimestampUnix,
		DifficultyBits: headers[2].DifficultyBits,
		Difficulty:     headers[2].Difficulty,
		Nonce:          headers[2].Nonce,
	}

	if err := engine.verifyHeaderChain(headers, tip); err == nil {
		t.Error("expected error for empty prevHash")
	}
}

func TestVerifyHeaderChain_InvalidPrevHashLength(t *testing.T) {
	bc := newFastSyncMockBC(10)
	engine := NewFastSyncEngine(bc, 100)
	tip := bc.LatestBlock()

	headers := make([]*core.BlockHeader, 3)
	for i := 0; i < 3; i++ {
		headers[i] = &bc.blocks[i+5].header
	}

	headers[2] = &core.BlockHeader{
		Version:        headers[2].Version,
		PrevHash:       []byte{0x01, 0x02},
		TimestampUnix:  headers[2].TimestampUnix,
		DifficultyBits: headers[2].DifficultyBits,
		Difficulty:     headers[2].Difficulty,
		Nonce:          headers[2].Nonce,
	}

	if err := engine.verifyHeaderChain(headers, tip); err == nil {
		t.Error("expected error for invalid prevHash length")
	}
}

func TestVerifyBlockMatchesHeader_ValidBlock(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)

	hdr := core.BlockHeader{
		Version:        1,
		PrevHash:       make([]byte, 32),
		TimestampUnix:  1775044800,
		DifficultyBits: 1 << 20,
		Difficulty:     1 << 20,
		Nonce:          42,
	}
	blk := &core.Block{
		Hash:   make([]byte, 32),
		Height: 10,
		Header: hdr,
	}

	if err := engine.verifyBlockMatchesHeader(blk, &hdr); err != nil {
		t.Errorf("expected valid block, got error: %v", err)
	}
}

func TestVerifyBlockMatchesHeader_NilBlock(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)
	hdr := core.BlockHeader{Version: 1}

	if err := engine.verifyBlockMatchesHeader(nil, &hdr); err == nil {
		t.Error("expected error for nil block")
	}
}

func TestVerifyBlockMatchesHeader_NilHeader(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)
	blk := &core.Block{Hash: make([]byte, 32), Height: 10}

	if err := engine.verifyBlockMatchesHeader(blk, nil); err == nil {
		t.Error("expected error for nil header")
	}
}

func TestVerifyBlockMatchesHeader_VersionMismatch(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)

	hdr := core.BlockHeader{Version: 1, PrevHash: make([]byte, 32), TimestampUnix: 100, DifficultyBits: 100, Difficulty: 100, Nonce: 1}
	blk := &core.Block{Hash: make([]byte, 32), Height: 10, Header: core.BlockHeader{Version: 2, PrevHash: make([]byte, 32), TimestampUnix: 100, DifficultyBits: 100, Difficulty: 100, Nonce: 1}}

	if err := engine.verifyBlockMatchesHeader(blk, &hdr); err == nil {
		t.Error("expected error for version mismatch")
	}
}

func TestVerifyBlockMatchesHeader_PrevHashMismatch(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)

	hdr := core.BlockHeader{Version: 1, PrevHash: make([]byte, 32), TimestampUnix: 100, DifficultyBits: 100, Difficulty: 100, Nonce: 1}
	differentPrev := make([]byte, 32)
	differentPrev[0] = 0xFF
	blk := &core.Block{Hash: make([]byte, 32), Height: 10, Header: core.BlockHeader{Version: 1, PrevHash: differentPrev, TimestampUnix: 100, DifficultyBits: 100, Difficulty: 100, Nonce: 1}}

	if err := engine.verifyBlockMatchesHeader(blk, &hdr); err == nil {
		t.Error("expected error for prevHash mismatch")
	}
}

func TestVerifyBlockMatchesHeader_TimestampMismatch(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)

	hdr := core.BlockHeader{Version: 1, PrevHash: make([]byte, 32), TimestampUnix: 100, DifficultyBits: 100, Difficulty: 100, Nonce: 1}
	blk := &core.Block{Hash: make([]byte, 32), Height: 10, Header: core.BlockHeader{Version: 1, PrevHash: make([]byte, 32), TimestampUnix: 200, DifficultyBits: 100, Difficulty: 100, Nonce: 1}}

	if err := engine.verifyBlockMatchesHeader(blk, &hdr); err == nil {
		t.Error("expected error for timestamp mismatch")
	}
}

func TestVerifyBlockMatchesHeader_DifficultyMismatch(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)

	hdr := core.BlockHeader{Version: 1, PrevHash: make([]byte, 32), TimestampUnix: 100, DifficultyBits: 100, Difficulty: 100, Nonce: 1}
	blk := &core.Block{Hash: make([]byte, 32), Height: 10, Header: core.BlockHeader{Version: 1, PrevHash: make([]byte, 32), TimestampUnix: 100, DifficultyBits: 200, Difficulty: 100, Nonce: 1}}

	if err := engine.verifyBlockMatchesHeader(blk, &hdr); err == nil {
		t.Error("expected error for difficultyBits mismatch")
	}
}

func TestVerifyBlockMatchesHeader_NonceMismatch(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)

	hdr := core.BlockHeader{Version: 1, PrevHash: make([]byte, 32), TimestampUnix: 100, DifficultyBits: 100, Difficulty: 100, Nonce: 1}
	blk := &core.Block{Hash: make([]byte, 32), Height: 10, Header: core.BlockHeader{Version: 1, PrevHash: make([]byte, 32), TimestampUnix: 100, DifficultyBits: 100, Difficulty: 100, Nonce: 2}}

	if err := engine.verifyBlockMatchesHeader(blk, &hdr); err == nil {
		t.Error("expected error for nonce mismatch")
	}
}

func TestNewFastSyncEngine_DefaultBatchSize(t *testing.T) {
	engine := NewFastSyncEngine(nil, 0)
	if engine.batchSize != defaultEnhancedBatchSize {
		t.Errorf("expected default batch size %d, got %d", defaultEnhancedBatchSize, engine.batchSize)
	}
}

func TestNewFastSyncEngine_NegativeBatchSize(t *testing.T) {
	engine := NewFastSyncEngine(nil, -1)
	if engine.batchSize != defaultEnhancedBatchSize {
		t.Errorf("expected default batch size %d for negative input, got %d", defaultEnhancedBatchSize, engine.batchSize)
	}
}

func TestNewFastSyncEngine_CustomBatchSize(t *testing.T) {
	engine := NewFastSyncEngine(nil, 250)
	if engine.batchSize != 250 {
		t.Errorf("expected batch size 250, got %d", engine.batchSize)
	}
}

func TestHeaderDownload_NoPeerAPI(t *testing.T) {
	bc := newFastSyncMockBC(10)
	engine := NewFastSyncEngine(bc, 100)

	_, err := engine.HeaderDownload(nil)
	if err == nil {
		t.Error("expected error when peer API not configured")
	}
}

func TestHeaderDownload_NoActivePeers(t *testing.T) {
	bc := newFastSyncMockBC(10)
	engine := NewFastSyncEngine(bc, 100)
	engine.SetPeerAPI(&fastSyncMockPM{peers: nil})

	_, err := engine.HeaderDownload(nil)
	if err == nil {
		t.Error("expected error when no active peers")
	}
}

func TestHeaderDownload_NilChain(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)
	engine.SetPeerAPI(&fastSyncMockPM{peers: []string{"peer-0"}})

	_, err := engine.HeaderDownload(nil)
	if err == nil {
		t.Error("expected error when chain is nil")
	}
}

func TestHeaderDownload_EmptyChain(t *testing.T) {
	bc := newFastSyncMockBC(0)
	engine := NewFastSyncEngine(bc, 100)
	engine.SetPeerAPI(&fastSyncMockPM{peers: []string{"peer-0"}})

	_, err := engine.HeaderDownload(nil)
	if err == nil {
		t.Error("expected error when chain is empty")
	}
}

func TestHeaderDownload_Success(t *testing.T) {
	localBC, pm := buildPeerChain(10, 20)
	engine := NewFastSyncEngine(localBC, 100)
	engine.SetPeerAPI(pm)

	engine.mu.Lock()
	engine.targetHeight = 20
	engine.mu.Unlock()

	headers, err := engine.HeaderDownload(nil)
	if err != nil {
		t.Fatalf("expected successful header download, got error: %v", err)
	}
	if len(headers) != 10 {
		t.Errorf("expected 10 headers, got %d", len(headers))
	}
}

func TestHeaderDownload_VerifiesChainContinuity(t *testing.T) {
	localBC, pm := buildPeerChain(10, 15)
	engine := NewFastSyncEngine(localBC, 100)
	engine.SetPeerAPI(pm)

	engine.mu.Lock()
	engine.targetHeight = 15
	engine.mu.Unlock()

	headers, err := engine.HeaderDownload(nil)
	if err != nil {
		t.Fatalf("expected successful header download, got error: %v", err)
	}

	tip := localBC.LatestBlock()
	if !bytes.Equal(headers[0].PrevHash, tip.Hash) {
		t.Error("first header prevHash should match local tip hash")
	}

	for i := 1; i < len(headers); i++ {
		if headers[i].TimestampUnix <= headers[i-1].TimestampUnix {
			t.Errorf("header %d timestamp %d should be > header %d timestamp %d",
				i, headers[i].TimestampUnix, i-1, headers[i-1].TimestampUnix)
		}
	}
}

func TestBlockDownloadAndValidate_NoPeerAPI(t *testing.T) {
	bc := newFastSyncMockBC(10)
	engine := NewFastSyncEngine(bc, 100)

	hdr := core.BlockHeader{Version: 1, PrevHash: make([]byte, 32), TimestampUnix: 100, DifficultyBits: 100, Difficulty: 100, Nonce: 1}
	err := engine.BlockDownloadAndValidate([]*core.BlockHeader{&hdr})
	if err == nil {
		t.Error("expected error when peer API not configured")
	}
}

func TestBlockDownloadAndValidate_EmptyHeaders(t *testing.T) {
	bc := newFastSyncMockBC(10)
	pm := newFastSyncMockPM(1, 20, bc)
	engine := NewFastSyncEngine(bc, 100)
	engine.SetPeerAPI(pm)

	err := engine.BlockDownloadAndValidate(nil)
	if err == nil {
		t.Error("expected error for empty headers")
	}
}

func TestSyncToCheckpoint_NotEligible(t *testing.T) {
	bc := newFastSyncMockBC(100)
	engine := NewFastSyncEngine(bc, 100)

	err := engine.SyncToCheckpoint(50, nil)
	if err == nil {
		t.Error("expected error when not eligible for fast sync")
	}
}

func TestSyncToCheckpoint_EqualHeight(t *testing.T) {
	bc := newFastSyncMockBC(100)
	engine := NewFastSyncEngine(bc, 100)

	err := engine.SyncToCheckpoint(100, nil)
	if err == nil {
		t.Error("expected error when local height equals checkpoint height")
	}
}

func TestSyncToCheckpoint_NilChain(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)

	err := engine.SyncToCheckpoint(200, nil)
	if err == nil {
		t.Error("expected error when chain is nil")
	}
}

func TestSyncToCheckpoint_EmptyChain(t *testing.T) {
	bc := newFastSyncMockBC(0)
	engine := NewFastSyncEngine(bc, 100)

	err := engine.SyncToCheckpoint(100, nil)
	if err == nil {
		t.Error("expected error when chain is empty")
	}
}

func TestSyncToCheckpoint_FullFlow(t *testing.T) {
	localBC, pm := buildPeerChain(10, 21)
	engine := NewFastSyncEngine(localBC, 100)
	engine.SetPeerAPI(pm)

	checkpointHash := make([]byte, 32)
	if blk, ok := pm.blocks[20]; ok {
		checkpointHash = blk.Hash
	}

	err := engine.SyncToCheckpoint(20, checkpointHash)
	if err != nil {
		t.Fatalf("expected successful sync, got error: %v", err)
	}

	newTip := localBC.LatestBlock()
	if newTip.GetHeight() < 20 {
		t.Errorf("expected height >= 20 after sync, got %d", newTip.GetHeight())
	}
}

func TestSyncToCheckpoint_CheckpointHashMismatch(t *testing.T) {
	localBC, pm := buildPeerChain(10, 21)
	engine := NewFastSyncEngine(localBC, 100)
	engine.SetPeerAPI(pm)

	wrongHash := make([]byte, 32)
	wrongHash[0] = 0xFF

	err := engine.SyncToCheckpoint(20, wrongHash)
	if err == nil {
		t.Error("expected error for checkpoint hash mismatch")
	}
}

func TestBuildBlockLocator_ValidChain(t *testing.T) {
	bc := newFastSyncMockBC(50)
	engine := NewFastSyncEngine(bc, 100)

	locator, err := engine.buildBlockLocator()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(locator) == 0 {
		t.Error("expected non-empty locator")
	}

	tipHash := bc.blocks[49].hash
	if !bytes.Equal(locator[0], tipHash) {
		t.Error("first locator entry should be tip hash")
	}
}

func TestBuildBlockLocator_NilChain(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)

	_, err := engine.buildBlockLocator()
	if err == nil {
		t.Error("expected error for nil chain")
	}
}

func TestBuildBlockLocator_EmptyChain(t *testing.T) {
	bc := newFastSyncMockBC(0)
	engine := NewFastSyncEngine(bc, 100)

	_, err := engine.buildBlockLocator()
	if err == nil {
		t.Error("expected error for empty chain")
	}
}

func TestResolveTargetHeight_UsesStoredTarget(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)

	engine.mu.Lock()
	engine.targetHeight = 500
	engine.mu.Unlock()

	result := engine.resolveTargetHeight(100)
	if result != 500 {
		t.Errorf("expected target height 500, got %d", result)
	}
}

func TestResolveTargetHeight_FallsBackToPeerHeight(t *testing.T) {
	bc := newFastSyncMockBC(10)
	pm := newFastSyncMockPM(1, 200, bc)
	engine := NewFastSyncEngine(bc, 100)
	engine.SetPeerAPI(pm)

	result := engine.resolveTargetHeight(100)
	if result != 200 {
		t.Errorf("expected peer height 200, got %d", result)
	}
}

func TestResolveTargetHeight_NoProgress(t *testing.T) {
	bc := newFastSyncMockBC(10)
	engine := NewFastSyncEngine(bc, 100)

	result := engine.resolveTargetHeight(100)
	if result != 100 {
		t.Errorf("expected local height 100 when no target or peers, got %d", result)
	}
}

func TestSetPeerAPI_ConcurrentAccess(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)
	pm := newFastSyncMockPM(1, 100, newFastSyncMockBC(10))

	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			engine.SetPeerAPI(pm)
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			engine.mu.RLock()
			_ = engine.pm
			engine.mu.RUnlock()
		}
		done <- true
	}()

	<-done
	<-done
}

func TestCheckFastSyncEligible_BoundaryValues(t *testing.T) {
	engine := NewFastSyncEngine(nil, 100)
	validHash := make([]byte, 32)
	validHash[0] = 0x01

	testCases := []struct {
		local      uint64
		checkpoint uint64
		cpHash     []byte
		eligible   bool
		desc       string
	}{
		{0, 1, validHash, true, "genesis behind first block"},
		{0, 0, validHash, false, "both at genesis"},
		{1, 0, validHash, false, "local ahead of checkpoint"},
		{999, 1000, validHash, true, "one block behind"},
		{1000, 1000, validHash, false, "equal heights"},
		{1001, 1000, validHash, false, "one block ahead"},
		{0, 10000, validHash, true, "far behind"},
		{50, 100, nil, false, "nil checkpoint hash"},
		{50, 100, []byte{}, false, "empty checkpoint hash"},
	}

	for _, tc := range testCases {
		result := engine.CheckFastSyncEligible(tc.local, tc.checkpoint, tc.cpHash)
		if result != tc.eligible {
			t.Errorf("CheckFastSyncEligible(%d, %d, hash=%v): expected %v, got %v (%s)",
				tc.local, tc.checkpoint, tc.cpHash != nil, tc.eligible, result, tc.desc)
		}
	}
}
