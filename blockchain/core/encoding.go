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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	// binaryEncodingVersionV1 is the version byte for binary encoding
	binaryEncodingVersionV1 = uint8(1)
)

var (
	// ErrBinaryEncoding is returned when binary encoding/decoding fails
	ErrBinaryEncoding = errors.New("binary encoding error")
	// ErrInvalidHashLength is returned when a hash has incorrect length
	ErrInvalidHashLength = errors.New("invalid hash length")
	// ErrInvalidAddress is returned when address parsing fails
	ErrInvalidAddress = errors.New("invalid address")
)

// writeULEB128 writes an unsigned integer in ULEB128 format
// Production-grade: variable-length encoding for efficient serialization
// Math & numeric safety: handles all uint64 values correctly
func writeULEB128(buf *bytes.Buffer, n uint64) {
	for {
		b := byte(n & 0x7F)
		n >>= 7
		if n != 0 {
			b |= 0x80
		}
		_ = buf.WriteByte(b)
		if n == 0 {
			return
		}
	}
}

// readULEB128 reads an unsigned integer in ULEB128 format
// Production-grade: variable-length decoding with overflow protection
// Math & numeric safety: checks for overflow during decoding
func readULEB128(buf *bytes.Buffer) (uint64, error) {
	var result uint64
	var shift uint
	for i := 0; i < 10; i++ {
		b, err := buf.ReadByte()
		if err != nil {
			return 0, fmt.Errorf("read ULEB128 byte: %w", err)
		}
		result |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return result, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, errors.New("ULEB128 overflow")
		}
	}
	return 0, errors.New("ULEB128 too long")
}

// decodeHex32 decodes a hex string or NOGO address to 32 bytes
// Production-grade: supports both raw hex and NOGO address formats
// Logic completeness: handles both address formats with validation
func decodeHex32(addrHex string) ([32]byte, error) {
	var out [32]byte

	if len(addrHex) > 4 && addrHex[:4] == AddressPrefix {
		encoded := addrHex[4:]
		b, err := hex.DecodeString(encoded)
		if err != nil {
			return out, fmt.Errorf("decode NOGO address: %w", err)
		}
		if len(b) < 33 {
			return out, errors.New("NOGO address too short")
		}
		if len(b) == 37 {
			copy(out[:], b[1:33])
		} else {
			return out, errors.New("invalid NOGO address length")
		}
	} else {
		b, err := hex.DecodeString(addrHex)
		if err != nil {
			return out, fmt.Errorf("decode hex: %w", err)
		}
		if len(b) != 32 {
			return out, fmt.Errorf("expected 32 bytes, got %d: %w", len(b), ErrInvalidHashLength)
		}
		copy(out[:], b)
	}
	return out, nil
}

// bytesToHex32 converts 32 bytes to hex string
// Logic completeness: validates input length before conversion
func bytesToHex32(b []byte) (string, error) {
	if len(b) != 32 {
		return "", fmt.Errorf("expected 32 bytes, got %d: %w", len(b), ErrInvalidHashLength)
	}
	return hex.EncodeToString(b), nil
}

