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
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// KeystoreVersion is the current keystore file format version
	KeystoreVersion = 1

	// DefaultKDFIterations is the default number of PBKDF2 iterations
	// Security: higher iterations increase brute-force resistance
	DefaultKDFIterations = 600_000

	// MinKDFIterations is the minimum acceptable KDF iterations
	MinKDFIterations = 100_000

	// SaltLen is the length of random salt in bytes
	SaltLen = 16

	// KeyLen is the derived key length in bytes (256 bits)
	KeyLen = 32

	// KDFNamePBKDF2 is the name for PBKDF2 KDF
	KDFNamePBKDF2 = "pbkdf2-sha256"

	// CipherNameAESGCM is the name for AES-256-GCM cipher
	CipherNameAESGCM = "aes-256-gcm"
)

var (
	// ErrKeystoreWrongPassword is returned when decryption fails
	ErrKeystoreWrongPassword = errors.New("wrong password")

	// ErrKeystoreInvalidVersion is returned when keystore version is unsupported
	ErrKeystoreInvalidVersion = errors.New("unsupported keystore version")

	// ErrKeystoreInvalidKDF is returned when KDF is unsupported
	ErrKeystoreInvalidKDF = errors.New("unsupported KDF")

	// ErrKeystoreInvalidCipher is returned when cipher is unsupported
	ErrKeystoreInvalidCipher = errors.New("unsupported cipher")

	// ErrKeystoreCorrupted is returned when keystore data is corrupted
	ErrKeystoreCorrupted = errors.New("keystore corrupted")

	// ErrKeystoreWriteFailed is returned when keystore write fails
	ErrKeystoreWriteFailed = errors.New("failed to write keystore")
)

// KeystoreFile represents the on-disk keystore structure
// Production-grade: JSON-serializable with all encryption parameters
type KeystoreFile struct {
	Version int            `json:"version"`
	Address string         `json:"address"`
	KDF     KeystoreKDF    `json:"kdf"`
	Cipher  KeystoreCipher `json:"cipher"`

	// EncryptedMnemonic stores encrypted mnemonic for backup
	// Optional: only present if wallet was created from mnemonic
	EncryptedMnemonic *EncryptedMnemonic `json:"encryptedMnemonic,omitempty"`

	// CreatedAt is the keystore creation timestamp
	CreatedAt int64 `json:"createdAt,omitempty"`

	// CryptoVersion tracks the encryption algorithm version
	CryptoVersion int `json:"cryptoVersion,omitempty"`
}

// EncryptedMnemonic stores an encrypted mnemonic phrase
// Security: uses separate salt/nonce for enhanced security
type EncryptedMnemonic struct {
	SaltB64       string `json:"saltB64"`
	NonceB64      string `json:"nonceB64"`
	CiphertextB64 string `json:"ciphertextB64"`
}

// KeystoreKDF represents key derivation function parameters
type KeystoreKDF struct {
	Name       string `json:"name"`
	Iterations int    `json:"iterations"`
	SaltB64    string `json:"saltB64"`
}

// KeystoreCipher represents cipher parameters
type KeystoreCipher struct {
	Name          string `json:"name"`
	NonceB64      string `json:"nonceB64"`
	CiphertextB64 string `json:"ciphertextB64"`
}

// keystoreCache caches decrypted wallets in memory
// Concurrency safety: protected by mutex
var keystoreCache = struct {
	mu      sync.RWMutex
	wallets map[string]*WalletCacheEntry
}{
	wallets: make(map[string]*WalletCacheEntry),
}

// WalletCacheEntry holds a cached decrypted wallet
type WalletCacheEntry struct {
	Wallet    *Wallet
	ExpiresAt time.Time
	mu        sync.RWMutex
}

// DefaultKeystoreParams returns default KDF parameters
// Production-grade: balances security and performance
func DefaultKeystoreParams() KeystoreKDF {
	return KeystoreKDF{
		Name:       KDFNamePBKDF2,
		Iterations: DefaultKDFIterations,
	}
}

// WriteKeystore writes an encrypted keystore file
// Production-grade: atomic write with temp file + rename
// Security: binds ciphertext to address to prevent swap attacks
func WriteKeystore(path string, w *Wallet, password string, params KeystoreKDF) error {
	return WriteKeystoreWithConfig(path, w, password, "", params)
}

