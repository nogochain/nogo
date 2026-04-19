package network

import (
	"context"
	"math/big"

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
	FetchBlockByHeight(ctx context.Context, peer string, height uint64) (*core.Block, error)
	FetchAnyBlockByHash(ctx context.Context, hashHex string) (*core.Block, string, error)
	// FetchBlocksByHeightRange fetches multiple blocks in a single TCP connection.
	// This is more efficient than calling FetchBlockByHeight multiple times.
	FetchBlocksByHeightRange(ctx context.Context, peer string, startHeight, count uint64) ([]*core.Block, error)

	BroadcastTransaction(ctx context.Context, tx core.Transaction, hops int)
	BroadcastBlock(ctx context.Context, block *core.Block) error
	BroadcastNewStatus(ctx context.Context, height uint64, work *big.Int, latestHash string)
	EnsureAncestors(ctx context.Context, bc BlockchainInterface, missingHashHex string) error
}
