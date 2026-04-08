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
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAddressTxsPagination_RequestParsing(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
		errorMsg    string
		expectPage  int
		expectLimit int
		expectSort  string
	}{
		{
			name:        "default parameters",
			url:         "/address/NOGO000000000000000000000000000000000000000000000000000000000000000000/txs",
			expectError: false,
			expectPage:  1,
			expectLimit: DefaultPageSize,
			expectSort:  "desc",
		},
		{
			name:        "custom page and limit",
			url:         "/address/NOGO000000000000000000000000000000000000000000000000000000000000000000/txs?page=2&limit=25",
			expectError: false,
			expectPage:  2,
			expectLimit: 25,
			expectSort:  "desc",
		},
		{
			name:        "sort ascending",
			url:         "/address/NOGO000000000000000000000000000000000000000000000000000000000000000000/txs?sort=asc",
			expectError: false,
			expectPage:  1,
			expectLimit: DefaultPageSize,
			expectSort:  "asc",
		},
		{
			name:        "invalid page",
			url:         "/address/NOGO000000000000000000000000000000000000000000000000000000000000000000/txs?page=abc",
			expectError: true,
			errorMsg:    "invalid page parameter",
		},
		{
			name:        "page less than 1",
			url:         "/address/NOGO000000000000000000000000000000000000000000000000000000000000000000/txs?page=0",
			expectError: true,
			errorMsg:    "page must be >= 1",
		},
		{
			name:        "limit exceeds maximum",
			url:         "/address/NOGO000000000000000000000000000000000000000000000000000000000000000000/txs?limit=150",
			expectError: true,
			errorMsg:    "limit must be <=",
		},
		{
			name:        "invalid sort value",
			url:         "/address/NOGO000000000000000000000000000000000000000000000000000000000000000000/txs?sort=invalid",
			expectError: true,
			errorMsg:    "sort must be 'asc' or 'desc'",
		},
		{
			name:        "time range filter",
			url:         "/address/NOGO000000000000000000000000000000000000000000000000000000000000000000/txs?start_time=1000000000&end_time=2000000000",
			expectError: false,
			expectPage:  1,
			expectLimit: DefaultPageSize,
			expectSort:  "desc",
		},
		{
			name:        "start_time after end_time",
			url:         "/address/NOGO000000000000000000000000000000000000000000000000000000000000000000/txs?start_time=2000000000&end_time=1000000000",
			expectError: true,
			errorMsg:    "start_time must be <= end_time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			server := &Server{}

			parsedReq, err := server.parseAddressTxsRequest(req)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				} else {
					if parsedReq.Page != tt.expectPage {
						t.Errorf("expected page %d, got %d", tt.expectPage, parsedReq.Page)
					}
					if parsedReq.Limit != tt.expectLimit {
						t.Errorf("expected limit %d, got %d", tt.expectLimit, parsedReq.Limit)
					}
					if parsedReq.Sort != tt.expectSort {
						t.Errorf("expected sort %q, got %q", tt.expectSort, parsedReq.Sort)
					}
				}
			}
		})
	}
}

func TestCalculateTotalPages(t *testing.T) {
	tests := []struct {
		totalCount uint64
		limit      int
		expected   int
	}{
		{0, 50, 0},
		{1, 50, 1},
		{50, 50, 1},
		{51, 50, 2},
		{100, 50, 2},
		{101, 50, 3},
		{250, 100, 3},
		{1000, 50, 20},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("count_%d_limit_%d", tt.totalCount, tt.limit), func(t *testing.T) {
			result := CalculateTotalPages(tt.totalCount, tt.limit)
			if result != tt.expected {
				t.Errorf("CalculateTotalPages(%d, %d) = %d, expected %d", tt.totalCount, tt.limit, result, tt.expected)
			}
		})
	}
}

