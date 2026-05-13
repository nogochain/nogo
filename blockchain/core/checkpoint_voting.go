package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	CheckpointInterval   = 1000
	CheckpointMinVotes   = 2
	CheckpointThreshold  = 0.67
	CheckpointVoteTimeout = 60 * time.Second
	MaxPendingCheckpoints = 5
)

type ValidatorSignature struct {
	ValidatorID string `json:"validator_id"`
	PubKey      []byte `json:"pub_key"`
	Signature   []byte `json:"signature"`
	Timestamp   int64  `json:"timestamp"`
}

type CheckpointRecord struct {
	Height            uint64               `json:"height"`
	BlockHash         string               `json:"block_hash"`
	Timestamp         int64                `json:"timestamp"`
	PrevCheckpointHash string              `json:"prev_checkpoint_hash"`
	Signatures        []ValidatorSignature `json:"signatures"`
	ActiveValidatorCount int               `json:"active_validator_count"`
}

type CheckpointVote struct {
	Height       uint64 `json:"height"`
	BlockHash    string `json:"block_hash"`
	ValidatorID  string `json:"validator_id"`
	PubKey       []byte `json:"pub_key"`
	Signature    []byte `json:"signature"`
	Timestamp    int64  `json:"timestamp"`
}

type CheckpointVoter struct {
	mu              sync.Mutex
	privateKey      ed25519.PrivateKey
	publicKey       ed25519.PublicKey
	validatorID     string
	votes           map[uint64]map[string]*CheckpointVote
	pendingRecords  map[uint64]*CheckpointRecord
	latestCheckpoint *CheckpointRecord
	store           ChainStore
	onCheckpointFinalized func(*CheckpointRecord)
}

func NewCheckpointVoter(privKey ed25519.PrivateKey, pubKey ed25519.PublicKey, validatorID string, store ChainStore) *CheckpointVoter {
	return &CheckpointVoter{
		privateKey:      privKey,
		publicKey:       pubKey,
		validatorID:     validatorID,
		votes:           make(map[uint64]map[string]*CheckpointVote),
		pendingRecords:  make(map[uint64]*CheckpointRecord),
		store:           store,
	}
}

func (cv *CheckpointVoter) SetOnCheckpointFinalized(cb func(*CheckpointRecord)) {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	cv.onCheckpointFinalized = cb
}

func (cv *CheckpointVoter) SignCheckpoint(height uint64, blockHash string) (*CheckpointVote, error) {
	payload := buildVotePayload(height, blockHash)
	sig := ed25519.Sign(cv.privateKey, payload)

	vote := &CheckpointVote{
		Height:      height,
		BlockHash:   blockHash,
		ValidatorID: cv.validatorID,
		PubKey:      cv.publicKey,
		Signature:   sig,
		Timestamp:   time.Now().Unix(),
	}

	cv.mu.Lock()
	defer cv.mu.Unlock()

	if cv.votes[height] == nil {
		cv.votes[height] = make(map[string]*CheckpointVote)
		if len(cv.votes) > MaxPendingCheckpoints {
			for h := range cv.votes {
				if h < height-MaxPendingCheckpoints*CheckpointInterval {
					delete(cv.votes, h)
					delete(cv.pendingRecords, h)
				}
			}
		}
	}
	cv.votes[height][cv.validatorID] = vote

	log.Printf("[CheckpointVoter] Signed checkpoint h=%d hash=%s", height, blockHash[:16])
	return vote, nil
}

func (cv *CheckpointVoter) ReceiveVote(vote *CheckpointVote) (bool, *CheckpointRecord, error) {
	if vote == nil || vote.Height == 0 || vote.BlockHash == "" {
		return false, nil, fmt.Errorf("receive vote: invalid vote data")
	}

	payload := buildVotePayload(vote.Height, vote.BlockHash)
	if !ed25519.Verify(vote.PubKey, payload, vote.Signature) {
		return false, nil, fmt.Errorf("receive vote: signature verification failed for validator %s", vote.ValidatorID[:16])
	}

	voteHash := sha256.Sum256(vote.PubKey)
	voteID := fmt.Sprintf("%x", voteHash[:8])

	cv.mu.Lock()
	defer cv.mu.Unlock()

	if cv.votes[vote.Height] == nil {
		cv.votes[vote.Height] = make(map[string]*CheckpointVote)
	}
	cv.votes[vote.Height][voteID] = vote

	return cv.checkQuorumLocked(vote.Height)
}

func (cv *CheckpointVoter) GetLatestCheckpoint() *CheckpointRecord {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	if cv.latestCheckpoint != nil {
		cp := *cv.latestCheckpoint
		return &cp
	}
	return nil
}

func (cv *CheckpointVoter) LoadLatestCheckpoint() *CheckpointRecord {
	cv.mu.Lock()
	defer cv.mu.Unlock()

	if cv.store == nil {
		return nil
	}

	_, hash, err := cv.store.LatestCheckpoint()
	if err != nil || hash == "" {
		return nil
	}

	cpBytes, found, cpErr := cv.store.GetCheckpoints()
	if cpErr != nil || !found {
		return nil
	}

	var records []CheckpointRecord
	if unmarshalErr := json.Unmarshal(cpBytes, &records); unmarshalErr != nil {
		return nil
	}

	if len(records) > 0 {
		cv.latestCheckpoint = &records[len(records)-1]
		return cv.latestCheckpoint
	}
	return nil
}

