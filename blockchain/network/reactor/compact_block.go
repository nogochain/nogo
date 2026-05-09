package reactor

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/mempool"
)

const (
	// SyncMsgCompactBlock is the message type for compact block relay.
	// Compact blocks contain only the header, short tx IDs, and coinbase tx.
	SyncMsgCompactBlock byte = 0x0B

	// SyncMsgRequestMissingTxs is the message type for requesting missing
	// transactions from a compact block reconstruction.
	SyncMsgRequestMissingTxs byte = 0x0C

	// SyncMsgTx is the message type for sending a single serialized transaction.
	// Used in compact block fallback to fulfill missing tx requests.
	SyncMsgTx byte = 0x0D

	// ShortTxIDBytes defines how many bytes of the full tx hash are used
	// as the short identifier.  4 bytes gives a collision probability
	// of ~2^-32 per pair, negligible for a mempool of <10K entries.
	ShortTxIDBytes = 4

	// maxMempoolLookup is the maximum number of mempool entries to scan
	// when reconstructing a block from short IDs.
	maxMempoolLookup = 50000
)

// CompactBlockMsg contains a compressed block representation for fast relay.
// The receiver reconstructs the full Block by matching ShortTxIDs against
// local mempool entries and requesting only missing transactions.
type CompactBlockMsg struct {
	Header     core.BlockHeader  `json:"header"`
	Height     uint64            `json:"height"`
	ShortTxIDs []string          `json:"short_tx_ids"`
	CoinbaseTx *core.Transaction `json:"coinbase_tx,omitempty"`
	PrevHash   string            `json:"prev_hash"`
	Nonce      uint64            `json:"nonce"`
	FullHash   string            `json:"full_hash"`
}

// MissingTxRequest is sent when a receiver needs full transaction data
// for transactions that could not be reconstructed from the local mempool.
type MissingTxRequest struct {
	BlockHeight uint64   `json:"block_height"`
	BlockHash   string   `json:"block_hash"`
	TxIDs       []string `json:"tx_ids"`
}

// MempoolInterface abstracts the transaction pool for compact block reconstruction.
type MempoolInterface interface {
	GetTx(txID string) (*core.Transaction, bool)
	GetTxIDs() []string
	Size() int
}

// compactBlockReconstructor reconstructs full blocks from compact block messages.
type compactBlockReconstructor struct {
	mu      sync.RWMutex
	mempool MempoolInterface
}

// newCompactBlockReconstructor creates a reconstructor backed by the given mempool.
func newCompactBlockReconstructor(mp MempoolInterface) *compactBlockReconstructor {
	return &compactBlockReconstructor{mempool: mp}
}

// BuildCompactBlock creates a CompactBlockMsg from a full Block.
func BuildCompactBlock(block *core.Block) *CompactBlockMsg {
	if block == nil || len(block.Transactions) == 0 {
		return nil
	}

	shortIDs := make([]string, 0, len(block.Transactions))
	for _, tx := range block.Transactions {
		txID := tx.GetID()
		shortID := txID
		if len(shortID) > ShortTxIDBytes*2 {
			shortID = shortID[:ShortTxIDBytes*2]
		}
		shortIDs = append(shortIDs, shortID)
	}

	blockHash := hex.EncodeToString(block.Hash)
	prevHash := hex.EncodeToString(block.Header.PrevHash)

	return &CompactBlockMsg{
		Header:     block.Header,
		Height:     block.Height,
		ShortTxIDs: shortIDs,
		CoinbaseTx: block.CoinbaseTx,
		PrevHash:   prevHash,
		Nonce:      block.Header.Nonce,
		FullHash:   blockHash,
	}
}

