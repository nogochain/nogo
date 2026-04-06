package main

import (
	"context"
	"encoding/json"
	"math/big"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/mempool"
	"github.com/nogochain/nogo/blockchain/metrics"
	"github.com/nogochain/nogo/blockchain/miner"
	"github.com/nogochain/nogo/blockchain/network"
)

// chainWrapper wraps core.Chain to implement miner.Blockchain
type chainWrapper struct {
	chain *core.Chain
}

func newChainWrapper(chain *core.Chain) *chainWrapper {
	return &chainWrapper{chain: chain}
}

// LatestBlock returns the latest block
func (w *chainWrapper) LatestBlock() *core.Block {
	return w.chain.GetTip()
}

// BlockByHeight returns block by height (for network.BlockchainInterface)
func (w *chainWrapper) BlockByHeight(height uint64) (*core.Block, bool) {
	return w.chain.GetBlockByHeight(height)
}

// SelectMempoolTxs selects transactions from mempool for miner.Blockchain
func (w *chainWrapper) SelectMempoolTxs(mp miner.Mempool, maxTxPerBlock int) ([]core.Transaction, []string, error) {
	entries := mp.EntriesSortedByFeeDesc()
	var picked []core.Transaction
	var pickedIDs []string

	// Track expected nonces for each address based on already-picked transactions
	expectedNonces := make(map[string]uint64)

	for _, e := range entries {
		if len(picked) >= maxTxPerBlock {
			break
		}
		tx := e.Tx()
		if tx.Type != core.TxTransfer {
			continue
		}
		if tx.ChainID == 0 {
			tx.ChainID = w.chain.GetChainID()
		}
		if err := tx.VerifyForConsensus(w.chain.GetConsensus(), w.chain.LatestBlock().Height+1); err != nil {
			continue
		}

		fromAddr, _ := tx.FromAddress()
		
		// Get current nonce from chain state
		acct, exists := w.chain.Balance(fromAddr)
		if !exists {
			acct = core.Account{Balance: 0, Nonce: 0}
		}
		
		// Check if we've already picked a transaction from this address
		expectedNonce, hasPending := expectedNonces[fromAddr]
		if hasPending {
			// Use the next expected nonce for this address
			if tx.Nonce != expectedNonce {
				continue
			}
		} else {
			// First transaction from this address should use chain nonce + 1
			if tx.Nonce != acct.Nonce+1 {
				continue
			}
		}
		
		totalDebit := tx.Amount + tx.Fee
		if acct.Balance < totalDebit {
			continue
		}

		// Update balance and nonce for next transaction from this address
		acct.Balance -= totalDebit
		acct.Nonce = tx.Nonce
		expectedNonces[fromAddr] = tx.Nonce + 1

		picked = append(picked, tx)
		pickedIDs = append(pickedIDs, e.TxID())
	}

	return picked, pickedIDs, nil
}

// SelectMempoolTxsNetwork selects transactions from mempool for network.BlockchainInterface
func (w *chainWrapper) SelectMempoolTxsNetwork(mp network.Mempool, maxTxPerBlock int) ([]core.Transaction, []string, error) {
	entries := mp.EntriesSortedByFeeDesc()
	var picked []core.Transaction
	var pickedIDs []string

	// Track expected nonces for each address based on already-picked transactions
	expectedNonces := make(map[string]uint64)

	for _, e := range entries {
		if len(picked) >= maxTxPerBlock {
			break
		}
		tx := e.Tx
		if tx.Type != core.TxTransfer {
			continue
		}
		if tx.ChainID == 0 {
			tx.ChainID = w.chain.GetChainID()
		}
		if err := tx.VerifyForConsensus(w.chain.GetConsensus(), w.chain.LatestBlock().Height+1); err != nil {
			continue
		}

		fromAddr, _ := tx.FromAddress()
		
		// Get current nonce from chain state
		acct, exists := w.chain.Balance(fromAddr)
		if !exists {
			acct = core.Account{Balance: 0, Nonce: 0}
		}
		
		// Check if we've already picked a transaction from this address
		expectedNonce, hasPending := expectedNonces[fromAddr]
		if hasPending {
			// Use the next expected nonce for this address
			if tx.Nonce != expectedNonce {
				continue
			}
		} else {
			// First transaction from this address should use chain nonce + 1
			if tx.Nonce != acct.Nonce+1 {
				continue
			}
		}
		
		totalDebit := tx.Amount + tx.Fee
		if acct.Balance < totalDebit {
			continue
		}

		// Update balance and nonce for next transaction from this address
		acct.Balance -= totalDebit
		acct.Nonce = tx.Nonce
		expectedNonces[fromAddr] = tx.Nonce + 1

		picked = append(picked, tx)
		pickedIDs = append(pickedIDs, e.TxIDHex)
	}

	return picked, pickedIDs, nil
}

