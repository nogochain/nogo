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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/crypto/hkdf"
)

const (
	// WalletVersion is the current wallet structure version
	WalletVersion = 1

	// MinSeedLength is the minimum seed length in bytes
	MinSeedLength = 32

	// MaxSeedLength is the maximum seed length in bytes
	MaxSeedLength = 128
)

var (
	// ErrInvalidPrivateKey is returned when private key is invalid
	ErrInvalidPrivateKey = errors.New("invalid private key")

	// ErrInvalidPublicKey is returned when public key is invalid
	ErrInvalidPublicKey = errors.New("invalid public key")

	// ErrInvalidSignature is returned when signature verification fails
	ErrInvalidSignature = errors.New("invalid signature")

	// ErrSeedTooShort is returned when seed is too short
	ErrSeedTooShort = errors.New("seed too short")

	// ErrSeedTooLong is returned when seed is too long
	ErrSeedTooLong = errors.New("seed too long")

	// ErrInvalidWalletVersion is returned when wallet version is unsupported
	ErrInvalidWalletVersion = errors.New("unsupported wallet version")

	// Rand is the secure random reader for cryptographic operations
	Rand = rand.Reader
)

// Wallet represents a NogoChain wallet with Ed25519 keys
// Production-grade: includes all necessary fields for secure wallet operations
// Concurrency safety: immutable after creation, safe for concurrent reads
type Wallet struct {
	mu sync.RWMutex

	Version    int                `json:"version"`
	PrivateKey ed25519.PrivateKey `json:"-"`
	PublicKey  ed25519.PublicKey  `json:"publicKey"`
	Address    string             `json:"address"`
	Label      string             `json:"label,omitempty"`
}

// NewWallet creates a new wallet with random Ed25519 key pair
// Security: uses crypto/rand for secure random generation
func NewWallet() (*Wallet, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	return &Wallet{
		Version:    WalletVersion,
		PrivateKey: priv,
		PublicKey:  pub,
		Address:    GenerateAddress(pub),
	}, nil
}

// NewWalletFromSeed creates a wallet from a deterministic seed
// Security: uses HKDF for key derivation from seed
func NewWalletFromSeed(seed []byte) (*Wallet, error) {
	if len(seed) < MinSeedLength {
		return nil, fmt.Errorf("%w: got %d, need at least %d", ErrSeedTooShort, len(seed), MinSeedLength)
	}

	if len(seed) > MaxSeedLength {
		return nil, fmt.Errorf("%w: got %d, max %d", ErrSeedTooLong, len(seed), MaxSeedLength)
	}

	h := hkdf.New(sha256.New, seed, nil, []byte("NogoChain wallet key derivation"))
	keySeed := make([]byte, ed25519.SeedSize)
	if _, err := h.Read(keySeed); err != nil {
		return nil, fmt.Errorf("failed to derive key seed: %w", err)
	}

	priv := ed25519.NewKeyFromSeed(keySeed)
	pub := priv.Public().(ed25519.PublicKey)

	return &Wallet{
		Version:    WalletVersion,
		PrivateKey: priv,
		PublicKey:  pub,
		Address:    GenerateAddress(pub),
	}, nil
}

// WalletFromPrivateKeyBase64 creates a wallet from a base64-encoded private key
// Production-grade: validates key length and format
func WalletFromPrivateKeyBase64(privB64 string) (*Wallet, error) {
	raw, err := base64.StdEncoding.DecodeString(privB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: got %d, expected %d", ErrInvalidPrivateKey, len(raw), ed25519.PrivateKeySize)
	}

	priv := ed25519.PrivateKey(raw)
	pub := priv.Public().(ed25519.PublicKey)

	return &Wallet{
		Version:    WalletVersion,
		PrivateKey: priv,
		PublicKey:  pub,
		Address:    GenerateAddress(pub),
	}, nil
}

// WalletFromPrivateKeyHex creates a wallet from a hex-encoded private key
func WalletFromPrivateKeyHex(privHex string) (*Wallet, error) {
	raw, err := hex.DecodeString(privHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex: %w", err)
	}

	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: got %d, expected %d", ErrInvalidPrivateKey, len(raw), ed25519.PrivateKeySize)
	}

	priv := ed25519.PrivateKey(raw)
	pub := priv.Public().(ed25519.PublicKey)

	return &Wallet{
		Version:    WalletVersion,
		PrivateKey: priv,
		PublicKey:  pub,
		Address:    GenerateAddress(pub),
	}, nil
}

// WalletFromPrivateKeyBytes creates a wallet from raw private key bytes
// Security: makes a copy of the private key to prevent external modification
func WalletFromPrivateKeyBytes(raw []byte) (*Wallet, error) {
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: got %d, expected %d", ErrInvalidPrivateKey, len(raw), ed25519.PrivateKeySize)
	}

	priv := make([]byte, len(raw))
	copy(priv, raw)

	pub := ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)

	return &Wallet{
		Version:    WalletVersion,
		PrivateKey: ed25519.PrivateKey(priv),
		PublicKey:  pub,
		Address:    GenerateAddress(pub),
	}, nil
}

