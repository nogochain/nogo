// Copyright 2026 The NogoChain Authors
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
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package crypto

import (
	"strings"
	"testing"
	"time"
)

// TestDeriveAddressesRequest_Validate tests request validation
func TestDeriveAddressesRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     *DeriveAddressesRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid_request",
			req: &DeriveAddressesRequest{
				Mnemonic:   "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
				Network:    NetworkMainnet,
				StartIndex: 0,
				Count:      10,
			},
			wantErr: false,
		},
		{
			name: "missing_mnemonic",
			req: &DeriveAddressesRequest{
				Network:    NetworkMainnet,
				StartIndex: 0,
				Count:      10,
			},
			wantErr: true,
			errMsg:  "mnemonic is required",
		},
		{
			name: "zero_count",
			req: &DeriveAddressesRequest{
				Mnemonic:   "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
				Network:    NetworkMainnet,
				StartIndex: 0,
				Count:      0,
			},
			wantErr: true,
			errMsg:  "invalid count",
		},
		{
			name: "count_exceeds_limit",
			req: &DeriveAddressesRequest{
				Mnemonic:   "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
				Network:    NetworkMainnet,
				StartIndex: 0,
				Count:      51,
			},
			wantErr: true,
			errMsg:  "derivation limit exceeded",
		},
		{
			name: "invalid_network",
			req: &DeriveAddressesRequest{
				Mnemonic:   "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
				Network:    "invalid",
				StartIndex: 0,
				Count:      10,
			},
			wantErr: true,
			errMsg:  "invalid network type",
		},
		{
			name: "max_count_allowed",
			req: &DeriveAddressesRequest{
				Mnemonic:   "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
				Network:    NetworkMainnet,
				StartIndex: 0,
				Count:      50,
			},
			wantErr: false,
		},
		{
			name: "testnet_valid",
			req: &DeriveAddressesRequest{
				Mnemonic:   "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
				Network:    NetworkTestnet,
				StartIndex: 0,
				Count:      5,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Validate() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

// TestDeriveAddresses_Basic tests basic address derivation
func TestDeriveAddresses_Basic(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	req := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Passphrase: "",
		Network:    NetworkMainnet,
		CoinType:   0,
		Account:    0,
		Change:     0,
		StartIndex: 0,
		Count:      5,
	}

	resp, err := DeriveAddresses(req)
	if err != nil {
		t.Fatalf("DeriveAddresses() error = %v", err)
	}

	if len(resp.Addresses) != 5 {
		t.Errorf("Expected 5 addresses, got %d", len(resp.Addresses))
	}

	// Verify addresses are unique
	seen := make(map[string]bool)
	for _, addr := range resp.Addresses {
		if seen[addr.Address] {
			t.Errorf("Duplicate address: %s", addr.Address)
		}
		seen[addr.Address] = true

		// Verify address format
		if !strings.HasPrefix(addr.Address, "NOGO") {
			t.Errorf("Invalid address prefix: %s", addr.Address)
		}

		// Verify path format
		expectedPathPrefix := "m/44'/0'/0'/0/"
		if !strings.HasPrefix(addr.Path, expectedPathPrefix) {
			t.Errorf("Invalid path prefix: %s", addr.Path)
		}
	}
}

// TestDeriveAddresses_DifferentNetworks tests derivation on different networks
func TestDeriveAddresses_DifferentNetworks(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	// Mainnet
	mainnetReq := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Network:    NetworkMainnet,
		StartIndex: 0,
		Count:      1,
	}
	mainnetResp, err := DeriveAddresses(mainnetReq)
	if err != nil {
		t.Fatalf("Mainnet derivation failed: %v", err)
	}

	// Testnet
	testnetReq := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Network:    NetworkTestnet,
		StartIndex: 0,
		Count:      1,
	}
	testnetResp, err := DeriveAddresses(testnetReq)
	if err != nil {
		t.Fatalf("Testnet derivation failed: %v", err)
	}

	// Addresses should be different due to different version bytes
	if mainnetResp.Addresses[0].Address == testnetResp.Addresses[0].Address {
		t.Error("Mainnet and testnet addresses should differ")
	}
}