// MineTransfers mines transfers into a block
func (w *chainWrapper) MineTransfers(txs []core.Transaction) (*core.Block, error) {
	return w.chain.MineTransfers(txs)
}

// CanonicalWork returns the total work on canonical chain
func (w *chainWrapper) CanonicalWork() *big.Int {
	return w.chain.GetCanonicalWork()
}

// RollbackToHeight rolls back to a given height
func (w *chainWrapper) RollbackToHeight(height uint64) error {
	return w.chain.RollbackToHeight(height)
}

// AddBlock adds a block to the chain (for network.BlockchainInterface)
func (w *chainWrapper) AddBlock(block *core.Block) (bool, error) {
	return w.chain.AddBlock(block)
}

// AddressTxs returns transactions for an address (for network.BlockchainInterface)
func (w *chainWrapper) AddressTxs(addr string, limit, cursor int) ([]core.AddressTxEntry, int, bool) {
	return w.chain.AddressTxs(addr, limit, cursor)
}

// Balance returns account balance (for network.BlockchainInterface)
func (w *chainWrapper) Balance(addr string) (core.Account, bool) {
	return w.chain.Balance(addr)
}

// TxByID returns transaction by ID (for network.BlockchainInterface)
func (w *chainWrapper) TxByID(txid string) (*core.Transaction, *core.TxLocation, bool) {
	return w.chain.TxByID(txid)
}

// AuditChain audits the chain integrity (for network.BlockchainInterface)
func (w *chainWrapper) AuditChain() error {
	return w.chain.AuditChain()
}

// BlockByHash returns block by hash (for network.BlockchainInterface)
func (w *chainWrapper) BlockByHash(hashHex string) (*core.Block, bool) {
	return w.chain.GetBlockByHashHex(hashHex)
}

// HeadersFrom returns block headers from a given height
func (w *chainWrapper) HeadersFrom(from uint64, count uint64) []*core.BlockHeader {
	blocks := w.chain.GetBlocksFrom(from, count)
	headers := make([]*core.BlockHeader, len(blocks))
	for i, block := range blocks {
		headers[i] = &block.Header
	}
	return headers
}

// BlocksFrom returns blocks from a given height
func (w *chainWrapper) BlocksFrom(from uint64, count uint64) []*core.Block {
	return w.chain.GetBlocksFrom(from, count)
}

// Blocks returns all blocks on canonical chain
func (w *chainWrapper) Blocks() []*core.Block {
	return w.chain.GetCanonicalBlocks()
}

// GetChainID returns the chain ID
func (w *chainWrapper) GetChainID() uint64 {
	return w.chain.GetChainID()
}

// GetMinerAddress returns the miner address
func (w *chainWrapper) GetMinerAddress() string {
	return w.chain.GetMinerAddress()
}

// TotalSupply returns the total supply
func (w *chainWrapper) TotalSupply() uint64 {
	return w.chain.TotalSupply()
}

// SyncLoop returns the sync loop interface
func (w *networkChainWrapper) SyncLoop() network.SyncLoopInterface {
	return w.syncLoop
}

// GetConsensus returns consensus parameters
func (w *chainWrapper) GetConsensus() config.ConsensusParams {
	return w.chain.GetConsensus()
}

// RulesHashHex returns the rules hash
func (w *chainWrapper) RulesHashHex() string {
	return w.chain.GetRulesHashHex()
}

// networkChainWrapper wraps core.Chain to implement network.BlockchainInterface
type networkChainWrapper struct {
	chain    *core.Chain
	syncLoop *network.SyncLoop
}

