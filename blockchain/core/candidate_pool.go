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

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"sort"
	"sync"
	"time"
)

const (
	defaultTotalInterval      = 30 * time.Second
	defaultMinWindowDuration  = 10 * time.Second
	defaultMaxLateness        = 120 * time.Second
	defaultMaxCandidates      = 50
	defaultMaxExtensionWindow = 30 * time.Second
	defaultPITargetMiningTime = 17 * time.Second
)

type Candidate struct {
	Block       *Block
	SourceID    string
	SubmittedAt time.Time
	MinedAt     time.Time
	Work        *big.Int
	Validated   bool
}

type HeightPool struct {
	Height       uint64
	FirstArrival time.Time
	Deadline     time.Time
	Candidates   []*Candidate
	timer        *time.Timer
	Closed       bool
	mu           sync.Mutex
}

type PoolStats struct {
	CandidateCount int
	WindowState    string
	FirstArrival   time.Time
	Deadline       time.Time
}

type CandidatePool struct {
	pools              map[uint64]*HeightPool
	mu                 sync.RWMutex
	TotalInterval      time.Duration
	MinWindowDuration  time.Duration
	MaxLateness        time.Duration
	MaxCandidates      int
	MaxExtensionWindow time.Duration
	chain              *Chain
	workCalc           *WorkCalculator
	stopped            bool
	stopCh             chan struct{}

	// OnBlockSelected is invoked after selectBest() adds a winning block to the chain.
	// This callback enables the network layer to broadcast the winning block
	// to all peers after the candidate pool competition concludes.
	// Signature: func(block *Block)
	OnBlockSelected func(block *Block)
}

func NewCandidatePool() *CandidatePool {
	return &CandidatePool{
		pools:              make(map[uint64]*HeightPool),
		TotalInterval:      defaultTotalInterval,
		MinWindowDuration:  defaultMinWindowDuration,
		MaxLateness:        defaultMaxLateness,
		MaxCandidates:      defaultMaxCandidates,
		MaxExtensionWindow: defaultMaxExtensionWindow,
		stopCh:             make(chan struct{}),
	}
}

func (p *CandidatePool) SetChainReference(c *Chain, wc *WorkCalculator) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return
	}
	p.chain = c
	p.workCalc = wc
}

func (p *CandidatePool) SubmitCandidate(block *Block, sourceID string, minedAt time.Time) error {
	p.mu.RLock()
	if p.stopped {
		p.mu.RUnlock()
		return fmt.Errorf("candidate pool: pool is stopped")
	}
	p.mu.RUnlock()

	if block == nil {
		return fmt.Errorf("candidate pool: nil block")
	}

	if len(block.Hash) == 0 {
		return fmt.Errorf("candidate pool: block hash is empty for height %d", block.Header.Height)
	}

	blockHash := hex.EncodeToString(block.Hash)
	height := block.Header.Height
	if height == 0 && block.Height != 0 {
		log.Printf("[CandidatePool] WARNING: Header.Height=0, using Block.Height=%d as fallback", block.Height)
		height = block.Height
	}

	submissionDelay := time.Since(minedAt)
	if submissionDelay > p.MaxLateness {
		log.Printf("[CandidatePool] WARNING: late submission from %s for height %d: delay %v (max %v)",
			sourceID, height, submissionDelay, p.MaxLateness)
		return fmt.Errorf("candidate pool: late submission for height %d: delay %v exceeds max %v",
			height, submissionDelay, p.MaxLateness)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return fmt.Errorf("candidate pool: pool is stopped (re-check after write lock)")
	}

	pool, exists := p.pools[height]
	if exists {
		pool.mu.Lock()

		if pool.Closed || time.Now().After(pool.Deadline) {
			pool.mu.Unlock()
			return fmt.Errorf("candidate pool: window closed for height %d", height)
		}

		for _, c := range pool.Candidates {
			if c != nil && c.Block != nil && hex.EncodeToString(c.Block.Hash) == blockHash {
				pool.mu.Unlock()
				return fmt.Errorf("candidate pool: duplicate candidate %s for height %d", blockHash[:16], height)
			}
			if c.SourceID == sourceID {
				pool.mu.Unlock()
				return fmt.Errorf("candidate pool: source %s already submitted for height %d (keeping existing)", sourceID[:min(12, len(sourceID))], height)
			}
		}

		if len(pool.Candidates) >= p.MaxCandidates {
			pool.mu.Unlock()
			return fmt.Errorf("candidate pool: pool full for height %d (max %d)", height, p.MaxCandidates)
		}
	} else {
		pool = &HeightPool{
			Height:     height,
			Candidates: make([]*Candidate, 0, p.MaxCandidates),
		}
		pool.mu.Lock()
		p.pools[height] = pool

		now := time.Now()
		pool.FirstArrival = now

		miningDuration := time.Since(minedAt)
		if miningDuration < 0 {
			miningDuration = 0
		}

		var remainingWindow time.Duration
		if miningDuration <= p.TotalInterval {
			remainingWindow = p.TotalInterval - miningDuration
		} else {
			remainingWindow = p.MinWindowDuration
			log.Printf("[CandidatePool] EXTENDED mode height %d: mining %v > target %v", height, miningDuration, p.TotalInterval)
		}

		if remainingWindow < p.MinWindowDuration {
			remainingWindow = p.MinWindowDuration
		}
		if remainingWindow > p.MaxExtensionWindow {
			remainingWindow = p.MaxExtensionWindow
		}
		pool.Deadline = now.Add(remainingWindow)

		log.Printf("[CandidatePool] ✓ height %d | window %v | deadline %v | cycle %v",
			height, remainingWindow, pool.Deadline.Format("15:04:05"), miningDuration+remainingWindow)

		deadlineCopy := pool.Deadline
		heightCopy := height
		pool.timer = time.AfterFunc(time.Until(deadlineCopy), func() {
			p.selectBest(heightCopy)
		})
	}

	work := new(big.Int)
	if p.workCalc != nil {
		work = p.workCalc.GetBlockProof(block.Header.DifficultyBits)
	}

	candidate := &Candidate{
		Block:       block,
		SourceID:    sourceID,
		SubmittedAt: time.Now(),
		MinedAt:     minedAt,
		Work:        work,
		Validated:   false,
	}

	pool.Candidates = append(pool.Candidates, candidate)
	pool.mu.Unlock()

	if len(pool.Candidates) > 1 {
		log.Printf("[CandidatePool] ⚡ height %d | candidates %d | competing", height, len(pool.Candidates))
	}

	return nil
}

