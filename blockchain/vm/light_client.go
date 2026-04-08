package vm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// blockchain Interface for LightClient
type blockchain interface {
	LatestBlock() *core.Block
	GetBlockByHeight(height uint64) *core.Block
}

type LightClient struct {
	bc           blockchain
	trustedBlock *core.Block
	maxSPVDepth  int
	headersChain []core.BlockHeader
	addressIndex map[string][]string
	txIndex      map[string]string
	mu           sync.RWMutex
	serverURL    string
}

type LightClientConfig struct {
	ServerURL     string
	MaxSPVDepth   int
	TrustedHeight uint64
}

func NewLightClient(config LightClientConfig) *LightClient {
	return &LightClient{
		maxSPVDepth:  config.MaxSPVDepth,
		trustedBlock: nil,
		headersChain: make([]core.BlockHeader, 0),
		addressIndex: make(map[string][]string),
		txIndex:      make(map[string]string),
		serverURL:    config.ServerURL,
	}
}

type SPVProof struct {
	TxHash        string   `json:"tx_hash"`
	BlockHash     string   `json:"block_hash"`
	BlockHeight   uint64   `json:"block_height"`
	MerkleProof   []string `json:"merkle_proof"`
	Confirmations uint64   `json:"confirmations"`
}

type SimplifiedPaymentVerification struct {
	Headers      []core.BlockHeader `json:"headers"`
	Transactions []SPVTransaction   `json:"transactions"`
}

type SPVTransaction struct {
	TxID        string `json:"txid"`
	From        string `json:"from"`
	To          string `json:"to"`
	Amount      uint64 `json:"amount"`
	Fee         uint64 `json:"fee"`
	Nonce       uint64 `json:"nonce"`
	BlockHeight uint64 `json:"block_height"`
	Confirmed   bool   `json:"confirmed"`
}

func (lc *LightClient) ConnectToServer(ctx context.Context) error {
	if lc.serverURL == "" {
		return fmt.Errorf("no server URL configured")
	}
	return nil
}

func (lc *LightClient) FetchHeaders(ctx context.Context, fromHeight uint64, count int) error {
	if lc.serverURL == "" {
		return fmt.Errorf("no server configured")
	}

	resp, err := lc.httpGet(ctx, fmt.Sprintf("%s/headers/%d/%d", lc.serverURL, fromHeight, count))
	if err != nil {
		return err
	}

	var headers []core.BlockHeader
	if err := json.Unmarshal(resp, &headers); err != nil {
		return err
	}

	lc.mu.Lock()
	lc.headersChain = append(lc.headersChain, headers...)
	lc.mu.Unlock()

	return nil
}

func (lc *LightClient) VerifyHeader(header core.BlockHeader) bool {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	if len(lc.headersChain) == 0 {
		return false
	}

	for i := range lc.headersChain {
		if lc.headersChain[i].PrevHash == nil || header.PrevHash == nil {
			continue
		}
		if bytes.Equal(lc.headersChain[i].PrevHash, header.PrevHash) &&
			lc.headersChain[i].TimestampUnix == header.TimestampUnix &&
			lc.headersChain[i].DifficultyBits == header.DifficultyBits {
			return true
		}
	}

	return false
}

func (lc *LightClient) FetchTransaction(ctx context.Context, txHash string) (*SPVTransaction, error) {
	if lc.serverURL == "" {
		return nil, fmt.Errorf("no server configured")
	}

	resp, err := lc.httpGet(ctx, fmt.Sprintf("%s/tx/%s", lc.serverURL, txHash))
	if err != nil {
		return nil, err
	}

	var tx core.Transaction
	if err := json.Unmarshal(resp, &tx); err != nil {
		return nil, err
	}

	fromAddr, _ := tx.FromAddress()

	return &SPVTransaction{
		TxID:   txHash,
		From:   fromAddr,
		To:     tx.ToAddress,
		Amount: tx.Amount,
		Fee:    tx.Fee,
		Nonce:  tx.Nonce,
	}, nil
}

func (lc *LightClient) GetBalance(address string) (uint64, error) {
	lc.mu.RLock()
	txHashes := lc.addressIndex[address]
	lc.mu.RUnlock()

	var total uint64
	ctx := context.Background()

	for _, txHash := range txHashes {
		tx, err := lc.FetchTransaction(ctx, txHash)
		if err != nil {
			continue
		}
		if tx.To == address {
			total += tx.Amount
		}
		if tx.From == address {
			total -= tx.Amount + tx.Fee
		}
	}

	return total, nil
}

func (lc *LightClient) SyncAddress(ctx context.Context, address string) error {
	if lc.serverURL == "" {
		return fmt.Errorf("no server configured")
	}

	resp, err := lc.httpGet(ctx, fmt.Sprintf("%s/address/%s/txs", lc.serverURL, address))
	if err != nil {
		return err
	}

	var txs []string
	if err := json.Unmarshal(resp, &txs); err != nil {
		return err
	}

	lc.mu.Lock()
	lc.addressIndex[address] = txs
	lc.mu.Unlock()

	return nil
}

