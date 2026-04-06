package storage

import (
	"bufio"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/nogochain/nogo/blockchain/core"
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

	hashLength = 32
	filePerm   = 0o600
	dirPerm    = 0o755
)

// ChainStore persists the canonical chain and (optionally) the full block DAG.
type ChainStore interface {
	ReadCanonical() ([]*core.Block, error)
	AppendCanonical(block *core.Block) error
	RewriteCanonical(blocks []*core.Block) error

	// PutBlock persists a block by hash (for fork survival across restarts).
	// Implementations must be idempotent.
	PutBlock(block *core.Block) error
	ReadAllBlocks() (map[string]*core.Block, error)

	// RulesHash persists the consensus rules hash (32 bytes).
	// This is an operator safety mechanism to prevent accidental config forks.
	GetRulesHash() ([]byte, bool, error)
	PutRulesHash(hash []byte) error

	// GenesisHash persists the genesis block hash (32 bytes).
	// This is a network identity anchor to prevent cross-genesis sync.
	GetGenesisHash() ([]byte, bool, error)
	PutGenesisHash(hash []byte) error

	// Checkpoint persistence for fast sync
	GetCheckpoints() ([]byte, bool, error)
	PutCheckpoints(data []byte) error
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
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	return &GobStore{path: path}, nil
}

func (s *GobStore) ReadCanonical() ([]*core.Block, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open gob file: %w", err)
	}
	defer f.Close()

	r := bufio.NewReader(f)
	dec := gob.NewDecoder(r)

	var blocks []*core.Block
	for {
		var b core.Block
		if err := dec.Decode(&b); err == nil {
			blocks = append(blocks, &b)
			continue
		} else if err == io.EOF {
			return blocks, nil
		} else {
			return nil, fmt.Errorf("decode block: %w", err)
		}
	}
}

func (s *GobStore) AppendCanonical(block *core.Block) error {
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePerm)
	if err != nil {
		return fmt.Errorf("open file for append: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	enc := gob.NewEncoder(w)
	if err := enc.Encode(block); err != nil {
		return fmt.Errorf("encode block: %w", err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush buffer: %w", err)
	}
	return nil
}

func (s *GobStore) RewriteCanonical(blocks []*core.Block) error {
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, filePerm)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Printf("store: failed to close temp file: %v", closeErr)
		}
	}()

	w := bufio.NewWriter(f)
	enc := gob.NewEncoder(w)
	for _, b := range blocks {
		if err := enc.Encode(b); err != nil {
			return fmt.Errorf("encode block at height %d: %w", b.Height, err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush buffer: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func (s *GobStore) PutBlock(_ *core.Block) error {
	// legacy store does not persist forks
	return nil
}

func (s *GobStore) ReadAllBlocks() (map[string]*core.Block, error) {
	blocks, err := s.ReadCanonical()
	if err != nil {
		return nil, err
	}
	out := map[string]*core.Block{}
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
	if s.path != "" {
		path = filepath.Join(filepath.Dir(s.path), "rules.hash")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read rules hash: %w", err)
	}
	if len(b) == 0 {
		return nil, false, nil
	}
	if len(b) != hashLength {
		return nil, false, fmt.Errorf("invalid rules hash length: got %d, want %d", len(b), hashLength)
	}
	result := make([]byte, hashLength)
	copy(result, b)
	return result, true, nil
}

func (s *GobStore) PutRulesHash(hash []byte) error {
	if len(hash) != hashLength {
		return fmt.Errorf("invalid rules hash length: got %d, want %d", len(hash), hashLength)
	}
	path := rulesHashRel
	if s.path != "" {
		path = filepath.Join(filepath.Dir(s.path), "rules.hash")
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("create rules hash directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, hash, filePerm); err != nil {
		return fmt.Errorf("write rules hash temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename rules hash file: %w", err)
	}
	return nil
}

func (s *GobStore) GetGenesisHash() ([]byte, bool, error) {
	path := genesisHashRel
	if s.path != "" {
		path = filepath.Join(filepath.Dir(s.path), "genesis.hash")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read genesis hash: %w", err)
	}
	if len(b) == 0 {
		return nil, false, nil
	}
	if len(b) != hashLength {
		return nil, false, fmt.Errorf("invalid genesis hash length: got %d, want %d", len(b), hashLength)
	}
	result := make([]byte, hashLength)
	copy(result, b)
	return result, true, nil
}

func (s *GobStore) PutGenesisHash(hash []byte) error {
	if len(hash) != hashLength {
		return fmt.Errorf("invalid genesis hash length: got %d, want %d", len(hash), hashLength)
	}
	path := genesisHashRel
	if s.path != "" {
		path = filepath.Join(filepath.Dir(s.path), "genesis.hash")
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("create genesis hash directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, hash, filePerm); err != nil {
		return fmt.Errorf("write genesis hash temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename genesis hash file: %w", err)
	}
	return nil
}

func (s *GobStore) GetCheckpoints() ([]byte, bool, error) {
	path := filepath.Join(filepath.Dir(s.path), "checkpoints.dat")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read checkpoints: %w", err)
	}
	if len(b) == 0 {
		return nil, false, nil
	}
	result := make([]byte, len(b))
	copy(result, b)
	return result, true, nil
}

func (s *GobStore) PutCheckpoints(data []byte) error {
	path := filepath.Join(filepath.Dir(s.path), "checkpoints.dat")
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("create checkpoints directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, filePerm); err != nil {
		return fmt.Errorf("write checkpoints temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename checkpoints file: %w", err)
	}
	return nil
}

func maybeMigrateGobToBolt(bolt *BoltStore, gobPath string) error {
	canon, err := bolt.ReadCanonical()
	if err != nil {
		return fmt.Errorf("read canonical from bolt: %w", err)
	}
	if len(canon) > 0 {
		return nil
	}

	gobStore, err := OpenGobStore(gobPath)
	if err != nil {
		return fmt.Errorf("open gob store: %w", err)
	}
	blocks, err := gobStore.ReadCanonical()
	if err != nil {
		return fmt.Errorf("read canonical from gob: %w", err)
	}
	if len(blocks) == 0 {
		return nil
	}
	for _, b := range blocks {
		if err := bolt.PutBlock(b); err != nil {
			return fmt.Errorf("put block %d: %w", b.Height, err)
		}
	}
	if gobPath != "" {
		rulesPath := filepath.Join(filepath.Dir(gobPath), "rules.hash")
		if raw, err := os.ReadFile(rulesPath); err == nil && len(raw) == hashLength {
			if err := bolt.PutRulesHash(raw); err != nil {
				log.Printf("migration: failed to put rules hash: %v", err)
			}
		}
		genesisPath := filepath.Join(filepath.Dir(gobPath), "genesis.hash")
		if raw, err := os.ReadFile(genesisPath); err == nil && len(raw) == hashLength {
			if err := bolt.PutGenesisHash(raw); err != nil {
				log.Printf("migration: failed to put genesis hash: %v", err)
			}
		}
	}
	if err := bolt.RewriteCanonical(blocks); err != nil {
		return fmt.Errorf("rewrite canonical: %w", err)
	}
	return nil
}
