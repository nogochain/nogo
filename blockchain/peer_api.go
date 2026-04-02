package main

import "context"

// PeerAPI abstracts the transport used for peer-to-peer sync/gossip.
// Implementations must provide deterministic data for the same underlying chain.
type PeerAPI interface {
	Peers() []string
	AddPeer(addr string)
	GetActivePeers() []string

	FetchChainInfo(ctx context.Context, peer string) (*chainInfo, error)
	FetchHeadersFrom(ctx context.Context, peer string, fromHeight uint64, count int) ([]BlockHeader, error)
	FetchBlockByHash(ctx context.Context, peer, hashHex string) (*Block, error)
	FetchAnyBlockByHash(ctx context.Context, hashHex string) (*Block, string, error)

	BroadcastTransaction(ctx context.Context, tx Transaction, hops int)
	EnsureAncestors(ctx context.Context, bc *Blockchain, missingHashHex string) error
}
