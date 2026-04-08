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
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// ProposalType represents the type of community fund proposal
type ProposalType uint8

const (
	// ProposalTreasury represents treasury fund allocation
	ProposalTreasury ProposalType = iota
	// ProposalEcosystem represents ecosystem development funding
	ProposalEcosystem
	// ProposalGrant represents grant program
	ProposalGrant
	// ProposalEvent represents community event funding
	ProposalEvent
)

// String returns the string representation of proposal type
func (p ProposalType) String() string {
	switch p {
	case ProposalTreasury:
		return "treasury"
	case ProposalEcosystem:
		return "ecosystem"
	case ProposalGrant:
		return "grant"
	case ProposalEvent:
		return "event"
	default:
		return "unknown"
	}
}

// ProposalStatus represents the status of a proposal
type ProposalStatus uint8

const (
	// StatusActive represents active proposal (voting in progress)
	StatusActive ProposalStatus = iota
	// StatusPassed represents passed proposal (ready for execution)
	StatusPassed
	// StatusRejected represents rejected proposal
	StatusRejected
	// StatusExecuted represents executed proposal
	StatusExecuted
	// StatusExpired represents expired proposal
	StatusExpired
)

// String returns the string representation of proposal status
func (s ProposalStatus) String() string {
	switch s {
	case StatusActive:
		return "active"
	case StatusPassed:
		return "passed"
	case StatusRejected:
		return "rejected"
	case StatusExecuted:
		return "executed"
	case StatusExpired:
		return "expired"
	default:
		return "unknown"
	}
}

// Vote represents a vote on a proposal
type Vote struct {
	// Voter is the voter's address
	Voter string `json:"voter"`
	// Support is true if the vote is in favor
	Support bool `json:"support"`
	// VotingPower is the voting power (1 token = 1 vote)
	VotingPower uint64 `json:"votingPower"`
	// Timestamp is when the vote was cast
	Timestamp int64 `json:"timestamp"`
}

// Proposal represents a community fund proposal
type Proposal struct {
	// ID is the unique proposal identifier
	ID string `json:"id"`
	// Proposer is the proposer's address
	Proposer string `json:"proposer"`
	// Title is the proposal title
	Title string `json:"title"`
	// Description is the proposal description
	Description string `json:"description"`
	// Type is the proposal type
	Type ProposalType `json:"type"`
	// Amount is the requested funding amount in wei
	Amount uint64 `json:"amount"`
	// Recipient is the fund recipient address
	Recipient string `json:"recipient"`
	// Deposit is the proposal deposit (prevents spam)
	Deposit uint64 `json:"deposit"`
	// CreatedAt is the proposal creation timestamp
	CreatedAt int64 `json:"createdAt"`
	// VotingEndTime is when voting ends
	VotingEndTime int64 `json:"votingEndTime"`
	// VotesFor is the total votes in favor
	VotesFor uint64 `json:"votesFor"`
	// VotesAgainst is the total votes against
	VotesAgainst uint64 `json:"votesAgainst"`
	// Voters is the map of voter address to vote
	Voters map[string]Vote `json:"voters"`
	// Status is the current proposal status
	Status ProposalStatus `json:"status"`
	// ExecutedAt is the execution timestamp (if executed)
	ExecutedAt int64 `json:"executedAt"`
}

// CommunityFundGovernanceContract represents the community fund governance contract
// Production-grade: pure on-chain governance, community-controlled
type CommunityFundGovernanceContract struct {
	mu sync.RWMutex

	// ContractAddress is the contract address (auto-generated on deployment)
	ContractAddress string `json:"contractAddress"`
	// DeployedAt is the deployment timestamp
	DeployedAt int64 `json:"deployedAt"`
	// FundBalance is the current fund balance in wei
	FundBalance uint64 `json:"fundBalance"`
	// Proposals is the map of proposal ID to proposal
	Proposals map[string]*Proposal `json:"proposals"`
	// ProposalCount is the total number of proposals
	ProposalCount uint64 `json:"proposalCount"`
	// VotingPeriod is the voting period in seconds (default: 7 days)
	VotingPeriod int64 `json:"votingPeriod"`
	// MinimumDeposit is the minimum proposal deposit in wei
	MinimumDeposit uint64 `json:"minimumDeposit"`
	// QuorumPercent is the minimum voting participation required (10%)
	QuorumPercent uint8 `json:"quorumPercent"`
	// ApprovalThreshold is the approval threshold (60%)
	ApprovalThreshold uint8 `json:"approvalThreshold"`
	// TotalVotingPower is the total voting power in the system
	TotalVotingPower uint64 `json:"totalVotingPower"`
	// ExecutedProposals is the number of executed proposals
	ExecutedProposals uint64 `json:"executedProposals"`
}

