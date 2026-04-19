package reactor

import (
	"sync"

	"github.com/nogochain/nogo/blockchain/network/mconnection"
)

// BaseReactor provides a no-op default implementation of the Reactor interface.
//
// Concrete reactors embed BaseReactor to inherit default behavior and only
// override the methods they need. This follows the Go embedding composition
// pattern for interface implementation.
//
// Thread-safety: all public methods are concurrency-safe using sync.RWMutex.
type BaseReactor struct {
	mu  sync.RWMutex
	sw  SwitchInterface
	chs []*mconnection.ChannelDescriptor
}

// SetSwitch stores the switch reference for later use in sending messages.
// Base implementation stores the switch with write lock protection.
func (br *BaseReactor) SetSwitch(sw SwitchInterface) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.sw = sw
}

// GetSwitch returns the current switch reference.
// Returns nil if SetSwitch has not been called.
func (br *BaseReactor) GetSwitch() SwitchInterface {
	br.mu.RLock()
	defer br.mu.RUnlock()
	return br.sw
}

// SetChannels sets the channel descriptors for this reactor.
func (br *BaseReactor) SetChannels(chs []*mconnection.ChannelDescriptor) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.chs = make([]*mconnection.ChannelDescriptor, len(chs))
	copy(br.chs, chs)
}

// GetChannels returns the channel descriptors this reactor listens on.
// Returns nil by default; concrete reactors should override or call SetChannels.
func (br *BaseReactor) GetChannels() []*mconnection.ChannelDescriptor {
	br.mu.RLock()
	defer br.mu.RUnlock()
	if br.chs == nil {
		return nil
	}
	result := make([]*mconnection.ChannelDescriptor, len(br.chs))
	copy(result, br.chs)
	return result
}

// AddPeer is a no-op default implementation.
// Concrete reactors should override to perform peer-specific initialization.
func (br *BaseReactor) AddPeer(_ string, _ map[string]string) error {
	return nil
}

// RemovePeer is a no-op default implementation.
// Concrete reactors should override to clean up peer-specific state.
func (br *BaseReactor) RemovePeer(_ string, _ interface{}) {
}

// Receive is a no-op default implementation.
// Concrete reactors must override to handle incoming messages.
func (br *BaseReactor) Receive(_ byte, _ string, _ []byte) {
}
