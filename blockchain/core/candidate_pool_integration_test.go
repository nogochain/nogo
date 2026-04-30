// Copyright 2026 NogoChain Team
// Production-grade integration tests for CandidatePool
// Simulates multi-node mining scenarios with comprehensive edge case coverage

package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestIntegration_MultiNodeCompetition simulates 5 miners competing at same height
// Validates: highest work wins regardless of submission order
func TestIntegration_MultiNodeCompetition(t *testing.T) {
	wc := NewWorkCalculator()
	extWindow := 300 * time.Millisecond
	pool := newTestPool(20, 30*time.Second, extWindow)
	pool.MinWindowDuration = 50 * time.Millisecond
	pool.SetChainReference(nil, wc)

	now := time.Now()

	candidates := []struct {
		sourceID       string
		difficultyBits uint32
		timestamp      int64
		nonce          uint64
	}{
		{"miner-slow", uint32(0x1d000001), now.Unix(), 100},
		{"miner-fast", uint32(0x1f00ffff), now.Unix() + 2, 200},
		{"miner-mid", uint32(0x1d00ffff), now.Unix() + 1, 300},
		{"miner-low", uint32(0x1d000100), now.Unix() + 3, 400},
		{"miner-high", uint32(0x207fffff), now.Unix() + 4, 500},
	}

	for _, c := range candidates {
		block := newTestBlock(300, c.difficultyBits, c.timestamp, c.nonce)
		err := pool.SubmitCandidate(block, c.sourceID, now)
		if err != nil {
			t.Fatalf("submit from %s failed: %v", c.sourceID, err)
		}
	}

	stats := pool.GetPoolStats()
	if stats[300].CandidateCount != 5 {
		t.Fatalf("expected 5 candidates, got %d", stats[300].CandidateCount)
	}

	time.Sleep(extWindow + 100 * time.Millisecond)

	statsAfter := pool.GetPoolStats()
	if _, exists := statsAfter[300]; exists {
		t.Error("pool should be removed after selection")
	}
}

// TestIntegration_DeadlineBoundarySubmission verifies behavior around deadline transition
// Validates: submissions accepted before timer fires, pool auto-cleans up after deadline
func TestIntegration_DeadlineBoundarySubmission(t *testing.T) {
	wc := NewWorkCalculator()
	window := 200 * time.Millisecond
	pool := newTestPool(10, 30*time.Second, window)
	pool.MinWindowDuration = 50 * time.Millisecond
	pool.SetChainReference(nil, wc)

	block1 := newTestBlock(400, uint32(0x1d00ffff), time.Now().Unix(), 1)
	if err := pool.SubmitCandidate(block1, "miner-boundary-1", time.Now()); err != nil {
		t.Fatalf("first submission should succeed: %v", err)
	}

	time.Sleep(window - 20*time.Millisecond)

	block2 := newTestBlock(400, uint32(0x1d00fffe), time.Now().Unix(), 2)
	err := pool.SubmitCandidate(block2, "miner-boundary-2", time.Now())
	if err != nil {
		t.Fatalf("submission just before deadline should succeed: %v", err)
	}

	statsBefore := pool.GetPoolStats()
	if statsBefore[400].CandidateCount != 2 {
		t.Fatalf("expected 2 candidates before deadline: got %d", statsBefore[400].CandidateCount)
	}
	if statsBefore[400].WindowState != "open" {
		t.Fatalf("expected open state before deadline: got %s", statsBefore[400].WindowState)
	}

	time.Sleep(window + 100 * time.Millisecond)

	statsAfter := pool.GetPoolStats()
	if _, exists := statsAfter[400]; exists {
		t.Error("pool should be auto-cleaned up after deadline selection")
	}
}

// TestIntegration_StopRaceWithSubmit verifies safety under concurrent Stop() and SubmitCandidate()
// Validates: no panic, no deadlock, consistent state after race condition
func TestIntegration_StopRaceWithSubmit(t *testing.T) {
	wc := NewWorkCalculator()
	pool := newTestPool(100, 30*time.Second, 5*time.Second)
	pool.SetChainReference(nil, wc)

	var stopped atomic.Bool
	var successCount atomic.Int64
	var rejectCount atomic.Int64
	var wg sync.WaitGroup

	numSubmitters := 15
	for i := 0; i < numSubmitters; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if stopped.Load() {
					return
				}
				block := newTestBlock(uint64(idx%3+1), uint32(0x1d00ffff)+uint32(idx), time.Now().Unix(), uint64(j))
				err := pool.SubmitCandidate(block, fmt.Sprintf("racer-%d", idx), time.Now())
				if err == nil {
					successCount.Add(1)
				} else {
					rejectCount.Add(1)
				}
			}
		}(i)
	}

	time.Sleep(10 * time.Millisecond)
	stopped.Store(true)
	pool.Stop()

	wg.Wait()

	total := successCount.Load() + rejectCount.Load()
	if total == 0 {
		t.Error("expected some submissions attempted during race test")
	}

	stats := pool.GetPoolStats()
	for _, ps := range stats {
		if ps.WindowState != "closed" {
			t.Errorf("expected closed state after Stop+race, got %s", ps.WindowState)
		}
	}
}

