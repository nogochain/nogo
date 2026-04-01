package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func envBool(name string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	v = strings.ToLower(v)
	return v == "1" || v == "true" || v == "yes" || v == "y"
}

func envInt(name string, def int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envUint32(name string, def uint32) uint32 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return def
	}
	return uint32(n)
}

func envUint64(name string, def uint64) uint64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func envInt64(name string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func envDurationMS(name string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms <= 0 {
		return def
	}
	return time.Duration(ms) * time.Millisecond
}

func envIsSet(name string) bool {
	return strings.TrimSpace(os.Getenv(name)) != ""
}

func consensusEnvOverridesSet() bool {
	names := []string{
		"DIFFICULTY_ENABLE",
		"DIFFICULTY_TARGET_MS",
		"DIFFICULTY_WINDOW",
		"DIFFICULTY_MAX_STEP",
		"DIFFICULTY_MIN_BITS",
		"DIFFICULTY_MAX_BITS",
		"GENESIS_DIFFICULTY_BITS",
		"MTP_WINDOW",
		"MAX_TIME_DRIFT",
		"MAX_FUTURE_DRIFT_SEC",
		"MAX_BLOCK_SIZE",
		"MERKLE_ENABLE",
		"MERKLE_ACTIVATION_HEIGHT",
		"BINARY_ENCODING_ENABLE",
		"BINARY_ENCODING_ACTIVATION_HEIGHT",
	}
	for _, name := range names {
		if envIsSet(name) {
			return true
		}
	}
	return false
}

const (
	defaultDifficultyBits = uint32(18)
)

// defaultConsensusParamsFromEnv initializes consensus parameters from environment variables
// Note: For production use, genesis.json should be the authoritative source
func defaultConsensusParamsFromEnv() ConsensusParams {
	p := ConsensusParams{
		DifficultyEnable:      envBool("DIFFICULTY_ENABLE", false),
		TargetBlockTime:       envDurationMS("DIFFICULTY_TARGET_MS", 15*time.Second),
		DifficultyWindow:      envInt("DIFFICULTY_WINDOW", 20),
		DifficultyMaxStep:     envUint32("DIFFICULTY_MAX_STEP", 1),
		MinDifficultyBits:     envUint32("DIFFICULTY_MIN_BITS", 1),
		MaxDifficultyBits:     envUint32("DIFFICULTY_MAX_BITS", 255),
		GenesisDifficultyBits: envUint32("GENESIS_DIFFICULTY_BITS", defaultDifficultyBits),
		MedianTimePastWindow:  envInt("MTP_WINDOW", 11),
		MaxTimeDrift:          envInt64("MAX_TIME_DRIFT", envInt64("MAX_FUTURE_DRIFT_SEC", 2*60*60)),
		MaxBlockSize:          envUint64("MAX_BLOCK_SIZE", 1_000_000),
		MerkleEnable:          envBool("MERKLE_ENABLE", false),
		MerkleActivationHeight: envUint64(
			"MERKLE_ACTIVATION_HEIGHT",
			0,
		),
		BinaryEncodingEnable: envBool("BINARY_ENCODING_ENABLE", false),
		BinaryEncodingActivationHeight: envUint64(
			"BINARY_ENCODING_ACTIVATION_HEIGHT",
			0,
		),
	}

	if p.TargetBlockTime <= 0 {
		p.TargetBlockTime = 15 * time.Second
	}
	if p.DifficultyWindow <= 0 {
		p.DifficultyWindow = 20
	}
	if p.DifficultyMaxStep == 0 {
		p.DifficultyMaxStep = 1
	}
	if p.MinDifficultyBits == 0 {
		p.MinDifficultyBits = 1
	}
	if p.MaxDifficultyBits == 0 {
		p.MaxDifficultyBits = 255
	}
	if p.MaxDifficultyBits > maxDifficultyBits {
		p.MaxDifficultyBits = maxDifficultyBits
	}
	if p.MinDifficultyBits > p.MaxDifficultyBits {
		p.MinDifficultyBits = p.MaxDifficultyBits
	}
	if p.GenesisDifficultyBits < p.MinDifficultyBits {
		p.GenesisDifficultyBits = p.MinDifficultyBits
	}
	if p.GenesisDifficultyBits > p.MaxDifficultyBits {
		p.GenesisDifficultyBits = p.MaxDifficultyBits
	}
	if p.MedianTimePastWindow <= 0 {
		p.MedianTimePastWindow = 11
	}
	if p.MaxTimeDrift <= 0 {
		p.MaxTimeDrift = 2 * 60 * 60
	}
	if p.MaxBlockSize == 0 {
		p.MaxBlockSize = 1_000_000
	}
	return p
}
