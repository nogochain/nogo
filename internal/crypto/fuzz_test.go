package crypto

import (
	"testing"
)

func FuzzHashFunctions(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = Hash256(data)
		_ = Hash160(data)
		_ = DoubleSHA256(data)

		if len(data) > 1000 {
			data = data[:1000]
		}
		chunks := make([][]byte, 0)
		for i := 0; i < len(data); i += 32 {
			end := i + 32
			if end > len(data) {
				end = len(data)
			}
			chunks = append(chunks, data[i:end])
		}
		if len(chunks) > 0 {
			_, _ = MerkleRoot(chunks)
		}
	})
}

func FuzzAddressValidation(f *testing.F) {
	f.Fuzz(func(t *testing.T, input string) {
		_ = ValidateAddress(input)
		_ = IsValidNogoAddress(input)
	})
}

func FuzzSignatureVerification(f *testing.F) {
	f.Fuzz(func(t *testing.T, pubKey, msg, sig []byte) {
		if len(pubKey) == 0 || len(msg) == 0 || len(sig) == 0 {
			return
		}
		if len(pubKey) > 64 {
			pubKey = pubKey[:64]
		}
		if len(sig) > 128 {
			sig = sig[:128]
		}
		if len(msg) > 10000 {
			msg = msg[:10000]
		}

		_ = Verify(pubKey, msg, sig)
	})
}