func (cv *CheckpointVoter) checkQuorumLocked(height uint64) (bool, *CheckpointRecord, error) {
	votes := cv.votes[height]
	if len(votes) < CheckpointMinVotes {
		return false, nil, nil
	}

	hashCounts := make(map[string]int)
	for _, v := range votes {
		hashCounts[v.BlockHash]++
		if hashCounts[v.BlockHash] >= CheckpointMinVotes {
			return cv.finalizeLocked(height, v.BlockHash), nil, nil
		}
	}

	bestHash := ""
	bestCount := 0
	for h, c := range hashCounts {
		if c > bestCount {
			bestCount = c
			bestHash = h
		}
	}

	if float64(bestCount) >= float64(len(votes))*CheckpointThreshold {
		return cv.finalizeLocked(height, bestHash), nil, nil
	}

	return false, nil, nil
}

func (cv *CheckpointVoter) finalizeLocked(height uint64, blockHash string) bool {
	votes := cv.votes[height]
	signatures := make([]ValidatorSignature, 0, len(votes))
	for _, v := range votes {
		if v.BlockHash == blockHash {
			signatures = append(signatures, ValidatorSignature{
				ValidatorID: v.ValidatorID,
				PubKey:      v.PubKey,
				Signature:   v.Signature,
				Timestamp:   v.Timestamp,
			})
		}
	}

	prevHash := ""
	if cv.latestCheckpoint != nil {
		prevHash = sha256Hash(fmt.Sprintf("%d:%s", cv.latestCheckpoint.Height, cv.latestCheckpoint.BlockHash))
	}

	record := &CheckpointRecord{
		Height:               height,
		BlockHash:            blockHash,
		Timestamp:            time.Now().Unix(),
		PrevCheckpointHash:   prevHash,
		Signatures:           signatures,
		ActiveValidatorCount: len(votes),
	}

	cv.latestCheckpoint = record
	cv.pendingRecords[height] = record

	if cv.store != nil {
		var allRecords []CheckpointRecord
		if existingBytes, found, _ := cv.store.GetCheckpoints(); found {
			_ = json.Unmarshal(existingBytes, &allRecords)
		}

		replaced := false
		for i, r := range allRecords {
			if r.Height == height {
				allRecords[i] = *record
				replaced = true
				break
			}
		}
		if !replaced {
			allRecords = append(allRecords, *record)
		}

		if data, marshalErr := json.Marshal(allRecords); marshalErr == nil {
			if putErr := cv.store.PutCheckpoints(data); putErr != nil {
				log.Printf("[CheckpointVoter] WARNING: failed to persist finalized checkpoint h=%d: %v", height, putErr)
			}
		}
	}

	log.Printf("[CheckpointVoter] Checkpoint FINALIZED h=%d hash=%s sigs=%d validators=%d",
		height, blockHash[:16], len(signatures), len(votes))

	delete(cv.votes, height)

	if cv.onCheckpointFinalized != nil {
		go cv.onCheckpointFinalized(record)
	}

	return true
}

func (cv *CheckpointVoter) GetPendingVotes(height uint64) ([]*CheckpointVote, bool) {
	cv.mu.Lock()
	defer cv.mu.Unlock()

	votes, exists := cv.votes[height]
	if !exists {
		return nil, false
	}

	result := make([]*CheckpointVote, 0, len(votes))
	for _, v := range votes {
		result = append(result, v)
	}
	return result, true
}

func (cv *CheckpointVoter) VerifyCheckpointSignature(sig ValidatorSignature, height uint64, blockHash string) bool {
	payload := buildVotePayload(height, blockHash)
	return ed25519.Verify(sig.PubKey, payload, sig.Signature)
}

func VerifyCheckpointRecord(record *CheckpointRecord) error {
	if record == nil {
		return fmt.Errorf("verify checkpoint: nil record")
	}
	if record.Height == 0 {
		return fmt.Errorf("verify checkpoint: invalid height")
	}
	if record.BlockHash == "" {
		return fmt.Errorf("verify checkpoint: empty block hash")
	}

	minRequired := 2
	if record.ActiveValidatorCount > 0 {
		requiredByRatio := int(float64(record.ActiveValidatorCount) * CheckpointThreshold)
		if requiredByRatio < CheckpointMinVotes {
			requiredByRatio = CheckpointMinVotes
		}
		minRequired = requiredByRatio
	}

	if len(record.Signatures) < minRequired {
		return fmt.Errorf("verify checkpoint: insufficient signatures (got %d, need %d of %d validators)",
			len(record.Signatures), minRequired, record.ActiveValidatorCount)
	}

	seenValidators := make(map[string]bool)
	for _, sig := range record.Signatures {
		if seenValidators[sig.ValidatorID] {
			return fmt.Errorf("verify checkpoint: duplicate validator %s", sig.ValidatorID[:16])
		}
		seenValidators[sig.ValidatorID] = true

		payload := buildVotePayload(record.Height, record.BlockHash)
		if !ed25519.Verify(sig.PubKey, payload, sig.Signature) {
			return fmt.Errorf("verify checkpoint: invalid signature from validator %s", sig.ValidatorID[:16])
		}
	}

	if record.PrevCheckpointHash != "" {
		if len(record.PrevCheckpointHash) < 8 {
			return fmt.Errorf("verify checkpoint: invalid previous checkpoint hash")
		}
	}

	return nil
}

func buildVotePayload(height uint64, blockHash string) []byte {
	data := fmt.Sprintf("NOGO_CHECKPOINT:%d:%s", height, blockHash)
	h := sha256.Sum256([]byte(data))
	return h[:]
}

func sha256Hash(data string) string {
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h[:])
}

func GenerateValidatorKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}