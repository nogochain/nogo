package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	secondsPerYear = 365 * 24 * 60 * 60
	NogoWei        = 1
	NogoNOGO       = 100_000_000
)

const (
	TxTransfer = core.TxTransfer
)

var (
	initialBlockRewardWei = big.NewInt(5000000000000000000)
	minimumBlockRewardWei = big.NewInt(1000000000000000000)
)

func getBlocksPerYear(targetBlockTimeSec int64) uint64 {
	if targetBlockTimeSec <= 0 {
		targetBlockTimeSec = 15
	}
	return uint64(secondsPerYear / targetBlockTimeSec)
}

type MonetaryPolicy struct {
	InitialBlockReward     uint64
	AnnualReductionPercent uint8
	MinimumBlockReward     uint64
}

func (p MonetaryPolicy) BlockReward(height uint64) uint64 {
	reduction := uint64(p.AnnualReductionPercent) * (height / getBlocksPerYear(15))
	reward := p.InitialBlockReward - (p.InitialBlockReward * reduction / 100)
	if reward < p.MinimumBlockReward {
		return p.MinimumBlockReward
	}
	return reward
}

func (p MonetaryPolicy) MinerFeeAmount(totalFees uint64) uint64 {
	return totalFees
}

type MempoolEntry interface {
	GetTx() core.Transaction
	GetTxID() string
	GetReceived() time.Time
}

type Mempool interface {
	Size() int
	Snapshot() []MempoolEntry
	GetTxBytes(tx core.Transaction) int
}

type Blockchain interface {
	LatestBlock() *core.Block
	CanonicalTxCount() uint64
	BlockByHeight(height uint64) (*core.Block, bool)
}

type PeerManager interface {
	Peers() []string
	Count() int
	MaxPeers() int
	GetPeerScore(peerID string) float64
	GetPeerLatency(peerID string) time.Duration
}

type SyncLoop interface {
	GetPeerManager() PeerManager
	IsSyncing() bool
	GetOrphanPoolSize() int
	IsMining() bool
	GetActiveWorkerCount() int
}

type Metrics struct {
	bc    Blockchain
	mp    Mempool
	peers PeerManager
	sync  SyncLoop

	mu sync.RWMutex

	httpRequestsTotal map[string]map[int]int64
	httpDurations     map[string]*histogramData

	nodeID               string
	chainID              uint64
	inflation            *InflationCalculator
	registry             *prometheus.Registry
	startTime            time.Time
	lastBlockTime        time.Time
	blockCount           uint64
	totalBlockVerifyTime time.Duration
	totalTxVerifyTime    time.Duration
	txCount              uint64

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

	nogoIsSyncing                prometheus.Gauge
	nogoOrphanPoolSize           prometheus.Gauge
	nogoMiningPaused             prometheus.Gauge
	nogoChainSwitchesTotal       prometheus.Counter
	nogoInstantValidationEnabled prometheus.Gauge
	nogoBlockEventsTotal         prometheus.Counter
	nogoHeaderEventsTotal        prometheus.Counter
	nogoSyncWorkersActive        prometheus.Gauge

	nogoBlockProductionRate    prometheus.Gauge
	nogoMiningHashesTotal      prometheus.Counter
	nogoMiningDifficulty       prometheus.Gauge
	nogoBlocksMinedTotal       prometheus.Counter
	nogoMiningEfficiency       prometheus.Gauge
	nogoBlockIntervalSeconds   prometheus.Histogram
	nogoTps                    prometheus.Gauge
	nogoUptimeSeconds          prometheus.Gauge
	nogoGoRoutines             prometheus.Gauge
	nogoMemStatsAllocBytes     prometheus.Gauge
	nogoMemStatsSysBytes       prometheus.Gauge
	nogoNtpOffsetSeconds       prometheus.Gauge
	nogoNtpSynchronized        prometheus.Gauge
	nogoBatchVerificationTotal prometheus.Counter
	nogoBatchStoreTotal        prometheus.Counter
	nogoOrphanParentRequests   prometheus.Counter
	nogoOrphanParentFound      prometheus.Counter
}

type histogramData struct {
	buckets []float64
	counts  []int64
	sum     float64
	count   int64
}

func newHistogramData() *histogramData {
	buckets := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	return &histogramData{
		buckets: buckets,
		counts:  make([]int64, len(buckets)),
	}
}

