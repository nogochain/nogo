package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
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

var (
	batchVerifyPool = sync.Pool{
		New: func() interface{} {
			return &batchVerifyWorkspace{}
		},
	}
)

type batchVerifyWorkspace struct {
	scalars    []*big.Int
	points     [][]byte
	tempPoints [][]byte
}

func getBatchWorkspace() *batchVerifyWorkspace {
	ws := batchVerifyPool.Get().(*batchVerifyWorkspace)
	if ws.scalars == nil {
		ws.scalars = make([]*big.Int, 0, BATCH_VERIFY_MAX_SIZE*2)
		ws.points = make([][]byte, 0, BATCH_VERIFY_MAX_SIZE*2)
		ws.tempPoints = make([][]byte, 0, BATCH_VERIFY_MAX_SIZE*2)
	}
	return ws
}

func putBatchWorkspace(ws *batchVerifyWorkspace) {
	ws.scalars = ws.scalars[:0]
	ws.points = ws.points[:0]
	ws.tempPoints = ws.tempPoints[:0]
	batchVerifyPool.Put(ws)
}

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

func aggregatePoints(scalars []*big.Int, points [][]byte, randomScalars []*big.Int) []byte {
	if len(scalars) == 0 || len(scalars) != len(points) || len(scalars) != len(randomScalars) {
		return nil
	}

	resultX := new(big.Int)
	resultY := new(big.Int)
	resultX.SetInt64(0)
	resultY.SetInt64(1)

	p := new(big.Int).SetBytes([]byte{
		0x7F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xED,
	})

	d := new(big.Int).SetBytes([]byte{
		0x52, 0x03, 0x3C, 0xAE, 0x49, 0x8D, 0x88, 0x3D,
		0x68, 0x12, 0x28, 0xD6, 0x88, 0x2A, 0x92, 0x7D,
		0x77, 0x25, 0x35, 0x3E, 0xCA, 0x40, 0x20, 0x09,
		0x4C, 0x39, 0x3E, 0x26, 0x28, 0x1C, 0xA0, 0x98,
	})

	for i := 0; i < len(scalars); i++ {
		if scalars[i] == nil || points[i] == nil || randomScalars[i] == nil {
			continue
		}

		pointY := new(big.Int).SetBytes(points[i])
		pointX := recoverX(pointY, p)
		if pointX == nil {
			continue
		}

		k := new(big.Int).Mul(randomScalars[i], scalars[i])
		k.Mod(k, ed25519Order)

		scaledX, scaledY := scalarMultSimple(pointX, pointY, k, p)

		tmpX, tmpY := addPointsSimple(resultX, resultY, scaledX, scaledY, p, d)
		resultX.Set(tmpX)
		resultY.Set(tmpY)
	}

	compressed := compressPoint(resultX, resultY)
	return compressed
}

func recoverX(y, p *big.Int) *big.Int {
	ySquared := new(big.Int).Mul(y, y)
	ySquared.Mod(ySquared, p)

	u := new(big.Int).Sub(ySquared, big.NewInt(1))
	u.Mod(u, p)

	v := new(big.Int).Mul(y, y)
	v.Mul(v, new(big.Int).SetBytes([]byte{
		0x52, 0x03, 0x3C, 0xAE, 0x49, 0x8D, 0x88, 0x3D,
		0x68, 0x12, 0x28, 0xD6, 0x88, 0x2A, 0x92, 0x7D,
		0x77, 0x25, 0x35, 0x3E, 0xCA, 0x40, 0x20, 0x09,
		0x4C, 0x39, 0x3E, 0x26, 0x28, 0x1C, 0xA0, 0x98,
	}))
	v.Sub(v, big.NewInt(1))
	v.Mod(v, p)

	if v.Sign() == 0 {
		return big.NewInt(0)
	}

	vInv := new(big.Int).ModInverse(v, p)
	if vInv == nil {
		return nil
	}

	xSquared := new(big.Int).Mul(u, vInv)
	xSquared.Mod(xSquared, p)

	x := new(big.Int).ModSqrt(xSquared, p)
	if x == nil {
		return nil
	}

	return x
}

