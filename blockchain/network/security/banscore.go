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
	"time"
)

const (
	BanScoreHalflife = int64(60)
	BanScoreLifetime = int64(1800)
	BanThreshold     = uint32(100)
)

type DynamicBanScore struct {
	lastUnix   int64
	transient  float64
	persistent uint32
	mtx        sync.Mutex
}

func NewDynamicBanScore() *DynamicBanScore {
	return &DynamicBanScore{
		lastUnix: time.Now().Unix(),
	}
}

func (bs *DynamicBanScore) Increase(persistent, transient uint32) uint32 {
	bs.mtx.Lock()
	defer bs.mtx.Unlock()

	now := time.Now().Unix()
	dt := now - bs.lastUnix
	bs.lastUnix = now

	if bs.transient > 0 && dt > 0 {
		if dt >= BanScoreLifetime {
			bs.transient = 0
		} else {
			lambda := math.Ln2 / float64(BanScoreHalflife)
			decay := math.Exp(-lambda * float64(dt))
			bs.transient *= decay
		}
	}

	if transient > 0 {
		bs.transient += float64(transient)
	}

	if persistent > 0 {
		if bs.persistent > math.MaxUint32-persistent {
			bs.persistent = math.MaxUint32
		} else {
			bs.persistent += persistent
		}
	}

	return bs.calculateScore()
}

func (bs *DynamicBanScore) Score() uint32 {
	bs.mtx.Lock()
	defer bs.mtx.Unlock()
	return bs.calculateScore()
}

func (bs *DynamicBanScore) calculateScore() uint32 {
	now := time.Now().Unix()
	dt := now - bs.lastUnix

	var decayedTransient float64
	if bs.transient > 0 && dt > 0 {
		if dt >= BanScoreLifetime || bs.transient < 1 {
			decayedTransient = 0
		} else {
			lambda := math.Ln2 / float64(BanScoreHalflife)
			decay := math.Exp(-lambda * float64(dt))
			decayedTransient = bs.transient * decay
		}
	} else if dt <= 0 {
		decayedTransient = bs.transient
	} else {
		decayedTransient = 0
	}

	total := uint32(math.Round(decayedTransient)) + bs.persistent
	if total < bs.persistent {
		return math.MaxUint32
	}
	return total
}

func (bs *DynamicBanScore) IsBanned() bool {
	return bs.Score() >= BanThreshold
}

func (bs *DynamicBanScore) Reset() {
	bs.mtx.Lock()
	defer bs.mtx.Unlock()

	bs.lastUnix = time.Now().Unix()
	bs.transient = 0
	bs.persistent = 0
}
