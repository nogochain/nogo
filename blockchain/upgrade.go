package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type ProtocolVersion struct {
	Major            uint32 `json:"major"`
	Minor            uint32 `json:"minor"`
	Patch            uint32 `json:"patch"`
	Name             string `json:"name"`
	ActivationHeight uint64 `json:"activationHeight"`
	Mandatory        bool   `json:"mandatory"`
	MinPeerVersion   string `json:"minPeerVersion"`
}

type UpgradeProposal struct {
	Version         ProtocolVersion `json:"version"`
	Description     string          `json:"description"`
	ProposedBy      string          `json:"proposedBy"`
	ProposedAt      time.Time       `json:"proposedAt"`
	VotesFor        uint64          `json:"votesFor"`
	VotesAgainst    uint64          `json:"votesAgainst"`
	Status          string          `json:"status"`
	ActivationBlock uint64          `json:"activationBlock"`
}

type ProtocolUpgrade struct {
	mu        sync.RWMutex
	current   ProtocolVersion
	proposals map[string]*UpgradeProposal
	history   []ProtocolVersion
	activated map[uint64]bool
}

func NewProtocolUpgrade() *ProtocolUpgrade {
	current := ProtocolVersion{
		Major:            1,
		Minor:            1,
		Patch:            0,
		Name:             "v1.1",
		ActivationHeight: 0,
		Mandatory:        true,
		MinPeerVersion:   "1.0.0",
	}

	return &ProtocolUpgrade{
		current:   current,
		proposals: make(map[string]*UpgradeProposal),
		history:   []ProtocolVersion{current},
		activated: make(map[uint64]bool),
	}
}

func (pu *ProtocolUpgrade) GetCurrentVersion() ProtocolVersion {
	pu.mu.RLock()
	defer pu.mu.RUnlock()
	return pu.current
}

func (pu *ProtocolUpgrade) ProposeUpgrade(version ProtocolVersion, description, proposer string, activationHeight uint64) (*UpgradeProposal, error) {
	pu.mu.Lock()
	defer pu.mu.Unlock()

	versionKey := fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)
	if _, exists := pu.proposals[versionKey]; exists {
		return nil, fmt.Errorf("upgrade proposal already exists")
	}

	if version.Major < pu.current.Major || (version.Major == pu.current.Major && version.Minor < pu.current.Minor) {
		return nil, fmt.Errorf("cannot downgrade protocol")
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

	pu.proposals[versionKey] = proposal
	return proposal, nil
}

func (pu *ProtocolUpgrade) Vote(versionKey string, approve bool, voter string) error {
	pu.mu.Lock()
	defer pu.mu.Unlock()

	proposal, exists := pu.proposals[versionKey]
	if !exists {
		return fmt.Errorf("proposal not found")
	}

	if proposal.Status != "pending" {
		return fmt.Errorf("proposal no longer pending")
	}

	if approve {
		proposal.VotesFor++
	} else {
		proposal.VotesAgainst++
	}

	return nil
}

func (pu *ProtocolUpgrade) ActivateProposal(versionKey string, currentHeight uint64) error {
	pu.mu.Lock()
	defer pu.mu.Unlock()

	proposal, exists := pu.proposals[versionKey]
	if !exists {
		return fmt.Errorf("proposal not found")
	}

	if proposal.ActivationBlock > currentHeight {
		return fmt.Errorf("activation height not reached")
	}

	proposal.Status = "activated"
	pu.current = proposal.Version
	pu.history = append(pu.history, proposal.Version)
	pu.activated[currentHeight] = true

	return nil
}

func (pu *ProtocolUpgrade) GetProposal(versionKey string) (*UpgradeProposal, bool) {
	pu.mu.RLock()
	defer pu.mu.RUnlock()

	proposal, exists := pu.proposals[versionKey]
	return proposal, exists
}

func (pu *ProtocolUpgrade) ListProposals() []*UpgradeProposal {
	pu.mu.RLock()
	defer pu.mu.RUnlock()

	var result []*UpgradeProposal
	for _, p := range pu.proposals {
		result = append(result, p)
	}
	return result
}

func (pu *ProtocolUpgrade) IsUpgradeRequired(peerVersion string) bool {
	pu.mu.RLock()
	defer pu.mu.RUnlock()

	return peerVersion < pu.current.MinPeerVersion
}

func (pu *ProtocolUpgrade) GetMinRequiredVersion() string {
	pu.mu.RLock()
	defer pu.mu.RUnlock()

	return pu.current.MinPeerVersion
}

func (pu *ProtocolUpgrade) GetVersionString() string {
	pu.mu.RLock()
	defer pu.mu.RUnlock()

	return fmt.Sprintf("%d.%d.%d", pu.current.Major, pu.current.Minor, pu.current.Patch)
}

func (pu *ProtocolUpgrade) ShouldSignalUpgrade(height uint64) bool {
	pu.mu.RLock()
	activated := pu.activated[height]
	pu.mu.RUnlock()

	return activated
}

type UpgradeJSON struct {
	Current   ProtocolVersion    `json:"current"`
	Proposals []*UpgradeProposal `json:"proposals"`
	History   []ProtocolVersion  `json:"history"`
}

func (pu *ProtocolUpgrade) MarshalJSON() ([]byte, error) {
	pu.mu.RLock()
	defer pu.mu.RUnlock()

	return json.Marshal(UpgradeJSON{
		Current:   pu.current,
		Proposals: pu.ListProposals(),
		History:   pu.history,
	})
}

func (pu *ProtocolUpgrade) CanConnect(peerVersion string) bool {
	return !pu.IsUpgradeRequired(peerVersion)
}

func (pu *ProtocolUpgrade) GetUpgradeInfo(height uint64) map[string]any {
	pu.mu.RLock()
	defer pu.mu.RUnlock()

	info := map[string]any{
		"currentVersion":  pu.GetVersionString(),
		"minPeerVersion":  pu.current.MinPeerVersion,
		"isUpgradeActive": pu.activated[height],
	}

	pendingUpgrades := make([]string, 0)
	for _, p := range pu.proposals {
		if p.Status == "pending" {
			pendingUpgrades = append(pendingUpgrades, fmt.Sprintf("%d.%d.%d", p.Version.Major, p.Version.Minor, p.Version.Patch))
		}
	}
	info["pendingUpgrades"] = pendingUpgrades

	return info
}
