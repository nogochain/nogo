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

package security

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestNewDynamicBanScore(t *testing.T) {
	bs := NewDynamicBanScore()
	if bs == nil {
		t.Fatal("NewDynamicBanScore returned nil")
	}
	if bs.persistent != 0 {
		t.Errorf("expected persistent=0, got %d", bs.persistent)
	}
	if bs.transient != 0 {
		t.Errorf("expected transient=0, got %f", bs.transient)
	}
	if bs.lastUnix == 0 {
		t.Error("lastUnix should be initialized to current time")
	}
}

func TestIncreaseBasic(t *testing.T) {
	bs := NewDynamicBanScore()

	score := bs.Increase(10, 20)
	if score < 30 {
		t.Errorf("expected score >= 30, got %d", score)
	}

	bs.mtx.Lock()
	persist := bs.persistent
	trans := bs.transient
	bs.mtx.Unlock()

	if persist != 10 {
		t.Errorf("expected persistent=10, got %d", persist)
	}
	if trans < 19 || trans > 21 {
		t.Errorf("expected transient ~20, got %f", trans)
	}
}

func TestIncreasePersistentOnly(t *testing.T) {
	bs := NewDynamicBanScore()

	score := bs.Increase(50, 0)
	if score != 50 {
		t.Errorf("expected score=50, got %d", score)
	}

	score = bs.Increase(25, 0)
	if score != 75 {
		t.Errorf("expected score=75, got %d", score)
	}
}

func TestIncreaseTransientOnly(t *testing.T) {
	bs := NewDynamicBanScore()

	score := bs.Increase(0, 100)
	if score < 99 || score > 101 {
		t.Errorf("expected score ~100, got %d", score)
	}
}

func TestExponentialDecayMath(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(0, 100)

	bs.mtx.Lock()
	bs.lastUnix = time.Now().Unix() - int64(BanScoreHalflife)
	bs.mtx.Unlock()

	score := bs.Score()
	lambda := math.Ln2 / float64(BanScoreHalflife)
	expectedDecayed := 100.0 * math.Exp(-lambda*float64(BanScoreHalflife))
	expectedScore := uint32(math.Round(expectedDecayed))

	if math.Abs(float64(score)-float64(expectedScore)) > 2 {
		t.Errorf("after one halflife, expected score ~%d (theoretical %.1f), got %d",
			expectedScore, expectedDecayed, score)
	}
}

func TestExponentialDecayMultipleHalflives(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(0, 100)

	bs.mtx.Lock()
	bs.lastUnix = time.Now().Unix() - int64(2*BanScoreHalflife)
	bs.mtx.Unlock()

	score := bs.Score()
	lambda := math.Ln2 / float64(BanScoreHalflife)
	dt := float64(2 * BanScoreHalflife)
	expectedDecayed := 100.0 * math.Exp(-lambda*dt)
	expectedScore := uint32(math.Round(expectedDecayed))

	if math.Abs(float64(score)-float64(expectedScore)) > 1 {
		t.Errorf("after 2 halflives, expected score ~%d (theoretical %.1f), got %d",
			expectedScore, expectedDecayed, score)
	}

	if score >= 30 {
		t.Errorf("after 2 halflives, score should be < 30%% of original, got %d", score)
	}
}

func TestLifetimeExpiration(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(0, 200)

	bs.mtx.Lock()
	bs.lastUnix = time.Now().Unix() - BanScoreLifetime - 1
	bs.mtx.Unlock()

	score := bs.Score()
	if score != 0 {
		t.Errorf("after lifetime expiration, expected score=0, got %d", score)
	}
}

func TestLifetimeBoundaryExact(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(0, 100)

	bs.mtx.Lock()
	bs.lastUnix = time.Now().Unix() - int64(BanScoreLifetime)
	bs.mtx.Unlock()

	score := bs.Score()
	if score != 0 {
		t.Errorf("at exactly lifetime boundary, expected score=0, got %d", score)
	}
}

