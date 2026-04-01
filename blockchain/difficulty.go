package main

import "time"

const maxDifficultyBits = uint32(256)

type ConsensusParams struct {
	DifficultyEnable bool

	TargetBlockTime   time.Duration
	DifficultyWindow  int
	DifficultyMaxStep uint32

	MinDifficultyBits uint32
	MaxDifficultyBits uint32

	GenesisDifficultyBits uint32

	MedianTimePastWindow int
	MaxTimeDrift         int64
	MaxBlockSize         uint64

	// MerkleEnable gates a new block header commitment scheme (v2 blocks).
	// When enabled, blocks at height >= MerkleActivationHeight must use Version=2 and commit to a Merkle root.
	MerkleEnable           bool
	MerkleActivationHeight uint64

	// BinaryEncodingEnable switches consensus-critical hashing away from JSON serialization
	// (tx signing hash / txid, and PoW header hashing).
	//
	// If enabled, blocks at height >= BinaryEncodingActivationHeight must use the binary
	// encoding scheme for:
	// - Transaction signing hash (and therefore txid)
	// - Block header hashing (PoW)
	BinaryEncodingEnable           bool
	BinaryEncodingActivationHeight uint64

	// MonetaryPolicy defines block subsidy + fee allocation rules.
	MonetaryPolicy MonetaryPolicy
}

func defaultConsensusParamsFromEnv() ConsensusParams {
	p := ConsensusParams{
		DifficultyEnable:      envBool("DIFFICULTY_ENABLE", false),
		TargetBlockTime:       envDurationMS("DIFFICULTY_TARGET_MS", 15*time.Second),
		DifficultyWindow:      envInt("DIFFICULTY_WINDOW", 20),
		DifficultyMaxStep:     envUint32("DIFFICULTY_MAX_STEP", 1),
		MinDifficultyBits:     envUint32("DIFFICULTY_MIN_BITS", 1),
		MaxDifficultyBits:     envUint32("DIFFICULTY_MAX_BITS", 255),
		GenesisDifficultyBits: envUint32("GENESIS_DIFFICULTY_BITS", defaultDifficultyBits),
		MedianTimePastWindow:  envInt("MTP_WINDOW", 11),
		MaxTimeDrift:          envInt64("MAX_TIME_DRIFT", envInt64("MAX_FUTURE_DRIFT_SEC", 2*60*60)),
		MaxBlockSize:          envUint64("MAX_BLOCK_SIZE", 1_000_000),
		MerkleEnable:          envBool("MERKLE_ENABLE", false),
		MerkleActivationHeight: envUint64(
			"MERKLE_ACTIVATION_HEIGHT",
			0,
		),
		BinaryEncodingEnable: envBool("BINARY_ENCODING_ENABLE", false),
		BinaryEncodingActivationHeight: envUint64(
			"BINARY_ENCODING_ACTIVATION_HEIGHT",
			0,
		),
	}

	if p.TargetBlockTime <= 0 {
		p.TargetBlockTime = 15 * time.Second
	}
	if p.DifficultyWindow <= 0 {
		p.DifficultyWindow = 20
	}
	if p.DifficultyMaxStep == 0 {
		p.DifficultyMaxStep = 1
	}
	if p.MinDifficultyBits == 0 {
		p.MinDifficultyBits = 1
	}
	if p.MaxDifficultyBits == 0 {
		p.MaxDifficultyBits = 255
	}
	if p.MaxDifficultyBits > maxDifficultyBits {
		p.MaxDifficultyBits = maxDifficultyBits
	}
	if p.MinDifficultyBits > p.MaxDifficultyBits {
		p.MinDifficultyBits = p.MaxDifficultyBits
	}
	if p.GenesisDifficultyBits < p.MinDifficultyBits {
		p.GenesisDifficultyBits = p.MinDifficultyBits
	}
	if p.GenesisDifficultyBits > p.MaxDifficultyBits {
		p.GenesisDifficultyBits = p.MaxDifficultyBits
	}
	if p.MedianTimePastWindow <= 0 {
		p.MedianTimePastWindow = 11
	}
	if p.MaxTimeDrift <= 0 {
		p.MaxTimeDrift = 2 * 60 * 60
	}
	if p.MaxBlockSize == 0 {
		p.MaxBlockSize = 1_000_000
	}
	return p
}

func (p ConsensusParams) BinaryEncodingActive(height uint64) bool {
	return p.BinaryEncodingEnable && height >= p.BinaryEncodingActivationHeight
}