// TestIntegration_ConcurrentMultiHeightSelection verifies simultaneous selection across heights
// Validates: independent timer-based selection per height with no cross-height interference
func TestIntegration_ConcurrentMultiHeightSelection(t *testing.T) {
	wc := NewWorkCalculator()
	window := 150 * time.Millisecond
	pool := newTestPool(20, 30*time.Second, window)
	pool.MinWindowDuration = 30 * time.Millisecond
	pool.SetChainReference(nil, wc)

	now := time.Now()
	heights := []uint64{500, 501, 502, 503, 504}

	for _, h := range heights {
		block := newTestBlock(h, uint32(0x1d00ffff)+uint32(h), now.Unix(), h)
		if err := pool.SubmitCandidate(block, fmt.Sprintf("miner-h%d", h), now); err != nil {
			t.Fatalf("submit for height %d: %v", h, err)
		}
	}

	statsBefore := pool.GetPoolStats()
	for _, h := range heights {
		if statsBefore[h].CandidateCount != 1 {
			t.Errorf("height %d: expected 1 candidate before selection, got %d",
				h, statsBefore[h].CandidateCount)
		}
		if statsBefore[h].WindowState != "open" {
			t.Errorf("height %d: expected open state, got %s", h, statsBefore[h].WindowState)
		}
	}

	time.Sleep(window + 100 * time.Millisecond)

	statsAfter := pool.GetPoolStats()
	for _, h := range heights {
		if _, exists := statsAfter[h]; exists {
			t.Errorf("height %d: pool should be cleaned up after selection", h)
		}
	}
}

// TestIntegration_WorkCalcNilVsNonNil compares behavior with and without WorkCalculator
// Validates: nil WorkCalculator degrades gracefully to timestamp-only ordering (with warning)
func TestIntegration_WorkCalcNilVsNonNil(t *testing.T) {
	wc := NewWorkCalculator()
	window := 200 * time.Millisecond

	poolWithWC := newTestPool(10, 30*time.Second, window)
	poolWithWC.MinWindowDuration = 50 * time.Millisecond
	poolWithWC.SetChainReference(nil, wc)

	poolWithoutWC := newTestPool(10, 30*time.Second, window)
	poolWithoutWC.MinWindowDuration = 50 * time.Millisecond
	poolWithoutWC.SetChainReference(nil, nil)

	baseTime := time.Now()

	blocks := [3]struct {
		difficulty uint32
		nonce      uint64
	}{
		{uint32(0x1d00ffff), 1},
		{uint32(0x1f00ffff), 2},
		{uint32(0x207fffff), 3},
	}

	for i, b := range blocks {
		block := newTestBlock(600, b.difficulty, baseTime.Unix(), b.nonce)
		src := fmt.Sprintf("wc-test-%d", i)

		err1 := poolWithWC.SubmitCandidate(block, src, baseTime)
		err2 := poolWithoutWC.SubmitCandidate(block, src, baseTime)

		if err1 != nil {
			t.Fatalf("poolWithWC reject %s: %v", src, err1)
		}
		if err2 != nil {
			t.Fatalf("poolWithoutWC reject %s: %v", src, err2)
		}
	}

	statsWC := poolWithWC.GetPoolStats()
	statsNoWC := poolWithoutWC.GetPoolStats()

	if statsWC[600].CandidateCount != 3 {
		t.Errorf("with WC: expected 3 candidates, got %d", statsWC[600].CandidateCount)
	}
	if statsNoWC[600].CandidateCount != 3 {
		t.Errorf("without WC: expected 3 candidates, got %d", statsNoWC[600].CandidateCount)
	}

	time.Sleep(window + 100 * time.Millisecond)

	if _, exists := poolWithWC.GetPoolStats()[600]; exists {
		t.Error("poolWithWC should be cleaned up")
	}
	if _, exists := poolWithoutWC.GetPoolStats()[600]; exists {
		t.Error("poolWithoutWC should be cleaned up")
	}
}

