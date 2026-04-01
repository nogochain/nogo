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

type API struct {
	engine *NogopowEngine
}

func (api *API) Hashrate() float64 {
	return float64(api.engine.HashRate())
}

func (api *API) Mining() bool {
	return api.engine.running
}

func (api *API) GetCacheStats() map[string]interface{} {
	if api.engine.cache != nil {
		return api.engine.cache.Stats()
	}
	return nil
}

func (api *API) GetDifficulty() uint64 {
	return GetMetrics().GetPowSuccess()
}

func (api *API) GetHashRate() uint64 {
	return GetMetrics().GetMatrixOps()
}
