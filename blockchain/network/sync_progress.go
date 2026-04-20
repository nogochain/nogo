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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package network

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	SyncProgressFileName = "sync_progress.json"
	SyncProgressVersion  = 1
	MaxProgressAge       = 24 * time.Hour
	SaveInterval         = 30 * time.Second
)

type SyncProgressState struct {
	Version           int       `json:"version"`
	LastSyncedHeight  uint64    `json:"last_synced_height"`
	TargetHeight      uint64    `json:"target_height"`
	LastBlockHash     string    `json:"last_block_hash"`
	LastBlockPrevHash string    `json:"last_block_prev_hash"`
	SyncPeerID        string    `json:"sync_peer_id"`
	StartTime         time.Time `json:"start_time"`
	LastUpdateTime    time.Time `json:"last_update_time"`
	IsComplete        bool      `json:"is_complete"`
	ErrorMessage      string    `json:"error_message,omitempty"`
	RetryCount        int       `json:"retry_count"`
	BlocksPerSecond   float64   `json:"blocks_per_second"`
	EstimatedTimeLeft int64     `json:"estimated_time_left_seconds"`
}

type SyncProgressStore struct {
	mu          sync.RWMutex
	filePath    string
	progress    *SyncProgressState
	lastSave    time.Time
	dirty       bool
	saveTicker  *time.Ticker
	stopChan    chan struct{}
	autoSave    bool
}

func NewSyncProgressStore(dataDir string) (*SyncProgressStore, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("data directory cannot be empty")
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	filePath := filepath.Join(dataDir, SyncProgressFileName)

	store := &SyncProgressStore{
		filePath: filePath,
		stopChan: make(chan struct{}),
		autoSave: true,
	}

	if err := store.load(); err != nil {
		store.progress = &SyncProgressState{
			Version: SyncProgressVersion,
		}
	}

	return store, nil
}

func (s *SyncProgressStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read progress file: %w", err)
	}

	var progress SyncProgressState
	if err := json.Unmarshal(data, &progress); err != nil {
		return fmt.Errorf("unmarshal progress: %w", err)
	}

	if progress.Version != SyncProgressVersion {
		return fmt.Errorf("incompatible progress version: got %d, expected %d",
			progress.Version, SyncProgressVersion)
	}

	s.progress = &progress
	return nil
}

func (s *SyncProgressStore) save() error {
	if s.progress == nil {
		return nil
	}

	s.progress.LastUpdateTime = time.Now()

	data, err := json.MarshalIndent(s.progress, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal progress: %w", err)
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write progress file: %w", err)
	}

	if err := os.Rename(tmpPath, s.filePath); err != nil {
		return fmt.Errorf("rename progress file: %w", err)
	}

	s.lastSave = time.Now()
	s.dirty = false
	return nil
}

func (s *SyncProgressStore) StartAutoSave() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.saveTicker != nil {
		return
	}

	s.saveTicker = time.NewTicker(SaveInterval)
	go func() {
		for {
			select {
			case <-s.saveTicker.C:
				s.mu.Lock()
				if s.dirty && s.autoSave {
					if err := s.save(); err != nil {
						fmt.Printf("[SyncProgress] Auto-save failed: %v\n", err)
					}
				}
				s.mu.Unlock()
			case <-s.stopChan:
				return
			}
		}
	}()
}

func (s *SyncProgressStore) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.saveTicker != nil {
		s.saveTicker.Stop()
		s.saveTicker = nil
	}

	close(s.stopChan)

	if s.dirty {
		if err := s.save(); err != nil {
			fmt.Printf("[SyncProgress] Final save failed: %v\n", err)
		}
	}
}

func (s *SyncProgressStore) UpdateProgress(height uint64, blockHash, prevHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.progress == nil {
		s.progress = &SyncProgressState{
			Version: SyncProgressVersion,
		}
	}

	s.progress.LastSyncedHeight = height
	s.progress.LastBlockHash = blockHash
	s.progress.LastBlockPrevHash = prevHash
	s.progress.LastUpdateTime = time.Now()
	s.progress.IsComplete = false
	s.progress.ErrorMessage = ""
	s.dirty = true

	return nil
}

