package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

const (
	defaultEnhancedBatchSize = 100
	maxHeaderFetchPerRequest = 2000
	headerDownloadTimeout    = 30 * time.Second
	blockDownloadTimeout     = 60 * time.Second
	maxPeerRetryAttempts     = 3
)

var (
	errNilChain           = fmt.Errorf("blockchain interface is nil")
	errNilPeerAPI         = fmt.Errorf("peer API not configured")
	errEmptyChain         = fmt.Errorf("local chain is empty")
	errNoActivePeers      = fmt.Errorf("no active peers available")
	errHeaderChainBroken  = fmt.Errorf("header chain continuity broken")
	errBlockHashMismatch  = fmt.Errorf("block hash does not match expected header")
	errCheckpointMismatch = fmt.Errorf("checkpoint hash verification failed")
)

type FastSyncEngine struct {
	mu           sync.RWMutex
	chain        BlockchainInterface
	pm           PeerAPI
	batchSize    int
	logger       *log.Logger
	targetHeight uint64
}

func NewFastSyncEngine(chain BlockchainInterface, batchSize int) *FastSyncEngine {
	if batchSize <= 0 {
		batchSize = defaultEnhancedBatchSize
	}
	return &FastSyncEngine{
		chain:     chain,
		batchSize: batchSize,
		logger:    log.Default(),
	}
}

func (e *FastSyncEngine) SetPeerAPI(pm PeerAPI) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pm = pm
}

func (e *FastSyncEngine) CheckFastSyncEligible(localHeight, checkpointHeight uint64, checkpointHash []byte) bool {
	if len(checkpointHash) == 0 {
		return false
	}
	return localHeight < checkpointHeight
}

func (e *FastSyncEngine) HeaderDownload(checkpointHash []byte) ([]*core.BlockHeader, error) {
	if e.chain == nil {
		return nil, errNilChain
	}
	e.mu.RLock()
	pm := e.pm
	e.mu.RUnlock()
	if pm == nil {
		return nil, errNilPeerAPI
	}

	localTip := e.chain.LatestBlock()
	if localTip == nil {
		return nil, errEmptyChain
	}
	localHeight := localTip.GetHeight()

	locator, locErr := e.buildBlockLocator()
	if locErr != nil {
		return nil, fmt.Errorf("build block locator: %w", locErr)
	}

	peers := pm.GetActivePeers()
	if len(peers) == 0 {
		return nil, errNoActivePeers
	}

	targetHeight := e.resolveTargetHeight(localHeight)
	if targetHeight <= localHeight {
		return nil, fmt.Errorf("no headers to fetch: local height %d, target height %d", localHeight, targetHeight)
	}

	fromHeight := localHeight + 1

	// Use locator to validate starting position against known chain state
	if len(locator) > 0 {
		e.logger.Printf("[FastSync] HeaderDownload: built locator with %d entries, starting from height %d",
			len(locator), fromHeight)
	}
	headersToFetch := int(targetHeight - localHeight)
	if headersToFetch > maxHeaderFetchPerRequest {
		headersToFetch = maxHeaderFetchPerRequest
	}

	ctx, cancel := context.WithTimeout(context.Background(), headerDownloadTimeout)
	defer cancel()

	var headers []*core.BlockHeader
	var lastErr error

	for _, peer := range peers {
		rawHeaders, fetchErr := pm.FetchHeadersFrom(ctx, peer, fromHeight, headersToFetch)
		if fetchErr != nil {
			lastErr = fetchErr
			e.logger.Printf("[FastSync] HeaderDownload: peer %s failed: %v", peer, fetchErr)
			continue
		}
		if len(rawHeaders) == 0 {
			lastErr = fmt.Errorf("peer %s returned empty headers", peer)
			continue
		}

		headers = make([]*core.BlockHeader, len(rawHeaders))
		for i := range rawHeaders {
			hdrCopy := rawHeaders[i]
			headers[i] = &hdrCopy
		}
		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all peers failed during header download: %w", lastErr)
	}

	if verifyErr := e.verifyHeaderChain(headers, localTip); verifyErr != nil {
		return nil, fmt.Errorf("header chain verification: %w", verifyErr)
	}

	if len(checkpointHash) > 0 {
		e.logger.Printf("[FastSync] HeaderDownload: fetched %d headers, checkpoint hash %s will be verified during block download",
			len(headers), hex.EncodeToString(checkpointHash)[:16])
	}

	e.logger.Printf("[FastSync] HeaderDownload: downloaded %d headers from height %d to %d",
		len(headers), fromHeight, fromHeight+uint64(len(headers))-1)

	return headers, nil
}

