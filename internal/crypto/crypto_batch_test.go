package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func generateTestBatch(size int) ([]PublicKey, [][]byte, [][]byte) {
	pubKeys := make([]PublicKey, size)
	messages := make([][]byte, size)
	signatures := make([][]byte, size)

	for i := 0; i < size; i++ {
		pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
		message := make([]byte, 64)
		rand.Read(message)
		signature := ed25519.Sign(privKey, message)

		pubKeys[i] = pubKey
		messages[i] = message
		signatures[i] = signature
	}

	return pubKeys, messages, signatures
}

func BenchmarkVerifyIndividual(b *testing.B) {
	pubKeys, messages, signatures := generateTestBatch(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(pubKeys); j++ {
			Verify(pubKeys[j], messages[j], signatures[j])
		}
	}
}

func BenchmarkVerifyBatch(b *testing.B) {
	pubKeys, messages, signatures := generateTestBatch(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyBatch(pubKeys, messages, signatures)
	}
}

func BenchmarkVerifyBatchSmall(b *testing.B) {
	pubKeys, messages, signatures := generateTestBatch(5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyBatch(pubKeys, messages, signatures)
	}
}

func BenchmarkVerifyBatchMedium(b *testing.B) {
	pubKeys, messages, signatures := generateTestBatch(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyBatch(pubKeys, messages, signatures)
	}
}

func BenchmarkVerifyBatchLarge(b *testing.B) {
	pubKeys, messages, signatures := generateTestBatch(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyBatch(pubKeys, messages, signatures)
	}
}

func BenchmarkVerifyBatchXLarge(b *testing.B) {
	pubKeys, messages, signatures := generateTestBatch(500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyBatch(pubKeys, messages, signatures)
	}
}

func BenchmarkVerifyBatchXXLarge(b *testing.B) {
	pubKeys, messages, signatures := generateTestBatch(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyBatch(pubKeys, messages, signatures)
	}
}

func TestVerifyBatchCorrectness(t *testing.T) {
	pubKeys, messages, signatures := generateTestBatch(100)

	batchResults, err := VerifyBatch(pubKeys, messages, signatures)
	if err != nil {
		t.Fatalf("Batch verification failed: %v", err)
	}

	for i := 0; i < len(pubKeys); i++ {
		individualResult := Verify(pubKeys[i], messages[i], signatures[i])
		if batchResults[i] != individualResult {
			t.Errorf("Mismatch at index %d: batch=%v, individual=%v", i, batchResults[i], individualResult)
		}
	}
}

func TestVerifyBatchDetectsInvalid(t *testing.T) {
	pubKeys, messages, signatures := generateTestBatch(20)

	randSig := make([]byte, ed25519.SignatureSize)
	rand.Read(randSig)
	signatures[10] = randSig

	batchResults, err := VerifyBatch(pubKeys, messages, signatures)
	if err != nil {
		t.Fatalf("Batch verification failed: %v", err)
	}

	if batchResults[10] {
		t.Error("Batch verification should have detected invalid signature at index 10")
	}

	for i := 0; i < len(batchResults); i++ {
		if i == 10 {
			continue
		}
		individualResult := Verify(pubKeys[i], messages[i], signatures[i])
		if batchResults[i] != individualResult {
			t.Errorf("Mismatch at index %d: batch=%v, individual=%v", i, batchResults[i], individualResult)
		}
	}
}

func TestVerifyBatchSizeMismatch(t *testing.T) {
	pubKeys := make([]PublicKey, 10)
	messages := make([][]byte, 9)
	signatures := make([][]byte, 10)

	_, err := VerifyBatch(pubKeys, messages, signatures)
	if err == nil {
		t.Error("Expected error for size mismatch")
	}
}

func TestVerifyBatchEmpty(t *testing.T) {
	pubKeys := []PublicKey{}
	messages := [][]byte{}
	signatures := [][]byte{}

	results, err := VerifyBatch(pubKeys, messages, signatures)
	if err != nil {
		t.Fatalf("Empty batch verification failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected empty results, got %d results", len(results))
	}
}

func TestVerifyBatchThreshold(t *testing.T) {
	pubKeys, messages, signatures := generateTestBatch(BATCH_VERIFY_THRESHOLD - 1)

	results, err := VerifyBatch(pubKeys, messages, signatures)
	if err != nil {
		t.Fatalf("Batch verification failed: %v", err)
	}

	for i := range results {
		if !results[i] {
			t.Errorf("Signature %d should be valid", i)
		}
	}
}

func TestVerifyBatchMaxSize(t *testing.T) {
	pubKeys, messages, signatures := generateTestBatch(BATCH_VERIFY_MAX_SIZE)

	results, err := VerifyBatch(pubKeys, messages, signatures)
	if err != nil {
		t.Fatalf("Batch verification failed: %v", err)
	}

	for i := range results {
		if !results[i] {
			t.Errorf("Signature %d should be valid", i)
		}
	}
}

func TestVerifyBatchConcurrency(t *testing.T) {
	pubKeys, messages, signatures := generateTestBatch(100)

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			results, err := VerifyBatch(pubKeys, messages, signatures)
			if err != nil {
				t.Errorf("Concurrent batch verification failed: %v", err)
				done <- false
				return
			}
			for _, valid := range results {
				if !valid {
					t.Error("Expected all signatures to be valid")
					done <- false
					return
				}
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		if !<-done {
			t.FailNow()
		}
	}
}

func BenchmarkVerifyBatchSimple(b *testing.B) {
	pubKeys, messages, signatures := generateTestBatch(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyBatchSimple(pubKeys, messages, signatures)
	}
}

func TestValidateSignatureCanonical(t *testing.T) {
	validSig := make([]byte, 64)
	validSig[63] = 0x0F

	if !validateSignatureCanonical(validSig) {
		t.Error("Valid signature marked as non-canonical")
	}

	invalidSig := make([]byte, 64)
	invalidSig[32] = 0xFF
	invalidSig[63] = 0xFF
	if validateSignatureCanonical(invalidSig) {
		t.Error("Invalid S value marked as canonical")
	}
}

func TestValidatePublicKey(t *testing.T) {
	pubKey, _, _ := ed25519.GenerateKey(rand.Reader)

	if !validatePublicKey(pubKey) {
		t.Error("Valid public key marked as invalid")
	}

	invalidKey := make([]byte, ed25519.PublicKeySize-1)
	if validatePublicKey(invalidKey) {
		t.Error("Invalid public key marked as valid")
	}
}
