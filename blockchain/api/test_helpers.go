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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package api

import (
	"encoding/hex"
	"math/big"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/mempool"
	"github.com/nogochain/nogo/blockchain/network"
)

// mockBlockchain provides mock implementation for testing
type mockBlockchain struct {
	blocks      []*core.Block
	txIndex     map[string]*core.TxLocation
	state       map[string]core.Account
	chainID     uint64
	minerAddr   string
	latestBlock *core.Block
}

// SetAccount sets the account state for testing
func (m *mockBlockchain) SetAccount(addr string, account core.Account) {
	m.state[addr] = account
}

func newMockBlockchain() *mockBlockchain {
	return &mockBlockchain{
		blocks:    make([]*core.Block, 0),
		txIndex:   make(map[string]*core.TxLocation),
		state:     make(map[string]core.Account),
		chainID:   1,
		minerAddr: "NOGO0000000000000000000000000000000000000000000000000000000000",
	}
}

func (m *mockBlockchain) LatestBlock() *core.Block {
	if m.latestBlock != nil {
		return m.latestBlock
	}
	if len(m.blocks) > 0 {
		return m.blocks[len(m.blocks)-1]
	}
	return &core.Block{Height: 0, Hash: make([]byte, 32)}
}

func (m *mockBlockchain) BlockByHeight(height uint64) (*core.Block, bool) {
	if height >= uint64(len(m.blocks)) {
		return nil, false
	}
	return m.blocks[height], true
}

func (m *mockBlockchain) BlockByHash(hashHex string) (*core.Block, bool) {
	for _, block := range m.blocks {
		if hex.EncodeToString(block.Hash) == hashHex {
			return block, true
		}
	}
	return nil, false
}

func (m *mockBlockchain) HeadersFrom(from uint64, count uint64) []*core.BlockHeader {
	headers := make([]*core.BlockHeader, 0)
	for i := from; i < from+count && i < uint64(len(m.blocks)); i++ {
		headers = append(headers, &m.blocks[i].Header)
	}
	return headers
}

func (m *mockBlockchain) BlocksFrom(from uint64, count uint64) []*core.Block {
	if from >= uint64(len(m.blocks)) {
		return nil
	}
	end := from + count
	if end > uint64(len(m.blocks)) {
		end = uint64(len(m.blocks))
	}
	return m.blocks[from:end]
}

func (m *mockBlockchain) Blocks() []*core.Block {
	return m.blocks
}

func (m *mockBlockchain) CanonicalWork() *big.Int {
	return big.NewInt(0)
}

func (m *mockBlockchain) RulesHashHex() string {
	return ""
}

func (m *mockBlockchain) GetChainID() uint64 {
	return m.chainID
}

func (m *mockBlockchain) GetMinerAddress() string {
	return m.minerAddr
}

func (m *mockBlockchain) TotalSupply() uint64 {
	return 0
}

func (m *mockBlockchain) AddBlock(block *core.Block) (bool, error) {
	m.blocks = append(m.blocks, block)
	return true, nil
}

func (m *mockBlockchain) RollbackToHeight(height uint64) error {
	if height >= uint64(len(m.blocks)) {
		return nil
	}
	m.blocks = m.blocks[:height+1]
	return nil
}

func (m *mockBlockchain) SelectMempoolTxs(mp network.Mempool, maxTxPerBlock int) ([]core.Transaction, []string, error) {
	return nil, nil, nil
}

func (m *mockBlockchain) MineTransfers(txs []core.Transaction) (*core.Block, error) {
	return nil, nil
}

func (m *mockBlockchain) AuditChain() error {
	return nil
}

func (m *mockBlockchain) TxByID(txid string) (*core.Transaction, *core.TxLocation, bool) {
	loc, exists := m.txIndex[txid]
	if !exists {
		return nil, nil, false
	}
	block := m.blocks[loc.Height]
	if loc.Index >= len(block.Transactions) {
		return nil, nil, false
	}
	tx := block.Transactions[loc.Index]
	return &tx, loc, true
}

func (m *mockBlockchain) AddressTxs(addr string, limit, cursor int) ([]core.AddressTxEntry, int, bool) {
	return nil, 0, false
}

func (m *mockBlockchain) Balance(addr string) (core.Account, bool) {
	acct, exists := m.state[addr]
	return acct, exists
}

func (m *mockBlockchain) HasTransaction(txHash []byte) bool {
	return false
}

func (m *mockBlockchain) GetContractManager() *core.ContractManager {
	return nil
}

func (m *mockBlockchain) SyncLoop() network.SyncLoopInterface {
	return nil
}

func (m *mockBlockchain) CalcNextDifficulty(latest *core.Block, currentTime int64) uint32 {
	// Simple mock implementation: return fixed difficulty for tests
	if latest == nil {
		return 8
	}
	return latest.Header.DifficultyBits
}

func (m *mockBlockchain) GetBlockByHash(hash []byte) (*core.Block, bool) {
	return nil, false
}

func (m *mockBlockchain) GetBlockByHashBytes(hash []byte) (*core.Block, bool) {
	return nil, false
}

func (m *mockBlockchain) GetAllBlocks() ([]*core.Block, error) {
	return nil, nil
}

// IsReorgInProgress returns false for mock (no reorg in tests)
func (m *mockBlockchain) IsReorgInProgress() bool {
	return false
}

// GetConsensus returns default consensus params for mock
func (m *mockBlockchain) GetConsensus() config.ConsensusParams {
	return config.DefaultConfig().Consensus
}

// createTestServer creates a test server with mock dependencies
func createTestServer(bc *mockBlockchain, mp *mempool.Mempool) *Server {
	return NewServer(
		bc,
		"",
		mp,
		nil,
		nil,
		false,
		nil,
		"",
		nil,
		false,
		false,
		nil,
	)
}
