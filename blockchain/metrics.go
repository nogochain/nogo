package main

import (
	"bytes"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Metrics struct {
	bc    *Blockchain
	mp    *Mempool
	peers *PeerManager

	mu sync.Mutex

	httpRequestsTotal map[string]map[int]int64 // route -> status -> count
	httpDurations     map[string]*histogram    // route -> histogram
}

type histogram struct {
	buckets []float64
	counts  []int64
	sum     float64
	count   int64
}

func newHistogram() *histogram {
	// Prometheus best-practice-ish defaults for HTTP latency.
	b := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	return &histogram{buckets: b, counts: make([]int64, len(b))}
}

func (h *histogram) observe(seconds float64) {
	h.sum += seconds
	h.count++
	for i, le := range h.buckets {
		if seconds <= le {
			h.counts[i]++
		}
	}
}

func NewMetrics(bc *Blockchain, mp *Mempool, peers *PeerManager) *Metrics {
	return &Metrics{
		bc:                bc,
		mp:                mp,
		peers:             peers,
		httpRequestsTotal: map[string]map[int]int64{},
		httpDurations:     map[string]*histogram{},
	}
}

func (m *Metrics) ObserveHTTP(route string, status int, dur time.Duration) {
	if m == nil || route == "" {
		return
	}
	seconds := dur.Seconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.httpRequestsTotal[route] == nil {
		m.httpRequestsTotal[route] = map[int]int64{}
	}
	m.httpRequestsTotal[route][status]++
	h := m.httpDurations[route]
	if h == nil {
		h = newHistogram()
		m.httpDurations[route] = h
	}
	h.observe(seconds)
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if m == nil {
		http.Error(w, "metrics not configured", http.StatusInternalServerError)
		return
	}

	type snap struct {
		httpRequestsTotal map[string]map[int]int64
		httpDurations     map[string]*histogram
	}

	m.mu.Lock()
	s := snap{
		httpRequestsTotal: make(map[string]map[int]int64, len(m.httpRequestsTotal)),
		httpDurations:     make(map[string]*histogram, len(m.httpDurations)),
	}
	for route, byStatus := range m.httpRequestsTotal {
		s.httpRequestsTotal[route] = make(map[int]int64, len(byStatus))
		for code, c := range byStatus {
			s.httpRequestsTotal[route][code] = c
		}
	}
	for route, h := range m.httpDurations {
		cp := &histogram{
			buckets: append([]float64(nil), h.buckets...),
			counts:  append([]int64(nil), h.counts...),
			sum:     h.sum,
			count:   h.count,
		}
		s.httpDurations[route] = cp
	}
	m.mu.Unlock()

	// Chain/mempool gauges.
	height := m.bc.LatestBlock().Height
	difficulty := m.bc.LatestBlock().DifficultyBits
	mempoolSize := 0
	if m.mp != nil {
		mempoolSize = m.mp.Size()
	}
	peersCount := 0
	if m.peers != nil {
		peersCount = len(m.peers.Peers())
	}

	blocksCanonical := float64(height + 1)
	txsCanonical := float64(m.bc.CanonicalTxCount())

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	var buf bytes.Buffer
	writeGauge := func(name string, value any) {
		fmt.Fprintf(&buf, "%s %v\n", name, value)
	}

	buf.WriteString("# HELP nogo_chain_height Current canonical height.\n")
	buf.WriteString("# TYPE nogo_chain_height gauge\n")
	writeGauge("nogo_chain_height", height)

	buf.WriteString("# HELP nogo_difficulty_bits Current difficulty bits.\n")
	buf.WriteString("# TYPE nogo_difficulty_bits gauge\n")
	writeGauge("nogo_difficulty_bits", difficulty)

	buf.WriteString("# HELP nogo_mempool_size Current mempool size.\n")
	buf.WriteString("# TYPE nogo_mempool_size gauge\n")
	writeGauge("nogo_mempool_size", mempoolSize)

	buf.WriteString("# HELP nogo_peers_count Configured peers.\n")
	buf.WriteString("# TYPE nogo_peers_count gauge\n")
	writeGauge("nogo_peers_count", peersCount)

	buf.WriteString("# HELP nogo_blocks_canonical Canonical blocks count.\n")
	buf.WriteString("# TYPE nogo_blocks_canonical gauge\n")
	writeGauge("nogo_blocks_canonical", blocksCanonical)

	buf.WriteString("# HELP nogo_txs_canonical Canonical transactions count.\n")
	buf.WriteString("# TYPE nogo_txs_canonical gauge\n")
	writeGauge("nogo_txs_canonical", txsCanonical)

	buf.WriteString("# HELP go_goroutines Number of goroutines.\n")
	buf.WriteString("# TYPE go_goroutines gauge\n")
	writeGauge("go_goroutines", runtime.NumGoroutine())

	buf.WriteString("# HELP go_memstats_alloc_bytes Bytes allocated and still in use.\n")
	buf.WriteString("# TYPE go_memstats_alloc_bytes gauge\n")
	writeGauge("go_memstats_alloc_bytes", ms.Alloc)

	// HTTP counters + histograms.
	buf.WriteString("# HELP http_requests_total Total HTTP requests by route and status code.\n")
	buf.WriteString("# TYPE http_requests_total counter\n")

	routes := make([]string, 0, len(s.httpRequestsTotal))
	for route := range s.httpRequestsTotal {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		byStatus := s.httpRequestsTotal[route]
		codes := make([]int, 0, len(byStatus))
		for code := range byStatus {
			codes = append(codes, code)
		}
		sort.Ints(codes)
		for _, code := range codes {
			fmt.Fprintf(&buf, "http_requests_total{route=%q,code=%q} %d\n", route, strconv.Itoa(code), byStatus[code])
		}
	}

	buf.WriteString("# HELP http_request_duration_seconds HTTP request latency by route.\n")
	buf.WriteString("# TYPE http_request_duration_seconds histogram\n")
	for _, route := range routes {
		h := s.httpDurations[route]
		if h == nil {
			continue
		}
		var cumulative int64
		for i, le := range h.buckets {
			cumulative += h.counts[i]
			fmt.Fprintf(&buf, "http_request_duration_seconds_bucket{route=%q,le=%q} %d\n", route, fmt.Sprintf("%.3f", le), cumulative)
		}
		fmt.Fprintf(&buf, "http_request_duration_seconds_bucket{route=%q,le=%q} %d\n", route, "+Inf", h.count)
		fmt.Fprintf(&buf, "http_request_duration_seconds_sum{route=%q} %f\n", route, h.sum)
		fmt.Fprintf(&buf, "http_request_duration_seconds_count{route=%q} %d\n", route, h.count)
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}
