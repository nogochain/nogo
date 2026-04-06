// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package sync

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

const (
	SnapshotVersion        = 1
	MaxSnapshotSize        = 100 << 20 // 100MB
	MaxAccountProofSize    = 4 << 10   // 4KB
	DefaultSnapshotTimeout = 5 * time.Minute
	MinSnapshotPeers       = 3
	SnapshotBatchSize      = 1000
)

var (
	ErrSnapshotNil            = errors.New("snapshot is nil")
	ErrSnapshotVersion        = errors.New("unsupported snapshot version")
	ErrSnapshotSignature      = errors.New("invalid snapshot signature")
	ErrSnapshotMerkleRoot     = errors.New("merkle root mismatch")
	ErrSnapshotTooLarge       = errors.New("snapshot too large")
	ErrSnapshotTimeout        = errors.New("snapshot download timeout")
	ErrSnapshotIncomplete     = errors.New("incomplete snapshot")
	ErrSnapshotStateRoot      = errors.New("state root mismatch")
	ErrSnapshotAccountProof   = errors.New("invalid account proof")
	ErrNoSnapshotPeers        = errors.New("no snapshot peers available")
	ErrSnapshotDownload       = errors.New("snapshot download failed")
)

type AccountState struct {
	Address string `json:"address"`
	Balance uint64 `json:"balance"`
	Nonce   uint64 `json:"nonce"`
}

type StateSnapshot struct {
	Version       uint8            `json:"version"`
	Checkpoint    *Checkpoint      `json:"checkpoint"`
	AccountStates []AccountState   `json:"accountStates"`
	StateRoot     string           `json:"stateRoot"`
	Timestamp     int64            `json:"timestamp"`
	Validator     []byte           `json:"validator"`
	Signature     []byte           `json:"signature"`
	MerkleRoot    []byte           `json:"merkleRoot"`
	TotalAccounts uint64           `json:"totalAccounts"`
	BlockSize     uint64           `json:"blockSize"`
}

type SnapshotDownloader struct {
	mu              sync.RWMutex
	httpClient      *http.Client
	checkpointMgr   *CheckpointManager
	downloadTimeout time.Duration
	minPeers        int
	currentPeer     string
	progress        float64
	downloading     bool
}

func NewSnapshotDownloader(checkpointMgr *CheckpointManager) *SnapshotDownloader {
	return &SnapshotDownloader{
		httpClient: &http.Client{
			Timeout: DefaultSnapshotTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		checkpointMgr:   checkpointMgr,
		downloadTimeout: DefaultSnapshotTimeout,
		minPeers:        MinSnapshotPeers,
	}
}

func (sd *SnapshotDownloader) SetMinPeers(count int) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.minPeers = count
}

func (sd *SnapshotDownloader) SetTimeout(timeout time.Duration) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.downloadTimeout = timeout
}

func (sd *SnapshotDownloader) GetProgress() float64 {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.progress
}

func (sd *SnapshotDownloader) IsDownloading() bool {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.downloading
}

func (sd *SnapshotDownloader) DownloadFromHTTP(url string) (*StateSnapshot, error) {
	sd.mu.Lock()
	if sd.downloading {
		sd.mu.Unlock()
		return nil, fmt.Errorf("snapshot download already in progress")
	}
	sd.downloading = true
	sd.progress = 0
	sd.mu.Unlock()

	defer func() {
		sd.mu.Lock()
		sd.downloading = false
		sd.mu.Unlock()
	}()

	ctx := &http.Client{
		Timeout: sd.downloadTimeout,
	}

	resp, err := ctx.Get(url)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSnapshotDownload, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP status %d", ErrSnapshotDownload, resp.StatusCode)
	}

	if resp.ContentLength > MaxSnapshotSize {
		return nil, fmt.Errorf("%w: size %d > %d", ErrSnapshotTooLarge, resp.ContentLength, MaxSnapshotSize)
	}

	var bodyReader io.Reader = resp.Body
	if resp.ContentLength > 0 {
		bodyReader = &io.LimitedReader{R: resp.Body, N: MaxSnapshotSize}
	}

	data, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrSnapshotDownload, err)
	}

	sd.mu.Lock()
	sd.progress = 0.5
	sd.mu.Unlock()

	snapshot, err := DecodeSnapshot(data)
	if err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}

	sd.mu.Lock()
	sd.progress = 0.8
	sd.mu.Unlock()

	if err := sd.VerifySnapshot(snapshot); err != nil {
		return nil, fmt.Errorf("verify snapshot: %w", err)
	}

	sd.mu.Lock()
	sd.progress = 1.0
	sd.mu.Unlock()

	return snapshot, nil
}

