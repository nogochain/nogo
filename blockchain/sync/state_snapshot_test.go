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
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

func TestStateSnapshot_BasicValidation(t *testing.T) {
	snapshot := &StateSnapshot{
		Version:       SnapshotVersion,
		TotalAccounts: 0,
		AccountStates: make([]AccountState, 0),
	}
	err := snapshot.ValidateBasic()
	if err == nil {
		t.Error("ValidateBasic should fail for nil checkpoint")
	}
	snapshot.Checkpoint = &Checkpoint{
		Version:   CheckpointVersion,
		Height:    10000,
		BlockHash: make([]byte, 32),
		StateRoot: "state_root",
		Timestamp: time.Now().Unix(),
	}
	err = snapshot.ValidateBasic()
	if err == nil {
		t.Error("ValidateBasic should fail for invalid validator length")
	}
	snapshot.Validator = make([]byte, PubKeySize)
	err = snapshot.ValidateBasic()
	if err == nil {
		t.Error("ValidateBasic should fail for invalid signature length")
	}
	snapshot.Signature = make([]byte, SignatureSize)
	err = snapshot.ValidateBasic()
	if err != nil {
		t.Errorf("ValidateBasic failed: %v", err)
	}
}

func TestStateSnapshot_CreateAndVerify(t *testing.T) {
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
	accounts := map[string]*core.Account{
		"addr1": {Balance: 1000, Nonce: 1},
		"addr2": {Balance: 2000, Nonce: 2},
		"addr3": {Balance: 3000, Nonce: 3},
	}
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	signFunc := func(hash []byte) ([]byte, error) {
		return ed25519.Sign(privKey, hash), nil
	}
	snapshot, err := CreateStateSnapshot(checkpoint, accounts, pubKey, signFunc)
	if err != nil {
		t.Fatalf("CreateStateSnapshot failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("CreateStateSnapshot returned nil")
	}
	if snapshot.Version != SnapshotVersion {
		t.Errorf("expected version %d, got %d", SnapshotVersion, snapshot.Version)
	}
	if snapshot.TotalAccounts != 3 {
		t.Errorf("expected 3 accounts, got %d", snapshot.TotalAccounts)
	}
	downloader := NewSnapshotDownloader(NewCheckpointManager())
	err = downloader.VerifySnapshot(snapshot)
	if err != nil {
		t.Errorf("VerifySnapshot failed: %v", err)
	}
}

func TestStateSnapshot_EncodeDecode(t *testing.T) {
	checkpoint := &Checkpoint{
		Version:   CheckpointVersion,
		Height:    10000,
		BlockHash: make([]byte, 32),
		StateRoot: "state_root",
		Timestamp: time.Now().Unix(),
	}
	snapshot := &StateSnapshot{
		Version:    SnapshotVersion,
		Checkpoint: checkpoint,
		AccountStates: []AccountState{
			{Address: "addr1", Balance: 1000, Nonce: 1},
			{Address: "addr2", Balance: 2000, Nonce: 2},
		},
		StateRoot:     "computed_root",
		Timestamp:     time.Now().Unix(),
		Validator:     make([]byte, PubKeySize),
		Signature:     make([]byte, SignatureSize),
		MerkleRoot:    make([]byte, 32),
		TotalAccounts: 2,
		BlockSize:     1024,
	}
	for i := range snapshot.Validator {
		snapshot.Validator[i] = byte(i)
	}
	for i := range snapshot.Signature {
		snapshot.Signature[i] = byte(i)
	}
	for i := range snapshot.MerkleRoot {
		snapshot.MerkleRoot[i] = byte(i)
	}
	data, err := EncodeSnapshot(snapshot)
	if err != nil {
		t.Fatalf("EncodeSnapshot failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("EncodeSnapshot returned empty data")
	}
	decoded, err := DecodeSnapshot(data)
	if err != nil {
		t.Fatalf("DecodeSnapshot failed: %v", err)
	}
	if decoded.Version != snapshot.Version {
		t.Errorf("version mismatch: %d != %d", decoded.Version, snapshot.Version)
	}
	if decoded.TotalAccounts != snapshot.TotalAccounts {
		t.Errorf("total accounts mismatch: %d != %d", decoded.TotalAccounts, snapshot.TotalAccounts)
	}
	if len(decoded.AccountStates) != len(snapshot.AccountStates) {
		t.Errorf("account count mismatch: %d != %d", len(decoded.AccountStates), len(snapshot.AccountStates))
	}
	for i := range snapshot.AccountStates {
		if decoded.AccountStates[i].Address != snapshot.AccountStates[i].Address {
			t.Errorf("account %d address mismatch", i)
		}
		if decoded.AccountStates[i].Balance != snapshot.AccountStates[i].Balance {
			t.Errorf("account %d balance mismatch", i)
		}
		if decoded.AccountStates[i].Nonce != snapshot.AccountStates[i].Nonce {
			t.Errorf("account %d nonce mismatch", i)
		}
	}
}

func TestStateSnapshot_DecodeFailures(t *testing.T) {
	_, err := DecodeSnapshot(nil)
	if err == nil {
		t.Error("DecodeSnapshot should fail for nil data")
	}
	_, err = DecodeSnapshot([]byte{99})
	if err == nil {
		t.Error("DecodeSnapshot should fail for invalid version")
	}
	data := []byte{SnapshotVersion}
	_, err = DecodeSnapshot(data)
	if err == nil {
		t.Error("DecodeSnapshot should fail for truncated data")
	}
}

func TestSnapshotDownloader_Basic(t *testing.T) {
	checkpointMgr := NewCheckpointManager()
	downloader := NewSnapshotDownloader(checkpointMgr)
	if downloader == nil {
		t.Fatal("NewSnapshotDownloader returned nil")
	}
	if downloader.GetProgress() != 0 {
		t.Error("initial progress should be 0")
	}
	if downloader.IsDownloading() {
		t.Error("should not be downloading initially")
	}
	downloader.SetMinPeers(5)
	downloader.SetTimeout(10 * time.Second)
}

func TestSnapshotDownloader_VerifySnapshotFailures(t *testing.T) {
	downloader := NewSnapshotDownloader(NewCheckpointManager())
	tests := []struct {
		name        string
		snapshot    *StateSnapshot
		expectError bool
	}{
		{
			name:        "nil snapshot",
			snapshot:    nil,
			expectError: true,
		},
		{
			name: "wrong version",
			snapshot: &StateSnapshot{
				Version:       99,
				TotalAccounts: 0,
				AccountStates: make([]AccountState, 0),
			},
			expectError: true,
		},
		{
			name: "missing checkpoint",
			snapshot: &StateSnapshot{
				Version:       SnapshotVersion,
				TotalAccounts: 0,
				AccountStates: make([]AccountState, 0),
			},
			expectError: true,
		},
		{
			name: "invalid validator length",
			snapshot: &StateSnapshot{
				Version:       SnapshotVersion,
				Checkpoint:    &Checkpoint{Version: CheckpointVersion, Height: 10000, BlockHash: make([]byte, 32), StateRoot: "root", Timestamp: time.Now().Unix()},
				Validator:     make([]byte, 10),
				TotalAccounts: 0,
				AccountStates: make([]AccountState, 0),
			},
			expectError: true,
		},
		{
			name: "invalid signature length",
			snapshot: &StateSnapshot{
				Version:       SnapshotVersion,
				Checkpoint:    &Checkpoint{Version: CheckpointVersion, Height: 10000, BlockHash: make([]byte, 32), StateRoot: "root", Timestamp: time.Now().Unix()},
				Validator:     make([]byte, PubKeySize),
				Signature:     make([]byte, 10),
				TotalAccounts: 0,
				AccountStates: make([]AccountState, 0),
			},
			expectError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := downloader.VerifySnapshot(tt.snapshot)
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestStateSnapshot_AccountQueries(t *testing.T) {
	snapshot := &StateSnapshot{
		Version:    SnapshotVersion,
		Checkpoint: &Checkpoint{Version: CheckpointVersion, Height: 10000, BlockHash: make([]byte, 32), StateRoot: "root", Timestamp: time.Now().Unix()},
		AccountStates: []AccountState{
			{Address: "addr1", Balance: 1000, Nonce: 1},
			{Address: "addr2", Balance: 2000, Nonce: 2},
			{Address: "addr3", Balance: 3000, Nonce: 3},
		},
		TotalAccounts: 3,
	}
	balance, exists := snapshot.GetAccountBalance("addr1")
	if !exists {
		t.Error("addr1 should exist")
	}
	if balance != 1000 {
		t.Errorf("expected balance 1000, got %d", balance)
	}
	_, exists = snapshot.GetAccountBalance("nonexistent")
	if exists {
		t.Error("nonexistent address should not exist")
	}
	nonce, exists := snapshot.GetAccountNonce("addr2")
	if !exists {
		t.Error("addr2 should exist")
	}
	if nonce != 2 {
		t.Errorf("expected nonce 2, got %d", nonce)
	}
	acc, exists := snapshot.GetAccount("addr3")
	if !exists {
		t.Error("addr3 should exist")
	}
	if acc == nil || acc.Balance != 3000 {
		t.Error("addr3 balance incorrect")
	}
}

func TestStateSnapshot_MerkleRootComputation(t *testing.T) {
	snapshot := &StateSnapshot{
		Version:    SnapshotVersion,
		Checkpoint: &Checkpoint{Version: CheckpointVersion, Height: 10000, BlockHash: make([]byte, 32), StateRoot: "root", Timestamp: time.Now().Unix()},
		AccountStates: []AccountState{
			{Address: "addr1", Balance: 1000, Nonce: 1},
			{Address: "addr2", Balance: 2000, Nonce: 2},
			{Address: "addr3", Balance: 3000, Nonce: 3},
		},
		TotalAccounts: 3,
	}
	downloader := NewSnapshotDownloader(NewCheckpointManager())
	computedRoot := downloader.computeSnapshotMerkleRoot(snapshot)
	if computedRoot == nil {
		t.Fatal("computeSnapshotMerkleRoot returned nil")
	}
	snapshot.MerkleRoot = computedRoot
	computedStateRoot := downloader.computeStateRoot(snapshot)
	if computedStateRoot == "" {
		t.Error("computeStateRoot returned empty string")
	}
	snapshot.StateRoot = computedStateRoot
	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	hashToSign := downloader.computeSnapshotHash(snapshot)
	signature := ed25519.Sign(privKey, hashToSign)
	snapshot.Validator = pubKey
	snapshot.Signature = signature
	err := downloader.VerifySnapshot(snapshot)
	if err != nil {
		t.Errorf("VerifySnapshot failed after proper setup: %v", err)
	}
}

func TestSnapshotDownloader_AccountProof(t *testing.T) {
	snapshot := &StateSnapshot{
		Version:    SnapshotVersion,
		Checkpoint: &Checkpoint{Version: CheckpointVersion, Height: 10000, BlockHash: make([]byte, 32), StateRoot: "root", Timestamp: time.Now().Unix()},
		AccountStates: []AccountState{
			{Address: "addr1", Balance: 1000, Nonce: 1},
			{Address: "addr2", Balance: 2000, Nonce: 2},
		},
		TotalAccounts: 2,
	}
	downloader := NewSnapshotDownloader(NewCheckpointManager())
	proof, err := downloader.GetAccountProof(snapshot, "addr1")
	if err != nil {
		t.Fatalf("GetAccountProof failed: %v", err)
	}
	err = downloader.VerifyAccountProof(snapshot, "addr1", proof, 1000)
	if err != nil {
		t.Errorf("VerifyAccountProof failed: %v", err)
	}
	err = downloader.VerifyAccountProof(snapshot, "addr1", proof, 999)
	if err == nil {
		t.Error("VerifyAccountProof should fail for wrong balance")
	}
	_, err = downloader.GetAccountProof(snapshot, "nonexistent")
	if err == nil {
		t.Error("GetAccountProof should fail for nonexistent account")
	}
}

func TestStateSnapshot_JSONMarshaling(t *testing.T) {
	snapshot := &StateSnapshot{
		Version:    SnapshotVersion,
		Checkpoint: &Checkpoint{Version: CheckpointVersion, Height: 10000, BlockHash: make([]byte, 32), StateRoot: "root", Timestamp: time.Now().Unix()},
		AccountStates: []AccountState{
			{Address: "addr1", Balance: 1000, Nonce: 1},
		},
		Validator:     make([]byte, PubKeySize),
		Signature:     make([]byte, SignatureSize),
		MerkleRoot:    make([]byte, 32),
		TotalAccounts: 1,
	}
	for i := range snapshot.Validator {
		snapshot.Validator[i] = byte(i)
	}
	data, err := snapshot.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("MarshalJSON returned empty data")
	}
}

func TestStateSnapshot_Size(t *testing.T) {
	snapshot := &StateSnapshot{
		Version:    SnapshotVersion,
		Checkpoint: &Checkpoint{Version: CheckpointVersion, Height: 10000, BlockHash: make([]byte, 32), StateRoot: "root", Timestamp: time.Now().Unix()},
		AccountStates: []AccountState{
			{Address: "addr1", Balance: 1000, Nonce: 1},
			{Address: "addr2", Balance: 2000, Nonce: 2},
		},
		TotalAccounts: 2,
	}
	size := snapshot.Size()
	if size == 0 {
		t.Error("Size returned 0")
	}
}

func TestCreateStateSnapshot_Validation(t *testing.T) {
	_, err := CreateStateSnapshot(nil, nil, nil, nil)
	if err == nil {
		t.Error("CreateStateSnapshot should fail for nil checkpoint")
	}
	checkpoint := &Checkpoint{Version: CheckpointVersion, Height: 10000, BlockHash: make([]byte, 32), StateRoot: "root", Timestamp: time.Now().Unix()}
	_, err = CreateStateSnapshot(checkpoint, nil, make([]byte, 10), nil)
	if err == nil {
		t.Error("CreateStateSnapshot should fail for invalid key length")
	}
}
