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

package config

import (
	"errors"
	"fmt"
	"sort"
)

// TimeRules defines timestamp validation rules
type TimeRules struct {
	// MaxBlockTimeDriftSeconds is the maximum allowed timestamp drift
	MaxBlockTimeDriftSeconds int64

	// MedianTimePastWindow is the window size for MTP calculation
	MedianTimePastWindow int

	// InitialSyncDriftSeconds is the relaxed drift for initial sync
	InitialSyncDriftSeconds int64

	// InitialSyncHeightThreshold is the height threshold for initial sync
	InitialSyncHeightThreshold uint64
}

// DefaultTimeRules returns time rules with default values
func DefaultTimeRules() *TimeRules {
	return &TimeRules{
		MaxBlockTimeDriftSeconds:   7200,
		MedianTimePastWindow:       11,
		InitialSyncDriftSeconds:    7 * 24 * 3600,
		InitialSyncHeightThreshold: 100,
	}
}

// Validate validates time rules
func (t *TimeRules) Validate() error {
	if t.MaxBlockTimeDriftSeconds <= 0 {
		return ErrInvalidMaxTimeDrift
	}

	if t.MedianTimePastWindow <= 0 || t.MedianTimePastWindow%2 == 0 {
		return ErrInvalidMTPWindow
	}

	if t.InitialSyncDriftSeconds <= 0 {
		return ErrInvalidInitialSyncDrift
	}

	return nil
}

// ValidateBlockTime validates block timestamp using deterministic rules
func (t *TimeRules) ValidateBlockTime(p ConsensusParams, path []BlockReader, idx int) error {
	if idx <= 0 || idx >= len(path) {
		return nil
	}
	prev := path[idx-1]
	cur := path[idx]

	if cur.GetTimestamp() <= prev.GetTimestamp() {
		return fmt.Errorf("timestamp not increasing at height %d: current=%d, parent=%d",
			cur.GetHeight(), cur.GetTimestamp(), prev.GetTimestamp())
	}

	mtp := t.medianTimePast(p, path, idx-1)
	if cur.GetTimestamp() <= mtp {
		return fmt.Errorf("timestamp too old at height %d: current=%d, MTP=%d",
			cur.GetHeight(), cur.GetTimestamp(), mtp)
	}

	maxDrift := t.MaxBlockTimeDriftSeconds
	if cur.GetHeight() < t.InitialSyncHeightThreshold {
		maxDrift = t.InitialSyncDriftSeconds
	}

	if cur.GetTimestamp() > mtp+maxDrift {
		return fmt.Errorf("timestamp too far in future at height %d: current=%d, MTP+maxDrift=%d",
			cur.GetHeight(), cur.GetTimestamp(), mtp+maxDrift)
	}

	return nil
}

// medianTimePast calculates the median timestamp of the last N blocks
func (t *TimeRules) medianTimePast(p ConsensusParams, path []BlockReader, endIdx int) int64 {
	window := t.MedianTimePastWindow
	if window <= 0 {
		window = 11
	}
	if window%2 == 0 {
		window++
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
		ts = append(ts, path[i].GetTimestamp())
	}

	if len(ts) == 0 {
		return 0
	}

	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })

	return ts[len(ts)/2]
}

// Error definitions for time rules validation
var (
	ErrInvalidMaxTimeDrift     = errors.New("maxBlockTimeDriftSeconds must be > 0")
	ErrInvalidMTPWindow        = errors.New("medianTimePastWindow must be positive and odd")
	ErrInvalidInitialSyncDrift = errors.New("initialSyncDriftSeconds must be > 0")
)
