package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	AddressPrefix  = "NOGO"
	AddressVersion = 0x00
	ChecksumLen    = 4
	HashLen        = 32
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
