package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/nogochain/nogo/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	bc    *Blockchain
	mp    *Mempool
	peers *PeerManager
	sync  *SyncLoop

	mu sync.Mutex

	httpRequestsTotal map[string]map[int]int64 // route -> status -> count
	httpDurations     map[string]*histogram    // route -> histogram

	nodeID    string
	chainID   uint64
	inflation *InflationCalculator

	nogoMempoolSize           prometheus.Gauge
	nogoMempoolBytes          prometheus.Gauge
	nogoSyncProgressPercent   prometheus.Gauge
	nogoBlockVerificationDur  prometheus.Histogram
	nogoTxVerificationDur     prometheus.Histogram
	nogoPeerScoreDistribution prometheus.Histogram
	nogoInflationRate         prometheus.Gauge
	nogoChainHeight           prometheus.Gauge
	nogoDifficultyBits        prometheus.Gauge
	nogoPeersCount            prometheus.Gauge
	nogoBlocksCanonical       prometheus.Gauge
	nogoTxsCanonical          prometheus.Gauge
	nogoPeerSuccessRate       prometheus.Gauge
	nogoPeerLatencyAvg        prometheus.Gauge
	nogoBlockReward           prometheus.Gauge
	nogoAnnualReductionRate   prometheus.Gauge
}

type histogram struct {
	buckets []float64
	counts  []int64
	sum     float64
	count   int64
}

func newHistogram() *histogram {
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

func NewMetrics(bc *Blockchain, mp *Mempool, peers *PeerManager, sl *SyncLoop, nodeID string, chainID uint64) *Metrics {
	m := &Metrics{
		bc:                bc,
		mp:                mp,
		peers:             peers,
		sync:              sl,
		nodeID:            nodeID,
		chainID:           chainID,
		httpRequestsTotal: map[string]map[int]int64{},
		httpDurations:     map[string]*histogram{},
		inflation:         NewInflationCalculator(),
	}

	commonLabels := prometheus.Labels{
		"node_id":  nodeID,
		"chain_id": strconv.FormatUint(chainID, 10),
	}

	m.nogoMempoolSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_mempool_size",
		Help:        "Current number of transactions in mempool",
		ConstLabels: commonLabels,
	})

	m.nogoMempoolBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_mempool_bytes",
		Help:        "Total size of mempool in bytes",
		ConstLabels: commonLabels,
	})

	m.nogoSyncProgressPercent = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_sync_progress_percent",
		Help:        "Sync progress percentage (0-100)",
		ConstLabels: commonLabels,
	})

	m.nogoBlockVerificationDur = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:        "nogo_block_verification_duration_seconds",
		Help:        "Block verification latency in seconds",
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		ConstLabels: commonLabels,
	})

	m.nogoTxVerificationDur = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:        "nogo_transaction_verification_duration_seconds",
		Help:        "Transaction verification latency in seconds",
		Buckets:     []float64{0.0001, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5},
		ConstLabels: commonLabels,
	})

	m.nogoPeerScoreDistribution = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:        "nogo_peer_score_distribution",
		Help:        "Distribution of peer scores",
		Buckets:     []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		ConstLabels: commonLabels,
	})

	m.nogoInflationRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_inflation_rate",
		Help:        "Current inflation rate percentage",
		ConstLabels: commonLabels,
	})

	m.nogoChainHeight = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_chain_height",
		Help:        "Current canonical height",
		ConstLabels: commonLabels,
	})

	m.nogoDifficultyBits = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_difficulty_bits",
		Help:        "Current difficulty bits",
		ConstLabels: commonLabels,
	})

	m.nogoPeersCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_peers_count",
		Help:        "Number of connected peers",
		ConstLabels: commonLabels,
	})

	m.nogoBlocksCanonical = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_blocks_canonical",
		Help:        "Canonical blocks count",
		ConstLabels: commonLabels,
	})

	m.nogoTxsCanonical = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_txs_canonical",
		Help:        "Canonical transactions count",
		ConstLabels: commonLabels,
	})

	m.nogoPeerSuccessRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_peer_success_rate",
		Help:        "Average peer success rate",
		ConstLabels: commonLabels,
	})

	m.nogoPeerLatencyAvg = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_peer_latency_avg",
		Help:        "Average peer latency in milliseconds",
		ConstLabels: commonLabels,
	})

	m.nogoBlockReward = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_block_reward",
		Help:        "Current block reward in NOGO",
		ConstLabels: commonLabels,
	})

	m.nogoAnnualReductionRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_annual_reduction_rate",
		Help:        "Annual block reward reduction rate percentage",
		ConstLabels: commonLabels,
	})

	m.nogoAnnualReductionRate.Set(10.0)

	return m
}

// InflationCalculator calculates real-time inflation rate
type InflationCalculator struct {
	mu sync.RWMutex
}

// NewInflationCalculator creates a new inflation calculator
func NewInflationCalculator() *InflationCalculator {
	return &InflationCalculator{}
}

// CalculateInflationRate calculates current annual inflation rate
// Formula: (annual_block_production * block_reward) / total_supply * 100
func (ic *InflationCalculator) CalculateInflationRate(currentHeight uint64, currentSupply uint64) float64 {
	ic.mu.RLock()
	defer ic.mu.RUnlock()

	if currentSupply == 0 {
		return 0
	}

	blocksPerYear := config.GetBlocksPerYear()
	policy := MonetaryPolicy{
		InitialBlockReward:     initialBlockRewardWei.Uint64(),
		AnnualReductionPercent: 10,
		MinimumBlockReward:     minimumBlockRewardWei.Uint64(),
	}

	currentReward := policy.BlockReward(currentHeight)
	annualEmission := blocksPerYear * currentReward

	inflationRate := float64(annualEmission) / float64(currentSupply) * 100.0
	return inflationRate
}

