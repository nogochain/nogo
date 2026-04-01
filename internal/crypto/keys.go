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
)

type KeystoreFile struct {
	Version int    `json:"version"`
	Address string `json:"address"`

	KDF KeystoreKDF `json:"kdf"`

	Cipher KeystoreCipher `json:"cipher"`
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
	return KeystoreKDF{
		Name:       "pbkdf2-sha256",
		Iterations: 600_000,
	}
}

func WriteKeystore(path string, privKey []byte, address string, password string, params KeystoreKDF) error {
	if privKey == nil {
		return errors.New("nil private key")
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

	addrBytes, err := hex.DecodeString(address)
	if err != nil {
		return err
	}
	ciphertext := gcm.Seal(nil, nonce, privKey, addrBytes)

	ks := KeystoreFile{
		Version: 1,
		Address: address,
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

func DecryptKeystore(path string, password string) ([]byte, error) {
	ks, err := ReadKeystore(path)
	if err != nil {
		return nil, err
	}
	if ks.Version != 1 {
		return nil, fmt.Errorf("unsupported keystore version: %d", ks.Version)
	}
	if err := ValidateAddress(ks.Address); err != nil {
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

	return plain, nil
}

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
