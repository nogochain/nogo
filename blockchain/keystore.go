package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
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
)

type KeystoreFile struct {
	Version int    `json:"version"`
	Address string `json:"address"`

	KDF KeystoreKDF `json:"kdf"`

	Cipher KeystoreCipher `json:"cipher"`

	// Optional encrypted mnemonic for backup purposes
	// Only present if wallet was created from mnemonic
	EncryptedMnemonic *EncryptedMnemonic `json:"encryptedMnemonic,omitempty"`
}

// EncryptedMnemonic stores an encrypted mnemonic phrase
// Uses separate salt/nonce for enhanced security
type EncryptedMnemonic struct {
	SaltB64       string `json:"saltB64"`
	NonceB64      string `json:"nonceB64"`
	CiphertextB64 string `json:"ciphertextB64"`
}

type KeystoreKDF struct {
	Name string `json:"name"`

	Iterations int `json:"iterations"`

	SaltB64 string `json:"saltB64"`
}

type KeystoreCipher struct {
	Name string `json:"name"`

	NonceB64      string `json:"nonceB64"`
	CiphertextB64 string `json:"ciphertextB64"`
}

var (
	ErrKeystoreWrongPassword = errors.New("wrong password")
)

func DefaultKeystoreParams() KeystoreKDF {
	// Reasonable defaults for a local keystore (tune up for real usage).
	return KeystoreKDF{
		Name:       "pbkdf2-sha256",
		Iterations: 600_000,
	}
}

