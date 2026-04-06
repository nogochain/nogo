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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ContractType represents the type of governance contract
type ContractType uint8

const (
	// ContractCommunityFund represents community fund governance contract
	ContractCommunityFund ContractType = iota
	// ContractIntegrityReward represents integrity reward contract
	ContractIntegrityReward
)

// String returns the string representation of contract type
func (c ContractType) String() string {
	switch c {
	case ContractCommunityFund:
		return "community_fund"
	case ContractIntegrityReward:
		return "integrity_reward"
	default:
		return "unknown"
	}
}

// ContractDeployment represents a deployed contract record
type ContractDeployment struct {
	// Type is the contract type
	Type ContractType `json:"type"`
	// Address is the deployed contract address
	Address string `json:"address"`
	// DeployedAt is the deployment timestamp
	DeployedAt int64 `json:"deployedAt"`
	// DeployHeight is the block height at deployment
	DeployHeight uint64 `json:"deployHeight"`
	// DeployTxHash is the deployment transaction hash
	DeployTxHash string `json:"deployTxHash"`
	// Active indicates if the contract is active
	Active bool `json:"active"`
}

// ContractDeployer handles deployment and management of governance contracts
// Production-grade: ensures contracts are deployed at blockchain initialization
// Contract addresses are auto-generated and cannot be manually created
type ContractDeployer struct {
	mu sync.RWMutex

	// DeployedContracts is the map of contract type to deployment record
	DeployedContracts map[ContractType]*ContractDeployment
	// GenesisBlockHash is the genesis block hash (used for address generation)
	GenesisBlockHash string
	// ChainID is the blockchain chain ID
	ChainID uint64
}

// NewContractDeployer creates a new contract deployer
func NewContractDeployer(genesisHash string, chainID uint64) *ContractDeployer {
	return &ContractDeployer{
		DeployedContracts: make(map[ContractType]*ContractDeployment),
		GenesisBlockHash:  genesisHash,
		ChainID:           chainID,
	}
}

// DeployContracts deploys all governance contracts
// Called during blockchain initialization
// Returns error if contracts are already deployed
func (d *ContractDeployer) DeployContracts(genesisHeight uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if already deployed
	if len(d.DeployedContracts) > 0 {
		return errors.New("contracts already deployed")
	}

	// Generate deployment transaction hash
	deployTxData := d.GenesisBlockHash + string(rune(d.ChainID)) + string(rune(time.Now().UnixNano()))
	hash := sha256.Sum256([]byte(deployTxData))
	deployTxHash := hex.EncodeToString(hash[:])

	now := time.Now().Unix()

	// Deploy Community Fund Governance Contract
	communityFundContract := NewCommunityFundGovernanceContract()
	d.DeployedContracts[ContractCommunityFund] = &ContractDeployment{
		Type:         ContractCommunityFund,
		Address:      communityFundContract.GetContractAddress(),
		DeployedAt:   now,
		DeployHeight: genesisHeight,
		DeployTxHash: deployTxHash + "_community",
		Active:       true,
	}

	// Deploy Integrity Reward Contract
	integrityRewardContract := NewIntegrityRewardContract()
	d.DeployedContracts[ContractIntegrityReward] = &ContractDeployment{
		Type:         ContractIntegrityReward,
		Address:      integrityRewardContract.GetContractAddress(),
		DeployedAt:   now,
		DeployHeight: genesisHeight,
		DeployTxHash: deployTxHash + "_integrity",
		Active:       true,
	}

	return nil
}

// GetContractAddress returns the address of a deployed contract
func (d *ContractDeployer) GetContractAddress(contractType ContractType) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	deployment, exists := d.DeployedContracts[contractType]
	if !exists {
		return "", errors.New("contract not found")
	}

	if !deployment.Active {
		return "", errors.New("contract is not active")
	}

	return deployment.Address, nil
}

// GetCommunityFundAddress returns the community fund contract address
func (d *ContractDeployer) GetCommunityFundAddress() (string, error) {
	return d.GetContractAddress(ContractCommunityFund)
}

// GetIntegrityRewardAddress returns the integrity reward contract address
func (d *ContractDeployer) GetIntegrityRewardAddress() (string, error) {
	return d.GetContractAddress(ContractIntegrityReward)
}

// IsDeployed checks if a contract is deployed
func (d *ContractDeployer) IsDeployed(contractType ContractType) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	deployment, exists := d.DeployedContracts[contractType]
	return exists && deployment.Active
}

