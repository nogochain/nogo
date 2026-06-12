package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

const (
	AddressPrefix  = "NOGO"
	AddressVersion = 0x00
	ChecksumLen    = 4
	HashLen        = 32
)

const (
	BATCH_VERIFY_THRESHOLD = 10
	BATCH_VERIFY_MAX_SIZE  = 1000
)

type Address struct {
	Version  byte
	Hash     []byte
	Checksum []byte
}

func GenerateAddress(pubKey []byte) string {
	hash := sha256.Sum256(pubKey)
	addressHash := hash[:HashLen]

	addressData := make([]byte, 1+len(addressHash))
	addressData[0] = AddressVersion
	copy(addressData[1:], addressHash)

	checksum := sha256.Sum256(addressData)
	addressData = append(addressData, checksum[:ChecksumLen]...)

	encoded := hex.EncodeToString(addressData)

	return fmt.Sprintf("%s%s", AddressPrefix, encoded)
}

func ValidateAddress(addr string) error {
	if len(addr) < len(AddressPrefix)+10 {
		return fmt.Errorf("address too short")
	}

	if addr[:len(AddressPrefix)] != AddressPrefix {
		return fmt.Errorf("invalid prefix, expected %s", AddressPrefix)
	}

	encoded := addr[len(AddressPrefix):]

	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("invalid hex: %w", err)
	}

	if len(decoded) < ChecksumLen+1 {
		return fmt.Errorf("invalid encoded length")
	}

	addressData := decoded[:len(decoded)-ChecksumLen]
	storedChecksum := decoded[len(decoded)-ChecksumLen:]

	checksum := sha256.Sum256(addressData)

	for i := 0; i < ChecksumLen; i++ {
		if storedChecksum[i] != checksum[i] {
			return fmt.Errorf("checksum mismatch")
		}
	}

	return nil
}

func GetAddressFromPubKey(pubKey []byte) string {
	return GenerateAddress(pubKey)
}

func DecodeAddress(addr string) ([]byte, error) {
	if addr[:len(AddressPrefix)] != AddressPrefix {
		return nil, fmt.Errorf("invalid prefix")
	}

	encoded := addr[len(AddressPrefix):]
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	if len(decoded) < ChecksumLen {
		return nil, fmt.Errorf("invalid encoded length")
	}

	return decoded[:len(decoded)-ChecksumLen], nil
}

func FormatAddress(addr string) string {
	if len(addr) <= 16 {
		return addr
	}
	return addr[:8] + "..." + addr[len(addr)-8:]
}

func IsValidNogoAddress(addr string) bool {
	return ValidateAddress(addr) == nil
}

func GenerateTestAddress(seed byte) string {
	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = seed
	}
	return GenerateAddress(pub)
}

func GenerateTestAddress2(seed1, seed2 byte) string {
	pub := make([]byte, 32)
	for i := range pub {
		if i%2 == 0 {
			pub[i] = seed1
		} else {
			pub[i] = seed2
		}
	}
	return GenerateAddress(pub)
}

var (
	TestAddressA     = GenerateTestAddress(0x01)
	TestAddressB     = GenerateTestAddress2(0x02, 0x03)
	TestAddressC     = GenerateTestAddress2(0x04, 0x05)
	TestAddressMiner = GenerateTestAddress(0x10)
)

func GenerateKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func Sign(privKey ed25519.PrivateKey, message []byte) []byte {
	return ed25519.Sign(privKey, message)
}

func Verify(pubKey ed25519.PublicKey, message []byte, signature []byte) bool {
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pubKey, message, signature)
}

func DoubleSHA256(data []byte) []byte {
	h1 := sha256.Sum256(data)
	h2 := sha256.Sum256(h1[:])
	return h2[:]
}

func Hash256(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

func Hash160(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:20]
}

type PublicKey = ed25519.PublicKey
type Signature = []byte

