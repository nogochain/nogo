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

package miner

import (
	"encoding/hex"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/nogopow"
)

// WorkManager manages mining work distribution and difficulty adjustment
// Production-grade: implements thread-safe work caching and distribution
type WorkManager struct {
	mu sync.RWMutex

	cache        map[string]*big.Int // hashHex -> cumulative work
	bc           Blockchain
	lastAdjust   time.Time
	adjustWindow uint64
}

// NewWorkManager creates a new work manager instance
func NewWorkManager(bc Blockchain) *WorkManager {
	return &WorkManager{
		cache:        make(map[string]*big.Int),
		bc:           bc,
		lastAdjust:   time.Now(),
		adjustWindow: config.DefaultDifficultyWindow,
	}
}

// GetBlockWork calculates the work for a single block
// Production-grade: uses NogoPow work calculation formula
func GetBlockWork(block *core.Block) *big.Int {
	if block == nil {
		return big.NewInt(0)
	}

	hashInt := new(big.Int).SetBytes(block.Hash)
	if hashInt.Sign() == 0 {
		hashInt.SetInt64(1)
	}

	difficulty := DifficultyFromBits(block.Header.DifficultyBits)
	work := new(big.Int).Set(difficulty)
	work.Lsh(work, 32)

	hashPlusOne := new(big.Int).Add(hashInt, big.NewInt(1))
	work.Div(work, hashPlusOne)

	return work
}

// CalculateChainWork calculates cumulative work from genesis to given block
func CalculateChainWork(bc Blockchain, block *core.Block) *big.Int {
	if block == nil {
		return big.NewInt(0)
	}

	totalWork := big.NewInt(0)
	current := block

	for current != nil {
		blockWork := GetBlockWork(current)
		totalWork.Add(totalWork, blockWork)

		if current.Height == 0 {
			break
		}

		parentHash := hex.EncodeToString(current.Header.PrevHash)
		current = GetBlockByHash(bc, parentHash)
	}

	return totalWork
}

// GetBlockByHash retrieves a block by its hash
func GetBlockByHash(bc Blockchain, hashHex string) *core.Block {
	if bc == nil {
		return nil
	}

	hash, err := hex.DecodeString(hashHex)
	if err != nil {
		return nil
	}

	block := bc.LatestBlock()
	if block == nil {
		return nil
	}

	if string(block.Hash) == string(hash) {
		return block
	}

	if string(block.Header.PrevHash) == string(hash) {
		return block
	}

	return nil
}

// CompareWork compares two work values
// Returns: 1 if work1 > work2, -1 if work1 < work2, 0 if equal
func CompareWork(work1, work2 *big.Int) int {
	if work1 == nil && work2 == nil {
		return 0
	}
	if work1 == nil {
		return -1
	}
	if work2 == nil {
		return 1
	}
	return work1.Cmp(work2)
}

// GetBlockCumulativeWork gets cumulative work for a block with caching
func (wm *WorkManager) GetBlockCumulativeWork(blockHash string) *big.Int {
	wm.mu.RLock()
	if cached, ok := wm.cache[blockHash]; ok {
		wm.mu.RUnlock()
		return new(big.Int).Set(cached)
	}
	wm.mu.RUnlock()

	block := GetBlockByHash(wm.bc, blockHash)
	if block == nil {
		return big.NewInt(0)
	}

	cumulativeWork := CalculateChainWork(wm.bc, block)

	wm.mu.Lock()
	wm.cache[blockHash] = new(big.Int).Set(cumulativeWork)
	wm.mu.Unlock()

	return cumulativeWork
}

// ClearCache clears the entire work cache
func (wm *WorkManager) ClearCache() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.cache = make(map[string]*big.Int)
}

// DifficultyFromBits converts difficulty bits to big.Int difficulty
// Production-grade: implements Bitcoin-style difficulty encoding
func DifficultyFromBits(bits uint32) *big.Int {
	exponent := bits >> 24
	mantissa := bits & 0x00ffffff

	difficulty := new(big.Int).SetInt64(int64(mantissa))

	if exponent > 3 {
		shift := uint(8 * (exponent - 3))
		difficulty.Lsh(difficulty, shift)
	} else if exponent < 3 {
		shift := uint(8 * (3 - exponent))
		difficulty.Rsh(difficulty, shift)
	}

	return difficulty
}

