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

import "fmt"

type Mode uint

const (
	ModeNormal Mode = iota
	ModeFake
	ModeTest
)

// Difficulty constants - Production-grade configuration parameters
const (
	minimumDifficulty      = 1    // Minimum difficulty floor (ensures network liveness)
	targetBlockTime        = 17   // Target block time in seconds (economic equilibrium point)
	difficultyBoundDivisor = 2048 // Controls maximum adjustment magnitude (2048 = ~0.05% per second deviation)
	lowDifficultyThreshold = 100  // Threshold for switching to high-precision calculation
	adjustmentSensitivity  = 0.5  // PI controller damping coefficient (50% correction per block)
)

type Config struct {
	PowMode      Mode
	CacheDir     string
	Log          Logger
	Difficulty   *DifficultyConfig
	UseSIMD      bool
	UseBitShift  bool
	ReuseObjects bool
}

type DifficultyConfig struct {
	MinimumDifficulty      uint64  // Minimum difficulty floor
	AdjustmentWindow       uint64  // Number of blocks for difficulty calculation
	TargetBlockTime        uint64  // Target block time in seconds
	BoundDivisor           uint64  // Maximum adjustment bound divisor
	LowDifficultyThreshold uint64  // Threshold for low-difficulty regime
	AdjustmentSensitivity  float64 // PI controller sensitivity (damping coefficient)
	DifficultyBombDelay    uint64  // Delay before difficulty bomb activation
	UseDifficultyBomb      bool    // Enable difficulty bomb (for chain transitions)
}

func DefaultConfig() *Config {
	return &Config{
		PowMode:      ModeNormal,
		CacheDir:     "",
		Log:          &defaultLogger{},
		Difficulty:   DefaultDifficultyConfig(),
		UseSIMD:      false,
		UseBitShift:  false,
		ReuseObjects: true,
	}
}

func DefaultDifficultyConfig() *DifficultyConfig {
	return &DifficultyConfig{
		MinimumDifficulty:      minimumDifficulty,
		AdjustmentWindow:       1,                      // Calculate difficulty every block
		TargetBlockTime:        targetBlockTime,        // 17 seconds target
		BoundDivisor:           difficultyBoundDivisor, // 2048 for smooth adjustment
		LowDifficultyThreshold: lowDifficultyThreshold, // 100 for high-precision regime
		AdjustmentSensitivity:  adjustmentSensitivity,  // 0.5 (50% correction per block)
		DifficultyBombDelay:    0,                      // Disabled by default
		UseDifficultyBomb:      false,                  // Disabled by default
	}
}

type Logger interface {
	Info(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
}

type defaultLogger struct{}

func (l *defaultLogger) Info(msg string, args ...interface{}) {
	println(formatLog("INFO", msg, args...))
}

func (l *defaultLogger) Debug(msg string, args ...interface{}) {
	println(formatLog("DEBUG", msg, args...))
}

func (l *defaultLogger) Error(msg string, args ...interface{}) {
	println(formatLog("ERROR", msg, args...))
}

func (l *defaultLogger) Warn(msg string, args ...interface{}) {
	println(formatLog("WARN", msg, args...))
}

func formatLog(level, msg string, args ...interface{}) string {
	result := "[" + level + "] " + msg
	if len(args) > 0 {
		result += " " + sprintArgs(args...)
	}
	return result
}

func sprintArgs(args ...interface{}) string {
	result := ""
	for i, arg := range args {
		if i > 0 {
			result += " "
		}
		result += fmt.Sprintf("%v", arg)
	}
	return result
}