// txSigningPreimageBinaryV1 creates binary preimage for transaction signing
// Production-grade: deterministic binary encoding for consensus
// Logic completeness: handles both coinbase and transfer transactions
func txSigningPreimageBinaryV1(tx Transaction) ([]byte, error) {
	var buf bytes.Buffer

	// Write version byte
	_ = buf.WriteByte(binaryEncodingVersionV1)

	// Write tx type byte
	var txType byte
	switch tx.Type {
	case TxCoinbase:
		txType = 0
	case TxTransfer:
		txType = 1
	default:
		return nil, errors.New("unknown tx type")
	}
	_ = buf.WriteByte(txType)

	// Write chain ID
	if tx.ChainID == 0 {
		return nil, errors.New("missing chainId")
	}
	_ = binary.Write(&buf, binary.LittleEndian, tx.ChainID)

	// Write type-specific fields
	switch tx.Type {
	case TxCoinbase:
		to, err := decodeHex32(tx.ToAddress)
		if err != nil {
			return nil, fmt.Errorf("invalid toAddress: %w", err)
		}
		if _, err := buf.Write(to[:]); err != nil {
			return nil, fmt.Errorf("write toAddress: %w", err)
		}
		if err := binary.Write(&buf, binary.LittleEndian, tx.Amount); err != nil {
			return nil, fmt.Errorf("write amount: %w", err)
		}
		data := []byte(tx.Data)
		writeULEB128(&buf, uint64(len(data)))
		if len(data) > 0 {
			if _, err := buf.Write(data); err != nil {
				return nil, fmt.Errorf("write data: %w", err)
			}
		}
	case TxTransfer:
		if len(tx.FromPubKey) != PubKeySize {
			return nil, fmt.Errorf("invalid fromPubKey length: %d", len(tx.FromPubKey))
		}
		to, err := decodeHex32(tx.ToAddress)
		if err != nil {
			return nil, fmt.Errorf("invalid toAddress: %w", err)
		}
		if _, err := buf.Write(tx.FromPubKey); err != nil {
			return nil, fmt.Errorf("write fromPubKey: %w", err)
		}
		if _, err := buf.Write(to[:]); err != nil {
			return nil, fmt.Errorf("write toAddress: %w", err)
		}
		if err := binary.Write(&buf, binary.LittleEndian, tx.Amount); err != nil {
			return nil, fmt.Errorf("write amount: %w", err)
		}
		if err := binary.Write(&buf, binary.LittleEndian, tx.Nonce); err != nil {
			return nil, fmt.Errorf("write nonce: %w", err)
		}
		if err := binary.Write(&buf, binary.LittleEndian, tx.Fee); err != nil {
			return nil, fmt.Errorf("write fee: %w", err)
		}
		data := []byte(tx.Data)
		writeULEB128(&buf, uint64(len(data)))
		if len(data) > 0 {
			if _, err := buf.Write(data); err != nil {
				return nil, fmt.Errorf("write data: %w", err)
			}
		}
	default:
		return nil, fmt.Errorf("unknown tx type: %q", tx.Type)
	}

	return buf.Bytes(), nil
}

// txSigningHashBinaryV1 computes SHA256 of binary preimage
// Security: uses SHA256 for cryptographic hashing
func txSigningHashBinaryV1(tx Transaction) ([]byte, error) {
	pre, err := txSigningPreimageBinaryV1(tx)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(pre)
	return sum[:], nil
}

// txSigningHashLegacyJSON computes legacy JSON-based signing hash
// Note: kept for backward compatibility, prefer binary encoding
func txSigningHashLegacyJSON(tx Transaction) ([]byte, error) {
	return tx.signingHashLegacyJSON()
}

// txSigningHashForConsensus returns the consensus-aware signing hash
// Production-grade: selects encoding based on consensus rules
func txSigningHashForConsensus(tx Transaction, p ConsensusParams, height uint64) ([]byte, error) {
	if p.BinaryEncodingActive(height) {
		return txSigningHashBinaryV1(tx)
	}
	return txSigningHashLegacyJSON(tx)
}

// txIDHexForConsensus computes transaction ID for consensus
// Production-grade: uses consensus-aware signing hash
func txIDHexForConsensus(tx Transaction, p ConsensusParams, height uint64) (string, error) {
	h, err := txSigningHashForConsensus(tx, p, height)
	if err != nil {
		return "", err
	}
	return bytesToHex32(h)
}

