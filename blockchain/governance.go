package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type GovernanceProposal struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Type         string    `json:"type"`
	Amount       uint64    `json:"amount,omitempty"`
	Target       string    `json:"target,omitempty"`
	Creator      string    `json:"creator"`
	VotesFor     uint64    `json:"votesFor"`
	VotesAgainst uint64    `json:"votesAgainst"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	Status       string    `json:"status"`
	Executed     bool      `json:"executed"`
}

type GovernanceVote struct {
	ProposalID string    `json:"proposalId"`
	Voter      string    `json:"voter"`
	Vote       bool      `json:"vote"`
	Weight     uint64    `json:"weight"`
	Timestamp  time.Time `json:"timestamp"`
}

type Governance struct {
	proposals         map[string]*GovernanceProposal
	votes             map[string][]GovernanceVote
	daoTreasury       uint64
	mu                sync.RWMutex
	minQuorum         uint64
	approvalThreshold float64
}

func NewGovernance() *Governance {
	return &Governance{
		proposals:         make(map[string]*GovernanceProposal),
		votes:             make(map[string][]GovernanceVote),
		daoTreasury:       0,
		minQuorum:         1000,
		approvalThreshold: 0.6,
	}
}

func (g *Governance) CreateProposal(title, desc, proposalType, creator string, amount uint64, target string, durationDays int) (*GovernanceProposal, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	proposal := &GovernanceProposal{
		ID:           fmt.Sprintf("prop-%d", time.Now().UnixNano()),
		Title:        title,
		Description:  desc,
		Type:         proposalType,
		Amount:       amount,
		Target:       target,
		Creator:      creator,
		VotesFor:     0,
		VotesAgainst: 0,
		StartTime:    time.Now(),
		EndTime:      time.Now().AddDate(0, 0, durationDays),
		Status:       "active",
		Executed:     false,
	}

	g.proposals[proposal.ID] = proposal
	g.votes[proposal.ID] = make([]GovernanceVote, 0)

	return proposal, nil
}

func (g *Governance) Vote(proposalID, voter string, approve bool, weight uint64) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	proposal, ok := g.proposals[proposalID]
	if !ok {
		return fmt.Errorf("proposal not found")
	}

	if time.Now().After(proposal.EndTime) {
		return fmt.Errorf("voting period ended")
	}

	for _, v := range g.votes[proposalID] {
		if v.Voter == voter {
			return fmt.Errorf("already voted")
		}
	}

	vote := GovernanceVote{
		ProposalID: proposalID,
		Voter:      voter,
		Vote:       approve,
		Weight:     weight,
		Timestamp:  time.Now(),
	}

	g.votes[proposalID] = append(g.votes[proposalID], vote)

	if approve {
		proposal.VotesFor += weight
	} else {
		proposal.VotesAgainst += weight
	}

	return nil
}

func (g *Governance) ExecuteProposal(proposalID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	proposal, ok := g.proposals[proposalID]
	if !ok {
		return fmt.Errorf("proposal not found")
	}

	if proposal.Executed {
		return fmt.Errorf("already executed")
	}

	if time.Now().Before(proposal.EndTime) {
		return fmt.Errorf("voting period not ended")
	}

	totalVotes := proposal.VotesFor + proposal.VotesAgainst
	if totalVotes < g.minQuorum {
		proposal.Status = "rejected"
		return fmt.Errorf("quorum not reached")
	}

	approvalRate := float64(proposal.VotesFor) / float64(totalVotes)
	if approvalRate < g.approvalThreshold {
		proposal.Status = "rejected"
		return fmt.Errorf("approval threshold not met")
	}

	switch proposal.Type {
	case "treasury":
		if g.daoTreasury < proposal.Amount {
			return fmt.Errorf("insufficient treasury")
		}
		g.daoTreasury -= proposal.Amount
	case "parameter":
	case "upgrade":
	default:
		return fmt.Errorf("unknown proposal type")
	}

	proposal.Status = "executed"
	proposal.Executed = true
	proposal.Status = "passed"

	return nil
}

func (g *Governance) GetProposal(id string) (*GovernanceProposal, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	proposal, ok := g.proposals[id]
	return proposal, ok
}

func (g *Governance) ListProposals(status string) []*GovernanceProposal {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*GovernanceProposal
	for _, p := range g.proposals {
		if status == "" || p.Status == status {
			result = append(result, p)
		}
	}
	return result
}

func (g *Governance) GetVotes(proposalID string) []GovernanceVote {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.votes[proposalID]
}

func (g *Governance) AddToTreasury(amount uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.daoTreasury += amount
}

func (g *Governance) GetTreasury() uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.daoTreasury
}

type GovernanceJSON struct {
	Proposals map[string]*GovernanceProposal `json:"proposals"`
	Treasury  uint64                         `json:"treasury"`
	MinQuorum uint64                         `json:"minQuorum"`
	Threshold float64                        `json:"threshold"`
}

func (g *Governance) MarshalJSON() ([]byte, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return json.Marshal(GovernanceJSON{
		Proposals: g.proposals,
		Treasury:  g.daoTreasury,
		MinQuorum: g.minQuorum,
		Threshold: g.approvalThreshold,
	})
}
