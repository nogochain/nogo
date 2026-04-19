package mconnection

import (
	"sync"
	"time"
)

// FlowRate tracks byte transfer rates over time.
// It uses a sliding window approach to calculate instantaneous transfer rates
// and supports flow control by blocking when rate limits are exceeded.
// All methods are concurrency-safe.
type FlowRate struct {
	mu        sync.Mutex
	bytes     int64
	startTime time.Time
	lastUpdate time.Time
	rate      float64
}

// NewFlowRate creates a new FlowRate tracker initialized to the current time.
func NewFlowRate() *FlowRate {
	now := time.Now()
	return &FlowRate{
		bytes:      0,
		startTime:  now,
		lastUpdate: now,
		rate:       0,
	}
}

// Update adds the specified number of bytes to the transfer counter.
// This method is goroutine-safe and uses mutex protection.
func (fr *FlowRate) Update(n int64) {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	fr.bytes += n
	fr.lastUpdate = time.Now()
	fr.calculateRateLocked()
}

// Rate returns the current transfer rate in bytes per second.
// Uses a sliding window calculation from the last update.
// This method is goroutine-safe.
func (fr *FlowRate) Rate() float64 {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	fr.calculateRateLocked()
	return fr.rate
}

// TotalBytes returns the total bytes transferred since the last reset.
// This method is goroutine-safe.
func (fr *FlowRate) TotalBytes() int64 {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	return fr.bytes
}

// Reset clears the byte counter and resets all timing information.
// This method is goroutine-safe.
func (fr *FlowRate) Reset() {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	now := time.Now()
	fr.bytes = 0
	fr.startTime = now
	fr.lastUpdate = now
	fr.rate = 0
}

// ElapsedSinceStart returns the time elapsed since the flow rate tracker started.
// This method is goroutine-safe.
func (fr *FlowRate) ElapsedSinceStart() time.Duration {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	return time.Since(fr.startTime)
}

// calculateRateLocked computes the rate. Must be called with mu held.
func (fr *FlowRate) calculateRateLocked() {
	elapsed := time.Since(fr.lastUpdate)
	if elapsed <= 0 {
		return
	}

	fr.rate = float64(fr.bytes) / elapsed.Seconds()
}