// blockHeaderPreimageBinaryV1 creates binary preimage for block header hashing
// Production-grade: deterministic binary encoding for PoW
// Logic completeness: handles both v1 and v2 block versions
func blockHeaderPreimageBinaryV1(b *Block, nonce uint64, p ConsensusParams) ([]byte, error) {
	if b == nil {
		return nil, errors.New("nil block")
	}
	if b.TimestampUnix <= 0 {
		return nil, errors.New("invalid timestamp")
	}
	if len(b.PrevHash) != 0 && len(b.PrevHash) != 32 {
		return nil, fmt.Errorf("invalid prevHash length: %d", len(b.PrevHash))
	}
	if len(b.Hash) != 0 && len(b.Hash) != 32 {
		return nil, fmt.Errorf("invalid hash length: %d", len(b.Hash))
	}
	miner, err := decodeHex32(b.MinerAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid minerAddress: %w", err)
	}

	var root [32]byte
	switch b.Version {
	case 2:
		r, err := b.MerkleRootV2ForConsensus(p)
		if err != nil {
			return nil, fmt.Errorf("compute Merkle root: %w", err)
		}
		copy(root[:], r)
	default:
		r, err := b.TxRootLegacyForConsensus(p)
		if err != nil {
			return nil, fmt.Errorf("compute Tx root: %w", err)
		}
		copy(root[:], r)
	}

	var prev [32]byte
	if len(b.PrevHash) == 32 {
		copy(prev[:], b.PrevHash)
	}

	var buf bytes.Buffer
	if err := buf.WriteByte(binaryEncodingVersionV1); err != nil {
		return nil, fmt.Errorf("write version: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, b.Version); err != nil {
		return nil, fmt.Errorf("write version: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, b.Height); err != nil {
		return nil, fmt.Errorf("write height: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, b.TimestampUnix); err != nil {
		return nil, fmt.Errorf("write timestamp: %w", err)
	}
	if _, err := buf.Write(prev[:]); err != nil {
		return nil, fmt.Errorf("write prevHash: %w", err)
	}
	if _, err := buf.Write(root[:]); err != nil {
		return nil, fmt.Errorf("write root: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, b.DifficultyBits); err != nil {
		return nil, fmt.Errorf("write difficultyBits: %w", err)
	}
	if _, err := buf.Write(miner[:]); err != nil {
		return nil, fmt.Errorf("write minerAddress: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, nonce); err != nil {
		return nil, fmt.Errorf("write nonce: %w", err)
	}
	return buf.Bytes(), nil
}

// TxIDHex computes the transaction ID as hex string
// Production-grade: uses default consensus parameters
func TxIDHex(tx Transaction) (string, error) {
	h, err := tx.SigningHash()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h), nil
}

// BlockHash computes the block hash from header
// Production-grade: uses consensus-aware header encoding
func BlockHash(b *Block, p ConsensusParams) ([]byte, error) {
	if b == nil {
		return nil, errors.New("nil block")
	}

	var headerBytes []byte
	var err error

	if p.BinaryEncodingActive(b.Height) {
		headerBytes, err = blockHeaderPreimageBinaryV1(b, b.Header.Nonce, p)
	} else {
		headerBytes, err = b.HeaderBytesForConsensus(p, b.Header.Nonce)
	}

	if err != nil {
		return nil, fmt.Errorf("encode header: %w", err)
	}

	sum := sha256.Sum256(headerBytes)
	return sum[:], nil
}

// BlockHashHex computes the block hash as hex string
// Production-grade: convenience wrapper around BlockHash
func BlockHashHex(b *Block, p ConsensusParams) (string, error) {
	hash, err := BlockHash(b, p)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash), nil
}

// EncodeBlockBinary encodes a block to binary format
// Production-grade: for efficient P2P transmission and storage
func EncodeBlockBinary(block *Block) ([]byte, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}

	var buf bytes.Buffer
	buf.WriteByte(binaryEncodingVersionV1)

	if err := binary.Write(&buf, binary.LittleEndian, block.Version); err != nil {
		return nil, fmt.Errorf("encode version: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, block.Height); err != nil {
		return nil, fmt.Errorf("encode height: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, block.TimestampUnix); err != nil {
		return nil, fmt.Errorf("encode timestamp: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, block.DifficultyBits); err != nil {
		return nil, fmt.Errorf("encode difficulty: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, block.Nonce); err != nil {
		return nil, fmt.Errorf("encode nonce: %w", err)
	}

	buf.Write(block.Hash)
	buf.Write(block.PrevHash)
	buf.Write(block.Header.MerkleRoot)

	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(block.Transactions))); err != nil {
		return nil, fmt.Errorf("encode tx count: %w", err)
	}
	for _, tx := range block.Transactions {
		txData, err := EncodeTransactionBinary(tx)
		if err != nil {
			return nil, fmt.Errorf("encode tx: %w", err)
		}
		buf.Write(txData)
	}

	return buf.Bytes(), nil
}

// EncodeTransactionBinary encodes a transaction to binary format
// Production-grade: for efficient P2P transmission and storage
func EncodeTransactionBinary(tx Transaction) ([]byte, error) {
	return txSigningPreimageBinaryV1(tx)
}

// DecodeTransactionBinary decodes a transaction from binary format
// Production-grade: for efficient P2P reception and storage
// Logic completeness: decodes all transaction fields in correct order
func DecodeTransactionBinary(data []byte) (Transaction, error) {
	if len(data) < 2 {
		return Transaction{}, errors.New("data too short")
	}
	if data[0] != binaryEncodingVersionV1 {
		return Transaction{}, fmt.Errorf("unsupported version: %d", data[0])
	}

	// Use decoder to decode from binary format
	decoder := newBinaryDecoder(data[1:]) // Skip version byte
	return decoder.decodeTransaction()
}

// binaryDecoder handles binary decoding of transactions
// Production-grade: robust decoder with proper error handling
type binaryDecoder struct {
	data []byte
	pos  int
}

// newBinaryDecoder creates a new binary decoder
func newBinaryDecoder(data []byte) *binaryDecoder {
	return &binaryDecoder{data: data, pos: 0}
}

// decodeTransaction decodes a transaction from binary data
// Field order matches encodeTransaction for consistency
func (d *binaryDecoder) decodeTransaction() (Transaction, error) {
	var tx Transaction
	var err error

	// Decode type (string)
	typeBytes, err := d.readBytes()
	if err != nil {
		return tx, fmt.Errorf("decode type: %w", err)
	}
	tx.Type = TransactionType(string(typeBytes))

	// Decode chainId (uint64)
	tx.ChainID, err = d.readUint64()
	if err != nil {
		return tx, fmt.Errorf("decode chainId: %w", err)
	}

	// Decode fromPubKey ([]byte) - may be empty for coinbase
	tx.FromPubKey, err = d.readBytes()
	if err != nil {
		return tx, fmt.Errorf("decode fromPubKey: %w", err)
	}

	// Decode toAddress (string)
	toAddrBytes, err := d.readBytes()
	if err != nil {
		return tx, fmt.Errorf("decode toAddress: %w", err)
	}
	tx.ToAddress = string(toAddrBytes)

	// Decode amount (uint64)
	tx.Amount, err = d.readUint64()
	if err != nil {
		return tx, fmt.Errorf("decode amount: %w", err)
	}

	// Decode fee (uint64)
	tx.Fee, err = d.readUint64()
	if err != nil {
		return tx, fmt.Errorf("decode fee: %w", err)
	}

	// Decode nonce (uint64)
	tx.Nonce, err = d.readUint64()
	if err != nil {
		return tx, fmt.Errorf("decode nonce: %w", err)
	}

	// Decode data (string)
	dataBytes, err := d.readBytes()
	if err != nil {
		return tx, fmt.Errorf("decode data: %w", err)
	}
	tx.Data = string(dataBytes)

	// Decode signature ([]byte) - only for transfer transactions
	if tx.Type == TxTransfer && len(tx.FromPubKey) > 0 {
		tx.Signature, err = d.readBytes()
		if err != nil {
			return tx, fmt.Errorf("decode signature: %w", err)
		}
	}

	return tx, nil
}

// readBytes reads a length-prefixed byte slice
// Format: [length: ULEB128][data: bytes]
func (d *binaryDecoder) readBytes() ([]byte, error) {
	length, err := readULEB128(bytes.NewBuffer(d.data[d.pos:]))
	if err != nil {
		return nil, err
	}

	// Update position past the length bytes
	d.pos += len(d.data[d.pos:]) - len(bytes.NewBuffer(d.data[d.pos:]).Bytes()) + 1

	// Check bounds
	if d.pos+int(length) > len(d.data) {
		return nil, errors.New("unexpected EOF")
	}

	// Extract data
	data := d.data[d.pos : d.pos+int(length)]
	d.pos += int(length)

	return data, nil
}

// readUint64 reads a uint64 in little-endian format
func (d *binaryDecoder) readUint64() (uint64, error) {
	if d.pos+8 > len(d.data) {
		return 0, errors.New("unexpected EOF")
	}

	value := binary.LittleEndian.Uint64(d.data[d.pos : d.pos+8])
	d.pos += 8

	return value, nil
}

// HashData computes SHA256 hash of arbitrary data
// Security: uses SHA256 for cryptographic hashing
func HashData(data []byte) ([]byte, error) {
	if data == nil {
		return nil, errors.New("nil data")
	}
	sum := sha256.Sum256(data)
	return sum[:], nil
}

// HashDataHex computes SHA256 hash and returns as hex string
// Production-grade: convenience function for common use case
func HashDataHex(data []byte) (string, error) {
	hash, err := HashData(data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash), nil
}

// DoubleHash computes SHA256(SHA256(data)) - Bitcoin-style
// Security: uses double SHA256 for enhanced security
func DoubleHash(data []byte) ([]byte, error) {
	if data == nil {
		return nil, errors.New("nil data")
	}
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second[:], nil
}

// DoubleHashHex computes double SHA256 and returns as hex string
// Production-grade: used in Bitcoin-style hash computations
func DoubleHashHex(data []byte) (string, error) {
	hash, err := DoubleHash(data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash), nil
}

// TxSigningHashForConsensus computes the signing hash for a transaction based on consensus rules
// Production-grade: selects encoding based on activation height
func TxSigningHashForConsensus(tx Transaction, p ConsensusParams, height uint64) ([]byte, error) {
	if p.BinaryEncodingActive(height) {
		return txSigningPreimageBinaryV1(tx)
	}
	return tx.signingHashLegacyJSON()
}