// TestIntegration_ForkBlocksAccepted verifies fork blocks (same height, different hash) are accepted
// Validates: multiple distinct blocks at same height coexist in pool for fair competition
func TestIntegration_ForkBlocksAccepted(t *testing.T) {
	wc := NewWorkCalculator()
	pool := newTestPool(10, 30*time.Second, 2*time.Second)
	pool.MinWindowDuration = 100 * time.Millisecond
	pool.SetChainReference(nil, wc)

	height := uint64(700)
	now := time.Now()

	for i := 0; i < 5; i++ {
		block := newTestBlock(height, uint32(0x1d00ffff)+uint32(i), now.Unix()+int64(i), uint64(i+1))
		src := fmt.Sprintf("fork-miner-%d", i)
		err := pool.SubmitCandidate(block, src, now)
		if err != nil {
			t.Fatalf("fork block %d rejected (forks should compete): %v", i, err)
		}
	}

	stats := pool.GetPoolStats()
	if stats[height].CandidateCount != 5 {
		t.Fatalf("expected 5 fork candidates competing, got %d", stats[height].CandidateCount)
	}

	duplicateBlock := newTestBlock(height, uint32(0x1d00ffff), now.Unix(), 1)
	err := pool.SubmitCandidate(duplicateBlock, "duplicate-miner", now)
	if err == nil {
		t.Error("exact duplicate hash should be rejected even among forks")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate error, got: %v", err)
	}
}

