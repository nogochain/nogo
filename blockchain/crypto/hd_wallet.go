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
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	// BIP39SeedLength is the expected BIP39 seed length in bytes
	BIP39SeedLength = 64

	// BIP32HardenedOffset is the offset for hardened keys
	BIP32HardenedOffset = 0x80000000

	// HDKeyLen is the length of Ed25519 private key
	HDKeyLen = 32

	// HDChainCodeLen is the length of chain code
	HDChainCodeLen = 32

	// MasterKeyPath is the root derivation path
	MasterKeyPath = "m"

	// DefaultDerivationPath is the default path for NogoChain (BIP44-style)
	// m/44'/0'/0'/0/0 where 0 is the coin type for NogoChain
	DefaultDerivationPath = "m/44'/0'/0'/0/0"
)

var (
	// ErrInvalidSeedLength is returned when seed is too short
	ErrInvalidSeedLength = errors.New("seed too short for HD wallet")

	// ErrInvalidDerivationPath is returned for malformed paths
	ErrInvalidDerivationPath = errors.New("invalid derivation path")

	// ErrInvalidChildIndex is returned for invalid child index
	ErrInvalidChildIndex = errors.New("invalid child index")

	// ed25519CurveName is the curve name for Ed25519
	ed25519CurveName = []byte("ed25519 seed")
)

// HDWallet represents a hierarchical deterministic wallet
// Production-grade: BIP32-compliant for Ed25519
// Concurrency safety: immutable after creation
type HDWallet struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	ChainCode  []byte
	Depth      uint8
	Index      uint32
	ParentFP   []byte
	Parent     *HDWallet
	PathStr    string
}

// DerivationPath represents a parsed BIP32 derivation path
type DerivationPath struct {
	Components []uint32
	Original   string
}

// NewHDWallet creates a master HD wallet from seed
// BIP32 compliant: uses HMAC-SHA512 with "ed25519 seed"
func NewHDWallet(seed []byte) (*HDWallet, error) {
	if len(seed) < BIP39SeedLength {
		return nil, fmt.Errorf("%w: got %d, need %d", ErrInvalidSeedLength, len(seed), BIP39SeedLength)
	}

	h := hmac.New(sha512.New, ed25519CurveName)
	h.Write(seed)
	I := h.Sum(nil)

	key := make([]byte, HDKeyLen)
	chainCode := make([]byte, HDChainCodeLen)
	copy(key, I[:HDKeyLen])
	copy(chainCode, I[HDKeyLen:])

	priv := ed25519.NewKeyFromSeed(key)
	pub := priv.Public().(ed25519.PublicKey)

	return &HDWallet{
		PrivateKey: priv,
		PublicKey:  pub,
		ChainCode:  chainCode,
		Depth:      0,
		Index:      0,
		ParentFP:   nil,
	}, nil
}

// Derive derives a child wallet at the specified path
// BIP32 compliant: supports hardened (') and non-hardened derivation
func (w *HDWallet) Derive(path string) (*HDWallet, error) {
	parsed, err := ParseDerivationPath(path)
	if err != nil {
		return nil, err
	}

	current := w
	for _, index := range parsed.Components {
		isHardened := index >= BIP32HardenedOffset

		childIndex := index
		if isHardened {
			childIndex = index - BIP32HardenedOffset
		}

		current, err = current.DeriveChild(childIndex, isHardened, index)
		if err != nil {
			return nil, fmt.Errorf("failed to derive child %d: %w", childIndex, err)
		}
	}

	return current, nil
}

// DeriveChild derives a child wallet at a specific index
// Security: uses HMAC-SHA512 for key derivation
func (w *HDWallet) DeriveChild(index uint32, hardened bool, fullPathIndex uint32) (*HDWallet, error) {
	if index > 0x7FFFFFFF {
		return nil, ErrInvalidChildIndex
	}

	var data []byte
	if hardened {
		data = make([]byte, 1+HDKeyLen+4)
		data[0] = 0x00
		copy(data[1:33], w.PrivateKey.Seed())
		binary.BigEndian.PutUint32(data[33:], index+BIP32HardenedOffset)
	} else {
		data = make([]byte, HDChainCodeLen+4)
		copy(data[:32], w.PublicKey)
		binary.BigEndian.PutUint32(data[32:], index)
	}

	h := hmac.New(sha512.New, w.ChainCode)
	h.Write(data)
	I := h.Sum(nil)

	childKey := make([]byte, HDKeyLen)
	childChainCode := make([]byte, HDChainCodeLen)
	copy(childKey, I[:HDKeyLen])
	copy(childChainCode, I[HDKeyLen:])

	for i := range childKey {
		childKey[i] ^= w.PrivateKey.Seed()[i]
	}

	childPriv := ed25519.NewKeyFromSeed(childKey)
	childPub := childPriv.Public().(ed25519.PublicKey)

	parentFP := make([]byte, 4)
	if len(w.PublicKey) >= 4 {
		copy(parentFP, w.PublicKey[:4])
	}

	return &HDWallet{
		PrivateKey: childPriv,
		PublicKey:  childPub,
		ChainCode:  childChainCode,
		Depth:      w.Depth + 1,
		Index:      fullPathIndex,
		ParentFP:   parentFP,
		Parent:     w,
	}, nil
}