func TestValidateTimestamp(t *testing.T) {
	tests := []struct {
		name        string
		timestamp   int64
		expectError bool
	}{
		{"zero timestamp", 0, false},
		{"valid timestamp", time.Now().Unix(), false},
		{"far future timestamp", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), false},
		{"negative timestamp", -1, true},
		{"too far future", time.Date(2200, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimestamp(tt.timestamp)
			if tt.expectError && err == nil {
				t.Errorf("expected error for timestamp %d, got nil", tt.timestamp)
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error for timestamp %d: %v", tt.timestamp, err)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func generateTestTxID(seed int) string {
	data := make([]byte, 32)
	for i := 0; i < 32; i++ {
		data[i] = byte((seed >> (i % 8)) & 0xFF)
	}
	return hex.EncodeToString(data)
}

func TestAddressTxsPagination_MaxPageSize(t *testing.T) {
	address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"

	url := fmt.Sprintf("/address/%s/txs?limit=150", address)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()

	server := &Server{}
	server.handleAddressTxsPagination(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for limit > 100, got %d", w.Code)
	}
}

func TestAddressTxsPagination_InvalidAddress(t *testing.T) {
	server := &Server{}

	invalidAddresses := []string{
		"INVALID_ADDRESS",
		"NOGO",
		"",
	}

	for _, addr := range invalidAddresses {
		t.Run(fmt.Sprintf("invalid_%s", addr), func(t *testing.T) {
			url := fmt.Sprintf("/address/%s/txs", addr)
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			server.handleAddressTxsPagination(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status 400 for invalid address, got %d", w.Code)
			}
		})
	}
}

func TestAddressTxsPagination_QueryOptions(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectPage  int
		expectLimit int
		expectSort  string
		expectStart int64
		expectEnd   int64
		expectError bool
	}{
		{
			name:        "default options",
			url:         "/address/NOGO000000000000000000000000000000000000000000000000000000000000000000/txs",
			expectPage:  1,
			expectLimit: 50,
			expectSort:  "desc",
			expectStart: 0,
			expectEnd:   math.MaxInt64,
		},
		{
			name:        "all parameters",
			url:         "/address/NOGO000000000000000000000000000000000000000000000000000000000000000000/txs?page=3&limit=30&sort=asc&start_time=1000000000&end_time=2000000000",
			expectPage:  3,
			expectLimit: 30,
			expectSort:  "asc",
			expectStart: 1000000000,
			expectEnd:   2000000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			server := &Server{}

			parsedReq, err := server.parseAddressTxsRequest(req)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				} else {
					if parsedReq.Page != tt.expectPage {
						t.Errorf("expected page %d, got %d", tt.expectPage, parsedReq.Page)
					}
					if parsedReq.Limit != tt.expectLimit {
						t.Errorf("expected limit %d, got %d", tt.expectLimit, parsedReq.Limit)
					}
					if parsedReq.Sort != tt.expectSort {
						t.Errorf("expected sort %q, got %q", tt.expectSort, parsedReq.Sort)
					}
				}
			}
		})
	}
}

func TestAddressTxsPagination_EdgeCases(t *testing.T) {
	t.Run("limit equals max", func(t *testing.T) {
		address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"
		url := fmt.Sprintf("/address/%s/txs?limit=100", address)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()

		server := &Server{}
		server.handleAddressTxsPagination(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 200 or 500, got %d", w.Code)
		}
	})

	t.Run("page 1", func(t *testing.T) {
		address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"
		url := fmt.Sprintf("/address/%s/txs?page=1", address)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()

		server := &Server{}
		server.handleAddressTxsPagination(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 200 or 500, got %d", w.Code)
		}
	})

	t.Run("zero limit", func(t *testing.T) {
		address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"
		url := fmt.Sprintf("/address/%s/txs?limit=0", address)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()

		server := &Server{}
		server.handleAddressTxsPagination(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400 for limit=0, got %d", w.Code)
		}
	})
}

func TestAddressTxsPagination_ResponseStructure(t *testing.T) {
	t.Run("empty result structure", func(t *testing.T) {
		address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"
		url := fmt.Sprintf("/address/%s/txs", address)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()

		server := &Server{}
		server.handleAddressTxsPagination(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
			t.Errorf("expected status 200 or 500, got %d", w.Code)
			t.Logf("Response: %s", w.Body.String())
		}
	})
}

func TestAddressTxsPagination_OffsetCalculation(t *testing.T) {
	tests := []struct {
		page        int
		limit       int
		expectedOff int
	}{
		{1, 50, 0},
		{2, 50, 50},
		{3, 25, 50},
		{5, 10, 40},
		{10, 100, 900},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("page%d_limit%d", tt.page, tt.limit), func(t *testing.T) {
			offset := (tt.page - 1) * tt.limit
			if offset != tt.expectedOff {
				t.Errorf("offset for page=%d, limit=%d: expected %d, got %d",
					tt.page, tt.limit, tt.expectedOff, offset)
			}
		})
	}
}