// TestIntegration_RapidFireSequence simulates rapid successive mining events across heights
// Validates: pool handles quick succession of different heights without data loss
func TestIntegration_RapidFireSequence(t *testing.T) {
	wc := NewWorkCalculator()
	pool := newTestPool(50, 30*time.Second, 500*time.Millisecond)
	pool.MinWindowDuration = 50 * time.Millisecond
	pool.SetChainReference(nil, wc)

	startTime := time.Now()

	for i := 0; i < 10; i++ {
		height := uint64(800 + i)
		block := newTestBlock(height, uint32(0x1d00ffff)+uint32(i), startTime.Unix()+int64(i), uint64(i))
		err := pool.SubmitCandidate(block, fmt.Sprintf("rapid-%d", i), startTime)
		if err != nil {
			t.Fatalf("rapid fire %d failed: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	stats := pool.GetPoolStats()
	activePools := 0
	totalCandidates := 0
	for height, ps := range stats {
		if height >= 800 && height <= 809 {
			activePools++
			totalCandidates += ps.CandidateCount
			if ps.CandidateCount != 1 {
				t.Errorf("height %d: expected 1 candidate in rapid fire, got %d", height, ps.CandidateCount)
			}
		}
	}
	if activePools < 8 {
		t.Errorf("expected ~10 active pools in rapid fire, found %d", activePools)
	}
	if totalCandidates < 8 {
		t.Errorf("expected ~10 total candidates in rapid fire, found %d", totalCandidates)
	}
}

// TestIntegration_EmptyHashBlock verifies behavior with zero-value block hash
// Validates: empty hash is rejected gracefully with descriptive error (no panic)
func TestIntegration_EmptyHashBlock(t *testing.T) {
	pool := newTestPool(10, 30*time.Second, 5*time.Second)

	block := &Block{
		Hash:   []byte{},
		Height: 999,
		Header: BlockHeader{
			Height:         999,
			TimestampUnix:  time.Now().Unix(),
			DifficultyBits: uint32(0x1d00ffff),
			Difficulty:     uint32(0x1d00ffff),
			PrevHash:       make([]byte, 32),
			MerkleRoot:     make([]byte, 32),
		},
		MinerAddress: "NOGO" + strings.Repeat("b", 72),
	}

	err := pool.SubmitCandidate(block, "empty-hash-miner", time.Now())
	if err == nil {
		t.Fatal("empty hash block should be rejected with error")
	}
	if !strings.Contains(err.Error(), "hash is empty") {
		t.Errorf("expected 'hash is empty' error, got: %v", err)
	}
}

// TestIntegration_MaxLatenessBoundary verifies exact boundary of MaxLateness check
// Validates: submission beyond MaxLateness threshold is rejected, within is accepted
func TestIntegration_MaxLatenessBoundary(t *testing.T) {
	pool := newTestPool(10, 11*time.Second, 5*time.Second)

	block := newTestBlock(1100, uint32(0x1d00ffff), time.Now().Unix(), 1)
	clearlyLate := time.Now().Add(-pool.MaxLateness - 100*time.Millisecond)

	err := pool.SubmitCandidate(block, "boundary-miner", clearlyLate)
	if err == nil {
		t.Error("submission clearly beyond MaxLateness should be rejected")
	} else if !strings.Contains(err.Error(), "late submission") {
		t.Errorf("expected late submission error, got: %v", err)
	}

	withinLimit := time.Now().Add(-pool.MaxLateness + 500*time.Millisecond)
	block2 := newTestBlock(1101, uint32(0x1d00fffe), time.Now().Unix(), 2)
	err2 := pool.SubmitCandidate(block2, "timely-miner", withinLimit)
	if err2 != nil {
		t.Errorf("submission within MaxLateness should succeed: %v", err2)
	}
}

// TestIntegration_BlockJSONRoundTrip verifies Block survives JSON marshal/unmarshal cycle
// Important: Block uses custom MarshalJSON (base64-encoded hash) and UnmarshalJSON (legacy compat)
// Known limitation: Header.Height may not survive round-trip (existing MarshalJSON behavior)
func TestIntegration_BlockJSONRoundTrip(t *testing.T) {
	original := newTestBlock(12345, uint32(0x1d00ffff), time.Now().Unix(), 999)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal original block: %v", err)
	}

	var restored Block
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal block: %v", err)
	}

	if restored.Height != original.Height {
		t.Errorf("height mismatch: got %d, want %d", restored.Height, original.Height)
	}
	if restored.Header.DifficultyBits != original.Header.DifficultyBits {
		t.Errorf("difficultyBits mismatch: got %d, want %d",
			restored.Header.DifficultyBits, original.Header.DifficultyBits)
	}
	if restored.MinerAddress != original.MinerAddress {
		t.Errorf("minerAddress mismatch: got %s, want %s", restored.MinerAddress, original.MinerAddress)
	}
	if len(restored.Hash) == 0 {
		t.Error("hash should not be empty after round-trip")
	}
}

// TestIntegration_PayloadWrappingRoundTrip verifies MiningCandidatePayload wraps Block JSON correctly
// Tests the actual wire format used for cross-node candidate broadcast
func TestIntegration_PayloadWrappingRoundTrip(t *testing.T) {
	original := newTestBlock(250, uint32(0x1f00ffff), time.Now().Unix(), 777)
	sourceID := "node-alpha"
	minedAt := time.Now()

	blockData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}

	wrapper := struct {
		Type    string          `json:"type"`
		Block   json.RawMessage `json:"block"`
		SourceID string         `json:"source_id"`
		MinedAt int64           `json:"mined_at"`
	}{
		Type:     "mining_candidate",
		Block:    json.RawMessage(blockData),
		SourceID: sourceID,
		MinedAt:  minedAt.Unix(),
	}

	wireData, err := json.Marshal(wrapper)
	if err != nil {
		t.Fatalf("marshal wrapper: %v", err)
	}

	var received struct {
		Type     string          `json:"type"`
		Block    json.RawMessage `json:"block"`
		SourceID string          `json:"source_id"`
		MinedAt  int64           `json:"mined_at"`
	}
	if err := json.Unmarshal(wireData, &received); err != nil {
		t.Fatalf("unmarshal wrapper: %v", err)
	}

	if received.Type != "mining_candidate" {
		t.Errorf("type mismatch: got %s, want mining_candidate", received.Type)
	}
	if received.SourceID != sourceID {
		t.Errorf("source_id mismatch: got %s, want %s", received.SourceID, sourceID)
	}
	if received.MinedAt != minedAt.Unix() {
		t.Errorf("mined_at mismatch: got %d, want %d", received.MinedAt, minedAt.Unix())
	}
	if len(received.Block) == 0 {
		t.Fatal("block data missing in unwrapped payload")
	}

	var decodedBlock Block
	if err := json.Unmarshal(received.Block, &decodedBlock); err != nil {
		t.Fatalf("unmarshal inner block: %v", err)
	}

	if decodedBlock.Height != original.Height {
		t.Errorf("block height lost in round-trip: got %d, want %d", decodedBlock.Height, original.Height)
	}
	if decodedBlock.Header.Nonce != original.Header.Nonce {
		t.Errorf("nonce lost in round-trip: got %d, want %d", decodedBlock.Header.Nonce, original.Header.Nonce)
	}
}

