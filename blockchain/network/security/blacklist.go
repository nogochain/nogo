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

package security

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	defaultCleanupInterval = time.Hour
	defaultTTL             = 24 * time.Hour
)

var (
	ErrInvalidIP     = fmt.Errorf("invalid IP address")
	ErrEmptyReason   = fmt.Errorf("reason cannot be empty")
	ErrNegativeTTL   = fmt.Errorf("TTL must be non-negative")
	ErrBlacklistFile = fmt.Errorf("blacklist file error")
)

type BlacklistEntry struct {
	IP        string    `json:"ip"`
	Reason    string    `json:"reason"`
	AddedAt   time.Time `json:"added_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Blacklist struct {
	entries    map[string]*BlacklistEntry
	filePath   string
	mu         sync.RWMutex
	saveMu     sync.Mutex
	defaultTTL time.Duration
}

func NewBlacklist(dataDir string) (*Blacklist, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("%w: data directory is empty", ErrBlacklistFile)
	}

	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve absolute path: %w", ErrBlacklistFile, err)
	}

	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: create data directory: %w", ErrBlacklistFile, err)
	}

	bl := &Blacklist{
		entries:    make(map[string]*BlacklistEntry),
		filePath:   filepath.Join(absDir, "blacklist.json"),
		defaultTTL: defaultTTL,
	}

	if err := bl.Load(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBlacklistFile, err)
	}

	if _, err := os.Stat(bl.filePath); os.IsNotExist(err) {
		if saveErr := bl.Save(); saveErr != nil {
			return nil, fmt.Errorf("%w: create initial blacklist file: %w", ErrBlacklistFile, saveErr)
		}
	}

	return bl, nil
}

func (bl *Blacklist) Add(ip, reason string, ttl time.Duration) error {
	if ip == "" {
		return fmt.Errorf("%w: IP is empty", ErrInvalidIP)
	}
	if reason == "" {
		return fmt.Errorf("%w", ErrEmptyReason)
	}
	if ttl < 0 {
		return fmt.Errorf("%w: got %v", ErrNegativeTTL, ttl)
	}
	if ttl == 0 {
		ttl = bl.defaultTTL
	}

	now := time.Now()
	entry := &BlacklistEntry{
		IP:        ip,
		Reason:    reason,
		AddedAt:   now,
		ExpiresAt: now.Add(ttl),
	}

	bl.mu.Lock()
	bl.entries[ip] = entry
	bl.mu.Unlock()

	return bl.Save()
}

func (bl *Blacklist) IsBlacklisted(ip string) (*BlacklistEntry, bool) {
	bl.mu.RLock()
	entry, exists := bl.entries[ip]
	bl.mu.RUnlock()

	if !exists {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		bl.mu.Lock()
		current, stillExists := bl.entries[ip]
		if !stillExists || time.Now().After(current.ExpiresAt) {
			delete(bl.entries, ip)
			bl.mu.Unlock()
			return nil, false
		}
		bl.mu.Unlock()
		return current, true
	}

	return entry, true
}

func (bl *Blacklist) Load() error {
	data, err := os.ReadFile(bl.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read blacklist file: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	var rawEntries []BlacklistEntry
	if err := json.Unmarshal(data, &rawEntries); err != nil {
		return fmt.Errorf("parse blacklist JSON: %w", err)
	}

	bl.mu.Lock()
	defer bl.mu.Unlock()

	bl.entries = make(map[string]*BlacklistEntry, len(rawEntries))
	now := time.Now()
	for i := range rawEntries {
		e := &rawEntries[i]
		if now.Before(e.ExpiresAt) || now.Equal(e.ExpiresAt) {
			bl.entries[e.IP] = e
		}
	}

	return nil
}

func (bl *Blacklist) Save() error {
	bl.saveMu.Lock()
	defer bl.saveMu.Unlock()

	bl.mu.RLock()
	entries := make([]BlacklistEntry, 0, len(bl.entries))
	for _, e := range bl.entries {
		entries = append(entries, *e)
	}
	bl.mu.RUnlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal blacklist: %w", err)
	}

	tmpPath := fmt.Sprintf("%s.tmp.%d", bl.filePath, time.Now().UnixNano())

	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	n, err := f.Write(data)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if n != len(data) {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("incomplete write: expected %d bytes, wrote %d", len(data), n)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, bl.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file to final: %w", err)
	}

	return nil
}

func (bl *Blacklist) CleanupExpired() int {
	now := time.Now()

	bl.mu.Lock()
	var expiredIPs []string
	for ip, entry := range bl.entries {
		if now.After(entry.ExpiresAt) {
			expiredIPs = append(expiredIPs, ip)
		}
	}
	for _, ip := range expiredIPs {
		delete(bl.entries, ip)
	}
	bl.mu.Unlock()

	count := len(expiredIPs)
	if count > 0 {
		if saveErr := bl.Save(); saveErr != nil {
			log.Printf("[Blacklist] failed to save after cleanup: %v", saveErr)
		}
	}

	return count
}

func (bl *Blacklist) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(defaultCleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				bl.CleanupExpired()
			}
		}
	}()
}

func (bl *Blacklist) Stop() {
	// Goroutine lifecycle is managed by context cancellation in Start()
}

func (bl *Blacklist) Len() int {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	return len(bl.entries)
}

func (bl *Blacklist) Remove(ip string) bool {
	bl.mu.Lock()
	_, existed := bl.entries[ip]
	delete(bl.entries, ip)
	bl.mu.Unlock()

	if existed {
		if saveErr := bl.Save(); saveErr != nil {
			log.Printf("[Blacklist] failed to save after removal: %v", saveErr)
		}
	}

	return existed
}

func (bl *Blacklist) GetAll() []*BlacklistEntry {
	bl.mu.RLock()
	result := make([]*BlacklistEntry, 0, len(bl.entries))
	for _, e := range bl.entries {
		result = append(result, e)
	}
	bl.mu.RUnlock()
	return result
}