// PrivateKeyBase64 returns the private key as base64-encoded string
// Security warning: handle with care, private key is sensitive
func (w *Wallet) PrivateKeyBase64() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return base64.StdEncoding.EncodeToString(w.PrivateKey)
}

// PrivateKeyHex returns the private key as hex-encoded string
func (w *Wallet) PrivateKeyHex() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return hex.EncodeToString(w.PrivateKey)
}

// PublicKeyBase64 returns the public key as base64-encoded string
func (w *Wallet) PublicKeyBase64() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return base64.StdEncoding.EncodeToString(w.PublicKey)
}

// PublicKeyHex returns the public key as hex-encoded string
func (w *Wallet) PublicKeyHex() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return hex.EncodeToString(w.PublicKey)
}

// Sign signs a message with the wallet's private key
// Production-grade: returns signature and error
// Security: uses Ed25519 deterministic signatures
func (w *Wallet) Sign(message []byte) ([]byte, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.PrivateKey == nil {
		return nil, ErrInvalidPrivateKey
	}

	signature := ed25519.Sign(w.PrivateKey, message)
	return signature, nil
}

// SignHex signs a message and returns hex-encoded signature
func (w *Wallet) SignHex(message []byte) (string, error) {
	signature, err := w.Sign(message)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(signature), nil
}

// SignBase64 signs a message and returns base64-encoded signature
func (w *Wallet) SignBase64(message []byte) (string, error) {
	signature, err := w.Sign(message)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// Verify verifies a signature against a message using the wallet's public key
// Security: constant-time signature verification
func (w *Wallet) Verify(message, signature []byte) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.PublicKey == nil || len(signature) != ed25519.SignatureSize {
		return false
	}

	return ed25519.Verify(w.PublicKey, message, signature)
}

// VerifyHex verifies a hex-encoded signature
func (w *Wallet) VerifyHex(message []byte, signatureHex string) bool {
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	return w.Verify(message, signature)
}

// VerifyBase64 verifies a base64-encoded signature
func (w *Wallet) VerifyBase64(message []byte, signatureB64 string) bool {
	signature, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return false
	}
	return w.Verify(message, signature)
}

// SignAndVerify signs a message and immediately verifies it
// Production-grade: sanity check for signing operations
func (w *Wallet) SignAndVerify(message []byte) ([]byte, error) {
	signature, err := w.Sign(message)
	if err != nil {
		return nil, err
	}

	if !w.Verify(message, signature) {
		return nil, ErrInvalidSignature
	}

	return signature, nil
}

// GetAddress returns the wallet address
// Concurrency safety: thread-safe read operation
func (w *Wallet) GetAddress() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.Address
}

// GetPublicKey returns a copy of the public key
// Security: returns copy to prevent external modification
func (w *Wallet) GetPublicKey() []byte {
	w.mu.RLock()
	defer w.mu.RUnlock()
	pubCopy := make([]byte, len(w.PublicKey))
	copy(pubCopy, w.PublicKey)
	return pubCopy
}

// SetLabel sets a human-readable label for the wallet
func (w *Wallet) SetLabel(label string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Label = label
}

// GetLabel returns the wallet label
func (w *Wallet) GetLabel() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.Label
}

// ClearPrivateKey zeros out the private key from memory
// Security: use this when wallet is no longer needed
func (w *Wallet) ClearPrivateKey() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.PrivateKey != nil {
		for i := range w.PrivateKey {
			w.PrivateKey[i] = 0
		}
		w.PrivateKey = nil
	}
}

// Export exports wallet data (excluding private key)
// Production-grade: safe for serialization
func (w *Wallet) Export() map[string]interface{} {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return map[string]interface{}{
		"version":   w.Version,
		"publicKey": w.PublicKeyHex(),
		"address":   w.Address,
		"label":     w.Label,
	}
}

// ValidateWallet validates wallet structure
// Logic completeness: checks all required fields
func ValidateWallet(w *Wallet) error {
	if w == nil {
		return errors.New("nil wallet")
	}

	if w.Version != WalletVersion {
		return fmt.Errorf("%w: got %d, expected %d", ErrInvalidWalletVersion, w.Version, WalletVersion)
	}

	if len(w.PrivateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("%w: length %d", ErrInvalidPrivateKey, len(w.PrivateKey))
	}

	if len(w.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: length %d", ErrInvalidPublicKey, len(w.PublicKey))
	}

	if err := ValidateAddress(w.Address); err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	expectedAddress := GenerateAddress(w.PublicKey)
	if w.Address != expectedAddress {
		return errors.New("address mismatch with public key")
	}

	return nil
}
