package network

import (
	"context"

	"github.com/nogochain/nogo/blockchain/core"
)

// PeerAPI abstracts the transport used for peer-to-peer sync/gossip.
type PeerAPI interface {
	Peers() []string
	AddPeer(addr string)
	GetActivePeers() []string

	FetchChainInfo(ctx context.Context, peer string) (*ChainInfo, error)
	FetchHeadersFrom(ctx context.Context, peer string, fromHeight uint64, count int) ([]core.BlockHeader, error)
	FetchBlockByHash(ctx context.Context, peer, hashHex string) (*core.Block, error)
	FetchAnyBlockByHash(ctx context.Context, hashHex string) (*core.Block, string, error)

	BroadcastTransaction(ctx context.Context, tx core.Transaction, hops int)
	EnsureAncestors(ctx context.Context, bc BlockchainInterface, missingHashHex string) error
}