func (p *CandidatePool) SubmitCandidateWithDeadline(block *Block, sourceID string, minedAt time.Time, deadline time.Time) error {
	p.mu.RLock()
	if p.stopped {
		p.mu.RUnlock()
		return fmt.Errorf("candidate pool: pool is stopped")
	}
	p.mu.RUnlock()

	if block == nil {
		return fmt.Errorf("candidate pool: nil block")
	}

	if len(block.Hash) == 0 {
		return fmt.Errorf("candidate pool: block hash is empty for height %d", block.Header.Height)
	}

	blockHash := hex.EncodeToString(block.Hash)
	height := block.Header.Height
	if height == 0 && block.Height != 0 {
		log.Printf("[CandidatePool] WARNING: Header.Height=0 in SubmitCandidateWithDeadline, using Block.Height=%d as fallback", block.Height)
		height = block.Height
	}

	submissionDelay := time.Since(minedAt)
	if submissionDelay > p.MaxLateness {
		log.Printf("[CandidatePool] WARNING: late submission from %s for height %d: delay %v (max %v)",
			sourceID, height, submissionDelay, p.MaxLateness)
		return fmt.Errorf("candidate pool: late submission for height %d: delay %v exceeds max %v",
			height, submissionDelay, p.MaxLateness)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return fmt.Errorf("candidate pool: pool is stopped (re-check after write lock)")
	}

	pool, exists := p.pools[height]
	if exists {
		pool.mu.Lock()

		if pool.Closed || time.Now().After(pool.Deadline) {
			pool.mu.Unlock()
			return fmt.Errorf("candidate pool: window closed for height %d", height)
		}

		for _, c := range pool.Candidates {
			if c != nil && c.Block != nil && hex.EncodeToString(c.Block.Hash) == blockHash {
				pool.mu.Unlock()
				return fmt.Errorf("candidate pool: duplicate candidate %s for height %d", blockHash[:16], height)
			}
			if c.SourceID == sourceID {
				pool.mu.Unlock()
				return fmt.Errorf("candidate pool: source %s already submitted for height %d (keeping existing)", sourceID[:min(12, len(sourceID))], height)
			}
		}

		if len(pool.Candidates) >= p.MaxCandidates {
			pool.mu.Unlock()
			return fmt.Errorf("candidate pool: pool full for height %d (max %d)", height, p.MaxCandidates)
		}
	} else {
		pool = &HeightPool{
			Height:     height,
			Candidates: make([]*Candidate, 0, p.MaxCandidates),
		}
		pool.mu.Lock()
		p.pools[height] = pool

		now := time.Now()
		pool.FirstArrival = now

		if deadline.IsZero() || deadline.Before(now) {
			log.Printf("[CandidatePool] WARNING: received invalid deadline=%v for height %d (now=%v), using local calculation",
				deadline.Format("15:04:05"), height, now.Format("15:04:05"))
			miningDuration := time.Since(minedAt)
			if miningDuration < 0 {
				miningDuration = 0
			}
			remainingWindow := p.TotalInterval - miningDuration
			if remainingWindow < p.MinWindowDuration {
				remainingWindow = p.MinWindowDuration
			}
			if remainingWindow > p.MaxExtensionWindow {
				remainingWindow = p.MaxExtensionWindow
			}
			deadline = now.Add(remainingWindow)
		}

		pool.Deadline = deadline

		log.Printf("[CandidatePool] ✓ height %d | synced deadline %v", height, deadline.Format("15:04:05"))

		deadlineCopy := pool.Deadline
		heightCopy := height
		pool.timer = time.AfterFunc(time.Until(deadlineCopy), func() {
			p.selectBest(heightCopy)
		})
	}

	work := new(big.Int)
	if p.workCalc != nil {
		work = p.workCalc.GetBlockProof(block.Header.DifficultyBits)
	}

	candidate := &Candidate{
		Block:       block,
		SourceID:    sourceID,
		SubmittedAt: time.Now(),
		MinedAt:     minedAt,
		Work:        work,
		Validated:   false,
	}

	pool.Candidates = append(pool.Candidates, candidate)
	pool.mu.Unlock()

	if len(pool.Candidates) > 1 {
		log.Printf("[CandidatePool] ⚡ height %d | candidates %d | competing", height, len(pool.Candidates))
	}

	return nil
}

