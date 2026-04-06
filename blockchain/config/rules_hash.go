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

package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log"
)

const rulesHashVersion = uint8(3)

// MarshalBinary serializes ConsensusParams into a deterministic byte slice
func (p ConsensusParams) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	if err := buf.WriteByte(rulesHashVersion); err != nil {
		return nil, err
	}
	if err := writeBool(&buf, p.DifficultyEnable); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.BlockTimeTargetSeconds); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, uint32(p.DifficultyAdjustmentInterval)); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MaxDifficultyChangePercent); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MinDifficulty); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MaxDifficulty); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MaxDifficulty); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, uint32(p.MedianTimePastWindow)); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, p.MaxBlockTimeDriftSeconds); err != nil {
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

// RulesHash returns the SHA256 of the canonical binary encoding
func (p ConsensusParams) RulesHash() ([32]byte, error) {
	preimage, err := p.MarshalBinary()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(preimage), nil
}

// MustRulesHash returns rules hash, logging error and returning zero value on failure
func (p ConsensusParams) MustRulesHash() [32]byte {
	h, err := p.RulesHash()
	if err != nil {
		log.Printf("config: failed to compute rules hash: %v", err)
		return [32]byte{}
	}
	return h
}

// ConsensusRulesHash returns rules hash as byte slice
func ConsensusRulesHash(p ConsensusParams) ([]byte, error) {
	h, err := p.RulesHash()
	if err != nil {
		return nil, err
	}
	return h[:], nil
}

// ConsensusRulesHashHex returns rules hash as hex string
func ConsensusRulesHashHex(p ConsensusParams) (string, error) {
	h, err := ConsensusRulesHash(p)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h), nil
}

func writeBool(buf *bytes.Buffer, v bool) error {
	if v {
		return buf.WriteByte(1)
	}
	return buf.WriteByte(0)
}

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
