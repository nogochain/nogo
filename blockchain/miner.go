package main

import (
	"context"
	"log"
	"sync"
	"time"
)

type Miner struct {
	bc *Blockchain
	mp *Mempool

	maxTxPerBlock    int
	forceEmptyBlocks bool

	events EventSink

	mu      sync.Mutex
	wakeCh  chan struct{}
	stopped chan struct{}
}

func NewMiner(bc *Blockchain, mp *Mempool, maxTxPerBlock int, forceEmptyBlocks bool) *Miner {
	if maxTxPerBlock <= 0 {
		maxTxPerBlock = 100
	}
	return &Miner{
		bc:               bc,
		mp:               mp,
		maxTxPerBlock:    maxTxPerBlock,
		forceEmptyBlocks: forceEmptyBlocks,
		wakeCh:           make(chan struct{}, 1),
		stopped:          make(chan struct{}),
	}
}

func (m *Miner) SetEventSink(sink EventSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = sink
}

func (m *Miner) Wake() {
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}

func (m *Miner) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	defer close(m.stopped)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = m.MineOnce(ctx, false)
		case <-m.wakeCh:
			_, _ = m.MineOnce(ctx, false)
		}
	}
}

func (m *Miner) MineOnce(ctx context.Context, force bool) (*Block, error) {
	if m.mp == nil {
		return nil, nil
	}

	selected, selectedIDs, err := m.bc.SelectMempoolTxs(m.mp, m.maxTxPerBlock)
	if err != nil {
		log.Printf("miner: select txs failed: %v", err)
		return nil, err
	}

	mineEmpty := force || m.forceEmptyBlocks
	log.Printf("miner: force=%v, forceEmptyBlocks=%v, mineEmpty=%v, selected=%d", force, m.forceEmptyBlocks, mineEmpty, len(selected))
	if len(selected) == 0 && !mineEmpty {
		return nil, nil
	}

	log.Printf("miner: attempting to mine block with %d transactions", len(selected))
	b, err := m.bc.MineTransfers(selected)
	if err != nil {
		log.Printf("miner: mine failed: %v", err)
		return nil, err
	}
	log.Printf("miner: successfully mined block at height %d, diff=%d", b.Height, b.DifficultyBits)
	if len(selectedIDs) > 0 {
		m.mp.RemoveMany(selectedIDs)
		m.mu.Lock()
		sink := m.events
		m.mu.Unlock()
		if sink != nil {
			addrs := addressesForBlock(&Block{Transactions: selected})
			sink.Publish(WSEvent{
				Type: "mempool_removed",
				Data: map[string]any{
					"txIds":     selectedIDs,
					"reason":    "mined",
					"addresses": addrs,
				},
			})
		}
	}
	return b, nil
}
