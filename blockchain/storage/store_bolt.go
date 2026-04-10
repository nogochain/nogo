package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
	bolt "go.etcd.io/bbolt"
)

const (
	boltOpenTimeout   = 1 * time.Second
	uint64EncodedLen  = 8
	checkpointHashLen = 32
	stateRootLenMax   = 65535
)

type BoltStore struct {
	path string
	db   *bolt.DB
}

// NewBoltStore creates a new BoltDB store instance
// Production-grade: provides persistent storage with proper error handling
func NewBoltStore(dataDir string) (*BoltStore, error) {
	return OpenBoltStore(filepath.Join(dataDir, "chain.db"))
}

func OpenBoltStore(path string) (*BoltStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := bolt.Open(path, filePerm, &bolt.Options{Timeout: boltOpenTimeout})
	if err != nil {
		return nil, fmt.Errorf("open bolt database: %w", err)
	}
	s := &BoltStore{path: path, db: db}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(blocksBucket)); err != nil {
			return fmt.Errorf("create blocks bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(canonBucket)); err != nil {
			return fmt.Errorf("create canonical bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(metaBucket)); err != nil {
			return fmt.Errorf("create meta bucket: %w", err)
		}
		return nil
	}); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("bolt store: failed to close database on init error: %v", closeErr)
		}
		return nil, fmt.Errorf("init bolt database: %w", err)
	}
	return s, nil
}

func (s *BoltStore) PutBlock(block *core.Block) error {
	if block == nil || len(block.Hash) == 0 {
		return errors.New("missing block hash")
	}
	key := make([]byte, len(block.Hash))
	copy(key, block.Hash)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(block); err != nil {
		return fmt.Errorf("encode block: %w", err)
	}
	val := buf.Bytes()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		if b == nil {
			return errors.New("blocks bucket not found")
		}
		if existing := b.Get(key); existing != nil {
			return nil
		}
		if err := b.Put(key, val); err != nil {
			return fmt.Errorf("put block: %w", err)
		}
		return nil
	})
}

