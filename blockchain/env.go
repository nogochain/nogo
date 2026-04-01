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
