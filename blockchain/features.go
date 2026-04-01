package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

type DNSDatabase struct {
	mu          sync.RWMutex
	domains     map[string]*DNSRecord
	ownerIndex  map[string][]string
	expiryIndex map[string]time.Time
}

type DNSRecord struct {
	Name     string
	Owner    string
	Resolver string
	TTL      uint64
	Created  time.Time
	Expires  time.Time
	Data     map[string]string
}

type DNSResolver struct {
	db *DNSDatabase
}

func NewDNSDatabase() *DNSDatabase {
	return &DNSDatabase{
		domains:     make(map[string]*DNSRecord),
		ownerIndex:  make(map[string][]string),
		expiryIndex: make(map[string]time.Time),
	}
}

func (db *DNSDatabase) RegisterDomain(name string, owner string, resolver string, ttl uint64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	name = strings.ToLower(name)
	if !isValidDomainName(name) {
		return fmt.Errorf("invalid domain name")
	}

	if _, exists := db.domains[name]; exists {
		return fmt.Errorf("domain already registered")
	}

	now := time.Now()
	expires := now.Add(time.Duration(ttl) * time.Second)

	record := &DNSRecord{
		Name:     name,
		Owner:    owner,
		Resolver: resolver,
		TTL:      ttl,
		Created:  now,
		Expires:  expires,
		Data:     make(map[string]string),
	}

	db.domains[name] = record
	db.ownerIndex[owner] = append(db.ownerIndex[owner], name)
	db.expiryIndex[name] = expires

	return nil
}

func (db *DNSDatabase) Resolve(name string) (*DNSRecord, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	name = strings.ToLower(name)
	record, exists := db.domains[name]
	if !exists {
		return nil, fmt.Errorf("domain not found")
	}

	if time.Now().After(record.Expires) {
		return nil, fmt.Errorf("domain expired")
	}

	return record, nil
}

func (db *DNSDatabase) UpdateResolver(name string, resolver string, owner string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	name = strings.ToLower(name)
	record, exists := db.domains[name]
	if !exists {
		return fmt.Errorf("domain not found")
	}

	if record.Owner != owner {
		return fmt.Errorf("not authorized")
	}

	record.Resolver = resolver
	return nil
}

func (db *DNSDatabase) SetRecordData(name string, recordType string, data string, owner string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	name = strings.ToLower(name)
	record, exists := db.domains[name]
	if !exists {
		return fmt.Errorf("domain not found")
	}

	if record.Owner != owner {
		return fmt.Errorf("not authorized")
	}

	record.Data[recordType] = data
	return nil
}

func (db *DNSDatabase) TransferDomain(name string, newOwner string, currentOwner string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	name = strings.ToLower(name)
	record, exists := db.domains[name]
	if !exists {
		return fmt.Errorf("domain not found")
	}

	if record.Owner != currentOwner {
		return fmt.Errorf("not authorized")
	}

	oldOwner := record.Owner
	record.Owner = newOwner

	db.ownerIndex[oldOwner] = removeString(db.ownerIndex[oldOwner], name)
	db.ownerIndex[newOwner] = append(db.ownerIndex[newOwner], name)

	return nil
}

func (db *DNSDatabase) RenewDomain(name string, owner string, ttl uint64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	record, exists := db.domains[name]
	if !exists {
		return fmt.Errorf("domain not found")
	}

	if record.Owner != owner {
		return fmt.Errorf("not authorized")
	}

	record.TTL = ttl
	record.Expires = time.Now().Add(time.Duration(ttl) * time.Second)
	db.expiryIndex[name] = record.Expires

	return nil
}

func (db *DNSDatabase) GetOwnerDomains(owner string) []string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.ownerIndex[owner]
}

func (db *DNSDatabase) GetAllDomains() []*DNSRecord {
	db.mu.RLock()
	defer db.mu.RUnlock()

	records := make([]*DNSRecord, 0, len(db.domains))
	for _, record := range db.domains {
		if time.Now().Before(record.Expires) {
			records = append(records, record)
		}
	}
	return records
}