// ObserveHTTP records HTTP request metrics
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

// ObserveBlockVerification records block verification metrics
func (m *Metrics) ObserveBlockVerification(duration time.Duration) {
	if m == nil {
		return
	}
	m.nogoBlockVerificationDur.Observe(duration.Seconds())
}

// ObserveTransactionVerification records transaction verification metrics
func (m *Metrics) ObserveTransactionVerification(duration time.Duration) {
	if m == nil {
		return
	}
	m.nogoTxVerificationDur.Observe(duration.Seconds())
}

// UpdateMempoolMetrics updates mempool size and bytes metrics
func (m *Metrics) UpdateMempoolMetrics() {
	if m == nil || m.mp == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	size := m.mp.Size()
	m.nogoMempoolSize.Set(float64(size))

	entries := m.mp.Snapshot()
	totalBytes := 0
	for _, entry := range entries {
		txBytes, _ := json.Marshal(entry.tx)
		totalBytes += len(txBytes)
	}
	m.nogoMempoolBytes.Set(float64(totalBytes))
}

// UpdateSyncProgress updates sync progress metric
func (m *Metrics) UpdateSyncProgress() {
	if m == nil || m.sync == nil || m.bc == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	localHeight := m.bc.LatestBlock().Height

	var networkHeight uint64
	if m.sync.pm != nil {
		peers := m.sync.pm.Peers()
		for _, peer := range peers {
			info, err := m.sync.pm.FetchChainInfo(nil, peer)
			if err != nil {
				continue
			}
			if info.Height > networkHeight {
				networkHeight = info.Height
			}
		}
	}

	if networkHeight == 0 || localHeight >= networkHeight {
		m.nogoSyncProgressPercent.Set(100.0)
	} else {
		progress := float64(localHeight) / float64(networkHeight) * 100.0
		m.nogoSyncProgressPercent.Set(progress)
	}
}

// UpdatePeerMetrics updates peer-related metrics
func (m *Metrics) UpdatePeerMetrics() {
	if m == nil || m.peers == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	peersCount := len(m.peers.Peers())
	m.nogoPeersCount.Set(float64(peersCount))
}

// UpdateChainMetrics updates chain-related metrics
func (m *Metrics) UpdateChainMetrics() {
	if m == nil || m.bc == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	height := m.bc.LatestBlock().Height
	difficulty := m.bc.LatestBlock().DifficultyBits
	txsCount := m.bc.CanonicalTxCount()

	m.nogoChainHeight.Set(float64(height))
	m.nogoDifficultyBits.Set(float64(difficulty))
	m.nogoBlocksCanonical.Set(float64(height + 1))
	m.nogoTxsCanonical.Set(float64(txsCount))

	policy := m.bc.consensus.MonetaryPolicy
	currentReward := policy.BlockReward(height)
	rewardInNogo := float64(currentReward) / float64(NogoNOGO)
	m.nogoBlockReward.Set(rewardInNogo)

	totalSupply := m.calculateTotalSupply(height)
	if totalSupply > 0 && m.inflation != nil {
		inflationRate := m.inflation.CalculateInflationRate(height, totalSupply)
		m.nogoInflationRate.Set(inflationRate)
	}
}

// calculateTotalSupply calculates total NOGO supply at given height
func (m *Metrics) calculateTotalSupply(height uint64) uint64 {
	if height == 0 {
		genesisBlock, _ := m.bc.BlockByHeight(0)
		if genesisBlock != nil && len(genesisBlock.Transactions) > 0 {
			return genesisBlock.Transactions[0].Amount
		}
		return 0
	}

	policy := m.bc.consensus.MonetaryPolicy
	totalSupply := uint64(0)

	for h := uint64(0); h <= height; h++ {
		blockReward := policy.BlockReward(h)
		totalSupply += blockReward

		if block, exists := m.bc.BlockByHeight(h); exists {
			var totalFees uint64
			for _, tx := range block.Transactions {
				if tx.Type == TxTransfer {
					totalFees += tx.Fee
				}
			}
			totalSupply += policy.MinerFeeAmount(totalFees)
		}
	}

	return totalSupply
}

// UpdateAllMetrics updates all metrics (called periodically)
func (m *Metrics) UpdateAllMetrics() {
	if m == nil {
		return
	}
	m.UpdateChainMetrics()
	m.UpdateMempoolMetrics()
	m.UpdateSyncProgress()
	m.UpdatePeerMetrics()
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

	m.UpdateAllMetrics()

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

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	var buf bytes.Buffer
	writeGauge := func(name string, value any) {
		fmt.Fprintf(&buf, "%s %v\n", name, value)
	}

	buf.WriteString("# HELP go_goroutines Number of goroutines.\n")
	buf.WriteString("# TYPE go_goroutines gauge\n")
	writeGauge("go_goroutines", runtime.NumGoroutine())

	buf.WriteString("# HELP go_memstats_alloc_bytes Bytes allocated and still in use.\n")
	buf.WriteString("# TYPE go_memstats_alloc_bytes gauge\n")
	writeGauge("go_memstats_alloc_bytes", ms.Alloc)

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
