package main

import (
	"fmt"
	"sort"
	"time"
)

func validateBlockTime(p ConsensusParams, path []*Block, idx int) error {
	if idx <= 0 || idx >= len(path) {
		return nil
	}
	prev := path[idx-1]
	cur := path[idx]

	// Deterministic: block time must move forward.
	if cur.TimestampUnix <= prev.TimestampUnix {
		return fmt.Errorf("timestamp not increasing at height %d", cur.Height)
	}

	mtp := medianTimePast(p, path, idx-1)
	if cur.TimestampUnix <= mtp {
		return fmt.Errorf("timestamp too old at height %d", cur.Height)
	}

	// Non-deterministic future bound to prevent extremely future-dated blocks.
	// Note: This intentionally uses local wall-clock time (common in many PoW chains).
	if p.MaxTimeDrift > 0 && cur.TimestampUnix > time.Now().Unix()+p.MaxTimeDrift {
		return fmt.Errorf("timestamp too far in future at height %d", cur.Height)
	}
	return nil
}

func medianTimePast(p ConsensusParams, path []*Block, endIdx int) int64 {
	// Median of timestamps of the last N blocks ending at endIdx (inclusive).
	window := p.MedianTimePastWindow
	if window <= 0 {
		window = 11
	}
	if endIdx < 0 {
		return 0
	}
	start := endIdx - (window - 1)
	if start < 0 {
		start = 0
	}
	ts := make([]int64, 0, endIdx-start+1)
	for i := start; i <= endIdx && i < len(path); i++ {
		ts = append(ts, path[i].TimestampUnix)
	}
	if len(ts) == 0 {
		return 0
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
	return ts[len(ts)/2]
}
