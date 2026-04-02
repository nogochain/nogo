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
	"fmt"
	"math"
	"reflect"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/crypto/sha3"
)

const (
	FixedPointFactor = 1 << 30
	FixedPointHalf   = 1 << 29
	FixedPointShift  = 30
)

func toFixed(val float64) int64 {
	return int64(val * FixedPointFactor)
}

func fromFixed(val int64) int8 {
	rounded := (val + FixedPointHalf) >> FixedPointShift
	if rounded > math.MaxInt8 {
		return math.MaxInt8
	}
	if rounded < math.MinInt8 {
		return math.MinInt8
	}
	return int8(rounded)
}

func toFixedShift(v int8) int64 {
	return int64(v) << FixedPointShift
}

type denseMatrix struct {
	data []int64
	rows int
	cols int
}

func (m *denseMatrix) Reset(rows, cols int) {
	if rows > m.rows || cols > m.cols {
		m.data = make([]int64, rows*cols)
	}
	m.rows = rows
	m.cols = cols
}

var matrixPool = sync.Pool{
	New: func() interface{} {
		return &denseMatrix{
			data: make([]int64, matSize*matSize),
			rows: matSize,
			cols: matSize,
		}
	},
}

func GetMatrix(rows, cols int) *denseMatrix {
	m := matrixPool.Get().(*denseMatrix)
	m.Reset(rows, cols)
	return m
}

func PutMatrix(m *denseMatrix) {
	matrixPool.Put(m)
}

func newDenseMatrix(rows, cols int, data []int64) *denseMatrix {
	if data == nil {
		data = make([]int64, rows*cols)
		for i := 0; i < rows; i++ {
			data[i*cols+i] = FixedPointFactor
		}
	}
	return &denseMatrix{
		data: data,
		rows: rows,
		cols: cols,
	}
}

func (m *denseMatrix) At(row, col int) int64 {
	return m.data[row*m.cols+col]
}

func (m *denseMatrix) Set(row, col int, v int64) {
	m.data[row*m.cols+col] = v
}

func (m *denseMatrix) T() *denseMatrix {
	transposed := make([]int64, m.rows*m.cols)
	for i := 0; i < m.rows; i++ {
		for j := 0; j < m.cols; j++ {
			transposed[j*m.rows+i] = m.data[i*m.cols+j]
		}
	}
	return &denseMatrix{
		data: transposed,
		rows: m.cols,
		cols: m.rows,
	}
}

func (m *denseMatrix) Mul(a, b *denseMatrix) error {
	if a.cols != b.rows {
		return fmt.Errorf("matrix dimensions mismatch: %d vs %d", a.cols, b.rows)
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > m.rows {
		numWorkers = m.rows
	}

	rowsPerWorker := (m.rows + numWorkers - 1) / numWorkers
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()

			startRow := workerID * rowsPerWorker
			endRow := startRow + rowsPerWorker
			if endRow > m.rows {
				endRow = m.rows
			}

			for i := startRow; i < endRow; i++ {
				for j := 0; j < b.cols; j++ {
					var sum int64 = 0
					for k := 0; k < a.cols; k++ {
						prod := a.At(i, k) * b.At(k, j)
						sum += (prod + FixedPointHalf) >> FixedPointShift
					}
					m.Set(i, j, sum)
				}
			}
		}(w)
	}

	wg.Wait()
	return nil
}

func mulMatrixInto(dst, a, b *denseMatrix, size int) {
	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			var sum int64 = 0
			for k := 0; k < size; k++ {
				prod := a.At(i, k) * b.At(k, j)
				sum += (prod + FixedPointHalf) >> FixedPointShift
			}
			dst.Set(i, j, sum)
		}
	}
}

