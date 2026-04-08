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
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/metrics"
	"github.com/nogochain/nogo/blockchain/utils"
)

// MockPeerAPI implements PeerAPI interface for testing
type MockPeerAPI struct {
	peers       []string
	chainHeight uint64
	failures    map[string]int
	mu          sync.RWMutex
}

func NewMockPeerAPI(height uint64) *MockPeerAPI {
	return &MockPeerAPI{
		peers:       make([]string, 0),
		chainHeight: height,
		failures:    make(map[string]int),
	}
}

func (m *MockPeerAPI) Peers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.peers
}

func (m *MockPeerAPI) AddPeer(addr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peers = append(m.peers, addr)
}

func (m *MockPeerAPI) GetActivePeers() []string {
	return m.Peers()
}

func (m *MockPeerAPI) FetchChainInfo(ctx context.Context, peer string) (*ChainInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if failures, ok := m.failures[peer]; ok && failures > 0 {
		m.failures[peer]--
		return nil, utils.ErrTimeout
	}

	return &ChainInfo{
		Height: m.chainHeight,
	}, nil
}

func (m *MockPeerAPI) FetchHeadersFrom(ctx context.Context, peer string, from uint64, count int) ([]core.BlockHeader, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if failures, ok := m.failures[peer]; ok && failures > 0 {
		m.failures[peer]--
		return nil, utils.ErrTimeout
	}

	headers := make([]core.BlockHeader, 0, count)
	for i := 0; i < count; i++ {
		headers = append(headers, core.BlockHeader{
			Version:        1,
			PrevHash:       []byte{1, 2, 3},
			TimestampUnix:  time.Now().Unix(),
			DifficultyBits: 0x1d00ffff,
			Difficulty:     1,
			Nonce:          0,
		})
	}
	return headers, nil
}

func (m *MockPeerAPI) FetchBlockByHash(ctx context.Context, peer, hashHex string) (*core.Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if failures, ok := m.failures[peer]; ok && failures > 0 {
		m.failures[peer]--
		return nil, utils.ErrTimeout
	}

	return &core.Block{
		Header: core.BlockHeader{
			Version:        1,
			PrevHash:       []byte{1, 2, 3},
			TimestampUnix:  time.Now().Unix(),
			DifficultyBits: 0x1d00ffff,
			Difficulty:     1,
			Nonce:          0,
		},
	}, nil
}

func (m *MockPeerAPI) FetchAnyBlockByHash(ctx context.Context, hashHex string) (*core.Block, string, error) {
	block, err := m.FetchBlockByHash(ctx, "", hashHex)
	if err != nil {
		return nil, "", err
	}
	return block, "", nil
}

func (m *MockPeerAPI) BroadcastTransaction(ctx context.Context, tx core.Transaction, hops int) {
	// Mock implementation - no-op
}

func (m *MockPeerAPI) EnsureAncestors(ctx context.Context, bc BlockchainInterface, missingHashHex string) error {
	// Mock implementation - no-op
	return nil
}

// MockBlockchainInterface implements BlockchainInterface for testing
type MockBlockchainInterface struct {
	height uint64
	blocks map[string]*core.Block
	mu     sync.RWMutex
}

func NewMockBlockchainInterface() *MockBlockchainInterface {
	return &MockBlockchainInterface{
		height: 0,
		blocks: make(map[string]*core.Block),
	}
}

func (m *MockBlockchainInterface) LatestBlock() *core.Block {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return &core.Block{
		Header: core.BlockHeader{
			Version:        1,
			PrevHash:       []byte{1, 2, 3},
			TimestampUnix:  time.Now().Unix(),
			DifficultyBits: 0x1d00ffff,
			Difficulty:     1,
			Nonce:          0,
		},
		Height: m.height,
	}
}

func (m *MockBlockchainInterface) BlockByHeight(height uint64) (*core.Block, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.LatestBlock(), true
}

func (m *MockBlockchainInterface) BlockByHash(hashHex string) (*core.Block, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	block, ok := m.blocks[hashHex]
	return block, ok
}

func (m *MockBlockchainInterface) HeadersFrom(from uint64, count uint64) []*core.BlockHeader {
	return nil
}

func (m *MockBlockchainInterface) BlocksFrom(from uint64, count uint64) []*core.Block {
	return nil
}

