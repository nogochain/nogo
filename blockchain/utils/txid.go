package utils

import (
	"encoding/hex"

	"github.com/nogochain/nogo/blockchain/core"
)

func TxIDHex(tx core.Transaction) (string, error) {
	h, err := tx.SigningHash()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h), nil
}