func (lc *LightClient) CreateMerkleProof(txHash, blockHash string) ([]string, error) {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	var targetBlock *core.Block
	for i := range lc.headersChain {
		height := uint64(i)
		if blker, ok := lc.bc.(interface{ GetBlockByHeight(uint64) *core.Block }); ok {
			candidate := blker.GetBlockByHeight(height)
			if candidate != nil && fmt.Sprintf("%x", candidate.Hash) == blockHash {
				targetBlock = candidate
				break
			}
		}
	}

	if targetBlock == nil {
		return nil, fmt.Errorf("block not found")
	}

	txHashes := make([]string, 0, len(targetBlock.Transactions))
	for _, tx := range targetBlock.Transactions {
		txHashes = append(txHashes, tx.GetID())
	}

	tree := NewSPVMerkleTree(txHashes)
	return tree.GetProof(txHash)
}

func (lc *LightClient) VerifyMerkleProof(proof []string, txHash, merkleRoot string) bool {
	if len(proof) == 0 {
		return false
	}

	currentHash := txHash
	for i := range proof {
		currentHash = hashPair(currentHash, proof[i])
	}

	return currentHash == merkleRoot
}

func (lc *LightClient) httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return body, nil
}

type LightClientServer struct {
	lc  *LightClient
	mux *http.ServeMux
}

func NewLightClientServer(lc *LightClient) *LightClientServer {
	s := &LightClientServer{
		lc:  lc,
		mux: http.NewServeMux(),
	}
	s.setupRoutes()
	return s
}

func (s *LightClientServer) setupRoutes() {
	s.mux.HandleFunc("/spv/balance/", s.handleBalance)
	s.mux.HandleFunc("/spv/sync/", s.handleSync)
	s.mux.HandleFunc("/spv/tx/", s.handleTx)
	s.mux.HandleFunc("/spv/headers", s.handleHeaders)
	s.mux.HandleFunc("/spv/proof/", s.handleProof)
}

func (s *LightClientServer) handleBalance(w http.ResponseWriter, r *http.Request) {
	addr := r.URL.Path[len("/spv/balance/"):]
	balance, err := s.lc.GetBalance(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"address": addr, "balance": balance})
}

func (s *LightClientServer) handleSync(w http.ResponseWriter, r *http.Request) {
	addr := r.URL.Path[len("/spv/sync/"):]
	if err := s.lc.SyncAddress(r.Context(), addr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"status": "synced"})
}

func (s *LightClientServer) handleTx(w http.ResponseWriter, r *http.Request) {
	txHash := r.URL.Path[len("/spv/tx/"):]
	tx, err := s.lc.FetchTransaction(r.Context(), txHash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(tx)
}

func (s *LightClientServer) handleHeaders(w http.ResponseWriter, r *http.Request) {
	s.lc.mu.RLock()
	defer s.lc.mu.RUnlock()
	json.NewEncoder(w).Encode(s.lc.headersChain)
}

func (s *LightClientServer) handleProof(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/spv/proof/"):]
	var txHash, blockHash string
	fmt.Sscanf(path, "%s/%s", &txHash, &blockHash)

	proof, err := s.lc.CreateMerkleProof(txHash, blockHash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(SPVProof{
		TxHash:      txHash,
		BlockHash:   blockHash,
		MerkleProof: proof,
	})
}

func (lc *LightClient) GetTransactionsForAddress(address string) []string {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.addressIndex[address]
}

type SPVMerkleTree struct {
	Nodes []string
}

func NewSPVMerkleTree(txHashes []string) *SPVMerkleTree {
	if len(txHashes) == 0 {
		return &SPVMerkleTree{Nodes: []string{}}
	}

	nodes := make([]string, len(txHashes))
	copy(nodes, txHashes)

	for len(nodes) > 1 {
		if len(nodes)%2 != 0 {
			nodes = append(nodes, nodes[len(nodes)-1])
		}

		var nextLevel []string
		for i := 0; i < len(nodes); i += 2 {
			combined := nodes[i] + nodes[i+1]
			hash := sha256.Sum256([]byte(combined))
			nextLevel = append(nextLevel, hex.EncodeToString(hash[:]))
		}
		nodes = nextLevel
	}

	return &SPVMerkleTree{Nodes: nodes}
}

func (mt *SPVMerkleTree) GetRoot() string {
	if len(mt.Nodes) == 0 {
		return ""
	}
	return mt.Nodes[0]
}

func (mt *SPVMerkleTree) GetProof(txHash string) ([]string, error) {
	idx := -1
	for i, h := range mt.Nodes {
		if h == txHash {
			idx = i
			break
		}
	}

	if idx == -1 {
		return nil, fmt.Errorf("transaction not found in merkle tree")
	}

	proof := make([]string, 0)
	level := mt.Nodes
	targetIdx := idx

	for len(level) > 1 {
		if len(level)%2 != 0 {
			level = append(level, level[len(level)-1])
		}

		siblingIdx := targetIdx ^ 1
		if siblingIdx < len(level) {
			proof = append(proof, level[siblingIdx])
		}

		var nextLevel []string
		for i := 0; i < len(level); i += 2 {
			combined := level[i] + level[i+1]
			hash := sha256.Sum256([]byte(combined))
			nextLevel = append(nextLevel, hex.EncodeToString(hash[:]))
		}

		level = nextLevel
		targetIdx /= 2
	}

	return proof, nil
}

func hashPair(a, b string) string {
	combined := a + b
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])
}
