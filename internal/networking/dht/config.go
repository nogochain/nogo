package dht

import (
	"crypto/ed25519"
	"errors"
	"net"
	"time"
)

// Config errors.
var ErrMissingPrivateKey = errors.New("missing private key")

// Config holds DHT configuration parameters.
type Config struct {
	PrivateKey      ed25519.PrivateKey
	ListenAddr      *net.UDPAddr
	SeedNodes       []*Node
	NodeDBPath      string
	RefreshInterval time.Duration
	BucketSize      int
}

// DefaultConfig returns a Config with sensible production defaults.
func DefaultConfig() Config {
	return Config{
		ListenAddr: &net.UDPAddr{
			IP:   net.IPv4zero,
			Port: 30303,
		},
		RefreshInterval: 1 * time.Hour,
		BucketSize:      16,
	}
}

// Validate checks required config fields and applies defaults.
func (c *Config) Validate() error {
	if c.PrivateKey == nil {
		return ErrMissingPrivateKey
	}
	if c.ListenAddr == nil {
		c.ListenAddr = &net.UDPAddr{IP: net.IPv4zero, Port: 30303}
	}
	if c.BucketSize <= 0 {
		c.BucketSize = 16
	}
	if c.RefreshInterval <= 0 {
		c.RefreshInterval = 1 * time.Hour
	}
	return nil
}