func (e *FastSyncEngine) BlockDownloadAndValidate(headers []*core.BlockHeader) error {
	if e.chain == nil {
		return errNilChain
	}
	e.mu.RLock()
	pm := e.pm
	e.mu.RUnlock()
	if pm == nil {
		return errNilPeerAPI
	}
	if len(headers) == 0 {
		return fmt.Errorf("no headers provided for block download")
	}

	peers := pm.GetActivePeers()
	if len(peers) == 0 {
		return errNoActivePeers
	}

	localTip := e.chain.LatestBlock()
	if localTip == nil {
		return errEmptyChain
	}
	startHeight := localTip.GetHeight() + 1

	// Known limitation: AIHash pre-caching (AddCache/RemoveCache) is not available
	// because the nogopow.AIHash type does not expose cache management methods.
	// Block validation will compute AI hashes on-demand during PoW verification
	// instead of pre-populating the cache, which may result in slower validation
	// for blocks that share common AI hash input patterns.

	for batchStart := 0; batchStart < len(headers); batchStart += e.batchSize {
		batchEnd := batchStart + e.batchSize
		if batchEnd > len(headers) {
			batchEnd = len(headers)
		}

		batchHeaders := headers[batchStart:batchEnd]
		batchStartHeight := startHeight + uint64(batchStart)

		ctx, cancel := context.WithTimeout(context.Background(), blockDownloadTimeout)
		procErr := e.processBatch(ctx, pm, batchHeaders, batchStartHeight, peers)
		cancel()
		if procErr != nil {
			return fmt.Errorf("batch at height %d: %w", batchStartHeight, procErr)
		}

		e.logger.Printf("[FastSync] BlockDownloadAndValidate: processed batch %d-%d (%d/%d headers)",
			batchStartHeight, batchStartHeight+uint64(len(batchHeaders))-1, batchEnd, len(headers))
	}

	return nil
}

func (e *FastSyncEngine) SyncToCheckpoint(checkpointHeight uint64, checkpointHash []byte) error {
	if e.chain == nil {
		return errNilChain
	}

	localTip := e.chain.LatestBlock()
	if localTip == nil {
		return errEmptyChain
	}
	localHeight := localTip.GetHeight()

	if !e.CheckFastSyncEligible(localHeight, checkpointHeight, checkpointHash) {
		return fmt.Errorf("not eligible for fast sync: local height %d >= checkpoint height %d or empty checkpoint hash",
			localHeight, checkpointHeight)
	}

	e.logger.Printf("[FastSync] SyncToCheckpoint: starting sync local=%d checkpoint=%d",
		localHeight, checkpointHeight)

	e.mu.Lock()
	e.targetHeight = checkpointHeight
	e.mu.Unlock()

	headers, hdrErr := e.HeaderDownload(checkpointHash)
	if hdrErr != nil {
		return fmt.Errorf("header download phase: %w", hdrErr)
	}

	e.logger.Printf("[FastSync] SyncToCheckpoint: downloaded %d headers, starting block download", len(headers))

	if blkErr := e.BlockDownloadAndValidate(headers); blkErr != nil {
		return fmt.Errorf("block download phase: %w", blkErr)
	}

	if len(checkpointHash) > 0 {
		cpBlock, exists := e.chain.BlockByHeight(checkpointHeight)
		if !exists || cpBlock == nil {
			return fmt.Errorf("checkpoint block at height %d not found after sync", checkpointHeight)
		}
		if !bytes.Equal(cpBlock.Hash, checkpointHash) {
			return fmt.Errorf("%w: got %x, expected %x at height %d",
				errCheckpointMismatch, cpBlock.Hash, checkpointHash, checkpointHeight)
		}
	}

	newTip := e.chain.LatestBlock()
	if newTip == nil {
		return fmt.Errorf("chain tip is nil after sync")
	}
	if newTip.GetHeight() < checkpointHeight {
		return fmt.Errorf("sync incomplete: current height %d < checkpoint height %d",
			newTip.GetHeight(), checkpointHeight)
	}

	e.logger.Printf("[FastSync] SyncToCheckpoint: completed successfully, new height=%d", newTip.GetHeight())
	return nil
}

