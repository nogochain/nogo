package main

import (
	"sync"
	"time"
)

type PeerScore struct {
	Peer           string
	Score          float64
	SuccessCount   int
	FailureCount   int
	TotalLatencyMs int64
	LastSeen       time.Time
	FirstSeen      time.Time
}

type PeerScorer struct {
	mu       sync.RWMutex
	peers    map[string]*PeerScore
	maxPeers int
}

func NewPeerScorer(maxPeers int) *PeerScorer {
	if maxPeers <= 0 {
		maxPeers = 100
	}
	return &PeerScorer{
		peers:    make(map[string]*PeerScore),
		maxPeers: maxPeers,
	}
}

func (ps *PeerScorer) RecordSuccess(peer string, latencyMs int64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	if p, ok := ps.peers[peer]; ok {
		p.SuccessCount++
		p.TotalLatencyMs += latencyMs
		p.LastSeen = now
		p.Score = ps.calculateScore(p)
	} else {
		ps.peers[peer] = &PeerScore{
			Peer:           peer,
			Score:          50.0,
			SuccessCount:   1,
			FailureCount:   0,
			TotalLatencyMs: latencyMs,
			LastSeen:       now,
			FirstSeen:      now,
		}
		ps.evictIfNeeded()
	}
}

func (ps *PeerScorer) RecordFailure(peer string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	if p, ok := ps.peers[peer]; ok {
		p.FailureCount++
		p.LastSeen = now
		p.Score = ps.calculateScore(p)
	} else {
		ps.peers[peer] = &PeerScore{
			Peer:           peer,
			Score:          25.0,
			SuccessCount:   0,
			FailureCount:   1,
			TotalLatencyMs: 0,
			LastSeen:       now,
			FirstSeen:      now,
		}
		ps.evictIfNeeded()
	}
}

func (ps *PeerScorer) calculateScore(p *PeerScore) float64 {
	total := p.SuccessCount + p.FailureCount
	if total == 0 {
		return 50.0
	}

	successRate := float64(p.SuccessCount) / float64(total)

	var avgLatency float64 = 1000
	if p.SuccessCount > 0 {
		avgLatency = float64(p.TotalLatencyMs) / float64(p.SuccessCount)
	}

	latencyFactor := 1.0
	if avgLatency < 100 {
		latencyFactor = 1.5
	} else if avgLatency < 500 {
		latencyFactor = 1.2
	} else if avgLatency > 2000 {
		latencyFactor = 0.5
	} else if avgLatency > 5000 {
		latencyFactor = 0.2
	}

	score := successRate * 100 * latencyFactor

	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	return score
}

func (ps *PeerScorer) evictIfNeeded() {
	if len(ps.peers) > ps.maxPeers {
		var worst string
		lowestScore := 101.0
		for peer, p := range ps.peers {
			if p.Score < lowestScore {
				lowestScore = p.Score
				worst = peer
			}
		}
		if worst != "" {
			delete(ps.peers, worst)
		}
	}
}

func (ps *PeerScorer) GetScore(peer string) float64 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	if p, ok := ps.peers[peer]; ok {
		return p.Score
	}
	return 0
}

func (ps *PeerScorer) GetTopPeers(n int) []string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	type scoredPeer struct {
		peer  string
		score float64
	}

	var scored []scoredPeer
	for peer, p := range ps.peers {
		scored = append(scored, scoredPeer{peer: peer, score: p.Score})
	}

	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	if n > len(scored) {
		n = len(scored)
	}

	result := make([]string, n)
	for i := 0; i < n; i++ {
		result[i] = scored[i].peer
	}
	return result
}

func (ps *PeerScorer) RemovePeer(peer string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.peers, peer)
}

func (ps *PeerScorer) Count() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.peers)
}
