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

// Package ntp provides network time synchronization for NogoChain
// This ensures all nodes have synchronized clocks for consensus
package ntp

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/beevik/ntp"
)

const (
	// DefaultSyncInterval is the default interval for NTP synchronization
	DefaultSyncInterval = 10 * time.Minute

	// DefaultMaxDrift is the maximum allowed clock drift before warning
	DefaultMaxDrift = 100 * time.Millisecond

	// DefaultTimeout is the default timeout for NTP requests
	DefaultTimeout = 5 * time.Second

	// DefaultMinServers is the minimum number of NTP servers to query
	DefaultMinServers = 3
)

// TimeSync provides network time synchronization
type TimeSync struct {
	mu              sync.RWMutex
	lastSync        time.Time
	offset          time.Duration
	servers         []string
	syncInterval    time.Duration
	maxDrift        time.Duration
	running         bool
	stopCh          chan struct{}
	wg              sync.WaitGroup
	onDriftExceeded func(offset time.Duration)
}

// NTPServer represents an NTP server configuration
type NTPServer struct {
	Address  string
	Port     int
	Stratum  int
	LastOffset time.Duration
	LastRTT    time.Duration
}

// DefaultNTPServers returns a list of reliable public NTP servers
// These are geographically distributed and maintained by reputable organizations
func DefaultNTPServers() []string {
	return []string{
		// Cloudflare NTP servers (global)
		"time.cloudflare.com",
		// Google NTP servers (global)
		"time.google.com",
		"time1.google.com",
		// NIST NTP servers (US)
		"time.nist.gov",
		// Pool NTP servers (global, geographically distributed)
		"0.pool.ntp.org",
		"1.pool.ntp.org",
		"2.pool.ntp.org",
		"3.pool.ntp.org",
	}
}

// NewTimeSync creates a new time synchronization instance
func NewTimeSync(servers []string, syncInterval, maxDrift time.Duration) *TimeSync {
	if len(servers) == 0 {
		servers = DefaultNTPServers()
	}
	if syncInterval <= 0 {
		syncInterval = DefaultSyncInterval
	}
	if maxDrift <= 0 {
		maxDrift = DefaultMaxDrift
	}

	return &TimeSync{
		servers:      servers,
		syncInterval: syncInterval,
		maxDrift:     maxDrift,
		stopCh:       make(chan struct{}),
	}
}

// SyncResult contains the results of an NTP synchronization
type SyncResult struct {
	Time          time.Time
	Offset        time.Duration
	RTT           time.Duration
	Stratum       int
	ReferenceID   string
	Server        string
	Confidence    float64 // 0.0 to 1.0, based on number of servers and agreement
}

// Synchronize performs a single NTP synchronization
// It queries multiple servers and uses the median offset for accuracy
func (ts *TimeSync) Synchronize(ctx context.Context) (*SyncResult, error) {
	if len(ts.servers) < DefaultMinServers {
		log.Printf("NTP: warning, fewer than %d servers configured", DefaultMinServers)
	}

	// Query multiple NTP servers concurrently
	type serverResult struct {
		server string
		resp   *ntp.Response
		err    error
		rtt    time.Duration
	}

	results := make([]serverResult, 0, len(ts.servers))
	resultCh := make(chan serverResult, len(ts.servers))

	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, DefaultTimeout*2)
	defer cancel()

	// Query all servers concurrently
	for _, server := range ts.servers {
		go func(srv string) {
			start := time.Now()
			resp, err := ntp.QueryWithOptions(srv, ntp.QueryOptions{
				Timeout: DefaultTimeout,
			})
			rtt := time.Since(start)
			resultCh <- serverResult{server: srv, resp: resp, err: err, rtt: rtt}
		}(server)
	}

	// Collect results
	for i := 0; i < len(ts.servers); i++ {
		select {
		case result := <-resultCh:
			if result.err == nil {
				results = append(results, result)
				log.Printf("NTP: server %s - stratum=%d, offset=%v, RTT=%v",
					result.server, result.resp.Stratum, result.resp.ClockOffset, result.rtt)
			} else {
				log.Printf("NTP: server %s failed: %v", result.server, result.err)
			}
		case <-timeoutCtx.Done():
			log.Printf("NTP: synchronization timeout")
			break
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no NTP servers responded")
	}

	// Calculate median offset from successful responses
	offsets := make([]time.Duration, len(results))
	for i, r := range results {
		offsets[i] = r.resp.ClockOffset
	}
	medianOffset := medianDuration(offsets)

	// Calculate weighted average RTT
	totalRTT := time.Duration(0)
	for _, r := range results {
		totalRTT += r.rtt
	}
	avgRTT := totalRTT / time.Duration(len(results))

	// Find best server (lowest stratum with good RTT)
	bestServer := results[0]
	for _, r := range results {
		if r.resp.Stratum < bestServer.resp.Stratum ||
			(r.resp.Stratum == bestServer.resp.Stratum && r.rtt < bestServer.rtt) {
			bestServer = r
		}
	}

	// Calculate confidence based on agreement between servers
	confidence := calculateConfidence(offsets, medianOffset)

	result := &SyncResult{
		Time:        time.Now().Add(medianOffset),
		Offset:      medianOffset,
		RTT:         avgRTT,
		Stratum:     int(bestServer.resp.Stratum),
		ReferenceID: bestServer.resp.ReferenceString(),
		Server:      bestServer.server,
		Confidence:  confidence,
	}

	// Update internal state
	ts.mu.Lock()
	ts.lastSync = time.Now()
	ts.offset = medianOffset
	ts.mu.Unlock()

	// Check if drift exceeds threshold
	if ts.onDriftExceeded != nil {
		if math.Abs(medianOffset.Seconds()) > ts.maxDrift.Seconds() {
			log.Printf("NTP: clock drift exceeded threshold: %v (max: %v)",
				medianOffset, ts.maxDrift)
			ts.onDriftExceeded(medianOffset)
		}
	}

	log.Printf("NTP: synchronized - offset=%v, RTT=%v, confidence=%.2f, servers=%d/%d",
		medianOffset, avgRTT, confidence, len(results), len(ts.servers))

	return result, nil
}