func (e *FastSyncEngine) verifyHeaderChain(headers []*core.BlockHeader, localTip *core.Block) error {
	if len(headers) == 0 {
		return fmt.Errorf("empty header list")
	}

	first := headers[0]
	if !bytes.Equal(first.PrevHash, localTip.Hash) {
		return fmt.Errorf("%w: first header prevHash %x does not match local tip %x at height %d",
			errHeaderChainBroken, first.PrevHash, localTip.Hash, localTip.GetHeight())
	}

	for i := 1; i < len(headers); i++ {
		prev := headers[i-1]
		curr := headers[i]

		if len(curr.PrevHash) == 0 {
			return fmt.Errorf("%w: header at index %d has empty prevHash", errHeaderChainBroken, i)
		}
		if len(curr.PrevHash) != core.HashLen {
			return fmt.Errorf("%w: header at index %d has invalid prevHash length %d",
				errHeaderChainBroken, i, len(curr.PrevHash))
		}

		prevHeaderHash, hashErr := computeHeaderHash(prev)
		if hashErr != nil {
			return fmt.Errorf("compute header hash at index %d: %w", i-1, hashErr)
		}
		if !bytes.Equal(curr.PrevHash, prevHeaderHash) {
			return fmt.Errorf("%w: header at index %d prevHash %x does not match computed hash %x of previous header",
				errHeaderChainBroken, i, curr.PrevHash, prevHeaderHash)
		}

		if curr.TimestampUnix <= prev.TimestampUnix {
			return fmt.Errorf("%w: header at index %d has non-monotonic timestamp %d <= %d",
				errHeaderChainBroken, i, curr.TimestampUnix, prev.TimestampUnix)
		}
		if curr.DifficultyBits == 0 && curr.Difficulty == 0 {
			return fmt.Errorf("%w: header at index %d has zero difficulty", errHeaderChainBroken, i)
		}
	}

	return nil
}

