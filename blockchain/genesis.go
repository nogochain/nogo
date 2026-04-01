package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nogochain/nogo/blockchain/nogopow"
)

func validateGenesisMinerAddress(addr string) error {
	if strings.HasPrefix(addr, "NOGO") {
		return ValidateAddress(addr)
	}
	b, err := hex.DecodeString(addr)
	if err != nil {
		return fmt.Errorf("invalid hex: %w", err)
	}
	if len(b) != 32 {
		return fmt.Errorf("raw address must be 32 bytes, got %d", len(b))
	}
	return nil
}

func NOGOToRawHex(addr string) string {
	if !strings.HasPrefix(addr, "NOGO") {
		return addr
	}
	encoded := addr[3:]
	decoded, _ := hex.DecodeString(encoded)
	if len(decoded) > 33 {
		return hex.EncodeToString(decoded[1:33])
	}
	return addr
}

type Uint64String uint64

func (u *Uint64String) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return errors.New("invalid uint64: empty")
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return errors.New("invalid uint64: empty string")
		}
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid uint64 string: %w", err)
		}
		*u = Uint64String(v)
		return nil
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&n); err != nil {
		return err
	}
	v, err := strconv.ParseUint(n.String(), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid uint64 number: %w", err)
	}
	*u = Uint64String(v)
	return nil
}

func (u Uint64String) Uint64() uint64 {
	return uint64(u)
}

type genesisConfigJSON struct {
	Network             string          `json:"network"`
	ChainID             uint64          `json:"chainId"`
	Timestamp           int64           `json:"timestamp"`
	GenesisMinerAddress string          `json:"genesisMinerAddress"`
	InitialSupply       Uint64String    `json:"initialSupply"`
	GenesisMessage      string          `json:"genesisMessage,omitempty"`
	MonetaryPolicy      json.RawMessage `json:"monetaryPolicy"`
	ConsensusParams     json.RawMessage `json:"consensusParams"`
}

type GenesisConfig struct {
	Network             string
	ChainID             uint64
	Timestamp           int64
	GenesisMinerAddress string
	InitialSupply       uint64
	GenesisMessage      string
	MonetaryPolicy      MonetaryPolicy
	ConsensusParams     ConsensusParams
}

func GenesisPathFromEnv(chainID uint64) (string, error) {
	if path := strings.TrimSpace(os.Getenv("GENESIS_PATH")); path != "" {
		return path, nil
	}
	if network := strings.TrimSpace(os.Getenv("GENESIS_NETWORK")); network != "" {
		return filepath.Join("genesis", network+".json"), nil
	}
	if network := strings.TrimSpace(os.Getenv("NETWORK")); network != "" {
		return filepath.Join("genesis", network+".json"), nil
	}
	switch chainID {
	case 0, 1:
		return filepath.Join("genesis", "mainnet.json"), nil
	case 2:
		return filepath.Join("genesis", "testnet.json"), nil
	default:
		return "", fmt.Errorf("GENESIS_PATH is required for chainId=%d", chainID)
	}
}

func LoadGenesisConfig(path string) (*GenesisConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read genesis file %s: %w", path, err)
	}

	var raw genesisConfigJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse genesis config: %w", err)
	}
	if raw.ChainID == 0 {
		return nil, errors.New("genesis chainId must be > 0")
	}
	if raw.Timestamp <= 0 {
		return nil, errors.New("genesis timestamp must be > 0")
	}
	if raw.InitialSupply.Uint64() == 0 {
		return nil, errors.New("genesis initialSupply must be > 0")
	}
	if err := validateGenesisMinerAddress(raw.GenesisMinerAddress); err != nil {
		return nil, fmt.Errorf("invalid genesisMinerAddress: %w", err)
	}

	policy, err := parseMonetaryPolicy(raw.MonetaryPolicy)
	if err != nil {
		return nil, err
	}
	consensus, err := parseConsensusParams(raw.ConsensusParams)
	if err != nil {
		return nil, err
	}
	consensus.MonetaryPolicy = policy

	cfg := &GenesisConfig{
		Network:             raw.Network,
		ChainID:             raw.ChainID,
		Timestamp:           raw.Timestamp,
		GenesisMinerAddress: raw.GenesisMinerAddress,
		InitialSupply:       raw.InitialSupply.Uint64(),
		GenesisMessage:      raw.GenesisMessage,
		MonetaryPolicy:      policy,
		ConsensusParams:     consensus,
	}
	return cfg, nil
}