// ParseDerivationPath parses a BIP32-style derivation path
// Supports: m/44'/0'/0'/0/0 or 44'/0'/0'/0/0
func ParseDerivationPath(path string) (*DerivationPath, error) {
	original := path
	path = strings.TrimSpace(path)

	if path == "" || path == "m" || path == "m/" {
		return &DerivationPath{
			Components: []uint32{},
			Original:   original,
		}, nil
	}

	if len(path) >= 2 && path[:2] == "m/" {
		path = path[2:]
	}

	parts := strings.Split(path, "/")
	components := make([]uint32, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		isHardened := strings.HasSuffix(part, "'")
		if isHardened {
			part = part[:len(part)-1]
		}

		var index uint32
		if _, err := fmt.Sscanf(part, "%d", &index); err != nil {
			return nil, fmt.Errorf("%w: invalid component %q", ErrInvalidDerivationPath, part)
		}

		if isHardened {
			index += BIP32HardenedOffset
		}

		components = append(components, index)
	}

	return &DerivationPath{
		Components: components,
		Original:   original,
	}, nil
}

// Address returns the wallet address
func (w *HDWallet) Address() string {
	return GenerateAddress(w.PublicKey)
}

// PrivateKeyHex returns private key as hex string
func (w *HDWallet) PrivateKeyHex() string {
	return hex.EncodeToString(w.PrivateKey)
}

// PublicKeyHex returns public key as hex string
func (w *HDWallet) PublicKeyHex() string {
	return hex.EncodeToString(w.PublicKey)
}

// ChainCodeHex returns chain code as hex string
func (w *HDWallet) ChainCodeHex() string {
	return hex.EncodeToString(w.ChainCode)
}

// Fingerprint returns the 4-byte fingerprint of the public key
func (w *HDWallet) Fingerprint() []byte {
	if len(w.PublicKey) < 4 {
		return nil
	}
	fp := make([]byte, 4)
	copy(fp, w.PublicKey[:4])
	return fp
}

// Identifier returns the identifier (pubkey hash)
func (w *HDWallet) Identifier() []byte {
	h := sha256.Sum256(w.PublicKey)
	return h[:]
}

// IsHardened checks if this key is hardened
func (w *HDWallet) IsHardened() bool {
	return w.Index >= BIP32HardenedOffset
}

// Path returns the full path as string
func (w *HDWallet) Path() string {
	if w.PathStr != "" {
		return w.PathStr
	}

	parts := []string{"m"}
	current := w

	path := []string{}
	for current != nil && current.Depth > 0 {
		index := current.Index
		component := ""
		if current.IsHardened() {
			component = fmt.Sprintf("%d'", index-BIP32HardenedOffset)
		} else {
			component = fmt.Sprintf("%d", index)
		}
		path = append([]string{component}, path...)
		current = current.Parent
	}

	parts = append(parts, path...)
	w.PathStr = strings.Join(parts, "/")
	return w.PathStr
}

// WalletFromMnemonic creates a wallet from mnemonic phrase
// BIP44 compliant: uses m/44'/coinType'/0'/0/0 path
func WalletFromMnemonic(mnemonic, passphrase string, coinType uint32) (*Wallet, error) {
	seed, err := MnemonicToSeed(mnemonic, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to derive seed: %w", err)
	}

	hdWallet, err := NewHDWallet(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to create HD wallet: %w", err)
	}

	path := fmt.Sprintf("m/44'/%d'/0'/0/0", coinType)
	derived, err := hdWallet.Derive(path)
	if err != nil {
		return nil, fmt.Errorf("failed to derive path %s: %w", path, err)
	}

	return &Wallet{
		PrivateKey: derived.PrivateKey,
		PublicKey:  derived.PublicKey,
		Address:    GenerateAddress(derived.PublicKey),
	}, nil
}

// WalletFromMnemonicDefault creates wallet with default coin type
func WalletFromMnemonicDefault(mnemonic, passphrase string) (*Wallet, error) {
	return WalletFromMnemonic(mnemonic, passphrase, 0)
}