func (db *DNSDatabase) GetExpiredDomains() []string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var expired []string
	now := time.Now()
	for name, expires := range db.expiryIndex {
		if now.After(expires) {
			expired = append(expired, name)
		}
	}
	return expired
}

func (db *DNSDatabase) CleanupExpired() int {
	db.mu.Lock()
	defer db.mu.Unlock()

	count := 0
	now := time.Now()
	for name, record := range db.domains {
		if now.After(record.Expires) {
			delete(db.domains, name)
			delete(db.expiryIndex, name)
			db.ownerIndex[record.Owner] = removeString(db.ownerIndex[record.Owner], name)
			count++
		}
	}
	return count
}

func isValidDomainName(name string) bool {
	if len(name) < 3 || len(name) > 64 {
		return false
	}

	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return false
	}

	for _, part := range parts {
		if len(part) == 0 || len(part) > 63 {
			return false
		}
		if part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for _, c := range part {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}

	return true
}

func generateDomainHash(name string) string {
	h := sha256.Sum256([]byte(name))
	return hex.EncodeToString(h[:])[:16]
}

func removeString(slice []string, item string) []string {
	result := []string{}
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

type GovernanceSystem struct {
	mu          sync.RWMutex
	proposals   map[string]*Proposal
	votes       map[string]map[string]*Vote
	voterWeight map[string]uint64
}

type Proposal struct {
	ID          string
	Title       string
	Description string
	Author      string
	Type        ProposalType
	Target      string
	Value       string
	StartTime   time.Time
	EndTime     time.Time
	YesVotes    uint64
	NoVotes     uint64
	Status      ProposalStatus
	Executed    bool
}

type Vote struct {
	Voter  string
	Choice bool
	Weight uint64
	Time   time.Time
}

type ProposalType string

const (
	ProposalTypeParameter ProposalType = "parameter"
	ProposalTypeUpgrade   ProposalType = "upgrade"
	ProposalTypeTreasury  ProposalType = "treasury"
	ProposalTypeDNS       ProposalType = "dns"
	ProposalTypeEmergency ProposalType = "emergency"
)

type ProposalStatus string

const (
	ProposalStatusPending  ProposalStatus = "pending"
	ProposalStatusActive   ProposalStatus = "active"
	ProposalStatusPassed   ProposalStatus = "passed"
	ProposalStatusFailed   ProposalStatus = "failed"
	ProposalStatusExecuted ProposalStatus = "executed"
)

func NewGovernanceSystem() *GovernanceSystem {
	return &GovernanceSystem{
		proposals:   make(map[string]*Proposal),
		votes:       make(map[string]map[string]*Vote),
		voterWeight: make(map[string]uint64),
	}
}

func (gs *GovernanceSystem) CreateProposal(title, desc, author, pType, target, value string, duration time.Duration) (*Proposal, error) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	proposalID := generateProposalID(title, author)

	now := time.Now()
	proposal := &Proposal{
		ID:          proposalID,
		Title:       title,
		Description: desc,
		Author:      author,
		Type:        ProposalType(pType),
		Target:      target,
		Value:       value,
		StartTime:   now,
		EndTime:     now.Add(duration),
		Status:      ProposalStatusActive,
	}

	gs.proposals[proposalID] = proposal
	gs.votes[proposalID] = make(map[string]*Vote)

	return proposal, nil
}

func (gs *GovernanceSystem) CastVote(proposalID, voter string, choice bool, weight uint64) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	proposal, exists := gs.proposals[proposalID]
	if !exists {
		return fmt.Errorf("proposal not found")
	}

	if time.Now().Before(proposal.StartTime) || time.Now().After(proposal.EndTime) {
		return fmt.Errorf("voting period closed")
	}

	if choice {
		proposal.YesVotes += weight
	} else {
		proposal.NoVotes += weight
	}

	gs.votes[proposalID][voter] = &Vote{
		Voter:  voter,
		Choice: choice,
		Weight: weight,
		Time:   time.Now(),
	}

	gs.voterWeight[voter] += weight

	return nil
}

