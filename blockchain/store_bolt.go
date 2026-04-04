package main

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

	bolt "go.etcd.io/bbolt"
)

type BoltStore struct {
	path string
	db   *bolt.DB
}

func OpenBoltStore(path string) (*BoltStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}
	s := &BoltStore{path: path, db: db}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(blocksBucket)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(canonBucket)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(metaBucket)); err != nil {
			return err
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

func (s *BoltStore) PutBlock(block *Block) error {
	if block == nil || len(block.Hash) == 0 {
		return errors.New("missing block hash")
	}
	key := append([]byte(nil), block.Hash...)
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(block); err != nil {
		return err
	}
	val := buf.Bytes()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		if existing := b.Get(key); existing != nil {
			return nil
		}
		return b.Put(key, val)
	})
}

func (s *BoltStore) ReadAllBlocks() (map[string]*Block, error) {
	out := map[string]*Block{}
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		return b.ForEach(func(k, v []byte) error {
			var blk Block
			if err := gob.NewDecoder(bytes.NewReader(v)).Decode(&blk); err != nil {
				return err
			}
			h := hex.EncodeToString(k)
			bc := blk
			out[h] = &bc
			return nil
		})
	})
	return out, err
}

func (s *BoltStore) ReadCanonical() ([]*Block, error) {
	var blocks []*Block
	err := s.db.View(func(tx *bolt.Tx) error {
		canonB := tx.Bucket([]byte(canonBucket))
		c := canonB.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			hash := v
			raw := tx.Bucket([]byte(blocksBucket)).Get(hash)
			if raw == nil {
				return errors.New("canonical block missing")
			}
			var blk Block
			if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&blk); err != nil {
				return err
			}
			bc := blk
			blocks = append(blocks, &bc)
		}
		return nil
	})
	return blocks, err
}

func (s *BoltStore) AppendCanonical(block *Block) error {
	if block == nil || len(block.Hash) == 0 {
		return errors.New("missing block hash")
	}
	heightKey := u64be(block.Height)
	hashKey := append([]byte(nil), block.Hash...)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(block); err != nil {
		return err
	}
	val := buf.Bytes()

	return s.db.Update(func(tx *bolt.Tx) error {
		blocksB := tx.Bucket([]byte(blocksBucket))
		canonB := tx.Bucket([]byte(canonBucket))
		metaB := tx.Bucket([]byte(metaBucket))

		// Store block by hash (idempotent).
		if existing := blocksB.Get(hashKey); existing == nil {
			if err := blocksB.Put(hashKey, val); err != nil {
				return err
			}
		}

		// Enforce linear append.
		if block.Height > 0 {
			prevHash := canonB.Get(u64be(block.Height - 1))
			if prevHash == nil {
				return errors.New("missing previous canonical height")
			}
			if !bytes.Equal(prevHash, block.PrevHash) {
				return errors.New("prevhash mismatch for append")
			}
		}

		if err := canonB.Put(heightKey, hashKey); err != nil {
			return err
		}
		if err := metaB.Put([]byte(metaTipHash), hashKey); err != nil {
			return err
		}
		return metaB.Put([]byte(metaTipHeight), heightKey)
	})
}

func (s *BoltStore) RewriteCanonical(blocks []*Block) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		blocksB := tx.Bucket([]byte(blocksBucket))
		canonB := tx.Bucket([]byte(canonBucket))
		metaB := tx.Bucket([]byte(metaBucket))

		// Clear canonical bucket.
		c := canonB.Cursor()
		for k, _ := c.First(); k != nil; {
			nextK, _ := c.Next()
			if err := canonB.Delete(k); err != nil {
				return err
			}
			k = nextK
		}

		var tipHash []byte
		var tipHeight uint64
		for _, b := range blocks {
			if b == nil || len(b.Hash) == 0 {
				return errors.New("missing block hash")
			}
			var buf bytes.Buffer
			if err := gob.NewEncoder(&buf).Encode(b); err != nil {
				return err
			}
			key := append([]byte(nil), b.Hash...)
			if existing := blocksB.Get(key); existing == nil {
				if err := blocksB.Put(key, buf.Bytes()); err != nil {
					return err
				}
			}
			if err := canonB.Put(u64be(b.Height), key); err != nil {
				return err
			}
			tipHash = key
			tipHeight = b.Height
		}
		if tipHash == nil {
			// empty chain allowed for init
			_ = metaB.Delete([]byte(metaTipHash))
			_ = metaB.Delete([]byte(metaTipHeight))
			return nil
		}
		if err := metaB.Put([]byte(metaTipHash), tipHash); err != nil {
			return err
		}
		return metaB.Put([]byte(metaTipHeight), u64be(tipHeight))
	})
}

func u64be(v uint64) []byte {
	var b [8]byte
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
		out = append([]byte(nil), v...)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	return out, true, nil
}

func (s *BoltStore) PutRulesHash(hash []byte) error {
	if len(hash) != 32 {
		return errors.New("rules hash must be 32 bytes")
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		metaB := tx.Bucket([]byte(metaBucket))
		if metaB == nil {
			var err error
			metaB, err = tx.CreateBucketIfNotExists([]byte(metaBucket))
			if err != nil {
				return err
			}
		}
		return metaB.Put([]byte(metaRulesHash), append([]byte(nil), hash...))
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
		out = append([]byte(nil), v...)
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	return out, true, nil
}

func (s *BoltStore) PutGenesisHash(hash []byte) error {
	if len(hash) != 32 {
		return errors.New("genesis hash must be 32 bytes")
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		metaB := tx.Bucket([]byte(metaBucket))
		if metaB == nil {
			var err error
			metaB, err = tx.CreateBucketIfNotExists([]byte(metaBucket))
			if err != nil {
				return err
			}
		}
		return metaB.Put([]byte(metaGenesisHash), append([]byte(nil), hash...))
	})
}
