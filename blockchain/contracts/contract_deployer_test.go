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
)

// TestContractDeployer_Creation tests deployer creation
func TestContractDeployer_Creation(t *testing.T) {
	deployer := NewContractDeployer("genesis_hash_123", 1)

	if deployer.GenesisBlockHash != "genesis_hash_123" {
		t.Errorf("Expected genesis hash 'genesis_hash_123', got '%s'", deployer.GenesisBlockHash)
	}

	if deployer.ChainID != 1 {
		t.Errorf("Expected chain ID 1, got %d", deployer.ChainID)
	}

	if len(deployer.DeployedContracts) != 0 {
		t.Errorf("Expected 0 deployed contracts initially, got %d", len(deployer.DeployedContracts))
	}
}

// TestContractDeployer_DeployContracts tests contract deployment
func TestContractDeployer_DeployContracts(t *testing.T) {
	deployer := NewContractDeployer("genesis_hash_123", 1)

	// Deploy contracts
	err := deployer.DeployContracts(0)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify contracts are deployed
	if !deployer.AreAllDeployed() {
		t.Fatal("Expected all contracts to be deployed")
	}

	// Verify community fund contract
	communityAddr, err := deployer.GetCommunityFundAddress()
	if err != nil {
		t.Fatalf("Failed to get community fund address: %v", err)
	}

	if communityAddr == "" {
		t.Error("Expected non-empty community fund address")
	}

	// Verify integrity reward contract
	integrityAddr, err := deployer.GetIntegrityRewardAddress()
	if err != nil {
		t.Fatalf("Failed to get integrity reward address: %v", err)
	}

	if integrityAddr == "" {
		t.Error("Expected non-empty integrity reward address")
	}

	// Addresses should be different
	if communityAddr == integrityAddr {
		t.Error("Expected different addresses for different contracts")
	}
}

// TestContractDeployer_DoubleDeployment tests preventing double deployment
func TestContractDeployer_DoubleDeployment(t *testing.T) {
	deployer := NewContractDeployer("genesis_hash_123", 1)

	// Deploy contracts first time
	err := deployer.DeployContracts(0)
	if err != nil {
		t.Fatalf("Unexpected error on first deployment: %v", err)
	}

	// Try to deploy again
	err = deployer.DeployContracts(0)
	if err == nil {
		t.Fatal("Expected error for double deployment, got nil")
	}
}

// TestContractDeployer_DeploymentInfo tests deployment info retrieval
func TestContractDeployer_DeploymentInfo(t *testing.T) {
	deployer := NewContractDeployer("genesis_hash_123", 1)
	deployer.DeployContracts(100)

	// Get community fund deployment info
	info, err := deployer.GetDeploymentInfo(ContractCommunityFund)
	if err != nil {
		t.Fatalf("Failed to get deployment info: %v", err)
	}

	if info.Type != ContractCommunityFund {
		t.Errorf("Expected type CommunityFund, got %v", info.Type)
	}

	if info.DeployHeight != 100 {
		t.Errorf("Expected deploy height 100, got %d", info.DeployHeight)
	}

	if !info.Active {
		t.Error("Expected contract to be active")
	}

	if info.Address == "" {
		t.Error("Expected non-empty address")
	}

	if info.DeployTxHash == "" {
		t.Error("Expected non-empty deploy tx hash")
	}
}

// TestContractDeployer_GetAllDeployments tests getting all deployments
func TestContractDeployer_GetAllDeployments(t *testing.T) {
	deployer := NewContractDeployer("genesis_hash_123", 1)
	deployer.DeployContracts(0)

	deployments := deployer.GetAllDeployments()

	if len(deployments) != 2 {
		t.Errorf("Expected 2 deployments, got %d", len(deployments))
	}

	// Verify both contract types are present
	foundCommunity := false
	foundIntegrity := false

	for _, deployment := range deployments {
		if deployment.Type == ContractCommunityFund {
			foundCommunity = true
		}
		if deployment.Type == ContractIntegrityReward {
			foundIntegrity = true
		}
	}

	if !foundCommunity {
		t.Error("Expected to find community fund contract")
	}

	if !foundIntegrity {
		t.Error("Expected to find integrity reward contract")
	}
}

// TestContractDeployer_ActiveCount tests active contract count
func TestContractDeployer_ActiveCount(t *testing.T) {
	deployer := NewContractDeployer("genesis_hash_123", 1)

	// Initially 0
	if deployer.GetActiveContractCount() != 0 {
		t.Errorf("Expected 0 active contracts initially, got %d", deployer.GetActiveContractCount())
	}

	// Deploy contracts
	deployer.DeployContracts(0)

	// Should be 2
	if deployer.GetActiveContractCount() != 2 {
		t.Errorf("Expected 2 active contracts after deployment, got %d", deployer.GetActiveContractCount())
	}
}

// TestContractDeployer_Validation tests contract address validation
func TestContractDeployer_Validation(t *testing.T) {
	deployer := NewContractDeployer("genesis_hash_123", 1)
	deployer.DeployContracts(0)

	// Get valid address
	validAddr, _ := deployer.GetCommunityFundAddress()

	// Validate correct address
	if !deployer.ValidateContractAddress(ContractCommunityFund, validAddr) {
		t.Error("Expected valid address to be validated")
	}

	// Validate incorrect address
	if deployer.ValidateContractAddress(ContractCommunityFund, "invalid_address") {
		t.Error("Expected invalid address to fail validation")
	}

	// Validate wrong contract type
	integrityAddr, _ := deployer.GetIntegrityRewardAddress()
	if deployer.ValidateContractAddress(ContractCommunityFund, integrityAddr) {
		t.Error("Expected integrity address to fail community fund validation")
	}
}