func mulMatrixBlocked(dst, a, b []int64, size int) {
	const blockSize = 32

	for i := 0; i < size*size; i++ {
		dst[i] = 0
	}

	for i0 := 0; i0 < size; i0 += blockSize {
		i1 := i0 + blockSize
		if i1 > size {
			i1 = size
		}

		for k0 := 0; k0 < size; k0 += blockSize {
			k1 := k0 + blockSize
			if k1 > size {
				k1 = size
			}

			for j0 := 0; j0 < size; j0 += blockSize {
				j1 := j0 + blockSize
				if j1 > size {
					j1 = size
				}

				for i := i0; i < i1; i++ {
					rowA := i * size
					rowDst := i * size

					for k := k0; k < k1; k++ {
						valA := a[rowA+k]

						if valA == 0 {
							continue
						}

						rowB := k * size

						for j := j0; j < j1; j++ {
							prod := valA * b[rowB+j]
							dst[rowDst+j] += (prod + FixedPointHalf) >> FixedPointShift
						}
					}
				}
			}
		}
	}
}

func mulMatrixWithPool(headerHash []byte, cache []uint32, matA, matB, matRes *denseMatrix) []uint8 {
	ui32data := make([]uint32, matNum*matSize*matSize/4)

	for i := 0; i < 128; i++ {
		start := i * 1024 * 32
		for j := 0; j < 512; j++ {
			copy(ui32data[start+j*32:start+j*32+32], cache[start+j*64:start+j*64+32])
			copy(ui32data[start+512*32+j*32:start+512*32+j*32+32], cache[start+j*64+32:start+j*64+64])
		}
	}

	header := *(*reflect.SliceHeader)(unsafe.Pointer(&ui32data))
	header.Len *= 4
	header.Cap *= 4
	i8data := *(*[]int8)(unsafe.Pointer(&header))

	fixedData := make([]int64, matNum*matSize*matSize)
	for i := 0; i < matNum*matSize*matSize; i++ {
		fixedData[i] = toFixed(float64(i8data[i]))
	}

	dataIdentity := make([]int64, matSize*matSize)
	for i := 0; i < matSize; i++ {
		dataIdentity[i*257] = FixedPointFactor
	}

	var tmp [matSize][matSize]int64
	var maArr [4][matSize][matSize]int64

	runtime.GOMAXPROCS(4)
	var wg sync.WaitGroup
	wg.Add(4)

	for k := 0; k < 4; k++ {
		go func(i int) {
			defer wg.Done()

			localMatA := GetMatrix(matSize, matSize)
			localMatB := GetMatrix(matSize, matSize)
			defer PutMatrix(localMatA)
			defer PutMatrix(localMatB)

			copy(localMatA.data, dataIdentity)

			var sequence [32]byte
			hasher := sha3.NewLegacyKeccak256()
			hasher.Write(headerHash[i*8 : (i+1)*8])
			copy(sequence[:], hasher.Sum(nil))

			for j := 0; j < 2; j++ {
				for k := 0; k < 32; k++ {
					index := int(sequence[k])
					mb := newDenseMatrix(matSize, matSize, fixedData[index*matSize*matSize:(index+1)*matSize*matSize])

					mulMatrixBlocked(localMatB.data, localMatA.data, mb.data, matSize)

					for row := 0; row < matSize; row++ {
						for col := 0; col < matSize; col++ {
							i8v := fromFixed(localMatB.At(row, col))
							localMatB.Set(row, col, toFixedShift(i8v))
						}
					}
					localMatA, localMatB = localMatB, localMatA
				}
			}

			for row := 0; row < matSize; row++ {
				for col := 0; col < matSize; col++ {
					maArr[i][row][col] = localMatA.At(row, col)
				}
			}
		}(k)
	}
	wg.Wait()

	for i := 0; i < 4; i++ {
		for row := 0; row < matSize; row++ {
			for col := 0; col < matSize; col++ {
				tmp[row][col] += maArr[i][row][col]
			}
		}
	}

	result := make([]uint8, 0, matSize*matSize)
	for i := 0; i < matSize; i++ {
		for j := 0; j < matSize; j++ {
			result = append(result, uint8(fromFixed(tmp[i][j])))
		}
	}
	return result
}

