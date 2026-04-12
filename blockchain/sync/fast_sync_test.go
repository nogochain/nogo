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

package sync

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

type mockStore struct{}

func (m *mockStore) SaveBlock(block *core.Block) error {
	return nil
}
func (m *mockStore) LoadBlock(hash []byte) (*core.Block, error) {
	return nil, nil
}
func (m *mockStore) LoadCanonicalChain() ([]*core.Block, error) {
	return nil, nil
}
func (m *mockStore) SaveCanonicalChain(blocks []*core.Block) error {
	return nil
}
func (m *mockStore) ReadCanonical() ([]*core.Block, error) {
	return nil, nil
}
func (m *mockStore) ReadAllBlocks() (map[string]*core.Block, error) {
	return nil, nil
}
func (m *mockStore) GetRulesHash() ([]byte, bool, error) {
	return nil, false, nil
}
func (m *mockStore) PutRulesHash(hash []byte) error {
	return nil
}
func (m *mockStore) AppendCanonical(block *core.Block) error {
	return nil
}
func (m *mockStore) RewriteCanonical(blocks []*core.Block) error {
	return nil
}
func (m *mockStore) PutBlock(block *core.Block) error {
	return nil
}
func (m *mockStore) GetGenesisHash() ([]byte, bool, error) {
	return nil, false, nil
}
func (m *mockStore) PutGenesisHash(hash []byte) error {
	return nil
}
func (m *mockStore) GetCheckpoints() ([]byte, bool, error) {
	return nil, false, nil
}
func (m *mockStore) PutCheckpoints(data []byte) error {
	return nil
}

type mockBlockchain struct {
	latestBlock *core.Block
	chainID     uint64
}

func (m *mockBlockchain) LatestBlock() *core.Block {
	return m.latestBlock
}
func (m *mockBlockchain) BlockByHeight(height uint64) (*core.Block, bool) {
	return nil, false
}
func (m *mockBlockchain) AddBlock(block *core.Block) (bool, error) {
	return true, nil
}
func (m *mockBlockchain) GetChainID() uint64 {
	return m.chainID
}

func TestFastSyncEngine_Basic(t *testing.T) {
	checkpointMgr := NewCheckpointManager()
	store := &mockStore{}
	blockchain := &mockBlockchain{
		latestBlock: &core.Block{Height: 0},
		chainID:     1,
	}
	fs, err := NewFastSyncEngine(checkpointMgr, store, blockchain)
	if err != nil {
		t.Fatalf("NewFastSyncEngine failed: %v", err)
	}
	if fs == nil {
		t.Fatal("NewFastSyncEngine returned nil")
	}
	if fs.IsSyncing() {
		t.Error("should not be syncing initially")
	}
	status := fs.GetStatus()
	if status.Phase != "idle" {
		t.Errorf("expected phase idle, got %s", status.Phase)
	}
	fs.AddPeer("http://peer1:9090")
	fs.AddPeer("http://peer2:9090")
	fs.AddSnapshotURL("http://snapshot1:8080")
	fs.AddSnapshotURL("http://snapshot2:8080")
	peers := fs.GetPeers()
	if len(peers) != 2 {
		t.Errorf("expected 2 peers, got %d", len(peers))
	}
	urls := fs.GetSnapshotURLs()
	if len(urls) != 2 {
		t.Errorf("expected 2 snapshot URLs, got %d", len(urls))
	}
}

func TestFastSyncEngine_StartStop(t *testing.T) {
	checkpointMgr := NewCheckpointManager()
	store := &mockStore{}
	blockchain := &mockBlockchain{
		latestBlock: &core.Block{Height: 0},
		chainID:     1,
	}
	fs, err := NewFastSyncEngine(checkpointMgr, store, blockchain)
	if err != nil {
		t.Fatalf("NewFastSyncEngine failed: %v", err)
	}
	ctx := context.Background()
	err = fs.Start(ctx, 10000)
	if err != nil {
		t.Logf("Start returned: %v (expected for mock)", err)
	}
	time.Sleep(100 * time.Millisecond)
	fs.Stop()
	if fs.IsSyncing() {
		t.Error("should not be syncing after Stop")
	}
	status := fs.GetStatus()
	if status.Phase != "stopped" {
		t.Errorf("expected phase stopped, got %s", status.Phase)
	}
}