func TestTransientLessThanOne(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(0, 1)

	bs.mtx.Lock()
	bs.lastUnix = time.Now().Unix() - int64(2*BanScoreHalflife)
	bs.mtx.Unlock()

	score := bs.Score()
	lambda := math.Ln2 / float64(BanScoreHalflife)
	dt := float64(2 * BanScoreHalflife)
	decayed := 1.0 * math.Exp(-lambda*dt)

	if decayed < 1 && score != 0 {
		t.Errorf("when decayed transient < 1, expected score=0, got %d", score)
	}
}

func TestNegativeDeltaTime(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.mtx.Lock()
	bs.lastUnix = time.Now().Unix() + 100
	bs.mtx.Unlock()

	score := bs.Increase(10, 20)
	if score < 29 || score > 31 {
		t.Errorf("with negative dt, expected no decay, score ~30, got %d", score)
	}
}

func TestZeroDeltaTime(t *testing.T) {
	bs := NewDynamicBanScore()

	for i := 0; i < 100; i++ {
		bs.Increase(1, 1)
	}

	score := bs.Score()
	if score < 190 || score > 210 {
		t.Errorf("with rapid increases (dt~0), expected score ~200, got %d", score)
	}
}

func TestPersistentOverflowProtection(t *testing.T) {
	bs := NewDynamicBanScore()

	maxUint32 := uint32(math.MaxUint32)
	halfMax := maxUint32/2 + 1

	bs.Increase(halfMax, 0)
	bs.Increase(halfMax, 0)

	bs.mtx.Lock()
	persist := bs.persistent
	bs.mtx.Unlock()

	if persist != math.MaxUint32 {
		t.Errorf("expected overflow protection to cap at MaxUint32, got %d", persist)
	}
}

func TestPersistentOverflowExact(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(math.MaxUint32-5, 0)
	bs.Increase(10, 0)

	bs.mtx.Lock()
	persist := bs.persistent
	bs.mtx.Unlock()

	if persist != math.MaxUint32 {
		t.Errorf("expected exact overflow cap at MaxUint32, got %d", persist)
	}
}

func TestScoreReadOnly(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(10, 20)

	score1 := bs.Score()
	score2 := bs.Score()

	if score1 != score2 {
		t.Errorf("Score() should not modify state: first=%d, second=%d", score1, score2)
	}
}

func TestIsBannedBelowThreshold(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(0, BanThreshold-1)

	if bs.IsBanned() {
		t.Error("should not be banned below threshold")
	}
}

func TestIsBannedAtThreshold(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(0, BanThreshold)

	if !bs.IsBanned() {
		t.Error("should be banned at threshold")
	}
}

func TestIsBannedAboveThreshold(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(0, BanThreshold+50)

	if !bs.IsBanned() {
		t.Error("should be banned above threshold")
	}
}

func TestIsBannedWithDecay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping banned decay test in short mode")
	}

	bs := NewDynamicBanScore()

	bs.Increase(0, BanThreshold*3)

	if !bs.IsBanned() {
		t.Error("should be banned immediately after increase")
	}

	bs.mtx.Lock()
	bs.lastUnix = time.Now().Unix() - int64(BanScoreHalflife)
	bs.mtx.Unlock()

	if !bs.IsBanned() {
		t.Error("should still be banned after one halflife (score still > threshold)")
	}
}

func TestIsBannedAfterFullDecay(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(0, BanThreshold+50)

	bs.mtx.Lock()
	bs.lastUnix = time.Now().Unix() - int64(BanScoreLifetime+1)
	bs.mtx.Unlock()

	if bs.IsBanned() {
		t.Error("should not be banned after full decay")
	}
}

func TestReset(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(100, 200)
	bs.Reset()

	bs.mtx.Lock()
	persist := bs.persistent
	trans := bs.transient
	bs.mtx.Unlock()

	if persist != 0 {
		t.Errorf("after Reset, expected persistent=0, got %d", persist)
	}
	if trans != 0 {
		t.Errorf("after Reset, expected transient=0, got %f", trans)
	}

	if bs.Score() != 0 {
		t.Errorf("after Reset, expected Score()=0, got %d", bs.Score())
	}

	if bs.IsBanned() {
		t.Error("after Reset, should not be banned")
	}
}

