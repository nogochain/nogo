// Copyright 2026 NogoChain Team
//
// Peer-assisted fork resolution: locate the last common height with a heavier
// peer by comparing block hashes, then roll back and apply the peer's canonical
// segment. Aligns with Bytom-style blockKeeper fork handling (height walk +
// batched block fetch) while reusing NogoChain batch sync primitives.

package network

import (
	"context"
	"encoding/hex"
	"log"

	"github.com/nogochain/nogo/blockchain/network/forkresolution"
)

// peerChainMetaFetcher is implemented by Switch to return extended ChainInfo
// including the tip parent hash for accurate synthetic fork headers.
type peerChainMetaFetcher interface {
	FetchPeerChainMeta(ctx context.Context, peerID string) (*ChainInfo, error)
}

// reorgChainToHeaviestPeer switches to the peer's chain when the peer reports
// strictly greater cumulative work. It finds the highest shared block height,
// rolls back to that height, then pulls and applies blocks from the peer in batches.
func (bk *blockKeeper) reorgChainToHeaviestPeer(peer PeerInterface) {
	if bk == nil || peer == nil || bk.chain == nil {
		return
	}

	peerID := peer.ID()
	origPeer := bk.syncPeer
	bk.syncPeer = peer
	defer func() { bk.syncPeer = origPeer }()

	localTip := bk.chain.LatestBlock()
	if localTip == nil {
		return
	}

	peerHeight, peerWork, _, err := bk.peers.GetPeerChainInfo(peerID)
	if err != nil || peerWork == nil {
		log.Printf("[BlockKeeper] reorgChainToHeaviestPeer: cannot read peer chain info for %s: %v", peerID, err)
		return
	}

	localWork := bk.chain.CanonicalWork()
	if localWork != nil && peerWork.Cmp(localWork) <= 0 {
		log.Printf("[BlockKeeper] reorgChainToHeaviestPeer: peer %s not heavier than local, skip", peerID)
		return
	}

	hMax := localTip.GetHeight()
	if peerHeight < hMax {
		hMax = peerHeight
	}

	var shared uint64
	found := false
	for h := hMax; ; {
		lb, ok := bk.chain.BlockByHeight(h)
		if !ok || lb == nil {
			break
		}
		rb, rerr := bk.requireBlock(h)
		if rerr != nil {
			log.Printf("[BlockKeeper] reorgChainToHeaviestPeer: peer block h=%d: %v", h, rerr)
			break
		}
		if hex.EncodeToString(lb.Hash) == hex.EncodeToString(rb.Hash) {
			shared = h
			found = true
			break
		}
		if h == 0 {
			break
		}
		h--
	}

	if !found {
		log.Printf("[BlockKeeper] reorgChainToHeaviestPeer: no shared height with %s, falling back to progressive rollback", peerID)
		bk.handleChainMismatchInSync(peer)
		return
	}

	if peerHeight > shared+uint64(forkresolution.MaxReorgDepth) {
		log.Printf("[BlockKeeper] reorgChainToHeaviestPeer: gap %d exceeds MaxReorgDepth=%d, rolling back to shared then batch sync",
			peerHeight-shared, forkresolution.MaxReorgDepth)
	}

	if err := bk.chain.RollbackToHeight(shared); err != nil {
		log.Printf("[BlockKeeper] reorgChainToHeaviestPeer: RollbackToHeight(%d): %v", shared, err)
		return
	}
	bk.syncSessionSeq++

	for start := shared + 1; start <= peerHeight; {
		remain := peerHeight - start + 1
		batch := uint64(maxBlockPerMsg)
		if remain < batch {
			batch = remain
		}
		blocks, berr := bk.requireBlocksBatch(start, batch)
		if berr != nil {
			log.Printf("[BlockKeeper] reorgChainToHeaviestPeer: batch from h=%d: %v", start, berr)
			bk.TriggerImmediateReSync()
			return
		}
		for h := start; h < start+batch; h++ {
			b, ok := blocks[h]
			if !ok || b == nil {
				log.Printf("[BlockKeeper] reorgChainToHeaviestPeer: missing block at height %d after batch fetch", h)
				bk.TriggerImmediateReSync()
				return
			}
			if _, aerr := bk.chain.AddBlock(b); aerr != nil {
				log.Printf("[BlockKeeper] reorgChainToHeaviestPeer: AddBlock h=%d: %v", h, aerr)
				bk.TriggerImmediateReSync()
				return
			}
		}
		start += batch
	}

	if tip := bk.chain.LatestBlock(); tip != nil {
		bk.recordSyncProgress(tip.GetHeight())
	}
	bk.TriggerImmediateReSync()
	log.Printf("[BlockKeeper] reorgChainToHeaviestPeer: completed switch via peer %s to height %d", peerID, peerHeight)
}
