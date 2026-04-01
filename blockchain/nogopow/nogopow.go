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

package nogopow

import (
	"encoding/binary"
	"errors"
	"math/big"
	"sync"
	"time"

	"golang.org/x/crypto/sha3"
)

// ErrInvalidSeal is returned when a seal is invalid
var ErrInvalidSeal = errors.New("invalid seal")

// NogopowEngine implements consensus.Engine for NogoPow PoW
type NogopowEngine struct {
	config       *Config
	sealCh       chan *Block
	exitCh       chan struct{}
	wg           sync.WaitGroup
	lock         sync.RWMutex
	running      bool
	hashrate     uint64
	cache        *Cache
	diffAdjuster *DifficultyAdjuster
	matA         *denseMatrix
	matB         *denseMatrix
	matRes       *denseMatrix
}

// New creates a new NogopowEngine
func New(config *Config) *NogopowEngine {
	if config == nil {
		config = DefaultConfig()
	}

	engine := &NogopowEngine{
		config:       config,
		sealCh:       make(chan *Block),
		exitCh:       make(chan struct{}),
		running:      false,
		hashrate:     0,
		cache:        NewCache(config),
		diffAdjuster: NewDifficultyAdjuster(config.Difficulty),
	}

	if config.ReuseObjects {
		engine.matA = GetMatrix(matSize, matSize)
		engine.matB = GetMatrix(matSize, matSize)
		engine.matRes = GetMatrix(matSize, matSize)
	}

	return engine
}

// NewFaker creates a fake engine for testing
func NewFaker() *NogopowEngine {
	config := DefaultConfig()
	config.PowMode = ModeFake
	return New(config)
}

// Author returns the header's coinbase as the author
func (t *NogopowEngine) Author(header *Header) (Address, error) {
	return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to consensus rules
func (t *NogopowEngine) VerifyHeader(chain ChainHeaderReader, header *Header, seal bool) error {
	// If we're running in fake mode, skip verification
	if t.config.PowMode == ModeFake {
		return nil
	}

	// Genesis block is always valid
	if header.Number.Uint64() == 0 {
		return nil
	}

	// Verify PoW seal if requested
	if seal {
		if err := t.verifySeal(chain, header); err != nil {
			return err
		}
	}

	return nil
}

// VerifyHeaders verifies a batch of headers concurrently
func (t *NogopowEngine) VerifyHeaders(chain ChainHeaderReader, headers []*Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))

	if len(headers) == 0 {
		close(results)
		return abort, results
	}

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()

		for i, header := range headers {
			seal := seals[i]

			t.wg.Add(1)
			go func(idx int, h *Header, s bool) {
				defer t.wg.Done()

				select {
				case <-abort:
					results <- nil
					return
				default:
					err := t.VerifyHeader(chain, h, s)
					results <- err
				}
			}(i, header, seal)
		}

		// Wait for all workers
		t.wg.Wait()
		close(results)
	}()

	return abort, results
}

// VerifyUncles verifies that uncles conform to consensus rules
func (t *NogopowEngine) VerifyUncles(chain ChainReader, block *Block) error {
	const maxUncles = 2
	if len(block.Uncles()) > maxUncles {
		return errors.New("too many uncles")
	}

	for _, uncle := range block.Uncles() {
		if uncle.Number.Cmp(block.Header().Number) != 0 {
			return ErrInvalidSeal
		}

		if t.config.PowMode != ModeFake {
			if err := t.verifySeal(chain, uncle); err != nil {
				return err
			}
		}
	}

	return nil
}

// Prepare initializes the difficulty field of a header
func (t *NogopowEngine) Prepare(chain ChainHeaderReader, header *Header) error {
	parent := chain.GetHeaderByHash(header.ParentHash)
	if parent == nil {
		return errors.New("parent not found")
	}

	// Calculate difficulty dynamically based on block time and parent difficulty
	header.Difficulty = t.CalcDifficulty(chain, header.Time, parent)
	return nil
}

// Finalize is a placeholder for block finalization
// Reward distribution is handled by the blockchain layer
func (t *NogopowEngine) Finalize(chain ChainHeaderReader, header *Header, stateDB StateDB, txs []*Transaction, uncles []*Header) {
	// Rewards are handled by the blockchain layer
	// This is a placeholder for future implementation
	header.Root = stateDB.IntermediateRoot(true)
}