func TestFastSyncEngine_GetCheckpoint(t *testing.T) {
	checkpointMgr := NewCheckpointManager()
	store := &mockStore{}
	blockchain := &mockBlockchain{
		latestBlock: &core.Block{Height: 0},
		chainID:     1,
	}
	fs, err := NewFastSyncEngine(checkpointMgr, store, blockchain)
	if err != nil {
		t.Fatalf("NewFastSyncEngine failed: %v", err)
	}
	cp, height := fs.GetLatestCheckpoint()
	if cp == nil {
		t.Error("GetLatestCheckpoint returned nil")
	}
	if height == 0 {
		t.Error("expected height > 0")
	}
	cp2, exists := fs.GetCheckpointByHeight(0)
	if !exists {
		t.Error("genesis checkpoint should exist")
	}
	if cp2.Height != 0 {
		t.Errorf("expected genesis height 0, got %d", cp2.Height)
	}
}

func TestFastSyncEngine_ProgressTracking(t *testing.T) {
	checkpointMgr := NewCheckpointManager()
	store := &mockStore{}
	blockchain := &mockBlockchain{
		latestBlock: &core.Block{Height: 0},
		chainID:     1,
	}
	fs, err := NewFastSyncEngine(checkpointMgr, store, blockchain)
	if err != nil {
		t.Fatalf("NewFastSyncEngine failed: %v", err)
	}
	fs.updateStatus("test_phase", 0.5)
	status := fs.GetStatus()
	if status.Phase != "test_phase" {
		t.Errorf("expected phase test_phase, got %s", status.Phase)
	}
	if status.Progress != 0.5 {
		t.Errorf("expected progress 0.5, got %f", status.Progress)
	}
	progress := fs.GetSyncProgress()
	if progress["phase"] != "test_phase" {
		t.Errorf("expected phase test_phase in progress map")
	}
}

func TestFastSyncEngine_EstimateSyncTime(t *testing.T) {
	checkpointMgr := NewCheckpointManager()
	store := &mockStore{}
	blockchain := &mockBlockchain{
		latestBlock: &core.Block{Height: 0},
		chainID:     1,
	}
	fs, err := NewFastSyncEngine(checkpointMgr, store, blockchain)
	if err != nil {
		t.Fatalf("NewFastSyncEngine failed: %v", err)
	}
	estimated := fs.EstimateSyncTime(10000, 100)
	if estimated <= 0 {
		t.Error("EstimateSyncTime returned invalid value")
	}
	expectedSeconds := 10000 / 100
	expected := time.Duration(expectedSeconds) * time.Second
	if estimated < expected/2 || estimated > expected*2 {
		t.Errorf("EstimateSyncTime seems inaccurate: %v vs expected ~%v", estimated, expected)
	}
}

func TestFastSyncEngine_CalculateSyncSpeed(t *testing.T) {
	checkpointMgr := NewCheckpointManager()
	store := &mockStore{}
	blockchain := &mockBlockchain{
		latestBlock: &core.Block{Height: 0},
		chainID:     1,
	}
	fs, err := NewFastSyncEngine(checkpointMgr, store, blockchain)
	if err != nil {
		t.Fatalf("NewFastSyncEngine failed: %v", err)
	}
	speed := fs.CalculateSyncSpeed(1000, 10*time.Second)
	if speed != 100 {
		t.Errorf("expected speed 100 blocks/sec, got %f", speed)
	}
	speedZero := fs.CalculateSyncSpeed(1000, 0)
	if speedZero != 0 {
		t.Error("CalculateSyncSpeed should return 0 for zero duration")
	}
}

func TestFastSyncEngine_IsCheckpointHeight(t *testing.T) {
	if !IsCheckpointHeight(0) {
		t.Error("height 0 should be checkpoint height")
	}
	if !IsCheckpointHeight(10000) {
		t.Error("height 10000 should be checkpoint height")
	}
	if !IsCheckpointHeight(20000) {
		t.Error("height 20000 should be checkpoint height")
	}
	if IsCheckpointHeight(12345) {
		t.Error("height 12345 should not be checkpoint height")
	}
}

func TestFastSyncEngine_GetCheckpointHeightsInRange(t *testing.T) {
	heights := GetCheckpointHeightsInRange(0, 30000)
	if len(heights) != 4 {
		t.Errorf("expected 4 checkpoints in range [0, 30000], got %d", len(heights))
	}
	expected := []uint64{0, 10000, 20000, 30000}
	for i, h := range heights {
		if h != expected[i] {
			t.Errorf("height[%d]: expected %d, got %d", i, expected[i], h)
		}
	}
	heightsEmpty := GetCheckpointHeightsInRange(50000, 10000)
	if heightsEmpty != nil {
		t.Error("GetCheckpointHeightsInRange should return nil for invalid range")
	}
	heightsPartial := GetCheckpointHeightsInRange(5000, 25000)
	if len(heightsPartial) != 2 {
		t.Errorf("expected 2 checkpoints in range [5000, 25000], got %d", len(heightsPartial))
	}
}

