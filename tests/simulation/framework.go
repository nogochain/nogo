// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// Simulation framework for multi-node testing.
// Provides in-memory multi-node simulation without network dependencies.
//
// In-memory multi-node simulation without network dependencies.
// Each SimNode has a real Chain, Mempool, and can mine blocks on demand.
// Nodes communicate through SimNetwork's in-memory message bus.
package simulation

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/mempool"
)

// =============================================================================
// SimCluster: manages a cluster of simulated nodes
// =============================================================================

type SimCluster struct {
	mu      sync.RWMutex
	Nodes   []*SimNode
	Network *SimNetwork
	Config  SimConfig
	ctx     context.Context
	cancel  context.CancelFunc
	started bool
}

type SimConfig struct {
	NodeCount     int
	BlockTime     time.Duration
	MaxTxPerBlock int
	ChainID       uint64
	LogOutput     bool
}

func DefaultSimConfig() SimConfig {
	return SimConfig{
		NodeCount:     3,
		BlockTime:     100 * time.Millisecond,
		MaxTxPerBlock: 100,
		ChainID:       0,
		LogOutput:     false,
	}
}

func NewSimCluster(cfg SimConfig) *SimCluster {
	if cfg.NodeCount <= 0 {
		cfg.NodeCount = 3
	}
	ctx, cancel := context.WithCancel(context.Background())
	network := &SimNetwork{
		BlockCh: make(chan *BlockMessage, 1000),
		TxCh:    make(chan *TxMessage, 1000),
		nodes:   make(map[int]*SimNode),
	}
	return &SimCluster{
		Nodes:   make([]*SimNode, cfg.NodeCount),
		Network: network,
		Config:  cfg,
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (c *SimCluster) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return nil
	}
	if !c.Config.LogOutput {
		log.SetOutput(nilLogWriter{})
	}
	for i := 0; i < c.Config.NodeCount; i++ {
		node, err := c.createNode(i)
		if err != nil {
			return err
		}
		c.Nodes[i] = node
		c.Network.nodes[i] = node
	}
	go c.Network.forwardBlocks(c.ctx)
	c.started = true
	return nil
}

func (c *SimCluster) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancel()
	for _, n := range c.Nodes {
		if n != nil {
			n.Stop()
		}
	}
	c.started = false
}

func (c *SimCluster) createNode(id int) (*SimNode, error) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	minerAddr := core.GenerateAddress(pub)
	store := newSTestStore()

	// MUST set fake mode BEFORE NewChain, otherwise init() defaults to production
	core.SetPowModeForTesting("fake")

	// Create unique temp dir for each node's index to avoid bbolt file lock conflicts
	indexPath := filepath.Join(os.TempDir(), fmt.Sprintf("nogosim_node%d_%d", id, time.Now().UnixNano()))

	chain, err := core.NewChain(core.ChainConfig{
		ChainID:      c.Config.ChainID,
		MinerAddress: minerAddr,
		Store:        store,
		GenesisPath:  "",
		IndexPath:    indexPath,
	})
	if err != nil {
		return nil, err
	}

	consensus := chain.GetConsensus()
	mp := mempool.NewMempool(1000, 1, 24*time.Hour, nil, consensus.ChainID, consensus, 0, config.MempoolConfig{
		MaxTransactions: 1000, MaxMemoryMB: 10, MinFeeRate: 1, TTL: 24 * time.Hour,
	})

	nodeCtx, _ := context.WithCancel(c.ctx)
	node := &SimNode{
		ID:         id,
		Chain:      chain,
		Mempool:    mp,
		Network:    c.Network,
		Config:     c.Config,
		Store:      store,
		MinerAddr:  minerAddr,
		KeyPubKey:  pub,
		indexPath:  indexPath,
		ctx:        nodeCtx,
	}

	return node, nil
}

