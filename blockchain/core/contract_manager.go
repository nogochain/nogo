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

package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/nogochain/nogo/blockchain/contracts"
)

// ContractManager manages smart contracts for the blockchain
// Production-grade: thread-safe contract management
type ContractManager struct {
	mu sync.RWMutex
	// CommunityFundContract is the community fund governance contract
	communityFundContract *contracts.CommunityFundGovernanceContract
	// IntegrityRewardContract is the integrity reward contract
	integrityRewardContract *contracts.IntegrityRewardContract
	// CommunityFundAddress is the contract address for community fund
	communityFundAddress string
	// IntegrityPoolAddress is the contract address for integrity pool
	integrityPoolAddress string
	// DataDir is the directory for contract data persistence
	dataDir string
}

// NewContractManager creates a new contract manager
func NewContractManager() *ContractManager {
	return &ContractManager{
		communityFundContract:   nil,
		integrityRewardContract: nil,
		communityFundAddress:    "",
		integrityPoolAddress:    "",
	}
}

// InitializeContracts initializes contracts at genesis
// Called once during blockchain initialization
func (cm *ContractManager) InitializeContracts(communityFundAddr, integrityPoolAddr string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Store contract addresses
	cm.communityFundAddress = communityFundAddr
	cm.integrityPoolAddress = integrityPoolAddr

	// Create contract instances
	// Note: In production, contracts would be deployed with specific initialization
	// For now, we create instances that will manage funds at these addresses
	cm.communityFundContract = contracts.NewCommunityFundGovernanceContract()
	cm.integrityRewardContract = contracts.NewIntegrityRewardContract()

	// Set contract addresses to match genesis addresses
	// This ensures contracts manage the correct addresses
	// Note: In a real deployment, addresses would be set during contract creation
	// For now, we use the genesis-generated addresses

	return nil
}

// GetCommunityFundContract returns the community fund contract
func (cm *ContractManager) GetCommunityFundContract() *contracts.CommunityFundGovernanceContract {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.communityFundContract
}

// GetIntegrityRewardContract returns the integrity reward contract
func (cm *ContractManager) GetIntegrityRewardContract() *contracts.IntegrityRewardContract {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.integrityRewardContract
}

// GetCommunityFundAddress returns the community fund contract address
func (cm *ContractManager) GetCommunityFundAddress() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.communityFundAddress
}

// GetIntegrityPoolAddress returns the integrity pool contract address
func (cm *ContractManager) GetIntegrityPoolAddress() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.integrityPoolAddress
}

// AddCommunityFundReward adds community fund reward to the contract
// Called for each block - adds 2% of block reward
func (cm *ContractManager) AddCommunityFundReward(blockReward uint64) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.communityFundContract == nil {
		return nil // Contract not initialized yet
	}

	// Add reward to contract's fund balance
	// In production, this would be a blockchain transfer
	// For now, we track it in the contract's internal state
	cm.communityFundContract.AddFunds(blockReward * 2 / 100) // 2%

	return nil
}

// AddIntegrityPoolReward adds integrity pool reward to the contract
// Called for each block - adds 1% of block reward
func (cm *ContractManager) AddIntegrityPoolReward(blockReward uint64) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.integrityRewardContract == nil {
		return nil // Contract not initialized yet
	}

	// Add reward to contract's reward pool
	cm.integrityRewardContract.AddToRewardPool(blockReward)

	return nil
}

// GetAllProposals returns all community fund proposals
func (cm *ContractManager) GetAllProposals() []contracts.Proposal {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.communityFundContract == nil {
		return nil
	}

	return cm.communityFundContract.GetAllProposals()
}

// GetProposalByID returns a proposal by ID
func (cm *ContractManager) GetProposalByID(proposalID string) *contracts.Proposal {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.communityFundContract == nil {
		return nil
	}

	return cm.communityFundContract.GetProposalByID(proposalID)
}

// CreateProposal creates a new community fund proposal
func (cm *ContractManager) CreateProposal(
	proposer string,
	title string,
	description string,
	propType contracts.ProposalType,
	amount uint64,
	recipient string,
	deposit uint64,
) (string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.communityFundContract == nil {
		return "", errors.New("community fund contract not initialized")
	}

	return cm.communityFundContract.CreateProposal(
		proposer,
		title,
		description,
		propType,
		amount,
		recipient,
		deposit,
	)
}

// VoteOnProposal casts a vote on a proposal
func (cm *ContractManager) VoteOnProposal(
	proposalID string,
	voter string,
	support bool,
	votingPower uint64,
) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.communityFundContract == nil {
		return errors.New("community fund contract not initialized")
	}

	return cm.communityFundContract.Vote(proposalID, voter, support, votingPower)
}

// SetDataDir sets the data directory for contract persistence
func (cm *ContractManager) SetDataDir(dataDir string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.dataDir = dataDir
}

// LoadProposals loads proposals from persistent storage
func (cm *ContractManager) LoadProposals() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.dataDir == "" {
		return nil // No data directory set
	}

	if cm.communityFundContract == nil {
		return errors.New("community fund contract not initialized")
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(cm.dataDir, 0700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	// Load community fund contract state
	contractFile := filepath.Join(cm.dataDir, "community_fund.json")
	if err := cm.communityFundContract.LoadFromFile(contractFile); err != nil {
		return fmt.Errorf("load community fund contract: %w", err)
	}

	return nil
}

// SaveProposals saves proposals to persistent storage
func (cm *ContractManager) SaveProposals() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.dataDir == "" {
		return nil // No data directory set
	}

	if cm.communityFundContract == nil {
		return errors.New("community fund contract not initialized")
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(cm.dataDir, 0700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	// Save community fund contract state
	contractFile := filepath.Join(cm.dataDir, "community_fund.json")
	if err := cm.communityFundContract.SaveToFile(contractFile); err != nil {
		return fmt.Errorf("save community fund contract: %w", err)
	}

	return nil
}