func (sd *SnapshotDownloader) DownloadFromPeer(peerURL string, checkpointHeight uint64) (*StateSnapshot, error) {
	sd.mu.Lock()
	if sd.downloading {
		sd.mu.Unlock()
		return nil, fmt.Errorf("snapshot download already in progress")
	}
	sd.downloading = true
	sd.progress = 0
	sd.currentPeer = peerURL
	sd.mu.Unlock()

	defer func() {
		sd.mu.Lock()
		sd.downloading = false
		sd.currentPeer = ""
		sd.mu.Unlock()
	}()

	url := fmt.Sprintf("%s/snapshot/%d", peerURL, checkpointHeight)
	return sd.DownloadFromHTTP(url)
}

func (sd *SnapshotDownloader) VerifySnapshot(snapshot *StateSnapshot) error {
	if snapshot == nil {
		return ErrSnapshotNil
	}
	if snapshot.Version != SnapshotVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrSnapshotVersion, snapshot.Version, SnapshotVersion)
	}
	if snapshot.Checkpoint == nil {
		return fmt.Errorf("missing checkpoint in snapshot")
	}
	if err := sd.checkpointMgr.VerifyCheckpoint(snapshot.Checkpoint); err != nil {
		return fmt.Errorf("checkpoint verification failed: %w", err)
	}
	if len(snapshot.Validator) != PubKeySize {
		return fmt.Errorf("invalid validator key length: %d", len(snapshot.Validator))
	}
	if len(snapshot.Signature) != SignatureSize {
		return fmt.Errorf("invalid signature length: %d", len(snapshot.Signature))
	}
	if !sd.verifySnapshotSignature(snapshot) {
		return ErrSnapshotSignature
	}
	computedRoot := sd.computeSnapshotMerkleRoot(snapshot)
	if len(computedRoot) != len(snapshot.MerkleRoot) {
		return fmt.Errorf("%w: length mismatch", ErrSnapshotMerkleRoot)
	}
	for i := range computedRoot {
		if computedRoot[i] != snapshot.MerkleRoot[i] {
			return fmt.Errorf("%w: mismatch at byte %d", ErrSnapshotMerkleRoot, i)
		}
	}
	computedStateRoot := sd.computeStateRoot(snapshot)
	if computedStateRoot != snapshot.StateRoot {
		return fmt.Errorf("%w: computed %s, got %s", ErrSnapshotStateRoot, computedStateRoot, snapshot.StateRoot)
	}
	if snapshot.TotalAccounts != uint64(len(snapshot.AccountStates)) {
		return fmt.Errorf("%w: expected %d, got %d", ErrSnapshotIncomplete, snapshot.TotalAccounts, len(snapshot.AccountStates))
	}
	return nil
}

func (sd *SnapshotDownloader) verifySnapshotSignature(snapshot *StateSnapshot) bool {
	hashToVerify := sd.computeSnapshotHash(snapshot)
	return ed25519.Verify(snapshot.Validator, hashToVerify, snapshot.Signature)
}

func (sd *SnapshotDownloader) computeSnapshotHash(snapshot *StateSnapshot) []byte {
	h := sha256.New()
	versionByte := []byte{snapshot.Version}
	h.Write(versionByte)
	h.Write([]byte(snapshot.StateRoot))
	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, uint64(snapshot.Timestamp))
	h.Write(timestampBytes)
	totalBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(totalBytes, snapshot.TotalAccounts)
	h.Write(totalBytes)
	h.Write(snapshot.MerkleRoot)
	if snapshot.Checkpoint != nil {
		cpHash := sd.checkpointMgr.computeCheckpointHash(snapshot.Checkpoint)
		h.Write(cpHash)
	}
	return h.Sum(nil)
}

func (sd *SnapshotDownloader) computeSnapshotMerkleRoot(snapshot *StateSnapshot) []byte {
	if len(snapshot.AccountStates) == 0 {
		return nil
	}
	leaves := make([][]byte, len(snapshot.AccountStates))
	for i, acc := range snapshot.AccountStates {
		h := sha256.New()
		h.Write([]byte(acc.Address))
		balanceBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(balanceBytes, acc.Balance)
		h.Write(balanceBytes)
		nonceBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(nonceBytes, acc.Nonce)
		h.Write(nonceBytes)
		leaves[i] = h.Sum(nil)
	}
	return computeMerkleRoot(leaves)
}

