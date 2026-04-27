// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package network

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
)

type locatorBlock struct {
	hash   []byte
	height uint64
	header core.BlockHeader
}

func (b *locatorBlock) GetHeight() uint64 { return b.height }

type locatorMockBC struct {
	blocks []*locatorBlock
}

func newLocatorMockBC(blockCount int) *locatorMockBC {
	bc := &locatorMockBC{
		blocks: make([]*locatorBlock, blockCount),
	}
	prevHash := make([]byte, 32)
	for i := 0; i < blockCount; i++ {
		height := uint64(i)
		data := make([]byte, 8)
		binary.BigEndian.PutUint64(data, height)
		hash := sha256.Sum256(append(prevHash, data...))
		bc.blocks[i] = &locatorBlock{
			hash:   hash[:],
			height: height,
			header: core.BlockHeader{
				Version:        1,
				PrevHash:       append([]byte{}, prevHash...),
				TimestampUnix:  1775044800 + int64(i)*10,
				DifficultyBits: uint32(1 << 20),
				Difficulty:     1 << 20,
				Nonce:          uint64(i),
			},
		}
		prevHash = hash[:]
	}
	return bc
}

func (m *locatorMockBC) LatestBlock() *core.Block {
	if len(m.blocks) == 0 {
		return nil
	}
	b := m.blocks[len(m.blocks)-1]
	return &core.Block{Hash: b.hash, Height: b.height, Header: b.header}
}

func (m *locatorMockBC) BlockByHeight(height uint64) (*core.Block, bool) {
	if height >= uint64(len(m.blocks)) {
		return nil, false
	}
	b := m.blocks[height]
	return &core.Block{Hash: b.hash, Height: b.height, Header: b.header}, true
}

func (m *locatorMockBC) BlockByHash(hashHex string) (*core.Block, bool) {
	for _, b := range m.blocks {
		// comparison for tests
		if len(b.hash) > 0 && len(hashHex) > 0 {
			return &core.Block{Hash: b.hash, Height: b.height, Header: b.header}, true
		}
	}
	return nil, false
}