// NewCommunityFundGovernanceContract creates a new community fund governance contract
// Contract address is auto-generated during deployment, cannot be manually created
// Address format: NOGO + version(1) + hash(32) + checksum(4) = 78 characters total
func NewCommunityFundGovernanceContract() *CommunityFundGovernanceContract {
	// Generate unique contract address using timestamp and random data
	// Format matches wallet address: NOGO + version byte + 32-byte hash + 4-byte checksum
	timestamp := time.Now().UnixNano()
	data := []byte(fmt.Sprintf("%d-COMMUNITY_FUND_GOVERNANCE", timestamp))
	
	// Generate 32-byte hash
	hash := sha256.Sum256(data)
	
	// Build address: version byte (0x00) + 32-byte hash
	addressData := make([]byte, 1+32)
	addressData[0] = 0x00 // Version byte
	copy(addressData[1:], hash[:32])
	
	// Calculate 4-byte checksum
	checksumHash := sha256.Sum256(addressData)
	addressData = append(addressData, checksumHash[:4]...)
	
	// Encode to hex and add prefix
	contractAddress := "NOGO" + hex.EncodeToString(addressData)

	return &CommunityFundGovernanceContract{
		ContractAddress:   contractAddress,
		DeployedAt:        time.Now().Unix(),
		FundBalance:       0,
		Proposals:         make(map[string]*Proposal),
		ProposalCount:     0,
		VotingPeriod:      7 * 24 * 60 * 60, // 7 days in seconds
		MinimumDeposit:    1000000000,       // 10 NOGO (1 NOGO = 10^8 wei)
		QuorumPercent:     10,               // 10% quorum required
		ApprovalThreshold: 60,               // 60% approval required
		TotalVotingPower:  0,
		ExecutedProposals: 0,
	}
}

// GetContractAddress returns the contract address (read-only)
// This address is auto-generated and cannot be manually created
func (c *CommunityFundGovernanceContract) GetContractAddress() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ContractAddress
}

// GetFundBalance returns the current fund balance (read-only)
func (c *CommunityFundGovernanceContract) GetFundBalance() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.FundBalance
}

// AddFunds adds funds to the community fund (from block rewards or donations)
// Thread-safe with mutex protection
func (c *CommunityFundGovernanceContract) AddFunds(amount uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Add with overflow protection
	if c.FundBalance > ^uint64(0)-amount {
		c.FundBalance = ^uint64(0)
	} else {
		c.FundBalance += amount
	}
}

// CreateProposal creates a new community fund proposal
// Requires deposit to prevent spam
// Returns proposal ID or error
func (c *CommunityFundGovernanceContract) CreateProposal(
	proposer string,
	title string,
	description string,
	propType ProposalType,
	amount uint64,
	recipient string,
	deposit uint64,
) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate deposit
	if deposit < c.MinimumDeposit {
		return "", errors.New("deposit below minimum requirement")
	}

	// Validate amount
	if amount == 0 {
		return "", errors.New("amount must be greater than 0")
	}

	// Validate recipient address
	if recipient == "" {
		return "", errors.New("recipient address required")
	}

	// Generate unique proposal ID using timestamp + random data
	// This ensures uniqueness even after node restart
	now := time.Now().UnixNano()
	randomData := make([]byte, 8)
	if _, err := rand.Read(randomData); err != nil {
		// Fallback to time-based if crypto rand fails
		for i := 0; i < 8; i++ {
			randomData[i] = byte(now >> (i * 8))
		}
	}
	hashData := append([]byte(proposer+title+description), randomData...)
	hashData = append(hashData, []byte(fmt.Sprintf("%d", now))...)
	hash := sha256.Sum256(hashData)
	proposalID := hex.EncodeToString(hash[:])

	createdAt := time.Now().Unix()
	proposal := &Proposal{
		ID:            proposalID,
		Proposer:      proposer,
		Title:         title,
		Description:   description,
		Type:          propType,
		Amount:        amount,
		Recipient:     recipient,
		Deposit:       deposit,
		CreatedAt:     createdAt,
		VotingEndTime: createdAt + c.VotingPeriod,
		VotesFor:      0,
		VotesAgainst:  0,
		Voters:        make(map[string]Vote),
		Status:        StatusActive,
		ExecutedAt:    0,
	}

	c.Proposals[proposalID] = proposal

	return proposalID, nil
}

