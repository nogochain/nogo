// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package nogopow

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"golang.org/x/crypto/sha3"
)

// Address represents a 20-byte NogoChain address
type Address [20]byte

// Bytes returns address as byte slice
func (a Address) Bytes() []byte { return a[:] }

// Hash represents a 32-byte hash
type Hash [32]byte

// Bytes returns hash as byte slice
func (h Hash) Bytes() []byte { return h[:] }

// Hex returns hex string representation
func (h Hash) Hex() string {
	return fmt.Sprintf("%x", h[:])
}

// BlockNonce represents a 32-byte nonce for mining
type BlockNonce [32]byte

// Header represents a block header
type Header struct {
	ParentHash Hash
	Coinbase   Address
	Root       Hash
	TxHash     Hash
	Number     *big.Int
	GasLimit   uint64
	Time       uint64
	Extra      []byte
	Nonce      BlockNonce
	Difficulty *big.Int
}

// Hash computes the hash of the header
func (h *Header) Hash() Hash {
	hasher := sha3.NewLegacyKeccak256()
	rlpEncode(hasher, h)
	return BytesToHash(hasher.Sum(nil))
}

// Block represents a complete block
type Block struct {
	header       *Header
	transactions []*Transaction
	uncles       []*Header
}

// Header returns the block header
func (b *Block) Header() *Header { return b.header }

// Transactions returns the block transactions
func (b *Block) Transactions() []*Transaction { return b.transactions }

// Uncles returns the block uncles
func (b *Block) Uncles() []*Header { return b.uncles }

// Number returns the block number
func (b *Block) Number() *big.Int { return b.header.Number }

// NewBlock creates a new block
func NewBlock(header *Header, txs []*Transaction, uncles []*Header, receipts []*Receipt) *Block {
	return &Block{
		header:       header,
		transactions: txs,
		uncles:       uncles,
	}
}

// Transaction represents a blockchain transaction
type Transaction struct {
	Type       TransactionType
	ChainID    uint64
	FromPubKey []byte
	ToAddress  string
	Amount     uint64
	Fee        uint64
	Nonce      uint64
	Data       string
	Signature  []byte
}

// TransactionType represents the type of transaction
type TransactionType string

const (
	TxCoinbase TransactionType = "coinbase"
	TxTransfer TransactionType = "transfer"
)

// Receipt represents a transaction receipt
type Receipt struct {
	Status uint64
}

// ChainHeaderReader defines the interface for header access
type ChainHeaderReader interface {
	GetHeaderByHash(hash Hash) *Header
}

// ChainReader defines the interface for chain access
type ChainReader interface {
	ChainHeaderReader
}

// StateDB defines the interface for state access
type StateDB interface {
	AddBalance(addr Address, amount *big.Int)
	IntermediateRoot(v bool) Hash
}

// rlpEncode encodes header fields sequentially
func rlpEncode(w interface{}, v interface{}) {
	// For Header: encode all fields sequentially
	// Uses custom RLP-like encoding for block headers

	header, ok := v.(*Header)
	if !ok {
		return
	}

	// Write all header fields to the writer
	writer, ok := w.(interface{ Write([]byte) (int, error) })
	if !ok {
		return
	}

	// Encode each field
	writer.Write(header.ParentHash.Bytes())
	writer.Write(header.Coinbase.Bytes())
	writer.Write(header.Root.Bytes())
	writer.Write(header.TxHash.Bytes())

	// Number as big.Int bytes
	if header.Number != nil {
		writer.Write(header.Number.Bytes())
	}

	// GasLimit as 8 bytes
	gasBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(gasBytes, header.GasLimit)
	writer.Write(gasBytes)

	// Time as 8 bytes
	timeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timeBytes, header.Time)
	writer.Write(timeBytes)

	// Extra data
	if len(header.Extra) > 0 {
		writer.Write(header.Extra)
	}

	// Nonce
	writer.Write(header.Nonce[:])

	// Difficulty as big.Int bytes
	if header.Difficulty != nil {
		writer.Write(header.Difficulty.Bytes())
	}
}

// BytesToHash converts bytes to hash
func BytesToHash(b []byte) Hash {
	var h Hash
	if len(b) > 32 {
		b = b[len(b)-32:]
	}
	copy(h[32-len(b):], b)
	return h
}

// BigToHash converts big.Int to hash
func BigToHash(b *big.Int) Hash {
	if b == nil {
		return Hash{}
	}
	return BytesToHash(b.Bytes())
}

// StringToAddress converts string to address
func StringToAddress(s string) Address {
	var a Address
	// Simple conversion for demo
	return a
}
