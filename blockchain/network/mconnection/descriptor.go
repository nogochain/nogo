package mconnection

import (
	"fmt"
)

// ChannelDescriptor defines the configuration for a multiplexed channel.
// Each channel has a unique ID and priority for message scheduling.
type ChannelDescriptor struct {
	// ID is the unique byte identifier for this channel (0x01-0xFF).
	ID byte

	// Priority determines the relative priority for send scheduling.
	// Higher values mean higher priority. Must be > 0.
	Priority int

	// SendQueueCapacity is the maximum number of pending messages in the send queue.
	// Must be > 0.
	SendQueueCapacity int

	// RecvBufferCapacity is the maximum buffer size for reassembling fragmented messages.
	// Must be > 0.
	RecvBufferCapacity int

	// RecvMessageCapacity is the maximum size of a complete reassembled message.
	// Must be > 0.
	RecvMessageCapacity int
}

// Validate checks that the channel descriptor has valid configuration.
// Returns an error if any constraint is violated.
func (cd *ChannelDescriptor) Validate() error {
	if cd.ID == 0x00 {
		return fmt.Errorf("channel ID must not be zero")
	}
	if cd.Priority <= 0 {
		return fmt.Errorf("channel %d: priority must be positive, got %d", cd.ID, cd.Priority)
	}
	if cd.SendQueueCapacity <= 0 {
		return fmt.Errorf("channel %d: send queue capacity must be positive, got %d", cd.ID, cd.SendQueueCapacity)
	}
	if cd.RecvBufferCapacity <= 0 {
		return fmt.Errorf("channel %d: recv buffer capacity must be positive, got %d", cd.ID, cd.RecvBufferCapacity)
	}
	if cd.RecvMessageCapacity <= 0 {
		return fmt.Errorf("channel %d: recv message capacity must be positive, got %d", cd.ID, cd.RecvMessageCapacity)
	}
	return nil
}

// ValidateUnique checks that all channel descriptors have unique IDs.
// Returns an error if duplicate IDs are found.
func ValidateUnique(descs []*ChannelDescriptor) error {
	seen := make(map[byte]bool, len(descs))
	for _, desc := range descs {
		if seen[desc.ID] {
			return fmt.Errorf("duplicate channel ID: 0x%02x", desc.ID)
		}
		seen[desc.ID] = true
	}
	return nil
}

// ValidateDescriptors validates all channel descriptors and checks for unique IDs.
// Returns an error if any descriptor is invalid or IDs are duplicated.
func ValidateDescriptors(descs []*ChannelDescriptor) error {
	for _, desc := range descs {
		if err := desc.Validate(); err != nil {
			return fmt.Errorf("invalid channel descriptor: %w", err)
		}
	}
	if err := ValidateUnique(descs); err != nil {
		return fmt.Errorf("channel validation: %w", err)
	}
	return nil
}