func TestResetThenIncrease(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(100, 200)
	bs.Reset()

	score := bs.Increase(10, 20)
	if score < 29 || score > 31 {
		t.Errorf("after Reset and new increase, expected score ~30, got %d", score)
	}
}

func TestConcurrentIncrease(t *testing.T) {
	bs := NewDynamicBanScore()

	var wg sync.WaitGroup
	const goroutines = 100
	const increasesPerGoroutine = 100

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < increasesPerGoroutine; j++ {
				bs.Increase(1, 1)
			}
		}()
	}
	wg.Wait()

	finalScore := bs.Score()
	expectedMin := uint32(goroutines * increasesPerGoroutine * 2)
	if finalScore < expectedMin/2 {
		t.Errorf("concurrent increases: expected at least %d, got %d", expectedMin/2, finalScore)
	}
}

func TestConcurrentScoreReads(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.Increase(50, 50)

	var wg sync.WaitGroup
	const readers = 50

	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				score := bs.Score()
				if score > 200 {
					t.Errorf("unexpected high score: %d", score)
				}
				_ = bs.IsBanned()
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentMixedOperations(t *testing.T) {
	bs := NewDynamicBanScore()

	var wg sync.WaitGroup
	const workers = 20

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				switch id % 4 {
				case 0:
					bs.Increase(1, 1)
				case 1:
					_ = bs.Score()
				case 2:
					_ = bs.IsBanned()
				case 3:
					if j == 25 {
						bs.Reset()
					}
				}
			}
		}(i)
	}
	wg.Wait()

	finalScore := bs.Score()
	if finalScore > 5000 {
		t.Errorf("mixed operations: unexpected high score %d", finalScore)
	}
}

func TestCombinedPersistentAndTransient(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping combined decay test in short mode")
	}

	bs := NewDynamicBanScore()

	bs.Increase(30, 70)

	score := bs.Score()
	if score < 99 || score > 101 {
		t.Errorf("combined scores: expected ~100, got %d", score)
	}

	bs.mtx.Lock()
	bs.lastUnix = time.Now().Unix() - int64(BanScoreHalflife)
	bs.mtx.Unlock()

	scoreAfterDecay := bs.Score()
	persistentPart := uint32(30)
	if scoreAfterDecay < persistentPart || scoreAfterDecay > persistentPart+40 {
		t.Errorf("after decay: persistent part should remain, expected ~30-70, got %d", scoreAfterDecay)
	}
}

func TestMultipleIncreasesWithDecay(t *testing.T) {
	bs := NewDynamicBanScore()

	totalPersistent := uint32(0)
	totalTransient := 0.0

	for i := 0; i < 10; i++ {
		p := uint32((i + 1) * 5)
		tr := uint32((i + 1) * 10)
		bs.Increase(p, tr)
		totalPersistent += p
		totalTransient += float64(tr)
		time.Sleep(100 * time.Millisecond)
	}

	score := bs.Score()
	minExpected := totalPersistent
	if score < minExpected {
		t.Errorf("multiple increases: expected at least %d (persistent), got %d", minExpected, score)
	}
}

func TestConstantDefinitions(t *testing.T) {
	if BanScoreHalflife != 60 {
		t.Errorf("expected BanScoreHalflife=60, got %d", BanScoreHalflife)
	}
	if BanScoreLifetime != 1800 {
		t.Errorf("expected BanScoreLifetime=1800, got %d", BanScoreLifetime)
	}
	if BanThreshold != 100 {
		t.Errorf("expected BanThreshold=100, got %d", BanThreshold)
	}

	halflifesInLifetime := BanScoreLifetime / BanScoreHalflife
	if halflifesInLifetime != 30 {
		t.Errorf("expected 30 halflifes in lifetime, got %d", halflifesInLifetime)
	}
}