// TestDeriveAddresses_DifferentAccounts tests derivation with different account indices
func TestDeriveAddresses_DifferentAccounts(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	req1 := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Network:    NetworkMainnet,
		Account:    0,
		StartIndex: 0,
		Count:      1,
	}
	resp1, err := DeriveAddresses(req1)
	if err != nil {
		t.Fatalf("Account 0 derivation failed: %v", err)
	}

	req2 := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Network:    NetworkMainnet,
		Account:    1,
		StartIndex: 0,
		Count:      1,
	}
	resp2, err := DeriveAddresses(req2)
	if err != nil {
		t.Fatalf("Account 1 derivation failed: %v", err)
	}

	if resp1.Addresses[0].Address == resp2.Addresses[0].Address {
		t.Error("Addresses from different accounts should differ")
	}
}

// TestDeriveAddresses_ChainType tests external vs internal chain derivation
func TestDeriveAddresses_ChainType(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	// External chain (change = 0)
	externalReq := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Network:    NetworkMainnet,
		Change:     0,
		StartIndex: 0,
		Count:      1,
	}
	externalResp, err := DeriveAddresses(externalReq)
	if err != nil {
		t.Fatalf("External chain derivation failed: %v", err)
	}

	// Internal chain (change = 1)
	internalReq := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Network:    NetworkMainnet,
		Change:     1,
		StartIndex: 0,
		Count:      1,
	}
	internalResp, err := DeriveAddresses(internalReq)
	if err != nil {
		t.Fatalf("Internal chain derivation failed: %v", err)
	}

	if externalResp.Addresses[0].Address == internalResp.Addresses[0].Address {
		t.Error("External and internal chain addresses should differ")
	}
}

// TestDeriveAddresses_StartIndex tests derivation with different start indices
func TestDeriveAddresses_StartIndex(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	req1 := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Network:    NetworkMainnet,
		StartIndex: 0,
		Count:      1,
	}
	resp1, err := DeriveAddresses(req1)
	if err != nil {
		t.Fatalf("Index 0 derivation failed: %v", err)
	}

	req2 := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Network:    NetworkMainnet,
		StartIndex: 10,
		Count:      1,
	}
	resp2, err := DeriveAddresses(req2)
	if err != nil {
		t.Fatalf("Index 10 derivation failed: %v", err)
	}

	if resp1.Addresses[0].Address == resp2.Addresses[0].Address {
		t.Error("Addresses at different indices should differ")
	}

	// Verify index in response
	if resp2.Addresses[0].Index != 10 {
		t.Errorf("Expected index 10, got %d", resp2.Addresses[0].Index)
	}
}

// TestDeriveAddresses_WithPassphrase tests derivation with BIP39 passphrase
func TestDeriveAddresses_WithPassphrase(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	// Without passphrase
	noPassReq := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Passphrase: "",
		Network:    NetworkMainnet,
		StartIndex: 0,
		Count:      1,
	}
	noPassResp, err := DeriveAddresses(noPassReq)
	if err != nil {
		t.Fatalf("No passphrase derivation failed: %v", err)
	}

	// With passphrase
	withPassReq := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Passphrase: "TREZOR",
		Network:    NetworkMainnet,
		StartIndex: 0,
		Count:      1,
	}
	withPassResp, err := DeriveAddresses(withPassReq)
	if err != nil {
		t.Fatalf("With passphrase derivation failed: %v", err)
	}

	if noPassResp.Addresses[0].Address == withPassResp.Addresses[0].Address {
		t.Error("Addresses with and without passphrase should differ")
	}
}

// TestDeriveAddresses_MaxLimit tests derivation at maximum limit (50 addresses)
func TestDeriveAddresses_MaxLimit(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	req := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Network:    NetworkMainnet,
		StartIndex: 0,
		Count:      50,
	}

	start := time.Now()
	resp, err := DeriveAddresses(req)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Max limit derivation failed: %v", err)
	}

	if len(resp.Addresses) != 50 {
		t.Errorf("Expected 50 addresses, got %d", len(resp.Addresses))
	}

	// Performance check: should complete in < 50ms
	if duration > 50*time.Millisecond {
		t.Errorf("Performance: derivation took %v, expected < 50ms", duration)
	}

	// Verify all addresses are unique
	seen := make(map[string]bool)
	for _, addr := range resp.Addresses {
		if seen[addr.Address] {
			t.Errorf("Duplicate address found: %s", addr.Address)
		}
		seen[addr.Address] = true
	}
}

