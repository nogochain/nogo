package main

import (
	"encoding/hex"
)

func TxIDHex(tx Transaction) (string, error) {
	h, err := tx.SigningHash()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h), nil
}

func TxIDHexForConsensus(tx Transaction, p ConsensusParams, height uint64) (string, error) {
	return txIDHexForConsensus(tx, p, height)
}
