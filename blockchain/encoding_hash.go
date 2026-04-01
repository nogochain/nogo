package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

func txSigningHashLegacyJSON(tx Transaction) ([]byte, error) {
	return tx.signingHashLegacyJSON()
}

func txSigningHashBinaryV1(tx Transaction) ([]byte, error) {
	pre, err := txSigningPreimageBinaryV1(tx)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(pre)
	return sum[:], nil
}

func txSigningHashForConsensus(tx Transaction, p ConsensusParams, height uint64) ([]byte, error) {
	if p.BinaryEncodingActive(height) {
		return txSigningHashBinaryV1(tx)
	}
	return txSigningHashLegacyJSON(tx)
}

func txIDHexForConsensus(tx Transaction, p ConsensusParams, height uint64) (string, error) {
	h, err := txSigningHashForConsensus(tx, p, height)
	if err != nil {
		return "", err
	}
	return bytesToHex32(h)
}

func bytesToHex32(b []byte) (string, error) {
	if len(b) != 32 {
		return "", errors.New("expected 32 bytes")
	}
	return hex.EncodeToString(b), nil
}
