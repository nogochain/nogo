// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/nogochain/nogo/blockchain/core"
)

// MaxBatchBalanceAddresses is the maximum number of addresses allowed per batch balance request.
const MaxBatchBalanceAddresses = 100

// handleBatchBalance handles POST /balance/batch requests for querying balances of
// multiple addresses in a single API call. Supports up to 100 addresses, performs
// deduplication and format validation, and returns warnings for invalid entries.
func (s *Server) handleBatchBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Addresses []string `json:"addresses"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	if len(req.Addresses) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "empty addresses"})
		return
	}
	if len(req.Addresses) > MaxBatchBalanceAddresses {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("max %d addresses per request", MaxBatchBalanceAddresses),
		})
		return
	}

	// Deduplicate addresses and validate format.
	// NogoChain addresses are 78 characters: "NOGO" prefix + 64 hex + 10 checksum.
	seen := make(map[string]bool, len(req.Addresses))
	valid := make([]string, 0, len(req.Addresses))
	var warnings []string

	for _, addr := range req.Addresses {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			warnings = append(warnings, "empty address skipped")
			continue
		}
		if !strings.HasPrefix(addr, "NOGO") || len(addr) != 78 {
			warnings = append(warnings, fmt.Sprintf("invalid address format: %s", truncateAddr(addr, 20)))
			continue
		}
		if seen[addr] {
			warnings = append(warnings, fmt.Sprintf("duplicate address skipped: %s", truncateAddr(addr, 20)))
			continue
		}
		seen[addr] = true
		valid = append(valid, addr)
	}

	// Query balances through the Blockchain interface.
	// Internal implementation uses BoltDB B-tree lookups; 100 calls < 1ms.
	type balanceResult struct {
		Address string `json:"address"`
		Balance uint64 `json:"balance"`
		Nonce   uint64 `json:"nonce"`
	}
	results := make([]balanceResult, 0, len(valid))

	for _, addr := range valid {
		acct, ok := s.bc.Balance(addr)
		if !ok {
			acct = core.Account{}
		}
		results = append(results, balanceResult{
			Address: addr,
			Balance: acct.Balance,
			Nonce:   acct.Nonce,
		})
	}

	resp := map[string]any{
		"balances": results,
		"count":    len(results),
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	writeJSON(w, http.StatusOK, resp)
}

// truncateAddr truncates an address string to maxLen characters for safe logging.
func truncateAddr(addr string, maxLen int) string {
	if len(addr) <= maxLen {
		return addr
	}
	return addr[:maxLen] + "..."
}
