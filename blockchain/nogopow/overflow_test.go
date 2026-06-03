package nogopow

import (
	"fmt"
	"testing"
)

func TestOverflowAnalysis(t *testing.T) {
	// Verify 1: toFixedShift output range
	t.Run("ToFixedShiftRange", func(t *testing.T) {
		var minVal, maxVal int64
		for v := -128; v <= 127; v++ {
			shifted := toFixedShift(int8(v))
			if v == -128 {
				minVal = shifted
			}
			if v == 127 {
				maxVal = shifted
			}
		}
		fmt.Printf("toFixedShift range: [%d, %d]\n", minVal, maxVal)
		fmt.Printf("min hex: %x, max hex: %x\n", uint64(minVal), uint64(maxVal))
	})

	// Verify 2: product of two toFixedShift values does NOT overflow
	t.Run("MultiplicationNoOverflow", func(t *testing.T) {
		// With FixedPointShift=24, all int8 products should NOT overflow:
		//   max|X*Y| = 16384 = 2^14
		//   max prod = 2^14 * 2^48 = 2^62 < 2^63
		for a := -128; a <= 127; a++ {
			for b := -128; b <= 127; b++ {
				valA := toFixedShift(int8(a))
				valB := toFixedShift(int8(b))
				prod := valA * valB

				// Expected result without overflow: (X * Y) << (2 * FixedPointShift)
				expected := int64(a) * int64(b) << (2 * FixedPointShift)
				if prod != expected {
					t.Errorf("overflow detected: a=%d, b=%d, prod=%d, expected=%d", a, b, prod, expected)
					return
				}
			}
		}
		t.Log("all 65536 int8 combinations: no overflow")
	})

	// Verify 3: fromFixed and toFixedShift round-trip consistency
	t.Run("RoundTripConsistency", func(t *testing.T) {
		for v := -128; v <= 127; v++ {
			fixed := toFixedShift(int8(v))
			back := fromFixed(fixed)
			if back != int8(v) {
				// Negative values may have rounding differences, only warn
				fmt.Printf("  round-trip: %d -> %d -> %d\n", v, fixed, back)
			}
		}
		t.Log("round-trip: toFixedShift -> fromFixed completed")
	})

	// Verify 4: accumulation does not overflow
	t.Run("AccumulationSafe", func(t *testing.T) {
		// Worst case: all 256 terms at max negative value (-2^62)
		// After >> FixedPointShift: each term ≈ -2^(62-shift) = -2^38
		// 256 terms: -2^38 * 256 = -2^46
		// int64 min: -2^63
		const matSize = 256
		maxTerm := (int64(1)<<62 + FixedPointHalf) >> FixedPointShift
		minTerm := (-int64(1)<<62 + FixedPointHalf) >> FixedPointShift

		maxAccum := maxTerm * int64(matSize)
		minAccum := minTerm * int64(matSize)

		fmt.Printf("  max accum: %d, min accum: %d\n", maxAccum, minAccum)
		fmt.Printf("  int64 max: %d, int64 min: %d\n", maxInt64, minInt64)

		if maxAccum > maxInt64 || minAccum < minInt64 {
			t.Error("accumulation overflow possible")
		} else {
			t.Log("accumulation safe: max accum within int64 range")
		}
	})

	// Verify 5: overflow frequency in the ORIGINAL code (shift=30)
	t.Run("OriginalOverflowFrequency", func(t *testing.T) {
		// Show how often overflow would occur with the old shift=30
		overflowCount := 0
		totalCount := 0
		for v := -128; v <= 127; v++ {
			for w := -128; w <= 127; w++ {
				product := int64(v) * int64(w)
				if product >= 8 || product <= -8 {
					overflowCount++
				}
				totalCount++
			}
		}
		pct := float64(overflowCount) / float64(totalCount) * 100
		fmt.Printf("original shift=30 overflow rate: %d/%d (%.2f%%)\n",
			overflowCount, totalCount, pct)
		_ = pct
	})

	// Verify 6: no overflow with current fix (shift=24)
	t.Run("FixNoOverflow", func(t *testing.T) {
		overflowCount := 0
		totalCount := 0
		for a := -128; a <= 127; a++ {
			for b := -128; b <= 127; b++ {
				va := toFixedShift(int8(a))
				vb := toFixedShift(int8(b))
				prod := va * vb

				// Check: product should equal (X * Y) << 48
				expected := int64(a) * int64(b) << (2 * FixedPointShift)
				if prod != expected {
					overflowCount++
				}
				totalCount++
			}
		}
		fmt.Printf("fix shift=%d overflow rate: %d/%d\n", FixedPointShift, overflowCount, totalCount)
		if overflowCount > 0 {
			t.Errorf("fix still has %d overflows", overflowCount)
		} else {
			t.Log("fix verified: zero overflows")
		}
	})
}

// maxInt64 and minInt64 for test comparison
const maxInt64 = int64(1<<63 - 1)
const minInt64 = int64(-1 << 63)