func newNetworkChainWrapper(chain *core.Chain) *networkChainWrapper {
	return &networkChainWrapper{chain: chain}
}

// SetSyncLoop sets the sync loop reference
func (w *networkChainWrapper) SetSyncLoop(syncLoop *network.SyncLoop) {
	w.syncLoop = syncLoop
}

// LatestBlock returns the latest block
func (w *networkChainWrapper) LatestBlock() *core.Block {
	return w.chain.GetTip()
}

// BlockByHeight returns block by height
func (w *networkChainWrapper) BlockByHeight(height uint64) (*core.Block, bool) {
	return w.chain.GetBlockByHeight(height)
}

// BlockByHash returns block by hash
func (w *networkChainWrapper) BlockByHash(hashHex string) (*core.Block, bool) {
	return w.chain.GetBlockByHashHex(hashHex)
}

// HeadersFrom returns block headers from a given height
func (w *networkChainWrapper) HeadersFrom(from uint64, count uint64) []*core.BlockHeader {
	blocks := w.chain.GetBlocksFrom(from, count)
	headers := make([]*core.BlockHeader, len(blocks))
	for i, block := range blocks {
		headers[i] = &block.Header
	}
	return headers
}

// BlocksFrom returns blocks from a given height
func (w *networkChainWrapper) BlocksFrom(from uint64, count uint64) []*core.Block {
	return w.chain.GetBlocksFrom(from, count)
}

// Blocks returns all blocks on canonical chain
func (w *networkChainWrapper) Blocks() []*core.Block {
	return w.chain.GetCanonicalBlocks()
}

// CanonicalWork returns the total work on canonical chain
func (w *networkChainWrapper) CanonicalWork() *big.Int {
	return w.chain.GetCanonicalWork()
}

// RulesHashHex returns the rules hash
func (w *networkChainWrapper) RulesHashHex() string {
	return w.chain.GetRulesHashHex()
}

// GetChainID returns the chain ID
func (w *networkChainWrapper) GetChainID() uint64 {
	return w.chain.GetChainID()
}

// GetMinerAddress returns the miner address
func (w *networkChainWrapper) GetMinerAddress() string {
	return w.chain.GetMinerAddress()
}

// TotalSupply returns the total supply
func (w *networkChainWrapper) TotalSupply() uint64 {
	return w.chain.TotalSupply()
}

// AddBlock adds a block to the chain
func (w *networkChainWrapper) AddBlock(block *core.Block) (bool, error) {
	return w.chain.AddBlock(block)
}

// SelectMempoolTxs selects transactions from mempool for network.BlockchainInterface
func (w *networkChainWrapper) SelectMempoolTxs(mp network.Mempool, maxTxPerBlock int) ([]core.Transaction, []string, error) {
	entries := mp.EntriesSortedByFeeDesc()
	txs := make([]core.Transaction, 0, maxTxPerBlock)
	txids := make([]string, 0)
	count := 0
	for _, entry := range entries {
		if count >= maxTxPerBlock {
			break
		}
		txs = append(txs, entry.Tx)
		txids = append(txids, entry.TxIDHex)
		count++
	}
	return txs, txids, nil
}

// MineTransfers mines transfers into a block
func (w *networkChainWrapper) MineTransfers(txs []core.Transaction) (*core.Block, error) {
	return w.chain.MineTransfers(txs)
}

// AuditChain audits the chain integrity
func (w *networkChainWrapper) AuditChain() error {
	return w.chain.AuditChain()
}

// TxByID returns transaction by ID
func (w *networkChainWrapper) TxByID(txid string) (*core.Transaction, *core.TxLocation, bool) {
	return w.chain.TxByID(txid)
}

// AddressTxs returns transactions for an address
func (w *networkChainWrapper) AddressTxs(addr string, limit, cursor int) ([]core.AddressTxEntry, int, bool) {
	txs, nextCursor, more := w.chain.AddressTxs(addr, limit, cursor)
	return txs, nextCursor, more
}

// Balance returns account balance
func (w *networkChainWrapper) Balance(addr string) (core.Account, bool) {
	return w.chain.Balance(addr)
}