type consensusParamsJSON struct {
	DifficultyEnable               *bool   `json:"difficultyEnable"`
	DifficultyTargetMs             *int64  `json:"difficultyTargetMs"`
	DifficultyTargetSpacing        *int64  `json:"difficultyTargetSpacing"`
	DifficultyWindow               *int    `json:"difficultyWindow"`
	DifficultyWindowSize           *int    `json:"difficultyWindowSize"`
	DifficultyAdjustmentInterval   *int    `json:"difficultyAdjustmentInterval"`
	DifficultyMaxStepBits          *uint32 `json:"difficultyMaxStepBits"`
	DifficultyMaxStep              *uint32 `json:"difficultyMaxStep"`
	MinDifficultyBits              *uint32 `json:"difficultyMinBits"`
	MaxDifficultyBits              *uint32 `json:"difficultyMaxBits"`
	GenesisDifficultyBits          *uint32 `json:"genesisDifficultyBits"`
	MedianTimePastWindow           *int    `json:"medianTimePastWindow"`
	MaxTimeDrift                   *int64  `json:"maxTimeDrift"`
	MaxBlockSize                   *uint64 `json:"maxBlockSize"`
	MerkleEnable                   *bool   `json:"merkleEnable"`
	MerkleActivationHeight         *uint64 `json:"merkleActivationHeight"`
	BinaryEncodingEnable           *bool   `json:"binaryEncodingEnable"`
	BinaryEncodingActivationHeight *uint64 `json:"binaryEncodingActivationHeight"`
}

func parseConsensusParams(raw json.RawMessage) (ConsensusParams, error) {
	if len(raw) == 0 {
		return ConsensusParams{}, errors.New("genesis consensusParams is required")
	}
	var aux consensusParamsJSON
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&aux); err != nil {
		return ConsensusParams{}, fmt.Errorf("parse consensusParams: %w", err)
	}

	if aux.DifficultyEnable == nil {
		return ConsensusParams{}, errors.New("consensusParams.difficultyEnable is required")
	}
	if aux.MerkleEnable == nil {
		return ConsensusParams{}, errors.New("consensusParams.merkleEnable is required")
	}
	if aux.BinaryEncodingEnable == nil {
		return ConsensusParams{}, errors.New("consensusParams.binaryEncodingEnable is required")
	}

	targetMs, err := pickInt64("difficultyTarget", aux.DifficultyTargetMs, toMillis(aux.DifficultyTargetSpacing))
	if err != nil {
		return ConsensusParams{}, err
	}
	window, err := pickInt("difficultyWindow", aux.DifficultyWindow, aux.DifficultyWindowSize, aux.DifficultyAdjustmentInterval)
	if err != nil {
		return ConsensusParams{}, err
	}
	maxStep, err := pickUint32("difficultyMaxStepBits", aux.DifficultyMaxStepBits, aux.DifficultyMaxStep)
	if err != nil {
		return ConsensusParams{}, err
	}
	minBits, err := requireUint32("difficultyMinBits", aux.MinDifficultyBits)
	if err != nil {
		return ConsensusParams{}, err
	}
	maxBits, err := requireUint32("difficultyMaxBits", aux.MaxDifficultyBits)
	if err != nil {
		return ConsensusParams{}, err
	}
	genesisBits, err := requireUint32("genesisDifficultyBits", aux.GenesisDifficultyBits)
	if err != nil {
		return ConsensusParams{}, err
	}
	mtpWindow, err := requireInt("medianTimePastWindow", aux.MedianTimePastWindow)
	if err != nil {
		return ConsensusParams{}, err
	}
	maxTimeDrift, err := requireInt64("maxTimeDrift", aux.MaxTimeDrift)
	if err != nil {
		return ConsensusParams{}, err
	}
	maxBlockSize, err := requireUint64("maxBlockSize", aux.MaxBlockSize)
	if err != nil {
		return ConsensusParams{}, err
	}
	merkleHeight, err := requireUint64("merkleActivationHeight", aux.MerkleActivationHeight)
	if err != nil {
		return ConsensusParams{}, err
	}
	binaryHeight, err := requireUint64("binaryEncodingActivationHeight", aux.BinaryEncodingActivationHeight)
	if err != nil {
		return ConsensusParams{}, err
	}

	p := ConsensusParams{
		DifficultyEnable:               *aux.DifficultyEnable,
		TargetBlockTime:                time.Duration(targetMs) * time.Millisecond,
		DifficultyWindow:               window,
		DifficultyMaxStep:              maxStep,
		MinDifficultyBits:              minBits,
		MaxDifficultyBits:              maxBits,
		GenesisDifficultyBits:          genesisBits,
		MedianTimePastWindow:           mtpWindow,
		MaxTimeDrift:                   maxTimeDrift,
		MaxBlockSize:                   maxBlockSize,
		MerkleEnable:                   *aux.MerkleEnable,
		MerkleActivationHeight:         merkleHeight,
		BinaryEncodingEnable:           *aux.BinaryEncodingEnable,
		BinaryEncodingActivationHeight: binaryHeight,
	}
	if err := validateConsensusParams(p); err != nil {
		return ConsensusParams{}, err
	}
	return p, nil
}

