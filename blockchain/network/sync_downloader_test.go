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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package network

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/metrics"
)

var ErrNilBlock = errors.New("block is nil")

type mockValidator struct {
	validateDelay time.Duration
	shouldFail    bool
}

func (m *mockValidator) ValidateBlockFast(block *core.Block) error {
	if m.validateDelay > 0 {
		time.Sleep(m.validateDelay)
	}
	if m.shouldFail {
		return ErrNilBlock
	}
	return nil
}

type mockBlockchain struct {
	height uint64
}

func (m *mockBlockchain) LatestBlock() *core.Block {
	return &core.Block{Height: m.height}
}

func (m *mockBlockchain) BlockByHeight(height uint64) (*core.Block, bool) {
	return nil, false
}

func (m *mockBlockchain) BlockByHash(hashHex string) (*core.Block, bool) {
	return nil, false
}

func (m *mockBlockchain) HeadersFrom(from uint64, count uint64) []*core.BlockHeader {
	return nil
}

func (m *mockBlockchain) BlocksFrom(from uint64, count uint64) []*core.Block {
	return nil
}

func (m *mockBlockchain) Blocks() []*core.Block {
	return nil
}

func (m *mockBlockchain) CanonicalWork() *big.Int {
	return big.NewInt(0)
}

func (m *mockBlockchain) RulesHashHex() string {
	return ""
}

func (m *mockBlockchain) GetChainID() uint64 {
	return 0
}

func (m *mockBlockchain) GetMinerAddress() string {
	return ""
}

func (m *mockBlockchain) TotalSupply() uint64 {
	return 0
}

func (m *mockBlockchain) AddBlock(block *core.Block) (bool, error) {
	return true, nil
}

func (m *mockBlockchain) SelectMempoolTxs(mp Mempool, maxTxPerBlock int) ([]core.Transaction, []string, error) {
	return nil, nil, nil
}

func (m *mockBlockchain) MineTransfers(txs []core.Transaction) (*core.Block, error) {
	return nil, nil
}

func (m *mockBlockchain) AuditChain() error {
	return nil
}

func (m *mockBlockchain) TxByID(txid string) (*core.Transaction, *core.TxLocation, bool) {
	return nil, nil, false
}

func (m *mockBlockchain) AddressTxs(addr string, limit, cursor int) ([]core.AddressTxEntry, int, bool) {
	return nil, 0, false
}

func (m *mockBlockchain) Balance(addr string) (core.Account, bool) {
	return core.Account{}, false
}

func (m *mockBlockchain) SyncLoop() SyncLoopInterface {
	return nil
}

func (m *mockBlockchain) GetContractManager() *core.ContractManager {
	return nil
}

func (m *mockBlockchain) HasTransaction(txHash []byte) bool {
	return false
}

func (m *mockBlockchain) GetBlockByHash(hash []byte) (*core.Block, bool) {
	return nil, false
}

func (m *mockBlockchain) GetBlockByHashBytes(hash []byte) (*core.Block, bool) {
	return nil, false
}

func (m *mockBlockchain) GetAllBlocks() ([]*core.Block, error) {
	return nil, nil
}

type mockMiner struct{}

func (m *mockMiner) InterruptMining()        {}
func (m *mockMiner) ResumeMining()           {}
func (m *mockMiner) IsVerifying() bool       { return false }
func (m *mockMiner) OnBlockAdded()           {}

type mockPeerAPI struct {
	blocks map[string]*core.Block
}

func (m *mockPeerAPI) Peers() []string {
	return nil
}

func (m *mockPeerAPI) AddPeer(addr string) {}

func (m *mockPeerAPI) GetActivePeers() []string {
	return nil
}

func (m *mockPeerAPI) FetchChainInfo(ctx context.Context, peer string) (*ChainInfo, error) {
	return nil, nil
}

func (m *mockPeerAPI) FetchHeadersFrom(ctx context.Context, peer string, fromHeight uint64, count int) ([]core.BlockHeader, error) {
	headers := make([]core.BlockHeader, count)
	for i := 0; i < count; i++ {
		headers[i] = core.BlockHeader{
			PrevHash: make([]byte, 32),
		}
	}
	return headers, nil
}

