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
	baseDataDir     = "nogodata"
	dataDirName     = "data"
	legacyDataDir   = "data"
	blocksGobRel    = "nogodata/data/blocks.gob"
	chainBoltRel    = "nogodata/data/chain.db"
	rulesHashRel    = "nogodata/data/rules.hash"
	genesisHashRel  = "nogodata/data/genesis.hash"
	blocksBucket        = "blocks"
	canonBucket         = "canonical"
	metaBucket          = "meta"
	checkpointBucket    = "checkpoints"
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

	// === Automated Checkpoint System (1000-block intervals) ===

	// PutCheckpointEntry records a block hash checkpoint at the given height.
	PutCheckpointEntry(height uint64, hash string) error

	// GetCheckpointByHeight returns the checkpoint hash at the given height.
	GetCheckpointByHeight(height uint64) (string, bool, error)

	// LatestCheckpoint returns the height and hash of the most recent checkpoint.
	LatestCheckpoint() (uint64, string, error)

	// SerializeSnapshot serializes the state snapshot at the given height for P2P transfer.
	SerializeSnapshot(height uint64) ([]byte, error)

	// DeserializeSnapshot restores a state snapshot from serialized P2P data.
	DeserializeSnapshot(data []byte) (uint64, []byte, map[string]core.Account, error)

	// === State Persistence Methods (P0-1 Fix: state persistence) ===

	// PutAccount persists a single account state.
	// Thread-safe: implementation must handle concurrent access.
	PutAccount(address string, account core.Account) error

	// GetAccount retrieves an account by address.
	// Returns the account, a boolean indicating if found, and any error.
	GetAccount(address string) (core.Account, bool, error)

	// BatchPutAccounts persists multiple accounts atomically.
	// This is more efficient than calling PutAccount multiple times.
	BatchPutAccounts(accounts map[string]core.Account) error

	// Snapshot creates a state snapshot at the specified height.
	// The snapshot includes all account states and the state root hash.
	// Thread-safe: this operation should be atomic.
	Snapshot(height uint64, stateRoot []byte, state map[string]core.Account) error

	// LoadSnapshot loads the most recent state snapshot at or before the specified height.
	// Returns the snapshot height, state root, state map, and any error.
	LoadSnapshot(height uint64) (uint64, []byte, map[string]core.Account, error)

	// LatestSnapshot returns the height of the most recent snapshot.
	// Returns 0 and no error if no snapshot exists.
	LatestSnapshot() (uint64, error)

	// DeleteSnapshot removes a snapshot at the specified height.
	// This is useful for pruning old snapshots to save storage space.
	DeleteSnapshot(height uint64) error

	// CalculateStateRoot calculates the state root hash from the account map.
	// This is used to verify state integrity.
	// Returns the state root hash (32 bytes) and any error.
	CalculateStateRoot(state map[string]core.Account) ([]byte, error)
}

func OpenChainStoreFromEnv() (ChainStore, error) {
	if err := migrateLegacyDataDir(); err != nil {
		log.Printf("WARNING: failed to migrate legacy data directory: %v", err)
	}

	backend := os.Getenv("STORE_BACKEND")
	switch backend {
	case "", "bolt":
		s, err := OpenBoltStore(chainBoltRel)
		if err == nil {
			_ = maybeMigrateGobToBolt(s, blocksGobRel)
			log.Printf("[Store] Opened BoltStore at %s (supports snapshots, fast startup)", chainBoltRel)
			return s, nil
		}
		log.Printf("[Store] WARNING: BoltStore failed to open (%v), falling back to GobStore (no snapshots, slow startup). Set STORE_BACKEND=gob to suppress this.", err)
		return OpenGobStore(blocksGobRel)
	case "gob":
		log.Printf("[Store] Opened GobStore at %s (no snapshots)", blocksGobRel)
		return OpenGobStore(blocksGobRel)
	default:
		return nil, fmt.Errorf("unknown STORE_BACKEND: %q", backend)
	}
}

