package network

type p2pChainInfoReq struct{}

type p2pHeadersFromReq struct {
	From  uint64 `json:"from"`
	Count int    `json:"count"`
}

type p2pBlockByHashReq struct {
	HashHex string `json:"hashHex"`
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
