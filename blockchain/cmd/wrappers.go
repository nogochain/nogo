package main

import (
	"context"
	"encoding/json"
	"fmt"
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
		if err := tx.VerifyForConsensus(w.chain.GetConsensus(), w.chain.LatestBlock().GetHeight()+1); err != nil {
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
		if err := tx.VerifyForConsensus(w.chain.GetConsensus(), w.chain.LatestBlock().GetHeight()+1); err != nil {
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
func (w *chainWrapper) MineTransfers(ctx context.Context, txs []core.Transaction) (*core.Block, error) {
	return w.chain.MineTransfers(ctx, txs)
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
		header := &core.BlockHeader{
			Version:        block.Header.Version,
			PrevHash:       block.Header.PrevHash,
			TimestampUnix:  block.Header.TimestampUnix,
			DifficultyBits: block.Header.DifficultyBits,
			Difficulty:     block.Header.Difficulty,
			Nonce:          block.Header.Nonce,
			MerkleRoot:     block.Header.MerkleRoot,
			Height:         from + uint64(i),
			MinerAddress:   block.MinerAddress,
		}
		headers[i] = header
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

// BestBlockHeader returns the best (tip) block header with height for block locator
func (w *chainWrapper) BestBlockHeader() (*network.HeaderLocator, error) {
	header, height := w.chain.GetTipHeader()
	if header == nil {
		return nil, fmt.Errorf("chain is empty, no tip header available")
	}
	return &network.HeaderLocator{Header: header, Height: height}, nil
}

// GetHeaderByHeight returns the block header at the given height for block locator
func (w *chainWrapper) GetHeaderByHeight(height uint64) (*network.HeaderLocator, error) {
	header, ok := w.chain.GetHeaderAtHeight(height)
	if !ok || header == nil {
		return nil, fmt.Errorf("header not found at height %d", height)
	}
	return &network.HeaderLocator{Header: header, Height: height}, nil
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
		header := &core.BlockHeader{
			Version:        block.Header.Version,
			PrevHash:       block.Header.PrevHash,
			TimestampUnix:  block.Header.TimestampUnix,
			DifficultyBits: block.Header.DifficultyBits,
			Difficulty:     block.Header.Difficulty,
			Nonce:          block.Header.Nonce,
			MerkleRoot:     block.Header.MerkleRoot,
			Height:         from + uint64(i),
			MinerAddress:   block.MinerAddress,
		}
		headers[i] = header
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

// BestBlockHeader returns the best (tip) block header with height for block locator
func (w *networkChainWrapper) BestBlockHeader() (*network.HeaderLocator, error) {
	header, height := w.chain.GetTipHeader()
	if header == nil {
		return nil, fmt.Errorf("chain is empty, no tip header available")
	}
	return &network.HeaderLocator{Header: header, Height: height}, nil
}

// GetHeaderByHeight returns the block header at the given height for block locator
func (w *networkChainWrapper) GetHeaderByHeight(height uint64) (*network.HeaderLocator, error) {
	header, ok := w.chain.GetHeaderAtHeight(height)
	if !ok || header == nil {
		return nil, fmt.Errorf("header not found at height %d", height)
	}
	return &network.HeaderLocator{Header: header, Height: height}, nil
}

// GetChainID returns the chain ID
func (w *networkChainWrapper) GetChainID() uint64 {
	return w.chain.GetChainID()
}

// GetBlockByHash retrieves a block by hash (implements BlockProvider)
func (w *networkChainWrapper) GetBlockByHash(hash []byte) (*core.Block, bool) {
	return w.chain.GetBlockByHash(hash)
}

// GetBlockByHashBytes retrieves a block by hash bytes (implements BlockchainInterface)
func (w *networkChainWrapper) GetBlockByHashBytes(hash []byte) (*core.Block, bool) {
	return w.chain.GetBlockByHash(hash)
}

// GetAllBlocks returns all blocks on canonical chain (implements BlockProvider)
func (w *networkChainWrapper) GetAllBlocks() ([]*core.Block, error) {
	return w.chain.GetAllBlocks()
}

// GetChain returns the underlying chain instance
func (w *networkChainWrapper) GetChain() *core.Chain {
	return w.chain
}

// GetUnderlyingChain returns the underlying chain instance (for fork resolution)
func (w *networkChainWrapper) GetUnderlyingChain() *core.Chain {
	return w.chain
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
func (w *networkChainWrapper) MineTransfers(ctx context.Context, txs []core.Transaction) (*core.Block, error) {
	return w.chain.MineTransfers(ctx, txs)
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

// HasTransaction checks if a transaction exists in the blockchain
func (w *networkChainWrapper) HasTransaction(txHash []byte) bool {
	return w.chain.HasTransaction(txHash)
}

// GetContractManager returns the contract manager
func (w *networkChainWrapper) GetContractManager() *core.ContractManager {
	return w.chain.GetContractManager()
}

// GetConsensus returns consensus parameters
func (w *networkChainWrapper) GetConsensus() config.ConsensusParams {
	return w.chain.GetConsensus()
}

// RollbackToHeight rolls back to a given height
func (w *networkChainWrapper) RollbackToHeight(height uint64) error {
	return w.chain.RollbackToHeight(height)
}

// SetOnMissingBlock sets the missing block callback
func (w *networkChainWrapper) SetOnMissingBlock(callback func(parentHash []byte, height uint64)) {
	w.chain.SetOnMissingBlock(callback)
}

// CalcNextDifficulty calculates the difficulty for the next block
func (w *networkChainWrapper) CalcNextDifficulty(latest *core.Block, currentTime int64) uint32 {
	return w.chain.CalcNextDifficulty(latest, currentTime)
}

// IsReorgInProgress returns whether a reorganization is in progress
func (w *networkChainWrapper) IsReorgInProgress() bool {
	return w.chain.IsReorgInProgress()
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

// UpdateHeight updates the current height for transaction validation
func (w *mempoolWrapper) UpdateHeight(height uint64) {
	w.mp.UpdateHeight(height)
}

// UpdateConsensus updates the consensus parameters for transaction validation
func (w *mempoolWrapper) UpdateConsensus(consensus config.ConsensusParams) {
	w.mp.UpdateConsensus(consensus)
}

// AddWithoutSignatureValidation adds a transaction without signature verification
func (w *mempoolWrapper) AddWithoutSignatureValidation(tx core.Transaction) (string, error) {
	return w.mp.AddWithoutSignatureValidation(tx)
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

// metricsWrapper wraps metrics.Metrics for api package
type metricsWrapper struct {
	m *metrics.Metrics
}

func newMetricsWrapper(m *metrics.Metrics) *metricsWrapper {
	return &metricsWrapper{m: m}
}
