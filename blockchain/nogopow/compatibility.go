// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License,
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
	"fmt"
	"math/big"

	"github.com/nogochain/nogo/blockchain/config"
)

// BlockchainCompatibility provides compatibility layer for blockchain package
// This allows the blockchain to use nogopow difficulty adjustment seamlessly

// BlockHeader represents a minimal block header for difficulty calculation
type BlockHeader struct {
	Height         uint64
	TimestampUnix  int64
	DifficultyBits uint32
	PrevHash       []byte
	Hash           []byte
}

// DifficultyCalculator provides difficulty calculation services
type DifficultyCalculator struct {
	adjuster        *DifficultyAdjuster
	consensusParams *config.ConsensusParams
}

// NewDifficultyCalculator creates a new difficulty calculator
func NewDifficultyCalculator(consensusParams *config.ConsensusParams) *DifficultyCalculator {
	if consensusParams == nil {
		consensusParams = &config.ConsensusParams{
			BlockTimeTargetSeconds:     15,
			MaxDifficultyChangePercent: 20,
		}
	}

	return &DifficultyCalculator{
		adjuster:        NewDifficultyAdjuster(consensusParams),
		consensusParams: consensusParams,
	}
}

// CalcNextDifficulty calculates difficulty for next block given parent block
func (dc *DifficultyCalculator) CalcNextDifficulty(parent *BlockHeader, currentTime uint64) uint32 {
	if parent == nil {
		return uint32(dc.consensusParams.MinDifficulty)
	}

	// Convert parent to nogopow.Header format
	parentHeader := &Header{
		Number:     big.NewInt(int64(parent.Height)),
		Time:       uint64(parent.TimestampUnix),
		Difficulty: big.NewInt(int64(parent.DifficultyBits)),
	}

	// Calculate new difficulty using PI controller
	newDifficulty := dc.adjuster.CalcDifficulty(currentTime, parentHeader)

	// Convert to uint32, ensuring it fits within bounds
	bits := newDifficulty.Uint64()
	if bits > 256 {
		bits = 256
	}
	if bits < 1 {
		bits = 1
	}

	return uint32(bits)
}

// ValidateDifficulty validates block difficulty against expected value
func (dc *DifficultyCalculator) ValidateDifficulty(parent, current *BlockHeader) error {
	if parent == nil || current == nil {
		return nil
	}

	// Calculate expected difficulty
	expected := dc.CalcNextDifficulty(parent, uint64(current.TimestampUnix))

	// Check if actual difficulty matches expected
	if current.DifficultyBits != expected {
		return &DifficultyMismatchError{
			Height:   current.Height,
			Expected: expected,
			Got:      current.DifficultyBits,
		}
	}

	return nil
}

// DifficultyMismatchError represents a difficulty validation error
type DifficultyMismatchError struct {
	Height   uint64
	Expected uint32
	Got      uint32
}

func (e *DifficultyMismatchError) Error() string {
	return fmt.Sprintf("bad difficulty at height %d: expected %d got %d", e.Height, e.Expected, e.Got)
}

// GetMinimumDifficulty returns the minimum difficulty value
func (dc *DifficultyCalculator) GetMinimumDifficulty() uint32 {
	return uint32(dc.consensusParams.MinDifficulty)
}

// GetMaximumDifficulty returns the maximum difficulty value (256 bits)
func (dc *DifficultyCalculator) GetMaximumDifficulty() uint32 {
	return maxDifficultyBits
}

// Constants for compatibility
const maxDifficultyBits = uint32(256)