// VerifyBatch performs parallel per-item verification of Ed25519 signatures.
// Signatures are verified individually using crypto/ed25519.Verify,
// with concurrent workers for batches exceeding BATCH_VERIFY_THRESHOLD (10).
func VerifyBatch(pubKeys []PublicKey, messages [][]byte, signatures [][]byte) ([]bool, error) {
	if len(pubKeys) != len(messages) || len(messages) != len(signatures) {
		return nil, fmt.Errorf("batch size mismatch: pubKeys=%d, messages=%d, signatures=%d",
			len(pubKeys), len(messages), len(signatures))
	}

	results := make([]bool, len(pubKeys))

	for batchStart := 0; batchStart < len(pubKeys); batchStart += BATCH_VERIFY_THRESHOLD {
		batchEnd := batchStart + BATCH_VERIFY_THRESHOLD
		if batchEnd > len(pubKeys) {
			batchEnd = len(pubKeys)
		}
		batchSize := batchEnd - batchStart

		if batchSize < BATCH_VERIFY_THRESHOLD {
			for i := batchStart; i < batchEnd; i++ {
				results[i] = Verify(pubKeys[i], messages[i], signatures[i])
			}
			continue
		}

		if batchSize > BATCH_VERIFY_MAX_SIZE {
			batchSize = BATCH_VERIFY_MAX_SIZE
			batchEnd = batchStart + BATCH_VERIFY_MAX_SIZE
		}

		batchResults := verifyBatchInternal(
			pubKeys[batchStart:batchEnd],
			messages[batchStart:batchEnd],
			signatures[batchStart:batchEnd],
		)

		for i := 0; i < len(batchResults); i++ {
			results[batchStart+i] = batchResults[i]
		}
	}

	return results, nil
}

func verifyBatchInternal(pubKeys []PublicKey, messages [][]byte, signatures [][]byte) []bool {
	batchSize := len(pubKeys)
	results := make([]bool, batchSize)

	if batchSize <= BATCH_VERIFY_THRESHOLD {
		for i := 0; i < batchSize; i++ {
			results[i] = Verify(pubKeys[i], messages[i], signatures[i])
		}
		return results
	}

	numWorkers := 4
	if batchSize < numWorkers {
		numWorkers = batchSize
	}

	chunkSize := (batchSize + numWorkers - 1) / numWorkers
	done := make(chan int, numWorkers)

	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > batchSize {
			end = batchSize
		}
		if start >= batchSize {
			done <- w
			continue
		}

		go func(workerID, s, e int) {
			for i := s; i < e && i < batchSize; i++ {
				results[i] = Verify(pubKeys[i], messages[i], signatures[i])
			}
			done <- workerID
		}(w, start, end)
	}

	for w := 0; w < numWorkers; w++ {
		<-done
	}

	return results
}

func validateSignatureCanonical(sig []byte) bool {
	if len(sig) != ed25519.SignatureSize {
		return false
	}

	S := sig[32:64]
	var S_int big.Int
	S_int.SetBytes(S)

	if S_int.Cmp(ed25519Order) >= 0 {
		return false
	}

	return true
}

func validatePublicKey(pubKey []byte) bool {
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}

	return true
}

var ed25519Order = new(big.Int).SetBytes([]byte{
	0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x14, 0xDE, 0xF9, 0xDE, 0xA2, 0xF7, 0x9C, 0xD6,
	0x58, 0x12, 0x63, 0x1A, 0x5C, 0xF5, 0xD3, 0xED,
})

// VerifyBatchSimple performs sequential per-item Ed25519 signature verification.
// Each signature is verified individually using crypto/ed25519.Verify.
func VerifyBatchSimple(pubKeys []PublicKey, messages [][]byte, signatures [][]byte) []bool {
	results := make([]bool, len(pubKeys))

	for i := 0; i < len(pubKeys); i++ {
		results[i] = Verify(pubKeys[i], messages[i], signatures[i])
	}

	return results
}
