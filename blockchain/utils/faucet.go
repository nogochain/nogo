package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Faucet struct {
	balance     uint64
	limitPerDay uint64
	cooldowns   map[string]time.Time
	mu          sync.Mutex
	enabled     bool
	chainID     uint64
}

func NewFaucet(initialBalance, limitPerDay uint64, chainID uint64) *Faucet {
	return &Faucet{
		balance:     initialBalance,
		limitPerDay: limitPerDay,
		cooldowns:   make(map[string]time.Time),
		enabled:     true,
		chainID:     chainID,
	}
}

func (f *Faucet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/faucet/info" {
		f.handleInfo(w)
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/faucet/claim" {
		f.handleClaim(w, r)
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/faucet/fund" {
		f.handleFund(w, r)
		return
	}

	http.Error(w, "not found", http.StatusNotFound)
}

func (f *Faucet) handleInfo(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()

	json.NewEncoder(w).Encode(map[string]any{
		"balance":     f.balance,
		"limitPerDay": f.limitPerDay,
		"enabled":     f.enabled,
		"chainId":     f.chainID,
	})
}

func (f *Faucet) handleClaim(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
		Amount  uint64 `json:"amount"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Address == "" {
		http.Error(w, "address required", http.StatusBadRequest)
		return
	}

	if req.Amount == 0 {
		req.Amount = 1000
	}

	if req.Amount > f.limitPerDay {
		req.Amount = f.limitPerDay
	}

	f.mu.Lock()

	if !f.enabled {
		f.mu.Unlock()
		http.Error(w, "faucet disabled", http.StatusServiceUnavailable)
		return
	}

	if f.balance < req.Amount {
		f.mu.Unlock()
		http.Error(w, "insufficient faucet balance", http.StatusServiceUnavailable)
		return
	}

	lastClaim, exists := f.cooldowns[req.Address]
	if exists && time.Since(lastClaim) < 24*time.Hour {
		f.mu.Unlock()
		http.Error(w, "cooldown period active (24h)", http.StatusTooManyRequests)
		return
	}

	f.balance -= req.Amount
	f.cooldowns[req.Address] = time.Now()

	f.mu.Unlock()

	json.NewEncoder(w).Encode(map[string]any{
		"success":      true,
		"txid":         fmt.Sprintf("faucet-%d-%s", time.Now().Unix(), req.Address[:16]),
		"amount":       req.Amount,
		"remaining":    f.balance,
		"cooldownEnds": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	})
}

func (f *Faucet) handleFund(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Amount uint64 `json:"amount"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	f.balance += req.Amount
	f.mu.Unlock()

	json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"newBalance": f.balance,
	})
}

func (f *Faucet) AddFunds(amount uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.balance += amount
}

func (f *Faucet) SetEnabled(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enabled = enabled
}
