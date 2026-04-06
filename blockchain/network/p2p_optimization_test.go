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

package network

import (
	"testing"
	"time"
)

func TestPeerScorerSorting(t *testing.T) {
	scorer := NewPeerScorer(100)

	peers := []string{"peer1", "peer2", "peer3", "peer4", "peer5"}
	scores := []float64{50.0, 80.0, 30.0, 90.0, 60.0}

	for i, peer := range peers {
		scorer.peers[peer] = &PeerScore{
			Peer:         peer,
			Score:        scores[i],
			SuccessCount: 10,
			FailureCount: 0,
			LastSeen:     time.Now(),
			TrustLevel:   0.8,
		}
	}

	topPeers := scorer.GetTopPeers(3)
	if len(topPeers) != 3 {
		t.Errorf("expected 3 top peers, got %d", len(topPeers))
	}

	if topPeers[0] != "peer4" {
		t.Errorf("expected highest score peer (peer4), got %s", topPeers[0])
	}

	if topPeers[1] != "peer2" {
		t.Errorf("expected second highest score peer (peer2), got %s", topPeers[1])
	}

	if topPeers[2] != "peer5" {
		t.Errorf("expected third highest score peer (peer5), got %s", topPeers[2])
	}
}

func TestAdvancedPeerScorerSorting(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100)

	peers := []string{"peer1", "peer2", "peer3", "peer4", "peer5"}
	scores := []float64{50.0, 80.0, 30.0, 90.0, 60.0}

	for i, peer := range peers {
		score := &AdvancedPeerScore{
			Peer:         peer,
			Score:        scores[i],
			SuccessCount: 10,
			FailureCount: 0,
			LastSeen:     time.Now(),
			TrustLevel:   0.8,
		}
		scorer.peers[peer] = score
		score.Signature = scorer.generateSignature(score)
	}

	topPeers := scorer.GetTopPeersByScore(3)
	if len(topPeers) != 3 {
		t.Errorf("expected 3 top peers, got %d", len(topPeers))
	}

	if topPeers[0] != "peer4" {
		t.Errorf("expected highest score peer (peer4), got %s", topPeers[0])
	}

	if topPeers[1] != "peer2" {
		t.Errorf("expected second highest score peer (peer2), got %s", topPeers[1])
	}

	if topPeers[2] != "peer5" {
		t.Errorf("expected third highest score peer (peer5), got %s", topPeers[2])
	}
}

func TestPeerScorerEviction(t *testing.T) {
	scorer := NewPeerScorer(3)

	peers := []string{"peer1", "peer2", "peer3", "peer4"}
	scores := []float64{50.0, 80.0, 30.0, 90.0}

	for i, peer := range peers {
		scorer.peers[peer] = &PeerScore{
			Peer:         peer,
			Score:        scores[i],
			SuccessCount: 10,
			FailureCount: 0,
			LastSeen:     time.Now(),
			TrustLevel:   0.8,
		}
		scorer.evictIfNeeded()
	}

	if len(scorer.peers) != 3 {
		t.Errorf("expected 3 peers after eviction, got %d", len(scorer.peers))
	}

	if _, exists := scorer.peers["peer3"]; exists {
		t.Error("lowest score peer (peer3) should have been evicted")
	}
}