func (h *histogramData) observe(seconds float64) {
	h.sum += seconds
	h.count++
	for i, le := range h.buckets {
		if seconds <= le {
			h.counts[i]++
			break
		}
	}
}

func NewMetrics(bc Blockchain, mp Mempool, peers PeerManager, sl SyncLoop, nodeID string, chainID uint64) (*Metrics, error) {
	m := &Metrics{
		bc:                bc,
		mp:                mp,
		peers:             peers,
		sync:              sl,
		nodeID:            nodeID,
		chainID:           chainID,
		httpRequestsTotal: make(map[string]map[int]int64),
		httpDurations:     make(map[string]*histogramData),
		inflation:         NewInflationCalculator(),
		registry:          prometheus.NewRegistry(),
		startTime:         time.Now(),
		lastBlockTime:     time.Now(),
	}

	commonLabels := prometheus.Labels{
		"node_id":  nodeID,
		"chain_id": strconv.FormatUint(chainID, 10),
	}

	m.nogoMempoolSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_mempool_size",
		Help:        "Current number of transactions in mempool",
		ConstLabels: commonLabels,
	})

	m.nogoMempoolBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_mempool_bytes",
		Help:        "Total size of mempool in bytes",
		ConstLabels: commonLabels,
	})

	m.nogoSyncProgressPercent = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_sync_progress_percent",
		Help:        "Sync progress percentage (0-100)",
		ConstLabels: commonLabels,
	})

	m.nogoBlockVerificationDur = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        "nogo_block_verification_duration_seconds",
		Help:        "Block verification latency in seconds",
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		ConstLabels: commonLabels,
	})

	m.nogoTxVerificationDur = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        "nogo_transaction_verification_duration_seconds",
		Help:        "Transaction verification latency in seconds",
		Buckets:     []float64{0.0001, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5},
		ConstLabels: commonLabels,
	})

	m.nogoPeerScoreDistribution = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        "nogo_peer_score_distribution",
		Help:        "Distribution of peer scores",
		Buckets:     []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		ConstLabels: commonLabels,
	})

	m.nogoInflationRate = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_inflation_rate",
		Help:        "Current inflation rate percentage",
		ConstLabels: commonLabels,
	})

	m.nogoChainHeight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_chain_height",
		Help:        "Current canonical height",
		ConstLabels: commonLabels,
	})

	m.nogoDifficultyBits = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_difficulty_bits",
		Help:        "Current difficulty bits",
		ConstLabels: commonLabels,
	})

	m.nogoPeersCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_peers_count",
		Help:        "Number of connected peers",
		ConstLabels: commonLabels,
	})

	m.nogoBlocksCanonical = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_blocks_canonical",
		Help:        "Canonical blocks count",
		ConstLabels: commonLabels,
	})

	m.nogoTxsCanonical = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_txs_canonical",
		Help:        "Canonical transactions count",
		ConstLabels: commonLabels,
	})

	m.nogoPeerSuccessRate = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_peer_success_rate",
		Help:        "Average peer success rate",
		ConstLabels: commonLabels,
	})

	m.nogoPeerLatencyAvg = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_peer_latency_avg",
		Help:        "Average peer latency in milliseconds",
		ConstLabels: commonLabels,
	})

	m.nogoBlockReward = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_block_reward",
		Help:        "Current block reward in NOGO",
		ConstLabels: commonLabels,
	})

	m.nogoAnnualReductionRate = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_annual_reduction_rate",
		Help:        "Annual block reward reduction rate percentage",
		ConstLabels: commonLabels,
	})

	m.nogoIsSyncing = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_is_syncing",
		Help:        "Whether the node is currently syncing (1 = syncing, 0 = not syncing)",
		ConstLabels: commonLabels,
	})

	m.nogoOrphanPoolSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_orphan_pool_size",
		Help:        "Current number of orphan blocks in the pool",
		ConstLabels: commonLabels,
	})

	m.nogoMiningPaused = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_mining_paused",
		Help:        "Whether mining is paused due to sync (1 = paused, 0 = active)",
		ConstLabels: commonLabels,
	})

	m.nogoChainSwitchesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "nogo_chain_switches_total",
		Help:        "Total number of chain reorganizations",
		ConstLabels: commonLabels,
	})

	m.nogoInstantValidationEnabled = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_instant_validation_enabled",
		Help:        "Whether instant validation is enabled (1 = enabled, 0 = disabled)",
		ConstLabels: commonLabels,
	})

	m.nogoBlockEventsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "nogo_block_events_total",
		Help:        "Total number of block events processed",
		ConstLabels: commonLabels,
	})

	m.nogoHeaderEventsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "nogo_header_events_total",
		Help:        "Total number of header events processed",
		ConstLabels: commonLabels,
	})

	m.nogoSyncWorkersActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_sync_workers_active",
		Help:        "Number of active sync workers",
		ConstLabels: commonLabels,
	})

	m.nogoBlockProductionRate = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_block_production_rate",
		Help:        "Blocks produced per minute",
		ConstLabels: commonLabels,
	})

	m.nogoMiningHashesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "nogo_mining_hashes_total",
		Help:        "Total mining hashes computed",
		ConstLabels: commonLabels,
	})

	m.nogoMiningDifficulty = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_mining_difficulty",
		Help:        "Current mining difficulty",
		ConstLabels: commonLabels,
	})

	m.nogoBlocksMinedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "nogo_blocks_mined_total",
		Help:        "Total blocks mined by this node",
		ConstLabels: commonLabels,
	})

	m.nogoMiningEfficiency = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_mining_efficiency",
		Help:        "Mining efficiency percentage",
		ConstLabels: commonLabels,
	})

	m.nogoBlockIntervalSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        "nogo_block_interval_seconds",
		Help:        "Time between consecutive blocks in seconds",
		Buckets:     []float64{5, 10, 15, 20, 30, 45, 60, 90, 120, 180, 300},
		ConstLabels: commonLabels,
	})

	m.nogoTps = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_tps",
		Help:        "Transactions per second",
		ConstLabels: commonLabels,
	})

	m.nogoUptimeSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_uptime_seconds",
		Help:        "Node uptime in seconds",
		ConstLabels: commonLabels,
	})

	m.nogoGoRoutines = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_go_routines",
		Help:        "Number of active goroutines",
		ConstLabels: commonLabels,
	})

	m.nogoMemStatsAllocBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_memstats_alloc_bytes",
		Help:        "Bytes allocated and still in use",
		ConstLabels: commonLabels,
	})

	m.nogoMemStatsSysBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_memstats_sys_bytes",
		Help:        "Total bytes of memory obtained from the OS",
		ConstLabels: commonLabels,
	})

	m.nogoNtpOffsetSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_ntp_offset_seconds",
		Help:        "NTP time offset in seconds",
		ConstLabels: commonLabels,
	})

	m.nogoNtpSynchronized = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "nogo_ntp_synchronized",
		Help:        "Whether NTP is synchronized (1 = synchronized, 0 = not)",
		ConstLabels: commonLabels,
	})

	m.nogoBatchVerificationTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "nogo_batch_verification_total",
		Help:        "Total number of batch verifications performed",
		ConstLabels: commonLabels,
	})

	m.nogoBatchStoreTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "nogo_batch_store_total",
		Help:        "Total number of batch store operations performed",
		ConstLabels: commonLabels,
	})

	m.nogoOrphanParentRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "nogo_orphan_parent_requests_total",
		Help:        "Total number of parent block requests for orphans",
		ConstLabels: commonLabels,
	})

	m.nogoOrphanParentFound = prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "nogo_orphan_parent_found_total",
		Help:        "Total number of parent blocks found for orphans",
		ConstLabels: commonLabels,
	})

	mustRegister := func(c prometheus.Collector) {
		if err := m.registry.Register(c); err != nil {
			log.Printf("metrics: failed to register metric: %v", err)
		}
	}

	mustRegister(m.nogoMempoolSize)
	mustRegister(m.nogoMempoolBytes)
	mustRegister(m.nogoSyncProgressPercent)
	mustRegister(m.nogoBlockVerificationDur)
	mustRegister(m.nogoTxVerificationDur)
	mustRegister(m.nogoPeerScoreDistribution)
	mustRegister(m.nogoInflationRate)
	mustRegister(m.nogoChainHeight)
	mustRegister(m.nogoDifficultyBits)
	mustRegister(m.nogoPeersCount)
	mustRegister(m.nogoBlocksCanonical)
	mustRegister(m.nogoTxsCanonical)
	mustRegister(m.nogoPeerSuccessRate)
	mustRegister(m.nogoPeerLatencyAvg)
	mustRegister(m.nogoBlockReward)
	mustRegister(m.nogoAnnualReductionRate)
	mustRegister(m.nogoIsSyncing)
	mustRegister(m.nogoOrphanPoolSize)
	mustRegister(m.nogoMiningPaused)
	mustRegister(m.nogoChainSwitchesTotal)
	mustRegister(m.nogoInstantValidationEnabled)
	mustRegister(m.nogoBlockEventsTotal)
	mustRegister(m.nogoHeaderEventsTotal)
	mustRegister(m.nogoSyncWorkersActive)
	mustRegister(m.nogoBlockProductionRate)
	mustRegister(m.nogoMiningHashesTotal)
	mustRegister(m.nogoMiningDifficulty)
	mustRegister(m.nogoBlocksMinedTotal)
	mustRegister(m.nogoMiningEfficiency)
	mustRegister(m.nogoBlockIntervalSeconds)
	mustRegister(m.nogoTps)
	mustRegister(m.nogoUptimeSeconds)
	mustRegister(m.nogoGoRoutines)
	mustRegister(m.nogoMemStatsAllocBytes)
	mustRegister(m.nogoMemStatsSysBytes)
	mustRegister(m.nogoNtpOffsetSeconds)
	mustRegister(m.nogoNtpSynchronized)
	mustRegister(m.nogoBatchVerificationTotal)
	mustRegister(m.nogoBatchStoreTotal)
	mustRegister(m.nogoOrphanParentRequests)
	mustRegister(m.nogoOrphanParentFound)

	m.nogoInstantValidationEnabled.Set(1)
	m.nogoIsSyncing.Set(0)
	m.nogoMiningPaused.Set(0)
	m.nogoOrphanPoolSize.Set(0)
	m.nogoSyncWorkersActive.Set(0)
	m.nogoAnnualReductionRate.Set(10.0)
	m.nogoNtpSynchronized.Set(0)

	return m, nil
}