// Vote casts a vote on a proposal
// 1 token = 1 vote
// Thread-safe with mutex protection
func (c *CommunityFundGovernanceContract) Vote(
	proposalID string,
	voter string,
	support bool,
	votingPower uint64,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Get proposal
	proposal, exists := c.Proposals[proposalID]
	if !exists {
		return errors.New("proposal not found")
	}

	// Check if voting is still active
	if proposal.Status != StatusActive {
		return errors.New("voting is not active for this proposal")
	}

	// Check if voting period has ended
	if time.Now().Unix() > proposal.VotingEndTime {
		proposal.Status = StatusExpired
		return errors.New("voting period has ended")
	}

	// Check if voter already voted
	if _, alreadyVoted := proposal.Voters[voter]; alreadyVoted {
		return errors.New("voter has already voted")
	}

	// Validate voting power
	if votingPower == 0 {
		return errors.New("voting power must be greater than 0")
	}

	// Cast vote
	vote := Vote{
		Voter:       voter,
		Support:     support,
		VotingPower: votingPower,
		Timestamp:   time.Now().Unix(),
	}

	proposal.Voters[voter] = vote

	if support {
		proposal.VotesFor += votingPower
	} else {
		proposal.VotesAgainst += votingPower
	}

	return nil
}

// FinalizeVoting finalizes voting for a proposal
// Checks if proposal passes or fails
// Thread-safe with mutex protection
func (c *CommunityFundGovernanceContract) FinalizeVoting(proposalID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	proposal, exists := c.Proposals[proposalID]
	if !exists {
		return errors.New("proposal not found")
	}

	// Check if voting period has ended
	if time.Now().Unix() <= proposal.VotingEndTime {
		return errors.New("voting period has not ended yet")
	}

	// Check if already finalized
	if proposal.Status != StatusActive {
		return errors.New("proposal already finalized")
	}

	// Calculate total votes
	totalVotes := proposal.VotesFor + proposal.VotesAgainst

	// Check quorum (minimum 10% participation)
	quorumVotes := c.TotalVotingPower * uint64(c.QuorumPercent) / 100
	if totalVotes < quorumVotes {
		proposal.Status = StatusRejected
		// Refund deposit
		// In production, this would transfer back to proposer
		return nil
	}

	// Check approval threshold (minimum 60% approval)
	if proposal.VotesFor == 0 {
		proposal.Status = StatusRejected
		return nil
	}

	approvalPercent := (proposal.VotesFor * 100) / totalVotes
	if approvalPercent >= uint64(c.ApprovalThreshold) {
		proposal.Status = StatusPassed
	} else {
		proposal.Status = StatusRejected
	}

	return nil
}

// ExecuteProposal executes a passed proposal
// Transfers funds to recipient
// Thread-safe with mutex protection
func (c *CommunityFundGovernanceContract) ExecuteProposal(proposalID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	proposal, exists := c.Proposals[proposalID]
	if !exists {
		return errors.New("proposal not found")
	}

	// Check if proposal passed
	if proposal.Status != StatusPassed {
		return errors.New("proposal has not passed")
	}

	// Check if fund has sufficient balance
	if c.FundBalance < proposal.Amount {
		return errors.New("insufficient fund balance")
	}

	// Execute transfer (in production, this would be an actual blockchain transfer)
	c.FundBalance -= proposal.Amount
	proposal.Status = StatusExecuted
	proposal.ExecutedAt = time.Now().Unix()
	c.ExecutedProposals++

	// Return deposit to proposer (in production, this would be a transfer)
	// proposal.Deposit would be transferred back to proposal.Proposer

	return nil
}

// GetAllProposals returns all proposals (read-only copies)
func (c *CommunityFundGovernanceContract) GetAllProposals() []Proposal {
	c.mu.RLock()
	defer c.mu.RUnlock()

	proposals := make([]Proposal, 0, len(c.Proposals))
	for _, proposal := range c.Proposals {
		// Create a copy to prevent external modification
		proposalCopy := *proposal
		proposalCopy.Voters = make(map[string]Vote)
		for voter, vote := range proposal.Voters {
			proposalCopy.Voters[voter] = vote
		}
		proposals = append(proposals, proposalCopy)
	}

	return proposals
}