func (gs *GovernanceSystem) TallyProposal(proposalID string) (ProposalStatus, error) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	proposal, exists := gs.proposals[proposalID]
	if !exists {
		return "", fmt.Errorf("proposal not found")
	}

	if time.Now().Before(proposal.EndTime) {
		return "", fmt.Errorf("voting period not ended")
	}

	quorum := uint64(1000000)
	totalVotes := proposal.YesVotes + proposal.NoVotes

	if totalVotes < quorum {
		proposal.Status = ProposalStatusFailed
		return ProposalStatusFailed, nil
	}

	if proposal.YesVotes > proposal.NoVotes {
		proposal.Status = ProposalStatusPassed
		return ProposalStatusPassed, nil
	}

	proposal.Status = ProposalStatusFailed
	return ProposalStatusFailed, nil
}

func (gs *GovernanceSystem) ExecuteProposal(proposalID string) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	proposal, exists := gs.proposals[proposalID]
	if !exists {
		return fmt.Errorf("proposal not found")
	}

	if proposal.Status != ProposalStatusPassed {
		return fmt.Errorf("proposal not passed")
	}

	if proposal.Executed {
		return fmt.Errorf("already executed")
	}

	proposal.Executed = true
	proposal.Status = ProposalStatusExecuted

	return nil
}

func (gs *GovernanceSystem) GetProposal(id string) (*Proposal, error) {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	proposal, exists := gs.proposals[id]
	if !exists {
		return nil, fmt.Errorf("proposal not found")
	}

	return proposal, nil
}

func (gs *GovernanceSystem) GetActiveProposals() []*Proposal {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	var active []*Proposal
	now := time.Now()
	for _, p := range gs.proposals {
		if now.After(p.StartTime) && now.Before(p.EndTime) && p.Status == ProposalStatusActive {
			active = append(active, p)
		}
	}
	return active
}

func generateProposalID(title, author string) string {
	data := fmt.Sprintf("%s-%s-%d", title, author, time.Now().UnixNano())
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])[:16]
}

type PriceOracle struct {
	mu        sync.RWMutex
	feeds     map[string]*PriceFeed
	aggPrices map[string]uint64
}

type PriceFeed struct {
	Name      string
	Symbol    string
	Value     uint64
	Decimals  uint8
	Updated   time.Time
	Signature string
}

func NewPriceOracle() *PriceOracle {
	return &PriceOracle{
		feeds:     make(map[string]*PriceFeed),
		aggPrices: make(map[string]uint64),
	}
}

func (po *PriceOracle) SetPrice(name, symbol string, value uint64, decimals uint8, signature string) {
	po.mu.Lock()
	defer po.mu.Unlock()

	po.feeds[name] = &PriceFeed{
		Name:      name,
		Symbol:    symbol,
		Value:     value,
		Decimals:  decimals,
		Updated:   time.Now(),
		Signature: signature,
	}

	po.aggregatePrices()
}

func (po *PriceOracle) GetPrice(symbol string) (uint64, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	price, exists := po.aggPrices[symbol]
	if !exists {
		return 0, fmt.Errorf("price not available")
	}

	return price, nil
}

func (po *PriceOracle) GetAllPrices() map[string]uint64 {
	po.mu.RLock()
	defer po.mu.RUnlock()

	result := make(map[string]uint64)
	for k, v := range po.aggPrices {
		result[k] = v
	}
	return result
}

func (po *PriceOracle) aggregatePrices() {
	symbols := make(map[string][]uint64)
	for _, feed := range po.feeds {
		symbols[feed.Symbol] = append(symbols[feed.Symbol], feed.Value)
	}

	for symbol, values := range symbols {
		if len(values) == 0 {
			continue
		}

		var sum uint64
		for _, v := range values {
			sum += v
		}
		po.aggPrices[symbol] = sum / uint64(len(values))
	}
}