// GetConsensus returns consensus parameters
func (w *networkChainWrapper) GetConsensus() config.ConsensusParams {
	return w.chain.GetConsensus()
}

// RollbackToHeight rolls back to a given height
func (w *networkChainWrapper) RollbackToHeight(height uint64) error {
	return w.chain.RollbackToHeight(height)
}

// mempoolWrapper wraps mempool.Mempool to implement network.Mempool
type mempoolWrapper struct {
	mp *mempool.Mempool
}

func newMempoolWrapper(mp *mempool.Mempool) *mempoolWrapper {
	return &mempoolWrapper{mp: mp}
}

// Contains checks if transaction exists in mempool
func (w *mempoolWrapper) Contains(txID string) bool {
	return w.mp.Contains(txID)
}

// GetTx returns transaction by ID
func (w *mempoolWrapper) GetTx(txID string) (*core.Transaction, bool) {
	return w.mp.GetTx(txID)
}

// GetTxIDs returns all transaction IDs
func (w *mempoolWrapper) GetTxIDs() []string {
	return w.mp.GetTxIDs()
}

// Add adds a transaction to mempool
func (w *mempoolWrapper) Add(tx core.Transaction) (string, error) {
	return w.mp.Add(tx)
}

// Remove removes a transaction from mempool
func (w *mempoolWrapper) Remove(txID string) {
	w.mp.Remove(txID)
}

// RemoveMany removes multiple transactions
func (w *mempoolWrapper) RemoveMany(txids []string) {
	w.mp.RemoveMany(txids)
}

// Size returns mempool size
func (w *mempoolWrapper) Size() int {
	return w.mp.Size()
}

// EntriesSortedByFeeDesc returns entries sorted by fee for network.Mempool
func (w *mempoolWrapper) EntriesSortedByFeeDesc() []network.MempoolEntry {
	return w.EntriesSortedByFeeDescNetwork()
}

// EntriesSortedByFeeDescMiner returns entries for miner.Mempool
func (w *mempoolWrapper) EntriesSortedByFeeDescMiner() []miner.MempoolEntry {
	entries := w.mp.EntriesSortedByFeeDesc()
	result := make([]miner.MempoolEntry, len(entries))
	for i, entry := range entries {
		result[i] = &minerMempoolEntry{
			tx:       entry.Tx(),
			txIDHex:  entry.TxID(),
			received: entry.Received(),
		}
	}
	return result
}

// EntriesSortedByFeeDescNetwork returns entries for network.Mempool
func (w *mempoolWrapper) EntriesSortedByFeeDescNetwork() []network.MempoolEntry {
	entries := w.mp.EntriesSortedByFeeDesc()
	result := make([]network.MempoolEntry, len(entries))
	for i, entry := range entries {
		result[i] = network.MempoolEntry{
			Tx:       entry.Tx(),
			TxIDHex:  entry.TxID(),
			Received: entry.Received(),
		}
	}
	return result
}

// GetTxBytes returns transaction bytes (for backward compatibility)
func (w *mempoolWrapper) GetTxBytes(txID string) ([]byte, bool) {
	tx, exists := w.mp.GetTx(txID)
	if !exists {
		return nil, false
	}
	data, err := json.Marshal(tx)
	if err != nil {
		return nil, false
	}
	return data, true
}

// minerMempoolEntryWrapper wraps mempool.Mempool to implement miner.Mempool
type minerMempoolWrapper struct {
	mp *mempool.Mempool
}

func newMinerMempoolWrapper(mp *mempool.Mempool) *minerMempoolWrapper {
	return &minerMempoolWrapper{mp: mp}
}

// EntriesSortedByFeeDesc returns entries for miner.Mempool
func (w *minerMempoolWrapper) EntriesSortedByFeeDesc() []miner.MempoolEntry {
	return w.EntriesSortedByFeeDescMiner()
}

// EntriesSortedByFeeDescMiner returns entries for miner.Mempool
func (w *minerMempoolWrapper) EntriesSortedByFeeDescMiner() []miner.MempoolEntry {
	entries := w.mp.EntriesSortedByFeeDesc()
	result := make([]miner.MempoolEntry, len(entries))
	for i, entry := range entries {
		result[i] = &minerMempoolEntry{
			tx:       entry.Tx(),
			txIDHex:  entry.TxID(),
			received: entry.Received(),
		}
	}
	return result
}