func (p *CandidatePool) selectBest(height uint64) {
	p.mu.Lock()
	pool, exists := p.pools[height]
	if !exists {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	if pool == nil {
		return
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if len(pool.Candidates) == 0 {
		pool.Closed = true
		if pool.timer != nil {
			pool.timer.Stop()
		}
		return
	}

	// Weighted hash selection: each candidate's block hash is XOR-mixed
	// with a deterministic prefix derived from the source miner ID.
	// This produces a stable selection key that converges identically
	// when all nodes share the same candidate set, while still requiring
	// real Proof-of-Work to influence the outcome.
	sort.Slice(pool.Candidates, func(i, j int) bool {
		candI := pool.Candidates[i]
		candJ := pool.Candidates[j]

		if candI.Block == nil && candJ.Block == nil {
			return candI.SubmittedAt.Before(candJ.SubmittedAt)
		}
		if candI.Block == nil {
			return false
		}
		if candJ.Block == nil {
			return true
		}

		keyI := weightedSelectKey(candI)
		keyJ := weightedSelectKey(candJ)

		for k := 0; k < len(keyI) && k < len(keyJ); k++ {
			if keyI[k] < keyJ[k] {
				return true
			}
			if keyI[k] > keyJ[k] {
				return false
			}
		}
		// Fallback: earlier timestamp wins when all else is equal
		return candI.Block.Header.TimestampUnix < candJ.Block.Header.TimestampUnix
	})

	best := pool.Candidates[0]
	pool.Closed = true

	if pool.timer != nil {
		pool.timer.Stop()
	}

	if len(pool.Candidates) > 1 {
		log.Printf("[CandidatePool] 🏆 height %d | election results:", height)
		for rank, c := range pool.Candidates {
			shortID := c.SourceID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			hashPreview := "N/A"
			if c.Block != nil && len(c.Block.Hash) >= 4 {
				hashPreview = hex.EncodeToString(c.Block.Hash[:4])
			}
			marker := "🥇"
			if rank == 0 {
				marker = "👑"
			}
			timestamp := int64(0)
			if c.Block != nil {
				timestamp = c.Block.Header.TimestampUnix
			}
			log.Printf("  %s #%d %s hash=%s... time=%d", marker, rank+1, shortID, hashPreview, timestamp)
		}
	}

	p.mu.Lock()
	delete(p.pools, height)
	p.mu.Unlock()

	if best.Block == nil {
		log.Printf("[CandidatePool] height %d | best candidate has nil block, cannot commit", height)
		return
	}

	if p.chain != nil {
		if accepted, err := p.chain.AddBlock(best.Block); err != nil {
			log.Printf("[CandidatePool] ✗ height %d | failed to add best: %v", height, err)
		} else if accepted {
			shortID := best.SourceID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}
			log.Printf("[CandidatePool] ✓ height %d | winner %s | %d candidates",
				height, shortID, len(pool.Candidates))

			if p.OnBlockSelected != nil {
				p.OnBlockSelected(best.Block)
			}
		}

		for i := 1; i < len(pool.Candidates); i++ {
			c := pool.Candidates[i]
			if c.Block == nil {
				continue
			}
			if _, err := p.chain.AddBlock(c.Block); err != nil {
				log.Printf("[CandidatePool] failed to add fork block %d from %s: %v",
					i, c.SourceID, err)
			}
		}
	} else {
		log.Printf("[CandidatePool] WARNING: chain reference not set, cannot commit blocks for height %d", height)
	}
}

func (p *CandidatePool) GetPoolStats() map[uint64]PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := make(map[uint64]PoolStats, len(p.pools))
	for height, pool := range p.pools {
		if pool == nil {
			continue
		}
		pool.mu.Lock()
		state := "open"
		if pool.Closed {
			state = "closed"
		} else if time.Now().After(pool.Deadline) {
			state = "expired"
		}
		stats[height] = PoolStats{
			CandidateCount: len(pool.Candidates),
			WindowState:    state,
			FirstArrival:   pool.FirstArrival,
			Deadline:       pool.Deadline,
		}
		pool.mu.Unlock()
	}
	return stats
}

