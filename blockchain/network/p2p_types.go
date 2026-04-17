package network

import "github.com/nogochain/nogo/blockchain/core"

// Bitcoin-style P2P message types
// All messages use content-based matching (hash, nonce) instead of request IDs
// This allows concurrent bidirectional communication without protocol conflicts

// Inventory types (similar to Bitcoin's INV vector)
type InventoryType uint32

const (
	InvTypeError  InventoryType = 0
	InvTypeTx    InventoryType = 1
	InvTypeBlock InventoryType = 2
)

// Inventory vector entry (similar to Bitcoin's CInv)
type InventoryEntry struct {
	Type InventoryType `json:"type"`
	Hash string      `json:"hash"`
}

// INV message announces available data (Bitcoin-style)
type p2pInvMsg struct {
	Entries []InventoryEntry `json:"entries"`
}

// GETDATA message requests data from peer (Bitcoin-style)
type p2pGetDataMsg struct {
	Entries []InventoryEntry `json:"entries"`
}

// Chain info request and response
type p2pChainInfoReq struct{}

type p2pChainInfo struct {
	ChainID              uint64 `json:"chainId"`
	RulesHash            string `json:"rulesHash"`
	Height               uint64 `json:"height"`
	LatestHash           string `json:"latestHash"`
	GenesisHash          string `json:"genesisHash"`
	GenesisTimestampUnix  int64  `json:"genesisTimestampUnix"`
	PeersCount           int    `json:"peersCount"`
	Work                 string `json:"work"`
}

// Headers request and response
type p2pHeadersFromReq struct {
	From  uint64 `json:"from"`
	Count int    `json:"count"`
}

type p2pHeaders struct {
	Headers []core.BlockHeader `json:"headers"`
}

// Transaction messages
type p2pTxMsg struct {
	Tx core.Transaction `json:"tx"`
}

// Block messages (direct block data)
type p2pBlockMsg struct {
	Block *core.Block `json:"block"`
}

// Address messages
type p2pAddrEntry struct {
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	Timestamp int64  `json:"timestamp"`
}

type p2pAddrMsg struct {
	Addresses []p2pAddrEntry `json:"addresses"`
}

// Ping/Pong with nonce (Bitcoin-style random nonce for concurrent ping matching)
type p2pPingMsg struct {
	Nonce uint64 `json:"nonce"`
}

type p2pPongMsg struct {
	Nonce uint64 `json:"nonce"`
}

// Legacy types for backward compatibility
type p2pPing struct {
	Timestamp int64 `json:"timestamp"`
}

type p2pPong struct {
	Timestamp int64 `json:"timestamp"`
}

// Authentication types (reserved for future use)
type p2pAuthChallenge struct {
	Challenge string `json:"challenge"`
	NodeID    string `json:"nodeId"`
}

type p2pAuthResponse struct {
	Response string `json:"response"`
	NodeID   string `json:"nodeId"`
	PubKey   string `json:"pubKey"`
}

// Not found message (when requested data is unavailable)
type p2pNotFoundMsg struct {
	Entries []InventoryEntry `json:"entries"`
}

// Legacy request types (for backward compatibility, will be deprecated)
type p2pBlockByHashReq struct {
	HashHex string `json:"hashHex"`
}

type p2pBlockByHeightReq struct {
	Height uint64 `json:"height"`
}

type p2pBlocksByRangeReq struct {
	StartHeight uint64 `json:"startHeight"`
	Count       uint64 `json:"count"`
}

type p2pTransactionReq struct {
	TxHex string `json:"txHex"`
}

type p2pTransactionBroadcast struct {
	TxHex string `json:"txHex"`
}

type p2pBlockBroadcast struct {
	BlockHex string `json:"blockHex"`
}

type p2pBlockReq struct {
	HashHex string `json:"hashHex"`
}