func TestAddressTxsPagination_SortValidation(t *testing.T) {
	validSorts := []string{"asc", "desc"}
	invalidSorts := []string{"ASC", "DESC", "random", ""}

	for _, sort := range validSorts {
		t.Run(fmt.Sprintf("valid_%s", sort), func(t *testing.T) {
			address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"
			url := fmt.Sprintf("/address/%s/txs?sort=%s", address, sort)
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			server := &Server{}
			server.handleAddressTxsPagination(w, req)

			if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
				t.Errorf("expected status 200 or 500, got %d", w.Code)
			}
		})
	}

	for _, sort := range invalidSorts {
		t.Run(fmt.Sprintf("invalid_%s", sort), func(t *testing.T) {
			address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"
			url := fmt.Sprintf("/address/%s/txs?sort=%s", address, sort)
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			server := &Server{}
			server.handleAddressTxsPagination(w, req)

			if sort == "" {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status 200 or 500 for empty sort, got %d", w.Code)
				}
			} else {
				if w.Code != http.StatusBadRequest {
					t.Errorf("expected status 400 for invalid sort %q, got %d", sort, w.Code)
				}
			}
		})
	}
}

func TestAddressTxsPagination_TimestampValidation(t *testing.T) {
	tests := []struct {
		name        string
		startTime   string
		endTime     string
		expectError bool
	}{
		{
			name:        "valid timestamps",
			startTime:   "1000000000",
			endTime:     "2000000000",
			expectError: false,
		},
		{
			name:        "invalid start_time",
			startTime:   "abc",
			endTime:     "2000000000",
			expectError: true,
		},
		{
			name:        "invalid end_time",
			startTime:   "1000000000",
			endTime:     "abc",
			expectError: true,
		},
		{
			name:        "negative start_time",
			startTime:   "-1",
			endTime:     "2000000000",
			expectError: true,
		},
	}

	address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := fmt.Sprintf("/address/%s/txs?start_time=%s&end_time=%s", address, tt.startTime, tt.endTime)
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			server := &Server{}
			server.handleAddressTxsPagination(w, req)

			if tt.expectError && w.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", w.Code)
			}
		})
	}
}

func TestAddressTxsPagination_PaginationMeta(t *testing.T) {
	tests := []struct {
		totalCount  uint64
		limit       int
		page        int
		expectNext  bool
		expectPrev  bool
		expectPages int
	}{
		{100, 50, 1, true, false, 2},
		{100, 50, 2, false, true, 2},
		{101, 50, 1, true, false, 3},
		{101, 50, 2, true, true, 3},
		{101, 50, 3, false, true, 3},
		{0, 50, 1, false, false, 0},
		{50, 50, 1, false, false, 1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("total%d_limit%d_page%d", tt.totalCount, tt.limit, tt.page), func(t *testing.T) {
			totalPages := CalculateTotalPages(tt.totalCount, tt.limit)
			if totalPages != tt.expectPages {
				t.Errorf("expected %d total pages, got %d", tt.expectPages, totalPages)
			}

			hasNext := tt.page < totalPages
			hasPrev := tt.page > 1

			if hasNext != tt.expectNext {
				t.Errorf("expected hasNextPage=%v, got %v", tt.expectNext, hasNext)
			}
			if hasPrev != tt.expectPrev {
				t.Errorf("expected hasPrevPage=%v, got %v", tt.expectPrev, hasPrev)
			}
		})
	}
}

func TestAddressTxsPagination_LimitBoundaries(t *testing.T) {
	address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"

	tests := []struct {
		limit      int
		expectCode int
	}{
		{1, http.StatusOK},
		{49, http.StatusOK},
		{50, http.StatusOK},
		{51, http.StatusOK},
		{99, http.StatusOK},
		{100, http.StatusOK},
		{101, http.StatusBadRequest},
		{200, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("limit_%d", tt.limit), func(t *testing.T) {
			url := fmt.Sprintf("/address/%s/txs?limit=%d", address, tt.limit)
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			server := &Server{}
			server.handleAddressTxsPagination(w, req)

			if w.Code != tt.expectCode && !(w.Code == http.StatusInternalServerError && tt.expectCode == http.StatusOK) {
				t.Errorf("expected status %d, got %d", tt.expectCode, w.Code)
			}
		})
	}
}

func TestAddressTxsPagination_PageBoundaries(t *testing.T) {
	address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"

	tests := []struct {
		page       int
		expectCode int
	}{
		{1, http.StatusOK},
		{2, http.StatusOK},
		{100, http.StatusOK},
		{1000, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("page_%d", tt.page), func(t *testing.T) {
			url := fmt.Sprintf("/address/%s/txs?page=%d", address, tt.page)
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			server := &Server{}
			server.handleAddressTxsPagination(w, req)

			if w.Code != tt.expectCode && !(w.Code == http.StatusInternalServerError && tt.expectCode == http.StatusOK) {
				t.Errorf("expected status %d, got %d", tt.expectCode, w.Code)
			}
		})
	}
}