// GetProposalByID returns a proposal by ID (read-only copy)
func (c *CommunityFundGovernanceContract) GetProposalByID(proposalID string) *Proposal {
	c.mu.RLock()
	defer c.mu.RUnlock()

	proposal, exists := c.Proposals[proposalID]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	proposalCopy := *proposal
	proposalCopy.Voters = make(map[string]Vote)
	for voter, vote := range proposal.Voters {
		proposalCopy.Voters[voter] = vote
	}

	return &proposalCopy
}

// GetProposal returns a proposal by ID (read-only copy)
func (c *CommunityFundGovernanceContract) GetProposal(proposalID string) (*Proposal, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	proposal, exists := c.Proposals[proposalID]
	if !exists {
		return nil, errors.New("proposal not found")
	}

	// Return a copy to prevent external modification
	proposalCopy := *proposal
	proposalCopy.Voters = make(map[string]Vote)
	for k, v := range proposal.Voters {
		proposalCopy.Voters[k] = v
	}

	return &proposalCopy, nil
}

// GetActiveProposals returns all active proposals
func (c *CommunityFundGovernanceContract) GetActiveProposals() []*Proposal {
	c.mu.RLock()
	defer c.mu.RUnlock()

	proposals := make([]*Proposal, 0)
	for _, proposal := range c.Proposals {
		if proposal.Status == StatusActive {
			proposals = append(proposals, proposal)
		}
	}

	return proposals
}

// GetPassedProposals returns all passed proposals ready for execution
func (c *CommunityFundGovernanceContract) GetPassedProposals() []*Proposal {
	c.mu.RLock()
	defer c.mu.RUnlock()

	proposals := make([]*Proposal, 0)
	for _, proposal := range c.Proposals {
		if proposal.Status == StatusPassed {
			proposals = append(proposals, proposal)
		}
	}

	return proposals
}

// GetExecutedProposals returns all executed proposals
func (c *CommunityFundGovernanceContract) GetExecutedProposals() []*Proposal {
	c.mu.RLock()
	defer c.mu.RUnlock()

	proposals := make([]*Proposal, 0)
	for _, proposal := range c.Proposals {
		if proposal.Status == StatusExecuted {
			proposals = append(proposals, proposal)
		}
	}

	return proposals
}

// SetTotalVotingPower sets the total voting power in the system
// Called by the blockchain when token supply changes
func (c *CommunityFundGovernanceContract) SetTotalVotingPower(total uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.TotalVotingPower = total
}

// GetVotingStats returns voting statistics for a proposal
type VotingStats struct {
	TotalVotes           uint64 `json:"totalVotes"`
	VotesFor             uint64 `json:"votesFor"`
	VotesAgainst         uint64 `json:"votesAgainst"`
	ParticipationPercent uint8  `json:"participationPercent"`
	ApprovalPercent      uint8  `json:"approvalPercent"`
	QuorumReached        bool   `json:"quorumReached"`
	ThresholdReached     bool   `json:"thresholdReached"`
}

// GetVotingStats returns voting statistics for a proposal
func (c *CommunityFundGovernanceContract) GetVotingStats(proposalID string) (*VotingStats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	proposal, exists := c.Proposals[proposalID]
	if !exists {
		return nil, errors.New("proposal not found")
	}

	totalVotes := proposal.VotesFor + proposal.VotesAgainst

	// Calculate participation percentage
	var participationPercent uint8
	if c.TotalVotingPower > 0 {
		participationPercent = uint8((totalVotes * 100) / c.TotalVotingPower)
	}

	// Calculate approval percentage
	var approvalPercent uint8
	if totalVotes > 0 {
		approvalPercent = uint8((proposal.VotesFor * 100) / totalVotes)
	}

	// Check if quorum reached
	quorumVotes := c.TotalVotingPower * uint64(c.QuorumPercent) / 100
	quorumReached := totalVotes >= quorumVotes

	// Check if threshold reached
	thresholdReached := approvalPercent >= c.ApprovalThreshold

	return &VotingStats{
		TotalVotes:           totalVotes,
		VotesFor:             proposal.VotesFor,
		VotesAgainst:         proposal.VotesAgainst,
		ParticipationPercent: participationPercent,
		ApprovalPercent:      approvalPercent,
		QuorumReached:        quorumReached,
		ThresholdReached:     thresholdReached,
	}, nil
}

// GetProposalCount returns the total number of proposals
func (c *CommunityFundGovernanceContract) GetProposalCount() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ProposalCount
}

// GetExecutedCount returns the number of executed proposals
func (c *CommunityFundGovernanceContract) GetExecutedCount() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ExecutedProposals
}