func (m *mockPeerAPI) FetchBlockByHash(ctx context.Context, peer string, hashHex string) (*core.Block, error) {
	if block, exists := m.blocks[hashHex]; exists {
		return block, nil
	}
	return &core.Block{Height: 1}, nil
}

func (m *mockPeerAPI) FetchBlockByHeight(ctx context.Context, peer string, height uint64) (*core.Block, error) {
	return &core.Block{Height: height}, nil
}

func (m *mockPeerAPI) FetchAnyBlockByHash(ctx context.Context, hashHex string) (*core.Block, string, error) {
	return nil, "", nil
}

func (m *mockPeerAPI) BroadcastTransaction(ctx context.Context, tx core.Transaction, hops int) {
}

func (m *mockPeerAPI) BroadcastBlock(ctx context.Context, block *core.Block) {
}

func (m *mockPeerAPI) SendBlock(ctx context.Context, peer string, block *core.Block) error {
	return nil
}

func (m *mockPeerAPI) SendHeaders(ctx context.Context, peer string, headers []core.BlockHeader) error {
	return nil
}

func (m *mockPeerAPI) EnsureAncestors(ctx context.Context, bc BlockchainInterface, missingHashHex string) error {
	return nil
}

func TestBlockDownloader_New(t *testing.T) {
	pm := &mockPeerAPI{}
	bc := &mockBlockchain{height: 100}
	validator := &mockValidator{}
	m := &metrics.Metrics{}
	syncConfig := config.SyncConfig{
		BatchSize:              500,
		MaxConcurrentDownloads: 8,
		MemoryThresholdMB:      1500,
	}

	downloader := NewBlockDownloader(pm, bc, validator, m, syncConfig)
	if downloader == nil {
		t.Fatal("expected downloader to be created")
	}

	config := downloader.GetConfig()
	if config.BatchSize != 500 {
		t.Errorf("expected batch size 500, got %d", config.BatchSize)
	}
	if config.MaxConcurrent != 8 {
		t.Errorf("expected max concurrent 8, got %d", config.MaxConcurrent)
	}
}

func TestBlockDownloader_UpdateConfig(t *testing.T) {
	pm := &mockPeerAPI{}
	bc := &mockBlockchain{height: 100}
	validator := &mockValidator{}
	m := &metrics.Metrics{}
	syncConfig := config.SyncConfig{
		BatchSize:              500,
		MaxConcurrentDownloads: 8,
		MemoryThresholdMB:      1500,
	}

	downloader := NewBlockDownloader(pm, bc, validator, m, syncConfig)

	newConfig := DownloaderConfig{
		BatchSize:     1000,
		MaxConcurrent: 16,
	}
	downloader.UpdateConfig(newConfig)

	config := downloader.GetConfig()
	if config.BatchSize != 1000 {
		t.Errorf("expected batch size 1000, got %d", config.BatchSize)
	}
	if config.MaxConcurrent != 16 {
		t.Errorf("expected max concurrent 16, got %d", config.MaxConcurrent)
	}
}

func TestBlockDownloader_BatchDownloadBlocks(t *testing.T) {
	pm := &mockPeerAPI{
		blocks: make(map[string]*core.Block),
	}
	bc := &mockBlockchain{height: 100}
	validator := &mockValidator{validateDelay: 1 * time.Millisecond}
	m := &metrics.Metrics{}

	downloader := NewBlockDownloader(pm, bc, validator, m)

	ctx := context.Background()
	progressChan := make(chan DownloadProgress, 10)

	blocks, err := downloader.BatchDownloadBlocks(ctx, "peer1", 101, 100, progressChan)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(blocks) != 100 {
		t.Errorf("expected 100 blocks, got %d", len(blocks))
	}

	select {
	case progress := <-progressChan:
		if progress.Downloaded != 100 {
			t.Errorf("expected downloaded 100, got %d", progress.Downloaded)
		}
	default:
	}
}

