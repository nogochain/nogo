//go:build fuzz
// +build fuzz

package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
)

func FuzzTxValidation(data []byte) int {
	if len(data) < 64 {
		return 0
	}

	pubKey := data[:32]
	privKey := data[32:64]
	toAddr := data[64:96]
	amount := data[97]
	fee := data[98]
	nonce := data[99]

	_ = pubKey

	if len(toAddr) != 32 {
		return 0
	}

	tx := &Tx{
		FromPubKey:  pubKey,
		ToAddress:   hex.EncodeToString(toAddr),
		Amount:      uint64(amount) + 1,
		Nonce:       uint64(nonce) + 1,
		Fee:         uint64(fee) + 1,
		Data:        data[100:],
		ChainID:     1,
	}

	key := ed25519.PrivateKey(privKey)
	if len(key) != 64 {
		return 0
	}

	if err := tx.Sign(key[:32]); err != nil {
		return 0
	}

	if err := tx.Verify(); err != nil {
		return 0
	}

	return 1
}

func FuzzBlockValidation(data []byte) int {
	if len(data) < 100 {
		return 0
	}

	_ = fmt.Sprintf("%s", data)

	return 1
}
