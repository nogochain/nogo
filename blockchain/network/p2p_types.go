package network

type p2pChainInfoReq struct{}

type p2pHeadersFromReq struct {
	From  uint64 `json:"from"`
	Count int    `json:"count"`
}

type p2pBlockByHashReq struct {
	HashHex string `json:"hashHex"`
}

type p2pBlockByHeightReq struct {
	Height uint64 `json:"height"`
}

// p2pBlocksByRangeReq requests multiple blocks by height range
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

// p2pPing represents a ping message for connection keepalive
type p2pPing struct {
	Timestamp int64 `json:"timestamp"`
}

// p2pPong represents a pong response to a ping
type p2pPong struct {
	Timestamp int64 `json:"timestamp"`
}

// p2pAuthChallenge reserved for future use //nolint:unused
type p2pAuthChallenge struct {
	Challenge string `json:"challenge"`
	NodeID    string `json:"nodeId"`
}

type p2pAuthResponse struct {
	Response string `json:"response"`
	NodeID   string `json:"nodeId"`
	PubKey   string `json:"pubKey"`
}
