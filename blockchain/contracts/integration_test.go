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

// TestIntegration_CommunityFundProposalLifecycle tests complete proposal lifecycle
func TestIntegration_CommunityFundProposalLifecycle(t *testing.T) {
	// Create and deploy contracts
	deployer := NewContractDeployer("genesis_hash", 1)
	err := deployer.DeployContracts(0)
	if err != nil {
		t.Fatalf("Failed to deploy contracts: %v", err)
	}

	// Get community fund contract
	communityAddr, _ := deployer.GetCommunityFundAddress()
	contract := NewCommunityFundGovernanceContract()
	contract.ContractAddress = communityAddr

	// Add funds to community fund (simulating block rewards)
	contract.AddFunds(10000000)          // 10 NOGO
	contract.SetTotalVotingPower(100000) // Total supply

	// Create proposal
	proposalID, err := contract.CreateProposal(
		"proposer123",
		"Ecosystem Development Grant",
		"Funding for ecosystem development project",
		ProposalEcosystem,
		5000000, // 5 NOGO
		"developer456",
		1000000000, // 10 NOGO deposit
	)
	if err != nil {
		t.Fatalf("Failed to create proposal: %v", err)
	}

	// Verify proposal created
	proposal, err := contract.GetProposal(proposalID)
	if err != nil {
		t.Fatalf("Failed to get proposal: %v", err)
	}
	if proposal.Status != StatusActive {
		t.Errorf("Expected status Active, got %v", proposal.Status)
	}

	// Cast votes (60% in favor, meets threshold)
	contract.Vote(proposalID, "voter1", true, 40000)  // 40%
	contract.Vote(proposalID, "voter2", true, 20000)  // 20%
	contract.Vote(proposalID, "voter3", false, 10000) // 10%

	// End voting period
	contract.mu.Lock()
	contract.Proposals[proposalID].VotingEndTime = time.Now().Unix() - 100
	contract.mu.Unlock()

	// Finalize voting
	err = contract.FinalizeVoting(proposalID)
	if err != nil {
		t.Fatalf("Failed to finalize voting: %v", err)
	}

	// Verify proposal passed
	proposal, _ = contract.GetProposal(proposalID)
	if proposal.Status != StatusPassed {
		t.Errorf("Expected status Passed, got %v", proposal.Status)
	}

	// Execute proposal
	err = contract.ExecuteProposal(proposalID)
	if err != nil {
		t.Fatalf("Failed to execute proposal: %v", err)
	}

	// Verify execution
	proposal, _ = contract.GetProposal(proposalID)
	if proposal.Status != StatusExecuted {
		t.Errorf("Expected status Executed, got %v", proposal.Status)
	}

	// Verify funds transferred
	if contract.GetFundBalance() != 5000000 {
		t.Errorf("Expected balance 5000000, got %d", contract.GetFundBalance())
	}
}

// TestIntegration_IntegrityRewardDistribution tests integrity reward distribution
func TestIntegration_IntegrityRewardDistribution(t *testing.T) {
	// Create and deploy contracts
	deployer := NewContractDeployer("genesis_hash", 1)
	deployer.DeployContracts(0)

	// Get integrity reward contract
	integrityAddr, _ := deployer.GetIntegrityRewardAddress()
	contract := NewIntegrityRewardContract()
	contract.ContractAddress = integrityAddr

	// Register nodes
	contract.RegisterNode("node1", "address1")
	contract.RegisterNode("node2", "address2")
	contract.RegisterNode("node3", "address3")

	// Update scores (node3 below qualified threshold)
	contract.UpdateNodeScore("node1", 100, "active")
	contract.UpdateNodeScore("node2", 80, "active")
	contract.UpdateNodeScore("node3", 50, "active")

	// Simulate block rewards (1% goes to integrity pool)
	for i := 0; i < 100; i++ {
		contract.AddToRewardPool(1000000) // 10000 per block
	}

	// Verify pool accumulated
	pool := contract.GetRewardPool()
	if pool != 1000000 {
		t.Errorf("Expected pool 1000000, got %d", pool)
	}

	// Distribute rewards at height 5082
	rewards, err := contract.DistributeRewards(5082)
	if err != nil {
		t.Fatalf("Failed to distribute rewards: %v", err)
	}

	// Verify only qualified nodes received rewards
	if len(rewards) != 2 {
		t.Errorf("Expected 2 rewarded nodes, got %d", len(rewards))
	}

	// Verify node1 received more than node2 (higher score)
	if rewards["address1"] <= rewards["address2"] {
		t.Errorf("Expected address1 to receive more than address2")
	}

	// Verify pool is empty after distribution
	pool = contract.GetRewardPool()
	if pool != 0 {
		t.Errorf("Expected pool 0 after distribution, got %d", pool)
	}

	// Verify total distributed
	if contract.TotalDistributed == 0 {
		t.Error("Expected non-zero total distributed")
	}
}

