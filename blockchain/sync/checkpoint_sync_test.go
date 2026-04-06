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
	"crypto/ed25519"
	"encoding/hex"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

func TestCheckpointManager_NewCheckpointManager(t *testing.T) {
	cm := NewCheckpointManager()
	if cm == nil {
		t.Fatal("NewCheckpointManager returned nil")
	}
	if cm.Count() < MinCheckpoints {
		t.Errorf("expected at least %d hardcoded checkpoints, got %d", MinCheckpoints, cm.Count())
	}
}

func TestCheckpointManager_AddTrustedValidator(t *testing.T) {
	cm := NewCheckpointManager()
	pubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pubKeyHex := hex.EncodeToString(pubKey)
	err = cm.AddTrustedValidator(pubKeyHex)
	if err != nil {
		t.Errorf("AddTrustedValidator failed: %v", err)
	}
	err = cm.AddTrustedValidator("invalid_hex")
	if err == nil {
		t.Error("AddTrustedValidator should fail for invalid hex")
	}
	err = cm.AddTrustedValidator("0123456789abcdef")
	if err == nil {
		t.Error("AddTrustedValidator should fail for short key")
	}
}

func TestCheckpointManager_CreateAndVerifyCheckpoint(t *testing.T) {
	cm := NewCheckpointManager()
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pubKeyHex := hex.EncodeToString(pubKey)
	err = cm.AddTrustedValidator(pubKeyHex)
	if err != nil {
		t.Fatalf("AddTrustedValidator failed: %v", err)
	}
	block := &core.Block{
		Hash:        make([]byte, 32),
		Height:      10000,
		TotalWork:   "1000000",
		Transactions: make([]core.Transaction, 5),
	}
	for i := range block.Hash {
		block.Hash[i] = byte(i)
	}
	signFunc := func(hash []byte) ([]byte, error) {
		signature := ed25519.Sign(privKey, hash)
		return signature, nil
	}
	cp, err := cm.CreateCheckpoint(10000, block, "state_root_hex", pubKey, signFunc)
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}
	if cp == nil {
		t.Fatal("CreateCheckpoint returned nil")
	}
	if cp.Height != 10000 {
		t.Errorf("expected height 10000, got %d", cp.Height)
	}
	if len(cp.Signature) != SignatureSize {
		t.Errorf("expected signature length %d, got %d", SignatureSize, len(cp.Signature))
	}
	err = cm.VerifyCheckpoint(cp)
	if err != nil {
		t.Errorf("VerifyCheckpoint failed: %v", err)
	}
}

