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

package reactor

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/network/mconnection"
)

// mockSwitch implements SwitchInterface for testing.
type mockSwitch struct {
	mu          sync.Mutex
	reactors    map[string]Reactor
	broadcasts  []broadcastRecord
	sends       []sendRecord
}

type broadcastRecord struct {
	ChID byte
	Msg  []byte
}

type sendRecord struct {
	PeerID string
	ChID   byte
	Msg    []byte
}

func newMockSwitch() *mockSwitch {
	return &mockSwitch{
		reactors: make(map[string]Reactor),
	}
}

func (ms *mockSwitch) AddReactor(name string, reactor Reactor) Reactor {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.reactors[name] = reactor
	reactor.SetSwitch(ms)
	return reactor
}

func (ms *mockSwitch) Broadcast(chID byte, msg []byte) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	msgCopy := make([]byte, len(msg))
	copy(msgCopy, msg)
	ms.broadcasts = append(ms.broadcasts, broadcastRecord{ChID: chID, Msg: msgCopy})
}

func (ms *mockSwitch) Send(peerID string, chID byte, msg []byte) bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	msgCopy := make([]byte, len(msg))
	copy(msgCopy, msg)
	ms.sends = append(ms.sends, sendRecord{PeerID: peerID, ChID: chID, Msg: msgCopy})
	return true
}

// mockSyncHandler implements SyncHandler for testing.
type mockSyncHandler struct {
	mu            sync.Mutex
	getHeaders    []getHeadersCall
	headers       []headersCall
	getBlocks     []getBlocksCall
	blocks        []blocksCall
	getLocators   []getLocatorCall
	locators      []locatorCall
	notFounds     []notFoundCall
}

type getHeadersCall struct {
	PeerID string
	From   uint64
	Count  uint64
}

type headersCall struct {
	PeerID  string
	Headers []byte
	HasMore bool
}

type getBlocksCall struct {
	PeerID  string
	Heights []uint64
}

type blocksCall struct {
	PeerID string
	Blocks []byte
}

type getLocatorCall struct {
	PeerID    string
	TipHeight uint64
}

type locatorCall struct {
	PeerID   string
	Locators [][]byte
}

type notFoundCall struct {
	PeerID  string
	MsgType byte
	IDs     []string
}

func (m *mockSyncHandler) OnGetHeaders(peerID string, from uint64, count uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getHeaders = append(m.getHeaders, getHeadersCall{PeerID: peerID, From: from, Count: count})
	return nil
}

func (m *mockSyncHandler) OnHeaders(peerID string, headers []byte, hasMore bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.headers = append(m.headers, headersCall{PeerID: peerID, Headers: headers, HasMore: hasMore})
	return nil
}

func (m *mockSyncHandler) OnGetBlocks(peerID string, heights []uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getBlocks = append(m.getBlocks, getBlocksCall{PeerID: peerID, Heights: heights})
	return nil
}

func (m *mockSyncHandler) OnBlocks(peerID string, blocks []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocks = append(m.blocks, blocksCall{PeerID: peerID, Blocks: blocks})
	return nil
}

func (m *mockSyncHandler) OnGetBlockLocator(peerID string, tipHeight uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getLocators = append(m.getLocators, getLocatorCall{PeerID: peerID, TipHeight: tipHeight})
	return nil
}

func (m *mockSyncHandler) OnBlockLocator(peerID string, locators [][]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.locators = append(m.locators, locatorCall{PeerID: peerID, Locators: locators})
	return nil
}

func (m *mockSyncHandler) OnNotFound(peerID string, msgType byte, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notFounds = append(m.notFounds, notFoundCall{PeerID: peerID, MsgType: msgType, IDs: ids})
	return nil
}

func (m *mockSyncHandler) OnStatus(peerID string, height uint64, work string, latestHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return nil
}

// mockTxHandler implements TxHandler for testing.
type mockTxHandler struct {
	mu    sync.Mutex
	invTxs []invTxCall
	getTxs []getTxCall
	txs    []txCall
}

type invTxCall struct {
	PeerID string
	TxIDs  []string
}

type getTxCall struct {
	PeerID string
	TxIDs  []string
}

type txCall struct {
	PeerID string
	Txs    []core.Transaction
}

func (m *mockTxHandler) OnInvTx(peerID string, txIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invTxs = append(m.invTxs, invTxCall{PeerID: peerID, TxIDs: txIDs})
	return nil
}

func (m *mockTxHandler) OnGetTx(peerID string, txIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getTxs = append(m.getTxs, getTxCall{PeerID: peerID, TxIDs: txIDs})
	return nil
}

func (m *mockTxHandler) OnTx(peerID string, txs []core.Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.txs = append(m.txs, txCall{PeerID: peerID, Txs: txs})
	return nil
}

// mockBlockHandler implements BlockHandler for testing.
type mockBlockHandler struct {
	mu      sync.Mutex
	invBlks []invBlockCall
	getBlks []getBlockCall
	blks    []blockCall
}