// Size returns mempool size
func (w *minerMempoolWrapper) Size() int {
	return w.mp.Size()
}

// RemoveMany removes multiple transactions
func (w *minerMempoolWrapper) RemoveMany(txids []string) {
	w.mp.RemoveMany(txids)
}

// InterruptMining interrupts the mining process (for network.Miner compatibility)
func (w *minerMempoolWrapper) InterruptMining() {
	// Mempool doesn't have mining, so this is a no-op
}

// ResumeMining resumes the mining process (for network.Miner compatibility)
func (w *minerMempoolWrapper) ResumeMining() {
	// Mempool doesn't have mining, so this is a no-op
}

// IsVerifying returns true if verifying (for network.Miner compatibility)
func (w *minerMempoolWrapper) IsVerifying() bool {
	return false
}

// OnBlockAdded is called when a block is added (for network.Miner compatibility)
func (w *minerMempoolWrapper) OnBlockAdded() {
	// Mempool doesn't have block added callback, so this is a no-op
}

// minerMempoolEntry implements miner.MempoolEntry
type minerMempoolEntry struct {
	tx       core.Transaction
	txIDHex  string
	received time.Time
}

// Tx returns the transaction
func (e *minerMempoolEntry) Tx() core.Transaction {
	return e.tx
}

// TxID returns the transaction ID
func (e *minerMempoolEntry) TxID() string {
	return e.txIDHex
}

// Received returns the received time
func (e *minerMempoolEntry) Received() time.Time {
	return e.received
}

// networkMempoolWrapper wraps mempool for network package
type networkMempoolWrapper struct {
	mp *mempool.Mempool
}

// Contains checks if transaction exists
func (w *networkMempoolWrapper) Contains(txID string) bool {
	_, exists := w.mp.GetTx(txID)
	return exists
}

// GetTx returns transaction by ID
func (w *networkMempoolWrapper) GetTx(txID string) (*core.Transaction, bool) {
	return w.mp.GetTx(txID)
}

// GetTxIDs returns all transaction IDs
func (w *networkMempoolWrapper) GetTxIDs() []string {
	return w.mp.GetTxIDs()
}

// Add adds a transaction
func (w *networkMempoolWrapper) Add(tx core.Transaction) (string, error) {
	return w.mp.Add(tx)
}

// Remove removes a transaction
func (w *networkMempoolWrapper) Remove(txID string) {
	w.mp.Remove(txID)
}

// RemoveMany removes multiple transactions
func (w *networkMempoolWrapper) RemoveMany(txids []string) {
	w.mp.RemoveMany(txids)
}

// Size returns mempool size
func (w *networkMempoolWrapper) Size() int {
	return w.mp.Size()
}

// EntriesSortedByFeeDesc returns entries for network.Mempool
func (w *networkMempoolWrapper) EntriesSortedByFeeDesc() []network.MempoolEntry {
	entries := w.mp.EntriesSortedByFeeDesc()
	result := make([]network.MempoolEntry, len(entries))
	for i, entry := range entries {
		result[i] = network.MempoolEntry{
			Tx:       entry.Tx(),
			TxIDHex:  entry.TxID(),
			Received: entry.Received(),
		}
	}
	return result
}

// metricsMempoolWrapper wraps mempool for metrics package
type metricsMempoolWrapper struct {
	mp *mempool.Mempool
}

// Size returns mempool size
func (w *metricsMempoolWrapper) Size() int {
	return w.mp.Size()
}

// Snapshot returns mempool snapshot
func (w *metricsMempoolWrapper) Snapshot() []metrics.MempoolEntry {
	entries := w.mp.EntriesSortedByFeeDesc()
	result := make([]metrics.MempoolEntry, len(entries))
	for i, entry := range entries {
		result[i] = &metricsMempoolEntry{
			tx:       entry.Tx(),
			txIDHex:  entry.TxID(),
			received: entry.Received(),
		}
	}
	return result
}