func WriteKeystore(path string, w *Wallet, password string, params KeystoreKDF) error {
	if w == nil {
		return errors.New("nil wallet")
	}
	if password == "" {
		return errors.New("empty password")
	}
	if params.Name == "" {
		params = DefaultKeystoreParams()
	}
	params.Name = strings.ToLower(strings.TrimSpace(params.Name))
	if params.Name != "pbkdf2-sha256" {
		return fmt.Errorf("unsupported kdf: %s", params.Name)
	}
	if params.Iterations < 10_000 {
		return errors.New("invalid kdf params")
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}

	key := pbkdf2HMACSHA256([]byte(password), salt, params.Iterations, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	// Bind ciphertext to the address to avoid swap attacks.
	addrBytes, err := hex.DecodeString(w.Address)
	if err != nil {
		return err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(w.PrivateKey), addrBytes)

	ks := KeystoreFile{
		Version: 1,
		Address: w.Address,
		KDF: KeystoreKDF{
			Name:       "pbkdf2-sha256",
			Iterations: params.Iterations,
			SaltB64:    base64.StdEncoding.EncodeToString(salt),
		},
		Cipher: KeystoreCipher{
			Name:          "aes-256-gcm",
			NonceB64:      base64.StdEncoding.EncodeToString(nonce),
			CiphertextB64: base64.StdEncoding.EncodeToString(ciphertext),
		},
	}

	b, err := json.MarshalIndent(ks, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil && filepath.Dir(path) != "." {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ReadKeystore(path string) (*KeystoreFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ks KeystoreFile
	if err := json.Unmarshal(b, &ks); err != nil {
		return nil, err
	}
	return &ks, nil
}

func WalletFromKeystore(path string, password string) (*Wallet, error) {
	ks, err := ReadKeystore(path)
	if err != nil {
		return nil, err
	}
	if ks.Version != 1 {
		return nil, fmt.Errorf("unsupported keystore version: %d", ks.Version)
	}
	if err := validateAddress(ks.Address); err != nil {
		return nil, fmt.Errorf("invalid keystore address: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(ks.KDF.Name)) != "pbkdf2-sha256" {
		return nil, fmt.Errorf("unsupported kdf: %s", ks.KDF.Name)
	}
	if ks.Cipher.Name != "aes-256-gcm" {
		return nil, fmt.Errorf("unsupported cipher: %s", ks.Cipher.Name)
	}
	salt, err := base64.StdEncoding.DecodeString(ks.KDF.SaltB64)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(ks.Cipher.NonceB64)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ks.Cipher.CiphertextB64)
	if err != nil {
		return nil, err
	}

	if ks.KDF.Iterations < 10_000 {
		return nil, errors.New("invalid kdf params")
	}
	key := pbkdf2HMACSHA256([]byte(password), salt, ks.KDF.Iterations, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	addrBytes, _ := hex.DecodeString(ks.Address)
	plain, err := gcm.Open(nil, nonce, ciphertext, addrBytes)
	if err != nil {
		return nil, ErrKeystoreWrongPassword
	}

	w, err := WalletFromPrivateKeyBytes(plain)
	if err != nil {
		return nil, err
	}
	if w.Address != ks.Address {
		return nil, errors.New("keystore address mismatch")
	}
	return w, nil
}

func WalletFromPrivateKeyBytes(raw []byte) (*Wallet, error) {
	if len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid private key length")
	}
	priv := make([]byte, len(raw))
	copy(priv, raw)
	pub := ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)
	sum := sha256.Sum256(pub)
	return &Wallet{
		PrivateKey: ed25519.PrivateKey(priv),
		PublicKey:  pub,
		Address:    hex.EncodeToString(sum[:]),
	}, nil
}

// pbkdf2HMACSHA256 is a small in-tree PBKDF2 (RFC 2898) implementation to avoid extra dependencies.
func pbkdf2HMACSHA256(password, salt []byte, iter, keyLen int) []byte {
	hLen := 32
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

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func binaryBigEndianPutUint32(b []byte, v uint32) {
	_ = b[3]
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}

// WriteKeystoreWithMnemonic writes a keystore file with encrypted mnemonic backup
// The mnemonic is encrypted with a separate key derived from the password
func WriteKeystoreWithMnemonic(path string, w *Wallet, mnemonic, password string, params KeystoreKDF) error {
	// Write the base keystore
	if err := WriteKeystore(path, w, password, params); err != nil {
		return err
	}

	// If mnemonic provided, encrypt and store it
	if mnemonic == "" {
		return nil
	}

	// Read existing keystore
	ks, err := ReadKeystore(path)
	if err != nil {
		return err
	}

	// Derive separate key for mnemonic encryption
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}

	mnemonicKey := pbkdf2HMACSHA256([]byte(password), salt, params.Iterations, 32)

	// Encrypt mnemonic
	block, err := aes.NewCipher(mnemonicKey)
	if err != nil {
		return err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(mnemonic), nil)

	// Store encrypted mnemonic
	ks.EncryptedMnemonic = &EncryptedMnemonic{
		SaltB64:       base64.StdEncoding.EncodeToString(salt),
		NonceB64:      base64.StdEncoding.EncodeToString(nonce),
		CiphertextB64: base64.StdEncoding.EncodeToString(ciphertext),
	}

	// Write updated keystore
	b, err := json.MarshalIndent(ks, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ExportMnemonicFromKeystore decrypts and returns the mnemonic from a keystore file
// Returns empty string if no mnemonic is stored
func ExportMnemonicFromKeystore(path string, password string) (string, error) {
	ks, err := ReadKeystore(path)
	if err != nil {
		return "", err
	}

	// Check if mnemonic is stored
	if ks.EncryptedMnemonic == nil {
		return "", errors.New("no mnemonic stored in keystore")
	}

	// Derive mnemonic decryption key
	salt, err := base64.StdEncoding.DecodeString(ks.EncryptedMnemonic.SaltB64)
	if err != nil {
		return "", err
	}

	mnemonicKey := pbkdf2HMACSHA256([]byte(password), salt, ks.KDF.Iterations, 32)

	// Decrypt mnemonic
	block, err := aes.NewCipher(mnemonicKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce, err := base64.StdEncoding.DecodeString(ks.EncryptedMnemonic.NonceB64)
	if err != nil {
		return "", err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ks.EncryptedMnemonic.CiphertextB64)
	if err != nil {
		return "", err
	}

	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrKeystoreWrongPassword
	}

	return string(plain), nil
}

// CreateWalletFromMnemonic creates a new wallet from a mnemonic phrase
// Returns the wallet and the mnemonic (either provided or newly generated)
func CreateWalletFromMnemonic(mnemonic, passphrase string) (*Wallet, string, error) {
	var err error

	// If no mnemonic provided, generate a new one
	if mnemonic == "" {
		mnemonic, err = GenerateMnemonic()
		if err != nil {
			return nil, "", err
		}
	}

	// Validate mnemonic
	if !ValidateMnemonic(mnemonic) {
		return nil, "", errors.New("invalid mnemonic phrase")
	}

	// Create wallet from mnemonic
	wallet, err := WalletFromMnemonic(mnemonic, passphrase)
	if err != nil {
		return nil, "", err
	}

	return wallet, mnemonic, nil
}