type SocialRecovery struct {
	mu          sync.RWMutex
	recoveryMap map[string]*RecoveryConfig
	pendingReqs map[string]*RecoveryRequest
}

type RecoveryConfig struct {
	Owner       string
	Guardians   []string
	Threshold   int
	DelayPeriod time.Duration
}

type RecoveryRequest struct {
	Owner         string
	NewOwner      string
	Initiator     string
	Timestamp     time.Time
	Confirmations int
	ConfirmedBy   map[string]bool
}

func NewSocialRecovery() *SocialRecovery {
	return &SocialRecovery{
		recoveryMap: make(map[string]*RecoveryConfig),
		pendingReqs: make(map[string]*RecoveryRequest),
	}
}

func (sr *SocialRecovery) SetupRecovery(owner string, guardians []string, threshold int, delayPeriod time.Duration) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if len(guardians) < threshold {
		return fmt.Errorf("threshold cannot exceed guardian count")
	}

	sr.recoveryMap[owner] = &RecoveryConfig{
		Owner:       owner,
		Guardians:   guardians,
		Threshold:   threshold,
		DelayPeriod: delayPeriod,
	}

	return nil
}

func (sr *SocialRecovery) InitiateRecovery(owner, newOwner, initiator string) (string, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	config, exists := sr.recoveryMap[owner]
	if !exists {
		return "", fmt.Errorf("recovery not configured")
	}

	isGuardian := false
	for _, g := range config.Guardians {
		if g == initiator {
			isGuardian = true
			break
		}
	}

	if !isGuardian && initiator != owner {
		return "", fmt.Errorf("only owner or guardians can initiate recovery")
	}

	requestID := generateRecoveryID(owner, newOwner)
	req := &RecoveryRequest{
		Owner:         owner,
		NewOwner:      newOwner,
		Initiator:     initiator,
		Timestamp:     time.Now(),
		Confirmations: 0,
		ConfirmedBy:   make(map[string]bool),
	}

	sr.pendingReqs[requestID] = req
	return requestID, nil
}

func (sr *SocialRecovery) ConfirmRecovery(requestID, guardian string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	req, exists := sr.pendingReqs[requestID]
	if !exists {
		return fmt.Errorf("request not found")
	}

	config, exists := sr.recoveryMap[req.Owner]
	if !exists {
		return fmt.Errorf("recovery not configured")
	}

	isGuardian := false
	for _, g := range config.Guardians {
		if g == guardian {
			isGuardian = true
			break
		}
	}

	if !isGuardian {
		return fmt.Errorf("not a guardian")
	}

	if req.ConfirmedBy[guardian] {
		return fmt.Errorf("already confirmed")
	}

	req.ConfirmedBy[guardian] = true
	req.Confirmations++

	return nil
}

func (sr *SocialRecovery) CompleteRecovery(requestID string) (string, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	req, exists := sr.pendingReqs[requestID]
	if !exists {
		return "", fmt.Errorf("request not found")
	}

	config, exists := sr.recoveryMap[req.Owner]
	if !exists {
		return "", fmt.Errorf("recovery not configured")
	}

	if req.Confirmations < config.Threshold {
		return "", fmt.Errorf("not enough confirmations: have %d, need %d", req.Confirmations, config.Threshold)
	}

	return req.NewOwner, nil
}

func (sr *SocialRecovery) GetRecoveryConfig(owner string) (*RecoveryConfig, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	config, exists := sr.recoveryMap[owner]
	if !exists {
		return nil, fmt.Errorf("recovery not configured")
	}

	return config, nil
}

func generateRecoveryID(owner, newOwner string) string {
	data := fmt.Sprintf("%s-%s-%d", owner, newOwner, time.Now().UnixNano())
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])[:16]
}