func (p *CandidatePool) GetWindowDeadline(height uint64) (time.Time, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pool, exists := p.pools[height]
	if !exists || pool == nil {
		return time.Time{}, false
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if pool.Closed {
		return time.Time{}, false
	}

	return pool.Deadline, true
}

const poolHeightTolerance = 2

// ShouldPool determines if a block at the given height should be routed through
// the candidate pool for fair competition, or added directly to the chain.
// Blocks at or near the tip (competition frontier) must go through the pool to
// ensure Work-based fair selection across all sources (local mining + P2P received).
// Historical/sync blocks well behind the tip bypass the pool for fast catch-up.
func (p *CandidatePool) ShouldPool(height uint64) bool {
	if p.stopped {
		return false
	}
	if p.chain == nil {
		return false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	tip := p.chain.LatestBlock()
	if tip == nil {
		return false
	}

	tipHeight := tip.GetHeight()
	if height >= tipHeight && height <= tipHeight+poolHeightTolerance {
		return true
	}
	return false
}

func (p *CandidatePool) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	poolsCopy := make(map[uint64]*HeightPool, len(p.pools))
	for k, v := range p.pools {
		poolsCopy[k] = v
	}
	p.mu.Unlock()

	close(p.stopCh)

	for _, pool := range poolsCopy {
		if pool == nil {
			continue
		}
		pool.mu.Lock()
		if pool.timer != nil {
			pool.timer.Stop()
		}
		pool.Closed = true
		pool.mu.Unlock()
	}

	log.Printf("[CandidatePool] stopped: closed %d pools", len(poolsCopy))
}

// weightedSelectKey computes a deterministic selection key for candidate competition.
// The key is formed by XOR-mixing the block hash with a source-derived prefix
// (SHA-256 of miner SourceID, truncated to 4 bytes and cycled across the hash length).
// Properties:
//   - Deterministic: same (block, sourceID) → same key on every node
//   - Source-bound: different sources produce different XOR masks, preventing
//     a miner from predicting the final sort order across nodes
//   - PoW-preserving: the block hash still dominates the comparison,
//     so the miner with the most hashrate has the best chance to win
func weightedSelectKey(c *Candidate) []byte {
	if c == nil || c.Block == nil || len(c.Block.Hash) == 0 {
		return nil
	}

	hash := c.Block.Hash
	mask := computeSourceMask(c.SourceID)

	key := make([]byte, len(hash))
	for i := range hash {
		key[i] = hash[i] ^ mask[i%len(mask)]
	}
	return key
}

// computeSourceMask derives a 4-byte deterministic mask from the source miner ID.
func computeSourceMask(sourceID string) []byte {
	if sourceID == "" {
		return []byte{0, 0, 0, 0}
	}
	digest := sha256.Sum256([]byte(sourceID))
	return digest[:4]
}