func TestCheckpointManager_VerifyCheckpointFailures(t *testing.T) {
	cm := NewCheckpointManager()
	tests := []struct {
		name        string
		checkpoint  *Checkpoint
		expectError bool
	}{
		{
			name:        "nil checkpoint",
			checkpoint:  nil,
			expectError: true,
		},
		{
			name: "wrong version",
			checkpoint: &Checkpoint{
				Version:   99,
				Height:    10000,
				BlockHash: make([]byte, 32),
				StateRoot: "state_root",
				Timestamp: time.Now().Unix(),
			},
			expectError: true,
		},
		{
			name: "invalid height",
			checkpoint: &Checkpoint{
				Version:   CheckpointVersion,
				Height:    12345,
				BlockHash: make([]byte, 32),
				StateRoot: "state_root",
				Timestamp: time.Now().Unix(),
			},
			expectError: true,
		},
		{
			name: "invalid hash length",
			checkpoint: &Checkpoint{
				Version:   CheckpointVersion,
				Height:    10000,
				BlockHash: make([]byte, 16),
				StateRoot: "state_root",
				Timestamp: time.Now().Unix(),
			},
			expectError: true,
		},
		{
			name: "empty state root",
			checkpoint: &Checkpoint{
				Version:   CheckpointVersion,
				Height:    10000,
				BlockHash: make([]byte, 32),
				StateRoot: "",
				Timestamp: time.Now().Unix(),
			},
			expectError: true,
		},
		{
			name: "invalid timestamp",
			checkpoint: &Checkpoint{
				Version:   CheckpointVersion,
				Height:    10000,
				BlockHash: make([]byte, 32),
				StateRoot: "state_root",
				Timestamp: 0,
			},
			expectError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cm.VerifyCheckpoint(tt.checkpoint)
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestCheckpointManager_GetCheckpoint(t *testing.T) {
	cm := NewCheckpointManager()
	cp, exists := cm.GetCheckpoint(0)
	if !exists {
		t.Error("expected genesis checkpoint to exist")
	}
	if cp == nil {
		t.Fatal("GetCheckpoint returned nil for genesis")
	}
	if cp.Height != 0 {
		t.Errorf("expected genesis height 0, got %d", cp.Height)
	}
	_, exists = cm.GetCheckpoint(999999)
	if exists {
		t.Error("expected non-existent checkpoint to not exist")
	}
}

func TestCheckpointManager_GetLatestCheckpoint(t *testing.T) {
	cm := NewCheckpointManager()
	cp, height := cm.GetLatestCheckpoint()
	if cp == nil {
		t.Fatal("GetLatestCheckpoint returned nil")
	}
	if height == 0 {
		t.Error("expected latest height > 0")
	}
	if cp.Height != height {
		t.Errorf("checkpoint height %d != latest height %d", cp.Height, height)
	}
}

func TestCheckpointManager_GetAllCheckpoints(t *testing.T) {
	cm := NewCheckpointManager()
	checkpoints := cm.GetAllCheckpoints()
	if len(checkpoints) < MinCheckpoints {
		t.Errorf("expected at least %d checkpoints, got %d", MinCheckpoints, len(checkpoints))
	}
	for i := 1; i < len(checkpoints); i++ {
		if checkpoints[i].Height <= checkpoints[i-1].Height {
			t.Errorf("checkpoints not sorted: %d <= %d", checkpoints[i].Height, checkpoints[i-1].Height)
		}
	}
}

func TestCheckpointManager_MerkleRoot(t *testing.T) {
	cm := NewCheckpointManager()
	root := cm.GetMerkleRoot()
	if root == nil {
		t.Error("GetMerkleRoot returned nil")
	}
	checkpoints := cm.GetAllCheckpoints()
	valid := cm.VerifyMerkleRoot(checkpoints, root)
	if !valid {
		t.Error("Merkle root verification failed")
	}
	invalidRoot := make([]byte, len(root))
	copy(invalidRoot, root)
	invalidRoot[0] ^= 0xFF
	valid = cm.VerifyMerkleRoot(checkpoints, invalidRoot)
	if valid {
		t.Error("Merkle root verification should fail for invalid root")
	}
}

func TestCheckpointManager_FinalizedHeight(t *testing.T) {
	cm := NewCheckpointManager()
	finalized := cm.GetFinalizedHeight()
	if finalized == 0 {
		t.Error("expected finalized height > 0")
	}
	if finalized%CheckpointInterval != 0 {
		t.Errorf("finalized height %d not multiple of %d", finalized, CheckpointInterval)
	}
	isFinalized := cm.IsFinalized(finalized)
	if !isFinalized {
		t.Error("expected finalized height to be finalized")
	}
	isNotFinalized := cm.IsFinalized(finalized + CheckpointInterval + 1000)
	if isNotFinalized {
		t.Error("expected future height to not be finalized")
	}
}

func TestCheckpointManager_PruneOldCheckpoints(t *testing.T) {
	cm := NewCheckpointManager()
	initialCount := cm.Count()
	keepCount := MinCheckpoints
	err := cm.PruneOldCheckpoints(keepCount)
	if err != nil {
		t.Errorf("PruneOldCheckpoints failed: %v", err)
	}
	finalCount := cm.Count()
	if finalCount > initialCount {
		t.Errorf("checkpoint count increased after prune: %d -> %d", initialCount, finalCount)
	}
	if finalCount < keepCount && initialCount > keepCount {
		t.Errorf("pruned too many checkpoints: expected at least %d, got %d", keepCount, finalCount)
	}
	err = cm.PruneOldCheckpoints(1)
	if err == nil {
		t.Error("PruneOldCheckpoints should fail for keepCount < MinCheckpoints")
	}
}

func TestCheckpointManager_JSONMarshaling(t *testing.T) {
	cm := NewCheckpointManager()
	data, err := cm.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("MarshalJSON returned empty data")
	}
}

func TestCheckpointManager_CreateCheckpointValidation(t *testing.T) {
	cm := NewCheckpointManager()
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pubKeyHex := hex.EncodeToString(pubKey)
	err = cm.AddTrustedValidator(pubKeyHex)
	if err != nil {
		t.Fatalf("AddTrustedValidator failed: %v", err)
	}
	signFunc := func(hash []byte) ([]byte, error) {
		return ed25519.Sign(privKey, hash), nil
	}
	block := &core.Block{
		Hash:        make([]byte, 32),
		Height:      10000,
		TotalWork:   "1000000",
		Transactions: make([]core.Transaction, 5),
	}
	_, err = cm.CreateCheckpoint(10000, nil, "state_root", pubKey, signFunc)
	if err == nil {
		t.Error("CreateCheckpoint should fail for nil block")
	}
	_, err = cm.CreateCheckpoint(10000, block, "", pubKey, signFunc)
	if err == nil {
		t.Error("CreateCheckpoint should fail for empty state root")
	}
	invalidBlock := &core.Block{
		Hash:      make([]byte, 16),
		Height:    10000,
		TotalWork: "1000000",
	}
	_, err = cm.CreateCheckpoint(10000, invalidBlock, "state_root", pubKey, signFunc)
	if err == nil {
		t.Error("CreateCheckpoint should fail for invalid block hash length")
	}
}

func TestCheckpointManager_SignatureVerification(t *testing.T) {
	cm := NewCheckpointManager()
	pubKey1, privKey1, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key 1: %v", err)
	}
	pubKey2, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key 2: %v", err)
	}
	pubKeyHex1 := hex.EncodeToString(pubKey1)
	err = cm.AddTrustedValidator(pubKeyHex1)
	if err != nil {
		t.Fatalf("AddTrustedValidator failed: %v", err)
	}
	block := &core.Block{
		Hash:        make([]byte, 32),
		Height:      10000,
		TotalWork:   "1000000",
		Transactions: make([]core.Transaction, 5),
	}
	for i := range block.Hash {
		block.Hash[i] = byte(i)
	}
	signFunc := func(hash []byte) ([]byte, error) {
		return ed25519.Sign(privKey1, hash), nil
	}
	cp, err := cm.CreateCheckpoint(10000, block, "state_root", pubKey1, signFunc)
	if err != nil {
		t.Fatalf("CreateCheckpoint failed: %v", err)
	}
	cp.Validator = pubKey2
	err = cm.VerifyCheckpoint(cp)
	if err == nil {
		t.Error("VerifyCheckpoint should fail for untrusted validator")
	}
	cp.Validator = pubKey1
	cp.Signature[0] ^= 0xFF
	err = cm.VerifyCheckpoint(cp)
	if err == nil {
		t.Error("VerifyCheckpoint should fail for invalid signature")
	}
}

