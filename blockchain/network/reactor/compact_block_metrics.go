package reactor

import (
	"sync/atomic"
	"time"
)

// CompactBlockMetrics tracks compact block relay performance.
// All counters are lock-free via atomic operations.
// Exported for integration with external monitoring (e.g., Prometheus).
type CompactBlockMetrics struct {
	// Counters
	CompactBlocksReceived  atomic.Uint64
	CompactBlocksSent      atomic.Uint64
	FullReconstructions    atomic.Uint64 // 100% mempool hit
	PartialReconstructions atomic.Uint64 // needed missing tx requests
	FailedReconstructions  atomic.Uint64 // timed out or permanently failed
	MissingTxRequestsSent  atomic.Uint64
	MissingTxResponsesRecv atomic.Uint64

	// Timing (cumulative, nanoseconds)
	totalReconTimeNanos atomic.Uint64
	totalReconCount     atomic.Uint64

	// Coverage tracking
	totalTxMatched atomic.Uint64 // transactions matched from mempool
	totalTxMissing atomic.Uint64 // transactions requested remotely
}

// RecordReconstruction records a compact block reconstruction result.
func (m *CompactBlockMetrics) RecordReconstruction(matched, missing int, duration time.Duration) {
	m.totalReconCount.Add(1)
	m.totalReconTimeNanos.Add(uint64(duration.Nanoseconds()))
	m.totalTxMatched.Add(uint64(matched))
	m.totalTxMissing.Add(uint64(missing))

	if missing == 0 {
		m.FullReconstructions.Add(1)
	} else {
		m.PartialReconstructions.Add(1)
	}
}

// AverageReconTime returns the average reconstruction time.
func (m *CompactBlockMetrics) AverageReconTime() time.Duration {
	count := m.totalReconCount.Load()
	if count == 0 {
		return 0
	}
	return time.Duration(m.totalReconTimeNanos.Load() / count)
}

// MemPoolHitRate returns the current mempool hit rate from historical data.
func (m *CompactBlockMetrics) MemPoolHitRate() float64 {
	matched := m.totalTxMatched.Load()
	missing := m.totalTxMissing.Load()
	total := matched + missing
	if total == 0 {
		return 0
	}
	return float64(matched) / float64(total)
}

// Snapshot returns an atomic point-in-time summary of all metrics.
func (m *CompactBlockMetrics) Snapshot() CompactBlockMetricsSnapshot {
	return CompactBlockMetricsSnapshot{
		CompactBlocksReceived:  m.CompactBlocksReceived.Load(),
		CompactBlocksSent:      m.CompactBlocksSent.Load(),
		FullReconstructions:    m.FullReconstructions.Load(),
		PartialReconstructions: m.PartialReconstructions.Load(),
		FailedReconstructions:  m.FailedReconstructions.Load(),
		MemPoolHitRate:         m.MemPoolHitRate(),
		AverageReconTime:       m.AverageReconTime(),
	}
}

// CompactBlockMetricsSnapshot is a point-in-time copy of compact block metrics.
// Exported for JSON serialization (RPC / admin API).
type CompactBlockMetricsSnapshot struct {
	CompactBlocksReceived  uint64        `json:"compact_blocks_received"`
	CompactBlocksSent      uint64        `json:"compact_blocks_sent"`
	FullReconstructions    uint64        `json:"full_reconstructions"`
	PartialReconstructions uint64        `json:"partial_reconstructions"`
	FailedReconstructions  uint64        `json:"failed_reconstructions"`
	MemPoolHitRate         float64       `json:"mempool_hit_rate"`
	AverageReconTime       time.Duration `json:"avg_recon_time_ns"`
}