type invBlockCall struct {
	PeerID      string
	BlockHashes []string
}

type getBlockCall struct {
	PeerID      string
	BlockHashes []string
}

type blockCall struct {
	PeerID string
	Blocks []*core.Block
}

func (m *mockBlockHandler) OnInvBlock(peerID string, blockHashes []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invBlks = append(m.invBlks, invBlockCall{PeerID: peerID, BlockHashes: blockHashes})
	return nil
}

func (m *mockBlockHandler) OnGetBlock(peerID string, blockHashes []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getBlks = append(m.getBlks, getBlockCall{PeerID: peerID, BlockHashes: blockHashes})
	return nil
}

func (m *mockBlockHandler) OnBlock(peerID string, blocks []*core.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blks = append(m.blks, blockCall{PeerID: peerID, Blocks: blocks})
	return nil
}

// TestReactorRegistration verifies that reactors can be registered with the switch.
func TestReactorRegistration(t *testing.T) {
	sw := newMockSwitch()

	syncHandler := &mockSyncHandler{}
	syncR, err := NewSyncReactor(syncHandler)
	if err != nil {
		t.Fatalf("NewSyncReactor failed: %v", err)
	}

	txHandler := &mockTxHandler{}
	txR, err := NewTxReactor(txHandler)
	if err != nil {
		t.Fatalf("NewTxReactor failed: %v", err)
	}

	blockHandler := &mockBlockHandler{}
	blockR, err := NewBlockReactor(blockHandler)
	if err != nil {
		t.Fatalf("NewBlockReactor failed: %v", err)
	}

	sw.AddReactor("sync", syncR)
	sw.AddReactor("tx", txR)
	sw.AddReactor("block", blockR)

	sw.mu.Lock()
	if len(sw.reactors) != 3 {
		t.Errorf("expected 3 reactors, got %d", len(sw.reactors))
	}
	if _, ok := sw.reactors["sync"]; !ok {
		t.Error("sync reactor not registered")
	}
	if _, ok := sw.reactors["tx"]; !ok {
		t.Error("tx reactor not registered")
	}
	if _, ok := sw.reactors["block"]; !ok {
		t.Error("block reactor not registered")
	}
	sw.mu.Unlock()
}

// TestMessageRoutingToCorrectReactor verifies that messages are dispatched
// to the correct reactor based on channel ID.
func TestMessageRoutingToCorrectReactor(t *testing.T) {
	sw := newMockSwitch()

	syncHandler := &mockSyncHandler{}
	syncR, err := NewSyncReactor(syncHandler)
	if err != nil {
		t.Fatalf("NewSyncReactor failed: %v", err)
	}

	txHandler := &mockTxHandler{}
	txR, err := NewTxReactor(txHandler)
	if err != nil {
		t.Fatalf("NewTxReactor failed: %v", err)
	}

	blockHandler := &mockBlockHandler{}
	blockR, err := NewBlockReactor(blockHandler)
	if err != nil {
		t.Fatalf("NewBlockReactor failed: %v", err)
	}

	sw.AddReactor("sync", syncR)
	sw.AddReactor("tx", txR)
	sw.AddReactor("block", blockR)

	peerID := "test-peer-1"

	getHeadersMsg, err := BuildGetHeadersMsg(0, 10)
	if err != nil {
		t.Fatalf("BuildGetHeadersMsg failed: %v", err)
	}

	txInvMsg, err := BuildTxInvMsg([]string{"tx1", "tx2"})
	if err != nil {
		t.Fatalf("BuildTxInvMsg failed: %v", err)
	}

	blockInvMsg, err := BuildBlockInvMsg([]string{"abc123"})
	if err != nil {
		t.Fatalf("BuildBlockInvMsg failed: %v", err)
	}

	syncR.Receive(mconnection.ChannelSync, peerID, getHeadersMsg)
	txR.Receive(mconnection.ChannelTx, peerID, txInvMsg)
	blockR.Receive(mconnection.ChannelBlock, peerID, blockInvMsg)

	syncHandler.mu.Lock()
	if len(syncHandler.getHeaders) != 1 {
		t.Errorf("expected 1 getHeaders call, got %d", len(syncHandler.getHeaders))
	} else {
		call := syncHandler.getHeaders[0]
		if call.From != 0 || call.Count != 10 {
			t.Errorf("unexpected getHeaders params: from=%d, count=%d", call.From, call.Count)
		}
	}
	syncHandler.mu.Unlock()

	txHandler.mu.Lock()
	if len(txHandler.invTxs) != 1 {
		t.Errorf("expected 1 invTx call, got %d", len(txHandler.invTxs))
	} else {
		call := txHandler.invTxs[0]
		if len(call.TxIDs) != 2 {
			t.Errorf("expected 2 txIDs, got %d", len(call.TxIDs))
		}
	}
	txHandler.mu.Unlock()

	blockHandler.mu.Lock()
	if len(blockHandler.invBlks) != 1 {
		t.Errorf("expected 1 invBlock call, got %d", len(blockHandler.invBlks))
	} else {
		call := blockHandler.invBlks[0]
		if len(call.BlockHashes) != 1 {
			t.Errorf("expected 1 blockHash, got %d", len(call.BlockHashes))
		}
	}
	blockHandler.mu.Unlock()
}

