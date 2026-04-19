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

package reactor

import (
	"github.com/nogochain/nogo/blockchain/network/mconnection"
)

// Reactor defines the interface for P2P protocol modules that handle
// message routing and peer lifecycle events over multiplexed channels.
//
// Reactors are the core building blocks of the NogoChain P2P layer,
// following the Tendermint-style reactor pattern for modular protocol handling.
// Each reactor manages a specific protocol domain (sync, tx, block, etc.)
// and receives messages from its assigned channels.
type Reactor interface {
	// SetSwitch assigns the SwitchInterface to the reactor.
	// Called once during reactor registration before any other methods.
	SetSwitch(sw SwitchInterface)

	// GetChannels returns the list of channel descriptors this reactor
	// wants to receive messages from. Each channel has a unique ID,
	// priority, and capacity configuration.
	GetChannels() []*mconnection.ChannelDescriptor

	// AddPeer is called when a new peer is connected and ready for
	// communication. The nodeInfo map contains peer metadata such as
	// version, chain ID, and capabilities.
	// Returns an error if the peer should be rejected.
	AddPeer(peerID string, nodeInfo map[string]string) error

	// RemovePeer is called when a peer disconnects or is evicted.
	// The reason parameter provides context for the disconnection
	// (e.g., error message, ban reason, or normal shutdown).
	RemovePeer(peerID string, reason interface{})

	// Receive is called when a message arrives on one of the reactor's
	// channels. The chID identifies which channel the message came from,
	// peerID identifies the sender, and msgBytes contains the raw message
	// payload that must be parsed and dispatched.
	Receive(chID byte, peerID string, msgBytes []byte)
}

// SwitchInterface defines the interface for the P2P switch that manages
// reactor registration and message broadcasting across peer connections.
//
// The Switch acts as the central message router, dispatching incoming
// messages to the appropriate reactor based on channel ID, and providing
// reactors with methods to send messages to peers.
type SwitchInterface interface {
	// AddReactor registers a reactor with the switch under the given name.
	// Returns the registered reactor for chaining.
	AddReactor(name string, reactor Reactor) Reactor

	// Broadcast sends a message to all connected peers on the specified channel.
	// Used for gossip-style message propagation (e.g., new transactions, blocks).
	Broadcast(chID byte, msg []byte)

	// Send delivers a message to a specific peer on the specified channel.
	// Returns false if the peer is not connected or the send fails.
	Send(peerID string, chID byte, msg []byte) bool
}
