package utils

import (
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// DefaultOrphanPoolSize defines the default maximum number of orphan blocks
const DefaultOrphanPoolSize = 1000

// DefaultOrphanTTL defines the default time-to-live for orphan blocks
const DefaultOrphanTTL = 24 * time.Hour

// OrphanPool manages orphan blocks (blocks with unknown parents)
// Thread-safe implementation with TTL and size limit
type OrphanPool struct {
	// blocks maps block hash to block
	blocks map[string]*core.Block

	// parentIndex maps parent hash to list of child block hashes
	parentIndex map[string][]string

	// timestamps maps block hash to insertion time
	timestamps map[string]time.Time

	// maxSize limits the maximum number of orphan blocks
	maxSize int

	// ttl defines how long orphan blocks are kept
	ttl time.Duration

	// mu protects concurrent access
	mu sync.RWMutex
}

// NewOrphanPool creates a new orphan pool with specified size and TTL
func NewOrphanPool(maxSize int, ttl time.Duration) *OrphanPool {
	if maxSize <= 0 {
		maxSize = DefaultOrphanPoolSize
	}
	if ttl <= 0 {
		ttl = DefaultOrphanTTL
	}

	return &OrphanPool{
		blocks:      make(map[string]*core.Block),
		parentIndex: make(map[string][]string),
		timestamps:  make(map[string]time.Time),
		maxSize:     maxSize,
		ttl:         ttl,
	}
}

// AddOrphan adds an orphan block to the pool
// Returns false if pool is at capacity or block already exists
func (op *OrphanPool) AddOrphan(block *core.Block) bool {
	if block == nil {
		return false
	}

	hash := string(block.Hash)
	parentHash := string(block.Header.PrevHash)

	op.mu.Lock()
	defer op.mu.Unlock()

	// Reject if already exists
	if _, exists := op.blocks[hash]; exists {
		return false
	}

	// Reject if at capacity
	if len(op.blocks) >= op.maxSize {
		return false
	}

	// Add block to main storage
	op.blocks[hash] = block

	// Add to parent index
	op.parentIndex[parentHash] = append(op.parentIndex[parentHash], hash)

	// Record insertion timestamp
	op.timestamps[hash] = time.Now()

	return true
}

// GetOrphansByParent returns all orphan blocks with the specified parent hash
func (op *OrphanPool) GetOrphansByParent(parentHash string) []*core.Block {
	op.mu.RLock()
	defer op.mu.RUnlock()

	childHashes, exists := op.parentIndex[parentHash]
	if !exists {
		return nil
	}

	result := make([]*core.Block, 0, len(childHashes))
	for _, hash := range childHashes {
		if block, ok := op.blocks[hash]; ok {
			result = append(result, block)
		}
	}

	return result
}

// RemoveOrphan removes an orphan block by its hash
// Returns the removed block or nil if not found
func (op *OrphanPool) RemoveOrphan(hash string) *core.Block {
	op.mu.Lock()
	defer op.mu.Unlock()

	// Check if block exists
	block, exists := op.blocks[hash]
	if !exists {
		return nil
	}

	// Remove from parent index
	parentHash := string(block.Header.PrevHash)
	if childHashes, ok := op.parentIndex[parentHash]; ok {
		// Find and remove hash from slice
		for i, h := range childHashes {
			if h == hash {
				// Remove element at index i
				op.parentIndex[parentHash] = append(childHashes[:i], childHashes[i+1:]...)
				break
			}
		}
		// Clean up empty slice
		if len(op.parentIndex[parentHash]) == 0 {
			delete(op.parentIndex, parentHash)
		}
	}

	// Remove from timestamps
	delete(op.timestamps, hash)

	// Remove from blocks
	delete(op.blocks, hash)

	return block
}

// CleanupExpired removes all orphan blocks that have exceeded their TTL
// Returns the number of blocks removed
func (op *OrphanPool) CleanupExpired() int {
	op.mu.Lock()
	defer op.mu.Unlock()

	now := time.Now()
	removed := 0

	for hash, addedAt := range op.timestamps {
		if now.Sub(addedAt) > op.ttl {
			// Get block to find parent hash
			block, exists := op.blocks[hash]
			if !exists {
				continue
			}

			// Remove from parent index
			parentHash := string(block.Header.PrevHash)
			if childHashes, ok := op.parentIndex[parentHash]; ok {
				for i, h := range childHashes {
					if h == hash {
						op.parentIndex[parentHash] = append(childHashes[:i], childHashes[i+1:]...)
						break
					}
				}
				if len(op.parentIndex[parentHash]) == 0 {
					delete(op.parentIndex, parentHash)
				}
			}

			// Remove from all maps
			delete(op.timestamps, hash)
			delete(op.blocks, hash)
			removed++
		}
	}

	return removed
}

// Size returns the current number of orphan blocks in the pool
func (op *OrphanPool) Size() int {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return len(op.blocks)
}

// SetMaxSize updates the maximum size limit
// If new size is smaller than current count, oldest blocks are removed
func (op *OrphanPool) SetMaxSize(maxSize int) {
	if maxSize <= 0 {
		maxSize = DefaultOrphanPoolSize
	}

	op.mu.Lock()
	defer op.mu.Unlock()

	oldMaxSize := op.maxSize
	op.maxSize = maxSize

	// If new size is smaller, remove oldest blocks
	if maxSize < oldMaxSize && maxSize < len(op.blocks) {
		// Sort by timestamp and remove oldest
		type hashTime struct {
			hash string
			time time.Time
		}

		blocks := make([]hashTime, 0, len(op.timestamps))
		for hash, t := range op.timestamps {
			blocks = append(blocks, hashTime{hash: hash, time: t})
		}

		// Sort by time (oldest first)
		for i := 0; i < len(blocks)-1; i++ {
			for j := i + 1; j < len(blocks); j++ {
				if blocks[j].time.Before(blocks[i].time) {
					blocks[i], blocks[j] = blocks[j], blocks[i]
				}
			}
		}

		// Remove oldest blocks
		toRemove := len(blocks) - maxSize
		for i := 0; i < toRemove; i++ {
			hash := blocks[i].hash
			block, exists := op.blocks[hash]
			if !exists {
				continue
			}

			// Remove from parent index
			parentHash := string(block.Header.PrevHash)
			if childHashes, ok := op.parentIndex[parentHash]; ok {
				for idx, h := range childHashes {
					if h == hash {
						op.parentIndex[parentHash] = append(childHashes[:idx], childHashes[idx+1:]...)
						break
					}
				}
				if len(op.parentIndex[parentHash]) == 0 {
					delete(op.parentIndex, parentHash)
				}
			}

			delete(op.timestamps, hash)
			delete(op.blocks, hash)
		}
	}
}

// GetOrphan returns an orphan block by its hash
func (op *OrphanPool) GetOrphan(hash string) *core.Block {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.blocks[hash]
}

// HasOrphan checks if an orphan block exists in the pool
func (op *OrphanPool) HasOrphan(hash string) bool {
	op.mu.RLock()
	defer op.mu.RUnlock()
	_, exists := op.blocks[hash]
	return exists
}

// Clear removes all orphan blocks from the pool
func (op *OrphanPool) Clear() {
	op.mu.Lock()
	defer op.mu.Unlock()

	op.blocks = make(map[string]*core.Block)
	op.parentIndex = make(map[string][]string)
	op.timestamps = make(map[string]time.Time)
}
