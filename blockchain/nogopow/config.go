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
	"fmt"

	"github.com/nogochain/nogo/blockchain/config"
)

type Mode uint

const (
	ModeNormal Mode = iota
	ModeFake
	ModeTest
)

type Config struct {
	PowMode         Mode
	CacheDir        string
	Log             Logger
	ConsensusParams *config.ConsensusParams
	UseSIMD         bool
	UseBitShift     bool
	ReuseObjects    bool
}

func DefaultConfig() *Config {
	return &Config{
		PowMode:  ModeNormal,
		CacheDir: "",
		Log:      &defaultLogger{},
		ConsensusParams: &config.ConsensusParams{
			ChainID:                      1,
			DifficultyEnable:             true,
			BlockTimeTargetSeconds:       17,
			DifficultyAdjustmentInterval: 1,
			MaxBlockTimeDriftSeconds:     900,
			MinDifficulty:                1,
			MaxDifficulty:                4294967295,
			MinDifficultyBits:            1,
			MaxDifficultyBits:            255,
			MaxDifficultyChangePercent:   100, // Increased for faster convergence when network hashrate changes
			MedianTimePastWindow:         11,
			// GenesisDifficultyBits: 100 = target 2^256/100
			// This allows genesis block to be mined quickly on CPU
			// PI controller will adjust upward based on actual hashrate
			GenesisDifficultyBits: 100,
		},
		UseSIMD:      false,
		UseBitShift:  false,
		ReuseObjects: true,
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
