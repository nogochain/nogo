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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package config

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// UpgradeProposal represents a protocol upgrade proposal
type UpgradeProposal struct {
	mu              sync.RWMutex
	Version         Version   `json:"version"`
	Description     string    `json:"description"`
	ProposedBy      string    `json:"proposedBy"`
	ProposedAt      time.Time `json:"proposedAt"`
	VotesFor        uint64    `json:"votesFor"`
	VotesAgainst    uint64    `json:"votesAgainst"`
	Status          string    `json:"status"`
	ActivationBlock uint64    `json:"activationBlock"`
}

// UpgradeManager manages protocol upgrades
type UpgradeManager struct {
	mu        sync.RWMutex
	current   Version
	proposals map[string]*UpgradeProposal
	history   []Version
	activated map[uint64]bool
}

// NewUpgradeManager creates a new upgrade manager
func NewUpgradeManager(currentVersion Version) *UpgradeManager {
	return &UpgradeManager{
		current:   currentVersion,
		proposals: make(map[string]*UpgradeProposal),
		history:   []Version{currentVersion},
		activated: make(map[uint64]bool),
	}
}

// GetCurrentVersion returns the current version
func (m *UpgradeManager) GetCurrentVersion() Version {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// ProposeUpgrade creates a new upgrade proposal
func (m *UpgradeManager) ProposeUpgrade(version Version, description, proposer string, activationHeight uint64) (*UpgradeProposal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	versionKey := fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)
	if _, exists := m.proposals[versionKey]; exists {
		return nil, ErrUpgradeProposalExists
	}

	if version.Major < m.current.Major || (version.Major == m.current.Major && version.Minor < m.current.Minor) {
		return nil, ErrCannotDowngrade
	}

	proposal := &UpgradeProposal{
		Version:         version,
		Description:     description,
		ProposedBy:      proposer,
		ProposedAt:      time.Now(),
		VotesFor:        0,
		VotesAgainst:    0,
		Status:          "pending",
		ActivationBlock: activationHeight,
	}

	m.proposals[versionKey] = proposal
	return proposal, nil
}

// Vote casts a vote on an upgrade proposal
func (m *UpgradeManager) Vote(versionKey string, approve bool, voter string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	proposal, exists := m.proposals[versionKey]
	if !exists {
		return ErrProposalNotFound
	}

	if proposal.Status != "pending" {
		return ErrProposalNotPending
	}

	if approve {
		proposal.VotesFor++
	} else {
		proposal.VotesAgainst++
	}

	return nil
}

// ActivateProposal activates an upgrade proposal
func (m *UpgradeManager) ActivateProposal(versionKey string, currentHeight uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	proposal, exists := m.proposals[versionKey]
	if !exists {
		return ErrProposalNotFound
	}

	if proposal.ActivationBlock > currentHeight {
		return ErrActivationHeightNotReached
	}

	proposal.Status = "activated"
	m.current = proposal.Version
	m.history = append(m.history, proposal.Version)
	m.activated[currentHeight] = true

	return nil
}

// GetProposal returns a proposal by version key
func (m *UpgradeManager) GetProposal(versionKey string) (*UpgradeProposal, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	proposal, exists := m.proposals[versionKey]
	return proposal, exists
}

// ListProposals returns all proposals
func (m *UpgradeManager) ListProposals() []*UpgradeProposal {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*UpgradeProposal
	for _, p := range m.proposals {
		result = append(result, p)
	}
	return result
}

// IsUpgradeRequired checks if upgrade is required for peer version
func (m *UpgradeManager) IsUpgradeRequired(peerVersion string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peer, err := ParseVersion(peerVersion)
	if err != nil {
		return true
	}

	return peer.IsLess(m.current)
}

// GetVersionString returns the current version as string
func (m *UpgradeManager) GetVersionString() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current.String()
}

// ShouldSignalUpgrade checks if upgrade should be signaled at height
func (m *UpgradeManager) ShouldSignalUpgrade(height uint64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activated[height]
}

// GetUpgradeHistory returns the upgrade history
func (m *UpgradeManager) GetUpgradeHistory() []Version {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history := make([]Version, len(m.history))
	copy(history, m.history)
	return history
}

// Error definitions for upgrade manager
var (
	ErrUpgradeProposalExists      = errors.New("upgrade proposal already exists")
	ErrCannotDowngrade            = errors.New("cannot downgrade protocol")
	ErrProposalNotFound           = errors.New("proposal not found")
	ErrProposalNotPending         = errors.New("proposal no longer pending")
	ErrActivationHeightNotReached = errors.New("activation height not reached")
)