// DifficultyToBits converts big.Int difficulty to difficulty bits
// Production-grade: implements Bitcoin-style difficulty encoding
func DifficultyToBits(difficulty *big.Int) uint32 {
	if difficulty == nil || difficulty.Sign() <= 0 {
		return 0
	}

	size := difficulty.BitLen()
	exponent := (size + 7) / 8

	mantissa := new(big.Int).Set(difficulty)
	if exponent > 3 {
		shift := uint(8 * (exponent - 3))
		mantissa.Rsh(mantissa, shift)
	} else if exponent < 3 {
		shift := uint(8 * (3 - exponent))
		mantissa.Lsh(mantissa, shift)
	}

	mantissaAnd := new(big.Int).And(mantissa, big.NewInt(0x00ffffff))
	if !mantissaAnd.IsUint64() {
		return 0
	}

	bits := uint32(exponent)<<24 | uint32(mantissaAnd.Uint64())
	return bits
}

// AdjustDifficulty adjusts difficulty based on actual vs target block times
// Production-grade: implements PI controller for difficulty adjustment
// This function is deprecated - use nogopow.DifficultyAdjuster.CalcDifficulty instead
func AdjustDifficulty(
	lastBlocks []*core.Block,
	targetBlockTime time.Duration,
	minDifficulty uint32,
	maxDifficulty uint32,
) uint32 {
	if len(lastBlocks) < 2 {
		return lastBlocks[len(lastBlocks)-1].Header.DifficultyBits
	}

	// Use the new PI controller from nogopow package
	parentBlock := lastBlocks[len(lastBlocks)-1]
	currentTime := uint64(time.Now().Unix())

	// Create consensus params for difficulty calculation
	consensusParams := &config.ConsensusParams{
		BlockTimeTargetSeconds:     int64(targetBlockTime.Seconds()),
		MaxDifficultyChangePercent: 20,
		MinDifficulty:              uint32(minDifficulty),
	}

	// Create a temporary adjuster for this calculation
	adjuster := nogopow.NewDifficultyAdjuster(consensusParams)

	// Create a header from the parent block
	parentHeader := &nogopow.Header{
		Number:     new(big.Int).SetUint64(parentBlock.Height),
		Difficulty: new(big.Int).SetUint64(uint64(parentBlock.Header.DifficultyBits)),
		Time:       uint64(parentBlock.Header.TimestampUnix),
	}

	newDifficulty := adjuster.CalcDifficulty(currentTime, parentHeader)
	return DifficultyToBits(newDifficulty)
}

// CalculateNextDifficulty calculates the next difficulty after an adjustment interval
func CalculateNextDifficulty(
	bc Blockchain,
	lastHeight uint64,
	targetBlockTime time.Duration,
) uint32 {
	if bc == nil || lastHeight < config.DefaultDifficultyWindow {
		return config.DefaultGenesisDifficultyBits
	}

	startHeight := lastHeight - config.DefaultDifficultyWindow
	blocks := make([]*core.Block, 0, config.DefaultDifficultyWindow+1)

	currentHeight := lastHeight
	for currentHeight >= startHeight {
		block := GetBlockAtHeight(bc, currentHeight)
		if block == nil {
			break
		}
		blocks = append(blocks, block)
		currentHeight--
	}

	if len(blocks) < 2 {
		return config.DefaultGenesisDifficultyBits
	}

	for i, j := 0, len(blocks)-1; i < j; i, j = i+1, j-1 {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	}

	return AdjustDifficulty(
		blocks,
		targetBlockTime,
		config.DefaultMinimumDifficulty,
		config.DefaultGenesisDifficultyBits,
	)
}

// GetBlockAtHeight retrieves a block at the specified height
func GetBlockAtHeight(bc Blockchain, height uint64) *core.Block {
	if bc == nil {
		return nil
	}

	latest := bc.LatestBlock()
	if latest == nil {
		return nil
	}

	if latest.Height == height {
		return latest
	}

	if height > latest.Height {
		return nil
	}

	current := latest
	for current != nil && current.Height > height {
		parentHash := hex.EncodeToString(current.Header.PrevHash)
		current = GetBlockByHash(bc, parentHash)
	}

	if current != nil && current.Height == height {
		return current
	}

	return nil
}

// WorkDistribution manages work distribution to multiple mining threads
type WorkDistribution struct {
	mu sync.RWMutex

	workers    map[uint64]*Worker
	workQueue  chan *WorkItem
	resultChan chan *WorkResult
	maxWorkers uint64
}

// Worker represents a mining worker
type Worker struct {
	ID        uint64
	IsActive  bool
	WorkCount uint64
	lastWork  time.Time
}

// WorkItem represents a work item for mining
type WorkItem struct {
	Block      *core.Block
	Target     *big.Int
	StartTime  time.Time
	WorkerID   uint64
	Difficulty uint32
	ExtraNonce uint64
}

// WorkResult represents the result of a mining attempt
type WorkResult struct {
	Block     *core.Block
	Nonce     uint64
	WorkerID  uint64
	Duration  time.Duration
	IsSuccess bool
	Hash      []byte
}

