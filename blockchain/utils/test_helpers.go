// Test helpers for integration testing
// Production-grade helper methods for two-node mining tests

package utils

// This file contains test utilities that are shared across packages.
// Design principle: Test helpers that work with core types should be defined
// in their respective packages as methods. Only generic helpers belong here.

// OrphanPoolProvider defines interface for accessing orphan pool
// Production-grade: proper abstraction for test coordination
type OrphanPoolProvider interface {
	GetOrphanPool() *OrphanPool
}

// GetOrphanPool retrieves orphan pool from provider
// Production-grade: uses interface-based access pattern
// Usage: Useful in integration tests to coordinate between nodes
func GetOrphanPool(provider OrphanPoolProvider) *OrphanPool {
	if provider == nil {
		return nil
	}
	return provider.GetOrphanPool()
}

// Note: SyncLoop implements OrphanPoolProvider interface
// Example implementation in network/sync.go:
//   func (s *SyncLoop) GetOrphanPool() *OrphanPool { return s.orphanPool }
//
// This interface-based design enables:
// 1. Clean separation of concerns
// 2. Easy mocking in unit tests
// 3. Type-safe access to orphan pool