func (m *Metrics) GetRegistry() *prometheus.Registry {
	return m.registry
}

type InflationCalculator struct {
	mu sync.RWMutex
}

func NewInflationCalculator() *InflationCalculator {
	return &InflationCalculator{}
}

func (ic *InflationCalculator) CalculateInflationRate(currentHeight uint64, currentSupply uint64) float64 {
	ic.mu.RLock()
	defer ic.mu.RUnlock()

	if currentSupply == 0 {
		return 0
	}

	blocksPerYear := getBlocksPerYear(15)
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

func (m *Metrics) ObserveHTTP(route string, status int, dur time.Duration) {
	if m == nil || route == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.httpRequestsTotal[route] == nil {
		m.httpRequestsTotal[route] = make(map[int]int64)
	}
	m.httpRequestsTotal[route][status]++

	h, exists := m.httpDurations[route]
	if !exists {
		h = newHistogramData()
		m.httpDurations[route] = h
	}
	h.observe(dur.Seconds())
}

func (m *Metrics) ObserveBlockVerification(duration time.Duration) {
	if m == nil {
		return
	}
	m.nogoBlockVerificationDur.Observe(duration.Seconds())

	m.mu.Lock()
	m.totalBlockVerifyTime += duration
	m.blockCount++
	m.mu.Unlock()
}

func (m *Metrics) ObserveTransactionVerification(duration time.Duration) {
	if m == nil {
		return
	}
	m.nogoTxVerificationDur.Observe(duration.Seconds())

	m.mu.Lock()
	m.totalTxVerifyTime += duration
	m.txCount++
	m.mu.Unlock()
}

func (m *Metrics) ObserveBatchVerification(total, valid, invalid int) {
	if m == nil {
		return
	}
	m.nogoBatchVerificationTotal.Inc()
}

func (m *Metrics) ObserveBatchStore(total, stored, failed int, duration time.Duration) {
	if m == nil {
		return
	}
	m.nogoBatchStoreTotal.Inc()
}

func (m *Metrics) RecordBlockMined() {
	if m == nil {
		return
	}
	m.nogoBlocksMinedTotal.Inc()

	m.mu.Lock()
	now := time.Now()
	interval := now.Sub(m.lastBlockTime)
	m.lastBlockTime = now
	m.mu.Unlock()

	m.nogoBlockIntervalSeconds.Observe(interval.Seconds())
}

func (m *Metrics) AddMiningHashes(hashes uint64) {
	if m == nil {
		return
	}
	m.nogoMiningHashesTotal.Add(float64(hashes))
}

func (m *Metrics) SetMiningDifficulty(difficulty float64) {
	if m == nil {
		return
	}
	m.nogoMiningDifficulty.Set(difficulty)
}

func (m *Metrics) SetMiningEfficiency(efficiency float64) {
	if m == nil {
		return
	}
	if efficiency < 0 {
		efficiency = 0
	}
	if efficiency > 100 {
		efficiency = 100
	}
	m.nogoMiningEfficiency.Set(efficiency)
}

func (m *Metrics) UpdateMempoolMetrics() {
	if m == nil || m.mp == nil {
		return
	}

	size := m.mp.Size()
	m.nogoMempoolSize.Set(float64(size))

	entries := m.mp.Snapshot()
	totalBytes := 0
	for _, entry := range entries {
		tx := entry.GetTx()
		bytes := m.mp.GetTxBytes(tx)
		totalBytes += bytes
	}
	m.nogoMempoolBytes.Set(float64(totalBytes))
}

func (m *Metrics) UpdateSyncProgress() {
	if m == nil || m.sync == nil || m.bc == nil {
		return
	}

	latestBlock := m.bc.LatestBlock()
	var localHeight uint64
	if latestBlock != nil {
		localHeight = latestBlock.GetHeight()
	}

	var networkHeight uint64
	pm := m.sync.GetPeerManager()
	if pm != nil {
		peers := pm.Peers()
		for _, peer := range peers {
			info := m.fetchPeerChainInfo(peer, pm)
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

type PeerChainInfo struct {
	Height uint64
}

func (m *Metrics) fetchPeerChainInfo(peerID string, pm PeerManager) PeerChainInfo {
	_ = pm.GetPeerScore(peerID)
	_ = pm.GetPeerLatency(peerID)
	return PeerChainInfo{Height: 0}
}

func (m *Metrics) UpdatePeerMetrics() {
	if m == nil || m.peers == nil {
		return
	}

	peers := m.peers.Peers()
	peersCount := len(peers)
	m.nogoPeersCount.Set(float64(peersCount))

	if peersCount > 0 {
		var totalScore, totalLatency float64
		validPeers := 0

		for _, peer := range peers {
			score := m.peers.GetPeerScore(peer)
			latency := m.peers.GetPeerLatency(peer).Seconds() * 1000

			totalScore += score
			totalLatency += latency
			validPeers++

			m.nogoPeerScoreDistribution.Observe(score)
		}

		if validPeers > 0 {
			m.nogoPeerSuccessRate.Set(totalScore / float64(validPeers))
			m.nogoPeerLatencyAvg.Set(totalLatency / float64(validPeers))
		}
	}
}

func (m *Metrics) UpdateChainMetrics() {
	if m == nil || m.bc == nil {
		return
	}

	latestBlock := m.bc.LatestBlock()
	if latestBlock == nil {
		return
	}

	height := latestBlock.GetHeight()
	difficulty := latestBlock.GetDifficultyBits()
	txsCount := m.bc.CanonicalTxCount()

	m.nogoChainHeight.Set(float64(height))
	m.nogoDifficultyBits.Set(float64(difficulty))
	m.nogoBlocksCanonical.Set(float64(height + 1))
	m.nogoTxsCanonical.Set(float64(txsCount))

	policy := MonetaryPolicy{
		InitialBlockReward:     initialBlockRewardWei.Uint64(),
		AnnualReductionPercent: 10,
		MinimumBlockReward:     minimumBlockRewardWei.Uint64(),
	}
	currentReward := policy.BlockReward(height)
	rewardInNogo := float64(currentReward) / float64(NogoNOGO)
	m.nogoBlockReward.Set(rewardInNogo)

	totalSupply := m.calculateTotalSupply(height)
	if totalSupply > 0 {
		inflationRate := m.inflation.CalculateInflationRate(height, totalSupply)
		m.nogoInflationRate.Set(inflationRate)
	}
}

func (m *Metrics) calculateTotalSupply(height uint64) uint64 {
	if height == 0 {
		genesisBlock, exists := m.bc.BlockByHeight(0)
		if exists && len(genesisBlock.GetTransactions()) > 0 {
			return 0
		}
		return 0
	}

	policy := MonetaryPolicy{
		InitialBlockReward:     initialBlockRewardWei.Uint64(),
		AnnualReductionPercent: 10,
		MinimumBlockReward:     minimumBlockRewardWei.Uint64(),
	}
	totalSupply := uint64(0)

	for h := uint64(0); h <= height; h++ {
		blockReward := policy.BlockReward(h)
		totalSupply += blockReward

		block, exists := m.bc.BlockByHeight(h)
		if exists {
			var totalFees uint64
			for _, tx := range block.GetTransactions() {
				_ = tx
				totalFees += 0
			}
			totalSupply += policy.MinerFeeAmount(totalFees)
		}
	}

	return totalSupply
}

func (m *Metrics) UpdateAllMetrics() {
	if m == nil {
		return
	}
	m.UpdateChainMetrics()
	m.UpdateMempoolMetrics()
	m.UpdateSyncProgress()
	m.UpdatePeerMetrics()
	m.UpdateSyncMetrics()
	m.UpdateRuntimeMetrics()
	m.UpdateMiningMetrics()
}

func (m *Metrics) UpdateSyncMetrics() {
	if m == nil || m.sync == nil {
		return
	}

	if m.sync.IsSyncing() {
		m.nogoIsSyncing.Set(1)
	} else {
		m.nogoIsSyncing.Set(0)
	}

	orphanSize := m.sync.GetOrphanPoolSize()
	m.nogoOrphanPoolSize.Set(float64(orphanSize))

	if m.sync.IsMining() {
		m.nogoMiningPaused.Set(0)
	} else {
		m.nogoMiningPaused.Set(1)
	}

	workerCount := m.sync.GetActiveWorkerCount()
	m.nogoSyncWorkersActive.Set(float64(workerCount))
}

func (m *Metrics) UpdateRuntimeMetrics() {
	uptime := time.Since(m.startTime).Seconds()
	m.nogoUptimeSeconds.Set(uptime)

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m.nogoGoRoutines.Set(float64(runtime.NumGoroutine()))
	m.nogoMemStatsAllocBytes.Set(float64(ms.Alloc))
	m.nogoMemStatsSysBytes.Set(float64(ms.Sys))

	m.mu.RLock()
	totalTxCount := m.txCount
	m.mu.RUnlock()
	tps := float64(totalTxCount) / uptime
	if uptime < 1 {
		tps = 0
	}
	m.nogoTps.Set(tps)

	if ntpStatus := getNTPStatus(); ntpStatus != nil {
		m.nogoNtpOffsetSeconds.Set(ntpStatus.Offset.Seconds())
		if ntpStatus.Synchronized {
			m.nogoNtpSynchronized.Set(1)
		} else {
			m.nogoNtpSynchronized.Set(0)
		}
	}
}

func (m *Metrics) UpdateMiningMetrics() {
	m.mu.RLock()
	blockCount := m.blockCount
	totalBlockVerifyTime := m.totalBlockVerifyTime
	m.mu.RUnlock()

	if blockCount > 0 && totalBlockVerifyTime > 0 {
		avgBlockTime := totalBlockVerifyTime.Seconds() / float64(blockCount)
		blocksPerMinute := 60.0 / avgBlockTime
		m.nogoBlockProductionRate.Set(blocksPerMinute)
	}

	m.mu.RLock()
	uptime := time.Since(m.startTime).Seconds()
	totalTxCount := m.txCount
	m.mu.RUnlock()

	if uptime > 0 {
		tps := float64(totalTxCount) / uptime
		m.nogoTps.Set(tps)
	}
}

func (m *Metrics) RecordChainSwitch() {
	if m == nil {
		return
	}
	m.nogoChainSwitchesTotal.Inc()
}

func (m *Metrics) RecordBlockEvent() {
	if m == nil {
		return
	}
	m.nogoBlockEventsTotal.Inc()
}

func (m *Metrics) RecordHeaderEvent() {
	if m == nil {
		return
	}
	m.nogoHeaderEventsTotal.Inc()
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.UpdateAllMetrics()

	handler := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
	handler.ServeHTTP(w, r)
}

func (m *Metrics) ServeHTTPCustom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.UpdateAllMetrics()

	m.mu.RLock()
	defer m.mu.RUnlock()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	var buf bytes.Buffer

	buf.WriteString("# HELP go_goroutines Number of goroutines.\n")
	buf.WriteString("# TYPE go_goroutines gauge\n")
	buf.WriteString(fmt.Sprintf("go_goroutines %d\n", runtime.NumGoroutine()))

	buf.WriteString("# HELP go_memstats_alloc_bytes Bytes allocated and still in use.\n")
	buf.WriteString("# TYPE go_memstats_alloc_bytes gauge\n")
	buf.WriteString(fmt.Sprintf("go_memstats_alloc_bytes %d\n", ms.Alloc))

	buf.WriteString("# HELP http_requests_total Total HTTP requests by route and status code.\n")
	buf.WriteString("# TYPE http_requests_total counter\n")

	routes := make([]string, 0, len(m.httpRequestsTotal))
	for route := range m.httpRequestsTotal {
		routes = append(routes, route)
	}
	sort.Strings(routes)

	for _, route := range routes {
		byStatus := m.httpRequestsTotal[route]
		codes := make([]int, 0, len(byStatus))
		for code := range byStatus {
			codes = append(codes, code)
		}
		sort.Ints(codes)

		for _, code := range codes {
			count := byStatus[code]
			buf.WriteString(fmt.Sprintf("http_requests_total{route=%q,code=%q} %d\n", route, strconv.Itoa(code), count))
		}
	}

	buf.WriteString("# HELP http_request_duration_seconds HTTP request latency by route.\n")
	buf.WriteString("# TYPE http_request_duration_seconds histogram\n")

	for _, route := range routes {
		h := m.httpDurations[route]
		if h == nil {
			continue
		}

		var cumulative int64
		for i, le := range h.buckets {
			cumulative += h.counts[i]
			buf.WriteString(fmt.Sprintf("http_request_duration_seconds_bucket{route=%q,le=%q} %d\n", route, fmt.Sprintf("%.3f", le), cumulative))
		}
		buf.WriteString(fmt.Sprintf("http_request_duration_seconds_bucket{route=%q,le=\"+Inf\"} %d\n", route, h.count))
		buf.WriteString(fmt.Sprintf("http_request_duration_seconds_sum{route=%q} %f\n", route, h.sum))
		buf.WriteString(fmt.Sprintf("http_request_duration_seconds_count{route=%q} %d\n", route, h.count))
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, err := w.Write(buf.Bytes())
	if err != nil {
		return
	}
}

func getNTPStatus() *utils.NTPStatus {
	return utils.GetNTPStatus()
}

func (m *Metrics) GetMetricsJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	uptime := time.Since(m.startTime).Seconds()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	ntpStatus := getNTPStatus()

	metrics := map[string]any{
		"uptime_seconds":          uptime,
		"mempool_size":            m.mp.Size(),
		"mempool_bytes":           uint64(0),
		"peers_connected":         len(m.peers.Peers()),
		"sync_progress_percent":   100.0,
		"is_syncing":              false,
		"mining_paused":           false,
		"chain_height":            uint64(0),
		"difficulty_bits":         uint32(0),
		"inflation_rate":          0.0,
		"block_reward":            0.0,
		"tps":                     float64(m.txCount) / uptime,
		"go_routines":             runtime.NumGoroutine(),
		"alloc_bytes":             ms.Alloc,
		"sys_bytes":               ms.Sys,
		"ntp_synchronized":        false,
		"ntp_offset_seconds":      0.0,
		"blocks_mined_total":      uint64(0),
		"mining_hashes_total":     uint64(0),
		"mining_difficulty":       0.0,
		"mining_efficiency":       0.0,
		"block_production_rate":   0.0,
		"block_interval_seconds":  0.0,
		"orphan_pool_size":        0,
		"sync_workers_active":     0,
		"chain_switches_total":    uint64(0),
		"block_events_total":      uint64(0),
		"header_events_total":     uint64(0),
		"peer_success_rate":       0.0,
		"peer_latency_avg":        0.0,
		"peer_score_distribution": 0.0,
		"timestamp":               time.Now().Unix(),
	}

	if ntpStatus != nil {
		metrics["ntp_synchronized"] = ntpStatus.Synchronized
		metrics["ntp_offset_seconds"] = ntpStatus.Offset.Seconds()
	}

	data, err := json.Marshal(metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metrics: %w", err)
	}

	return data, nil
}
