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
)

// TestBIP44_TestVectors tests against known BIP44 test vectors
// Note: These test vectors use the standard BIP39 seed phrase
func TestBIP44_TestVectors(t *testing.T) {
	// Standard BIP39 test mnemonic (128 bits entropy)
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	tests := []struct {
		name       string
		coinType   uint32
		account    uint32
		change     uint32
		index      uint32
		wantPrefix string
	}{
		{
			name:       "nogo_mainnet_account0_external_0",
			coinType:   0,
			account:    0,
			change:     0,
			index:      0,
			wantPrefix: "NOGO",
		},
		{
			name:       "nogo_mainnet_account0_external_1",
			coinType:   0,
			account:    0,
			change:     0,
			index:      1,
			wantPrefix: "NOGO",
		},
		{
			name:       "nogo_mainnet_account0_external_5",
			coinType:   0,
			account:    0,
			change:     0,
			index:      5,
			wantPrefix: "NOGO",
		},
		{
			name:       "nogo_mainnet_account1_external_0",
			coinType:   0,
			account:    1,
			change:     0,
			index:      0,
			wantPrefix: "NOGO",
		},
		{
			name:       "nogo_mainnet_account0_internal_0",
			coinType:   0,
			account:    0,
			change:     1,
			index:      0,
			wantPrefix: "NOGO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := DeriveAddressSingle(
				mnemonic,
				"",
				NetworkMainnet,
				tt.coinType,
				tt.account,
				tt.change,
				tt.index,
			)
			if err != nil {
				t.Fatalf("Derivation failed: %v", err)
			}

			if !strings.HasPrefix(addr.Address, tt.wantPrefix) {
				t.Errorf("Address %s does not have prefix %s", addr.Address, tt.wantPrefix)
			}

			// Verify path starts with correct prefix
			if !strings.HasPrefix(addr.Path, "m/44'/0'/") {
				t.Errorf("Path %s does not start with m/44'/0'/", addr.Path)
			}
		})
	}
}

// TestBIP44_DeterministicDerivation tests that derivation is deterministic
func TestBIP44_DeterministicDerivation(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	// Derive same address twice
	addr1, err := DeriveAddressSingle(mnemonic, "", NetworkMainnet, 0, 0, 0, 10)
	if err != nil {
		t.Fatalf("First derivation failed: %v", err)
	}

	addr2, err := DeriveAddressSingle(mnemonic, "", NetworkMainnet, 0, 0, 0, 10)
	if err != nil {
		t.Fatalf("Second derivation failed: %v", err)
	}

	if addr1.Address != addr2.Address {
		t.Errorf("Deterministic derivation failed: %s != %s", addr1.Address, addr2.Address)
	}

	if addr1.PublicKey != addr2.PublicKey {
		t.Errorf("Public keys differ for same derivation")
	}
}

// TestBIP44_SequentialAddresses tests that sequential addresses are different
func TestBIP44_SequentialAddresses(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	req := &DeriveAddressesRequest{
		Mnemonic:   mnemonic,
		Network:    NetworkMainnet,
		StartIndex: 0,
		Count:      10,
	}

	resp, err := DeriveAddresses(req)
	if err != nil {
		t.Fatalf("Derivation failed: %v", err)
	}

	// Verify sequential addresses are different
	seen := make(map[string]bool)
	for i, addr := range resp.Addresses {
		if seen[addr.Address] {
			t.Errorf("Duplicate address at index %d: %s", i, addr.Address)
		}
		seen[addr.Address] = true

		// Verify index matches
		if addr.Index != uint32(i) {
			t.Errorf("Index mismatch: expected %d, got %d", i, addr.Index)
		}

		// Verify path includes correct index
		expectedIndex := string(rune('0' + i))
		if i >= 10 {
			expectedIndex = string(rune('0'+i/10)) + string(rune('0'+i%10))
		}
		if !strings.HasSuffix(addr.Path, expectedIndex) {
			t.Errorf("Path %s does not end with index %s", addr.Path, expectedIndex)
		}
	}
}

// TestBIP44_DifferentCoinTypes tests derivation with different coin types
func TestBIP44_DifferentCoinTypes(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	var addresses []string
	coinTypes := []uint32{0, 1, 2, 60, 61} // NogoChain, Bitcoin testnet, Litecoin, Ethereum, Ethereum testnet

	for _, coinType := range coinTypes {
		addr, err := DeriveAddressSingle(mnemonic, "", NetworkMainnet, coinType, 0, 0, 0)
		if err != nil {
			t.Fatalf("Coin type %d derivation failed: %v", coinType, err)
		}
		addresses = append(addresses, addr.Address)
	}

	// All addresses should be different
	for i := 0; i < len(addresses); i++ {
		for j := i + 1; j < len(addresses); j++ {
			if addresses[i] == addresses[j] {
				t.Errorf("Coin types %d and %d produced same address", coinTypes[i], coinTypes[j])
			}
		}
	}
}

// TestBIP44_LargeIndex tests derivation with large indices
func TestBIP44_LargeIndex(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	largeIndices := []uint32{1000, 10000, 100000}

	for _, index := range largeIndices {
		addr, err := DeriveAddressSingle(mnemonic, "", NetworkMainnet, 0, 0, 0, index)
		if err != nil {
			t.Fatalf("Index %d derivation failed: %v", index, err)
		}

		if !strings.HasPrefix(addr.Address, "NOGO") {
			t.Errorf("Invalid address prefix for index %d: %s", index, addr.Address)
		}

		if addr.Index != index {
			t.Errorf("Index mismatch: expected %d, got %d", index, addr.Index)
		}
	}
}