// TestContractDeployer_DeployTxHash tests deployment transaction hash retrieval
func TestContractDeployer_DeployTxHash(t *testing.T) {
	deployer := NewContractDeployer("genesis_hash_123", 1)
	deployer.DeployContracts(0)

	// Get community fund tx hash
	txHash, err := deployer.GetDeployTxHash(ContractCommunityFund)
	if err != nil {
		t.Fatalf("Failed to get deploy tx hash: %v", err)
	}

	if txHash == "" {
		t.Error("Expected non-empty deploy tx hash")
	}

	// Get integrity reward tx hash
	txHash2, err := deployer.GetDeployTxHash(ContractIntegrityReward)
	if err != nil {
		t.Fatalf("Failed to get deploy tx hash: %v", err)
	}

	if txHash2 == "" {
		t.Error("Expected non-empty deploy tx hash")
	}

	// Hashes should be different
	if txHash == txHash2 {
		t.Error("Expected different tx hashes for different contracts")
	}
}

// TestContractRegistry_Initialization tests registry initialization
func TestContractRegistry_Initialization(t *testing.T) {
	// Create a fresh registry for this test
	registry := &ContractRegistry{}

	if registry.IsInitialized() {
		t.Error("Expected registry to not be initialized initially")
	}

	// Deployer should be nil initially
	if registry.GetDeployer() != nil {
		t.Error("Expected deployer to be nil before initialization")
	}

	// Initialize with deployer
	deployer := NewContractDeployer("genesis_hash", 1)
	registry.Initialize(deployer)

	if registry.GetDeployer() == nil {
		t.Error("Expected deployer to be set after initialization")
	}

	if !registry.IsInitialized() {
		t.Error("Expected registry to be initialized")
	}
}

// TestContractRegistry_ContractAccess tests contract access through registry
func TestContractRegistry_ContractAccess(t *testing.T) {
	registry := GetContractRegistry()

	// Initialize with deployer
	deployer := NewContractDeployer("genesis_hash", 1)
	deployer.DeployContracts(0)
	registry.Initialize(deployer)

	// Get community fund contract
	communityContract, err := registry.GetCommunityFundContract()
	if err != nil {
		t.Fatalf("Failed to get community fund contract: %v", err)
	}

	if communityContract == nil {
		t.Fatal("Expected non-nil community fund contract")
	}

	// Get integrity reward contract
	integrityContract, err := registry.GetIntegrityRewardContract()
	if err != nil {
		t.Fatalf("Failed to get integrity reward contract: %v", err)
	}

	if integrityContract == nil {
		t.Fatal("Expected non-nil integrity reward contract")
	}
}

// TestContractRegistry_DeploymentCheck tests deployment status check
func TestContractRegistry_DeploymentCheck(t *testing.T) {
	// Create a fresh registry for this test
	registry := &ContractRegistry{}

	// Should not be deployed initially
	if registry.AreContractsDeployed() {
		t.Error("Expected contracts to not be deployed initially")
	}

	// Initialize and deploy
	deployer := NewContractDeployer("genesis_hash", 1)
	deployer.DeployContracts(0)
	registry.Initialize(deployer)

	// Should be deployed now
	if !registry.AreContractsDeployed() {
		t.Error("Expected contracts to be deployed")
	}
}

// TestContractDeployer_IsDeployed tests IsDeployed method
func TestContractDeployer_IsDeployed(t *testing.T) {
	deployer := NewContractDeployer("genesis_hash_123", 1)

	// Initially not deployed
	if deployer.IsDeployed(ContractCommunityFund) {
		t.Error("Expected community fund to not be deployed initially")
	}

	if deployer.IsDeployed(ContractIntegrityReward) {
		t.Error("Expected integrity reward to not be deployed initially")
	}

	// Deploy contracts
	deployer.DeployContracts(0)

	// Now should be deployed
	if !deployer.IsDeployed(ContractCommunityFund) {
		t.Error("Expected community fund to be deployed")
	}

	if !deployer.IsDeployed(ContractIntegrityReward) {
		t.Error("Expected integrity reward to be deployed")
	}
}

// TestContractDeployer_ConcurrentAccess tests thread safety
func TestContractDeployer_ConcurrentAccess(t *testing.T) {
	deployer := NewContractDeployer("genesis_hash_123", 1)
	deployer.DeployContracts(0)

	done := make(chan bool)

	// Concurrent reads
	go func() {
		for i := 0; i < 50; i++ {
			_, _ = deployer.GetCommunityFundAddress()
			_, _ = deployer.GetIntegrityRewardAddress()
			_ = deployer.AreAllDeployed()
			_ = deployer.GetActiveContractCount()
		}
		done <- true
	}()

	// Concurrent deployment info retrieval
	go func() {
		for i := 0; i < 50; i++ {
			_, _ = deployer.GetDeploymentInfo(ContractCommunityFund)
			_, _ = deployer.GetDeploymentInfo(ContractIntegrityReward)
			_ = deployer.GetAllDeployments()
		}
		done <- true
	}()

	// Concurrent validation
	go func() {
		for i := 0; i < 50; i++ {
			addr, _ := deployer.GetCommunityFundAddress()
			_ = deployer.ValidateContractAddress(ContractCommunityFund, addr)
			_ = deployer.IsDeployed(ContractCommunityFund)
		}
		done <- true
	}()

	<-done
	<-done
	<-done
}