// WriteKeystoreWithConfig writes keystore with optional mnemonic
func WriteKeystoreWithConfig(path string, w *Wallet, password, mnemonic string, params KeystoreKDF) error {
	if w == nil {
		return errors.New("nil wallet")
	}

	if password == "" {
		return errors.New("empty password")
	}

	if params.Name == "" || params.Iterations == 0 {
		params = DefaultKeystoreParams()
	}

	params.Name = strings.ToLower(strings.TrimSpace(params.Name))
	if params.Name != KDFNamePBKDF2 {
		return fmt.Errorf("%w: %s", ErrKeystoreInvalidKDF, params.Name)
	}

	if params.Iterations < MinKDFIterations {
		return fmt.Errorf("kdf iterations too low: %d, minimum %d", params.Iterations, MinKDFIterations)
	}

	salt := make([]byte, SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	key := pbkdf2HMACSHA256([]byte(password), salt, params.Iterations, KeyLen)

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	addrBytes, err := hex.DecodeString(w.Address)
	if err != nil {
		return fmt.Errorf("failed to decode address: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(w.PrivateKey), addrBytes)

	ks := KeystoreFile{
		Version:       KeystoreVersion,
		Address:       w.Address,
		CreatedAt:     time.Now().Unix(),
		CryptoVersion: 1,
		KDF: KeystoreKDF{
			Name:       KDFNamePBKDF2,
			Iterations: params.Iterations,
			SaltB64:    base64.StdEncoding.EncodeToString(salt),
		},
		Cipher: KeystoreCipher{
			Name:          CipherNameAESGCM,
			NonceB64:      base64.StdEncoding.EncodeToString(nonce),
			CiphertextB64: base64.StdEncoding.EncodeToString(ciphertext),
		},
	}

	if mnemonic != "" {
		mnemonicSalt := make([]byte, SaltLen)
		if _, err := rand.Read(mnemonicSalt); err != nil {
			return fmt.Errorf("failed to generate mnemonic salt: %w", err)
		}

		mnemonicKey := pbkdf2HMACSHA256([]byte(password), mnemonicSalt, params.Iterations, KeyLen)

		mnemonicBlock, err := aes.NewCipher(mnemonicKey)
		if err != nil {
			return fmt.Errorf("failed to create mnemonic cipher: %w", err)
		}

		mnemonicGCM, err := cipher.NewGCM(mnemonicBlock)
		if err != nil {
			return fmt.Errorf("failed to create mnemonic GCM: %w", err)
		}

		mnemonicNonce := make([]byte, mnemonicGCM.NonceSize())
		if _, err := rand.Read(mnemonicNonce); err != nil {
			return fmt.Errorf("failed to generate mnemonic nonce: %w", err)
		}

		mnemonicCiphertext := mnemonicGCM.Seal(nil, mnemonicNonce, []byte(mnemonic), nil)

		ks.EncryptedMnemonic = &EncryptedMnemonic{
			SaltB64:       base64.StdEncoding.EncodeToString(mnemonicSalt),
			NonceB64:      base64.StdEncoding.EncodeToString(mnemonicNonce),
			CiphertextB64: base64.StdEncoding.EncodeToString(mnemonicCiphertext),
		}
	}

	b, err := json.MarshalIndent(ks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal keystore: %w", err)
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("%w: %v", ErrKeystoreWriteFailed, err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("%w: %v", ErrKeystoreWriteFailed, err)
	}

	return nil
}

// ReadKeystore reads a keystore file without decrypting
func ReadKeystore(path string) (*KeystoreFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read keystore: %w", err)
	}

	var ks KeystoreFile
	if err := json.Unmarshal(b, &ks); err != nil {
		return nil, fmt.Errorf("failed to unmarshal keystore: %w", err)
	}

	return &ks, nil
}

// WalletFromKeystore decrypts and returns wallet from keystore
// Production-grade: validates all parameters before decryption
func WalletFromKeystore(path string, password string) (*Wallet, error) {
	cacheKey := path

	keystoreCache.mu.RLock()
	if entry, exists := keystoreCache.wallets[cacheKey]; exists {
		entry.mu.RLock()
		if time.Now().Before(entry.ExpiresAt) {
			wallet := entry.Wallet
			entry.mu.RUnlock()
			keystoreCache.mu.RUnlock()
			return wallet, nil
		}
		entry.mu.RUnlock()
	}
	keystoreCache.mu.RUnlock()

	ks, err := ReadKeystore(path)
	if err != nil {
		return nil, err
	}

	wallet, err := decryptKeystore(ks, password)
	if err != nil {
		return nil, err
	}

	keystoreCache.mu.Lock()
	keystoreCache.wallets[cacheKey] = &WalletCacheEntry{
		Wallet:    wallet,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	keystoreCache.mu.Unlock()

	return wallet, nil
}

// decryptKeystore decrypts a keystore with password
func decryptKeystore(ks *KeystoreFile, password string) (*Wallet, error) {
	if ks.Version != KeystoreVersion {
		return nil, fmt.Errorf("%w: %d", ErrKeystoreInvalidVersion, ks.Version)
	}

	if err := ValidateAddress(ks.Address); err != nil {
		return nil, fmt.Errorf("invalid keystore address: %w", err)
	}

	if strings.ToLower(strings.TrimSpace(ks.KDF.Name)) != KDFNamePBKDF2 {
		return nil, fmt.Errorf("%w: %s", ErrKeystoreInvalidKDF, ks.KDF.Name)
	}

	if ks.Cipher.Name != CipherNameAESGCM {
		return nil, fmt.Errorf("%w: %s", ErrKeystoreInvalidCipher, ks.Cipher.Name)
	}

	salt, err := base64.StdEncoding.DecodeString(ks.KDF.SaltB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(ks.Cipher.NonceB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ks.Cipher.CiphertextB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	if ks.KDF.Iterations < MinKDFIterations {
		return nil, errors.New("kdf iterations too low")
	}

	key := pbkdf2HMACSHA256([]byte(password), salt, ks.KDF.Iterations, KeyLen)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	addrBytes, _ := hex.DecodeString(ks.Address)
	plain, err := gcm.Open(nil, nonce, ciphertext, addrBytes)
	if err != nil {
		return nil, ErrKeystoreWrongPassword
	}

	w, err := WalletFromPrivateKeyBytes(plain)
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	if w.Address != ks.Address {
		return nil, errors.New("keystore address mismatch")
	}

	return w, nil
}

// ExportMnemonicFromKeystore decrypts and returns mnemonic from keystore
// Returns empty string if no mnemonic is stored
func ExportMnemonicFromKeystore(path string, password string) (string, error) {
	ks, err := ReadKeystore(path)
	if err != nil {
		return "", err
	}

	if ks.EncryptedMnemonic == nil {
		return "", errors.New("no mnemonic stored in keystore")
	}

	salt, err := base64.StdEncoding.DecodeString(ks.EncryptedMnemonic.SaltB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode mnemonic salt: %w", err)
	}

	mnemonicKey := pbkdf2HMACSHA256([]byte(password), salt, ks.KDF.Iterations, KeyLen)

	block, err := aes.NewCipher(mnemonicKey)
	if err != nil {
		return "", fmt.Errorf("failed to create mnemonic cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create mnemonic GCM: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(ks.EncryptedMnemonic.NonceB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode mnemonic nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ks.EncryptedMnemonic.CiphertextB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode mnemonic ciphertext: %w", err)
	}

	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrKeystoreWrongPassword
	}

	return string(plain), nil
}

// ClearKeystoreCache clears the decrypted wallet cache
// Security: call this after sensitive operations
func ClearKeystoreCache() {
	keystoreCache.mu.Lock()
	defer keystoreCache.mu.Unlock()

	for _, entry := range keystoreCache.wallets {
		if entry.Wallet != nil {
			entry.Wallet.ClearPrivateKey()
		}
	}

	keystoreCache.wallets = make(map[string]*WalletCacheEntry)
}

// pbkdf2HMACSHA256 implements PBKDF2 with HMAC-SHA256
// Production-grade: RFC 2898 compliant implementation
func pbkdf2HMACSHA256(password, salt []byte, iter, keyLen int) []byte {
	hLen := sha256.Size
	numBlocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, numBlocks*hLen)
	var intBuf [4]byte

	for block := 1; block <= numBlocks; block++ {
		binaryBigEndianPutUint32(intBuf[:], uint32(block))
		u := hmacSHA256(password, append(append([]byte(nil), salt...), intBuf[:]...))
		t := make([]byte, len(u))
		copy(t, u)

		for i := 1; i < iter; i++ {
			u = hmacSHA256(password, u)
			for j := 0; j < len(t); j++ {
				t[j] ^= u[j]
			}
		}

		out = append(out, t...)
	}

	return out[:keyLen]
}

// hmacSHA256 computes HMAC-SHA256
func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

// binaryBigEndianPutUint32 encodes uint32 in big-endian
func binaryBigEndianPutUint32(b []byte, v uint32) {
	_ = b[3]
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}