// GetTxBytes returns transaction bytes size
func (w *metricsMempoolWrapper) GetTxBytes(tx core.Transaction) int {
	// Serialize transaction to bytes using JSON
	data, err := json.Marshal(tx)
	if err != nil {
		return 0
	}
	return len(data)
}

// metricsMempoolEntry implements metrics.MempoolEntry
type metricsMempoolEntry struct {
	tx       core.Transaction
	txIDHex  string
	received time.Time
}

// GetTx returns the transaction
func (e *metricsMempoolEntry) GetTx() core.Transaction {
	return e.tx
}

// GetTxID returns the transaction ID
func (e *metricsMempoolEntry) GetTxID() string {
	return e.txIDHex
}

// GetReceived returns the received time
func (e *metricsMempoolEntry) GetReceived() time.Time {
	return e.received
}

// p2pManagerWrapper wraps network.P2PPeerManager to implement miner.PeerAPI and network.PeerManager
type p2pManagerWrapper struct {
	mgr *network.P2PPeerManager
}

func newP2PManagerWrapper(mgr *network.P2PPeerManager) *p2pManagerWrapper {
	return &p2pManagerWrapper{mgr: mgr}
}

// Peers returns list of peer addresses
func (w *p2pManagerWrapper) Peers() []string {
	return w.mgr.Peers()
}

// FetchChainInfo fetches chain info from a peer (for miner.PeerAPI)
func (w *p2pManagerWrapper) FetchChainInfo(ctx context.Context, peer string) (*miner.ChainInfo, error) {
	networkInfo, err := w.mgr.FetchChainInfo(ctx, peer)
	if err != nil {
		return nil, err
	}
	// Convert network.ChainInfo to miner.ChainInfo
	return &miner.ChainInfo{
		Height: networkInfo.Height,
		Work:   networkInfo.Work,
	}, nil
}

// FetchChainInfoNetwork fetches chain info from a peer (for network.PeerAPI)
func (w *p2pManagerWrapper) FetchChainInfoNetwork(ctx context.Context, peer string) (*network.ChainInfo, error) {
	return w.mgr.FetchChainInfo(ctx, peer)
}

// AddPeer adds a peer
func (w *p2pManagerWrapper) AddPeer(addr string) bool {
	w.mgr.AddPeer(addr)
	return true
}

// RemovePeer removes a peer
func (w *p2pManagerWrapper) RemovePeer(addr string) {
	// P2PPeerManager doesn't have RemovePeer, so this is a no-op
}

// GetActivePeers returns active peers
func (w *p2pManagerWrapper) GetActivePeers() []string {
	return w.mgr.GetActivePeers()
}

// BroadcastTransaction broadcasts a transaction to all peers
func (w *p2pManagerWrapper) BroadcastTransaction(ctx context.Context, tx core.Transaction, maxHops int) {
	// P2PPeerManager doesn't have this method, so this is a no-op for now
}

// InterruptMining interrupts the mining process
func (w *p2pManagerWrapper) InterruptMining() {
	// P2PPeerManager doesn't have mining, so this is a no-op
}

// ResumeMining resumes the mining process
func (w *p2pManagerWrapper) ResumeMining() {
	// P2PPeerManager doesn't have mining, so this is a no-op
}

// IsVerifying returns true if verifying
func (w *p2pManagerWrapper) IsVerifying() bool {
	return false
}

// OnBlockAdded is called when a block is added
func (w *p2pManagerWrapper) OnBlockAdded() {
	// P2PPeerManager doesn't have block added callback, so this is a no-op
}

// EnsureAncestors ensures ancestor blocks are synced
func (w *p2pManagerWrapper) EnsureAncestors(ctx context.Context, bc network.BlockchainInterface, missingHashHex string) error {
	// P2PPeerManager doesn't have this method, so this is a no-op for now
	return nil
}

// FetchBlocks fetches blocks from a peer
func (w *p2pManagerWrapper) FetchBlocks(ctx context.Context, peer string, from uint64, count uint64) ([]*core.Block, error) {
	// P2PPeerManager doesn't have this method, so this is a no-op for now
	return nil, nil
}