// HasVoted checks if a voter has already voted on a proposal
func (c *CommunityFundGovernanceContract) HasVoted(proposalID string, voter string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	proposal, exists := c.Proposals[proposalID]
	if !exists {
		return false
	}

	_, voted := proposal.Voters[voter]
	return voted
}

// CanExecute checks if a proposal can be executed
func (c *CommunityFundGovernanceContract) CanExecute(proposalID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	proposal, exists := c.Proposals[proposalID]
	if !exists {
		return false
	}

	return proposal.Status == StatusPassed && c.FundBalance >= proposal.Amount
}

// ToJSON serializes the contract state to JSON
func (c *CommunityFundGovernanceContract) ToJSON() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	type ContractState struct {
		ContractAddress   string             `json:"contractAddress"`
		DeployedAt        int64              `json:"deployedAt"`
		FundBalance       uint64             `json:"fundBalance"`
		Proposals         []*Proposal        `json:"proposals"`
		ProposalCount     uint64             `json:"proposalCount"`
		VotingPeriod      int64              `json:"votingPeriod"`
		MinimumDeposit    uint64             `json:"minimumDeposit"`
		QuorumPercent     uint8              `json:"quorumPercent"`
		ApprovalThreshold uint8              `json:"approvalThreshold"`
		TotalVotingPower  uint64             `json:"totalVotingPower"`
		ExecutedProposals uint64             `json:"executedProposals"`
	}

	state := &ContractState{
		ContractAddress:   c.ContractAddress,
		DeployedAt:        c.DeployedAt,
		FundBalance:       c.FundBalance,
		Proposals:         make([]*Proposal, 0, len(c.Proposals)),
		ProposalCount:     c.ProposalCount,
		VotingPeriod:      c.VotingPeriod,
		MinimumDeposit:    c.MinimumDeposit,
		QuorumPercent:     c.QuorumPercent,
		ApprovalThreshold: c.ApprovalThreshold,
		TotalVotingPower:  c.TotalVotingPower,
		ExecutedProposals: c.ExecutedProposals,
	}

	for _, proposal := range c.Proposals {
		state.Proposals = append(state.Proposals, proposal)
	}

	return json.Marshal(state)
}

// FromJSON deserializes the contract state from JSON
func (c *CommunityFundGovernanceContract) FromJSON(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	type ContractState struct {
		ContractAddress   string             `json:"contractAddress"`
		DeployedAt        int64              `json:"deployedAt"`
		FundBalance       uint64             `json:"fundBalance"`
		Proposals         []*Proposal        `json:"proposals"`
		ProposalCount     uint64             `json:"proposalCount"`
		VotingPeriod      int64              `json:"votingPeriod"`
		MinimumDeposit    uint64             `json:"minimumDeposit"`
		QuorumPercent     uint8              `json:"quorumPercent"`
		ApprovalThreshold uint8              `json:"approvalThreshold"`
		TotalVotingPower  uint64             `json:"totalVotingPower"`
		ExecutedProposals uint64             `json:"executedProposals"`
	}

	var state ContractState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("unmarshal contract state: %w", err)
	}

	c.ContractAddress = state.ContractAddress
	c.DeployedAt = state.DeployedAt
	c.FundBalance = state.FundBalance
	c.Proposals = make(map[string]*Proposal)
	for _, proposal := range state.Proposals {
		c.Proposals[proposal.ID] = proposal
	}
	c.ProposalCount = state.ProposalCount
	c.VotingPeriod = state.VotingPeriod
	c.MinimumDeposit = state.MinimumDeposit
	c.QuorumPercent = state.QuorumPercent
	c.ApprovalThreshold = state.ApprovalThreshold
	c.TotalVotingPower = state.TotalVotingPower
	c.ExecutedProposals = state.ExecutedProposals

	return nil
}

// SaveToFile saves the contract state to a file
func (c *CommunityFundGovernanceContract) SaveToFile(filePath string) error {
	data, err := c.ToJSON()
	if err != nil {
		return fmt.Errorf("serialize contract: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("write contract file: %w", err)
	}

	return nil
}

// LoadFromFile loads the contract state from a file
func (c *CommunityFundGovernanceContract) LoadFromFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, start with empty state
		}
		return fmt.Errorf("read contract file: %w", err)
	}

	if err := c.FromJSON(data); err != nil {
		return fmt.Errorf("deserialize contract: %w", err)
	}

	return nil
}
