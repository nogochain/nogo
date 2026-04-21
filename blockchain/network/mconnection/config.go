package mconnection

import (
	"fmt"
	"time"
)

// Default rate and timing constants for MConnection configuration.
const (
	// defaultSendRate is the default maximum send rate in bytes per second (500 KB/s).
	defaultSendRate = int64(512000)

	// defaultRecvRate is the default maximum receive rate in bytes per second (500 KB/s).
	defaultRecvRate = int64(512000)

	// defaultPingTimeout is the default interval between ping packets.
	// Production-grade: 30 seconds to handle network latency and system load
	// This prevents premature disconnection during high-load or high-latency scenarios
	defaultPingTimeout = 30 * time.Second

	// defaultMaxPacketPayloadSize is the default maximum payload per packet fragment.
	defaultMaxPacketPayloadSize = 1024

	// defaultPongTimeout is the maximum time to wait for a pong response.
	// Should be greater than ping timeout to allow for network round-trip
	defaultPongTimeout = 45 * time.Second
)

// MConnConfig holds configuration parameters for MConnection.
type MConnConfig struct {
	// SendRate is the maximum send rate in bytes per second.
	SendRate int64

	// RecvRate is the maximum receive rate in bytes per second.
	RecvRate int64

	// PingTimeout is the interval between heartbeat ping packets.
	PingTimeout time.Duration

	// PongTimeout is the maximum time to wait for a pong response before considering the connection dead.
	PongTimeout time.Duration

	// MaxPacketPayloadSize is the maximum payload size per packet fragment.
	MaxPacketPayloadSize int
}

// DefaultMConnConfig returns a configuration with sensible production defaults.
// SendRate=512000 (500KB/s), RecvRate=512000, PingTimeout=30s, PongTimeout=45s, MaxPacketPayloadSize=1024.
func DefaultMConnConfig() MConnConfig {
	return MConnConfig{
		SendRate:             defaultSendRate,
		RecvRate:             defaultRecvRate,
		PingTimeout:          defaultPingTimeout,
		PongTimeout:          defaultPongTimeout,
		MaxPacketPayloadSize: defaultMaxPacketPayloadSize,
	}
}

// Validate checks that the configuration values are within acceptable ranges.
// Returns an error if any constraint is violated.
func (c *MConnConfig) Validate() error {
	if c.SendRate <= 0 {
		return fmt.Errorf("send rate must be positive, got %d", c.SendRate)
	}
	if c.RecvRate <= 0 {
		return fmt.Errorf("recv rate must be positive, got %d", c.RecvRate)
	}
	if c.PingTimeout <= 0 {
		return fmt.Errorf("ping timeout must be positive, got %v", c.PingTimeout)
	}
	if c.PongTimeout <= 0 {
		return fmt.Errorf("pong timeout must be positive, got %v", c.PongTimeout)
	}
	if c.PongTimeout < c.PingTimeout {
		return fmt.Errorf("pong timeout (%v) must be >= ping timeout (%v)", c.PongTimeout, c.PingTimeout)
	}
	if c.MaxPacketPayloadSize <= 0 {
		return fmt.Errorf("max packet payload size must be positive, got %d", c.MaxPacketPayloadSize)
	}
	if c.MaxPacketPayloadSize > 65535 {
		return fmt.Errorf("max packet payload size must not exceed 65535, got %d", c.MaxPacketPayloadSize)
	}
	return nil
}