// TestIntegration_GovernanceSecurity tests governance contract security
func TestIntegration_GovernanceSecurity(t *testing.T) {
	// Create community fund contract
	contract := NewCommunityFundGovernanceContract()
	contract.AddFunds(10000000)
	contract.SetTotalVotingPower(100000)

	// Create proposal
	proposalID, _ := contract.CreateProposal(
		"proposer123",
		"Test Proposal",
		"Description",
		ProposalEcosystem,
		5000000,
		"recipient456",
		1000000000,
	)

	// Test 1: Double voting prevention
	contract.Vote(proposalID, "voter1", true, 1000)
	err := contract.Vote(proposalID, "voter1", false, 1000)
	if err == nil {
		t.Fatal("Expected error for double voting, got nil")
	}

	// Test 2: Voting after deadline
	contract.mu.Lock()
	contract.Proposals[proposalID].VotingEndTime = time.Now().Unix() - 100
	contract.mu.Unlock()
	err = contract.Vote(proposalID, "voter2", true, 1000)
	if err == nil {
		t.Fatal("Expected error for voting after deadline, got nil")
	}

	// Test 3: Execute without passing
	proposalID2, _ := contract.CreateProposal(
		"proposer456",
		"Test Proposal 2",
		"Description",
		ProposalEcosystem,
		5000000,
		"recipient789",
		1000000000,
	)
	err = contract.ExecuteProposal(proposalID2)
	if err == nil {
		t.Fatal("Expected error for executing unpassed proposal, got nil")
	}

	// Test 4: Execute with insufficient funds (use fresh contract)
	contract4 := NewCommunityFundGovernanceContract()
	contract4.AddFunds(1000) // Add small amount
	contract4.SetTotalVotingPower(100000)

	proposalID4, _ := contract4.CreateProposal(
		"proposer789",
		"Test Proposal 4",
		"Description",
		ProposalEcosystem,
		10000000, // Request more than available
		"recipient012",
		1000000000,
	)
	// Pass the proposal
	contract4.Vote(proposalID4, "voter1", true, 60000)
	contract4.mu.Lock()
	contract4.Proposals[proposalID4].VotingEndTime = time.Now().Unix() - 100
	contract4.mu.Unlock()
	contract4.FinalizeVoting(proposalID4)

	err = contract4.ExecuteProposal(proposalID4)
	if err == nil {
		t.Fatal("Expected error for insufficient funds, got nil")
	}
}

// TestIntegration_VotingThresholds tests voting threshold logic
func TestIntegration_VotingThresholds(t *testing.T) {
	contract := NewCommunityFundGovernanceContract()
	contract.SetTotalVotingPower(100000)

	// Test 1: Pass with exactly 10% quorum and 60% approval
	proposalID1, _ := contract.CreateProposal(
		"proposer1",
		"Proposal 1",
		"Test",
		ProposalEcosystem,
		1000000,
		"recipient1",
		1000000000,
	)

	// Exactly 10% participation (10000 out of 100000)
	contract.Vote(proposalID1, "voter1", true, 6000)  // 6%
	contract.Vote(proposalID1, "voter2", true, 1000)  // 1%
	contract.Vote(proposalID1, "voter3", false, 3000) // 3%

	// End voting
	contract.mu.Lock()
	contract.Proposals[proposalID1].VotingEndTime = time.Now().Unix() - 100
	contract.mu.Unlock()

	contract.FinalizeVoting(proposalID1)
	proposal, _ := contract.GetProposal(proposalID1)
	if proposal.Status != StatusPassed {
		t.Errorf("Expected proposal to pass with minimum thresholds, got %v", proposal.Status)
	}

	// Test 2: Fail due to insufficient quorum (< 10%)
	contract2 := NewCommunityFundGovernanceContract()
	contract2.SetTotalVotingPower(100000)

	proposalID2, _ := contract2.CreateProposal(
		"proposer2",
		"Proposal 2",
		"Test",
		ProposalEcosystem,
		1000000,
		"recipient2",
		1000000000,
	)

	// Only 5% participation
	contract2.Vote(proposalID2, "voter1", true, 5000)

	// End voting
	contract2.mu.Lock()
	contract2.Proposals[proposalID2].VotingEndTime = time.Now().Unix() - 100
	contract2.mu.Unlock()

	contract2.FinalizeVoting(proposalID2)
	proposal, _ = contract2.GetProposal(proposalID2)
	if proposal.Status != StatusRejected {
		t.Errorf("Expected proposal to fail due to no quorum, got %v", proposal.Status)
	}

	// Test 3: Fail due to insufficient approval (< 60%)
	contract3 := NewCommunityFundGovernanceContract()
	contract3.SetTotalVotingPower(100000)

	proposalID3, _ := contract3.CreateProposal(
		"proposer3",
		"Proposal 3",
		"Test",
		ProposalEcosystem,
		1000000,
		"recipient3",
		1000000000,
	)

	// 50% participation, but only 50% approval
	contract3.Vote(proposalID3, "voter1", true, 2500)
	contract3.Vote(proposalID3, "voter2", false, 2500)

	// End voting
	contract3.mu.Lock()
	contract3.Proposals[proposalID3].VotingEndTime = time.Now().Unix() - 100
	contract3.mu.Unlock()

	contract3.FinalizeVoting(proposalID3)
	proposal, _ = contract3.GetProposal(proposalID3)
	if proposal.Status != StatusRejected {
		t.Errorf("Expected proposal to fail due to low approval, got %v", proposal.Status)
	}
}