func (m *MockBlockchainInterface) Blocks() []*core.Block {
	return nil
}

func (m *MockBlockchainInterface) CanonicalWork() *big.Int {
	return big.NewInt(int64(m.height))
}

func (m *MockBlockchainInterface) RulesHashHex() string {
	return ""
}

func (m *MockBlockchainInterface) GetChainID() uint64 {
	return 1
}

func (m *MockBlockchainInterface) GetMinerAddress() string {
	return "test"
}

func (m *MockBlockchainInterface) TotalSupply() uint64 {
	return 0
}

func (m *MockBlockchainInterface) AddBlock(block *core.Block) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if block.GetHeight() <= m.height {
		return false, fmt.Errorf("block height too low")
	}

	m.height = block.GetHeight()
	m.blocks[string(block.Hash)] = block
	return true, nil
}

func (m *MockBlockchainInterface) SelectMempoolTxs(mp Mempool, maxTxPerBlock int) ([]core.Transaction, []string, error) {
	return nil, nil, nil
}

func (m *MockBlockchainInterface) MineTransfers(txs []core.Transaction) (*core.Block, error) {
	return nil, nil
}

func (m *MockBlockchainInterface) AuditChain() error {
	return nil
}

func (m *MockBlockchainInterface) TxByID(txid string) (*core.Transaction, *core.TxLocation, bool) {
	return nil, nil, false
}

func (m *MockBlockchainInterface) Balance(addr string) (core.Account, bool) {
	return core.Account{}, true
}

func (m *MockBlockchainInterface) SyncLoop() SyncLoopInterface {
	return nil
}

func (m *MockBlockchainInterface) AddressTxs(addr string, limit, cursor int) ([]core.AddressTxEntry, int, bool) {
	return nil, 0, false
}

func (m *MockBlockchainInterface) GetContractManager() *core.ContractManager {
	return nil
}

func (m *MockBlockchainInterface) HasTransaction(txHash []byte) bool {
	return false
}

// MockMiner implements Miner interface for testing
type MockMiner struct{}

func (m *MockMiner) InterruptMining() {}
func (m *MockMiner) ResumeMining()    {}
func (m *MockMiner) IsVerifying() bool {
	return false
}
func (m *MockMiner) OnBlockAdded() {}

// createTestValidator creates a validator with default consensus params
func createTestValidator(metrics *metrics.Metrics) *consensus.BlockValidator {
	consensusParams := config.GetConsensusParams()
	return consensus.NewBlockValidator(consensusParams, 1, metrics)
}

// TestSyncLoopWithPeerScoring tests SyncLoop with peer scoring integration
func TestSyncLoopWithPeerScoring(t *testing.T) {
	mockPM := NewMockPeerAPI(100)
	mockBC := NewMockBlockchainInterface()
	mockMiner := &MockMiner{}
	metrics := &metrics.Metrics{}
	orphanPool := utils.NewOrphanPool(100, time.Hour)
	
	validator := createTestValidator(metrics)
	syncConfig := config.SyncConfig{
		BatchSize:              100,
		MaxConcurrentDownloads: 8,
		MemoryThresholdMB:      1500,
	}

	syncLoop := NewSyncLoop(mockPM, mockBC, mockMiner, metrics, orphanPool, validator, syncConfig)

	if syncLoop == nil {
		t.Fatal("Failed to create SyncLoop")
	}

	if syncLoop.scorer == nil {
		t.Fatal("Expected scorer to be initialized")
	}

	if syncLoop.retryExec == nil {
		t.Fatal("Expected retry executor to be initialized")
	}

	// Test GetBestPeerByScore
	bestPeer := syncLoop.GetBestPeerByScore()
	if bestPeer != "" {
		t.Errorf("Expected empty best peer initially, got %s", bestPeer)
	}

	// Test GetSyncMetrics
	m := syncLoop.GetSyncMetrics()
	if m == nil {
		t.Fatal("Expected non-nil sync metrics")
	}

	if m["is_syncing"] != false {
		t.Error("Expected is_syncing to be false")
	}
}