// TestPeerLifecycleHooks verifies AddPeer and RemovePeer are called correctly.
func TestPeerLifecycleHooks(t *testing.T) {
	syncHandler := &mockSyncHandler{}
	syncR, err := NewSyncReactor(syncHandler)
	if err != nil {
		t.Fatalf("NewSyncReactor failed: %v", err)
	}

	peerID := "peer-1"
	nodeInfo := map[string]string{
		"version": "1.0.0",
		"chainID": "1",
	}

	if err := syncR.AddPeer(peerID, nodeInfo); err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	reason := "connection closed"
	syncR.RemovePeer(peerID, reason)

	chs := syncR.GetChannels()
	if len(chs) != 1 {
		t.Errorf("expected 1 channel, got %d", len(chs))
	}
	if chs[0].ID != mconnection.ChannelSync {
		t.Errorf("expected channel ID 0x%02x, got 0x%02x", mconnection.ChannelSync, chs[0].ID)
	}
}

// TestBaseReactorDefaults verifies BaseReactor no-op implementations.
func TestBaseReactorDefaults(t *testing.T) {
	br := &BaseReactor{}

	if br.GetSwitch() != nil {
		t.Error("expected nil switch before SetSwitch")
	}

	sw := newMockSwitch()
	br.SetSwitch(sw)
	if br.GetSwitch() == nil {
		t.Error("expected non-nil switch after SetSwitch")
	}

	chs := []*mconnection.ChannelDescriptor{
		{ID: 0x01, Priority: 5, SendQueueCapacity: 10, RecvBufferCapacity: 10, RecvMessageCapacity: 100},
	}
	br.SetChannels(chs)

	returned := br.GetChannels()
	if len(returned) != 1 {
		t.Errorf("expected 1 channel, got %d", len(returned))
	}
	if returned[0].ID != 0x01 {
		t.Errorf("expected channel ID 0x01, got 0x%02x", returned[0].ID)
	}

	if err := br.AddPeer("p1", nil); err != nil {
		t.Errorf("BaseReactor.AddPeer should not error: %v", err)
	}

	br.RemovePeer("p1", "test")

	br.Receive(0x01, "p1", []byte{0x01, 0x02, 0x03})
}

// TestSyncReactorMessageParsing verifies all sync message types are parsed correctly.
func TestSyncReactorMessageParsing(t *testing.T) {
	tests := []struct {
		name    string
		build   func() ([]byte, error)
		msgType byte
	}{
		{
			name: "GetHeaders",
			build: func() ([]byte, error) {
				return BuildGetHeadersMsg(100, 50)
			},
			msgType: SyncMsgGetHeaders,
		},
		{
			name: "Headers",
			build: func() ([]byte, error) {
				return BuildHeadersMsg([]byte(`[{"height":1}]`), true)
			},
			msgType: SyncMsgHeaders,
		},
		{
			name: "GetBlocks",
			build: func() ([]byte, error) {
				return BuildGetBlocksMsg([]uint64{1, 2, 3})
			},
			msgType: SyncMsgGetBlocks,
		},
		{
			name: "Blocks",
			build: func() ([]byte, error) {
				return BuildBlocksMsg([]byte(`[{"height":1}]`))
			},
			msgType: SyncMsgBlocks,
		},
		{
			name: "GetBlockLocator",
			build: func() ([]byte, error) {
				return BuildGetBlockLocatorMsg(1000)
			},
			msgType: SyncMsgGetBlockLocator,
		},
		{
			name: "BlockLocator",
			build: func() ([]byte, error) {
				return BuildBlockLocatorMsg([][]byte{{0x01}, {0x02}})
			},
			msgType: SyncMsgBlockLocator,
		},
		{
			name: "NotFound",
			build: func() ([]byte, error) {
				return BuildNotFoundMsg(SyncMsgGetHeaders, []string{"id1"})
			},
			msgType: SyncMsgNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgBytes, err := tt.build()
			if err != nil {
				t.Fatalf("build failed: %v", err)
			}

			parsedType, err := ParseSyncMessageType(msgBytes)
			if err != nil {
				t.Fatalf("ParseSyncMessageType failed: %v", err)
			}
			if parsedType != tt.msgType {
				t.Errorf("expected msgType 0x%02x, got 0x%02x", tt.msgType, parsedType)
			}
		})
	}
}