// FinalizeAndAssemble runs Finalize and assembles the final block
func (t *NogopowEngine) FinalizeAndAssemble(chain ChainHeaderReader, header *Header, stateDB StateDB, txs []*Transaction, uncles []*Header, receipts []*Receipt) (*Block, error) {
	t.Finalize(chain, header, stateDB, txs, uncles)
	block := NewBlock(header, txs, uncles, receipts)
	return block, nil
}

// Seal generates a new sealing request for the given block
func (t *NogopowEngine) Seal(chain ChainHeaderReader, block *Block, results chan<- *Block, stop <-chan struct{}) error {
	if t.config.PowMode == ModeFake || t.config.PowMode == ModeTest {
		t.config.Log.Info("NogoPow fake/test mode - sealing immediately", "block", block.Number())
		select {
		case results <- block:
			t.config.Log.Info("NogoPow fake/test mode - block sealed and sent", "block", block.Number())
		case <-stop:
			t.config.Log.Info("NogoPow fake/test mode - stopped", "block", block.Number())
			return nil
		}
		return nil
	}

	t.config.Log.Debug("NogoPow normal mode - starting mining", "block", block.Number())
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.mineBlock(chain, block, results, stop)
	}()

	return nil
}

// mineBlock performs the actual mining operation
func (t *NogopowEngine) mineBlock(chain ChainHeaderReader, block *Block, results chan<- *Block, stop <-chan struct{}) {
	header := block.Header()
	startNonce := uint64(0)
	startTime := time.Now()

	t.config.Log.Info("NogoPow mining started",
		"block", header.Number.Uint64(),
		"difficulty", header.Difficulty,
		"threads", 1,
	)

	// Calculate seed from parent block (fixed for all nonce attempts)
	seed := t.calcSeed(chain, header)

	// Mining loop
	for nonce := startNonce; ; nonce++ {
		select {
		case <-stop:
			t.config.Log.Info("NogoPow mining stopped", "block", header.Number.Uint64())
			return
		case <-t.exitCh:
			t.config.Log.Info("NogoPow mining exit", "block", header.Number.Uint64())
			return
		default:
		}

		// Try to solve block
		header.Nonce = BlockNonce{}
		binary.LittleEndian.PutUint64(header.Nonce[:8], nonce)

		// Check if solution is valid
		if t.checkSolution(chain, header, seed) {
			elapsed := time.Since(startTime)
			t.config.Log.Info("Successfully sealed block",
				"number", header.Number.Uint64(),
				"hash", header.Hash().Hex(),
				"nonce", nonce,
				"elapsed", elapsed,
			)

			select {
			case results <- block:
				return
			case <-stop:
				return
			}
		}

		// Update hashrate
		t.hashrate++

		// Log progress every 1000 nonces
		if nonce%1000 == 0 && nonce > 0 {
			t.config.Log.Debug("Mining in progress",
				"block", header.Number.Uint64(),
				"nonce", nonce,
				"hashrate", t.hashrate,
			)
		}
	}
}

// checkSolution verifies if header has valid PoW (optimized for mining loop)
func (t *NogopowEngine) checkSolution(chain ChainHeaderReader, header *Header, seed Hash) bool {
	// Calculate block hash with nonce
	blockHash := t.SealHash(header)

	// Apply NogoPow PoW algorithm: H(blockHash, seed)
	powHash := t.computePoW(blockHash, seed)

	// Check if hash meets difficulty target
	return t.checkPow(powHash, header.Difficulty)
}

// verifySeal verifies that the block has a valid PoW seal (full version with logging)
func (t *NogopowEngine) verifySeal(chain ChainHeaderReader, header *Header) error {
	// Calculate seed from parent block
	seed := t.calcSeed(chain, header)

	t.config.Log.Info("NogoPow verifySeal",
		"number", header.Number.Uint64(),
		"nonce", binary.LittleEndian.Uint64(header.Nonce[:8]),
		"seed", seed.Hex(),
		"difficulty", header.Difficulty,
	)

	// Calculate block hash with nonce
	blockHash := t.SealHash(header)

	t.config.Log.Info("NogoPow block hash",
		"blockHash", blockHash.Hex(),
	)

	// Apply NogoPow PoW algorithm: H(blockHash, seed)
	powHash := t.computePoW(blockHash, seed)

	t.config.Log.Info("NogoPow pow hash",
		"powHash", powHash.Hex(),
	)

	// Check if hash meets difficulty target
	if !t.checkPow(powHash, header.Difficulty) {
		t.config.Log.Info("NogoPow checkPow failed",
			"powHash", powHash.Hex(),
			"difficulty", header.Difficulty,
		)
		return ErrInvalidSeal
	}

	t.config.Log.Info("NogoPow checkPow passed",
		"powHash", powHash.Hex(),
	)

	return nil
}