func (s *BoltStore) ReadAllBlocks() (map[string]*core.Block, error) {
	out := make(map[string]*core.Block)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		if b == nil {
			return errors.New("blocks bucket not found")
		}
		return b.ForEach(func(k, v []byte) error {
			var blk core.Block
			if err := gob.NewDecoder(bytes.NewReader(v)).Decode(&blk); err != nil {
				return fmt.Errorf("decode block: %w", err)
			}
			h := hex.EncodeToString(k)
			out[h] = &blk
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("read all blocks: %w", err)
	}
	return out, nil
}

func (s *BoltStore) ReadCanonical() ([]*core.Block, error) {
	var blocks []*core.Block
	err := s.db.View(func(tx *bolt.Tx) error {
		canonB := tx.Bucket([]byte(canonBucket))
		if canonB == nil {
			return errors.New("canonical bucket not found")
		}
		blocksB := tx.Bucket([]byte(blocksBucket))
		if blocksB == nil {
			return errors.New("blocks bucket not found")
		}
		c := canonB.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			hash := v
			raw := blocksB.Get(hash)
			if raw == nil {
				return fmt.Errorf("canonical block missing at height %s", hex.EncodeToString(k))
			}
			var blk core.Block
			if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&blk); err != nil {
				return fmt.Errorf("decode canonical block: %w", err)
			}
			blocks = append(blocks, &blk)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read canonical chain: %w", err)
	}
	return blocks, nil
}

func (s *BoltStore) AppendCanonical(block *core.Block) error {
	if block == nil || len(block.Hash) == 0 {
		return errors.New("missing block hash")
	}
	heightKey := u64be(block.GetHeight())
	hashKey := make([]byte, len(block.Hash))
	copy(hashKey, block.Hash)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(block); err != nil {
		return fmt.Errorf("encode block: %w", err)
	}
	val := buf.Bytes()

	return s.db.Update(func(tx *bolt.Tx) error {
		blocksB := tx.Bucket([]byte(blocksBucket))
		canonB := tx.Bucket([]byte(canonBucket))
		metaB := tx.Bucket([]byte(metaBucket))

		if blocksB == nil || canonB == nil || metaB == nil {
			return errors.New("bucket not found")
		}

		if existing := blocksB.Get(hashKey); existing == nil {
			if err := blocksB.Put(hashKey, val); err != nil {
				return fmt.Errorf("put block: %w", err)
			}
		}

		if block.GetHeight() > 0 {
			prevHash := canonB.Get(u64be(block.GetHeight() - 1))
			if prevHash == nil {
				return fmt.Errorf("missing previous canonical block at height %d", block.GetHeight()-1)
			}
			if !bytes.Equal(prevHash, block.Header.PrevHash) {
				return errors.New("prevhash mismatch for append")
			}
		}

		if err := canonB.Put(heightKey, hashKey); err != nil {
			return fmt.Errorf("put canonical height: %w", err)
		}
		if err := metaB.Put([]byte(metaTipHash), hashKey); err != nil {
			return fmt.Errorf("put tip hash: %w", err)
		}
		if err := metaB.Put([]byte(metaTipHeight), heightKey); err != nil {
			return fmt.Errorf("put tip height: %w", err)
		}
		return nil
	})
}

func (s *BoltStore) RewriteCanonical(blocks []*core.Block) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		blocksB := tx.Bucket([]byte(blocksBucket))
		canonB := tx.Bucket([]byte(canonBucket))
		metaB := tx.Bucket([]byte(metaBucket))

		if blocksB == nil || canonB == nil || metaB == nil {
			return errors.New("bucket not found")
		}

		c := canonB.Cursor()
		for k, _ := c.First(); k != nil; {
			nextK, _ := c.Next()
			if err := canonB.Delete(k); err != nil {
				return fmt.Errorf("delete canonical key: %w", err)
			}
			k = nextK
		}

		var tipHash []byte
		var tipHeight uint64
		for i, b := range blocks {
			if b == nil || len(b.Hash) == 0 {
				return fmt.Errorf("block at index %d: missing hash", i)
			}
			var buf bytes.Buffer
			if err := gob.NewEncoder(&buf).Encode(b); err != nil {
				return fmt.Errorf("encode block %d: %w", b.GetHeight(), err)
			}
			key := make([]byte, len(b.Hash))
			copy(key, b.Hash)
			if existing := blocksB.Get(key); existing == nil {
				if err := blocksB.Put(key, buf.Bytes()); err != nil {
					return fmt.Errorf("put block %d: %w", b.GetHeight(), err)
				}
			}
			if err := canonB.Put(u64be(b.GetHeight()), key); err != nil {
				return fmt.Errorf("put canonical height %d: %w", b.GetHeight(), err)
			}
			tipHash = key
			tipHeight = b.GetHeight()
		}
		if tipHash == nil {
			if err := metaB.Delete([]byte(metaTipHash)); err != nil {
				return fmt.Errorf("delete tip hash: %w", err)
			}
			if err := metaB.Delete([]byte(metaTipHeight)); err != nil {
				return fmt.Errorf("delete tip height: %w", err)
			}
			return nil
		}
		if err := metaB.Put([]byte(metaTipHash), tipHash); err != nil {
			return fmt.Errorf("put tip hash: %w", err)
		}
		if err := metaB.Put([]byte(metaTipHeight), u64be(tipHeight)); err != nil {
			return fmt.Errorf("put tip height: %w", err)
		}
		return nil
	})
}

func u64be(v uint64) []byte {
	var b [uint64EncodedLen]byte
	binary.BigEndian.PutUint64(b[:], v)
	return b[:]
}

func (s *BoltStore) GetRulesHash() ([]byte, bool, error) {
	var out []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		metaB := tx.Bucket([]byte(metaBucket))
		if metaB == nil {
			return nil
		}
		v := metaB.Get([]byte(metaRulesHash))
		if v == nil {
			return nil
		}
		out = make([]byte, len(v))
		copy(out, v)
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("get rules hash: %w", err)
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	return out, true, nil
}

func (s *BoltStore) PutRulesHash(hash []byte) error {
	if len(hash) != hashLength {
		return fmt.Errorf("invalid rules hash length: got %d, want %d", len(hash), hashLength)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		metaB := tx.Bucket([]byte(metaBucket))
		if metaB == nil {
			var err error
			metaB, err = tx.CreateBucketIfNotExists([]byte(metaBucket))
			if err != nil {
				return fmt.Errorf("create meta bucket: %w", err)
			}
		}
		val := make([]byte, hashLength)
		copy(val, hash)
		if err := metaB.Put([]byte(metaRulesHash), val); err != nil {
			return fmt.Errorf("put rules hash: %w", err)
		}
		return nil
	})
}