// Start begins automatic time synchronization
func (ts *TimeSync) Start(ctx context.Context) error {
	ts.mu.Lock()
	if ts.running {
		ts.mu.Unlock()
		return fmt.Errorf("time sync already running")
	}
	ts.running = true
	ts.mu.Unlock()

	ts.wg.Add(1)
	go func() {
		defer ts.wg.Done()

		// Perform initial synchronization
		if _, err := ts.Synchronize(ctx); err != nil {
			log.Printf("NTP: initial synchronization failed: %v", err)
		}

		// Periodic synchronization
		ticker := time.NewTicker(ts.syncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("NTP: stopping synchronization")
				return
			case <-ts.stopCh:
				log.Printf("NTP: stopping synchronization")
				return
			case <-ticker.C:
				if _, err := ts.Synchronize(ctx); err != nil {
					log.Printf("NTP: synchronization failed: %v", err)
				}
			}
		}
	}()

	return nil
}

// Stop halts automatic time synchronization
func (ts *TimeSync) Stop() {
	ts.mu.Lock()
	if !ts.running {
		ts.mu.Unlock()
		return
	}
	ts.running = false
	ts.mu.Unlock()

	close(ts.stopCh)
	ts.wg.Wait()
}

// Now returns the current synchronized time
// Falls back to local time if synchronization has not occurred
func (ts *TimeSync) Now() time.Time {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	if ts.lastSync.IsZero() {
		// No synchronization yet, use local time
		return time.Now()
	}

	// Return synchronized time (local time + offset)
	return time.Now().Add(ts.offset)
}

// NowUnix returns the current synchronized Unix timestamp
func (ts *TimeSync) NowUnix() int64 {
	return ts.Now().Unix()
}

// Offset returns the current time offset from local time
func (ts *TimeSync) Offset() time.Duration {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.offset
}

// LastSync returns the time of the last successful synchronization
func (ts *TimeSync) LastSync() time.Time {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.lastSync
}

// IsSynchronized returns true if time synchronization is active
func (ts *TimeSync) IsSynchronized() bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.running && !ts.lastSync.IsZero()
}

// SetOnDriftExceeded sets a callback for when clock drift exceeds threshold
func (ts *TimeSync) SetOnDriftExceeded(callback func(offset time.Duration)) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.onDriftExceeded = callback
}

// GetStatus returns current synchronization status
func (ts *TimeSync) GetStatus() map[string]interface{} {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return map[string]interface{}{
		"synchronized":  ts.running && !ts.lastSync.IsZero(),
		"last_sync":     ts.lastSync,
		"offset":        ts.offset.String(),
		"offset_ms":     ts.offset.Milliseconds(),
		"sync_interval": ts.syncInterval.String(),
		"max_drift":     ts.maxDrift.String(),
		"servers":       len(ts.servers),
	}
}

// medianDuration calculates the median of a slice of durations
func medianDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	// Sort durations
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Return median
	return sorted[len(sorted)/2]
}

// calculateConfidence calculates confidence based on server agreement
// Returns a value between 0.0 and 1.0
func calculateConfidence(offsets []time.Duration, median time.Duration) float64 {
	if len(offsets) == 0 {
		return 0.0
	}

	// Calculate standard deviation
	var sumSquares float64
	for _, offset := range offsets {
		diff := float64(offset - median)
		sumSquares += diff * diff
	}
	stdDev := math.Sqrt(sumSquares / float64(len(offsets)))

	// Convert to confidence (lower stdDev = higher confidence)
	// Confidence drops significantly if stdDev > 50ms
	confidence := 1.0 / (1.0 + stdDev*20.0)

	// Boost confidence based on number of servers
	serverFactor := math.Min(1.0, float64(len(offsets))/5.0)

	return confidence * (0.5 + 0.5*serverFactor)
}

// GlobalTimeSync is the global time synchronization instance
var GlobalTimeSync *TimeSync

// InitGlobalTimeSync initializes the global time synchronization
func InitGlobalTimeSync(servers []string, syncInterval, maxDrift time.Duration) {
	GlobalTimeSync = NewTimeSync(servers, syncInterval, maxDrift)
}

// GetGlobalTimeSync returns the global time synchronization instance
func GetGlobalTimeSync() *TimeSync {
	return GlobalTimeSync
}

// Now returns synchronized time from global instance
func Now() time.Time {
	if GlobalTimeSync != nil {
		return GlobalTimeSync.Now()
	}
	return time.Now()
}

// NowUnix returns synchronized Unix timestamp from global instance
func NowUnix() int64 {
	if GlobalTimeSync != nil {
		return GlobalTimeSync.NowUnix()
	}
	return time.Now().Unix()
}
