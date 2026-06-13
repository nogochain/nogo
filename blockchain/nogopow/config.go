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

// DefaultConfig returns a Config with safe defaults.
// ConsensusParams is intentionally nil - callers MUST set it before use.
// All production callers already override it with Chain.c.consensus.
// NewDifficultyAdjuster() has internal nil-check fallback for standalone mode.
func DefaultConfig() *Config {
	return &Config{
		PowMode:      ModeNormal,
		CacheDir:     "",
		Log:          &defaultLogger{},
		UseSIMD:      false,
		UseBitShift:  false,
		ReuseObjects: true,
	}
}

// LogLevel represents the logging level
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// GlobalLogLevel controls the minimum log level to display
// Production environments should set this to LogLevelInfo or higher
var GlobalLogLevel = LogLevelInfo

type Logger interface {
	Info(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
}

type defaultLogger struct{}

func (l *defaultLogger) Info(msg string, args ...interface{}) {
	if GlobalLogLevel <= LogLevelInfo {
		println(formatLog("INFO", msg, args...))
	}
}

func (l *defaultLogger) Debug(msg string, args ...interface{}) {
	if GlobalLogLevel <= LogLevelDebug {
		println(formatLog("DEBUG", msg, args...))
	}
}

func (l *defaultLogger) Error(msg string, args ...interface{}) {
	if GlobalLogLevel <= LogLevelError {
		println(formatLog("ERROR", msg, args...))
	}
}

func (l *defaultLogger) Warn(msg string, args ...interface{}) {
	if GlobalLogLevel <= LogLevelWarn {
		println(formatLog("WARN", msg, args...))
	}
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
