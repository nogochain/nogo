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

package core

import (
	"testing"
)

// TestFeeChecker_ValidateFee_LowFee tests fee validation with too low fee
func TestFeeChecker_ValidateFee_LowFee(t *testing.T) {
	checker := NewFeeChecker(MinFee, MinFeePerByte)

	tx := &Transaction{
		Type:    TxTransfer,
		Amount:  1000,
		Fee:     0, // Fee too low
		Nonce:   1,
		ChainID: 1,
	}

	err := checker.ValidateFee(tx)
	if err == nil {
		t.Fatal("Expected error for low fee, got nil")
	}
	// Check that error message contains "fee too low"
	if !contains(err.Error(), "fee too low") {
		t.Errorf("Expected 'fee too low' error, got: %v", err)
	}
}

// contains checks if substr is in s
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestFeeChecker_ValidateFee_HighFee tests fee validation with unusually high fee
func TestFeeChecker_ValidateFee_HighFee(t *testing.T) {
	checker := NewFeeChecker(MinFee, MinFeePerByte)

	tx := &Transaction{
		Type:    TxTransfer,
		Amount:  1000,
		Fee:     1000000, // Unusually high (100x minimum)
		Nonce:   1,
		ChainID: 1,
	}

	err := checker.ValidateFee(tx)
	if err == nil {
		t.Fatal("Expected error for high fee, got nil")
	}
	// Check that error message contains "fee unusually high"
	if !contains(err.Error(), "fee unusually high") {
		t.Errorf("Expected 'fee unusually high' error, got: %v", err)
	}
}

// TestFeeChecker_ValidateFee_Valid tests fee validation with valid fee
func TestFeeChecker_ValidateFee_Valid(t *testing.T) {
	checker := NewFeeChecker(MinFee, MinFeePerByte)

	// Set mempool size to 0 to avoid congestion adjustment
	checker.UpdateMempoolSize(0)

	tx := &Transaction{
		Type:    TxTransfer,
		Amount:  1000,
		Fee:     20000, // Valid fee (higher than minimum to account for size)
		Nonce:   1,
		ChainID: 1,
	}

	err := checker.ValidateFee(tx)
	if err != nil {
		t.Fatalf("Unexpected error for valid fee: %v", err)
	}
}

// TestFeeChecker_CongestionAdjustment tests fee calculation with mempool congestion
func TestFeeChecker_CongestionAdjustment(t *testing.T) {
	checker := NewFeeChecker(MinFee, MinFeePerByte)

	// Normal mempool size
	checker.UpdateMempoolSize(5000)
	tx := &Transaction{
		Type:    TxTransfer,
		Amount:  1000,
		Fee:     10000,
		Nonce:   1,
		ChainID: 1,
	}
	fee := checker.CalculateRequiredFee(tx)
	expectedFee := uint64(10000)
	if fee < expectedFee {
		t.Errorf("Expected fee >= %d with normal congestion, got %d", expectedFee, fee)
	}

	// High mempool size (congestion)
	checker.UpdateMempoolSize(15000)
	fee = checker.CalculateRequiredFee(tx)
	expectedMinFee := uint64(15000) // 1.5x congestion factor
	if fee < expectedMinFee {
		t.Errorf("Expected fee >= %d with high congestion, got %d", expectedMinFee, fee)
	}
}

// TestFeeChecker_HistoryAdjustment tests fee calculation with historical fees
func TestFeeChecker_HistoryAdjustment(t *testing.T) {
	checker := NewFeeChecker(MinFee, MinFeePerByte)

	// Add historical fees
	for i := 0; i < 100; i++ {
		checker.UpdateHistory(20000)
	}

	tx := &Transaction{
		Type:    TxTransfer,
		Amount:  1000,
		Fee:     5000, // Low fee compared to history
		Nonce:   1,
		ChainID: 1,
	}

	fee := checker.CalculateRequiredFee(tx)
	// Should have congestion adjustment due to low fee vs history
	if fee < 10000 {
		t.Errorf("Expected fee adjustment due to low historical fee, got %d", fee)
	}
}

// TestCalculateMinFee tests the exported CalculateMinFee function
func TestCalculateMinFee(t *testing.T) {
	// Normal conditions
	fee := CalculateMinFee(100, 5000)
	expectedFee := MinFee + 100*MinFeePerByte
	if fee != expectedFee {
		t.Errorf("Expected fee %d, got %d", expectedFee, fee)
	}

	// Congested conditions
	fee = CalculateMinFee(100, 20000)
	expectedMinFee := uint64(20000) // 2.0x congestion factor
	if fee < expectedMinFee {
		t.Errorf("Expected fee >= %d with congestion, got %d", expectedMinFee, fee)
	}
}

// TestFeeChecker_ThreadSafety tests concurrent access safety
func TestFeeChecker_ThreadSafety(t *testing.T) {
	checker := NewFeeChecker(MinFee, MinFeePerByte)

	done := make(chan bool)

	// Concurrent writes
	go func() {
		for i := 0; i < 100; i++ {
			checker.UpdateMempoolSize(i)
			checker.UpdateHistory(uint64(i))
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			_ = checker.GetMinFee()
			tx := &Transaction{
				Type:    TxTransfer,
				Amount:  1000,
				Fee:     10000,
				Nonce:   1,
				ChainID: 1,
			}
			_ = checker.CalculateRequiredFee(tx)
		}
		done <- true
	}()

	<-done
	<-done
}