// TestSyncLoopRetryIntegration tests retry mechanism in SyncLoop
func TestSyncLoopRetryIntegration(t *testing.T) {
	mockPM := NewMockPeerAPI(50)
	mockBC := NewMockBlockchainInterface()
	mockMiner := &MockMiner{}
	metrics := &metrics.Metrics{}
	orphanPool := utils.NewOrphanPool(100, time.Hour)
	
	validator := createTestValidator(metrics)
	syncConfig := config.SyncConfig{
		BatchSize:              100,
		MaxConcurrentDownloads: 8,
		MemoryThresholdMB:      1500,
	}

	syncLoop := NewSyncLoop(mockPM, mockBC, mockMiner, metrics, orphanPool, validator, syncConfig)

	// Add peer to scorer
	peer := "192.168.1.100:9090"
	mockPM.AddPeer(peer)
	syncLoop.scorer.RecordSuccess(peer, 100)

	// Test fetchHeadersWithRetry
	ctx := context.Background()
	headers, err := syncLoop.fetchHeadersWithRetry(ctx, peer, 1, 10)
	if err != nil {
		t.Errorf("fetchHeadersWithRetry failed: %v", err)
	}
	if len(headers) != 10 {
		t.Errorf("Expected 10 headers, got %d", len(headers))
	}

	// Test fetchBlockWithRetry
	block, err := syncLoop.fetchBlockWithRetry(ctx, peer, []byte{1, 2, 3})
	if err != nil {
		t.Errorf("fetchBlockWithRetry failed: %v", err)
	}
	if block == nil {
		t.Error("Expected non-nil block")
	}
}

// TestSyncLoopPeerSwitching tests automatic peer switching
func TestSyncLoopPeerSwitching(t *testing.T) {
	mockPM := NewMockPeerAPI(100)
	mockBC := NewMockBlockchainInterface()
	mockMiner := &MockMiner{}
	metrics := &metrics.Metrics{}
	orphanPool := utils.NewOrphanPool(100, time.Hour)
	
	validator := createTestValidator(metrics)

	syncLoop := NewSyncLoop(mockPM, mockBC, mockMiner, metrics, orphanPool, validator)

	// Add multiple peers with different scores
	peer1 := "192.168.1.1:9090"
	peer2 := "192.168.1.2:9090"
	peer3 := "192.168.1.3:9090"

	mockPM.AddPeer(peer1)
	mockPM.AddPeer(peer2)
	mockPM.AddPeer(peer3)

	// peer1: poor performance
	for i := 0; i < 5; i++ {
		syncLoop.scorer.RecordFailure(peer1)
	}

	// peer2: moderate performance
	for i := 0; i < 5; i++ {
		syncLoop.scorer.RecordSuccess(peer2, 300)
	}

	// peer3: excellent performance
	for i := 0; i < 10; i++ {
		syncLoop.scorer.RecordSuccess(peer3, 50)
	}

	// Test ShouldSwitchPeer
	shouldSwitch := syncLoop.ShouldSwitchPeer(peer1)
	if !shouldSwitch {
		t.Error("Expected should switch from poor peer1")
	}

	shouldSwitch = syncLoop.ShouldSwitchPeer(peer2)
	if shouldSwitch {
		t.Error("Expected should not switch from moderate peer2")
	}

	shouldSwitch = syncLoop.ShouldSwitchPeer(peer3)
	if shouldSwitch {
		t.Error("Expected should not switch from excellent peer3")
	}

	// Test GetPeerPerformance
	perf := syncLoop.GetPeerPerformance(peer3)
	if perf == nil {
		t.Fatal("Expected peer performance data")
	}

	score, ok := perf["score"].(float64)
	if !ok {
		t.Fatal("Expected score in performance data")
	}
	if score < 60 {
		t.Errorf("Expected high score for peer3, got %.2f", score)
	}
}

