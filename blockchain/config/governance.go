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
	"sync"
	"time"
)

// GovernanceParams defines governance configuration parameters
type GovernanceParams struct {
	mu sync.RWMutex

	// MinQuorum is the minimum number of votes for quorum
	MinQuorum uint64 `json:"minQuorum"`

	// ApprovalThreshold is the approval threshold (0.0-1.0)
	ApprovalThreshold float64 `json:"approvalThreshold"`

	// VotingPeriodDays is the voting period in days
	VotingPeriodDays int `json:"votingPeriodDays"`

	// ProposalDeposit is the deposit required to create a proposal
	ProposalDeposit uint64 `json:"proposalDeposit"`

	// ExecutionDelayBlocks is the delay before proposal execution
	ExecutionDelayBlocks uint64 `json:"executionDelayBlocks"`

	// QuorumPercentage is the quorum percentage of total supply (0-100)
	QuorumPercentage uint8 `json:"quorumPercentage"`

	// LockupPeriodDays is the lockup period for delegated votes
	LockupPeriodDays int `json:"lockupPeriodDays"`
}

// DefaultGovernanceParams returns governance parameters with default values
func DefaultGovernanceParams() *GovernanceParams {
	return &GovernanceParams{
		MinQuorum:            1000000,
		ApprovalThreshold:    0.6,
		VotingPeriodDays:     7,
		ProposalDeposit:      100000000000,
		ExecutionDelayBlocks: 100,
		QuorumPercentage:     10,
		LockupPeriodDays:     30,
	}
}

// Validate validates governance parameters
func (g *GovernanceParams) Validate() error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.MinQuorum == 0 {
		return ErrInvalidMinQuorum
	}

	if g.ApprovalThreshold < 0 || g.ApprovalThreshold > 1 {
		return ErrInvalidApprovalThreshold
	}

	if g.VotingPeriodDays <= 0 {
		return ErrInvalidVotingPeriod
	}

	if g.QuorumPercentage == 0 || g.QuorumPercentage > 100 {
		return ErrInvalidQuorumPercentage
	}

	if g.LockupPeriodDays < 0 {
		return ErrInvalidLockupPeriod
	}

	return nil
}

// GetMinQuorum returns the minimum quorum
func (g *GovernanceParams) GetMinQuorum() uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.MinQuorum
}

// GetApprovalThreshold returns the approval threshold
func (g *GovernanceParams) GetApprovalThreshold() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ApprovalThreshold
}

// GetVotingPeriod returns the voting period
func (g *GovernanceParams) GetVotingPeriod() time.Duration {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return time.Duration(g.VotingPeriodDays) * 24 * time.Hour
}

// GetProposalDeposit returns the proposal deposit
func (g *GovernanceParams) GetProposalDeposit() uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ProposalDeposit
}

// GetExecutionDelayBlocks returns the execution delay
func (g *GovernanceParams) GetExecutionDelayBlocks() uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ExecutionDelayBlocks
}

// Error definitions for governance validation
var (
	ErrInvalidMinQuorum         = errors.New("minQuorum must be > 0")
	ErrInvalidApprovalThreshold = errors.New("approvalThreshold must be between 0 and 1")
	ErrInvalidVotingPeriod      = errors.New("votingPeriodDays must be > 0")
	ErrInvalidQuorumPercentage  = errors.New("quorumPercentage must be between 1 and 100")
	ErrInvalidLockupPeriod      = errors.New("lockupPeriodDays must be >= 0")
)