func toMillis(v *int64) *int64 {
	if v == nil {
		return nil
	}
	ms := *v * 1000
	return &ms
}

func pickInt(name string, values ...*int) (int, error) {
	var out int
	var set bool
	for _, v := range values {
		if v == nil {
			continue
		}
		if !set {
			out = *v
			set = true
			continue
		}
		if *v != out {
			return 0, fmt.Errorf("consensusParams.%s mismatch: %d vs %d", name, out, *v)
		}
	}
	if !set {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return out, nil
}

func pickInt64(name string, values ...*int64) (int64, error) {
	var out int64
	var set bool
	for _, v := range values {
		if v == nil {
			continue
		}
		if !set {
			out = *v
			set = true
			continue
		}
		if *v != out {
			return 0, fmt.Errorf("consensusParams.%s mismatch: %d vs %d", name, out, *v)
		}
	}
	if !set {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return out, nil
}

func pickUint32(name string, values ...*uint32) (uint32, error) {
	var out uint32
	var set bool
	for _, v := range values {
		if v == nil {
			continue
		}
		if !set {
			out = *v
			set = true
			continue
		}
		if *v != out {
			return 0, fmt.Errorf("consensusParams.%s mismatch: %d vs %d", name, out, *v)
		}
	}
	if !set {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return out, nil
}

func requireInt(name string, v *int) (int, error) {
	if v == nil {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return *v, nil
}

func requireInt64(name string, v *int64) (int64, error) {
	if v == nil {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return *v, nil
}

func requireUint32(name string, v *uint32) (uint32, error) {
	if v == nil {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return *v, nil
}

func requireUint64(name string, v *uint64) (uint64, error) {
	if v == nil {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return *v, nil
}

func validateConsensusParams(p ConsensusParams) error {
	if p.TargetBlockTime <= 0 {
		return errors.New("consensusParams.difficultyTarget must be > 0")
	}
	if p.DifficultyWindow <= 0 {
		return errors.New("consensusParams.difficultyWindow must be > 0")
	}
	if p.DifficultyMaxStep == 0 {
		return errors.New("consensusParams.difficultyMaxStepBits must be > 0")
	}
	if p.MinDifficultyBits == 0 {
		return errors.New("consensusParams.difficultyMinBits must be > 0")
	}
	if p.MaxDifficultyBits == 0 {
		return errors.New("consensusParams.difficultyMaxBits must be > 0")
	}
	if p.MaxDifficultyBits > maxDifficultyBits {
		return fmt.Errorf("consensusParams.difficultyMaxBits must be <= %d", maxDifficultyBits)
	}
	if p.MinDifficultyBits > p.MaxDifficultyBits {
		return errors.New("consensusParams.difficultyMinBits must be <= difficultyMaxBits")
	}
	if p.GenesisDifficultyBits < p.MinDifficultyBits || p.GenesisDifficultyBits > p.MaxDifficultyBits {
		return errors.New("consensusParams.genesisDifficultyBits must be within min/max difficulty bits")
	}
	if p.MedianTimePastWindow <= 0 {
		return errors.New("consensusParams.medianTimePastWindow must be > 0")
	}
	if p.MaxTimeDrift <= 0 {
		return errors.New("consensusParams.maxTimeDrift must be > 0")
	}
	if p.MaxBlockSize == 0 {
		return errors.New("consensusParams.maxBlockSize must be > 0")
	}
	return nil
}

func genesisMessageOrDefault(cfg *GenesisConfig) string {
	if cfg.GenesisMessage != "" {
		return cfg.GenesisMessage
	}
	return fmt.Sprintf("genesis allocation (supply=%d)", cfg.InitialSupply)
}

func BuildGenesisBlock(cfg *GenesisConfig, consensus ConsensusParams) (*Block, error) {
	if cfg == nil {
		return nil, errors.New("missing genesis config")
	}
	msg := genesisMessageOrDefault(cfg)
	coinbase := Transaction{
		Type:      TxCoinbase,
		ChainID:   cfg.ChainID,
		ToAddress: cfg.GenesisMinerAddress,
		Amount:    cfg.InitialSupply,
		Data:      msg,
	}
	genesis := &Block{
		Version:        blockVersionForHeight(consensus, 0),
		Height:         0,
		TimestampUnix:  cfg.Timestamp,
		DifficultyBits: consensus.GenesisDifficultyBits,
		MinerAddress:   cfg.GenesisMinerAddress,
		Transactions:   []Transaction{coinbase},
	}

	// Mine genesis block using NogoPow engine
	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()

	genesisHeader := &nogopow.Header{
		ParentHash: nogopow.BytesToHash(genesis.PrevHash),
		Coinbase:   stringToAddress(genesis.MinerAddress),
		Number:     big.NewInt(int64(genesis.Height)),
		Time:       uint64(genesis.TimestampUnix),
		Difficulty: big.NewInt(int64(genesis.DifficultyBits)),
	}

	genesisBlock := nogopow.NewBlock(genesisHeader, nil, nil, nil)
	stop := make(chan struct{})
	resultCh := make(chan *nogopow.Block, 1)

	if err := engine.Seal(nil, genesisBlock, resultCh, stop); err != nil {
		return nil, err
	}

	result, ok := <-resultCh
	if !ok {
		close(stop)
		return nil, fmt.Errorf("genesis mining failed")
	}

	sealedHeader := result.Header()
	genesis.Nonce = binary.LittleEndian.Uint64(sealedHeader.Nonce[:8])
	hashBytes := sealedHeader.Hash().Bytes()
	genesis.Hash = hashBytes

	return genesis, nil
}

func ValidateGenesisBlock(b *Block, cfg *GenesisConfig, consensus ConsensusParams) error {
	if b == nil {
		return errors.New("missing genesis block")
	}
	if b.Height != 0 {
		return fmt.Errorf("invalid genesis height: %d", b.Height)
	}
	if len(b.PrevHash) != 0 {
		return errors.New("invalid genesis prevHash")
	}
	if b.Version != blockVersionForHeight(consensus, 0) {
		return fmt.Errorf("invalid genesis version: %d", b.Version)
	}
	if b.TimestampUnix != cfg.Timestamp {
		return fmt.Errorf("genesis timestamp mismatch: %d != %d", b.TimestampUnix, cfg.Timestamp)
	}
	if b.MinerAddress != cfg.GenesisMinerAddress {
		return fmt.Errorf("genesis miner mismatch: %s != %s", b.MinerAddress, cfg.GenesisMinerAddress)
	}
	if b.DifficultyBits != consensus.GenesisDifficultyBits {
		return fmt.Errorf("genesis difficulty mismatch: %d != %d", b.DifficultyBits, consensus.GenesisDifficultyBits)
	}
	if len(b.Transactions) != 1 {
		return errors.New("genesis must contain exactly one transaction")
	}
	cb := b.Transactions[0]
	if cb.Type != TxCoinbase {
		return errors.New("genesis tx must be coinbase")
	}
	if cb.ChainID != cfg.ChainID {
		return fmt.Errorf("genesis coinbase chainId mismatch: %d != %d", cb.ChainID, cfg.ChainID)
	}
	if cb.ToAddress != cfg.GenesisMinerAddress {
		return fmt.Errorf("genesis coinbase toAddress mismatch: %s != %s", cb.ToAddress, cfg.GenesisMinerAddress)
	}
	if cb.Amount != cfg.InitialSupply {
		return fmt.Errorf("genesis supply mismatch: %d != %d", cb.Amount, cfg.InitialSupply)
	}
	if cb.Data != genesisMessageOrDefault(cfg) {
		return fmt.Errorf("genesis message mismatch: %q != %q", cb.Data, genesisMessageOrDefault(cfg))
	}

	// Validate genesis block using NogoPow engine
	if err := validateGenesisPoWNogoPow(consensus, b); err != nil {
		return err
	}

	_, err := ensureBlockHash(b, consensus)
	return err
}

// validateGenesisPoWNogoPow validates genesis block PoW using NogoPow algorithm
func validateGenesisPoWNogoPow(consensus ConsensusParams, b *Block) error {
	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()

	header := &nogopow.Header{
		ParentHash: nogopow.BytesToHash(b.PrevHash),
		Coinbase:   stringToAddress(b.MinerAddress),
		Number:     big.NewInt(int64(b.Height)),
		Time:       uint64(b.TimestampUnix),
		Difficulty: big.NewInt(int64(b.DifficultyBits)),
		Nonce:      nogopow.BlockNonce{},
		Extra:      []byte{},
	}

	// Set nonce
	binary.LittleEndian.PutUint64(header.Nonce[:8], b.Nonce)

	// Verify the header
	if err := engine.VerifyHeader(nil, header, false); err != nil {
		return fmt.Errorf("invalid genesis pow: %w", err)
	}

	return nil
}

func ensureBlockHash(b *Block, consensus ConsensusParams) ([]byte, error) {
	header, err := b.HeaderBytesForConsensus(consensus, b.Nonce)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(header)
	if len(b.Hash) == 0 {
		b.Hash = append([]byte(nil), sum[:]...)
	}
	return sum[:], nil
}
