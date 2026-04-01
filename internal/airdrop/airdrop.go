package airdrop

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Config struct {
	StartTime        time.Time `json:"startTime"`
	EndTime          time.Time `json:"endTime"`
	TotalSupply      uint64    `json:"totalSupply"`
	PerAddress       uint64    `json:"perAddress"`
	MinHoldRequired  uint64    `json:"minHoldRequired"`
	MaxClaims        int       `json:"maxClaims"`
	VerifyHold       bool      `json:"verifyHold"`
	RequireEmail     bool      `json:"requireEmail"`
	AntiSybilEnabled bool      `json:"antiSybilEnabled"`
}

type ClaimRecord struct {
	Address   string    `json:"address"`
	Amount    uint64    `json:"amount"`
	ClaimTime time.Time `json:"claimTime"`
	IP        string    `json:"ip"`
	Email     string    `json:"email,omitempty"`
	TxHash    string    `json:"txHash,omitempty"`
	Verified  bool      `json:"verified"`
}

type Airdrop struct {
	config       Config
	claims       map[string]*ClaimRecord
	claimedTotal uint64
	mu           sync.RWMutex
	dataPath     string
}

func NewAirdrop(config Config, dataPath string) *Airdrop {
	a := &Airdrop{
		config:   config,
		claims:   make(map[string]*ClaimRecord),
		dataPath: dataPath,
	}

	if dataPath != "" {
		a.load()
	}

	return a
}

func (a *Airdrop) CanClaim(address string) (bool, string) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	now := time.Now()

	if now.Before(a.config.StartTime) {
		return false, "airdrop_not_started"
	}

	if now.After(a.config.EndTime) {
		return false, "airdrop_ended"
	}

	if a.config.MaxClaims > 0 && len(a.claims) >= a.config.MaxClaims {
		return false, "max_claims_reached"
	}

	if _, exists := a.claims[address]; exists {
		return false, "already_claimed"
	}

	return true, ""
}

func (a *Airdrop) Claim(address string, ip string, email string) (uint64, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	canClaim, reason := a.canClaimInternal(address)
	if !canClaim {
		return 0, reason, nil
	}

	claim := &ClaimRecord{
		Address:   address,
		Amount:    a.config.PerAddress,
		ClaimTime: time.Now(),
		IP:        ip,
		Email:     email,
	}

	a.claims[address] = claim
	a.claimedTotal += claim.Amount

	a.save()

	return claim.Amount, "", nil
}

func (a *Airdrop) canClaimInternal(address string) (bool, string) {
	now := time.Now()

	if now.Before(a.config.StartTime) {
		return false, "airdrop_not_started"
	}

	if now.After(a.config.EndTime) {
		return false, "airdrop_ended"
	}

	if a.config.MaxClaims > 0 && len(a.claims) >= a.config.MaxClaims {
		return false, "max_claims_reached"
	}

	if _, exists := a.claims[address]; exists {
		return false, "already_claimed"
	}

	return true, ""
}

func (a *Airdrop) GetClaim(address string) *ClaimRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.claims[address]
}

func (a *Airdrop) GetStats() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return map[string]interface{}{
		"totalClaims":     len(a.claims),
		"claimedTotal":    a.claimedTotal,
		"totalSupply":     a.config.TotalSupply,
		"remaining":       a.config.TotalSupply - a.claimedTotal,
		"maxClaims":       a.config.MaxClaims,
		"remainingClaims": a.config.MaxClaims - len(a.claims),
		"startTime":       a.config.StartTime,
		"endTime":         a.config.EndTime,
		"perAddress":      a.config.PerAddress,
	}
}

func (a *Airdrop) load() {
	if a.dataPath == "" {
		return
	}

	data, err := os.ReadFile(a.dataPath)
	if err != nil {
		return
	}

	var records map[string]*ClaimRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return
	}

	a.claims = records
	for _, claim := range records {
		a.claimedTotal += claim.Amount
	}
}

func (a *Airdrop) save() {
	if a.dataPath == "" {
		return
	}

	data, err := json.MarshalIndent(a.claims, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(a.dataPath, data, 0644)
}

func DefaultConfig() Config {
	return Config{
		StartTime:        time.Now(),
		EndTime:          time.Now().Add(30 * 24 * time.Hour),
		TotalSupply:      2100000,
		PerAddress:       100,
		MinHoldRequired:  0,
		MaxClaims:        10000,
		VerifyHold:       false,
		RequireEmail:     false,
		AntiSybilEnabled: true,
	}
}