// TestDeriveAddressSingle tests single address derivation
func TestDeriveAddressSingle(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	addr, err := DeriveAddressSingle(mnemonic, "", NetworkMainnet, 0, 0, 0, 5)
	if err != nil {
		t.Fatalf("DeriveAddressSingle() error = %v", err)
	}

	if !strings.HasPrefix(addr.Address, "NOGO") {
		t.Errorf("Invalid address prefix: %s", addr.Address)
	}

	if addr.Index != 5 {
		t.Errorf("Expected index 5, got %d", addr.Index)
	}

	expectedPath := "m/44'/0'/0'/0/5"
	if addr.Path != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, addr.Path)
	}
}

// TestBuildDerivationPath tests path construction
func TestBuildDerivationPath(t *testing.T) {
	tests := []struct {
		name      string
		purpose   uint32
		coinType  uint32
		account   uint32
		change    uint32
		index     uint32
		wantPath  string
		wantError bool
	}{
		{
			name:      "bip44_standard",
			purpose:   PurposeBIP44,
			coinType:  0,
			account:   0,
			change:    0,
			index:     0,
			wantPath:  "m/44'/0'/0'/0/0",
			wantError: false,
		},
		{
			name:      "bip49_purpose",
			purpose:   PurposeBIP49,
			coinType:  0,
			account:   0,
			change:    0,
			index:     0,
			wantPath:  "m/49'/0'/0'/0/0",
			wantError: false,
		},
		{
			name:      "bip84_purpose",
			purpose:   PurposeBIP84,
			coinType:  0,
			account:   0,
			change:    0,
			index:     0,
			wantPath:  "m/84'/0'/0'/0/0",
			wantError: false,
		},
		{
			name:      "high_index",
			purpose:   PurposeBIP44,
			coinType:  0,
			account:   0,
			change:    0,
			index:     1000000,
			wantPath:  "m/44'/0'/0'/0/1000000",
			wantError: false,
		},
		{
			name:      "invalid_purpose",
			purpose:   999,
			coinType:  0,
			account:   0,
			change:    0,
			index:     0,
			wantPath:  "",
			wantError: true,
		},
		{
			name:      "invalid_change",
			purpose:   PurposeBIP44,
			coinType:  0,
			account:   0,
			change:    2,
			index:     0,
			wantPath:  "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := BuildDerivationPath(tt.purpose, tt.coinType, tt.account, tt.change, tt.index)
			if (err != nil) != tt.wantError {
				t.Errorf("BuildDerivationPath() error = %v, wantErr %v", err, tt.wantError)
				return
			}
			if path != tt.wantPath {
				t.Errorf("BuildDerivationPath() = %v, want %v", path, tt.wantPath)
			}
		})
	}
}

// TestGetDefaultDerivationPath tests default path generation
func TestGetDefaultDerivationPath(t *testing.T) {
	path, err := GetDefaultDerivationPath(NetworkMainnet, 0, 0, 0)
	if err != nil {
		t.Fatalf("GetDefaultDerivationPath() error = %v", err)
	}

	expected := "m/44'/0'/0'/0/0"
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

// TestDeriveAddresses_InvalidMnemonic tests with invalid mnemonic
func TestDeriveAddresses_InvalidMnemonic(t *testing.T) {
	req := &DeriveAddressesRequest{
		Mnemonic:   "invalid mnemonic phrase",
		Network:    NetworkMainnet,
		StartIndex: 0,
		Count:      1,
	}

	_, err := DeriveAddresses(req)
	if err == nil {
		t.Error("Expected error for invalid mnemonic")
	}
}

// TestNetworkType_String tests network type constants
func TestNetworkType_String(t *testing.T) {
	if NetworkMainnet != "mainnet" {
		t.Errorf("NetworkMainnet = %v, want mainnet", NetworkMainnet)
	}
	if NetworkTestnet != "testnet" {
		t.Errorf("NetworkTestnet = %v, want testnet", NetworkTestnet)
	}
}