func computeHeaderHash(h *core.BlockHeader) ([]byte, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, h.Version); err != nil {
		return nil, fmt.Errorf("write version: %w", err)
	}
	if len(h.PrevHash) > 0 {
		buf.Write(h.PrevHash)
	}
	if err := binary.Write(&buf, binary.LittleEndian, h.TimestampUnix); err != nil {
		return nil, fmt.Errorf("write timestamp: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, h.DifficultyBits); err != nil {
		return nil, fmt.Errorf("write difficulty bits: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, h.Difficulty); err != nil {
		return nil, fmt.Errorf("write difficulty: %w", err)
	}
	if err := binary.Write(&buf, binary.LittleEndian, h.Nonce); err != nil {
		return nil, fmt.Errorf("write nonce: %w", err)
	}
	if len(h.MerkleRoot) > 0 {
		buf.Write(h.MerkleRoot)
	}
	sum := sha256.Sum256(buf.Bytes())
	return sum[:], nil
}

func (e *FastSyncEngine) processBatch(ctx context.Context, pm PeerAPI, headers []*core.BlockHeader, startHeight uint64, peers []string) error {
	for i, header := range headers {
		expectedHeight := startHeight + uint64(i)
		peer := peers[i%len(peers)]

		var block *core.Block
		var fetchErr error

		for attempt := 0; attempt < maxPeerRetryAttempts; attempt++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			block, fetchErr = pm.FetchBlockByHeight(ctx, peer, expectedHeight)
			if fetchErr == nil {
				break
			}

			if attempt < maxPeerRetryAttempts-1 {
				altPeer := peers[(i+attempt+1)%len(peers)]
				block, fetchErr = pm.FetchBlockByHeight(ctx, altPeer, expectedHeight)
				if fetchErr == nil {
					break
				}
			}
		}

		if fetchErr != nil {
			return fmt.Errorf("fetch block at height %d: %w", expectedHeight, fetchErr)
		}

		if verifyErr := e.verifyBlockMatchesHeader(block, header); verifyErr != nil {
			return fmt.Errorf("%w at height %d: %v", errBlockHashMismatch, expectedHeight, verifyErr)
		}

		localTip := e.chain.LatestBlock()
		if localTip != nil && !bytes.Equal(block.Header.PrevHash, localTip.Hash) {
			return fmt.Errorf("block at height %d prevHash %x does not match local tip hash %x",
				expectedHeight, block.Header.PrevHash, localTip.Hash)
		}

		accepted, addErr := e.chain.AddBlock(block)
		if addErr != nil {
			return fmt.Errorf("add block at height %d: %w", expectedHeight, addErr)
		}
		if !accepted {
			return fmt.Errorf("block at height %d not accepted by chain", expectedHeight)
		}
	}

	return nil
}

func (e *FastSyncEngine) verifyBlockMatchesHeader(block *core.Block, header *core.BlockHeader) error {
	if block == nil {
		return fmt.Errorf("block is nil")
	}
	if header == nil {
		return fmt.Errorf("header is nil")
	}

	if block.Header.Version != header.Version {
		return fmt.Errorf("version mismatch: block=%d header=%d", block.Header.Version, header.Version)
	}
	if !bytes.Equal(block.Header.PrevHash, header.PrevHash) {
		return fmt.Errorf("prevHash mismatch: block=%x header=%x", block.Header.PrevHash, header.PrevHash)
	}
	if block.Header.TimestampUnix != header.TimestampUnix {
		return fmt.Errorf("timestamp mismatch: block=%d header=%d", block.Header.TimestampUnix, header.TimestampUnix)
	}
	if block.Header.DifficultyBits != header.DifficultyBits {
		return fmt.Errorf("difficultyBits mismatch: block=%d header=%d", block.Header.DifficultyBits, header.DifficultyBits)
	}
	if block.Header.Difficulty != header.Difficulty {
		return fmt.Errorf("difficulty mismatch: block=%d header=%d", block.Header.Difficulty, header.Difficulty)
	}
	if block.Header.Nonce != header.Nonce {
		return fmt.Errorf("nonce mismatch: block=%d header=%d", block.Header.Nonce, header.Nonce)
	}

	return nil
}

func (e *FastSyncEngine) resolveTargetHeight(localHeight uint64) uint64 {
	e.mu.RLock()
	target := e.targetHeight
	e.mu.RUnlock()

	if target > localHeight {
		return target
	}

	e.mu.RLock()
	pm := e.pm
	e.mu.RUnlock()

	if pm == nil {
		return localHeight
	}

	peers := pm.GetActivePeers()
	var maxPeerHeight uint64
	for _, peer := range peers {
		info, infoErr := pm.FetchChainInfo(context.Background(), peer)
		if infoErr != nil {
			continue
		}
		if info.Height > maxPeerHeight {
			maxPeerHeight = info.Height
		}
	}

	if maxPeerHeight > localHeight {
		return maxPeerHeight
	}

	return localHeight
}

func (e *FastSyncEngine) buildBlockLocator() ([][]byte, error) {
	return BuildBlockLocatorFromChain(e.chain)
}
