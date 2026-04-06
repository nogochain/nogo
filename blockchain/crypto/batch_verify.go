package crypto

import (
	"crypto/ed25519"
	"errors"
)

// Batch verification constants for signature verification
const (
	// BATCH_VERIFY_THRESHOLD is the minimum number of transactions before using batch verification
	BATCH_VERIFY_THRESHOLD = 10

	// BATCH_VERIFY_MAX_SIZE is the maximum batch size for signature verification
	BATCH_VERIFY_MAX_SIZE = 100
)

// PublicKey is an alias for ed25519 public key
type PublicKey = []byte

// ErrInvalidBatchSize is returned when batch sizes don't match
var ErrInvalidBatchSize = errors.New("invalid batch size: pubKeys, messages, and signatures must have same length")

// Verify verifies a single ed25519 signature
func Verify(pubKey PublicKey, message, signature []byte) bool {
	if len(pubKey) != 32 || len(signature) != 64 {
		return false
	}
	return ed25519.Verify(pubKey, message, signature)
}

// Sign signs a message with ed25519
func Sign(privateKey []byte, message []byte) []byte {
	return ed25519.Sign(privateKey, message)
}

// VerifyBatch verifies multiple signatures in batch
// Returns a slice of bool indicating which signatures are valid
func VerifyBatch(pubKeys []PublicKey, messages [][]byte, signatures [][]byte) ([]bool, error) {
	if len(pubKeys) != len(messages) || len(pubKeys) != len(signatures) {
		return nil, ErrInvalidBatchSize
	}

	results := make([]bool, len(pubKeys))
	for i := range pubKeys {
		valid := Verify(pubKeys[i], messages[i], signatures[i])
		results[i] = valid
	}

	return results, nil
}
