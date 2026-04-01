package storage

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	BlocksBucket    = "blocks"
	CanonBucket     = "canonical"
	MetaBucket      = "meta"
	MetaTipHash     = "tipHash"
	MetaTipHeight   = "tipHeight"
	MetaRulesHash   = "rulesHash"
	MetaGenesisHash = "genesisHash"
)

type BoltDB struct {
	DB   *bolt.DB
	path string
}

func OpenBoltDB(path string) (*BoltDB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}
	s := &BoltDB{DB: db, path: path}
	if err := s.DB.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(BlocksBucket)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(CanonBucket)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(MetaBucket)); err != nil {
			return err
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *BoltDB) Close() error {
	return s.DB.Close()
}

func (s *BoltDB) Put(key, value []byte) error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlocksBucket))
		return b.Put(key, value)
	})
}

func (s *BoltDB) Get(key []byte) ([]byte, bool, error) {
	var value []byte
	err := s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlocksBucket))
		v := b.Get(key)
		if v != nil {
			value = append([]byte(nil), v...)
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if value == nil {
		return nil, false, nil
	}
	return value, true, nil
}

func (s *BoltDB) GetAll() (map[string][]byte, error) {
	out := map[string][]byte{}
	err := s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlocksBucket))
		return b.ForEach(func(k, v []byte) error {
			out[string(k)] = v
			return nil
		})
	})
	return out, err
}

func (s *BoltDB) Delete(key []byte) error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BlocksBucket))
		return b.Delete(key)
	})
}

func (s *BoltDB) PutCanonical(hash []byte) error {
	height, err := s.GetCanonicalHeight()
	if err != nil {
		return err
	}

	heightKey := U64BE(height + 1)
	hashKey := append([]byte(nil), hash...)

	return s.DB.Update(func(tx *bolt.Tx) error {
		blocksB := tx.Bucket([]byte(BlocksBucket))
		canonB := tx.Bucket([]byte(CanonBucket))
		metaB := tx.Bucket([]byte(MetaBucket))

		if blocksB.Get(hashKey) == nil {
			return errors.New("block not stored")
		}

		if height > 0 {
			prevHash := canonB.Get(U64BE(height))
			if prevHash == nil {
				return errors.New("missing previous canonical height")
			}
		}

		if err := canonB.Put(heightKey, hashKey); err != nil {
			return err
		}
		if err := metaB.Put([]byte(MetaTipHash), hashKey); err != nil {
			return err
		}
		return metaB.Put([]byte(MetaTipHeight), heightKey)
	})
}

func (s *BoltDB) GetCanonicalHeight() (uint64, error) {
	var height uint64
	err := s.DB.View(func(tx *bolt.Tx) error {
		metaB := tx.Bucket([]byte(MetaBucket))
		v := metaB.Get([]byte(MetaTipHeight))
		if v != nil && len(v) == 8 {
			height = binary.BigEndian.Uint64(v)
		}
		return nil
	})
	return height, err
}

func (s *BoltDB) ReadCanonicalHashes() ([][]byte, error) {
	var hashes [][]byte
	err := s.DB.View(func(tx *bolt.Tx) error {
		canonB := tx.Bucket([]byte(CanonBucket))
		c := canonB.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			hashes = append(hashes, append([]byte(nil), v...))
		}
		return nil
	})
	return hashes, err
}

func (s *BoltDB) RewriteCanonicalHashes(hashes [][]byte) error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		canonB := tx.Bucket([]byte(CanonBucket))
		metaB := tx.Bucket([]byte(MetaBucket))

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
		for i, h := range hashes {
			if len(h) == 0 {
				continue
			}
			hashKey := append([]byte(nil), h...)
			if err := canonB.Put(U64BE(uint64(i)), hashKey); err != nil {
				return err
			}
			tipHash = hashKey
			tipHeight = uint64(i)
		}
		if tipHash == nil {
			_ = metaB.Delete([]byte(MetaTipHash))
			_ = metaB.Delete([]byte(MetaTipHeight))
			return nil
		}
		if err := metaB.Put([]byte(MetaTipHash), tipHash); err != nil {
			return err
		}
		return metaB.Put([]byte(MetaTipHeight), U64BE(tipHeight))
	})
}

func U64BE(v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return b[:]
}

func (s *BoltDB) GetMeta(key []byte) ([]byte, bool, error) {
	var out []byte
	err := s.DB.View(func(tx *bolt.Tx) error {
		metaB := tx.Bucket([]byte(MetaBucket))
		if metaB == nil {
			return nil
		}
		v := metaB.Get(key)
		if v != nil {
			out = append([]byte(nil), v...)
		}
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

func (s *BoltDB) PutMeta(key, value []byte) error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		metaB := tx.Bucket([]byte(MetaBucket))
		if metaB == nil {
			var err error
			metaB, err = tx.CreateBucketIfNotExists([]byte(MetaBucket))
			if err != nil {
				return err
			}
		}
		return metaB.Put(key, value)
	})
}
