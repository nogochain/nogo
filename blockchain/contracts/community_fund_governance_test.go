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

package contracts

import (
	"testing"
	"time"
)

// TestCommunityFundContract_Creation tests contract creation
func TestCommunityFundContract_Creation(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()

	if contract.ContractAddress == "" {
		t.Fatal("Expected non-empty contract address")
	}

	// Verify address format
	if len(contract.ContractAddress) < 10 {
		t.Errorf("Contract address too short: %s", contract.ContractAddress)
	}

	if contract.FundBalance != 0 {
		t.Errorf("Expected initial balance 0, got %d", contract.FundBalance)
	}

	if contract.QuorumPercent != 10 {
		t.Errorf("Expected quorum 10%%, got %d%%", contract.QuorumPercent)
	}

	if contract.ApprovalThreshold != 60 {
		t.Errorf("Expected approval threshold 60%%, got %d%%", contract.ApprovalThreshold)
	}
}

// TestCommunityFundContract_AddFunds tests adding funds to the contract
func TestCommunityFundContract_AddFunds(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()

	// Add funds
	contract.AddFunds(1000000)
	if contract.GetFundBalance() != 1000000 {
		t.Errorf("Expected balance 1000000, got %d", contract.GetFundBalance())
	}

	// Add more funds
	contract.AddFunds(500000)
	if contract.GetFundBalance() != 1500000 {
		t.Errorf("Expected balance 1500000, got %d", contract.GetFundBalance())
	}
}

// TestCommunityFundContract_CreateProposal tests proposal creation
func TestCommunityFundContract_CreateProposal(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()

	// Create valid proposal
	proposalID, err := contract.CreateProposal(
		"proposer123",
		"Test Proposal",
		"This is a test proposal",
		ProposalEcosystem,
		1000000,
		"recipient456",
		1000000000, // 10 NOGO deposit
	)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if proposalID == "" {
		t.Fatal("Expected non-empty proposal ID")
	}

	// Verify proposal was created
	proposal, err := contract.GetProposal(proposalID)
	if err != nil {
		t.Fatalf("Failed to get proposal: %v", err)
	}

	if proposal.Proposer != "proposer123" {
		t.Errorf("Expected proposer 'proposer123', got '%s'", proposal.Proposer)
	}

	if proposal.Amount != 1000000 {
		t.Errorf("Expected amount 1000000, got %d", proposal.Amount)
	}

	if proposal.Status != StatusActive {
		t.Errorf("Expected status Active, got %v", proposal.Status)
	}
}

// TestCommunityFundContract_CreateProposal_InvalidDeposit tests proposal with insufficient deposit
func TestCommunityFundContract_CreateProposal_InvalidDeposit(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()

	// Create proposal with deposit below minimum
	_, err := contract.CreateProposal(
		"proposer123",
		"Test Proposal",
		"This is a test proposal",
		ProposalEcosystem,
		1000000,
		"recipient456",
		100, // Too low deposit
	)

	if err == nil {
		t.Fatal("Expected error for low deposit, got nil")
	}
}

// TestCommunityFundContract_Vote tests voting on proposals
func TestCommunityFundContract_Vote(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()
	contract.SetTotalVotingPower(10000)

	// Create proposal
	proposalID, _ := contract.CreateProposal(
		"proposer123",
		"Test Proposal",
		"Description",
		ProposalEcosystem,
		1000000,
		"recipient456",
		1000000000,
	)

	// Vote in favor
	err := contract.Vote(proposalID, "voter1", true, 1000)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Vote against
	err = contract.Vote(proposalID, "voter2", false, 500)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify votes
	proposal, _ := contract.GetProposal(proposalID)
	if proposal.VotesFor != 1000 {
		t.Errorf("Expected 1000 votes for, got %d", proposal.VotesFor)
	}
	if proposal.VotesAgainst != 500 {
		t.Errorf("Expected 500 votes against, got %d", proposal.VotesAgainst)
	}
}

