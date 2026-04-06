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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const (
	// MinMultisigSigners is the minimum number of signers
	MinMultisigSigners = 1

	// MaxMultisigSigners is the maximum number of signers
	MaxMultisigSigners = 100

	// MinRequiredSigners is the minimum required signatures
	MinRequiredSigners = 1

	// SignatureSize is the size of Ed25519 signature in bytes
	SignatureSize = 64

	// MultisigAddressPrefix is the prefix for multisig addresses
	MultisigAddressPrefix = "NOGOM"
)

var (
	// ErrInvalidSignerCount is returned for invalid signer count
	ErrInvalidSignerCount = errors.New("invalid number of signers")

	// ErrInvalidRequiredCount is returned for invalid required count
	ErrInvalidRequiredCount = errors.New("invalid required signature count")

	// ErrRequiredGreaterThanSigners is returned when required > signers
	ErrRequiredGreaterThanSigners = errors.New("required signatures cannot exceed signers")

	// ErrDuplicatePublicKey is returned for duplicate public keys
	ErrDuplicatePublicKey = errors.New("duplicate public key in signers")

	// ErrInvalidPublicKey is returned for invalid public key format
	ErrInvalidPublicKeyFormat = errors.New("invalid public key format")

	// ErrMissingSignature is returned when signature is missing
	ErrMissingSignature = errors.New("missing signature")

	// ErrInvalidSignature is returned for invalid signature
	ErrInvalidSignatureFormat = errors.New("invalid signature")

	// ErrInsufficientSignatures is returned when not enough signatures
	ErrInsufficientSignatures = errors.New("insufficient signatures")

	// ErrSignatureVerificationFailed is returned when signature fails verification
	ErrSignatureVerificationFailed = errors.New("signature verification failed")

	// ErrSignerNotFound is returned when signer is not in the list
	ErrSignerNotFound = errors.New("signer not found in multisig")
)

// MultisigConfig represents a multisig wallet configuration
// Production-grade: immutable after creation
type MultisigConfig struct {
	Required   int      `json:"required"`
	PublicKeys []string `json:"publicKeys"`
	Address    string   `json:"address"`
	CreatedAt  int64    `json:"createdAt,omitempty"`

	// mu protects concurrent access
	mu sync.RWMutex
}

// MultisigSignature represents a single signature in a multisig transaction
type MultisigSignature struct {
	SignerIndex int    `json:"signerIndex"`
	PublicKey   string `json:"publicKey"`
	Signature   string `json:"signature"`
}

// MultisigTransaction represents a multisig transaction with signatures
type MultisigTransaction struct {
	Data       []byte              `json:"data"`
	Signatures []MultisigSignature `json:"signatures"`
	Config     *MultisigConfig     `json:"config"`
}

// NewMultisigConfig creates a new multisig configuration
// Production-grade: validates all parameters
func NewMultisigConfig(required int, publicKeys []string) (*MultisigConfig, error) {
	if len(publicKeys) < MinMultisigSigners {
		return nil, fmt.Errorf("%w: minimum %d", ErrInvalidSignerCount, MinMultisigSigners)
	}

	if len(publicKeys) > MaxMultisigSigners {
		return nil, fmt.Errorf("%w: maximum %d", ErrInvalidSignerCount, MaxMultisigSigners)
	}

	if required < MinRequiredSigners {
		return nil, fmt.Errorf("%w: minimum %d", ErrInvalidRequiredCount, MinRequiredSigners)
	}

	if required > len(publicKeys) {
		return nil, ErrRequiredGreaterThanSigners
	}

	pubKeys := make([]string, len(publicKeys))
	for i, pk := range publicKeys {
		if err := validatePublicKeyHex(pk); err != nil {
			return nil, fmt.Errorf("public key %d: %w", i, err)
		}
		pubKeys[i] = strings.ToLower(strings.TrimSpace(pk))
	}

	if err := checkDuplicateKeys(pubKeys); err != nil {
		return nil, err
	}

	sort.Strings(pubKeys)

	address := GenerateMultisigAddress(required, pubKeys)

	return &MultisigConfig{
		Required:   required,
		PublicKeys: pubKeys,
		Address:    address,
	}, nil
}