func (c *SimCluster) WaitForHeight(targetHeight uint64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		allMatch := true
		for _, n := range c.Nodes {
			if n != nil && n.Height() < targetHeight {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
		time.Sleep(c.Config.BlockTime)
	}
	return false
}

func (c *SimCluster) AssertAllChainsEqual() bool {
	if len(c.Nodes) < 2 {
		return true
	}
	ref := c.Nodes[0]
	if ref == nil {
		return false
	}
	refHash := ref.Chain.LatestBlock().Hash
	refHeight := ref.Height()
	for i := 1; i < len(c.Nodes); i++ {
		n := c.Nodes[i]
		if n == nil {
			continue
		}
		latest := n.Chain.LatestBlock()
		if latest == nil {
			return false
		}
		if n.Height() != refHeight {
			return false
		}
		if hex.EncodeToString(latest.Hash) != hex.EncodeToString(refHash) {
			return false
		}
	}
	return true
}

// =============================================================================
// SimNode: a single simulated node
// =============================================================================

type SimNode struct {
	ID        int
	Chain     *core.Chain
	Mempool   *mempool.Mempool
	Network   *SimNetwork
	Config    SimConfig
	Store     *sTestStore
	MinerAddr string
	KeyPubKey []byte
	indexPath string // temp dir for address index (cleanup on stop)
	mu        sync.RWMutex
	ctx       context.Context
}

func (n *SimNode) Height() uint64 {
	latest := n.Chain.LatestBlock()
	if latest == nil {
		return 0
	}
	return latest.GetHeight()
}

func (n *SimNode) Stop() {
	n.Chain.Stop()
	n.Mempool.Close()
	if n.indexPath != "" {
		os.RemoveAll(n.indexPath)
	}
}

// MineBlock mines a block using POW_MODE=fake (fast)
func (n *SimNode) MineBlock(ctx context.Context) (*core.Block, error) {
	return n.Chain.MineTransfers(ctx, nil)
}

func (n *SimNode) AddBlock(block *core.Block) (bool, error) {
	return n.Chain.AddBlock(block)
}

func (n *SimNode) CanonicalWork() *big.Int {
	return n.Chain.CanonicalWork()
}

// CreateTransaction creates and signs a test transaction
func (n *SimNode) CreateTransaction(to string, amount, fee, nonce uint64) (core.Transaction, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return core.Transaction{}, err
	}
	tx := core.Transaction{
		Type:      core.TxTransfer,
		ChainID:   n.Chain.GetConsensus().ChainID,
		ToAddress: to,
		Amount:    amount,
		Fee:       fee,
		Nonce:     nonce,
	}
	tx.FromPubKey = n.KeyPubKey
	h, err := tx.SigningHash()
	if err != nil {
		return core.Transaction{}, err
	}
	tx.Signature = ed25519.Sign(priv, h)
	return tx, nil
}

// =============================================================================
// SimNetwork: in-memory network bus
// =============================================================================

type SimNetwork struct {
	mu       sync.RWMutex
	BlockCh  chan *BlockMessage
	TxCh     chan *TxMessage
	Latency  time.Duration
	DropRate float64
	nodes    map[int]*SimNode
}

type BlockMessage struct {
	Block     *core.Block
	SourceID  int
	Timestamp time.Time
}

type TxMessage struct {
	Tx        core.Transaction
	SourceID  int
	Timestamp time.Time
}

func (sn *SimNetwork) forwardBlocks(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-sn.BlockCh:
			sn.mu.RLock()
			for id, node := range sn.nodes {
				if id != msg.SourceID && node != nil {
					go func(n *SimNode, b *core.Block) {
						n.AddBlock(b)
					}(node, msg.Block)
				}
			}
			sn.mu.RUnlock()
		}
	}
}

// =============================================================================
// Test chain store (in-memory)
// =============================================================================

type sTestStore struct {
	mu        sync.RWMutex
	blocks    map[string]*core.Block
	canonical []*core.Block
	hashes    map[string][]byte
	accounts  map[string]core.Account
}

func newSTestStore() *sTestStore {
	return &sTestStore{
		blocks:   make(map[string]*core.Block),
		hashes:   make(map[string][]byte),
		accounts: make(map[string]core.Account),
	}
}

