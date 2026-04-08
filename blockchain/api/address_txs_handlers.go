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

package api

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/index"
)

const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

type AddressTxsPaginationRequest struct {
	Address   string
	Page      int
	Limit     int
	Sort      string
	StartTime int64
	EndTime   int64
}

type AddressTxsPaginationResponse struct {
	Address      string              `json:"address"`
	Transactions []AddressTxResponse `json:"transactions"`
	Pagination   PaginationMeta      `json:"pagination"`
	Sort         string              `json:"sort"`
	Filters      FilterMeta          `json:"filters"`
}

type AddressTxResponse struct {
	TxID      string          `json:"txId"`
	Height    uint64          `json:"height"`
	BlockHash string          `json:"blockHash"`
	Index     int             `json:"index"`
	FromAddr  string          `json:"fromAddr"`
	ToAddress string          `json:"toAddress"`
	Amount    uint64          `json:"amount"`
	Fee       uint64          `json:"fee"`
	Nonce     uint64          `json:"nonce"`
	Timestamp int64           `json:"timestamp"`
	Type      TransactionType `json:"type"`
}

type PaginationMeta struct {
	Page        int    `json:"page"`
	Limit       int    `json:"limit"`
	TotalCount  uint64 `json:"totalCount"`
	TotalPages  int    `json:"totalPages"`
	HasNextPage bool   `json:"hasNextPage"`
	HasPrevPage bool   `json:"hasPrevPage"`
}

type FilterMeta struct {
	StartTime int64 `json:"startTime,omitempty"`
	EndTime   int64 `json:"endTime,omitempty"`
}

type TransactionType string

const (
	TxCoinbase TransactionType = "coinbase"
	TxTransfer TransactionType = "transfer"
)

func (s *Server) handleAddressTxsPagination(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := s.parseAddressTxsRequest(r)
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(),
		})
		return
	}

	if err := core.ValidateAddress(req.Address); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid address: " + err.Error(),
		})
		return
	}

	response, err := s.queryAddressTxs(req)
	if err != nil {
		_ = writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "query failed: " + err.Error(),
		})
		return
	}

	_ = writeJSON(w, http.StatusOK, response)
}

func (s *Server) parseAddressTxsRequest(r *http.Request) (*AddressTxsPaginationRequest, error) {
	path := strings.TrimPrefix(r.URL.Path, "/address/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "txs" {
		return nil, fmt.Errorf("expected /address/{addr}/txs")
	}

	req := &AddressTxsPaginationRequest{
		Address:   parts[0],
		Page:      1,
		Limit:     DefaultPageSize,
		Sort:      "desc",
		StartTime: 0,
		EndTime:   math.MaxInt64,
	}

	query := r.URL.Query()

	if pageStr := query.Get("page"); pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil {
			return nil, fmt.Errorf("invalid page parameter: must be integer")
		}
		if page < 1 {
			return nil, fmt.Errorf("page must be >= 1")
		}
		req.Page = page
	}

	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, fmt.Errorf("invalid limit parameter: must be integer")
		}
		if limit < 1 {
			return nil, fmt.Errorf("limit must be >= 1")
		}
		if limit > MaxPageSize {
			return nil, fmt.Errorf("limit must be <= %d", MaxPageSize)
		}
		req.Limit = limit
	}

	if sortStr := query.Get("sort"); sortStr != "" {
		if sortStr != "asc" && sortStr != "desc" {
			return nil, fmt.Errorf("sort must be 'asc' or 'desc'")
		}
		req.Sort = sortStr
	}

	if startTimeStr := query.Get("start_time"); startTimeStr != "" {
		startTime, err := strconv.ParseInt(startTimeStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid start_time: must be Unix timestamp")
		}
		if startTime < 0 {
			return nil, fmt.Errorf("start_time must be non-negative")
		}
		req.StartTime = startTime
	}

	if endTimeStr := query.Get("end_time"); endTimeStr != "" {
		endTime, err := strconv.ParseInt(endTimeStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid end_time: must be Unix timestamp")
		}
		if endTime < 0 {
			return nil, fmt.Errorf("end_time must be non-negative")
		}
		req.EndTime = endTime
	}

	if req.StartTime > 0 && req.EndTime < math.MaxInt64 && req.StartTime > req.EndTime {
		return nil, fmt.Errorf("start_time must be <= end_time")
	}

	return req, nil
}

func (s *Server) queryAddressTxs(req *AddressTxsPaginationRequest) (*AddressTxsPaginationResponse, error) {
	chain := s.bc
	if chain == nil {
		return nil, fmt.Errorf("blockchain not available")
	}

	var addressIndex *index.AddressIndex
	if chainWithIndex, ok := chain.(interface{ GetAddressIndex() *index.AddressIndex }); ok {
		addressIndex = chainWithIndex.GetAddressIndex()
	}
	if addressIndex == nil {
		return nil, fmt.Errorf("address index not available")
	}

	sortOrder := index.SortDesc
	if req.Sort == "asc" {
		sortOrder = index.SortAsc
	}

	offset := (req.Page - 1) * req.Limit

	queryOpts := index.QueryOptions{
		Limit:     req.Limit,
		Offset:    offset,
		Sort:      sortOrder,
		MinHeight: 0,
		MaxHeight: math.MaxUint64,
	}

	entries, totalCount, err := addressIndex.QueryAddressTxs(req.Address, queryOpts)
	if err != nil {
		return nil, fmt.Errorf("query address transactions: %w", err)
	}

	filteredEntries := make([]index.AddressIndexEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Timestamp >= req.StartTime && entry.Timestamp <= req.EndTime {
			filteredEntries = append(filteredEntries, entry)
		}
	}

	totalPages := int(totalCount+uint64(req.Limit)-1) / req.Limit
	if totalPages == 0 && totalCount > 0 {
		totalPages = 1
	}

	response := &AddressTxsPaginationResponse{
		Address:      req.Address,
		Transactions: make([]AddressTxResponse, len(filteredEntries)),
		Pagination: PaginationMeta{
			Page:        req.Page,
			Limit:       req.Limit,
			TotalCount:  totalCount,
			TotalPages:  totalPages,
			HasNextPage: req.Page < totalPages,
			HasPrevPage: req.Page > 1,
		},
		Sort: req.Sort,
		Filters: FilterMeta{
			StartTime: req.StartTime,
			EndTime:   req.EndTime,
		},
	}

	for i, entry := range filteredEntries {
		response.Transactions[i] = AddressTxResponse{
			TxID:      entry.TxID,
			Height:    entry.Height,
			BlockHash: entry.BlockHash,
			Index:     entry.Index,
			FromAddr:  entry.FromAddr,
			ToAddress: entry.ToAddress,
			Amount:    entry.Amount,
			Fee:       entry.Fee,
			Nonce:     entry.Nonce,
			Timestamp: entry.Timestamp,
			Type:      TransactionType(entry.Type),
		}
	}

	return response, nil
}

func CalculateTotalPages(totalCount uint64, limit int) int {
	if totalCount == 0 {
		return 0
	}
	return int((totalCount + uint64(limit) - 1) / uint64(limit))
}

func ValidateTimestamp(timestamp int64) error {
	if timestamp < 0 {
		return fmt.Errorf("timestamp must be non-negative")
	}

	maxTimestamp := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	if timestamp > maxTimestamp {
		return fmt.Errorf("timestamp too far in the future")
	}

	return nil
}
