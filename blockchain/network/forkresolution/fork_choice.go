// DEPRECATED: This entire file is dead code.
//
// ForkChoice was designed as an alternative fork-selection mechanism with
// multi-dimensional scoring (work, height, stake, timestamp). It was never
// integrated into production code — zero instances are created in any
// initialization path (verified: 2026-06-11 cross-validation audit).
//
// CRITICAL WARNING: ReorgNeeded contains a non-deterministic random
// tiebreaker (rand.Float64() < 0.5). If this code were inadvertently
// activated, different nodes would make DIFFERENT tiebreaking decisions,
// causing an immediate network fork.
//
// Fork resolution is handled by:
//   - ForkResolver.RequestReorg (Nakamoto consensus + oscillation detection)
//   - core.Chain.reorganizeChainLocked (internal fallback)
//   - SeedConsensusEngine (seed-node pre-consensus voting)
//
// Do NOT instantiate ForkChoice. Do NOT call any of its methods.
// This file is kept for historical reference only and may be removed
// in a future release once all external references (documentation, tests)
// have been updated.

package forkresolution

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	mrand "math/rand"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

type ChainHeaderReader interface {
	BlockByHash(hash string) (*core.Block, bool)
	BlockByHeight(height uint64) (*core.Block, bool)
	LatestBlock() *core.Block
	CanonicalWork() *big.Int
	CalculateCumulativeWork(block *core.Block) *big.Int
}

type ForkChoice struct {
	chain    ChainHeaderReader
	rand     *mrand.Rand
	preserve func(header *core.Block) bool
}

func NewForkChoice(chainReader ChainHeaderReader, preserve func(header *core.Block) bool) *ForkChoice {
	var seedBytes [8]byte
	_, err := rand.Read(seedBytes[:])
	if err != nil {
		seedBytes = [8]byte{}
	}
	seed := int64(0)
	for i := 0; i < 8; i++ {
		seed = (seed << 8) | int64(seedBytes[i])
	}
	return &ForkChoice{
		chain:    chainReader,
		rand:     mrand.New(mrand.NewSource(seed)),
		preserve: preserve,
	}
}

func (f *ForkChoice) CommonAncestor(current, header *core.Block) (*core.Block, error) {
	oldH := current
	newH := header

	if oldH.GetHeight() > newH.GetHeight() {
		for oldH != nil && oldH.GetHeight() != newH.GetHeight() {
			parent, exists := f.chain.BlockByHash(hex.EncodeToString(oldH.Header.PrevHash))
			if !exists {
				return nil, fmt.Errorf("invalid old chain at height %d", oldH.GetHeight())
			}
			oldH = parent
		}
	} else {
		for newH != nil && newH.GetHeight() != oldH.GetHeight() {
			parent, exists := f.chain.BlockByHash(hex.EncodeToString(newH.Header.PrevHash))
			if !exists {
				return nil, fmt.Errorf("invalid new chain at height %d", newH.GetHeight())
			}
			newH = parent
		}
	}

	for {
		if bytes.Equal(oldH.Hash, newH.Hash) {
			return oldH, nil
		}
		oldParent, exists := f.chain.BlockByHash(hex.EncodeToString(oldH.Header.PrevHash))
		if !exists {
			return nil, fmt.Errorf("invalid old chain at height %d", oldH.GetHeight())
		}
		oldH = oldParent

		newParent, exists := f.chain.BlockByHash(hex.EncodeToString(newH.Header.PrevHash))
		if !exists {
			return nil, fmt.Errorf("invalid new chain at height %d", newH.GetHeight())
		}
		newH = newParent
	}
}

func (f *ForkChoice) ReorgNeeded(current, extern *core.Block) (bool, error) {
	localTD := f.chain.CanonicalWork()
	externTD := f.chain.CalculateCumulativeWork(extern)
	if localTD == nil || externTD == nil {
		return false, errors.New("missing total difficulty")
	}

	reorg := externTD.Cmp(localTD) > 0
	tie := externTD.Cmp(localTD) == 0
	if tie {
		externNum, localNum := extern.GetHeight(), current.GetHeight()
		if externNum < localNum {
			reorg = true
		} else if externNum == localNum {
			var currentPreserve, externPreserve bool
			if f.preserve != nil {
				currentPreserve = f.preserve(current)
				externPreserve = f.preserve(extern)
			}
			reorg = !currentPreserve && (externPreserve || f.rand.Float64() < 0.5)
		}
	}

	if !reorg {
		return reorg, nil
	}

	commonHeader, err := f.CommonAncestor(current, extern)
	if err != nil {
		return reorg, err
	}

	reorgDepth := current.GetHeight() - commonHeader.GetHeight()
	if reorgDepth > MaxReorgDepth {
		return false, fmt.Errorf("reorg depth %d exceeds maximum %d", reorgDepth, MaxReorgDepth)
	}

	if reorgDepth > 2 {
		_ = reorgDepth
	}

	return reorg, nil
}

func (f *ForkChoice) ShouldReorgTo(extern *core.Block) (bool, error) {
	current := f.chain.LatestBlock()
	if current == nil {
		return true, nil
	}
	if bytes.Equal(current.Hash, extern.Hash) {
		return false, nil
	}
	return f.ReorgNeeded(current, extern)
}

func (f *ForkChoice) FindForkDepth(current, extern *core.Block) (uint64, error) {
	common, err := f.CommonAncestor(current, extern)
	if err != nil {
		return 0, err
	}
	currentDepth := current.GetHeight() - common.GetHeight()
	externDepth := extern.GetHeight() - common.GetHeight()
	if currentDepth > externDepth {
		return currentDepth, nil
	}
	return externDepth, nil
}

func (f *ForkChoice) IsOnCanonicalPath(tip, candidate *core.Block) bool {
	if tip == nil || candidate == nil {
		return false
	}
	for cur := tip; cur != nil; {
		if cur.GetHeight() == candidate.GetHeight() && bytes.Equal(cur.Hash, candidate.Hash) {
			return true
		}
		if cur.GetHeight() == 0 {
			break
		}
		parent, exists := f.chain.BlockByHash(hex.EncodeToString(cur.Header.PrevHash))
		if !exists || parent == nil {
			break
		}
		cur = parent
	}
	return false
}

func (f *ForkChoice) TimeSinceCommonAncestor(current, extern *core.Block) time.Duration {
	common, err := f.CommonAncestor(current, extern)
	if err != nil {
		return 0
	}
	now := time.Now()
	commonTime := time.Unix(int64(common.Header.TimestampUnix), 0)
	return now.Sub(commonTime)
}