// calcSeed calculates the seed hash from parent block
func (t *NogopowEngine) calcSeed(chain ChainHeaderReader, header *Header) Hash {
	// For genesis block, use zero seed
	if header.Number.Uint64() == 0 {
		return Hash{}
	}

	// Get parent block
	parent := chain.GetHeaderByHash(header.ParentHash)
	if parent == nil {
		return Hash{}
	}

	// Seed is parent's hash
	return parent.Hash()
}

// computePoW computes the proof-of-work hash using NogoPow algorithm
func (t *NogopowEngine) computePoW(blockHash, seed Hash) Hash {
	cacheData := t.cache.GetData(seed.Bytes())

	t.config.Log.Info("NogoPow computePoW",
		"seed", seed.Hex(),
		"blockHash", blockHash.Hex(),
		"cacheDataLen", len(cacheData),
		"cacheDataFirst", cacheData[0],
	)

	if t.config.ReuseObjects && t.matA != nil {
		result := mulMatrixWithPool(blockHash.Bytes(), cacheData, t.matA, t.matB, t.matRes)
		return hashMatrix(result)
	}

	result := mulMatrix(blockHash.Bytes(), cacheData)
	return hashMatrix(result)
}

// SealHash returns the hash of a block prior to sealing
func (t *NogopowEngine) SealHash(header *Header) Hash {
	hasher := sha3.NewLegacyKeccak256()
	rlpEncode(hasher, header)
	return BytesToHash(hasher.Sum(nil))
}

// CalcDifficulty returns the difficulty for a new block
func (t *NogopowEngine) CalcDifficulty(chain ChainHeaderReader, time uint64, parent *Header) *big.Int {
	if parent == nil || parent.Difficulty == nil {
		return big.NewInt(int64(t.config.Difficulty.MinimumDifficulty))
	}

	// Use difficulty adjuster for smooth adjustment
	newDifficulty := t.diffAdjuster.CalcDifficulty(time, parent)

	t.config.Log.Info("NogoPow CalcDifficulty",
		"parentNumber", parent.Number.Uint64(),
		"parentDifficulty", parent.Difficulty.Uint64(),
		"newDifficulty", newDifficulty.Uint64(),
		"time", time,
		"parentTime", parent.Time,
		"timeDiff", time-parent.Time,
	)

	return newDifficulty
}

// APIs returns the RPC APIs this consensus engine provides
func (t *NogopowEngine) APIs(chain ChainHeaderReader) []API {
	// RPC APIs are handled by the blockchain layer
	return nil
}

// Close terminates all background threads
func (t *NogopowEngine) Close() error {
	close(t.exitCh)
	t.wg.Wait()

	if t.config.ReuseObjects {
		if t.matA != nil {
			PutMatrix(t.matA)
		}
		if t.matB != nil {
			PutMatrix(t.matB)
		}
		if t.matRes != nil {
			PutMatrix(t.matRes)
		}
	}

	return nil
}

// HashRate returns current hashrate
func (t *NogopowEngine) HashRate() uint64 {
	return t.hashrate
}

// checkPow verifies if hash meets difficulty target
func (t *NogopowEngine) checkPow(hash Hash, difficulty *big.Int) bool {
	target := difficultyToTarget(difficulty)
	hashInt := new(big.Int).SetBytes(hash.Bytes())
	result := hashInt.Cmp(target) <= 0

	t.config.Log.Info("NogoPow checkPow",
		"hash", hash.Hex(),
		"hashInt", hashInt.String(),
		"target", target.String(),
		"difficulty", difficulty.String(),
		"result", result,
	)

	return result
}

// difficultyToTarget converts difficulty to target threshold
func difficultyToTarget(difficulty *big.Int) *big.Int {
	maxTarget := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	target := new(big.Int).Div(maxTarget, difficulty)
	return target
}