func (sd *SnapshotDownloader) computeStateRoot(snapshot *StateSnapshot) string {
	h := sha256.New()
	for _, acc := range snapshot.AccountStates {
		h.Write([]byte(acc.Address))
		balanceBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(balanceBytes, acc.Balance)
		h.Write(balanceBytes)
	}
	hash := h.Sum(nil)
	return hex.EncodeToString(hash)
}

func (sd *SnapshotDownloader) GetAccountProof(snapshot *StateSnapshot, address string) ([]byte, error) {
	for i, acc := range snapshot.AccountStates {
		if acc.Address == address {
			proof := make([]byte, 8)
			binary.BigEndian.PutUint64(proof, uint64(i))
			return proof, nil
		}
	}
	return nil, fmt.Errorf("account not found in snapshot")
}

func (sd *SnapshotDownloader) VerifyAccountProof(snapshot *StateSnapshot, address string, proof []byte, expectedBalance uint64) error {
	if len(proof) < 8 {
		return fmt.Errorf("%w: proof too short", ErrSnapshotAccountProof)
	}
	index := binary.BigEndian.Uint64(proof[:8])
	if index >= uint64(len(snapshot.AccountStates)) {
		return fmt.Errorf("%w: index out of range", ErrSnapshotAccountProof)
	}
	acc := snapshot.AccountStates[index]
	if acc.Address != address {
		return fmt.Errorf("%w: address mismatch", ErrSnapshotAccountProof)
	}
	if acc.Balance != expectedBalance {
		return fmt.Errorf("%w: expected %d, got %d", ErrSnapshotAccountProof, expectedBalance, acc.Balance)
	}
	return nil
}

func EncodeSnapshot(snapshot *StateSnapshot) ([]byte, error) {
	if snapshot == nil {
		return nil, ErrSnapshotNil
	}
	if len(snapshot.AccountStates) > math.MaxInt32 {
		return nil, fmt.Errorf("too many accounts: %d", len(snapshot.AccountStates))
	}
	buffer := make([]byte, 0, 1024+len(snapshot.AccountStates)*64)
	buffer = append(buffer, snapshot.Version)
	cpData, err := json.Marshal(snapshot.Checkpoint)
	if err != nil {
		return nil, fmt.Errorf("marshal checkpoint: %w", err)
	}
	cpLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cpLenBytes, uint32(len(cpData)))
	buffer = append(buffer, cpLenBytes...)
	buffer = append(buffer, cpData...)
	stateRootBytes := []byte(snapshot.StateRoot)
	if len(stateRootBytes) > 255 {
		return nil, fmt.Errorf("state root too long: %d", len(stateRootBytes))
	}
	buffer = append(buffer, byte(len(stateRootBytes)))
	buffer = append(buffer, stateRootBytes...)
	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, uint64(snapshot.Timestamp))
	buffer = append(buffer, timestampBytes...)
	if len(snapshot.Validator) != PubKeySize {
		return nil, fmt.Errorf("invalid validator key length: %d", len(snapshot.Validator))
	}
	buffer = append(buffer, snapshot.Validator...)
	if len(snapshot.Signature) != SignatureSize {
		return nil, fmt.Errorf("invalid signature length: %d", len(snapshot.Signature))
	}
	buffer = append(buffer, snapshot.Signature...)
	merkleRoot := snapshot.MerkleRoot
	if len(merkleRoot) > 255 {
		return nil, fmt.Errorf("merkle root too long: %d", len(merkleRoot))
	}
	buffer = append(buffer, byte(len(merkleRoot)))
	buffer = append(buffer, merkleRoot...)
	totalBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(totalBytes, snapshot.TotalAccounts)
	buffer = append(buffer, totalBytes...)
	blockSizeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(blockSizeBytes, snapshot.BlockSize)
	buffer = append(buffer, blockSizeBytes...)
	accountCountBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(accountCountBytes, uint32(len(snapshot.AccountStates)))
	buffer = append(buffer, accountCountBytes...)
	for _, acc := range snapshot.AccountStates {
		addrBytes := []byte(acc.Address)
		if len(addrBytes) > 255 {
			return nil, fmt.Errorf("address too long: %s", acc.Address)
		}
		buffer = append(buffer, byte(len(addrBytes)))
		buffer = append(buffer, addrBytes...)
		balanceBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(balanceBytes, acc.Balance)
		buffer = append(buffer, balanceBytes...)
		nonceBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(nonceBytes, acc.Nonce)
		buffer = append(buffer, nonceBytes...)
	}
	return buffer, nil
}

