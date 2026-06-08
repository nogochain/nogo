package core

import (
	"testing"
)

// TestCalculateStateRootEmptyState verifies state root calculation with empty state.
func TestCalculateStateRootEmptyState(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()

	// Empty state should produce deterministic root
	root1, err := store.CalculateStateRoot(nil)
	if err == nil {
		t.Error("expected error for nil state")
	}

	root2, err := store.CalculateStateRoot(map[string]Account{})
	if err != nil {
		t.Fatalf("unexpected error for empty state: %v", err)
	}
	if len(root2) != 32 {
		t.Errorf("expected 32-byte root, got %d bytes", len(root2))
	}

	// Same empty state should produce same root
	root3, err := store.CalculateStateRoot(map[string]Account{})
	if err != nil {
		t.Fatalf("unexpected error for empty state: %v", err)
	}
	for i := 0; i < 32; i++ {
		if root2[i] != root3[i] {
			t.Error("empty state root not deterministic")
			break
		}
	}
}

// TestCalculateStateRootSingleAccount verifies state root with one account.
func TestCalculateStateRootSingleAccount(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()

	state := map[string]Account{
		"NOGO111111111111111111111111111111111111": {
			Balance: 1000000,
			Nonce:   5,
		},
	}

	root, err := store.CalculateStateRoot(state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(root) != 32 {
		t.Errorf("expected 32-byte root, got %d bytes", len(root))
	}
}

// TestCalculateStateRootDeterministic verifies state root is deterministic (sorted by address).
func TestCalculateStateRootDeterministic(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()

	// Two accounts in different insertion order
	state1 := map[string]Account{
		"NOGO111111111111111111111111111111111111": {Balance: 100, Nonce: 1},
		"NOGO222222222222222222222222222222222222": {Balance: 200, Nonce: 2},
	}
	state2 := map[string]Account{
		"NOGO222222222222222222222222222222222222": {Balance: 200, Nonce: 2},
		"NOGO111111111111111111111111111111111111": {Balance: 100, Nonce: 1},
	}

	root1, err := store.CalculateStateRoot(state1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	root2, err := store.CalculateStateRoot(state2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 32; i++ {
		if root1[i] != root2[i] {
			t.Error("state root not deterministic across insertion orders")
			break
		}
	}
}

// TestCalculateStateRootChangesWhenBalanceChanges verifies root changes when balance changes.
func TestCalculateStateRootChangesWhenBalanceChanges(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()

	addr := "NOGO111111111111111111111111111111111111"
	state1 := map[string]Account{addr: {Balance: 100, Nonce: 1}}
	state2 := map[string]Account{addr: {Balance: 200, Nonce: 1}}

	root1, err := store.CalculateStateRoot(state1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	root2, err := store.CalculateStateRoot(state2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	same := true
	for i := 0; i < 32; i++ {
		if root1[i] != root2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("state root should change when balance changes")
	}
}

// TestCalculateStateRootChangesWhenNonceChanges verifies root changes when nonce changes.
func TestCalculateStateRootChangesWhenNonceChanges(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()

	addr := "NOGO111111111111111111111111111111111111"
	state1 := map[string]Account{addr: {Balance: 100, Nonce: 1}}
	state2 := map[string]Account{addr: {Balance: 100, Nonce: 2}}

	root1, err := store.CalculateStateRoot(state1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	root2, err := store.CalculateStateRoot(state2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	same := true
	for i := 0; i < 32; i++ {
		if root1[i] != root2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("state root should change when nonce changes")
	}
}

// TestBlockHeaderStateRootField verifies BlockHeader.StateRoot field is correctly set.
func TestBlockHeaderStateRootField(t *testing.T) {
	header := BlockHeader{
		Version:        1,
		PrevHash:       make([]byte, 32),
		TimestampUnix:  1234567890,
		DifficultyBits: 18,
		Nonce:          12345,
		StateRoot:      make([]byte, 32),
		MerkleRoot:     make([]byte, 32),
		Height:         100,
		MinerAddress:   "NOGO111111111111111111111111111111111111",
	}

	// StateRoot should be 32 bytes
	if len(header.StateRoot) != 32 {
		t.Errorf("expected StateRoot length 32, got %d", len(header.StateRoot))
	}

	// MerkleRoot should be 32 bytes
	if len(header.MerkleRoot) != 32 {
		t.Errorf("expected MerkleRoot length 32, got %d", len(header.MerkleRoot))
	}
}

// newTestBoltStore creates a temporary BoltStore for testing.
func newTestBoltStore(t *testing.T) *storage.BoltStore {
	t.Helper()
	path := t.TempDir() + "/test.db"
	store, err := storage.NewBoltStore(path)
	if err != nil {
		t.Fatalf("failed to create BoltStore: %v", err)
	}
	return store
}