func TestCheckpointManager_ParentHash(t *testing.T) {
	cm := NewCheckpointManager()
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pubKeyHex := hex.EncodeToString(pubKey)
	err = cm.AddTrustedValidator(pubKeyHex)
	if err != nil {
		t.Fatalf("AddTrustedValidator failed: %v", err)
	}
	signFunc := func(hash []byte) ([]byte, error) {
		return ed25519.Sign(privKey, hash), nil
	}
	block1 := &core.Block{
		Hash:        make([]byte, 32),
		Height:      110000,
		TotalWork:   "1000000",
		Transactions: make([]core.Transaction, 5),
	}
	for i := range block1.Hash {
		block1.Hash[i] = byte(i)
	}
	cp1, err := cm.CreateCheckpoint(110000, block1, "state_root_1", pubKey, signFunc)
	if err != nil {
		t.Fatalf("CreateCheckpoint for block1 failed: %v", err)
	}
	if len(cp1.ParentHash) == 0 {
		t.Error("checkpoint at 110000 should have parent hash (from 100000)")
	}
	block2 := &core.Block{
		Hash:        make([]byte, 32),
		Height:      120000,
		TotalWork:   "2000000",
		Transactions: make([]core.Transaction, 10),
	}
	for i := range block2.Hash {
		block2.Hash[i] = byte(i + 10)
	}
	cp2, err := cm.CreateCheckpoint(120000, block2, "state_root_2", pubKey, signFunc)
	if err != nil {
		t.Fatalf("CreateCheckpoint for block2 failed: %v", err)
	}
	if len(cp2.ParentHash) == 0 {
		t.Error("checkpoint at 120000 should have parent hash")
	}
	if len(cp2.ParentHash) != len(cp1.BlockHash) {
		t.Error("parent hash length mismatch")
	}
}