func DecodeSnapshot(data []byte) (*StateSnapshot, error) {
	if len(data) < 100 {
		return nil, fmt.Errorf("%w: data too short", ErrSnapshotIncomplete)
	}
	offset := 0
	version := data[offset]
	offset++
	if version != SnapshotVersion {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrSnapshotVersion, version, SnapshotVersion)
	}
	if offset+4 > len(data) {
		return nil, fmt.Errorf("%w: missing checkpoint length", ErrSnapshotIncomplete)
	}
	cpLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4
	if offset+int(cpLen) > len(data) {
		return nil, fmt.Errorf("%w: checkpoint data truncated", ErrSnapshotIncomplete)
	}
	cpData := data[offset : offset+int(cpLen)]
	offset += int(cpLen)
	var checkpoint Checkpoint
	if err := json.Unmarshal(cpData, &checkpoint); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	if offset >= len(data) {
		return nil, fmt.Errorf("%w: missing state root length", ErrSnapshotIncomplete)
	}
	stateRootLen := int(data[offset])
	offset++
	if offset+stateRootLen > len(data) {
		return nil, fmt.Errorf("%w: state root truncated", ErrSnapshotIncomplete)
	}
	stateRoot := string(data[offset : offset+stateRootLen])
	offset += stateRootLen
	if offset+8 > len(data) {
		return nil, fmt.Errorf("%w: missing timestamp", ErrSnapshotIncomplete)
	}
	timestamp := int64(binary.BigEndian.Uint64(data[offset : offset+8]))
	offset += 8
	if offset+PubKeySize > len(data) {
		return nil, fmt.Errorf("%w: missing validator key", ErrSnapshotIncomplete)
	}
	validator := make([]byte, PubKeySize)
	copy(validator, data[offset:offset+PubKeySize])
	offset += PubKeySize
	if offset+SignatureSize > len(data) {
		return nil, fmt.Errorf("%w: missing signature", ErrSnapshotIncomplete)
	}
	signature := make([]byte, SignatureSize)
	copy(signature, data[offset:offset+SignatureSize])
	offset += SignatureSize
	if offset >= len(data) {
		return nil, fmt.Errorf("%w: missing merkle root length", ErrSnapshotIncomplete)
	}
	merkleRootLen := int(data[offset])
	offset++
	if offset+merkleRootLen > len(data) {
		return nil, fmt.Errorf("%w: merkle root truncated", ErrSnapshotIncomplete)
	}
	merkleRoot := make([]byte, merkleRootLen)
	copy(merkleRoot, data[offset:offset+merkleRootLen])
	offset += merkleRootLen
	if offset+8 > len(data) {
		return nil, fmt.Errorf("%w: missing total accounts", ErrSnapshotIncomplete)
	}
	totalAccounts := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8
	if offset+8 > len(data) {
		return nil, fmt.Errorf("%w: missing block size", ErrSnapshotIncomplete)
	}
	blockSize := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8
	if offset+4 > len(data) {
		return nil, fmt.Errorf("%w: missing account count", ErrSnapshotIncomplete)
	}
	accountCount := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4
	accountStates := make([]AccountState, 0, accountCount)
	for i := uint32(0); i < accountCount; i++ {
		if offset >= len(data) {
			return nil, fmt.Errorf("%w: missing account %d address length", ErrSnapshotIncomplete, i)
		}
		addrLen := int(data[offset])
		offset++
		if offset+addrLen > len(data) {
			return nil, fmt.Errorf("%w: account %d address truncated", ErrSnapshotIncomplete, i)
		}
		addr := string(data[offset : offset+addrLen])
		offset += addrLen
		if offset+8 > len(data) {
			return nil, fmt.Errorf("%w: account %d missing balance", ErrSnapshotIncomplete, i)
		}
		balance := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8
		if offset+8 > len(data) {
			return nil, fmt.Errorf("%w: account %d missing nonce", ErrSnapshotIncomplete, i)
		}
		nonce := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8
		accountStates = append(accountStates, AccountState{
			Address: addr,
			Balance: balance,
			Nonce:   nonce,
		})
	}
	return &StateSnapshot{
		Version:       version,
		Checkpoint:    &checkpoint,
		AccountStates: accountStates,
		StateRoot:     stateRoot,
		Timestamp:     timestamp,
		Validator:     validator,
		Signature:     signature,
		MerkleRoot:    merkleRoot,
		TotalAccounts: totalAccounts,
		BlockSize:     blockSize,
	}, nil
}

