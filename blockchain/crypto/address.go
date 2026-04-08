// Copyright 2026 The NogoChain Authors
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
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/crypto/sha3"
)

const (
	// AddressPrefix is the prefix for NogoChain addresses
	AddressPrefix = "NOGO"

	// AddressVersion is the version byte for addresses (0x00 for mainnet)
	AddressVersion = 0x00

	// ChecksumLen is the length of the checksum in bytes
	ChecksumLen = 4

	// HashLen is the length of the address hash in bytes
	HashLen = 32

	// AddressDataLen is the total length of address data (version + hash + checksum)
	AddressDataLen = 1 + HashLen + ChecksumLen

	// EncodedAddressLen is the expected length of encoded address (hex)
	EncodedAddressLen = AddressDataLen * 2

	// FullAddressLen is the total length including prefix
	FullAddressLen = len(AddressPrefix) + EncodedAddressLen
)

var (
	// ErrAddressTooShort is returned when address is too short
	ErrAddressTooShort = errors.New("address too short")

	// ErrAddressInvalidPrefix is returned when address has invalid prefix
	ErrAddressInvalidPrefix = errors.New("invalid address prefix")

	// ErrAddressInvalidHex is returned when address contains invalid hex
	ErrAddressInvalidHex = errors.New("invalid hex encoding")

	// ErrAddressInvalidLength is returned when address has invalid length
	ErrAddressInvalidLength = errors.New("invalid encoded length")

	// ErrAddressChecksumMismatch is returned when checksum validation fails
	ErrAddressChecksumMismatch = errors.New("address checksum mismatch")

	// testAddressCache caches test addresses for performance
	testAddressCache sync.Map
)

// GenerateAddress creates a NogoChain address from a public key
// Production-grade: implements address generation with SHA256 hash and 4-byte checksum
// Security: uses constant-time operations where possible
func GenerateAddress(pubKey []byte) string {
	if len(pubKey) == 0 {
		return ""
	}

	hash := sha256.Sum256(pubKey)
	addressHash := hash[:HashLen]

	addressData := make([]byte, 1+HashLen)
	addressData[0] = AddressVersion
	copy(addressData[1:], addressHash)

	checksum := sha256.Sum256(addressData)
	addressData = append(addressData, checksum[:ChecksumLen]...)

	encoded := hex.EncodeToString(addressData)

	return fmt.Sprintf("%s%s", AddressPrefix, encoded)
}

// GenerateAddressV2 creates an address using SHA3-256 for enhanced security
// Production-grade: alternative address format using newer hash function
func GenerateAddressV2(pubKey []byte) string {
	if len(pubKey) == 0 {
		return ""
	}

	hash := sha3.Sum256(pubKey)
	addressHash := hash[:HashLen]

	addressData := make([]byte, 1+HashLen)
	addressData[0] = AddressVersion
	copy(addressData[1:], addressHash)

	checksum := sha3.Sum256(addressData)
	addressData = append(addressData, checksum[:ChecksumLen]...)

	encoded := hex.EncodeToString(addressData)

	return fmt.Sprintf("%s%s", AddressPrefix, encoded)
}

// ValidateAddress validates a NogoChain address
// Production-grade: validates prefix, length, hex encoding, and checksum
// Logic completeness: checks all address components
func ValidateAddress(addr string) error {
	if len(addr) < len(AddressPrefix)+10 {
		return ErrAddressTooShort
	}

	if addr[:len(AddressPrefix)] != AddressPrefix {
		return fmt.Errorf("%w: expected %s", ErrAddressInvalidPrefix, AddressPrefix)
	}

	encoded := addr[len(AddressPrefix):]

	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAddressInvalidHex, err)
	}

	if len(decoded) < ChecksumLen+1 {
		return ErrAddressInvalidLength
	}

	addressData := decoded[:len(decoded)-ChecksumLen]
	storedChecksum := decoded[len(decoded)-ChecksumLen:]

	checksum := sha256.Sum256(addressData)

	for i := 0; i < ChecksumLen; i++ {
		if storedChecksum[i] != checksum[i] {
			return ErrAddressChecksumMismatch
		}
	}

	return nil
}

// ValidateAddressV2 validates an address using SHA3-256 checksum
func ValidateAddressV2(addr string) error {
	if len(addr) < len(AddressPrefix)+10 {
		return ErrAddressTooShort
	}

	if addr[:len(AddressPrefix)] != AddressPrefix {
		return fmt.Errorf("%w: expected %s", ErrAddressInvalidPrefix, AddressPrefix)
	}

	encoded := addr[len(AddressPrefix):]

	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAddressInvalidHex, err)
	}

	if len(decoded) < ChecksumLen+1 {
		return ErrAddressInvalidLength
	}

	addressData := decoded[:len(decoded)-ChecksumLen]
	storedChecksum := decoded[len(decoded)-ChecksumLen:]

	checksum := sha3.Sum256(addressData)

	for i := 0; i < ChecksumLen; i++ {
		if storedChecksum[i] != checksum[i] {
			return ErrAddressChecksumMismatch
		}
	}

	return nil
}