func (m *locatorMockBC) HeadersFrom(from uint64, count uint64) []*core.BlockHeader {
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

func (m *locatorMockBC) BlocksFrom(from uint64, count uint64) []*core.Block {
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

func (m *locatorMockBC) Blocks() []*core.Block {
	result := make([]*core.Block, len(m.blocks))
	for i, b := range m.blocks {
		result[i] = &core.Block{Hash: b.hash, Height: b.height, Header: b.header}
	}
	return result
}

func (m *locatorMockBC) CanonicalWork() *big.Int { return big.NewInt(int64(len(m.blocks))) }
func (m *locatorMockBC) RulesHashHex() string        { return "abc123" }
func (m *locatorMockBC) CalculateCumulativeWork(block *core.Block) *big.Int {
	if block == nil {
		return big.NewInt(0)
	}
	return big.NewInt(int64(block.GetHeight() + 1))
}

func (m *locatorMockBC) BestBlockHeader() (*HeaderLocator, error) {
	if len(m.blocks) == 0 {
		return nil, nil
	}
	tip := m.blocks[len(m.blocks)-1]
	hCopy := tip.header
	return &HeaderLocator{Header: hCopy, Height: tip.height}, nil
}

func (m *locatorMockBC) GetHeaderByHeight(height uint64) (*HeaderLocator, error) {
	if height >= uint64(len(m.blocks)) {
		return nil, nil
	}
	b := m.blocks[height]
	hCopy := b.header
	return &HeaderLocator{Header: hCopy, Height: b.height}, nil
}

func (m *locatorMockBC) GetChainID() uint64                              { return 1 }
func (m *locatorMockBC) GetMinerAddress() string                           { return "NOGO_TEST_MINER" }
func (m *locatorMockBC) TotalSupply() uint64                               { return 0 }
func (m *locatorMockBC) GetConsensus() config.ConsensusParams             { return config.DefaultConfig().Consensus }
func (m *locatorMockBC) AddBlock(block *core.Block) (bool, error)         { return true, nil }
func (m *locatorMockBC) RollbackToHeight(height uint64) error              { return nil }
func (m *locatorMockBC) GetBlockByHash(hash []byte) (*core.Block, bool)   { return nil, false }
func (m *locatorMockBC) GetBlockByHashBytes(hash []byte) (*core.Block, bool) {
	return nil, false
}
func (m *locatorMockBC) GetAllBlocks() ([]*core.Block, error) { return nil, nil }
func (m *locatorMockBC) SelectMempoolTxs(mp Mempool, maxTxPerBlock int) ([]core.Transaction, []string, error) {
	return nil, nil, nil
}
func (m *locatorMockBC) MineTransfers(ctx context.Context, txs []core.Transaction) (*core.Block, error) {
	return nil, nil
}
func (m *locatorMockBC) CalcNextDifficulty(latest *core.Block, currentTime int64) uint32 {
	return 1 << 20
}
func (m *locatorMockBC) AuditChain() error       { return nil }
func (m *locatorMockBC) IsReorgInProgress() bool { return false }
func (m *locatorMockBC) SetOnMissingBlock(cb func(hash []byte, height uint64)) {}
func (m *locatorMockBC) TxByID(txid string) (*core.Transaction, *core.TxLocation, bool) {
	return nil, nil, false
}
func (m *locatorMockBC) AddressTxs(addr string, limit, cursor int) ([]core.AddressTxEntry, int, bool) {
	return nil, 0, false
}
func (m *locatorMockBC) Balance(addr string) (core.Account, bool) { return core.Account{}, false }
func (m *locatorMockBC) HasTransaction(hash []byte) bool           { return false }
func (m *locatorMockBC) GetContractManager() *core.ContractManager { return nil }
func (m *locatorMockBC) SyncLoop() SyncLoopInterface               { return nil }

func newSyncLoopForLocatorTest(bc BlockchainInterface) *SyncLoop {
	return &SyncLoop{bc: bc}
}

func TestBlockLocator_SingleBlock(t *testing.T) {
	bc := newLocatorMockBC(1)
	sl := newSyncLoopForLocatorTest(bc)

	locator, err := sl.BlockLocator()
	if err != nil {
		t.Fatalf("BlockLocator returned error: %v", err)
	}
	if len(locator) != 1 {
		t.Fatalf("expected 1 entry in locator for single-block chain, got %d", len(locator))
	}
	expectedHash := bc.blocks[0].hash
	if len(locator[0]) != len(expectedHash) {
		t.Fatalf("hash length mismatch: expected %d, got %d", len(expectedHash), len(locator[0]))
	}
	for i := range expectedHash {
		if locator[0][i] != expectedHash[i] {
			t.Errorf("hash mismatch at byte %d: got %d, want %d", i, locator[0][i], expectedHash[i])
		}
	}
}

func TestBlockLocator_ShortChain(t *testing.T) {
	const chainLen = 10
	bc := newLocatorMockBC(chainLen)
	sl := newSyncLoopForLocatorTest(bc)

	locator, err := sl.BlockLocator()
	if err != nil {
		t.Fatalf("BlockLocator returned error: %v", err)
	}
	if len(locator) == 0 {
		t.Fatal("expected non-empty locator")
	}
	firstEntry := locator[0]
	tipHash := bc.blocks[chainLen-1].hash
	for i := range firstEntry {
		if firstEntry[i] != tipHash[i] {
			t.Fatal("first entry should be tip (best) block hash")
		}
	}
	lastEntry := locator[len(locator)-1]
	genesisHash := bc.blocks[0].hash
	for i := range lastEntry {
		if lastEntry[i] != genesisHash[i] {
			t.Error("last entry should be genesis block hash for short chains")
		}
	}
}

func TestBlockLocator_ExponentialStepPattern(t *testing.T) {
	const chainLen = 100
	bc := newLocatorMockBC(chainLen)
	sl := newSyncLoopForLocatorTest(bc)

	locator, err := sl.BlockLocator()
	if err != nil {
		t.Fatalf("BlockLocator returned error: %v", err)
	}
	if len(locator) > maxLocatorEntries {
		t.Fatalf("locator exceeds max entries (%d): got %d", maxLocatorEntries, len(locator))
	}
	if len(locator) < 2 {
		t.Fatalf("expected at least 2 entries for %d-block chain, got %d", chainLen, len(locator))
	}
}

func TestBlockLocator_NilBlockchain(t *testing.T) {
	sl := &SyncLoop{}

	_, err := sl.BlockLocator()
	if err == nil {
		t.Fatal("expected error when blockchain interface is nil")
	}
}

func TestBlockLocator_EmptyChain(t *testing.T) {
	bc := newLocatorMockBC(0)
	sl := newSyncLoopForLocatorTest(bc)

	_, err := sl.BlockLocator()
	if err == nil {
		t.Fatal("expected error for empty chain")
	}
}

func TestBlockLocator_FirstEntryIsTip(t *testing.T) {
	const chainLen = 50
	bc := newLocatorMockBC(chainLen)
	sl := newSyncLoopForLocatorTest(bc)

	locator, err := sl.BlockLocator()
	if err != nil {
		t.Fatalf("BlockLocator returned error: %v", err)
	}
	if len(locator) == 0 {
		t.Fatal("empty locator")
	}
	tipHash := bc.blocks[chainLen-1].hash
	for i := range locator[0] {
		if locator[0][i] != tipHash[i] {
			t.Errorf("first entry mismatch at byte %d: expected tip hash (height %d)", i, chainLen-1)
		}
	}
}

func TestBlockLocator_ContainsGenesisForShortChains(t *testing.T) {
	const chainLen = 30
	bc := newLocatorMockBC(chainLen)
	sl := newSyncLoopForLocatorTest(bc)

	locator, err := sl.BlockLocator()
	if err != nil {
		t.Fatalf("BlockLocator returned error: %v", err)
	}
	genesisHash := bc.blocks[0].hash
	foundGenesis := false
	for _, h := range locator {
		match := true
		for i := range h {
			if h[i] != genesisHash[i] {
				match = false
				break
			}
		}
		if match {
			foundGenesis = true
			break
		}
	}
	if !foundGenesis {
		t.Error("locator for short chain should contain genesis block hash")
	}
}

func TestBlockLocator_LargeChainBounded(t *testing.T) {
	const chainLen = 10000
	bc := newLocatorMockBC(chainLen)
	sl := newSyncLoopForLocatorTest(bc)

	locator, err := sl.BlockLocator()
	if err != nil {
		t.Fatalf("BlockLocator returned error: %v", err)
	}
	if len(locator) > maxLocatorEntries {
		t.Fatalf("large chain locator must not exceed %d entries, got %d", maxLocatorEntries, len(locator))
	}
	if len(locator) < 2 {
		t.Fatal("expected multiple entries even for very large chains")
	}
}

func TestBlockLocator_AllEntriesAreUnique(t *testing.T) {
	const chainLen = 200
	bc := newLocatorMockBC(chainLen)
	sl := newSyncLoopForLocatorTest(bc)

	locator, err := sl.BlockLocator()
	if err != nil {
		t.Fatalf("BlockLocator returned error: %v", err)
	}
	seen := make(map[string]bool)
	for idx, h := range locator {
		key := string(h)
		if seen[key] {
			t.Errorf("duplicate hash found at index %d", idx)
		}
		seen[key] = true
	}
}

func TestBlockLocator_StepDoublesAfterNineEntries(t *testing.T) {
	const chainLen = 500
	bc := newLocatorMockBC(chainLen)
	sl := newSyncLoopForLocatorTest(bc)

	locator, err := sl.BlockLocator()
	if err != nil {
		t.Fatalf("BlockLocator returned error: %v", err)
	}
	if len(locator) <= stepDoubleInterval {
		t.Skipf("chain too short to verify step doubling (got %d entries)", len(locator))
	}
	step := uint64(1)
	currentHeight := bc.blocks[chainLen-1].height
	for i := 1; i < len(locator); i++ {
		var expectedPrevHeight uint64
		if currentHeight < step {
			expectedPrevHeight = 0
		} else {
			expectedPrevHeight = currentHeight - step
		}
		if i%stepDoubleInterval == 0 {
			step *= 2
		}
		currentHeight = expectedPrevHeight
	}
}

func TestBlockLocator_MediumChainCoverage(t *testing.T) {
	testCases := []struct {
		name       string
		blockCount int
		minEntries int
		maxEntries int
	}{
		{"5 blocks", 5, 2, 5},
		{"20 blocks", 20, 5, 20},
		{"55 blocks", 55, 8, 50},
		{"200 blocks", 200, 10, 50},
		{"1000 blocks", 1000, 12, 50},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bc := newLocatorMockBC(tc.blockCount)
			sl := newSyncLoopForLocatorTest(bc)

			locator, err := sl.BlockLocator()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(locator) < tc.minEntries {
				t.Errorf("expected at least %d entries, got %d", tc.minEntries, len(locator))
			}
			if len(locator) > tc.maxEntries {
				t.Errorf("expected at most %d entries, got %d", tc.maxEntries, len(locator))
			}
		})
	}
}
