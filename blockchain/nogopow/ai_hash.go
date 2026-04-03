// Copyright 2026 NogoChain Team
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

package nogopow

import (
	"encoding/binary"
	"unsafe"

	"golang.org/x/crypto/sha3"
)

const (
	matSize = 256
	matNum  = 256
)

var UseSIMD = false

func legacyAlgorithm(headerHash, seed [32]byte) [32]byte {
	cache := calcSeedCache(seed[:])
	result := mulMatrix(headerHash[:], cache)
	return hashMatrix(result)
}

// CalcSeedCache calculates cache data from seed
// Exported version for external use
func CalcSeedCache(seed []byte) []uint32 {
	return calcSeedCache(seed)
}

func calcSeedCache(seed []byte) []uint32 {
	extSeed := extendBytes(seed, 3)
	v := make([]uint32, 32*1024)

	if !isLittleEndian() {
		swap(extSeed)
	}

	cache := make([]uint32, 0, 128*32*1024)
	for i := 0; i < 128; i++ {
		Smix(extSeed, v)
		cache = append(cache, v...)
	}

	return cache
}

func extendBytes(seed []byte, round int) []byte {
	extSeed := make([]byte, len(seed)*(round+1))
	copy(extSeed, seed)

	for i := 0; i < round; i++ {
		var h [32]byte
		hasher := sha3.NewLegacyKeccak256()
		start := i * 32
		hasher.Write(extSeed[start : start+32])
		copy(h[:], hasher.Sum(nil))
		copy(extSeed[(i+1)*32:(i+2)*32], h[:])
	}

	return extSeed
}

func isLittleEndian() bool {
	n := uint32(0x01020304)
	return *(*byte)(unsafe.Pointer(&n)) == 0x04
}

func swap(buffer []byte) {
	for i := 0; i < len(buffer); i += 4 {
		binary.BigEndian.PutUint32(buffer[i:], binary.LittleEndian.Uint32(buffer[i:]))
	}
}

func safeBytesToUint32(b []byte, offset int) uint32 {
	if offset+4 > len(b) {
		return 0
	}

	ptr := uintptr(unsafe.Pointer(&b[offset]))
	if ptr%4 != 0 {
		return binary.LittleEndian.Uint32(b[offset : offset+4])
	}

	return *(*uint32)(unsafe.Pointer(&b[offset]))
}

func safeUint32ToBytes(val uint32, dst []byte, offset int) {
	if offset+4 > len(dst) {
		return
	}
	binary.LittleEndian.PutUint32(dst[offset:], val)
}

func Smix(b []byte, v []uint32) {
	const r = 1
	const N = 1024

	x := make([]uint32, 16*2*r)
	// Unmarshal b into x
	for i := 0; i < 16*2*r; i++ {
		x[i] = binary.LittleEndian.Uint32(b[i*4:])
	}

	// Initialize v and compute x
	for i := 0; i < N; i++ {
		copy(v[i*16*2*r:], x)
		x = blockMix(x, r)
	}

	// Compute final x
	for i := 0; i < N; i++ {
		j := int(x[16*(2*r-1)] % uint32(N))
		for k := 0; k < 16*2*r; k++ {
			x[k] ^= v[j*16*2*r+k]
		}
		x = blockMix(x, r)
	}

	// Marshal x back into b
	for i := 0; i < 16*2*r; i++ {
		binary.LittleEndian.PutUint32(b[i*4:], x[i])
	}
}

func blockMix(x []uint32, r int) []uint32 {
	const blockSize = 16

	y := make([]uint32, blockSize)
	copy(y, x[(2*r-1)*blockSize:])

	result := make([]uint32, 2*r*blockSize)
	for i := 0; i < 2*r; i++ {
		t := make([]uint32, blockSize)
		for j := 0; j < blockSize; j++ {
			t[j] = x[i*blockSize+j] ^ y[j]
		}

		y = salsa20_8(t)

		for j := 0; j < blockSize; j++ {
			result[(i%2)*r*blockSize+(i/2)*blockSize+j] = y[j]
		}
	}

	return result
}

func salsa20_8(x []uint32) []uint32 {
	y := make([]uint32, len(x))
	copy(y, x)

	for i := 0; i < 4; i++ {
		// Column round
		y[12] ^= rotl(y[8]+y[4], 7)
		y[0] ^= rotl(y[12]+y[8], 9)
		y[4] ^= rotl(y[0]+y[12], 13)
		y[8] ^= rotl(y[4]+y[0], 18)

		y[13] ^= rotl(y[9]+y[5], 7)
		y[1] ^= rotl(y[13]+y[9], 9)
		y[5] ^= rotl(y[1]+y[13], 13)
		y[9] ^= rotl(y[5]+y[1], 18)

		y[14] ^= rotl(y[10]+y[6], 7)
		y[2] ^= rotl(y[14]+y[10], 9)
		y[6] ^= rotl(y[2]+y[14], 13)
		y[10] ^= rotl(y[6]+y[2], 18)

		y[15] ^= rotl(y[11]+y[7], 7)
		y[3] ^= rotl(y[15]+y[11], 9)
		y[7] ^= rotl(y[3]+y[15], 13)
		y[11] ^= rotl(y[7]+y[3], 18)

		// Row round
		y[1] ^= rotl(y[0]+y[3], 7)
		y[2] ^= rotl(y[1]+y[0], 9)
		y[3] ^= rotl(y[2]+y[1], 13)
		y[0] ^= rotl(y[3]+y[2], 18)

		y[6] ^= rotl(y[5]+y[4], 7)
		y[7] ^= rotl(y[6]+y[5], 9)
		y[4] ^= rotl(y[7]+y[6], 13)
		y[5] ^= rotl(y[4]+y[7], 18)

		y[11] ^= rotl(y[10]+y[9], 7)
		y[8] ^= rotl(y[11]+y[10], 9)
		y[9] ^= rotl(y[8]+y[11], 13)
		y[10] ^= rotl(y[9]+y[8], 18)

		y[12] ^= rotl(y[15]+y[14], 7)
		y[13] ^= rotl(y[12]+y[15], 9)
		y[14] ^= rotl(y[13]+y[12], 13)
		y[15] ^= rotl(y[14]+y[13], 18)
	}

	for i := 0; i < len(x); i++ {
		x[i] += y[i]
	}

	return x
}

func rotl(a, b uint32) uint32 {
	return (a << b) | (a >> (32 - b))
}