func migrateLegacyDataDir() error {
	legacyFiles := []string{
		filepath.Join(legacyDataDir, "chain.db"),
		filepath.Join(legacyDataDir, "blocks.gob"),
		filepath.Join(legacyDataDir, "rules.hash"),
		filepath.Join(legacyDataDir, "genesis.hash"),
		filepath.Join(legacyDataDir, "checkpoints.dat"),
	}

	newDataDir := filepath.Join(baseDataDir, dataDirName)
	if err := os.MkdirAll(newDataDir, dirPerm); err != nil {
		return fmt.Errorf("create new data directory: %w", err)
	}

	migrated := false
	for _, legacyFile := range legacyFiles {
		if _, err := os.Stat(legacyFile); err != nil {
			continue
		}

		fileName := filepath.Base(legacyFile)
		newPath := filepath.Join(newDataDir, fileName)

		if _, err := os.Stat(newPath); err == nil {
			continue
		}

		if err := os.Rename(legacyFile, newPath); err != nil {
			log.Printf("WARNING: failed to migrate %s: %v", legacyFile, err)
			continue
		}
		log.Printf("Migrated %s -> %s", legacyFile, newPath)
		migrated = true
	}

	if migrated {
		remaining, _ := os.ReadDir(legacyDataDir)
		if len(remaining) == 0 {
			os.Remove(legacyDataDir)
			log.Printf("Removed empty legacy data directory: %s", legacyDataDir)
		}
	}

	return nil
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
			return fmt.Errorf("encode block at height %d: %w", b.GetHeight(), err)
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

// === GobStore State Persistence Stubs (not supported) ===

// PutAccount is not supported by GobStore.
// Use BoltStore for state persistence.
func (s *GobStore) PutAccount(_ string, _ core.Account) error {
	return fmt.Errorf("put account: not supported by GobStore - please use BoltStore for state persistence")
}

// GetAccount is not supported by GobStore.
// Use BoltStore for state persistence.
func (s *GobStore) GetAccount(_ string) (core.Account, bool, error) {
	return core.Account{}, false, fmt.Errorf("get account: not supported by GobStore - please use BoltStore for state persistence")
}

// BatchPutAccounts is not supported by GobStore.
// Use BoltStore for state persistence.
func (s *GobStore) BatchPutAccounts(_ map[string]core.Account) error {
	return fmt.Errorf("batch put accounts: not supported by GobStore - please use BoltStore for state persistence")
}

// Snapshot is not supported by GobStore.
// Use BoltStore for state persistence.
func (s *GobStore) Snapshot(_ uint64, _ []byte, _ map[string]core.Account) error {
	return fmt.Errorf("snapshot: not supported by GobStore - please use BoltStore for state persistence")
}

// LoadSnapshot is not supported by GobStore.
// Use BoltStore for state persistence.
func (s *GobStore) LoadSnapshot(_ uint64) (uint64, []byte, map[string]core.Account, error) {
	return 0, nil, nil, fmt.Errorf("load snapshot: not supported by GobStore - please use BoltStore for state persistence")
}

// LatestSnapshot is not supported by GobStore.
// Use BoltStore for state persistence.
func (s *GobStore) LatestSnapshot() (uint64, error) {
	return 0, fmt.Errorf("latest snapshot: not supported by GobStore - please use BoltStore for state persistence")
}

// DeleteSnapshot is not supported by GobStore.
// Use BoltStore for state persistence.
func (s *GobStore) DeleteSnapshot(_ uint64) error {
	return fmt.Errorf("delete snapshot: not supported by GobStore - please use BoltStore for state persistence")
}

// CalculateStateRoot is not supported by GobStore.
// Use BoltStore for state persistence.
func (s *GobStore) CalculateStateRoot(_ map[string]core.Account) ([]byte, error) {
	return nil, fmt.Errorf("calculate state root: not supported by GobStore - please use BoltStore for state persistence")
}

// PutCheckpointEntry is not supported by GobStore.
func (s *GobStore) PutCheckpointEntry(_ uint64, _ string) error {
	return fmt.Errorf("checkpoint entry: not supported by GobStore - please use BoltStore")
}

// GetCheckpointByHeight is not supported by GobStore.
func (s *GobStore) GetCheckpointByHeight(_ uint64) (string, bool, error) {
	return "", false, fmt.Errorf("checkpoint by height: not supported by GobStore - please use BoltStore")
}

// LatestCheckpoint is not supported by GobStore.
func (s *GobStore) LatestCheckpoint() (uint64, string, error) {
	return 0, "", fmt.Errorf("latest checkpoint: not supported by GobStore - please use BoltStore")
}

// SerializeSnapshot is not supported by GobStore.
func (s *GobStore) SerializeSnapshot(_ uint64) ([]byte, error) {
	return nil, fmt.Errorf("serialize snapshot: not supported by GobStore - please use BoltStore")
}

// DeserializeSnapshot is not supported by GobStore.
func (s *GobStore) DeserializeSnapshot(_ []byte) (uint64, []byte, map[string]core.Account, error) {
	return 0, nil, nil, fmt.Errorf("deserialize snapshot: not supported by GobStore - please use BoltStore")
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
			return fmt.Errorf("put block %d: %w", b.GetHeight(), err)
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
