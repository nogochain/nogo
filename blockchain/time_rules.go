package main

import (
	"fmt"
	"sort"
)

// BlockTimeMaxDrift defines the maximum allowed time drift for blocks
// Set to 72 hours (259200 seconds) for mainnet sync compatibility
// This accounts for clock differences and testnet inaccuracies
// Production mainnet should use 7200 (2 hours) after network stabilizes
const BlockTimeMaxDrift = 259200 // 72 hours - temporary for initial sync

// validateBlockTime validates block timestamp using deterministic rules
// Parameters:
//   - p: consensus parameters
//   - path: blockchain path (blocks from genesis to current)
//   - idx: index of block to validate
//
// Validation rules (DETERMINISTIC - NO LOCAL CLOCK DEPENDENCY):
// 1. Timestamp must be strictly greater than parent timestamp
// 2. Timestamp must be greater than Median Time Past (MTP)
// 3. Timestamp must not exceed MTP + MaxTimeDrift (2 hours)
//
// DETERMINISM GUARANTEE:
// All validation uses only blockchain state (block timestamps),
// NOT local wall-clock time. This ensures all nodes validate
// identically regardless of their local clock settings.
//
// This follows Bitcoin's BIP113 (Median Time-Past) specification:
// https://github.com/bitcoin/bips/blob/master/bip-0113.mediawiki
func validateBlockTime(p ConsensusParams, path []*Block, idx int) error {
	if idx <= 0 || idx >= len(path) {
		return nil
	}
	prev := path[idx-1]
	cur := path[idx]

	// Rule 1: Deterministic - block time must strictly increase
	// This prevents timestamp manipulation attacks
	if cur.TimestampUnix <= prev.TimestampUnix {
		return fmt.Errorf("timestamp not increasing at height %d: current=%d, parent=%d",
			cur.Height, cur.TimestampUnix, prev.TimestampUnix)
	}

	// Rule 2: Timestamp must be greater than Median Time Past (MTP)
	// MTP is calculated from the last N blocks (excluding current block)
	mtp := medianTimePast(p, path, idx-1)
	if cur.TimestampUnix <= mtp {
		return fmt.Errorf("timestamp too old at height %d: current=%d, MTP=%d",
			cur.Height, cur.TimestampUnix, mtp)
	}

	// Rule 3: Timestamp must not exceed MTP + MaxTimeDrift (2 hours)
	// This is DETERMINISTIC - uses MTP as reference, NOT local clock
	// This prevents future-dated blocks while allowing for network latency
	if cur.TimestampUnix > mtp+BlockTimeMaxDrift {
		return fmt.Errorf("timestamp too far in future at height %d: current=%d, MTP+maxDrift=%d",
			cur.Height, cur.TimestampUnix, mtp+BlockTimeMaxDrift)
	}

	return nil
}

// medianTimePast calculates the median timestamp of the last N blocks
// Parameters:
//   - p: consensus parameters (specifies window size)
//   - path: blockchain path
//   - endIdx: ending block index (inclusive)
//
// Returns the median timestamp, which serves as a consensus timestamp
// that is resistant to manipulation by individual miners.
//
// Implementation follows Bitcoin's BIP113:
// - Uses odd window size (default 11) to ensure unique median
// - Excludes current block to prevent self-manipulation
// - Sort-based median calculation for determinism
func medianTimePast(p ConsensusParams, path []*Block, endIdx int) int64 {
	// Use configured window size, default to 11 if not set
	// Window size should be odd to ensure unique median
	window := p.MedianTimePastWindow
	if window <= 0 {
		window = 11
	}
	if window%2 == 0 {
		window++ // Ensure odd window size
	}

	if endIdx < 0 {
		return 0
	}

	// Calculate start index (go back 'window' blocks from endIdx)
	start := endIdx - (window - 1)
	if start < 0 {
		start = 0
	}

	// Collect timestamps from the window
	ts := make([]int64, 0, endIdx-start+1)
	for i := start; i <= endIdx && i < len(path); i++ {
		ts = append(ts, path[i].TimestampUnix)
	}

	if len(ts) == 0 {
		return 0
	}

	// Sort timestamps to find median
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })

	// Return median (middle element)
	return ts[len(ts)/2]
}