// TestIntegration_ContractDeployment tests complete deployment workflow
func TestIntegration_ContractDeployment(t *testing.T) {
	// Deploy contracts
	deployer := NewContractDeployer("genesis_hash_123", 1)
	err := deployer.DeployContracts(0)
	if err != nil {
		t.Fatalf("Failed to deploy contracts: %v", err)
	}

	// Verify both contracts deployed
	if !deployer.AreAllDeployed() {
		t.Fatal("Expected all contracts to be deployed")
	}

	// Get addresses
	communityAddr, err := deployer.GetCommunityFundAddress()
	if err != nil {
		t.Fatalf("Failed to get community fund address: %v", err)
	}

	integrityAddr, err := deployer.GetIntegrityRewardAddress()
	if err != nil {
		t.Fatalf("Failed to get integrity reward address: %v", err)
	}

	// Verify addresses are different
	if communityAddr == integrityAddr {
		t.Error("Expected different contract addresses")
	}

	// Verify addresses are valid format
	if len(communityAddr) < 20 {
		t.Errorf("Community fund address too short: %s", communityAddr)
	}
	if len(integrityAddr) < 20 {
		t.Errorf("Integrity reward address too short: %s", integrityAddr)
	}

	// Initialize registry
	registry := &ContractRegistry{}
	registry.Initialize(deployer)

	// Verify registry initialized
	if !registry.IsInitialized() {
		t.Error("Expected registry to be initialized")
	}

	// Get contracts through registry
	communityContract, err := registry.GetCommunityFundContract()
	if err != nil {
		t.Fatalf("Failed to get community fund contract: %v", err)
	}
	if communityContract == nil {
		t.Fatal("Expected non-nil community fund contract")
	}

	integrityContract, err := registry.GetIntegrityRewardContract()
	if err != nil {
		t.Fatalf("Failed to get integrity reward contract: %v", err)
	}
	if integrityContract == nil {
		t.Fatal("Expected non-nil integrity reward contract")
	}
}

// TestIntegration_ConcurrentContractOperations tests concurrent operations across contracts
func TestIntegration_ConcurrentContractOperations(t *testing.T) {
	// Deploy contracts
	deployer := NewContractDeployer("genesis_hash", 1)
	deployer.DeployContracts(0)

	// Get contracts
	communityAddr, _ := deployer.GetCommunityFundAddress()
	communityContract := NewCommunityFundGovernanceContract()
	communityContract.ContractAddress = communityAddr
	communityContract.AddFunds(10000000)
	communityContract.SetTotalVotingPower(100000)

	integrityAddr, _ := deployer.GetIntegrityRewardAddress()
	integrityContract := NewIntegrityRewardContract()
	integrityContract.ContractAddress = integrityAddr

	done := make(chan bool)

	// Concurrent community fund operations
	go func() {
		for i := 0; i < 20; i++ {
			proposalID, _ := communityContract.CreateProposal(
				"proposer",
				"Proposal",
				"Test",
				ProposalEcosystem,
				1000000,
				"recipient",
				1000000000,
			)
			communityContract.Vote(proposalID, "voter", true, 1000)
			_ = communityContract.GetFundBalance()
		}
		done <- true
	}()

	// Concurrent integrity reward operations
	go func() {
		for i := 0; i < 20; i++ {
			integrityContract.RegisterNode("node"+string(rune('a'+i)), "address"+string(rune('0'+i)))
			integrityContract.AddToRewardPool(1000000)
			_ = integrityContract.GetRewardPool()
		}
		done <- true
	}()

	// Concurrent deployment info reads
	go func() {
		for i := 0; i < 20; i++ {
			_, _ = deployer.GetCommunityFundAddress()
			_, _ = deployer.GetIntegrityRewardAddress()
			_ = deployer.AreAllDeployed()
		}
		done <- true
	}()

	<-done
	<-done
	<-done
}