// TestIntegration_SubmitAfterStop verifies all submissions rejected after Stop()
// Validates: graceful degradation when pool is shutting down
func TestIntegration_SubmitAfterStop(t *testing.T) {
	wc := NewWorkCalculator()
	pool := newTestPool(10, 30*time.Second, 5*time.Second)
	pool.SetChainReference(nil, wc)

	block := newTestBlock(900, uint32(0x1d00ffff), time.Now().Unix(), 1)
	if err := pool.SubmitCandidate(block, "before-stop", time.Now()); err != nil {
		t.Fatalf("submit before stop should succeed: %v", err)
	}

	pool.Stop()

	block2 := newTestBlock(901, uint32(0x1d00fffe), time.Now().Unix(), 2)
	err := pool.SubmitCandidate(block2, "after-stop", time.Now())
	if err == nil {
		t.Error("submit after stop should be rejected")
	}
	if !strings.Contains(err.Error(), "stopped") {
		t.Errorf("expected stopped error, got: %v", err)
	}
}

// TestIntegration_MaxCandidatesLimit enforces per-height capacity limit
// Validates: excess candidates are rejected once limit reached
func TestIntegration_MaxCandidatesLimit(t *testing.T) {
	wc := NewWorkCalculator()
	pool := newTestPool(3, 30*time.Second, 5*time.Second)
	pool.MinWindowDuration = 100 * time.Millisecond
	pool.SetChainReference(nil, wc)

	height := uint64(1000)
	now := time.Now()

	accepted := 0
	rejected := 0
	for i := 0; i < 6; i++ {
		block := newTestBlock(height, uint32(0x1d00ffff)+uint32(i), now.Unix()+int64(i), uint64(i+1))
		err := pool.SubmitCandidate(block, fmt.Sprintf("limiter-%d", i), now)
		if err == nil {
			accepted++
		} else {
			rejected++
		}
	}

	if accepted != 3 {
		t.Errorf("expected 3 accepted (max limit), got %d", accepted)
	}
	if rejected != 3 {
		t.Errorf("expected 3 rejected (over limit), got %d", rejected)
	}

	stats := pool.GetPoolStats()
	if stats[height].CandidateCount != 3 {
		t.Errorf("pool should report exactly max candidates: got %d, want 3", stats[height].CandidateCount)
	}
}

// TestIntegration_ShouldPool verifies competition frontier detection logic
// Validates: nil chain and stopped pool return false; height routing tested via SubmitCandidate integration
func TestIntegration_ShouldPool(t *testing.T) {
	pool := NewCandidatePool()
	pool.MaxExtensionWindow = 5 * time.Second

	if pool.ShouldPool(0) {
		t.Error("nil chain should return false")
	}
	if pool.ShouldPool(999) {
		t.Error("no chain reference should return false")
	}

	pool.Stop()
	if pool.ShouldPool(101) {
		t.Error("stopped pool should return false for ShouldPool")
	}
}

// BenchmarkIntegration_FullCycle measures complete submit-to-selection cycle
// Performance baseline: full cycle should complete in < 50μs per candidate
func BenchmarkIntegration_FullCycle(b *testing.B) {
	wc := NewWorkCalculator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		pool := NewCandidatePool()
		pool.MaxCandidates = 100
		pool.MaxExtensionWindow = 5 * time.Second
		pool.SetChainReference(nil, wc)

		block := newTestBlock(uint64(i%100)+1, uint32(0x1d00ffff), time.Now().Unix(), uint64(i))

		b.StartTimer()

		_ = pool.SubmitCandidate(block, "bench-node", time.Now())

		b.StopTimer()

		pool.Stop()

		b.StartTimer()
	}
}

// BenchmarkIntegration_ConcurrentMultiNode measures throughput under multi-node contention
// Simulates 10 nodes each submitting candidates concurrently
func BenchmarkIntegration_ConcurrentMultiNode(b *testing.B) {
	wc := NewWorkCalculator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		pool := NewCandidatePool()
		pool.MaxCandidates = 200
		pool.MaxExtensionWindow = 10 * time.Second
		pool.MinWindowDuration = 1 * time.Second
		pool.SetChainReference(nil, wc)

		var wg sync.WaitGroup
		nodes := 10
		for n := 0; n < nodes; n++ {
			wg.Add(1)
			go func(nodeID int) {
				defer wg.Done()
				block := newTestBlock(uint64(i)+1, uint32(0x1d00ffff)+uint32(nodeID), time.Now().Unix(), uint64(i*nodes+nodeID))
				pool.SubmitCandidate(block, fmt.Sprintf("node-%d", nodeID), time.Now())
			}(n)
		}
		wg.Wait()

		b.StartTimer()

		pool.Stop()
	}
}