func TestDecayFactorCalculation(t *testing.T) {
	lambda := math.Ln2 / float64(BanScoreHalflife)
	decayAfterOneHalflife := math.Exp(-lambda * float64(BanScoreHalflife))
	expectedRatio := 0.5

	if math.Abs(decayAfterOneHalflife-expectedRatio) > 0.001 {
		t.Errorf("decay factor after one halflife: expected %.3f, got %.3f",
			expectedRatio, decayAfterOneHalflife)
	}

	decayAfterTwoHalflifes := math.Exp(-lambda * float64(2*BanScoreHalflife))
	expectedRatioTwo := 0.25

	if math.Abs(decayAfterTwoHalflifes-expectedRatioTwo) > 0.001 {
		t.Errorf("decay factor after two halflifes: expected %.3f, got %.3f",
			expectedRatioTwo, decayAfterTwoHalflifes)
	}
}

func TestBanThresholdConstants(t *testing.T) {
	if BanThreshold == 0 {
		t.Fatal("BanThreshold must be > 0")
	}

	if BanThreshold > 10000 {
		t.Errorf("BanThreshold seems too high: %d", BanThreshold)
	}
}

func TestMutexProtection(t *testing.T) {
	bs := NewDynamicBanScore()

	bs.mtx.Lock()

	done := make(chan bool, 1)
	go func() {
		bs.Increase(1000000, 1000000)
		done <- true
	}()

	select {
	case <-done:
		t.Error("Increase should block when mutex is held")
	case <-time.After(100 * time.Millisecond):
		bs.mtx.Unlock()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("deadlock detected after mutex release")
		}
	}
}

func TestRapidFireIncreases(t *testing.T) {
	bs := NewDynamicBanScore()

	startTime := time.Now()
	const iterations = 10000

	for i := 0; i < iterations; i++ {
		bs.Increase(1, 1)
	}
	elapsed := time.Since(startTime)

	if elapsed > 5*time.Second {
		t.Errorf("rapid fire increases took too long: %v", elapsed)
	}

	score := bs.Score()
	if score < uint32(iterations)*2/2 {
		t.Errorf("rapid fire: expected reasonable score, got %d", score)
	}
}

func TestEdgeCaseZeroValues(t *testing.T) {
	bs := NewDynamicBanScore()

	score := bs.Increase(0, 0)
	if score != 0 {
		t.Errorf("zero increase should yield zero score, got %d", score)
	}

	if bs.IsBanned() {
		t.Error("zero score should not be banned")
	}
}

func TestLargeTransientValue(t *testing.T) {
	bs := NewDynamicBanScore()

	largeTransient := uint32(1000000)
	bs.Increase(0, largeTransient)

	score := bs.Score()
	if score < largeTransient-1 || score > largeTransient+1 {
		t.Errorf("large transient: expected ~%d, got %d", largeTransient, score)
	}
}

func TestScoreAccuracyOverTime(t *testing.T) {
	bs := NewDynamicBanScore()

	initialScore := bs.Increase(0, 1000)
	scores := []uint32{initialScore}

	for i := 1; i <= 2; i++ {
		bs.mtx.Lock()
		bs.lastUnix = time.Now().Unix() - int64(uint64(i)*uint64(BanScoreHalflife))
		bs.mtx.Unlock()

		score := bs.Score()
		scores = append(scores, score)

		if score >= scores[i-1] {
			t.Errorf("score should decrease over time: step %d: prev=%d, curr=%d",
				i, scores[i-1], score)
		}
	}

	ratio := float64(scores[len(scores)-1]) / float64(scores[0])
	expectedRatio := math.Pow(0.5, 2)

	if math.Abs(ratio-expectedRatio) > 0.05 {
		t.Errorf("over 2 halflives: expected ratio %.3f, got %.3f",
			expectedRatio, ratio)
	}
}

func BenchmarkNewDynamicBanScore(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewDynamicBanScore()
	}
}

func BenchmarkIncrease(b *testing.B) {
	bs := NewDynamicBanScore()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bs.Increase(1, 1)
	}
}

func BenchmarkScore(b *testing.B) {
	bs := NewDynamicBanScore()
	bs.Increase(50, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bs.Score()
	}
}

func BenchmarkIsBanned(b *testing.B) {
	bs := NewDynamicBanScore()
	bs.Increase(150, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bs.IsBanned()
	}
}

func BenchmarkConcurrentIncrease(b *testing.B) {
	bs := NewDynamicBanScore()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			bs.Increase(1, 1)
			i++
		}
	})
}