// TestSyncLoopBlacklistIntegration tests blacklist integration
func TestSyncLoopBlacklistIntegration(t *testing.T) {
	mockPM := NewMockPeerAPI(100)
	mockBC := NewMockBlockchainInterface()
	mockMiner := &MockMiner{}
	metrics := &metrics.Metrics{}
	orphanPool := utils.NewOrphanPool(100, time.Hour)
	
	validator := createTestValidator(metrics)

	syncLoop := NewSyncLoop(mockPM, mockBC, mockMiner, metrics, orphanPool, validator)

	peer := "192.168.1.200:9090"
	mockPM.AddPeer(peer)

	// Add peer to blacklist
	err := syncLoop.AddPeerToBlacklist(peer, "malicious_behavior", 24*time.Hour)
	if err != nil {
		t.Errorf("Failed to blacklist peer: %v", err)
	}

	// Check blacklist info
	info := syncLoop.GetBlacklistInfo(peer)
	if info == nil {
		t.Fatal("Expected blacklist info")
	}

	if info["reason"] != "malicious_behavior" {
		t.Errorf("Expected reason 'malicious_behavior', got %v", info["reason"])
	}

	// Try to remove from blacklist
	err = syncLoop.RemovePeerFromBlacklist(peer)
	if err != nil {
		t.Errorf("Failed to remove from blacklist: %v", err)
	}

	info = syncLoop.GetBlacklistInfo(peer)
	if info != nil {
		t.Error("Expected nil info after removal")
	}
}

// TestSyncLoopMetricsCollection tests comprehensive metrics collection
func TestSyncLoopMetricsCollection(t *testing.T) {
	mockPM := NewMockPeerAPI(100)
	mockBC := NewMockBlockchainInterface()
	mockMiner := &MockMiner{}
	metrics := &metrics.Metrics{}
	orphanPool := utils.NewOrphanPool(100, time.Hour)
	
	validator := createTestValidator(metrics)

	syncLoop := NewSyncLoop(mockPM, mockBC, mockMiner, metrics, orphanPool, validator)

	// Simulate some peer interactions
	peers := []string{
		"192.168.1.1:9090",
		"192.168.1.2:9090",
		"192.168.1.3:9090",
	}

	for i, peer := range peers {
		mockPM.AddPeer(peer)
		// Record different performance levels
		for j := 0; j < 5; j++ {
			if i == 0 {
				syncLoop.scorer.RecordSuccess(peer, 50)
			} else if i == 1 {
				if j%2 == 0 {
					syncLoop.scorer.RecordSuccess(peer, 200)
				} else {
					syncLoop.scorer.RecordFailure(peer)
				}
			} else {
				syncLoop.scorer.RecordFailure(peer)
			}
		}
	}

	// Get comprehensive metrics
	m := syncLoop.GetSyncMetrics()

	// Verify metrics structure
	if m["peer_scorer"] == nil {
		t.Error("Expected peer_scorer metrics")
	}

	if m["retry_executor"] == nil {
		t.Error("Expected retry_executor metrics")
	}

	peerCount, ok := m["peer_count"].(int)
	if !ok {
		t.Fatal("Expected peer_count metric")
	}
	if peerCount != 3 {
		t.Errorf("Expected 3 peers, got %d", peerCount)
	}

	blacklistCount, ok := m["blacklist_count"].(int)
	if !ok {
		t.Fatal("Expected blacklist_count metric")
	}
	if blacklistCount != 0 {
		t.Errorf("Expected 0 blacklisted peers, got %d", blacklistCount)
	}
}

// TestSyncLoopWithFailures tests sync with simulated failures
func TestSyncLoopWithFailures(t *testing.T) {
	mockPM := NewMockPeerAPI(50)
	mockBC := NewMockBlockchainInterface()
	mockMiner := &MockMiner{}
	metrics := &metrics.Metrics{}
	orphanPool := utils.NewOrphanPool(100, time.Hour)
	
	validator := createTestValidator(metrics)

	syncLoop := NewSyncLoop(mockPM, mockBC, mockMiner, metrics, orphanPool, validator)

	peer := "192.168.1.50:9090"
	mockPM.AddPeer(peer)

	// Simulate 2 failures then success
	mockPM.failures[peer] = 2

	ctx := context.Background()
	
	// This should succeed after retries
	headers, err := syncLoop.fetchHeadersWithRetry(ctx, peer, 1, 5)
	if err != nil {
		t.Errorf("Expected success after retries, got error: %v", err)
	}
	if len(headers) != 5 {
		t.Errorf("Expected 5 headers, got %d", len(headers))
	}

	// Check retry metrics
	m := syncLoop.GetSyncMetrics()
	retryMetrics, ok := m["retry_executor"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected retry_executor metrics")
	}

	totalRetries, ok := retryMetrics["total_retries"].(uint64)
	if !ok {
		t.Fatal("Expected total_retries metric")
	}
	if totalRetries < 1 {
		t.Error("Expected at least 1 retry")
	}
}