func TestCreateFastSyncCheckpoint(t *testing.T) {
	block := &core.Block{
		Hash:        make([]byte, 32),
		Height:      10000,
		TotalWork:   "1000000",
		Transactions: make([]core.Transaction, 5),
	}
	for i := range block.Hash {
		block.Hash[i] = byte(i)
	}
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	signFunc := func(hash []byte) ([]byte, error) {
		return ed25519.Sign(privKey, hash), nil
	}
	cp, err := CreateFastSyncCheckpoint(block, "state_root", pubKey, signFunc)
	if err != nil {
		t.Fatalf("CreateFastSyncCheckpoint failed: %v", err)
	}
	if cp == nil {
		t.Fatal("CreateFastSyncCheckpoint returned nil")
	}
	if cp.Height != 10000 {
		t.Errorf("expected height 10000, got %d", cp.Height)
	}
	if len(cp.Signature) != SignatureSize {
		t.Errorf("expected signature length %d, got %d", SignatureSize, len(cp.Signature))
	}
	_, err = CreateFastSyncCheckpoint(nil, "state_root", pubKey, signFunc)
	if err == nil {
		t.Error("CreateFastSyncCheckpoint should fail for nil block")
	}
	_, err = CreateFastSyncCheckpoint(&core.Block{Height: 12345}, "state_root", pubKey, signFunc)
	if err == nil {
		t.Error("CreateFastSyncCheckpoint should fail for non-checkpoint height")
	}
}

func TestCheckpoint_HashForSigning(t *testing.T) {
	cp := &Checkpoint{
		Version:   CheckpointVersion,
		Height:    10000,
		BlockHash: make([]byte, 32),
		StateRoot: "state_root",
		Timestamp: time.Now().Unix(),
	}
	for i := range cp.BlockHash {
		cp.BlockHash[i] = byte(i)
	}
	hash1 := cp.HashForSigning()
	if hash1 == nil {
		t.Fatal("HashForSigning returned nil")
	}
	if len(hash1) != 32 {
		t.Errorf("expected hash length 32, got %d", len(hash1))
	}
	hash2 := cp.HashForSigning()
	for i := range hash1 {
		if hash1[i] != hash2[i] {
			t.Errorf("hash not deterministic at byte %d", i)
		}
	}
	cp.Height = 20000
	hash3 := cp.HashForSigning()
	for i := range hash1 {
		if hash1[i] == hash3[i] {
			if i < 8 {
				t.Error("hash should change when height changes")
			}
		}
	}
}

func TestFastSyncEngine_FastSyncWithSnapshot(t *testing.T) {
	checkpointMgr := NewCheckpointManager()
	store := &mockStore{}
	blockchain := &mockBlockchain{
		latestBlock: &core.Block{Height: 0},
		chainID:     1,
	}
	fs, err := NewFastSyncEngine(checkpointMgr, store, blockchain)
	if err != nil {
		t.Fatalf("NewFastSyncEngine failed: %v", err)
	}
	checkpoint := &Checkpoint{
		Version:   CheckpointVersion,
		Height:    10000,
		BlockHash: make([]byte, 32),
		StateRoot: "state_root",
		Timestamp: time.Now().Unix(),
	}
	for i := range checkpoint.BlockHash {
		checkpoint.BlockHash[i] = byte(i)
	}
	snapshot := &StateSnapshot{
		Version:    SnapshotVersion,
		Checkpoint: checkpoint,
		AccountStates: []AccountState{
			{Address: "addr1", Balance: 1000, Nonce: 1},
		},
		StateRoot:     "computed_root",
		Timestamp:     time.Now().Unix(),
		Validator:     make([]byte, PubKeySize),
		Signature:     make([]byte, SignatureSize),
		MerkleRoot:    make([]byte, 32),
		TotalAccounts: 1,
	}
	ctx := context.Background()
	err := fs.FastSyncWithSnapshot(ctx, snapshot)
	if err == nil {
		t.Log("FastSyncWithSnapshot succeeded (mock implementation)")
	}
	errNil := fs.FastSyncWithSnapshot(ctx, nil)
	if errNil == nil {
		t.Error("FastSyncWithSnapshot should fail for nil snapshot")
	}
}

func TestFastSyncEngine_ConcurrentAccess(t *testing.T) {
	checkpointMgr := NewCheckpointManager()
	store := &mockStore{}
	blockchain := &mockBlockchain{
		latestBlock: &core.Block{Height: 0},
		chainID:     1,
	}
	fs, err := NewFastSyncEngine(checkpointMgr, store, blockchain)
	if err != nil {
		t.Fatalf("NewFastSyncEngine failed: %v", err)
	}
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = fs.GetStatus()
			_ = fs.GetSyncProgress()
			_ = fs.GetPeers()
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