// NewMultisigConfigFromPubKeyBytes creates multisig from raw public keys
func NewMultisigConfigFromPubKeyBytes(required int, pubKeyBytes [][]byte) (*MultisigConfig, error) {
	publicKeys := make([]string, len(pubKeyBytes))
	for i, pk := range pubKeyBytes {
		if len(pk) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: key %d has length %d", ErrInvalidPublicKeyFormat, i, len(pk))
		}
		publicKeys[i] = hex.EncodeToString(pk)
	}

	return NewMultisigConfig(required, publicKeys)
}

// GetPublicKey returns the public key at specified index
func (m *MultisigConfig) GetPublicKey(index int) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if index < 0 || index >= len(m.PublicKeys) {
		return "", fmt.Errorf("invalid signer index: %d", index)
	}

	return m.PublicKeys[index], nil
}

// GetRequired returns the required signature count
func (m *MultisigConfig) GetRequired() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Required
}

// GetSignerCount returns the total number of signers
func (m *MultisigConfig) GetSignerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.PublicKeys)
}

// GetAddress returns the multisig address
func (m *MultisigConfig) GetAddress() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Address
}

// GenerateMultisigAddress creates a unique address for the multisig configuration
// Production-grade: uses SHA256 of sorted public keys + required count
func GenerateMultisigAddress(required int, publicKeys []string) string {
	data := fmt.Sprintf("%d:", required)
	for i, pk := range publicKeys {
		if i > 0 {
			data += ","
		}
		data += pk
	}

	hash := sha256.Sum256([]byte(data))
	addressHash := hex.EncodeToString(hash[:])

	return fmt.Sprintf("%s%s", MultisigAddressPrefix, addressHash)
}

// ValidateMultisigAddress validates a multisig address format
func ValidateMultisigAddress(addr string) error {
	if len(addr) <= len(MultisigAddressPrefix) {
		return errors.New("multisig address too short")
	}

	if addr[:len(MultisigAddressPrefix)] != MultisigAddressPrefix {
		return fmt.Errorf("invalid prefix: expected %s", MultisigAddressPrefix)
	}

	encoded := addr[len(MultisigAddressPrefix):]
	if len(encoded) != 64 {
		return fmt.Errorf("invalid address hash length: %d", len(encoded))
	}

	_, err := hex.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("invalid hex encoding: %w", err)
	}

	return nil
}

// SignMultisig signs a transaction for a multisig wallet
func (m *MultisigConfig) SignMultisig(data []byte, wallet *Wallet, signerIndex int) (*MultisigSignature, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if signerIndex < 0 || signerIndex >= len(m.PublicKeys) {
		return nil, fmt.Errorf("invalid signer index: %d", signerIndex)
	}

	expectedPubKey := m.PublicKeys[signerIndex]
	actualPubKey := wallet.PublicKeyHex()

	if strings.ToLower(expectedPubKey) != strings.ToLower(actualPubKey) {
		return nil, ErrSignerNotFound
	}

	signature, err := wallet.Sign(data)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	return &MultisigSignature{
		SignerIndex: signerIndex,
		PublicKey:   actualPubKey,
		Signature:   hex.EncodeToString(signature),
	}, nil
}

// VerifyMultisigSignature verifies a single multisig signature
func (m *MultisigConfig) VerifyMultisigSignature(data []byte, sig *MultisigSignature) error {
	if sig == nil {
		return ErrMissingSignature
	}

	signatureBytes, err := hex.DecodeString(sig.Signature)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSignatureFormat, err)
	}

	if len(signatureBytes) != SignatureSize {
		return fmt.Errorf("%w: expected %d, got %d", ErrInvalidSignatureFormat, SignatureSize, len(signatureBytes))
	}

	pubKeyBytes, err := hex.DecodeString(sig.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}

	if !ed25519.Verify(pubKeyBytes, data, signatureBytes) {
		return ErrSignatureVerificationFailed
	}

	found := false
	for _, pk := range m.PublicKeys {
		pkBytes, _ := hex.DecodeString(pk)
		if hex.EncodeToString(pkBytes) == strings.ToLower(sig.PublicKey) {
			found = true
			break
		}
	}

	if !found {
		return ErrSignerNotFound
	}

	return nil
}