// TestSyncReactorReceiveDispatch verifies Receive dispatches to correct handlers.
func TestSyncReactorReceiveDispatch(t *testing.T) {
	handler := &mockSyncHandler{}
	r, err := NewSyncReactor(handler)
	if err != nil {
		t.Fatalf("NewSyncReactor failed: %v", err)
	}

	peerID := "peer-sync-1"

	getHeadersMsg, err := BuildGetHeadersMsg(0, 10)
	if err != nil {
		t.Fatalf("BuildGetHeadersMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelSync, peerID, getHeadersMsg)

	headersMsg, err := BuildHeadersMsg([]byte("header-data"), false)
	if err != nil {
		t.Fatalf("BuildHeadersMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelSync, peerID, headersMsg)

	getBlocksMsg, err := BuildGetBlocksMsg([]uint64{1, 2, 3})
	if err != nil {
		t.Fatalf("BuildGetBlocksMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelSync, peerID, getBlocksMsg)

	blocksMsg, err := BuildBlocksMsg([]byte(`[{"height":1}]`))
	if err != nil {
		t.Fatalf("BuildBlocksMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelSync, peerID, blocksMsg)

	getLocatorMsg, err := BuildGetBlockLocatorMsg(500)
	if err != nil {
		t.Fatalf("BuildGetBlockLocatorMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelSync, peerID, getLocatorMsg)

	locatorMsg, err := BuildBlockLocatorMsg([][]byte{{0xaa, 0xbb}})
	if err != nil {
		t.Fatalf("BuildBlockLocatorMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelSync, peerID, locatorMsg)

	notFoundMsg, err := BuildNotFoundMsg(SyncMsgGetBlocks, []string{"missing-1"})
	if err != nil {
		t.Fatalf("BuildNotFoundMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelSync, peerID, notFoundMsg)

	handler.mu.Lock()
	defer handler.mu.Unlock()

	if len(handler.getHeaders) != 1 {
		t.Errorf("expected 1 getHeaders, got %d", len(handler.getHeaders))
	}
	if len(handler.headers) != 1 {
		t.Errorf("expected 1 headers, got %d", len(handler.headers))
	}
	if len(handler.getBlocks) != 1 {
		t.Errorf("expected 1 getBlocks, got %d", len(handler.getBlocks))
	}
	if len(handler.blocks) != 1 {
		t.Errorf("expected 1 blocks, got %d", len(handler.blocks))
	}
	if len(handler.getLocators) != 1 {
		t.Errorf("expected 1 getLocator, got %d", len(handler.getLocators))
	}
	if len(handler.locators) != 1 {
		t.Errorf("expected 1 locator, got %d", len(handler.locators))
	}
	if len(handler.notFounds) != 1 {
		t.Errorf("expected 1 notFound, got %d", len(handler.notFounds))
	}
}

// TestTxReactorMessageParsing verifies all tx message types are parsed correctly.
func TestTxReactorMessageParsing(t *testing.T) {
	tests := []struct {
		name    string
		build   func() ([]byte, error)
		msgType byte
	}{
		{
			name: "TxInv",
			build: func() ([]byte, error) {
				return BuildTxInvMsg([]string{"tx1", "tx2"})
			},
			msgType: TxMsgInv,
		},
		{
			name: "TxGet",
			build: func() ([]byte, error) {
				return BuildTxGetMsg([]string{"tx3"})
			},
			msgType: TxMsgGet,
		},
		{
			name: "Tx",
			build: func() ([]byte, error) {
				txs := []core.Transaction{
					{Type: core.TxCoinbase, ChainID: 1, ToAddress: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000", Amount: 1000},
				}
				return BuildTxMsg(txs)
			},
			msgType: TxMsgTx,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgBytes, err := tt.build()
			if err != nil {
				t.Fatalf("build failed: %v", err)
			}

			parsedType, err := ParseTxMessageType(msgBytes)
			if err != nil {
				t.Fatalf("ParseTxMessageType failed: %v", err)
			}
			if parsedType != tt.msgType {
				t.Errorf("expected msgType 0x%02x, got 0x%02x", tt.msgType, parsedType)
			}
		})
	}
}

// TestTxReactorReceiveDispatch verifies Receive dispatches to correct handlers.
func TestTxReactorReceiveDispatch(t *testing.T) {
	handler := &mockTxHandler{}
	r, err := NewTxReactor(handler)
	if err != nil {
		t.Fatalf("NewTxReactor failed: %v", err)
	}

	peerID := "peer-tx-1"

	invMsg, err := BuildTxInvMsg([]string{"tx-a", "tx-b"})
	if err != nil {
		t.Fatalf("BuildTxInvMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelTx, peerID, invMsg)

	getMsg, err := BuildTxGetMsg([]string{"tx-a"})
	if err != nil {
		t.Fatalf("BuildTxGetMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelTx, peerID, getMsg)

	txs := []core.Transaction{
		{Type: core.TxCoinbase, ChainID: 1, ToAddress: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000", Amount: 5000},
	}
	txMsg, err := BuildTxMsg(txs)
	if err != nil {
		t.Fatalf("BuildTxMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelTx, peerID, txMsg)

	handler.mu.Lock()
	defer handler.mu.Unlock()

	if len(handler.invTxs) != 1 {
		t.Errorf("expected 1 invTx, got %d", len(handler.invTxs))
	}
	if len(handler.getTxs) != 1 {
		t.Errorf("expected 1 getTx, got %d", len(handler.getTxs))
	}
	if len(handler.txs) != 1 {
		t.Errorf("expected 1 tx, got %d", len(handler.txs))
	} else {
		if len(handler.txs[0].Txs) != 1 {
			t.Errorf("expected 1 transaction in payload, got %d", len(handler.txs[0].Txs))
		}
	}
}

// TestBlockReactorMessageParsing verifies all block message types are parsed correctly.
func TestBlockReactorMessageParsing(t *testing.T) {
	tests := []struct {
		name    string
		build   func() ([]byte, error)
		msgType byte
	}{
		{
			name: "BlockInv",
			build: func() ([]byte, error) {
				return BuildBlockInvMsg([]string{"blk1", "blk2"})
			},
			msgType: BlockMsgInv,
		},
		{
			name: "BlockGet",
			build: func() ([]byte, error) {
				return BuildBlockGetMsg([]string{"blk1"})
			},
			msgType: BlockMsgGet,
		},
		{
			name: "Block",
			build: func() ([]byte, error) {
				blocks := []*core.Block{
					{Height: 1, MinerAddress: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000"},
				}
				return BuildBlockMsg(blocks)
			},
			msgType: BlockMsgBlock,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgBytes, err := tt.build()
			if err != nil {
				t.Fatalf("build failed: %v", err)
			}

			parsedType, err := ParseBlockMessageType(msgBytes)
			if err != nil {
				t.Fatalf("ParseBlockMessageType failed: %v", err)
			}
			if parsedType != tt.msgType {
				t.Errorf("expected msgType 0x%02x, got 0x%02x", tt.msgType, parsedType)
			}
		})
	}
}

// TestBlockReactorReceiveDispatch verifies Receive dispatches to correct handlers.
func TestBlockReactorReceiveDispatch(t *testing.T) {
	handler := &mockBlockHandler{}
	r, err := NewBlockReactor(handler)
	if err != nil {
		t.Fatalf("NewBlockReactor failed: %v", err)
	}

	peerID := "peer-block-1"

	invMsg, err := BuildBlockInvMsg([]string{"aabbccdd", "11223344"})
	if err != nil {
		t.Fatalf("BuildBlockInvMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelBlock, peerID, invMsg)

	getMsg, err := BuildBlockGetMsg([]string{"aabbccdd"})
	if err != nil {
		t.Fatalf("BuildBlockGetMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelBlock, peerID, getMsg)

	blocks := []*core.Block{
		{Height: 10, MinerAddress: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000"},
		{Height: 11, MinerAddress: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000"},
	}
	blockMsg, err := BuildBlockMsg(blocks)
	if err != nil {
		t.Fatalf("BuildBlockMsg failed: %v", err)
	}
	r.Receive(mconnection.ChannelBlock, peerID, blockMsg)

	handler.mu.Lock()
	defer handler.mu.Unlock()

	if len(handler.invBlks) != 1 {
		t.Errorf("expected 1 invBlock, got %d", len(handler.invBlks))
	}
	if len(handler.getBlks) != 1 {
		t.Errorf("expected 1 getBlock, got %d", len(handler.getBlks))
	}
	if len(handler.blks) != 1 {
		t.Errorf("expected 1 block, got %d", len(handler.blks))
	} else {
		if len(handler.blks[0].Blocks) != 2 {
			t.Errorf("expected 2 blocks in payload, got %d", len(handler.blks[0].Blocks))
		}
	}
}

// TestReceiveWithEmptyPeerID verifies Receive ignores empty peerID.
func TestReceiveWithEmptyPeerID(t *testing.T) {
	syncHandler := &mockSyncHandler{}
	syncR, err := NewSyncReactor(syncHandler)
	if err != nil {
		t.Fatalf("NewSyncReactor failed: %v", err)
	}

	msg, err := BuildGetHeadersMsg(0, 10)
	if err != nil {
		t.Fatalf("BuildGetHeadersMsg failed: %v", err)
	}

	syncR.Receive(mconnection.ChannelSync, "", msg)

	syncHandler.mu.Lock()
	if len(syncHandler.getHeaders) != 0 {
		t.Error("Receive should ignore empty peerID")
	}
	syncHandler.mu.Unlock()
}

// TestReceiveWithTooShortMessage verifies Receive ignores too-short messages.
func TestReceiveWithTooShortMessage(t *testing.T) {
	syncHandler := &mockSyncHandler{}
	syncR, err := NewSyncReactor(syncHandler)
	if err != nil {
		t.Fatalf("NewSyncReactor failed: %v", err)
	}

	syncR.Receive(mconnection.ChannelSync, "peer", []byte{})
	syncR.Receive(mconnection.ChannelSync, "peer", []byte{0x01})

	txHandler := &mockTxHandler{}
	txR, err := NewTxReactor(txHandler)
	if err != nil {
		t.Fatalf("NewTxReactor failed: %v", err)
	}
	txR.Receive(mconnection.ChannelTx, "peer", []byte{})

	blockHandler := &mockBlockHandler{}
	blockR, err := NewBlockReactor(blockHandler)
	if err != nil {
		t.Fatalf("NewBlockReactor failed: %v", err)
	}
	blockR.Receive(mconnection.ChannelBlock, "peer", []byte{})
}

// TestReceiveWithUnknownMessageType verifies unknown message types are silently ignored.
func TestReceiveWithUnknownMessageType(t *testing.T) {
	syncHandler := &mockSyncHandler{}
	syncR, err := NewSyncReactor(syncHandler)
	if err != nil {
		t.Fatalf("NewSyncReactor failed: %v", err)
	}

	syncR.Receive(mconnection.ChannelSync, "peer", []byte{0xFE, 0x00})
	syncR.Receive(mconnection.ChannelSync, "peer", []byte{0xFD, 0x00, 0x00})

	txHandler := &mockTxHandler{}
	txR, err := NewTxReactor(txHandler)
	if err != nil {
		t.Fatalf("NewTxReactor failed: %v", err)
	}
	txR.Receive(mconnection.ChannelTx, "peer", []byte{0xFF})

	blockHandler := &mockBlockHandler{}
	blockR, err := NewBlockReactor(blockHandler)
	if err != nil {
		t.Fatalf("NewBlockReactor failed: %v", err)
	}
	blockR.Receive(mconnection.ChannelBlock, "peer", []byte{0x99})
}

// TestReceiveWithMalformedPayload verifies malformed JSON is handled gracefully.
func TestReceiveWithMalformedPayload(t *testing.T) {
	syncHandler := &mockSyncHandler{}
	syncR, err := NewSyncReactor(syncHandler)
	if err != nil {
		t.Fatalf("NewSyncReactor failed: %v", err)
	}

	syncR.Receive(mconnection.ChannelSync, "peer", []byte{SyncMsgGetHeaders, '{', 'i', 'n', 'v', 'a', 'l', 'i', 'd'})

	syncHandler.mu.Lock()
	if len(syncHandler.getHeaders) != 0 {
		t.Error("Receive should ignore malformed payload")
	}
	syncHandler.mu.Unlock()

	txHandler := &mockTxHandler{}
	txR, err := NewTxReactor(txHandler)
	if err != nil {
		t.Fatalf("NewTxReactor failed: %v", err)
	}
	txR.Receive(mconnection.ChannelTx, "peer", []byte{TxMsgInv, 'b', 'a', 'd'})

	txHandler.mu.Lock()
	if len(txHandler.invTxs) != 0 {
		t.Error("Receive should ignore malformed tx payload")
	}
	txHandler.mu.Unlock()
}

// TestNewReactorWithNilHandler verifies nil handler returns error.
func TestNewReactorWithNilHandler(t *testing.T) {
	_, err := NewSyncReactor(nil)
	if err == nil {
		t.Error("NewSyncReactor(nil) should return error")
	}

	_, err = NewTxReactor(nil)
	if err == nil {
		t.Error("NewTxReactor(nil) should return error")
	}

	_, err = NewBlockReactor(nil)
	if err == nil {
		t.Error("NewBlockReactor(nil) should return error")
	}
}

// TestSetHandlerWithNil verifies SetHandler rejects nil.
func TestSetHandlerWithNil(t *testing.T) {
	syncR, err := NewSyncReactor(&mockSyncHandler{})
	if err != nil {
		t.Fatalf("NewSyncReactor failed: %v", err)
	}
	if err := syncR.SetHandler(nil); err == nil {
		t.Error("SetHandler(nil) should return error for SyncReactor")
	}

	txR, err := NewTxReactor(&mockTxHandler{})
	if err != nil {
		t.Fatalf("NewTxReactor failed: %v", err)
	}
	if err := txR.SetHandler(nil); err == nil {
		t.Error("SetHandler(nil) should return error for TxReactor")
	}

	blockR, err := NewBlockReactor(&mockBlockHandler{})
	if err != nil {
		t.Fatalf("NewBlockReactor failed: %v", err)
	}
	if err := blockR.SetHandler(nil); err == nil {
		t.Error("SetHandler(nil) should return error for BlockReactor")
	}
}

// TestParseSyncMessageTypeErrors verifies ParseSyncMessageType returns error for short messages.
func TestParseSyncMessageTypeErrors(t *testing.T) {
	_, err := ParseSyncMessageType([]byte{})
	if err == nil {
		t.Error("ParseSyncMessageType should error on empty message")
	}
}

// TestParseTxMessageTypeErrors verifies ParseTxMessageType returns error for short messages.
func TestParseTxMessageTypeErrors(t *testing.T) {
	_, err := ParseTxMessageType([]byte{})
	if err == nil {
		t.Error("ParseTxMessageType should error on empty message")
	}
}

// TestParseBlockMessageTypeErrors verifies ParseBlockMessageType returns error for short messages.
func TestParseBlockMessageTypeErrors(t *testing.T) {
	_, err := ParseBlockMessageType([]byte{})
	if err == nil {
		t.Error("ParseBlockMessageType should error on empty message")
	}
}

// TestDecodeUint64FromBytes verifies binary uint64 encoding/decoding.
func TestDecodeUint64FromBytes(t *testing.T) {
	val := uint64(12345678901234567890)
	b := EncodeUint64ToBytes(val)

	decoded, err := DecodeUint64FromBytes(b)
	if err != nil {
		t.Fatalf("DecodeUint64FromBytes failed: %v", err)
	}
	if decoded != val {
		t.Errorf("expected %d, got %d", val, decoded)
	}

	_, err = DecodeUint64FromBytes([]byte{0x01, 0x02})
	if err == nil {
		t.Error("DecodeUint64FromBytes should error on short input")
	}
}

// TestTxReactorHandlesMalformedTxInBatch verifies malformed transactions
// in a batch are skipped while valid ones are processed.
func TestTxReactorHandlesMalformedTxInBatch(t *testing.T) {
	handler := &mockTxHandler{}
	r, err := NewTxReactor(handler)
	if err != nil {
		t.Fatalf("NewTxReactor failed: %v", err)
	}

	txs := []core.Transaction{
		{Type: core.TxCoinbase, ChainID: 1, ToAddress: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000", Amount: 1000},
		{Type: core.TxCoinbase, ChainID: 2, ToAddress: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000", Amount: 2000},
	}

	validTxs := make([]json.RawMessage, 0, len(txs))
	for _, tx := range txs {
		raw, err := json.Marshal(tx)
		if err != nil {
			t.Fatalf("marshal tx failed: %v", err)
		}
		validTxs = append(validTxs, raw)
	}

	txArrayPayload := []byte(`{"txs":[`)
	for i, raw := range validTxs {
		if i > 0 {
			txArrayPayload = append(txArrayPayload, ',')
		}
		txArrayPayload = append(txArrayPayload, raw...)
	}
	txArrayPayload = append(txArrayPayload, `,"not_an_object"]}`...)

	msg := make([]byte, 1+len(txArrayPayload))
	msg[0] = TxMsgTx
	copy(msg[1:], txArrayPayload)

	r.Receive(mconnection.ChannelTx, "peer-1", msg)

	handler.mu.Lock()
	defer handler.mu.Unlock()

	if len(handler.txs) != 1 {
		t.Fatalf("expected 1 tx call, got %d", len(handler.txs))
	}
	if len(handler.txs[0].Txs) != 2 {
		t.Errorf("expected 2 valid transactions, got %d (malformed one should be skipped)", len(handler.txs[0].Txs))
	}
}

// TestBlockReactorHandlesMalformedBlockInBatch verifies malformed blocks
// in a batch are skipped while valid ones are processed.
func TestBlockReactorHandlesMalformedBlockInBatch(t *testing.T) {
	handler := &mockBlockHandler{}
	r, err := NewBlockReactor(handler)
	if err != nil {
		t.Fatalf("NewBlockReactor failed: %v", err)
	}

	blocks := []*core.Block{
		{Height: 1, MinerAddress: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000"},
		{Height: 2, MinerAddress: "NOGO" + "00000000000000000000000000000000000000000000000000000000000000000000"},
	}

	validBlocks := make([]json.RawMessage, 0, len(blocks))
	for _, block := range blocks {
		raw, err := json.Marshal(block)
		if err != nil {
			t.Fatalf("marshal block failed: %v", err)
		}
		validBlocks = append(validBlocks, raw)
	}

	blockArrayPayload := []byte(`{"blocks":[`)
	for i, raw := range validBlocks {
		if i > 0 {
			blockArrayPayload = append(blockArrayPayload, ',')
		}
		blockArrayPayload = append(blockArrayPayload, raw...)
	}
	blockArrayPayload = append(blockArrayPayload, `,"not_an_object"]}`...)

	msg := make([]byte, 1+len(blockArrayPayload))
	msg[0] = BlockMsgBlock
	copy(msg[1:], blockArrayPayload)

	r.Receive(mconnection.ChannelBlock, "peer-1", msg)

	handler.mu.Lock()
	defer handler.mu.Unlock()

	if len(handler.blks) != 1 {
		t.Fatalf("expected 1 block call, got %d", len(handler.blks))
	}
	if len(handler.blks[0].Blocks) != 2 {
		t.Errorf("expected 2 valid blocks, got %d (malformed one should be skipped)", len(handler.blks[0].Blocks))
	}
}

// TestBroadcastFunctionality verifies switch broadcast records messages.
func TestBroadcastFunctionality(t *testing.T) {
	sw := newMockSwitch()

	syncHandler := &mockSyncHandler{}
	syncR, err := NewSyncReactor(syncHandler)
	if err != nil {
		t.Fatalf("NewSyncReactor failed: %v", err)
	}

	sw.AddReactor("sync", syncR)

	msg, err := BuildGetHeadersMsg(0, 10)
	if err != nil {
		t.Fatalf("BuildGetHeadersMsg failed: %v", err)
	}

	sw.Broadcast(mconnection.ChannelSync, msg)

	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(sw.broadcasts))
	}
	if sw.broadcasts[0].ChID != mconnection.ChannelSync {
		t.Errorf("expected broadcast on channel 0x%02x, got 0x%02x", mconnection.ChannelSync, sw.broadcasts[0].ChID)
	}
}

// TestSendFunctionality verifies switch send records messages.
func TestSendFunctionality(t *testing.T) {
	sw := newMockSwitch()

	txHandler := &mockTxHandler{}
	txR, err := NewTxReactor(txHandler)
	if err != nil {
		t.Fatalf("NewTxReactor failed: %v", err)
	}

	sw.AddReactor("tx", txR)

	msg, err := BuildTxInvMsg([]string{"tx1"})
	if err != nil {
		t.Fatalf("BuildTxInvMsg failed: %v", err)
	}

	success := sw.Send("peer-1", mconnection.ChannelTx, msg)
	if !success {
		t.Error("Send should return true for mockSwitch")
	}

	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.sends) != 1 {
		t.Fatalf("expected 1 send, got %d", len(sw.sends))
	}
	if sw.sends[0].PeerID != "peer-1" {
		t.Errorf("expected send to peer-1, got %s", sw.sends[0].PeerID)
	}
	if sw.sends[0].ChID != mconnection.ChannelTx {
		t.Errorf("expected send on channel 0x%02x, got 0x%02x", mconnection.ChannelTx, sw.sends[0].ChID)
	}
}

// TestSyncReactorChannelDescriptors verifies correct channel configuration.
func TestSyncReactorChannelDescriptors(t *testing.T) {
	r, err := NewSyncReactor(&mockSyncHandler{})
	if err != nil {
		t.Fatalf("NewSyncReactor failed: %v", err)
	}

	chs := r.GetChannels()
	if len(chs) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(chs))
	}

	desc := chs[0]
	if desc.ID != mconnection.ChannelSync {
		t.Errorf("expected ID 0x%02x, got 0x%02x", mconnection.ChannelSync, desc.ID)
	}
	if desc.Priority != 5 {
		t.Errorf("expected priority 5, got %d", desc.Priority)
	}
	if desc.SendQueueCapacity != 256 {
		t.Errorf("expected send queue capacity 256, got %d", desc.SendQueueCapacity)
	}
}

// TestTxReactorChannelDescriptors verifies correct channel configuration.
func TestTxReactorChannelDescriptors(t *testing.T) {
	r, err := NewTxReactor(&mockTxHandler{})
	if err != nil {
		t.Fatalf("NewTxReactor failed: %v", err)
	}

	chs := r.GetChannels()
	if len(chs) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(chs))
	}

	desc := chs[0]
	if desc.ID != mconnection.ChannelTx {
		t.Errorf("expected ID 0x%02x, got 0x%02x", mconnection.ChannelTx, desc.ID)
	}
	if desc.Priority != 3 {
		t.Errorf("expected priority 3, got %d", desc.Priority)
	}
}

// TestBlockReactorChannelDescriptors verifies correct channel configuration.
func TestBlockReactorChannelDescriptors(t *testing.T) {
	r, err := NewBlockReactor(&mockBlockHandler{})
	if err != nil {
		t.Fatalf("NewBlockReactor failed: %v", err)
	}

	chs := r.GetChannels()
	if len(chs) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(chs))
	}

	desc := chs[0]
	if desc.ID != mconnection.ChannelBlock {
		t.Errorf("expected ID 0x%02x, got 0x%02x", mconnection.ChannelBlock, desc.ID)
	}
	if desc.Priority != 5 {
		t.Errorf("expected priority 5, got %d", desc.Priority)
	}
}

// TestReactorImplementsInterface verifies reactors implement the Reactor interface.
func TestReactorImplementsInterface(t *testing.T) {
	var _ Reactor = &SyncReactor{}
	var _ Reactor = &TxReactor{}
	var _ Reactor = &BlockReactor{}
	var _ Reactor = &BaseReactor{}
}

// TestGetSwitchAfterSetSwitch verifies GetSwitch returns the switch after SetSwitch.
func TestGetSwitchAfterSetSwitch(t *testing.T) {
	br := &BaseReactor{}
	sw := newMockSwitch()

	br.SetSwitch(sw)
	if br.GetSwitch() != sw {
		t.Error("GetSwitch should return the switch set by SetSwitch")
	}
}