func CreateStateSnapshot(checkpoint *Checkpoint, accounts map[string]*core.Account, validatorPubKey []byte, signFunc func([]byte) ([]byte, error)) (*StateSnapshot, error) {
	if checkpoint == nil {
		return nil, fmt.Errorf("checkpoint is nil")
	}
	if len(validatorPubKey) != PubKeySize {
		return nil, fmt.Errorf("invalid validator key length: %d", len(validatorPubKey))
	}
	accountStates := make([]AccountState, 0, len(accounts))
	for addr, acc := range accounts {
		accountStates = append(accountStates, AccountState{
			Address: addr,
			Balance: acc.Balance,
			Nonce:   acc.Nonce,
		})
	}
	snapshot := &StateSnapshot{
		Version:       SnapshotVersion,
		Checkpoint:    checkpoint,
		AccountStates: accountStates,
		StateRoot:     "",
		Timestamp:     time.Now().Unix(),
		Validator:     make([]byte, len(validatorPubKey)),
		TotalAccounts: uint64(len(accounts)),
	}
	copy(snapshot.Validator, validatorPubKey)
	downloader := NewSnapshotDownloader(&CheckpointManager{})
	snapshot.StateRoot = downloader.computeStateRoot(snapshot)
	snapshot.MerkleRoot = downloader.computeSnapshotMerkleRoot(snapshot)
	hashToSign := downloader.computeSnapshotHash(snapshot)
	signature, err := signFunc(hashToSign)
	if err != nil {
		return nil, fmt.Errorf("sign snapshot: %w", err)
	}
	if len(signature) != SignatureSize {
		return nil, fmt.Errorf("invalid signature length: %d", len(signature))
	}
	snapshot.Signature = make([]byte, len(signature))
	copy(snapshot.Signature, signature)
	return snapshot, nil
}

func (ss *StateSnapshot) GetAccountBalance(address string) (uint64, bool) {
	for _, acc := range ss.AccountStates {
		if acc.Address == address {
			return acc.Balance, true
		}
	}
	return 0, false
}

func (ss *StateSnapshot) GetAccountNonce(address string) (uint64, bool) {
	for _, acc := range ss.AccountStates {
		if acc.Address == address {
			return acc.Nonce, true
		}
	}
	return 0, false
}

func (ss *StateSnapshot) GetAccount(address string) (*AccountState, bool) {
	for _, acc := range ss.AccountStates {
		if acc.Address == address {
			return &acc, true
		}
	}
	return nil, false
}

func (ss *StateSnapshot) MarshalJSON() ([]byte, error) {
	type Alias StateSnapshot
	return json.Marshal(&struct {
		Validator string `json:"validatorHex"`
		Signature string `json:"signatureHex"`
		MerkleRoot string `json:"merkleRootHex"`
		*Alias
	}{
		Validator: hex.EncodeToString(ss.Validator),
		Signature: hex.EncodeToString(ss.Signature),
		MerkleRoot: hex.EncodeToString(ss.MerkleRoot),
		Alias:     (*Alias)(ss),
	})
}

func (ss *StateSnapshot) Size() int {
	size := 1 + 4 + len(ss.StateRoot) + 8 + PubKeySize + SignatureSize + 1 + 8 + 8 + 4
	for _, acc := range ss.AccountStates {
		size += 1 + len(acc.Address) + 8 + 8
	}
	return size
}

func (ss *StateSnapshot) ValidateBasic() error {
	if ss == nil {
		return ErrSnapshotNil
	}
	if ss.Version != SnapshotVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrSnapshotVersion, ss.Version, SnapshotVersion)
	}
	if ss.Checkpoint == nil {
		return errors.New("missing checkpoint")
	}
	if len(ss.Validator) != PubKeySize {
		return fmt.Errorf("invalid validator length: %d", len(ss.Validator))
	}
	if len(ss.Signature) != SignatureSize {
		return fmt.Errorf("invalid signature length: %d", len(ss.Signature))
	}
	if ss.TotalAccounts != uint64(len(ss.AccountStates)) {
		return fmt.Errorf("account count mismatch: %d != %d", ss.TotalAccounts, len(ss.AccountStates))
	}
	if ss.Size() > MaxSnapshotSize {
		return fmt.Errorf("%w: size %d > %d", ErrSnapshotTooLarge, ss.Size(), MaxSnapshotSize)
	}
	return nil
}
