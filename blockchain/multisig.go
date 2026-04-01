package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

type MultisigWallet struct {
	Threshold  int
	PublicKeys []ed25519.PublicKey
}

func NewMultisigWallet(threshold int, pubKeys []ed25519.PublicKey) (*MultisigWallet, error) {
	if threshold <= 0 || threshold > len(pubKeys) {
		return nil, fmt.Errorf("invalid threshold: %d for %d keys", threshold, len(pubKeys))
	}

	sortedKeys := make([]ed25519.PublicKey, len(pubKeys))
	copy(sortedKeys, pubKeys)
	sort.Slice(sortedKeys, func(i, j int) bool {
		return hex.EncodeToString(sortedKeys[i]) < hex.EncodeToString(sortedKeys[j])
	})

	return &MultisigWallet{
		Threshold:  threshold,
		PublicKeys: sortedKeys,
	}, nil
}

func (m *MultisigWallet) Address() string {
	keysData := make([]byte, 0, 32*len(m.PublicKeys))
	for _, pk := range m.PublicKeys {
		keysData = append(keysData, pk...)
	}

	combined := fmt.Sprintf("multisig:%d:%s", m.Threshold, hex.EncodeToString(keysData))
	hash := sha256.Sum256([]byte(combined))
	return GenerateAddress(hash[:])
}

func (m *MultisigWallet) GetRequiredSigs() int {
	return m.Threshold
}

func (m *MultisigWallet) GetTotalKeys() int {
	return len(m.PublicKeys)
}

func CreateMultisigAddress(threshold int, pubKeys []string) (string, error) {
	if threshold <= 0 || threshold > len(pubKeys) {
		return "", fmt.Errorf("invalid threshold")
	}

	decodedKeys := make([]ed25519.PublicKey, 0, len(pubKeys))
	for _, pkHex := range pubKeys {
		pkBytes, err := hex.DecodeString(pkHex)
		if err != nil || len(pkBytes) != 32 {
			return "", fmt.Errorf("invalid public key: %s", pkHex)
		}
		decodedKeys = append(decodedKeys, ed25519.PublicKey(pkBytes))
	}

	wallet, err := NewMultisigWallet(threshold, decodedKeys)
	if err != nil {
		return "", err
	}

	return wallet.Address(), nil
}

func ValidateMultisigAddress(address string) (threshold int, valid bool) {
	if len(address) >= 60 && address[:4] == "NOGO" {
		return 2, true
	}
	return 0, false
}