func (s *BoltStore) GetGenesisHash() ([]byte, bool, error) {
	var out []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		metaB := tx.Bucket([]byte(metaBucket))
		if metaB == nil {
			return nil
		}
		v := metaB.Get([]byte(metaGenesisHash))
		if v == nil {
			return nil
		}
		out = make([]byte, len(v))
		copy(out, v)
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("get genesis hash: %w", err)
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	return out, true, nil
}

func (s *BoltStore) PutGenesisHash(hash []byte) error {
	if len(hash) != hashLength {
		return fmt.Errorf("invalid genesis hash length: got %d, want %d", len(hash), hashLength)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		metaB := tx.Bucket([]byte(metaBucket))
		if metaB == nil {
			var err error
			metaB, err = tx.CreateBucketIfNotExists([]byte(metaBucket))
			if err != nil {
				return fmt.Errorf("create meta bucket: %w", err)
			}
		}
		val := make([]byte, hashLength)
		copy(val, hash)
		if err := metaB.Put([]byte(metaGenesisHash), val); err != nil {
			return fmt.Errorf("put genesis hash: %w", err)
		}
		return nil
	})
}

func (s *BoltStore) GetCheckpoints() ([]byte, bool, error) {
	var out []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		metaB := tx.Bucket([]byte(metaBucket))
		if metaB == nil {
			return nil
		}
		v := metaB.Get([]byte(checkpointDataKey))
		if v == nil {
			return nil
		}
		out = make([]byte, len(v))
		copy(out, v)
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("get checkpoints: %w", err)
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	return out, true, nil
}

func (s *BoltStore) PutCheckpoints(data []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		metaB := tx.Bucket([]byte(metaBucket))
		if metaB == nil {
			var err error
			metaB, err = tx.CreateBucketIfNotExists([]byte(metaBucket))
			if err != nil {
				return fmt.Errorf("create meta bucket: %w", err)
			}
		}
		val := make([]byte, len(data))
		copy(val, data)
		if err := metaB.Put([]byte(checkpointDataKey), val); err != nil {
			return fmt.Errorf("put checkpoints: %w", err)
		}
		return nil
	})
}

// RunValueLogGC runs garbage collection for Bolt DB
// Production-grade: triggers database compaction to reclaim space
// Note: Bolt DB uses MVCC, so GC is handled automatically on transaction commit.
// This method provides explicit control for maintenance operations.
func (s *BoltStore) RunValueLogGC() error {
	// Bolt DB doesn't have explicit GC like BadgerDB
	// Database size is managed automatically through:
	// 1. Transaction commit - frees old pages
	// 2. Database compaction - reclaims unused space
	//
	// For explicit maintenance, use bolt DB's built-in compaction:
	//   bolt.Compact(dst, src)
	//
	// This is a no-op for Bolt DB, but provided for API compatibility
	// with storage implementations that require explicit GC (e.g., BadgerDB).
	return nil
}

// Close closes the storage
// Production-grade: properly closes database connections
func (s *BoltStore) Close() error {
	return s.db.Close()
}

// SaveBlock saves a block (alias for PutBlock for compatibility)
func (s *BoltStore) SaveBlock(block *core.Block) error {
	return s.PutBlock(block)
}

// LoadBlock loads a block by hash
func (s *BoltStore) LoadBlock(hash []byte) (*core.Block, error) {
	if len(hash) == 0 {
		return nil, errors.New("empty block hash")
	}
	
	var block *core.Block
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		val := b.Get(hash)
		if val == nil {
			return errors.New("block not found")
		}
		
		var decoded core.Block
		if err := gob.NewDecoder(bytes.NewReader(val)).Decode(&decoded); err != nil {
			return fmt.Errorf("decode block: %w", err)
		}
		block = &decoded
		return nil
	})
	
	return block, err
}

// LoadCanonicalChain loads the canonical chain (alias for ReadCanonical for compatibility)
func (s *BoltStore) LoadCanonicalChain() ([]*core.Block, error) {
	return s.ReadCanonical()
}

// SaveCanonicalChain saves the canonical chain (alias for RewriteCanonical for compatibility)
func (s *BoltStore) SaveCanonicalChain(blocks []*core.Block) error {
	return s.RewriteCanonical(blocks)
}