func mulMatrix(headerHash []byte, cache []uint32) []uint8 {
	ui32data := make([]uint32, matNum*matSize*matSize/4)

	for i := 0; i < 128; i++ {
		start := i * 1024 * 32
		for j := 0; j < 512; j++ {
			copy(ui32data[start+j*32:start+j*32+32], cache[start+j*64:start+j*64+32])
			copy(ui32data[start+512*32+j*32:start+512*32+j*32+32], cache[start+j*64+32:start+j*64+64])
		}
	}

	header := *(*reflect.SliceHeader)(unsafe.Pointer(&ui32data))
	header.Len *= 4
	header.Cap *= 4
	i8data := *(*[]int8)(unsafe.Pointer(&header))

	fixedData := make([]int64, matNum*matSize*matSize)
	for i := 0; i < matNum*matSize*matSize; i++ {
		fixedData[i] = toFixed(float64(i8data[i]))
	}

	dataIdentity := make([]int64, matSize*matSize)
	for i := 0; i < matSize; i++ {
		dataIdentity[i*257] = FixedPointFactor
	}

	var tmp [matSize][matSize]int64
	var maArr [4][matSize][matSize]int64

	runtime.GOMAXPROCS(4)
	var wg sync.WaitGroup
	wg.Add(4)

	for k := 0; k < 4; k++ {
		go func(i int) {
			defer wg.Done()

			ma := GetMatrix(matSize, matSize)
			mc := GetMatrix(matSize, matSize)

			defer PutMatrix(ma)
			defer PutMatrix(mc)

			copy(ma.data, dataIdentity)

			var sequence [32]byte
			hasher := sha3.NewLegacyKeccak256()
			hasher.Write(headerHash[i*8 : (i+1)*8])
			copy(sequence[:], hasher.Sum(nil))

			for j := 0; j < 2; j++ {
				for k := 0; k < 32; k++ {
					index := int(sequence[k])
					mb := newDenseMatrix(matSize, matSize, fixedData[index*matSize*matSize:(index+1)*matSize*matSize])

					mulMatrixBlocked(mc.data, ma.data, mb.data, matSize)

					for row := 0; row < matSize; row++ {
						for col := 0; col < matSize; col++ {
							i8v := fromFixed(mc.At(row, col))
							mc.Set(row, col, toFixedShift(i8v))
						}
					}
					ma, mc = mc, ma
				}
			}

			for row := 0; row < matSize; row++ {
				for col := 0; col < matSize; col++ {
					maArr[i][row][col] = ma.At(row, col)
				}
			}
		}(k)
	}
	wg.Wait()

	for i := 0; i < 4; i++ {
		for row := 0; row < matSize; row++ {
			for col := 0; col < matSize; col++ {
				tmp[row][col] += maArr[i][row][col]
			}
		}
	}

	result := make([]uint8, 0, matSize*matSize)
	for i := 0; i < matSize; i++ {
		for j := 0; j < matSize; j++ {
			result = append(result, uint8(fromFixed(tmp[i][j])))
		}
	}
	return result
}

func hashMatrix(result []uint8) [32]byte {
	var mat8 [matSize][matSize]uint8
	for i := 0; i < matSize; i++ {
		for j := 0; j < matSize; j++ {
			mat8[i][j] = result[i*matSize+j]
		}
	}

	var mat32 [matSize][matSize / 4]uint32

	for i := 0; i < matSize; i++ {
		for j := 0; j < matSize/4; j++ {
			mat32[i][j] = (uint32(mat8[i][j+192]) << 24) |
				(uint32(mat8[i][j+128]) << 16) |
				(uint32(mat8[i][j+64]) << 8) |
				(uint32(mat8[i][j]) << 0)
		}
	}

	for k := matSize; k > 1; k = k / 2 {
		for j := 0; j < k/2; j++ {
			for i := 0; i < matSize/4; i++ {
				mat32[j][i] = fnv(mat32[j][i], mat32[j+k/2][i])
			}
		}
	}

	ui32data := make([]uint32, 0, matSize/4)
	for i := 0; i < matSize/4; i++ {
		ui32data = append(ui32data, mat32[0][i])
	}

	header := *(*reflect.SliceHeader)(unsafe.Pointer(&ui32data))
	header.Len *= 4
	header.Cap *= 4
	dataBytes := *(*[]byte)(unsafe.Pointer(&header))

	var h [32]byte
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(dataBytes)
	copy(h[:], hasher.Sum(nil))

	return h
}

func fnv(a, b uint32) uint32 {
	return a*0x01000193 ^ b
}
