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

import "sync"

type Metrics struct {
	cacheHits        uint64
	cacheMisses      uint64
	verificationTime []float64
	matrixOps        uint64
	difficultyAdj    float64
	blockTime        []float64
	powSuccess       uint64
	powFailure       uint64
	lock             sync.Mutex
}

func NewMetrics() *Metrics {
	return &Metrics{
		verificationTime: make([]float64, 0),
		blockTime:        make([]float64, 0),
	}
}

func (m *Metrics) IncCacheHits() {
	if m != nil {
		m.lock.Lock()
		m.cacheHits++
		m.lock.Unlock()
	}
}

func (m *Metrics) IncCacheMisses() {
	if m != nil {
		m.lock.Lock()
		m.cacheMisses++
		m.lock.Unlock()
	}
}

func (m *Metrics) ObserveVerificationTime(duration float64) {
	if m != nil {
		m.lock.Lock()
		m.verificationTime = append(m.verificationTime, duration)
		m.lock.Unlock()
	}
}

func (m *Metrics) IncMatrixOps() {
	if m != nil {
		m.lock.Lock()
		m.matrixOps++
		m.lock.Unlock()
	}
}

func (m *Metrics) SetDifficultyAdjustment(value float64) {
	if m != nil {
		m.lock.Lock()
		m.difficultyAdj = value
		m.lock.Unlock()
	}
}

func (m *Metrics) ObserveBlockTime(duration float64) {
	if m != nil {
		m.lock.Lock()
		m.blockTime = append(m.blockTime, duration)
		m.lock.Unlock()
	}
}

func (m *Metrics) IncPowSuccess() {
	if m != nil {
		m.lock.Lock()
		m.powSuccess++
		m.lock.Unlock()
	}
}

func (m *Metrics) IncPowFailure() {
	if m != nil {
		m.lock.Lock()
		m.powFailure++
		m.lock.Unlock()
	}
}

func (m *Metrics) GetCacheHits() uint64 {
	if m == nil {
		return 0
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.cacheHits
}

func (m *Metrics) GetCacheMisses() uint64 {
	if m == nil {
		return 0
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.cacheMisses
}

func (m *Metrics) GetMatrixOps() uint64 {
	if m == nil {
		return 0
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.matrixOps
}

func (m *Metrics) GetPowSuccess() uint64 {
	if m == nil {
		return 0
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.powSuccess
}

func (m *Metrics) GetPowFailure() uint64 {
	if m == nil {
		return 0
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.powFailure
}

var globalMetrics = NewMetrics()

func GetMetrics() *Metrics {
	return globalMetrics
}