// AggregateMultisigSignatures aggregates multiple signatures
// Production-grade: validates all signatures and checks sufficiency
func (m *MultisigConfig) AggregateMultisigSignatures(data []byte, signatures []MultisigSignature) error {
	if len(signatures) < m.Required {
		return fmt.Errorf("%w: have %d, need %d", ErrInsufficientSignatures, len(signatures), m.Required)
	}

	seenSigners := make(map[int]bool)

	for _, sig := range signatures {
		if err := m.VerifyMultisigSignature(data, &sig); err != nil {
			return fmt.Errorf("signature from signer %d: %w", sig.SignerIndex, err)
		}

		if seenSigners[sig.SignerIndex] {
			return fmt.Errorf("duplicate signature from signer %d", sig.SignerIndex)
		}
		seenSigners[sig.SignerIndex] = true
	}

	return nil
}

// CreateMultisigTransaction creates a multisig transaction
func (m *MultisigConfig) CreateMultisigTransaction(data []byte) *MultisigTransaction {
	return &MultisigTransaction{
		Data:       data,
		Signatures: make([]MultisigSignature, 0),
		Config:     m,
	}
}

// AddSignature adds a signature to a multisig transaction
func (tx *MultisigTransaction) AddSignature(sig MultisigSignature) error {
	if tx.Config == nil {
		return errors.New("transaction has no config")
	}

	if err := tx.Config.VerifyMultisigSignature(tx.Data, &sig); err != nil {
		return err
	}

	for _, existing := range tx.Signatures {
		if existing.SignerIndex == sig.SignerIndex {
			return fmt.Errorf("signature already exists for signer %d", sig.SignerIndex)
		}
	}

	tx.Signatures = append(tx.Signatures, sig)
	return nil
}

// IsComplete checks if transaction has enough signatures
func (tx *MultisigTransaction) IsComplete() bool {
	if tx.Config == nil {
		return false
	}

	return len(tx.Signatures) >= tx.Config.Required
}

// ValidateTransaction validates a complete multisig transaction
func (tx *MultisigTransaction) ValidateTransaction() error {
	if tx.Config == nil {
		return errors.New("transaction has no config")
	}

	if !tx.IsComplete() {
		return fmt.Errorf("%w: have %d, need %d", ErrInsufficientSignatures, len(tx.Signatures), tx.Config.Required)
	}

	return tx.Config.AggregateMultisigSignatures(tx.Data, tx.Signatures)
}

// ExportMultisig exports multisig config as JSON-serializable map
func (m *MultisigConfig) ExportMultisig() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"required":    m.Required,
		"publicKeys":  m.PublicKeys,
		"address":     m.Address,
		"signerCount": len(m.PublicKeys),
	}
}

// validatePublicKeyHex validates a hex-encoded public key
func validatePublicKeyHex(pk string) error {
	pk = strings.TrimSpace(pk)
	if len(pk) == 0 {
		return errors.New("empty public key")
	}

	bytes, err := hex.DecodeString(pk)
	if err != nil {
		return fmt.Errorf("invalid hex: %w", err)
	}

	if len(bytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid length: expected %d, got %d", ed25519.PublicKeySize, len(bytes))
	}

	return nil
}

// checkDuplicateKeys checks for duplicate public keys
func checkDuplicateKeys(keys []string) error {
	seen := make(map[string]bool)
	for i, key := range keys {
		normalized := strings.ToLower(key)
		if seen[normalized] {
			return fmt.Errorf("%w at index %d", ErrDuplicatePublicKey, i)
		}
		seen[normalized] = true
	}
	return nil
}

// EncodeMultisigSignatures encodes signatures to base64
func EncodeMultisigSignatures(signatures []MultisigSignature) (string, error) {
	data := make([]byte, 0, len(signatures)*SignatureSize)
	for _, sig := range signatures {
		sigBytes, err := hex.DecodeString(sig.Signature)
		if err != nil {
			return "", err
		}
		data = append(data, sigBytes...)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// DecodeMultisigSignatures decodes signatures from base64
func DecodeMultisigSignatures(encoded string, publicKeys []string) ([]MultisigSignature, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	if len(data)%SignatureSize != 0 {
		return nil, errors.New("invalid signature data length")
	}

	count := len(data) / SignatureSize
	if count > len(publicKeys) {
		return nil, ErrInvalidSignerCount
	}

	signatures := make([]MultisigSignature, count)
	for i := 0; i < count; i++ {
		signatures[i] = MultisigSignature{
			SignerIndex: i,
			PublicKey:   publicKeys[i],
			Signature:   hex.EncodeToString(data[i*SignatureSize : (i+1)*SignatureSize]),
		}
	}

	return signatures, nil
}
