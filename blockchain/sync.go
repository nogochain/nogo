package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

type SyncLoop struct {
	pm PeerAPI
	bc *Blockchain
	miner *Miner // Reference to miner for pause/resume during sync

	interval time.Duration
	window   uint64
}

func NewSyncLoop(pm PeerAPI, bc *Blockchain, interval time.Duration) *SyncLoop {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	return &SyncLoop{
		pm:       pm,
		bc:       bc,
		miner:    nil, // Will be set later via SetMiner method
		interval: interval,
		window:   200,
	}
}

// SetMiner sets the miner reference for pause/resume during sync
func (s *SyncLoop) SetMiner(miner *Miner) {
	s.miner = miner
}

func (s *SyncLoop) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.SyncOnce(ctx)
		}
	}
}

func (s *SyncLoop) SyncOnce(ctx context.Context) {
	if s.pm == nil {
		return
	}
	
	// Check if miner is currently verifying a block
	// If so, skip this sync round to avoid conflicts
	if s.miner != nil && s.miner.IsVerifying() {
		log.Printf("sync: skipping sync round, miner is verifying block")
		return
	}
	
	localHeight := s.bc.LatestBlock().Height
	localRulesHash := s.bc.RulesHashHex()
	localGenesisHash := ""
	if genesis, ok := s.bc.BlockByHeight(0); ok {
		localGenesisHash = fmt.Sprintf("%x", genesis.Hash)
	}
	strictIdentity := envBool("STRICT_PEER_IDENTITY", true)

	peers := s.pm.Peers()
	log.Printf("sync: starting sync round, localHeight=%d, peers=%d", localHeight, len(peers))

	for _, peer := range peers {
		log.Printf("sync: checking peer %s", peer)
		info, err := s.pm.FetchChainInfo(ctx, peer)
		if err != nil {
			log.Printf("sync: failed to fetch chain info from %s: %v", peer, err)
			continue
		}
		log.Printf("sync: peer %s chain info: height=%d, chainId=%d, rulesHash=%s, genesisHash=%s", peer, info.Height, info.ChainID, info.RulesHash, info.GenesisHash)
		if info.ChainID != s.bc.ChainID {
			log.Printf("sync: peer %s chainId mismatch: local=%d, peer=%d", peer, s.bc.ChainID, info.ChainID)
			continue
		}
		if strictIdentity && (info.RulesHash == "" || info.GenesisHash == "") {
			log.Printf("sync: peer %s missing rulesHash or genesisHash (strict mode)", peer)
			continue
		}
		if info.RulesHash != "" && localRulesHash != "" && info.RulesHash != localRulesHash {
			log.Printf("sync: peer %s rulesHash mismatch: local=%s, peer=%s", peer, localRulesHash, info.RulesHash)
			continue
		}
		if info.GenesisHash != "" && localGenesisHash != "" && info.GenesisHash != localGenesisHash {
			log.Printf("sync: peer %s genesisHash mismatch: local=%s, peer=%s", peer, localGenesisHash, info.GenesisHash)
			continue
		}
		if info.Height <= localHeight {
			log.Printf("sync: peer %s height not ahead: local=%d, peer=%d", peer, localHeight, info.Height)
			continue
		}

		var from uint64
		var limit int
		
		// Always start from our current height + 1
		from = localHeight + 1
		
		// Limit the number of headers to fetch in one round
		limit = int(s.window)
		if info.Height-from+1 < uint64(limit) {
			limit = int(info.Height - from + 1)
		}
		
		log.Printf("sync: fetching headers from=%d limit=%d (local=%d, peer=%d)", from, limit, localHeight, info.Height)

		// Fetch headers
		headers, err := s.pm.FetchHeadersFrom(ctx, peer, from, limit)
		if err != nil {
			log.Printf("sync: failed to fetch headers: %v", err)
			continue
		}
		log.Printf("sync: fetched %d headers", len(headers))

		// Fetch and add blocks sequentially with full validation
		for _, h := range headers {
			if _, ok := s.bc.BlockByHash(h.HashHex); ok {
				continue
			}
			b, err := s.pm.FetchBlockByHash(ctx, peer, h.HashHex)
			if err != nil {
				log.Printf("sync: failed to fetch block %d: %v", h.Height, err)
				break
			}
			_, err = s.bc.AddBlock(b)
			if err != nil {
				log.Printf("sync: failed to add block %d: %v", h.Height, err)
				// Try to fetch missing ancestors
				if errors.Is(err, ErrUnknownParent) {
					if ferr := s.pm.EnsureAncestors(ctx, s.bc, h.PrevHashHex); ferr != nil {
						log.Printf("sync: failed to fetch ancestors: %v", ferr)
						break
					}
					// Retry adding the block after fetching ancestors
					_, err = s.bc.AddBlock(b)
					if err != nil {
						log.Printf("sync: still failed to add block %d after fetching ancestors: %v", h.Height, err)
						break
					}
				} else {
					break
				}
			}
		}
		// Log current height after sync round
		if latest := s.bc.LatestBlock(); latest != nil {
			log.Printf("sync: sync round completed, height=%d", latest.Height)
		}
	}

	s.discoverPeers(ctx)
}

func (s *SyncLoop) discoverPeers(ctx context.Context) {
	if s.pm == nil {
		return
	}
	currentPeers := s.pm.Peers()
	if len(currentPeers) >= 10 {
		return
	}
	for _, peer := range currentPeers {
		go func(p string) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, p+"/p2p/getaddr", nil)
			if err != nil {
				log.Printf("peer discovery: failed to create request for %s: %v", p, err)
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("peer discovery: failed to fetch addresses from %s: %v", p, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				log.Printf("peer discovery: non-OK status from %s: %d", p, resp.StatusCode)
				return
			}
			var result struct {
				Addresses []struct {
					IP   string `json:"ip"`
					Port int    `json:"port"`
				} `json:"addresses"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				log.Printf("peer discovery: failed to decode response from %s: %v", p, err)
				return
			}
			for _, a := range result.Addresses {
				addr := fmt.Sprintf("%s:%d", a.IP, a.Port)
				if addr != "" && addr != ":" {
					s.pm.AddPeer(addr)
				}
			}
		}(peer)
	}
}