func TestHelperFunctions(t *testing.T) {
	if !ValidateCheckpointInterval(0) {
		t.Error("ValidateCheckpointInterval should return true for height 0")
	}
	if !ValidateCheckpointInterval(10000) {
		t.Error("ValidateCheckpointInterval should return true for height 10000")
	}
	if ValidateCheckpointInterval(12345) {
		t.Error("ValidateCheckpointInterval should return false for non-interval height")
	}
	next := GetNextCheckpointHeight(5000)
	if next != 10000 {
		t.Errorf("GetNextCheckpointHeight(5000) = %d, want 10000", next)
	}
	prev := GetPreviousCheckpointHeight(15000)
	if prev != 10000 {
		t.Errorf("GetPreviousCheckpointHeight(15000) = %d, want 10000", prev)
	}
	maxHeight := MaxCheckpointHeight()
	if maxHeight == 0 {
		t.Error("MaxCheckpointHeight should return > 0")
	}
	minHeight := MinCheckpointHeight()
	if minHeight != 0 {
		t.Errorf("MinCheckpointHeight should return 0, got %d", minHeight)
	}
	hc, exists := GetHardcodedCheckpoint(10000)
	if !exists {
		t.Error("GetHardcodedCheckpoint should find checkpoint at 10000")
	}
	if hc == nil || hc.Height != 10000 {
		t.Error("GetHardcodedCheckpoint returned wrong checkpoint")
	}
	allHc := GetAllHardcodedCheckpoints()
	if len(allHc) < MinCheckpoints {
		t.Errorf("GetAllHardcodedCheckpoints returned %d, want at least %d", len(allHc), MinCheckpoints)
	}
}

func TestCreateCheckpointFromBlock(t *testing.T) {
	block := &core.Block{
		Hash:        make([]byte, 32),
		Height:      10000,
		TotalWork:   "1000000",
		Transactions: make([]core.Transaction, 5),
	}
	for i := range block.Hash {
		block.Hash[i] = byte(i)
	}
	block.Header.TimestampUnix = time.Now().Unix()
	cp := CreateCheckpointFromBlock(block, "state_root", 10000)
	if cp == nil {
		t.Fatal("CreateCheckpointFromBlock returned nil")
	}
	if cp.Height != 10000 {
		t.Errorf("expected height 10000, got %d", cp.Height)
	}
	if cp.TxCount != 5 {
		t.Errorf("expected tx count 5, got %d", cp.TxCount)
	}
	if cp.TotalWork != "1000000" {
		t.Errorf("expected total work 1000000, got %s", cp.TotalWork)
	}
	cpNil := CreateCheckpointFromBlock(nil, "state_root", 10000)
	if cpNil != nil {
		t.Error("CreateCheckpointFromBlock should return nil for nil block")
	}
}

func TestMerkleRootComputation(t *testing.T) {
	leaves := [][]byte{
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 1},
		{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 1, 2},
	}
	root := computeMerkleRoot(leaves)
	if root == nil {
		t.Fatal("computeMerkleRoot returned nil")
	}
	if len(root) != 32 {
		t.Errorf("expected merkle root length 32, got %d", len(root))
	}
	root2 := computeMerkleRoot(leaves)
	for i := range root {
		if root[i] != root2[i] {
			t.Errorf("merkle root not deterministic: %d != %d", root[i], root2[i])
		}
	}
	rootSingle := computeMerkleRoot(leaves[:1])
	if rootSingle == nil || len(rootSingle) != 32 {
		t.Error("computeMerkleRoot failed for single leaf")
	}
	rootEmpty := computeMerkleRoot(nil)
	if rootEmpty != nil {
		t.Error("computeMerkleRoot should return nil for empty input")
	}
}

func TestCheckpointManager_ConcurrentAccess(t *testing.T) {
	cm := NewCheckpointManager()
	pubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pubKeyHex := hex.EncodeToString(pubKey)
	err = cm.AddTrustedValidator(pubKeyHex)
	if err != nil {
		t.Fatalf("AddTrustedValidator failed: %v", err)
	}
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			cp, _ := cm.GetLatestCheckpoint()
			if cp != nil {
				_ = cm.VerifyCheckpoint(cp)
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
