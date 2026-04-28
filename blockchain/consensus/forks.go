// Copyright 2026 NogoChain Team
// Consensus Types, Constants and Utility Functions for NogoChain Blockchain
// This file contains core consensus type definitions and utility functions used across the blockchain

package consensus

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/nogopow"
)

var (
	ErrUnknownParent        = errors.New("unknown parent")
	ErrDuplicateBlock       = errors.New("duplicate block")
	ErrCycleDetected        = errors.New("cycle detected in chain")
	ErrMissingAncestor      = errors.New("missing ancestor block")
	ErrBadGenesisHeader     = errors.New("bad genesis header")
	ErrBadGenesisVersion    = errors.New("bad genesis version")
	ErrBadHeightLinkage     = errors.New("bad height linkage")
	ErrBadPrevHashLink      = errors.New("bad prevhash linkage")
	ErrReorgDepthExceeded   = errors.New("reorg depth exceeds maximum allowed")
	ErrRollbackFailed       = errors.New("rollback failed")
	ErrStateRecomputeFailed = errors.New("state recomputation failed")
	ErrDiskWriteFailed      = errors.New("disk write failed")
	ErrExecutionFailed      = errors.New("execution failed")
)

type (
	Block           = core.Block
	Account         = core.Account
	ConsensusParams = config.ConsensusParams
	ChainStore      = core.ChainStore
	EventSink       = core.EventSink
	WSEvent         = core.WSEvent
	TxLocation      = core.TxLocation
	AddressTxEntry  = core.AddressTxEntry
)

func WorkForDifficultyBits(bits uint32) *big.Int {
	if bits > 256 {
		bits = 256
	}
	if bits == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Lsh(big.NewInt(1), uint(bits))
}

func blockVersionForHeight(consensus ConsensusParams, height uint64) uint32 {
	if consensus.MerkleRootActive(height) || consensus.BinaryEncodingActive(height) {
		return 2
	}
	return 1
}

func TxIDHexForConsensus(tx Transaction, p ConsensusParams, height uint64) (string, error) {
	h, err := txSigningHashForConsensus(tx, p, height)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h), nil
}

func TxIDHex(tx Transaction) (string, error) {
	h, err := tx.SigningHash()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h), nil
}

func validateDifficultyNogoPow(consensus ConsensusParams, path []*Block, idx int) error {
	if idx <= 0 || idx >= len(path) {
		return nil
	}

	if idx == 0 {
		if path[0].Header.DifficultyBits != consensus.GenesisDifficultyBits {
			return fmt.Errorf("bad genesis difficulty: expected %d got %d",
				consensus.GenesisDifficultyBits, path[0].Header.DifficultyBits)
		}
		return nil
	}

	currentBlock := path[idx]
	parentBlock := path[idx-1]

	if currentBlock.Header.DifficultyBits < consensus.MinDifficultyBits {
		return fmt.Errorf("difficulty %d below min %d", currentBlock.Header.DifficultyBits, consensus.MinDifficultyBits)
	}
	if currentBlock.Header.DifficultyBits > consensus.MaxDifficultyBits {
		return fmt.Errorf("difficulty %d above max %d", currentBlock.Header.DifficultyBits, consensus.MaxDifficultyBits)
	}

	if consensus.DifficultyEnable {
		adjuster := nogopow.NewDifficultyAdjuster(&consensus)

		var parentHash nogopow.Hash
		if len(parentBlock.Hash) > 0 {
			copy(parentHash[:], parentBlock.Hash)
		} else {
			copy(parentHash[:], parentBlock.Header.PrevHash)
		}

		parentHeader := &nogopow.Header{
			Number:     big.NewInt(int64(parentBlock.GetHeight())),
			Time:       uint64(parentBlock.Header.TimestampUnix),
			Difficulty: big.NewInt(int64(parentBlock.Header.DifficultyBits)),
			ParentHash: parentHash,
		}

		expectedDifficulty := adjuster.CalcDifficulty(uint64(currentBlock.Header.TimestampUnix), parentHeader)
		actualDifficulty := big.NewInt(int64(currentBlock.Header.DifficultyBits))

		minAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(100-DifficultyTolerancePercent))
		minAllowed.Div(minAllowed, big.NewInt(100))

		maxAllowed := new(big.Int).Mul(expectedDifficulty, big.NewInt(100+DifficultyTolerancePercent))
		maxAllowed.Div(maxAllowed, big.NewInt(100))

		if actualDifficulty.Cmp(minAllowed) < 0 {
			return fmt.Errorf("difficulty adjustment too aggressive: actual %d < min allowed %d (expected %d, block height %d)",
				actualDifficulty.Uint64(), minAllowed.Uint64(), expectedDifficulty.Uint64(), currentBlock.GetHeight())
		}

		if actualDifficulty.Cmp(maxAllowed) > 0 {
			return fmt.Errorf("difficulty adjustment too aggressive: actual %d > max allowed %d (expected %d, block height %d)",
				actualDifficulty.Uint64(), maxAllowed.Uint64(), expectedDifficulty.Uint64(), currentBlock.GetHeight())
		}
	}

	return nil
}

func addressesForBlock(b *Block) []string {
	addresses := make(map[string]struct{})
	for _, tx := range b.Transactions {
		if tx.Type == TxTransfer {
			fromAddr, err := tx.FromAddress()
			if err == nil {
				addresses[fromAddr] = struct{}{}
			}
			addresses[tx.ToAddress] = struct{}{}
		} else if tx.Type == TxCoinbase {
			addresses[tx.ToAddress] = struct{}{}
		}
	}

	result := make([]string, 0, len(addresses))
	for addr := range addresses {
		result = append(result, addr)
	}
	return result
}

func GetKnownTipsFromBlocks(blocksByHash map[string]*Block) []string {
	var tips []string
	isParent := make(map[string]struct{})

	for _, b := range blocksByHash {
		if len(b.Header.PrevHash) > 0 {
			isParent[hex.EncodeToString(b.Header.PrevHash)] = struct{}{}
		}
	}

	for h := range blocksByHash {
		if _, ok := isParent[h]; !ok {
			tips = append(tips, h)
		}
	}

	sort.Strings(tips)
	return tips
}
