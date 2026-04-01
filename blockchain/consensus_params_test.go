package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"
)

func TestConsensusParamsMarshalBinary(t *testing.T) {
	p := ConsensusParams{
		DifficultyEnable:               true,
		TargetBlockTime:                15 * time.Second,
		DifficultyWindow:               20,
		DifficultyMaxStep:              2,
		MinDifficultyBits:              1,
		MaxDifficultyBits:              200,
		GenesisDifficultyBits:          18,
		MedianTimePastWindow:           11,
		MaxTimeDrift:                   7200,
		MaxBlockSize:                   1_000_000,
		MerkleEnable:                   true,
		MerkleActivationHeight:         100,
		BinaryEncodingEnable:           true,
		BinaryEncodingActivationHeight: 50,
		MonetaryPolicy: MonetaryPolicy{
			InitialBlockReward: 5_000_000_000,
			HalvingInterval:    210000,
			MinerFeeShare:      100,
			TailEmission:       0,
		},
	}

	preimage, err := p.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if len(preimage) == 0 {
		t.Fatal("expected non-empty preimage")
	}

	r := bytes.NewReader(preimage)
	version, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != rulesHashVersionV3 {
		t.Fatalf("unexpected version: %d", version)
	}

	readBool := func(name string) bool {
		b, err := r.ReadByte()
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return b == 1
	}

	if got := readBool("DifficultyEnable"); got != p.DifficultyEnable {
		t.Fatalf("DifficultyEnable: expected %v got %v", p.DifficultyEnable, got)
	}

	var targetBlockTime int64
	if err := binary.Read(r, binary.LittleEndian, &targetBlockTime); err != nil {
		t.Fatalf("TargetBlockTime: %v", err)
	}
	if time.Duration(targetBlockTime) != p.TargetBlockTime {
		t.Fatalf("TargetBlockTime: expected %v got %v", p.TargetBlockTime, time.Duration(targetBlockTime))
	}

	var difficultyWindow uint32
	if err := binary.Read(r, binary.LittleEndian, &difficultyWindow); err != nil {
		t.Fatalf("DifficultyWindow: %v", err)
	}
	if int(difficultyWindow) != p.DifficultyWindow {
		t.Fatalf("DifficultyWindow: expected %d got %d", p.DifficultyWindow, difficultyWindow)
	}

	var difficultyMaxStep uint32
	if err := binary.Read(r, binary.LittleEndian, &difficultyMaxStep); err != nil {
		t.Fatalf("DifficultyMaxStep: %v", err)
	}
	if difficultyMaxStep != p.DifficultyMaxStep {
		t.Fatalf("DifficultyMaxStep: expected %d got %d", p.DifficultyMaxStep, difficultyMaxStep)
	}

	var minBits uint32
	if err := binary.Read(r, binary.LittleEndian, &minBits); err != nil {
		t.Fatalf("MinDifficultyBits: %v", err)
	}
	if minBits != p.MinDifficultyBits {
		t.Fatalf("MinDifficultyBits: expected %d got %d", p.MinDifficultyBits, minBits)
	}

	var maxBits uint32
	if err := binary.Read(r, binary.LittleEndian, &maxBits); err != nil {
		t.Fatalf("MaxDifficultyBits: %v", err)
	}
	if maxBits != p.MaxDifficultyBits {
		t.Fatalf("MaxDifficultyBits: expected %d got %d", p.MaxDifficultyBits, maxBits)
	}

	var genesisBits uint32
	if err := binary.Read(r, binary.LittleEndian, &genesisBits); err != nil {
		t.Fatalf("GenesisDifficultyBits: %v", err)
	}
	if genesisBits != p.GenesisDifficultyBits {
		t.Fatalf("GenesisDifficultyBits: expected %d got %d", p.GenesisDifficultyBits, genesisBits)
	}

	var mtpWindow uint32
	if err := binary.Read(r, binary.LittleEndian, &mtpWindow); err != nil {
		t.Fatalf("MedianTimePastWindow: %v", err)
	}
	if int(mtpWindow) != p.MedianTimePastWindow {
		t.Fatalf("MedianTimePastWindow: expected %d got %d", p.MedianTimePastWindow, mtpWindow)
	}

	var maxTimeDrift int64
	if err := binary.Read(r, binary.LittleEndian, &maxTimeDrift); err != nil {
		t.Fatalf("MaxTimeDrift: %v", err)
	}
	if maxTimeDrift != p.MaxTimeDrift {
		t.Fatalf("MaxTimeDrift: expected %d got %d", p.MaxTimeDrift, maxTimeDrift)
	}

	if got := readBool("MerkleEnable"); got != p.MerkleEnable {
		t.Fatalf("MerkleEnable: expected %v got %v", p.MerkleEnable, got)
	}

	var merkleHeight uint64
	if err := binary.Read(r, binary.LittleEndian, &merkleHeight); err != nil {
		t.Fatalf("MerkleActivationHeight: %v", err)
	}
	if merkleHeight != p.MerkleActivationHeight {
		t.Fatalf("MerkleActivationHeight: expected %d got %d", p.MerkleActivationHeight, merkleHeight)
	}

	if got := readBool("BinaryEncodingEnable"); got != p.BinaryEncodingEnable {
		t.Fatalf("BinaryEncodingEnable: expected %v got %v", p.BinaryEncodingEnable, got)
	}

	var binaryHeight uint64
	if err := binary.Read(r, binary.LittleEndian, &binaryHeight); err != nil {
		t.Fatalf("BinaryEncodingActivationHeight: %v", err)
	}
	if binaryHeight != p.BinaryEncodingActivationHeight {
		t.Fatalf("BinaryEncodingActivationHeight: expected %d got %d", p.BinaryEncodingActivationHeight, binaryHeight)
	}

	var maxBlockSize uint64
	if err := binary.Read(r, binary.LittleEndian, &maxBlockSize); err != nil {
		t.Fatalf("MaxBlockSize: %v", err)
	}
	if maxBlockSize != p.MaxBlockSize {
		t.Fatalf("MaxBlockSize: expected %d got %d", p.MaxBlockSize, maxBlockSize)
	}

	var initialReward uint64
	if err := binary.Read(r, binary.LittleEndian, &initialReward); err != nil {
		t.Fatalf("InitialBlockReward: %v", err)
	}
	if initialReward != p.MonetaryPolicy.InitialBlockReward {
		t.Fatalf("InitialBlockReward: expected %d got %d", p.MonetaryPolicy.InitialBlockReward, initialReward)
	}

	var halvingInterval uint64
	if err := binary.Read(r, binary.LittleEndian, &halvingInterval); err != nil {
		t.Fatalf("HalvingInterval: %v", err)
	}
	if halvingInterval != p.MonetaryPolicy.HalvingInterval {
		t.Fatalf("HalvingInterval: expected %d got %d", p.MonetaryPolicy.HalvingInterval, halvingInterval)
	}

	minerFeeShare, err := r.ReadByte()
	if err != nil {
		t.Fatalf("MinerFeeShare: %v", err)
	}
	if minerFeeShare != p.MonetaryPolicy.MinerFeeShare {
		t.Fatalf("MinerFeeShare: expected %d got %d", p.MonetaryPolicy.MinerFeeShare, minerFeeShare)
	}

	var tailEmission uint64
	if err := binary.Read(r, binary.LittleEndian, &tailEmission); err != nil {
		t.Fatalf("TailEmission: %v", err)
	}
	if tailEmission != p.MonetaryPolicy.TailEmission {
		t.Fatalf("TailEmission: expected %d got %d", p.MonetaryPolicy.TailEmission, tailEmission)
	}

	if r.Len() != 0 {
		t.Fatalf("expected no trailing bytes, got %d", r.Len())
	}

	h, err := p.RulesHash()
	if err != nil {
		t.Fatalf("RulesHash: %v", err)
	}
	want := sha256.Sum256(preimage)
	if h != want {
		t.Fatalf("RulesHash mismatch")
	}

	p2 := p
	p2.MonetaryPolicy.MinerFeeShare = 50
	h2, err := p2.RulesHash()
	if err != nil {
		t.Fatalf("RulesHash (p2): %v", err)
	}
	if h2 == h {
		t.Fatalf("expected hash to change when params change")
	}
}