// ReconstructBlock attempts to rebuild a full Block from a CompactBlockMsg
// using the local mempool.  Returns the full block and a list of any
// transaction IDs that were NOT found in the local mempool.
func (cr *compactBlockReconstructor) ReconstructBlock(cb *CompactBlockMsg) (*core.Block, []string, error) {
	if cb == nil {
		return nil, nil, fmt.Errorf("compact block is nil")
	}
	if cb.CoinbaseTx == nil {
		return nil, nil, fmt.Errorf("compact block missing coinbase transaction")
	}

	cr.mu.RLock()
	mp := cr.mempool
	cr.mu.RUnlock()

	if mp == nil {
		return nil, nil, fmt.Errorf("mempool not available for reconstruction")
	}

	allTxIDs := mp.GetTxIDs()
	shortCollisions := make(map[string][]string)

	for _, fullID := range allTxIDs {
		shortID := fullID
		if len(shortID) > ShortTxIDBytes*2 {
			shortID = shortID[:ShortTxIDBytes*2]
		}
		shortCollisions[shortID] = append(shortCollisions[shortID], fullID)
	}

	var missingIDs []string
	var foundTxs []core.Transaction

	foundTxs = append(foundTxs, *cb.CoinbaseTx)

	for _, shortID := range cb.ShortTxIDs {
		candidates, exists := shortCollisions[shortID]
		if !exists {
			missingIDs = append(missingIDs, shortID)
			continue
		}
		matched := false
		for _, fullID := range candidates {
			tx, ok := mp.GetTx(fullID)
			if ok && tx != nil {
				foundTxs = append(foundTxs, *tx)
				matched = true
				break
			}
		}
		if !matched {
			missingIDs = append(missingIDs, candidates[0])
		}
	}

	blockHash, decodeErr := hex.DecodeString(cb.FullHash)
	if decodeErr != nil {
		return nil, nil, fmt.Errorf("decode block hash: %w", decodeErr)
	}

	reconstructed := &core.Block{
		Hash:         blockHash,
		Height:       cb.Height,
		Header:       cb.Header,
		Transactions: foundTxs,
		CoinbaseTx:   cb.CoinbaseTx,
	}

	return reconstructed, missingIDs, nil
}

// SerializeCompactBlock serializes a CompactBlockMsg to wire format.
func SerializeCompactBlock(cb *CompactBlockMsg) []byte {
	data, err := json.Marshal(cb)
	if err != nil {
		log.Printf("[CompactBlock] Marshal failed: %v", err)
		return nil
	}
	msg := make([]byte, 1+len(data))
	msg[0] = SyncMsgCompactBlock
	copy(msg[1:], data)
	return msg
}

// DeserializeCompactBlock parses a CompactBlockMsg from wire format.
func DeserializeCompactBlock(data []byte) (*CompactBlockMsg, error) {
	if len(data) < 2 || data[0] != SyncMsgCompactBlock {
		return nil, fmt.Errorf("invalid compact block message type: 0x%02x", data[0])
	}
	var cb CompactBlockMsg
	if err := json.Unmarshal(data[1:], &cb); err != nil {
		return nil, fmt.Errorf("unmarshal compact block: %w", err)
	}
	if cb.Header.Version == 0 {
		return nil, fmt.Errorf("compact block has zero-version header")
	}
	return &cb, nil
}

// SerializeMissingTxRequest serializes a MissingTxRequest to wire format.
func SerializeMissingTxRequest(req *MissingTxRequest) []byte {
	data, err := json.Marshal(req)
	if err != nil {
		return nil
	}
	msg := make([]byte, 1+len(data))
	msg[0] = SyncMsgRequestMissingTxs
	copy(msg[1:], data)
	return msg
}

// DeserializeMissingTxRequest parses a MissingTxRequest from wire format.
func DeserializeMissingTxRequest(data []byte) (*MissingTxRequest, error) {
	if len(data) < 2 || data[0] != SyncMsgRequestMissingTxs {
		return nil, fmt.Errorf("invalid missing tx request: 0x%02x", data[0])
	}
	var req MissingTxRequest
	if err := json.Unmarshal(data[1:], &req); err != nil {
		return nil, fmt.Errorf("unmarshal missing tx request: %w", err)
	}
	return &req, nil
}

// mempoolWrapper adapts a mempool.Mempool to the MempoolInterface.
type mempoolWrapper struct {
	pool *mempool.Mempool
}

// NewMempoolWrapper creates a MempoolInterface wrapper over a Mempool.
func NewMempoolWrapper(pool *mempool.Mempool) MempoolInterface {
	return &mempoolWrapper{pool: pool}
}

func (mw *mempoolWrapper) GetTx(txID string) (*core.Transaction, bool) {
	return mw.pool.GetTx(txID)
}

func (mw *mempoolWrapper) GetTxIDs() []string {
	return mw.pool.GetTxIDs()
}

func (mw *mempoolWrapper) Size() int {
	return mw.pool.Size()
}
