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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package network

import (
	"github.com/nogochain/nogo/blockchain/core"
)

// getResolutionPriority determines resolution priority based on fork event
// This is shared between SyncLoop and P2PServer fork resolution paths
func getResolutionPriority(event *core.ForkEvent) ResolutionPriority {
	switch event.Type {
	case core.ForkTypeDeep:
		return ResolutionPriorityCritical
	case core.ForkTypePersistent:
		return ResolutionPriorityHigh
	case core.ForkTypeTemporary:
		return ResolutionPriorityNormal
	default:
		return ResolutionPriorityLow
	}
}