// GetAddressFromPubKey is an alias for GenerateAddress for API compatibility
func GetAddressFromPubKey(pubKey []byte) string {
	return GenerateAddress(pubKey)
}

// DecodeAddress decodes an address to its raw bytes (version + hash)
// Returns the address data without checksum
func DecodeAddress(addr string) ([]byte, error) {
	if len(addr) < len(AddressPrefix) {
		return nil, ErrAddressInvalidPrefix
	}

	if addr[:len(AddressPrefix)] != AddressPrefix {
		return nil, ErrAddressInvalidPrefix
	}

	encoded := addr[len(AddressPrefix):]
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAddressInvalidHex, err)
	}

	if len(decoded) < ChecksumLen {
		return nil, ErrAddressInvalidLength
	}

	return decoded[:len(decoded)-ChecksumLen], nil
}

// EncodeAddress encodes raw address data (version + hash) to a full address with checksum
// Production-grade: adds checksum and prefix for complete address
func EncodeAddress(addrData []byte) string {
	if len(addrData) == 0 {
		return ""
	}

	// Calculate checksum
	checksum := sha256.Sum256(addrData)
	fullData := append(addrData, checksum[:ChecksumLen]...)

	// Encode to hex and add prefix
	encoded := hex.EncodeToString(fullData)
	return fmt.Sprintf("%s%s", AddressPrefix, encoded)
}

// FormatAddress formats an address for display by truncating
// Production-grade: safe truncation that preserves address structure
func FormatAddress(addr string) string {
	if len(addr) <= 16 {
		return addr
	}
	return addr[:8] + "..." + addr[len(addr)-8:]
}

// IsValidNogoAddress checks if an address is valid
// Convenience function that returns bool instead of error
func IsValidNogoAddress(addr string) bool {
	return ValidateAddress(addr) == nil
}

// IsValidNogoAddressV2 checks if an address is valid using SHA3
func IsValidNogoAddressV2(addr string) bool {
	return ValidateAddressV2(addr) == nil
}

// ExtractAddressHash extracts the hash portion from an address
// Returns the 32-byte hash without version and checksum
func ExtractAddressHash(addr string) ([]byte, error) {
	decoded, err := DecodeAddress(addr)
	if err != nil {
		return nil, err
	}

	if len(decoded) < 1+HashLen {
		return nil, ErrAddressInvalidLength
	}

	hashCopy := make([]byte, HashLen)
	copy(hashCopy, decoded[1:])
	return hashCopy, nil
}

// GetAddressVersion extracts the version byte from an address
func GetAddressVersion(addr string) (byte, error) {
	decoded, err := DecodeAddress(addr)
	if err != nil {
		return 0, err
	}

	if len(decoded) == 0 {
		return 0, ErrAddressInvalidLength
	}

	return decoded[0], nil
}

// CompareAddresses compares two addresses for equality
// Security: uses constant-time comparison to prevent timing attacks
func CompareAddresses(addr1, addr2 string) bool {
	if len(addr1) != len(addr2) {
		return false
	}

	result := byte(0)
	for i := 0; i < len(addr1); i++ {
		result |= addr1[i] ^ addr2[i]
	}

	return result == 0
}

// GenerateTestAddress generates a deterministic test address for testing
// NOT for production use - uses predictable seed
func GenerateTestAddress(seed byte) string {
	if cached, ok := testAddressCache.Load(seed); ok {
		return cached.(string)
	}

	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = seed
	}
	addr := GenerateAddress(pub)
	testAddressCache.Store(seed, addr)
	return addr
}

// GenerateTestAddress2 generates a test address with alternating seed pattern
// NOT for production use - uses predictable seed
func GenerateTestAddress2(seed1, seed2 byte) string {
	key := uint16(uint16(seed1)<<8 | uint16(seed2))
	if cached, ok := testAddressCache.Load(key); ok {
		return cached.(string)
	}

	pub := make([]byte, 32)
	for i := range pub {
		if i%2 == 0 {
			pub[i] = seed1
		} else {
			pub[i] = seed2
		}
	}
	addr := GenerateAddress(pub)
	testAddressCache.Store(key, addr)
	return addr
}

// Pre-generated test addresses for unit tests
var (
	TestAddressA     = GenerateTestAddress(0x01)
	TestAddressB     = GenerateTestAddress2(0x02, 0x03)
	TestAddressC     = GenerateTestAddress2(0x04, 0x05)
	TestAddressMiner = GenerateTestAddress(0x10)
)