// FetchHeaders fetches headers from a peer
func (w *p2pManagerWrapper) FetchHeaders(ctx context.Context, peer string, from uint64, count uint64) ([]*core.BlockHeader, error) {
	// P2PPeerManager doesn't have this method, so this is a no-op for now
	return nil, nil
}

// FetchHeadersFrom fetches headers from a peer
func (w *p2pManagerWrapper) FetchHeadersFrom(ctx context.Context, peer string, fromHeight uint64, count int) ([]core.BlockHeader, error) {
	// P2PPeerManager doesn't have this method, so this is a no-op for now
	return nil, nil
}

// FetchBlockByHash fetches a block by hash from a peer
func (w *p2pManagerWrapper) FetchBlockByHash(ctx context.Context, peer, hashHex string) (*core.Block, error) {
	// P2PPeerManager doesn't have this method, so this is a no-op for now
	return nil, nil
}

// FetchAnyBlockByHash fetches a block by hash from any peer
func (w *p2pManagerWrapper) FetchAnyBlockByHash(ctx context.Context, hashHex string) (*core.Block, string, error) {
	// P2PPeerManager doesn't have this method, so this is a no-op for now
	return nil, "", nil
}

// metricsChainWrapper wraps core.Chain for metrics package
type metricsChainWrapper struct {
	chain *core.Chain
}

func newMetricsChainWrapper(chain *core.Chain) *metricsChainWrapper {
	return &metricsChainWrapper{chain: chain}
}

// LatestBlock returns the latest block
func (w *metricsChainWrapper) LatestBlock() *core.Block {
	return w.chain.GetTip()
}

// CanonicalTxCount returns total transaction count
func (w *metricsChainWrapper) CanonicalTxCount() uint64 {
	return w.chain.GetTxCount()
}

// BlockByHeight returns block by height
func (w *metricsChainWrapper) BlockByHeight(height uint64) (*core.Block, bool) {
	return w.chain.GetBlockByHeight(height)
}

// syncLoopWrapper wraps network.SyncLoop for metrics package
type syncLoopWrapper struct {
	loop *network.SyncLoop
}

func newSyncLoopWrapper(loop *network.SyncLoop) *syncLoopWrapper {
	return &syncLoopWrapper{loop: loop}
}

// GetPeerManager returns peer manager
func (w *syncLoopWrapper) GetPeerManager() metrics.PeerManager {
	// Return nil for now since we can't access the internal peer manager
	return nil
}

// IsSyncing returns true if syncing
func (w *syncLoopWrapper) IsSyncing() bool {
	return w.loop.IsSyncing()
}

// GetOrphanPoolSize returns orphan pool size
func (w *syncLoopWrapper) GetOrphanPoolSize() int {
	return w.loop.GetOrphanPoolSize()
}

// IsMining returns true if mining
func (w *syncLoopWrapper) IsMining() bool {
	return w.loop.IsMining()
}

// GetActiveWorkerCount returns active worker count
func (w *syncLoopWrapper) GetActiveWorkerCount() int {
	return w.loop.GetActiveWorkerCount()
}

// metricsPeerManager wraps network.P2PPeerManager for metrics
type metricsPeerManager struct {
	mgr *network.P2PPeerManager
}

func newMetricsPeerManager(mgr *network.P2PPeerManager) *metricsPeerManager {
	return &metricsPeerManager{mgr: mgr}
}

// Peers returns list of peers
func (w *metricsPeerManager) Peers() []string {
	return w.mgr.Peers()
}

// Count returns peer count
func (w *metricsPeerManager) Count() int {
	return len(w.mgr.Peers())
}

// MaxPeers returns max peers
func (w *metricsPeerManager) MaxPeers() int {
	return 100 // Default max peers
}

// GetPeerScore returns peer score
func (w *metricsPeerManager) GetPeerScore(peerID string) float64 {
	return 1.0 // Default score
}

// GetPeerLatency returns peer latency
func (w *metricsPeerManager) GetPeerLatency(peerID string) time.Duration {
	return 100 * time.Millisecond // Default latency
}

// metricsWrapper wraps metrics.Metrics for api package
type metricsWrapper struct {
	m *metrics.Metrics
}

func newMetricsWrapper(m *metrics.Metrics) *metricsWrapper {
	return &metricsWrapper{m: m}
}