func (bc *Blockchain) NextDifficultyBits() uint32 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return nextDifficultyBitsFromPath(bc.consensus, bc.blocks)
}

func nextDifficultyBitsFromPath(p ConsensusParams, path []*Block) uint32 {
	if len(path) == 0 {
		return p.GenesisDifficultyBits
	}
	parentIdx := len(path) - 1
	parent := path[parentIdx]

	if !p.DifficultyEnable {
		if parent.DifficultyBits == 0 {
			return p.GenesisDifficultyBits
		}
		return clampDifficultyBits(p, parent.DifficultyBits)
	}

	// Calculate difficulty adjustment for every block starting from block 1
	// Use adaptive window: smaller window for early blocks, full window after enough history
	windowSize := p.DifficultyWindow
	if parentIdx < windowSize {
		// Early blocks: use available history (at least 1 block)
		if parentIdx == 0 {
			// First block after genesis: no adjustment yet, use genesis difficulty
			return clampDifficultyBits(p, p.GenesisDifficultyBits)
		}
		windowSize = parentIdx
	}

	olderIdx := parentIdx - windowSize
	older := path[olderIdx]
	actualSpanSec := parent.TimestampUnix - older.TimestampUnix
	if actualSpanSec <= 0 {
		actualSpanSec = 1
	}

	targetSec := int64(p.TargetBlockTime / time.Second)
	if targetSec <= 0 {
		targetSec = 1
	}
	expectedSpanSec := int64(windowSize) * targetSec
	if expectedSpanSec <= 0 {
		expectedSpanSec = 1
	}
	// Prevent extreme time-warp from heavily biasing adjustment decisions.
	if actualSpanSec < expectedSpanSec/4 {
		actualSpanSec = expectedSpanSec / 4
	}
	if actualSpanSec > expectedSpanSec*4 {
		actualSpanSec = expectedSpanSec * 4
	}

	// Calculate adjustment ratio: actual / expected
	// If blocks are too fast (ratio < 1), increase difficulty
	// If blocks are too slow (ratio > 1), decrease difficulty
	adjustmentRatio := float64(actualSpanSec) / float64(expectedSpanSec)

	next := float64(parent.DifficultyBits)

	// Use percentage-based adjustment for more responsive difficulty
	// Target: adjust difficulty proportionally to block time deviation
	const sensitivity = 0.5 // 50% adjustment per window
	if adjustmentRatio < 1.0 {
		// Blocks too fast: increase difficulty
		increaseFactor := (1.0 - adjustmentRatio) * sensitivity
		next = next * (1.0 + increaseFactor)
	} else if adjustmentRatio > 1.0 {
		// Blocks too slow: decrease difficulty
		decreaseFactor := (adjustmentRatio - 1.0) * sensitivity
		next = next * (1.0 - decreaseFactor)
	}

	// Apply minimum change of 1 for low difficulties
	if parent.DifficultyBits <= 10 {
		if adjustmentRatio < 1.0 && next <= float64(parent.DifficultyBits) {
			next = float64(parent.DifficultyBits) + 1
		} else if adjustmentRatio > 1.0 && next >= float64(parent.DifficultyBits) {
			if next > 1 {
				next = float64(parent.DifficultyBits) - 1
			}
		}
	}

	// Apply max step limit
	maxChange := float64(p.DifficultyMaxStep)
	if next > float64(parent.DifficultyBits)+maxChange {
		next = float64(parent.DifficultyBits) + maxChange
	}
	if next < float64(parent.DifficultyBits)-maxChange {
		next = float64(parent.DifficultyBits) - maxChange
	}

	if next < 1 {
		next = 1
	}
	return clampDifficultyBits(p, uint32(next))
}

func expectedDifficultyBitsForBlockIndex(p ConsensusParams, path []*Block, idx int) uint32 {
	if idx <= 0 || idx >= len(path) {
		return 0
	}
	// Expected bits for block idx are the same bits we'd compute for "next" given parent at idx-1.
	parentPath := path[:idx]
	return nextDifficultyBitsFromPath(p, parentPath)
}

func clampDifficultyBits(p ConsensusParams, bits uint32) uint32 {
	if bits < 1 {
		bits = 1
	}
	if bits > maxDifficultyBits {
		bits = maxDifficultyBits
	}
	if bits < p.MinDifficultyBits {
		return p.MinDifficultyBits
	}
	if bits > p.MaxDifficultyBits {
		return p.MaxDifficultyBits
	}
	return bits
}