func scalarMultSimple(x, y, k, p *big.Int) (*big.Int, *big.Int) {
	resultX := new(big.Int).SetInt64(0)
	resultY := new(big.Int).SetInt64(1)

	baseX := new(big.Int).Set(x)
	baseY := new(big.Int).Set(y)

	scalar := new(big.Int).Set(k)

	for scalar.Sign() > 0 {
		if scalar.Bit(0) == 1 {
			tmpX, tmpY := addPointsSimple(resultX, resultY, baseX, baseY, p, new(big.Int).SetBytes([]byte{
				0x52, 0x03, 0x3C, 0xAE, 0x49, 0x8D, 0x88, 0x3D,
				0x68, 0x12, 0x28, 0xD6, 0x88, 0x2A, 0x92, 0x7D,
				0x77, 0x25, 0x35, 0x3E, 0xCA, 0x40, 0x20, 0x09,
				0x4C, 0x39, 0x3E, 0x26, 0x28, 0x1C, 0xA0, 0x98,
			}))
			resultX.Set(tmpX)
			resultY.Set(tmpY)
		}

		baseX, baseY = doublePoint(baseX, baseY, p, new(big.Int).SetBytes([]byte{
			0x52, 0x03, 0x3C, 0xAE, 0x49, 0x8D, 0x88, 0x3D,
			0x68, 0x12, 0x28, 0xD6, 0x88, 0x2A, 0x92, 0x7D,
			0x77, 0x25, 0x35, 0x3E, 0xCA, 0x40, 0x20, 0x09,
			0x4C, 0x39, 0x3E, 0x26, 0x28, 0x1C, 0xA0, 0x98,
		}))

		scalar.Rsh(scalar, 1)
	}

	return resultX, resultY
}

func addPointsSimple(x1, y1, x2, y2, p, d *big.Int) (*big.Int, *big.Int) {
	x1y2 := new(big.Int).Mul(x1, y2)
	x2y1 := new(big.Int).Mul(x2, y1)
	numX := new(big.Int).Add(x1y2, x2y1)
	numX.Mod(numX, p)

	y1y2 := new(big.Int).Mul(y1, y2)
	x1x2 := new(big.Int).Mul(x1, x2)
	numY := new(big.Int).Add(y1y2, x1x2)
	numY.Mod(numY, p)

	denom := new(big.Int).Mul(d, x1)
	denom.Mul(denom, x2)
	denom.Mul(denom, y1)
	denom.Mul(denom, y2)
	denom.Add(big.NewInt(1), denom)
	denom.Mod(denom, p)

	denomInv := new(big.Int).ModInverse(denom, p)
	if denomInv == nil {
		return big.NewInt(0), big.NewInt(1)
	}

	x := new(big.Int).Mul(numX, denomInv)
	x.Mod(x, p)

	y := new(big.Int).Mul(numY, denomInv)
	y.Mod(y, p)

	return x, y
}

func doublePoint(x, y, p, d *big.Int) (*big.Int, *big.Int) {
	return addPointsSimple(x, y, x, y, p, d)
}

func compressPoint(x, y *big.Int) []byte {
	result := make([]byte, 32)
	yBytes := y.Bytes()
	copy(result[32-len(yBytes):], yBytes)

	if x.Bit(0) == 1 {
		result[31] |= 0x80
	}

	return result
}

func generateRandomScalars(count int) []*big.Int {
	scalars := make([]*big.Int, count)
	randomBytes := make([]byte, count*64)

	_, err := rand.Read(randomBytes)
	if err != nil {
		for i := 0; i < count; i++ {
			scalars[i] = big.NewInt(int64(i + 1))
		}
		return scalars
	}

	for i := 0; i < count; i++ {
		scalars[i] = new(big.Int).SetBytes(randomBytes[i*64 : (i+1)*64])
	}

	return scalars
}

func VerifyBatchSimple(pubKeys []PublicKey, messages [][]byte, signatures [][]byte) []bool {
	results := make([]bool, len(pubKeys))

	for i := 0; i < len(pubKeys); i++ {
		results[i] = Verify(pubKeys[i], messages[i], signatures[i])
	}

	return results
}