// AreAllDeployed checks if all governance contracts are deployed
func (d *ContractDeployer) AreAllDeployed() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.DeployedContracts[ContractCommunityFund] != nil &&
		d.DeployedContracts[ContractCommunityFund].Active &&
		d.DeployedContracts[ContractIntegrityReward] != nil &&
		d.DeployedContracts[ContractIntegrityReward].Active
}

// GetDeploymentInfo returns deployment information for a contract
func (d *ContractDeployer) GetDeploymentInfo(contractType ContractType) (*ContractDeployment, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	deployment, exists := d.DeployedContracts[contractType]
	if !exists {
		return nil, errors.New("contract not found")
	}

	// Return a copy
	deploymentCopy := *deployment
	return &deploymentCopy, nil
}

// GetAllDeployments returns all deployed contracts
func (d *ContractDeployer) GetAllDeployments() []*ContractDeployment {
	d.mu.RLock()
	defer d.mu.RUnlock()

	deployments := make([]*ContractDeployment, 0, len(d.DeployedContracts))
	for _, deployment := range d.DeployedContracts {
		// Return a copy
		deploymentCopy := *deployment
		deployments = append(deployments, &deploymentCopy)
	}

	return deployments
}

// GetActiveContractCount returns the number of active contracts
func (d *ContractDeployer) GetActiveContractCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	count := 0
	for _, deployment := range d.DeployedContracts {
		if deployment.Active {
			count++
		}
	}

	return count
}

// ValidateContractAddress validates a contract address
func (d *ContractDeployer) ValidateContractAddress(contractType ContractType, address string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	deployment, exists := d.DeployedContracts[contractType]
	if !exists {
		return false
	}

	return deployment.Address == address && deployment.Active
}

// GetDeployTxHash returns the deployment transaction hash for a contract
func (d *ContractDeployer) GetDeployTxHash(contractType ContractType) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	deployment, exists := d.DeployedContracts[contractType]
	if !exists {
		return "", errors.New("contract not found")
	}

	return deployment.DeployTxHash, nil
}

// ContractRegistry provides a global registry of deployed contracts
// This is used to access contracts from anywhere in the blockchain
type ContractRegistry struct {
	mu       sync.RWMutex
	deployer *ContractDeployer
}

// Global contract registry instance
var globalRegistry *ContractRegistry
var registryOnce sync.Once

// GetContractRegistry returns the global contract registry instance
func GetContractRegistry() *ContractRegistry {
	registryOnce.Do(func() {
		globalRegistry = &ContractRegistry{
			deployer: nil,
		}
	})
	return globalRegistry
}

// Initialize initializes the contract registry with a deployer
func (r *ContractRegistry) Initialize(deployer *ContractDeployer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deployer = deployer
}

// GetDeployer returns the contract deployer
func (r *ContractRegistry) GetDeployer() *ContractDeployer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.deployer
}

// GetCommunityFundContract creates and returns a new community fund contract instance
// In production, this would load the contract state from the blockchain
func (r *ContractRegistry) GetCommunityFundContract() (*CommunityFundGovernanceContract, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.deployer == nil {
		return nil, errors.New("registry not initialized")
	}

	address, err := r.deployer.GetCommunityFundAddress()
	if err != nil {
		return nil, err
	}

	// Create new contract instance
	// In production, this would load state from blockchain storage
	contract := NewCommunityFundGovernanceContract()

	// Verify the contract address matches
	if contract.GetContractAddress() != address {
		// In production, we would load the actual contract state
		// For now, we just create a new instance with the correct address
		contract.ContractAddress = address
	}

	return contract, nil
}

// GetIntegrityRewardContract creates and returns a new integrity reward contract instance
func (r *ContractRegistry) GetIntegrityRewardContract() (*IntegrityRewardContract, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.deployer == nil {
		return nil, errors.New("registry not initialized")
	}

	address, err := r.deployer.GetIntegrityRewardAddress()
	if err != nil {
		return nil, err
	}

	// Create new contract instance
	// In production, this would load the contract state from the blockchain
	contract := NewIntegrityRewardContract()

	// Verify the contract address matches
	if contract.GetContractAddress() != address {
		// In production, we would load the actual contract state
		contract.ContractAddress = address
	}

	return contract, nil
}

// IsInitialized checks if the registry is initialized
func (r *ContractRegistry) IsInitialized() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.deployer != nil
}

// AreContractsDeployed checks if all contracts are deployed
func (r *ContractRegistry) AreContractsDeployed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.deployer == nil {
		return false
	}

	return r.deployer.AreAllDeployed()
}