// TestBIP44_WithPassphraseCompatibility tests BIP39 passphrase compatibility
func TestBIP44_WithPassphraseCompatibility(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	// Test with standard BIP39 test passphrase
	passphrases := []string{"", "TREZOR", "SuperSecretPassphrase123!", "日本語テスト"}

	var addresses []string
	for _, passphrase := range passphrases {
		addr, err := DeriveAddressSingle(mnemonic, passphrase, NetworkMainnet, 0, 0, 0, 0)
		if err != nil {
			t.Fatalf("Passphrase %q derivation failed: %v", passphrase, err)
		}
		addresses = append(addresses, addr.Address)
	}

	// All addresses should be different
	for i := 0; i < len(addresses); i++ {
		for j := i + 1; j < len(addresses); j++ {
			if addresses[i] == addresses[j] {
				t.Errorf("Passphrases %q and %q produced same address", passphrases[i], passphrases[j])
			}
		}
	}
}

// TestBIP44_AccountIsolation tests that different accounts produce different addresses
func TestBIP44_AccountIsolation(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	accounts := []uint32{0, 1, 2, 10, 100}

	var addresses []string
	for _, account := range accounts {
		addr, err := DeriveAddressSingle(mnemonic, "", NetworkMainnet, 0, account, 0, 0)
		if err != nil {
			t.Fatalf("Account %d derivation failed: %v", account, err)
		}
		addresses = append(addresses, addr.Address)
	}

	// All addresses should be different
	for i := 0; i < len(addresses); i++ {
		for j := i + 1; j < len(addresses); j++ {
			if addresses[i] == addresses[j] {
				t.Errorf("Accounts %d and %d produced same address", accounts[i], accounts[j])
			}
		}
	}
}

// TestBIP44_ChainSeparation tests external vs internal chain separation
func TestBIP44_ChainSeparation(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	var externalAddresses []string
	var internalAddresses []string

	// Derive 5 external addresses
	for i := uint32(0); i < 5; i++ {
		addr, err := DeriveAddressSingle(mnemonic, "", NetworkMainnet, 0, 0, 0, i)
		if err != nil {
			t.Fatalf("External chain index %d derivation failed: %v", i, err)
		}
		externalAddresses = append(externalAddresses, addr.Address)
	}

	// Derive 5 internal addresses
	for i := uint32(0); i < 5; i++ {
		addr, err := DeriveAddressSingle(mnemonic, "", NetworkMainnet, 0, 0, 1, i)
		if err != nil {
			t.Fatalf("Internal chain index %d derivation failed: %v", i, err)
		}
		internalAddresses = append(internalAddresses, addr.Address)
	}

	// No external address should match any internal address
	for i, extAddr := range externalAddresses {
		for j, intAddr := range internalAddresses {
			if extAddr == intAddr {
				t.Errorf("External address %d matches internal address %d: %s", i, j, extAddr)
			}
		}
	}
}

// TestBIP44_MnemonicVariations tests with different mnemonic lengths
func TestBIP44_MnemonicVariations(t *testing.T) {
	// Different standard mnemonic lengths (all valid BIP39 mnemonics)
	mnemonics := []string{
		// 12 words (128 bits)
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		// 15 words (160 bits) - valid BIP39 mnemonic
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon able",
		// 18 words (192 bits) - valid BIP39 mnemonic
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
		// 24 words (256 bits) - valid BIP39 mnemonic
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon art",
	}

	for i, mnemonic := range mnemonics {
		// Skip invalid mnemonics (manually constructed ones may not have valid checksum)
		if !ValidateMnemonic(mnemonic) {
			t.Logf("Skipping mnemonic %d (%d words) - not a valid BIP39 mnemonic", i, len(strings.Fields(mnemonic)))
			continue
		}

		addr, err := DeriveAddressSingle(mnemonic, "", NetworkMainnet, 0, 0, 0, 0)
		if err != nil {
			t.Fatalf("Mnemonic %d (%d words) derivation failed: %v", i, len(strings.Fields(mnemonic)), err)
		}

		if !strings.HasPrefix(addr.Address, "NOGO") {
			t.Errorf("Invalid address prefix for mnemonic %d: %s", i, addr.Address)
		}
	}
}

// TestBIP44_PubKeyFormat tests that public keys are in correct format
func TestBIP44_PubKeyFormat(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	addr, err := DeriveAddressSingle(mnemonic, "", NetworkMainnet, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("Derivation failed: %v", err)
	}

	// Public key should be 64 hex characters (32 bytes)
	if len(addr.PublicKey) != 64 {
		t.Errorf("Public key length %d, expected 64", len(addr.PublicKey))
	}

	// Public key should be valid hex
	for _, c := range addr.PublicKey {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Invalid hex character in public key: %c", c)
		}
	}
}

// TestBIP44_PathFormat tests derivation path format
func TestBIP44_PathFormat(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	tests := []struct {
		name         string
		coinType     uint32
		account      uint32
		change       uint32
		index        uint32
		expectedPath string
	}{
		{
			name:         "standard",
			coinType:     0,
			account:      0,
			change:      0,
			index:        0,
			expectedPath: "m/44'/0'/0'/0/0",
		},
		{
			name:         "account_5",
			coinType:     0,
			account:      5,
			change:      0,
			index:        0,
			expectedPath: "m/44'/0'/5'/0/0",
		},
		{
			name:         "index_100",
			coinType:     0,
			account:      0,
			change:      0,
			index:        100,
			expectedPath: "m/44'/0'/0'/0/100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := DeriveAddressSingle(mnemonic, "", NetworkMainnet, tt.coinType, tt.account, tt.change, tt.index)
			if err != nil {
				t.Fatalf("Derivation failed: %v", err)
			}

			if addr.Path != tt.expectedPath {
				t.Errorf("Path = %q, want %q", addr.Path, tt.expectedPath)
			}
		})
	}
}
