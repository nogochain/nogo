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

package network

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/nogochain/nogo/blockchain/core"
)

// envBool reads a boolean from environment variable with default fallback
// Accepts: "true", "1", "yes" (case-insensitive) as true values
func envBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1" || strings.EqualFold(val, "yes")
	}
	return defaultVal
}

// envInt reads an integer from environment variable with default fallback
// Returns defaultVal if env var is empty or invalid
func envInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}

// envUint64 reads a uint64 from environment variable with default fallback
// Returns defaultVal if env var is empty or invalid
func envUint64(key string, defaultVal uint64) uint64 {
	if val := os.Getenv(key); val != "" {
		if uintVal, err := strconv.ParseUint(val, 10, 64); err == nil {
			return uintVal
		}
	}
	return defaultVal
}

// envInt64 reads an int64 from environment variable with default fallback
// Returns defaultVal if env var is empty or invalid
func envInt64(key string, defaultVal int64) int64 {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.ParseInt(val, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultVal
}

// envStr reads a string from environment variable with default fallback
// Returns defaultVal if env var is empty
func envStr(key string, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// mustJSON marshals a value to JSON, panicking on error
// Should only be used for values guaranteed to marshal successfully
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("network: failed to marshal JSON: %v", err)
		return json.RawMessage("{}")
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TxIDHex computes the transaction ID as a hex string
func TxIDHex(tx core.Transaction) (string, error) {
	// Use the core package's TxID function
	return core.TxIDHex(tx)
}