func (s *SyncProgressStore) SetTarget(targetHeight uint64, peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.progress == nil {
		s.progress = &SyncProgressState{
			Version: SyncProgressVersion,
		}
	}

	s.progress.TargetHeight = targetHeight
	s.progress.SyncPeerID = peerID
	s.progress.StartTime = time.Now()
	s.progress.LastUpdateTime = time.Now()
	s.progress.IsComplete = false
	s.progress.ErrorMessage = ""
	s.progress.RetryCount = 0
	s.dirty = true

	return nil
}

func (s *SyncProgressStore) MarkComplete() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.progress == nil {
		return nil
	}

	s.progress.IsComplete = true
	s.progress.LastUpdateTime = time.Now()
	s.progress.ErrorMessage = ""
	s.dirty = true

	return s.save()
}

func (s *SyncProgressStore) SetError(errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.progress == nil {
		s.progress = &SyncProgressState{
			Version: SyncProgressVersion,
		}
	}

	s.progress.ErrorMessage = errMsg
	s.progress.LastUpdateTime = time.Now()
	s.progress.RetryCount++
	s.dirty = true

	return s.save()
}

func (s *SyncProgressStore) UpdateStats(blocksPerSecond float64, estimatedSeconds int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.progress == nil {
		return nil
	}

	s.progress.BlocksPerSecond = blocksPerSecond
	s.progress.EstimatedTimeLeft = estimatedSeconds
	s.dirty = true

	return nil
}

func (s *SyncProgressStore) GetProgress() *SyncProgressState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progress == nil {
		return nil
	}

	copy := *s.progress
	return &copy
}

func (s *SyncProgressStore) CanResume() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progress == nil {
		return false
	}

	if s.progress.IsComplete {
		return false
	}

	if s.progress.LastSyncedHeight == 0 {
		return false
	}

	if s.progress.TargetHeight == 0 {
		return false
	}

	if s.progress.LastSyncedHeight >= s.progress.TargetHeight {
		return false
	}

	age := time.Since(s.progress.LastUpdateTime)
	if age > MaxProgressAge {
		return false
	}

	return true
}

func (s *SyncProgressStore) GetResumePoint() (height uint64, targetHeight uint64, peerID string, canResume bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progress == nil {
		return 0, 0, "", false
	}

	canResume = s.CanResume()
	if canResume {
		return s.progress.LastSyncedHeight, s.progress.TargetHeight, s.progress.SyncPeerID, true
	}

	return 0, 0, "", false
}

func (s *SyncProgressStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.progress = &SyncProgressState{
		Version: SyncProgressVersion,
	}
	s.dirty = true

	if err := os.Remove(s.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove progress file: %w", err)
	}

	return nil
}

func (s *SyncProgressStore) ForceSave() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.dirty {
		return nil
	}

	return s.save()
}

func (s *SyncProgressStore) GetRemainingBlocks() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progress == nil || s.progress.TargetHeight <= s.progress.LastSyncedHeight {
		return 0
	}

	return s.progress.TargetHeight - s.progress.LastSyncedHeight
}

func (s *SyncProgressStore) GetProgressPercent() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progress == nil || s.progress.TargetHeight == 0 {
		return 0
	}

	if s.progress.LastSyncedHeight >= s.progress.TargetHeight {
		return 100
	}

	return float64(s.progress.LastSyncedHeight) / float64(s.progress.TargetHeight) * 100
}

func (s *SyncProgressStore) GetElapsedTime() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progress == nil || s.progress.StartTime.IsZero() {
		return 0
	}

	return time.Since(s.progress.StartTime)
}

func (s *SyncProgressStore) GetEstimatedTimeRemaining() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progress == nil || s.progress.EstimatedTimeLeft <= 0 {
		return 0
	}

	return time.Duration(s.progress.EstimatedTimeLeft) * time.Second
}

func (s *SyncProgressStore) IsComplete() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progress == nil {
		return false
	}

	return s.progress.IsComplete
}

func (s *SyncProgressStore) GetRetryCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progress == nil {
		return 0
	}

	return s.progress.RetryCount
}

func (s *SyncProgressStore) SetAutoSave(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoSave = enabled
}

func (s *SyncProgressStore) GetLastBlockHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progress == nil {
		return ""
	}

	return s.progress.LastBlockHash
}