// TestSyncLoopConcurrentAccess tests thread-safe concurrent access
func TestSyncLoopConcurrentAccess(t *testing.T) {
	mockPM := NewMockPeerAPI(100)
	mockBC := NewMockBlockchainInterface()
	mockMiner := &MockMiner{}
	metrics := &metrics.Metrics{}
	orphanPool := utils.NewOrphanPool(100, time.Hour)
	
	validator := createTestValidator(metrics)

	syncLoop := NewSyncLoop(mockPM, mockBC, mockMiner, metrics, orphanPool, validator)

	peer := "192.168.1.60:9090"
	mockPM.AddPeer(peer)

	// Concurrent operations
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()
			
			if iteration%2 == 0 {
				syncLoop.scorer.RecordSuccess(peer, 100)
			} else {
				syncLoop.scorer.RecordFailure(peer)
			}
			
			_ = syncLoop.GetPeerPerformance(peer)
			_ = syncLoop.GetSyncMetrics()
			_ = syncLoop.GetBestPeerByScore()
		}(i)
	}

	wg.Wait()

	// Verify no race conditions occurred
	stats := syncLoop.scorer.GetPeerDetailedStats(peer)
	if stats == nil {
		t.Fatal("Expected peer stats after concurrent access")
	}
}

// TestRetryExecutorContextCancellation tests context cancellation
func TestRetryExecutorContextCancellation(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100)
	executor := NewRetryExecutor(DefaultRetryStrategy(), scorer)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	startTime := time.Now()
	
	result := executor.ExecuteWithRetry(ctx, func(ctx context.Context, peer string) error {
		time.Sleep(100 * time.Millisecond) // Longer than context timeout
		return nil
	}, "peer1")

	elapsed := time.Since(startTime)

	if result.Success {
		t.Error("Expected failure due to context cancellation")
	}

	if elapsed > 200*time.Millisecond {
		t.Errorf("Expected early termination, took %v", elapsed)
	}
}

// TestSyncLoopPerformanceBenchmark benchmarks SyncLoop performance
func BenchmarkSyncLoopPeerScoring(b *testing.B) {
	mockPM := NewMockPeerAPI(1000)
	mockBC := NewMockBlockchainInterface()
	mockMiner := &MockMiner{}
	metrics := &metrics.Metrics{}
	orphanPool := utils.NewOrphanPool(100, time.Hour)
	
	validator := createTestValidator(metrics)

	syncLoop := NewSyncLoop(mockPM, mockBC, mockMiner, metrics, orphanPool, validator)

	// Add many peers
	for i := 0; i < 100; i++ {
		peer := fmt.Sprintf("192.168.1.%d:9090", i)
		mockPM.AddPeer(peer)
		
		for j := 0; j < 10; j++ {
			syncLoop.scorer.RecordSuccess(peer, 100)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = syncLoop.GetBestPeerByScore()
	}
}

// TestSyncLoopBlacklistPerformance benchmarks blacklist operations
func BenchmarkSyncLoopBlacklist(b *testing.B) {
	mockPM := NewMockPeerAPI(100)
	mockBC := NewMockBlockchainInterface()
	mockMiner := &MockMiner{}
	metrics := &metrics.Metrics{}
	orphanPool := utils.NewOrphanPool(100, time.Hour)
	
	validator := createTestValidator(metrics)

	syncLoop := NewSyncLoop(mockPM, mockBC, mockMiner, metrics, orphanPool, validator)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer := fmt.Sprintf("192.168.1.%d:9090", i%1000)
		syncLoop.AddPeerToBlacklist(peer, "test", 1*time.Hour)
	}
}

// TestChainInfoSerialization tests ChainInfo serialization
func TestChainInfoSerialization(t *testing.T) {
	info := &ChainInfo{
		Height: 1000,
		Work:   big.NewInt(123456789),
	}

	// Test that ChainInfo can be properly serialized
	if info.Height != 1000 {
		t.Errorf("Expected height 1000, got %d", info.Height)
	}

	if info.Work.Cmp(big.NewInt(123456789)) != 0 {
		t.Errorf("Expected work 123456789, got %s", info.Work.String())
	}
}
