package mconnection

// Default channel IDs for NogoChain P2P protocol.
// Each channel serves a specific purpose with different priority levels.
const (
	// ChannelSync is used for blockchain synchronization messages (block headers, block bodies).
	// Priority 5 - High priority for keeping nodes in sync.
	ChannelSync byte = 0x01

	// ChannelTx is used for transaction propagation.
	// Priority 3 - Medium priority, transactions are important but not as critical as consensus.
	ChannelTx byte = 0x02

	// ChannelBlock is used for new block announcements and full block delivery.
	// Priority 5 - High priority to ensure rapid block propagation.
	ChannelBlock byte = 0x03

	// ChannelConsensus is used for consensus messages (votes, proposals, round changes).
	// Priority 7 - Highest priority to ensure consensus progresses without delay.
	ChannelConsensus byte = 0x04

	// ChannelGossip is used for low-priority gossip messages (peer discovery, status updates).
	// Priority 1 - Lowest priority, can be delayed without affecting core operations.
	ChannelGossip byte = 0x05
)

// defaultSendQueueCapacity is the default capacity for channel send queues.
const defaultSendQueueCapacity = 256

// defaultRecvBufferCapacity is the default buffer size for reassembling fragmented messages.
const defaultRecvBufferCapacity = 4096

// defaultRecvMessageCapacity is the default maximum size for a complete reassembled message (21MB).
const defaultRecvMessageCapacity = 22020096

// DefaultChannelDescriptors returns the standard set of channel descriptors
// used by NogoChain P2P connections.
//
// Channels are ordered by priority (not by ID):
//   - ChannelConsensus (0x04): Priority 7 - Highest
//   - ChannelSync (0x01):      Priority 5 - High
//   - ChannelBlock (0x03):     Priority 5 - High
//   - ChannelTx (0x02):        Priority 3 - Medium
//   - ChannelGossip (0x05):    Priority 1 - Lowest
func DefaultChannelDescriptors() []*ChannelDescriptor {
	return []*ChannelDescriptor{
		{
			ID:                  ChannelSync,
			Priority:            5,
			SendQueueCapacity:   defaultSendQueueCapacity,
			RecvBufferCapacity:  defaultRecvBufferCapacity,
			RecvMessageCapacity: defaultRecvMessageCapacity,
		},
		{
			ID:                  ChannelTx,
			Priority:            3,
			SendQueueCapacity:   defaultSendQueueCapacity,
			RecvBufferCapacity:  defaultRecvBufferCapacity,
			RecvMessageCapacity: defaultRecvMessageCapacity,
		},
		{
			ID:                  ChannelBlock,
			Priority:            5,
			SendQueueCapacity:   defaultSendQueueCapacity,
			RecvBufferCapacity:  defaultRecvBufferCapacity,
			RecvMessageCapacity: defaultRecvMessageCapacity,
		},
		{
			ID:                  ChannelConsensus,
			Priority:            7,
			SendQueueCapacity:   defaultSendQueueCapacity,
			RecvBufferCapacity:  defaultRecvBufferCapacity,
			RecvMessageCapacity: defaultRecvMessageCapacity,
		},
		{
			ID:                  ChannelGossip,
			Priority:            1,
			SendQueueCapacity:   defaultSendQueueCapacity,
			RecvBufferCapacity:  defaultRecvBufferCapacity,
			RecvMessageCapacity: defaultRecvMessageCapacity,
		},
	}
}

// ChannelName returns a human-readable name for a channel ID.
// Useful for logging and debugging.
func ChannelName(chID byte) string {
	switch chID {
	case ChannelSync:
		return "sync"
	case ChannelTx:
		return "tx"
	case ChannelBlock:
		return "block"
	case ChannelConsensus:
		return "consensus"
	case ChannelGossip:
		return "gossip"
	default:
		return "unknown"
	}
}
