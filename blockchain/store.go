package main

import (
	"bufio"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	dataDirName     = "data"
	blocksGobRel    = "data/blocks.gob"
	chainBoltRel    = "data/chain.db"
	rulesHashRel    = "data/rules.hash"
	genesisHashRel  = "data/genesis.hash"
	blocksBucket    = "blocks"
	canonBucket     = "canonical"
	metaBucket      = "meta"
	metaTipHash     = "tipHash"
	metaTipHeight   = "tipHeight"
	metaRulesHash   = "rulesHash"
	metaGenesisHash = "genesisHash"
)

// ChainStore persists the canonical chain and (optionally) the full block DAG.
type ChainStore interface {
	ReadCanonical() ([]*Block, error)
	AppendCanonical(block *Block) error
	RewriteCanonical(blocks []*Block) error

	// PutBlock persists a block by hash (for fork survival across restarts).
	// Implementations must be idempotent.
	PutBlock(block *Block) error
	ReadAllBlocks() (map[string]*Block, error)

	// RulesHash persists the consensus rules hash (32 bytes).
	// This is an operator safety mechanism to prevent accidental config forks.
	GetRulesHash() ([]byte, bool, error)
	PutRulesHash(hash []byte) error

	// GenesisHash persists the genesis block hash (32 bytes).
	// This is a network identity anchor to prevent cross-genesis sync.
	GetGenesisHash() ([]byte, bool, error)
	PutGenesisHash(hash []byte) error
}

func OpenChainStoreFromEnv() (ChainStore, error) {
	backend := os.Getenv("STORE_BACKEND")
	switch backend {
	case "", "bolt":
		s, err := OpenBoltStore(chainBoltRel)
		if err == nil {
			// Best-effort migration from legacy gob if bolt is empty.
			_ = maybeMigrateGobToBolt(s, blocksGobRel)
			return s, nil
		}
		// Fall back to gob.
		return OpenGobStore(blocksGobRel)
	case "gob":
		return OpenGobStore(blocksGobRel)
	default:
		return nil, fmt.Errorf("unknown STORE_BACKEND: %q", backend)
	}
}

// --- Legacy gob store (canonical only) ---

type GobStore struct {
	path string
}

func OpenGobStore(path string) (*GobStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &GobStore{path: path}, nil
}

func (s *GobStore) ReadCanonical() ([]*Block, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	dec := gob.NewDecoder(r)

	var blocks []*Block
	for {
		var b Block
		err := dec.Decode(&b)
		if err == nil {
			// capture a copy
			bc := b
			blocks = append(blocks, &bc)
			continue
		}
		if err == io.EOF {
			return blocks, nil
		}
		return nil, fmt.Errorf("decode blocks.gob: %w", err)
	}
}

func (s *GobStore) AppendCanonical(block *Block) error {
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	enc := gob.NewEncoder(w)
	if err := enc.Encode(block); err != nil {
		return err
	}
	return w.Flush()
}

func (s *GobStore) RewriteCanonical(blocks []*Block) error {
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	enc := gob.NewEncoder(w)
	for _, b := range blocks {
		if err := enc.Encode(b); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *GobStore) PutBlock(_ *Block) error {
	// legacy store does not persist forks
	return nil
}

func (s *GobStore) ReadAllBlocks() (map[string]*Block, error) {
	blocks, err := s.ReadCanonical()
	if err != nil {
		return nil, err
	}
	out := map[string]*Block{}
	for _, b := range blocks {
		if len(b.Hash) == 0 {
			continue
		}
		out[hex.EncodeToString(b.Hash)] = b
	}
	return out, nil
}

func (s *GobStore) GetRulesHash() ([]byte, bool, error) {
	path := rulesHashRel
	// Keep rules hash file near the gob store.
	if s.path != "" {
		path = filepath.Join(filepath.Dir(s.path), "rules.hash")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(b) == 0 {
		return nil, false, nil
	}
	if len(b) != 32 {
		return nil, false, fmt.Errorf("invalid rules hash length: %d", len(b))
	}
	return append([]byte(nil), b...), true, nil
}

func (s *GobStore) PutRulesHash(hash []byte) error {
	if len(hash) != 32 {
		return errors.New("rules hash must be 32 bytes")
	}
	path := rulesHashRel
	if s.path != "" {
		path = filepath.Join(filepath.Dir(s.path), "rules.hash")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, hash, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *GobStore) GetGenesisHash() ([]byte, bool, error) {
	path := genesisHashRel
	// Keep genesis hash file near the gob store.
	if s.path != "" {
		path = filepath.Join(filepath.Dir(s.path), "genesis.hash")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(b) == 0 {
		return nil, false, nil
	}
	if len(b) != 32 {
		return nil, false, fmt.Errorf("invalid genesis hash length: %d", len(b))
	}
	return append([]byte(nil), b...), true, nil
}

func (s *GobStore) PutGenesisHash(hash []byte) error {
	if len(hash) != 32 {
		return errors.New("genesis hash must be 32 bytes")
	}
	path := genesisHashRel
	if s.path != "" {
		path = filepath.Join(filepath.Dir(s.path), "genesis.hash")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, hash, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func maybeMigrateGobToBolt(bolt *BoltStore, gobPath string) error {
	// If there's already canonical data, don't migrate.
	canon, err := bolt.ReadCanonical()
	if err != nil {
		return err
	}
	if len(canon) > 0 {
		return nil
	}

	gobStore, err := OpenGobStore(gobPath)
	if err != nil {
		return err
	}
	blocks, err := gobStore.ReadCanonical()
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return nil
	}
	for _, b := range blocks {
		if err := bolt.PutBlock(b); err != nil {
			return err
		}
	}
	// Best-effort migrate rules hash (if present).
	if gobPath != "" {
		rulesPath := filepath.Join(filepath.Dir(gobPath), "rules.hash")
		if raw, err := os.ReadFile(rulesPath); err == nil && len(raw) == 32 {
			_ = bolt.PutRulesHash(raw)
		}
		genesisPath := filepath.Join(filepath.Dir(gobPath), "genesis.hash")
		if raw, err := os.ReadFile(genesisPath); err == nil && len(raw) == 32 {
			_ = bolt.PutGenesisHash(raw)
		}
	}
	return bolt.RewriteCanonical(blocks)
}