func (s *sTestStore) SaveBlock(block *core.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocks[hex.EncodeToString(block.Hash)] = block
	return nil
}
func (s *sTestStore) LoadBlock(hash []byte) (*core.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, _ := s.blocks[hex.EncodeToString(hash)]
	return b, nil
}
func (s *sTestStore) LoadCanonicalChain() ([]*core.Block, error)  { return s.ReadCanonical() }
func (s *sTestStore) SaveCanonicalChain(blocks []*core.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.canonical = append([]*core.Block{}, blocks...)
	return nil
}
func (s *sTestStore) ReadCanonical() ([]*core.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]*core.Block{}, s.canonical...), nil
}
func (s *sTestStore) ReadAllBlocks() (map[string]*core.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r := make(map[string]*core.Block, len(s.blocks))
	for k, v := range s.blocks { r[k] = v }
	return r, nil
}
func (s *sTestStore) GetRulesHash() ([]byte, bool, error) {
	s.mu.RLock(); defer s.mu.RUnlock()
	v, ok := s.hashes["rules_hash"]; return v, ok, nil
}
func (s *sTestStore) PutRulesHash(hash []byte) error {
	s.mu.Lock(); defer s.mu.Unlock(); s.hashes["rules_hash"] = hash; return nil
}
func (s *sTestStore) AppendCanonical(block *core.Block) error {
	s.mu.Lock(); defer s.mu.Unlock(); s.canonical = append(s.canonical, block); return nil
}
func (s *sTestStore) RewriteCanonical(blocks []*core.Block) error { return s.SaveCanonicalChain(blocks) }
func (s *sTestStore) PutBlock(block *core.Block) error            { return s.SaveBlock(block) }
func (s *sTestStore) GetGenesisHash() ([]byte, bool, error) {
	s.mu.RLock(); defer s.mu.RUnlock()
	v, ok := s.hashes["genesis_hash"]; return v, ok, nil
}
func (s *sTestStore) PutGenesisHash(hash []byte) error {
	s.mu.Lock(); defer s.mu.Unlock(); s.hashes["genesis_hash"] = hash; return nil
}
func (s *sTestStore) PutAccount(address string, account core.Account) error {
	s.mu.Lock(); defer s.mu.Unlock(); s.accounts[address] = account; return nil
}
func (s *sTestStore) GetAccount(address string) (core.Account, bool, error) {
	s.mu.RLock(); defer s.mu.RUnlock()
	acct, ok := s.accounts[address]; return acct, ok, nil
}
func (s *sTestStore) BatchPutAccounts(accounts map[string]core.Account) error {
	s.mu.Lock(); defer s.mu.Unlock()
	for a, acct := range accounts { s.accounts[a] = acct }
	return nil
}

func (s *sTestStore) CalculateStateRoot(state map[string]core.Account) ([]byte, error) {
	h := sha256.New()
	for addr := range state { h.Write([]byte(addr)) }
	return h.Sum(nil), nil
}
func (s *sTestStore) Snapshot(height uint64, stateRoot []byte, state map[string]core.Account) error { return nil }
func (s *sTestStore) LoadSnapshot(height uint64) (uint64, []byte, map[string]core.Account, error) { return 0, nil, nil, nil }
func (s *sTestStore) LatestSnapshot() (uint64, error)                                          { return 0, nil }
func (s *sTestStore) DeleteSnapshot(height uint64) error                                       { return nil }
func (s *sTestStore) PutCheckpointEntry(height uint64, hash string) error                      { return nil }
func (s *sTestStore) GetCheckpointByHeight(height uint64) (string, bool, error)                { return "", false, nil }
func (s *sTestStore) GetCheckpoints() ([]byte, bool, error)                                    { return nil, false, nil }
func (s *sTestStore) PutCheckpoints(data []byte) error                                         { return nil }
func (s *sTestStore) LatestCheckpoint() (uint64, string, error)                                { return 0, "", nil }
func (s *sTestStore) Close() error                                                             { return nil }
func (s *sTestStore) Path() string                                                             { return "" }

type nilLogWriter struct{}

func (nilLogWriter) Write(p []byte) (n int, err error) { return len(p), nil }
