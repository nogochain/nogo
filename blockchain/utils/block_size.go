package utils

import (
	"encoding/json"
	"errors"

	"github.com/nogochain/nogo/blockchain/core"
)

// blockSizeForConsensus returns the size in bytes of the JSON-encoded block.
// This mirrors the network/block payload encoding used today.
func blockSizeForConsensus(b *core.Block) (int, error) {
	if b == nil {
		return 0, errors.New("nil block")
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return 0, err
	}
	return len(raw), nil
}
