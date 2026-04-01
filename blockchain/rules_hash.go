package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

const rulesHashVersionV3 = uint8(3)

// MarshalBinary serializes ConsensusParams into a deterministic byte slice.
// Version 3 layout (all fields little-endian):
// - version (1 byte) = 3
// - DifficultyEnable (1 byte, 0/1)
// - TargetBlockTime (int64, nanoseconds)
// - DifficultyWindow (uint32)
// - DifficultyMaxStep (uint32)
// - MinDifficultyBits (uint32)
// - MaxDifficultyBits (uint32)
// - GenesisDifficultyBits (uint32)
// - MedianTimePastWindow (uint32)
// - MaxTimeDrift (int64)
// - MerkleEnable (1 byte, 0/1)
// - MerkleActivationHeight (uint64)
// - BinaryEncodingEnable (1 byte, 0/1)
// - BinaryEncodingActivationHeight (uint64)
// - MaxBlockSize (uint64)
// - InitialBlockReward (uint64)
// - HalvingInterval (uint64)
// - MinerFeeShare (1 byte)
// - TailEmission (uint64)
//
// When adding fields, increment the version and append at the end.
func (p ConsensusParams) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	if err := buf.WriteByte(rulesHashVersionV3); err != nil {
		return nil, err
	}
	if err := writeBool(&buf, p.DifficultyEnable); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, int64(p.TargetBlockTime)); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, uint32(p.DifficultyWindow)); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.DifficultyMaxStep); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MinDifficultyBits); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MaxDifficultyBits); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.GenesisDifficultyBits); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, uint32(p.MedianTimePastWindow)); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MaxTimeDrift); err != nil {
		return nil, err
	}
	if err := writeBool(&buf, p.MerkleEnable); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MerkleActivationHeight); err != nil {
		return nil, err
	}
	if err := writeBool(&buf, p.BinaryEncodingEnable); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.BinaryEncodingActivationHeight); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MaxBlockSize); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MonetaryPolicy.InitialBlockReward); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MonetaryPolicy.HalvingInterval); err != nil {
		return nil, err
	}
	if err := buf.WriteByte(p.MonetaryPolicy.MinerFeeShare); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MonetaryPolicy.TailEmission); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RulesHash returns the SHA256 of the canonical binary encoding of ConsensusParams.
func (p ConsensusParams) RulesHash() ([32]byte, error) {
	preimage, err := p.MarshalBinary()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(preimage), nil
}

// MustRulesHash panics on error (expected to be safe for stable inputs).
func (p ConsensusParams) MustRulesHash() [32]byte {
	h, err := p.RulesHash()
	if err != nil {
		panic(fmt.Sprintf("failed to compute rules hash: %v", err))
	}
	return h
}

func writeBool(buf *bytes.Buffer, v bool) error {
	if v {
		return buf.WriteByte(1)
	}
	return buf.WriteByte(0)
}

func ConsensusRulesHash(p ConsensusParams) ([]byte, error) {
	h, err := p.RulesHash()
	if err != nil {
		return nil, err
	}
	return h[:], nil
}

func ConsensusRulesHashHex(p ConsensusParams) (string, error) {
	h, err := ConsensusRulesHash(p)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h), nil
}

// parseRulesHashHex reserved for future use //nolint:unused
func parseRulesHashHex(s string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b) != 32 {
		return nil, errors.New("rulesHash must be 32 bytes")
	}
	return b, nil
}