func TestBlockDownloader_DynamicAdjustment(t *testing.T) {
	pm := &mockPeerAPI{}
	bc := &mockBlockchain{height: 100}
	validator := &mockValidator{}
	m := &metrics.Metrics{}

	downloader := NewBlockDownloader(pm, bc, validator, m)

	time.Sleep(6 * time.Second)

	stats := downloader.GetStats()
	if _, exists := stats["current_batch_size"]; !exists {
		t.Error("expected current_batch_size in stats")
	}
	if _, exists := stats["current_concurrency"]; !exists {
		t.Error("expected current_concurrency in stats")
	}
}

func TestBatchProcessor_New(t *testing.T) {
	validator := &mockValidator{}
	store := &mockChainStore{}
	m := &metrics.Metrics{}

	processor := NewBatchProcessor(validator, store, m)
	if processor == nil {
		t.Fatal("expected processor to be created")
	}

	stats := processor.GetStats()
	if stats["verify_workers"] != DefaultBatchVerifyWorkers {
		t.Errorf("expected verify_workers %d, got %v", DefaultBatchVerifyWorkers, stats["verify_workers"])
	}
}

func TestBatchProcessor_VerifyBatchBlocks(t *testing.T) {
	validator := &mockValidator{validateDelay: 1 * time.Millisecond}
	store := &mockChainStore{}
	m := &metrics.Metrics{}

	processor := NewBatchProcessor(validator, store, m)

	blocks := make([]*core.Block, 10)
	for i := 0; i < 10; i++ {
		blocks[i] = &core.Block{
			Height: uint64(i + 1),
			Hash:   make([]byte, 32),
		}
	}

	ctx := context.Background()
	results, err := processor.VerifyBatchBlocks(ctx, blocks)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(results) != 10 {
		t.Errorf("expected 10 results, got %d", len(results))
	}

	for i, result := range results {
		if !result.IsValid {
			t.Errorf("expected block %d to be valid", i)
		}
	}
}

func TestBatchProcessor_ProcessAndStoreBatch(t *testing.T) {
	validator := &mockValidator{validateDelay: 1 * time.Millisecond}
	store := &mockChainStore{}
	m := &metrics.Metrics{}

	processor := NewBatchProcessor(validator, store, m)

	blocks := make([]*core.Block, 5)
	for i := 0; i < 5; i++ {
		blocks[i] = &core.Block{
			Height: uint64(i + 1),
			Hash:   make([]byte, 32),
		}
	}

	ctx := context.Background()
	result, err := processor.ProcessAndStoreBatch(ctx, blocks)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.BlocksStored != 5 {
		t.Errorf("expected 5 blocks stored, got %d", result.BlocksStored)
	}
	if result.FailedBlocks != 0 {
		t.Errorf("expected 0 failed blocks, got %d", result.FailedBlocks)
	}
}

type mockChainStore struct {
	mu     sync.RWMutex
	blocks map[string]*core.Block
}

func (m *mockChainStore) PutBlock(block *core.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.blocks == nil {
		m.blocks = make(map[string]*core.Block)
	}
	m.blocks[string(block.Hash)] = block
	return nil
}

func (m *mockChainStore) ReadCanonical() ([]*core.Block, error) {
	return nil, nil
}

func (m *mockChainStore) AppendCanonical(block *core.Block) error {
	return nil
}

func (m *mockChainStore) RewriteCanonical(blocks []*core.Block) error {
	return nil
}

func (m *mockChainStore) ReadAllBlocks() (map[string]*core.Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.blocks, nil
}

func (m *mockChainStore) GetRulesHash() ([]byte, bool, error) {
	return nil, false, nil
}

func (m *mockChainStore) PutRulesHash(hash []byte) error {
	return nil
}

func (m *mockChainStore) GetGenesisHash() ([]byte, bool, error) {
	return nil, false, nil
}

func (m *mockChainStore) PutGenesisHash(hash []byte) error {
	return nil
}

func (m *mockChainStore) GetCheckpoints() ([]byte, bool, error) {
	return nil, false, nil
}

func (m *mockChainStore) PutCheckpoints(data []byte) error {
	return nil
}