// NewWorkDistribution creates a new work distribution manager
func NewWorkDistribution(maxWorkers uint64) *WorkDistribution {
	return &WorkDistribution{
		workers:    make(map[uint64]*Worker),
		workQueue:  make(chan *WorkItem, maxWorkers*2),
		resultChan: make(chan *WorkResult, maxWorkers),
		maxWorkers: maxWorkers,
	}
}

// AddWorker adds a new worker to the distribution pool
func (wd *WorkDistribution) AddWorker(id uint64) error {
	wd.mu.Lock()
	defer wd.mu.Unlock()

	if uint64(len(wd.workers)) >= wd.maxWorkers {
		return ErrMaxWorkersReached
	}

	if _, exists := wd.workers[id]; exists {
		return ErrWorkerExists
	}

	wd.workers[id] = &Worker{
		ID:       id,
		IsActive: true,
		lastWork: time.Now(),
	}

	return nil
}

// RemoveWorker removes a worker from the distribution pool
func (wd *WorkDistribution) RemoveWorker(id uint64) error {
	wd.mu.Lock()
	defer wd.mu.Unlock()

	if _, exists := wd.workers[id]; !exists {
		return ErrWorkerNotFound
	}

	delete(wd.workers, id)
	return nil
}

// DistributeWork distributes work to available workers
func (wd *WorkDistribution) DistributeWork(item *WorkItem) error {
	select {
	case wd.workQueue <- item:
		wd.mu.Lock()
		if worker, ok := wd.workers[item.WorkerID]; ok {
			worker.WorkCount++
			worker.lastWork = time.Now()
		}
		wd.mu.Unlock()
		return nil
	default:
		return ErrWorkQueueFull
	}
}

// GetResult retrieves a mining result
func (wd *WorkDistribution) GetResult() *WorkResult {
	select {
	case result := <-wd.resultChan:
		return result
	default:
		return nil
	}
}

// GetActiveWorkerCount returns the number of active workers
func (wd *WorkDistribution) GetActiveWorkerCount() int {
	wd.mu.RLock()
	defer wd.mu.RUnlock()

	count := 0
	for _, worker := range wd.workers {
		if worker.IsActive {
			count++
		}
	}
	return count
}

// Work errors
var (
	ErrMaxWorkersReached = &workError{"maximum workers reached"}
	ErrWorkerExists      = &workError{"worker already exists"}
	ErrWorkerNotFound    = &workError{"worker not found"}
	ErrWorkQueueFull     = &workError{"work queue is full"}
)

type workError struct {
	message string
}

func (e *workError) Error() string {
	return e.message
}

// CalculateMiningReward calculates the mining reward for a block height
func CalculateMiningReward(height uint64, totalFees uint64, consensus config.ConsensusParams) (blockReward uint64, minerFee uint64, total uint64) {
	blockReward = consensus.MonetaryPolicy.BlockReward(height)
	minerFee = consensus.MonetaryPolicy.MinerFeeAmount(totalFees)

	if blockReward > math.MaxUint64-minerFee {
		return blockReward, minerFee, math.MaxUint64
	}

	total = blockReward + minerFee
	return blockReward, minerFee, total
}

// ValidateWork validates mining work
func ValidateWork(work *WorkItem, result *WorkResult) bool {
	if work == nil || result == nil {
		return false
	}

	if !result.IsSuccess {
		return false
	}

	if result.WorkerID != work.WorkerID {
		return false
	}

	if result.Block == nil {
		return false
	}

	hashInt := new(big.Int).SetBytes(result.Hash)
	if hashInt.Cmp(work.Target) > 0 {
		return false
	}

	return true
}

// EstimateMiningTime estimates time to find a valid block
func EstimateMiningTime(
	hashRate uint64, // hashes per second
	difficulty uint32,
) time.Duration {
	if hashRate == 0 {
		return time.Duration(0)
	}

	target := DifficultyFromBits(difficulty)
	avgHashes := new(big.Float).SetInt(target)
	avgHashes.Quo(avgHashes, new(big.Float).SetFloat64(2.0))

	if !avgHashes.IsInt() {
		avgHashesInt := new(big.Int)
		avgHashes.Int(avgHashesInt)
		avgHashes = new(big.Float).SetInt(avgHashesInt)
	}

	hashesFloat := new(big.Float).SetUint64(uint64(hashRate))
	seconds := new(big.Float).Quo(avgHashes, hashesFloat)

	if !seconds.IsInt() {
		secondsInt := new(big.Int)
		seconds.Int(secondsInt)
		seconds = new(big.Float).SetInt(secondsInt)
	}

	secondsFloat, _ := seconds.Float64()
	return time.Duration(secondsFloat * float64(time.Second))
}
