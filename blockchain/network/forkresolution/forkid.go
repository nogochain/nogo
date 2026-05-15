package forkresolution

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"math"

	"github.com/nogochain/nogo/blockchain/core"
)

var (
	ErrRemoteStale              = errors.New("remote needs update")
	ErrLocalIncompatibleOrStale = errors.New("local incompatible or needs update")
)

const forkIDGenesisTimestamp = 1700000000

type ForkID struct {
	Hash [4]byte
	Next uint64
}

type ForkIDFilter func(id ForkID) error

type ForkIDChain interface {
	Genesis() *core.Block
	LatestBlock() *core.Block
}

func NewForkID(genesis *core.Block, head *core.Block, forks []uint64) ForkID {
	hash := crc32.ChecksumIEEE(genesis.Hash)
	for _, fork := range forks {
		if fork <= head.GetHeight() {
			hash = forkIDChecksumUpdate(hash, fork)
			continue
		}
		return ForkID{Hash: forkIDChecksumToBytes(hash), Next: fork}
	}
	return ForkID{Hash: forkIDChecksumToBytes(hash), Next: 0}
}

func NewForkIDWithChain(chain ForkIDChain, forks []uint64) ForkID {
	head := chain.LatestBlock()
	genesis := chain.Genesis()
	if head == nil || genesis == nil {
		return ForkID{}
	}
	return NewForkID(genesis, head, forks)
}

func NewForkIDFilter(chain ForkIDChain, forks []uint64) ForkIDFilter {
	return newForkIDFilter(chain, forks)
}

func newForkIDFilter(chain ForkIDChain, forks []uint64) ForkIDFilter {
	genesis := chain.Genesis()
	if genesis == nil {
		return func(id ForkID) error { return nil }
	}

	hash := crc32.ChecksumIEEE(genesis.Hash)
	sums := make([][4]byte, len(forks)+1)
	sums[0] = forkIDChecksumToBytes(hash)
	for i, fork := range forks {
		hash = forkIDChecksumUpdate(hash, fork)
		sums[i+1] = forkIDChecksumToBytes(hash)
	}

	forks = append(forks, math.MaxUint64)

	return func(id ForkID) error {
		head := chain.LatestBlock()
		if head == nil {
			return nil
		}
		block := head.GetHeight()

		for i, fork := range forks {
			if block >= fork {
				continue
			}
			if sums[i] == id.Hash {
				if id.Next > 0 && block >= id.Next {
					return ErrLocalIncompatibleOrStale
				}
				return nil
			}
			for j := 0; j < i; j++ {
				if sums[j] == id.Hash {
					if forks[j] != id.Next {
						return ErrRemoteStale
					}
					return nil
				}
			}
			for j := i + 1; j < len(sums); j++ {
				if sums[j] == id.Hash {
					return nil
				}
			}
			return ErrLocalIncompatibleOrStale
		}
		return nil
	}
}

func forkIDChecksumUpdate(hash uint32, fork uint64) uint32 {
	var blob [8]byte
	binary.BigEndian.PutUint64(blob[:], fork)
	return crc32.Update(hash, crc32.IEEETable, blob[:])
}

func forkIDChecksumToBytes(hash uint32) [4]byte {
	var blob [4]byte
	binary.BigEndian.PutUint32(blob[:], hash)
	return blob
}