// TestCommunityFundContract_Vote_DoubleVoting tests preventing double voting
func TestCommunityFundContract_Vote_DoubleVoting(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()

	// Create proposal
	proposalID, _ := contract.CreateProposal(
		"proposer123",
		"Test Proposal",
		"Description",
		ProposalEcosystem,
		1000000,
		"recipient456",
		1000000000,
	)

	// Vote once
	contract.Vote(proposalID, "voter1", true, 1000)

	// Try to vote again
	err := contract.Vote(proposalID, "voter1", false, 500)
	if err == nil {
		t.Fatal("Expected error for double voting, got nil")
	}
}

// TestCommunityFundContract_FinalizeVoting tests voting finalization
func TestCommunityFundContract_FinalizeVoting(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()
	contract.SetTotalVotingPower(10000)

	// Create proposal
	proposalID, _ := contract.CreateProposal(
		"proposer123",
		"Test Proposal",
		"Description",
		ProposalEcosystem,
		1000000,
		"recipient456",
		1000000000,
	)

	// Add votes (60% in favor, meets threshold)
	contract.Vote(proposalID, "voter1", true, 6000)
	contract.Vote(proposalID, "voter2", false, 4000)

	// Manually set voting end time to past
	contract.mu.Lock()
	contract.Proposals[proposalID].VotingEndTime = time.Now().Unix() - 100
	contract.mu.Unlock()

	// Finalize voting
	err := contract.FinalizeVoting(proposalID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify proposal passed
	proposal, _ := contract.GetProposal(proposalID)
	if proposal.Status != StatusPassed {
		t.Errorf("Expected status Passed, got %v", proposal.Status)
	}
}

// TestCommunityFundContract_FinalizeVoting_NoQuorum tests failure due to no quorum
func TestCommunityFundContract_FinalizeVoting_NoQuorum(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()
	contract.SetTotalVotingPower(10000)

	// Create proposal
	proposalID, _ := contract.CreateProposal(
		"proposer123",
		"Test Proposal",
		"Description",
		ProposalEcosystem,
		1000000,
		"recipient456",
		1000000000,
	)

	// Add votes but below quorum (only 5% participation, need 10%)
	contract.Vote(proposalID, "voter1", true, 500)

	// Set voting end time to past
	contract.mu.Lock()
	contract.Proposals[proposalID].VotingEndTime = time.Now().Unix() - 100
	contract.mu.Unlock()

	// Finalize voting
	contract.FinalizeVoting(proposalID)

	// Verify proposal rejected due to no quorum
	proposal, _ := contract.GetProposal(proposalID)
	if proposal.Status != StatusRejected {
		t.Errorf("Expected status Rejected (no quorum), got %v", proposal.Status)
	}
}

// TestCommunityFundContract_ExecuteProposal tests proposal execution
func TestCommunityFundContract_ExecuteProposal(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()
	contract.SetTotalVotingPower(10000)

	// Add funds to contract
	contract.AddFunds(2000000)

	// Create and pass a proposal
	proposalID, _ := contract.CreateProposal(
		"proposer123",
		"Test Proposal",
		"Description",
		ProposalEcosystem,
		1000000,
		"recipient456",
		1000000000,
	)

	// Add votes (passes)
	contract.Vote(proposalID, "voter1", true, 6000)
	contract.Vote(proposalID, "voter2", false, 4000)

	// Set voting end time to past
	contract.mu.Lock()
	contract.Proposals[proposalID].VotingEndTime = time.Now().Unix() - 100
	contract.mu.Unlock()

	// Finalize voting
	contract.FinalizeVoting(proposalID)

	// Execute proposal
	err := contract.ExecuteProposal(proposalID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify funds were transferred
	if contract.GetFundBalance() != 1000000 {
		t.Errorf("Expected balance 1000000 after execution, got %d", contract.GetFundBalance())
	}

	// Verify proposal status
	proposal, _ := contract.GetProposal(proposalID)
	if proposal.Status != StatusExecuted {
		t.Errorf("Expected status Executed, got %v", proposal.Status)
	}
}

// TestCommunityFundContract_ExecuteProposal_InsufficientFunds tests execution with insufficient funds
func TestCommunityFundContract_ExecuteProposal_InsufficientFunds(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()
	contract.SetTotalVotingPower(10000)

	// Add insufficient funds
	contract.AddFunds(500000)

	// Create and pass a proposal
	proposalID, _ := contract.CreateProposal(
		"proposer123",
		"Test Proposal",
		"Description",
		ProposalEcosystem,
		1000000, // Request more than available
		"recipient456",
		1000000000,
	)

	// Add votes (passes)
	contract.Vote(proposalID, "voter1", true, 6000)

	// Set voting end time to past
	contract.mu.Lock()
	contract.Proposals[proposalID].VotingEndTime = time.Now().Unix() - 100
	contract.mu.Unlock()

	// Finalize voting
	contract.FinalizeVoting(proposalID)

	// Try to execute with insufficient funds
	err := contract.ExecuteProposal(proposalID)
	if err == nil {
		t.Fatal("Expected error for insufficient funds, got nil")
	}
}

// TestCommunityFundContract_GetVotingStats tests voting statistics
func TestCommunityFundContract_GetVotingStats(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()
	contract.SetTotalVotingPower(10000)

	// Create proposal
	proposalID, _ := contract.CreateProposal(
		"proposer123",
		"Test Proposal",
		"Description",
		ProposalEcosystem,
		1000000,
		"recipient456",
		1000000000,
	)

	// Add votes
	contract.Vote(proposalID, "voter1", true, 6000)
	contract.Vote(proposalID, "voter2", false, 2000)

	// Get stats
	stats, err := contract.GetVotingStats(proposalID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if stats.TotalVotes != 8000 {
		t.Errorf("Expected total votes 8000, got %d", stats.TotalVotes)
	}

	if stats.VotesFor != 6000 {
		t.Errorf("Expected votes for 6000, got %d", stats.VotesFor)
	}

	if stats.ParticipationPercent != 80 {
		t.Errorf("Expected 80%% participation, got %d%%", stats.ParticipationPercent)
	}

	if stats.ApprovalPercent != 75 {
		t.Errorf("Expected 75%% approval, got %d%%", stats.ApprovalPercent)
	}

	if !stats.QuorumReached {
		t.Error("Expected quorum to be reached")
	}

	if !stats.ThresholdReached {
		t.Error("Expected threshold to be reached")
	}
}

// TestCommunityFundContract_ProposalTypes tests different proposal types
func TestCommunityFundContract_ProposalTypes(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()

	types := []ProposalType{
		ProposalTreasury,
		ProposalEcosystem,
		ProposalGrant,
		ProposalEvent,
	}

	for _, propType := range types {
		proposalID, err := contract.CreateProposal(
			"proposer123",
			"Test Proposal",
			"Description",
			propType,
			1000000,
			"recipient456",
			1000000000,
		)

		if err != nil {
			t.Fatalf("Unexpected error for type %v: %v", propType, err)
		}

		proposal, _ := contract.GetProposal(proposalID)
		if proposal.Type != propType {
			t.Errorf("Expected type %v, got %v", propType, proposal.Type)
		}
	}
}

// TestCommunityFundContract_ConcurrentAccess tests thread safety
func TestCommunityFundContract_ConcurrentAccess(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()
	contract.SetTotalVotingPower(10000)
	contract.AddFunds(10000000)

	done := make(chan bool)

	// Concurrent proposal creation
	go func() {
		for i := 0; i < 50; i++ {
			contract.CreateProposal(
				"proposer123",
				"Test Proposal",
				"Description",
				ProposalEcosystem,
				1000000,
				"recipient456",
				1000000000,
			)
		}
		done <- true
	}()

	// Concurrent fund additions
	go func() {
		for i := 0; i < 50; i++ {
			contract.AddFunds(100000)
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < 50; i++ {
			_ = contract.GetFundBalance()
			_ = contract.GetProposalCount()
		}
		done <- true
	}()

	<-done
	<-done
	<-done
}
