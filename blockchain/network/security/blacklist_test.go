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
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "blacklist-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func newTestBlacklist(t *testing.T) *Blacklist {
	t.Helper()
	bl, err := NewBlacklist(tempDir(t))
	require.NoError(t, err)
	return bl
}

func TestNewBlacklist(t *testing.T) {
	dir := tempDir(t)
	bl, err := NewBlacklist(dir)
	require.NoError(t, err)
	require.NotNil(t, bl)
	assert.Equal(t, 0, bl.Len())
	assert.FileExists(t, filepath.Join(dir, "blacklist.json"))
}

func TestNewBlacklistEmptyDir(t *testing.T) {
	_, err := NewBlacklist("")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "data directory is empty")
}

func TestNewBlacklistCreatesDir(t *testing.T) {
	dir := filepath.Join(tempDir(t), "nested", "subdir")
	bl, err := NewBlacklist(dir)
	require.NoError(t, err)
	require.NotNil(t, bl)
	assert.DirExists(t, dir)
}

func TestAddBasic(t *testing.T) {
	bl := newTestBlacklist(t)

	err := bl.Add("192.168.1.100", "brute force attack", 0)
	require.NoError(t, err)
	assert.Equal(t, 1, bl.Len())

	entry, found := bl.IsBlacklisted("192.168.1.100")
	assert.True(t, found)
	assert.Equal(t, "192.168.1.100", entry.IP)
	assert.Equal(t, "brute force attack", entry.Reason)
	assert.False(t, entry.AddedAt.IsZero())
	assert.True(t, entry.ExpiresAt.After(time.Now()))
}

func TestAddCustomTTL(t *testing.T) {
	bl := newTestBlacklist(t)

	customTTL := 2 * time.Hour
	err := bl.Add("10.0.0.1", "spam", customTTL)
	require.NoError(t, err)

	entry, found := bl.IsBlacklisted("10.0.0.1")
	require.True(t, found)
	expectedExpiry := time.Now().Add(customTTL)
	diff := entry.ExpiresAt.Sub(expectedExpiry)
	assert.Less(t, diff.Abs(), 10*time.Second)
}

func TestAddDefaultTTLWhenZero(t *testing.T) {
	bl := newTestBlacklist(t)

	err := bl.Add("172.16.0.1", "suspicious activity", 0)
	require.NoError(t, err)

	entry, _ := bl.IsBlacklisted("172.16.0.1")
	require.NotNil(t, entry)
	expectedExpiry := time.Now().Add(defaultTTL)
	diff := entry.ExpiresAt.Sub(expectedExpiry)
	assert.Less(t, diff.Abs(), 10*time.Second)
}

func TestAddInvalidIP(t *testing.T) {
	bl := newTestBlacklist(t)

	err := bl.Add("", "test", time.Hour)
	assert.ErrorIs(t, err, ErrInvalidIP)
	assert.Contains(t, err.Error(), "IP is empty")
	assert.Equal(t, 0, bl.Len())
}

func TestAddEmptyReason(t *testing.T) {
	bl := newTestBlacklist(t)

	err := bl.Add("1.2.3.4", "", time.Hour)
	assert.ErrorIs(t, err, ErrEmptyReason)
	assert.Equal(t, 0, bl.Len())
}

func TestAddNegativeTTL(t *testing.T) {
	bl := newTestBlacklist(t)

	err := bl.Add("1.2.3.4", "test", -1*time.Hour)
	assert.ErrorIs(t, err, ErrNegativeTTL)
	assert.Equal(t, 0, bl.Len())
}

func TestIsBlacklistedNotFound(t *testing.T) {
	bl := newTestBlacklist(t)

	entry, found := bl.IsBlacklisted("99.99.99.99")
	assert.False(t, found)
	assert.Nil(t, entry)
}

func TestIsBlacklistedExpiredEntry(t *testing.T) {
	bl := newTestBlacklist(t)

	err := bl.Add("8.8.8.8", "expired test", 1*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	entry, found := bl.IsBlacklisted("8.8.8.8")
	assert.False(t, found)
	assert.Nil(t, entry)
	assert.Equal(t, 0, bl.Len())
}

func TestIsBlacklistedExactExpiry(t *testing.T) {
	bl := newTestBlacklist(t)

	now := time.Now().Add(time.Second)

	bl.mu.Lock()
	bl.entries["exact-expiry"] = &BlacklistEntry{
		IP:        "exact-expiry",
		Reason:    "boundary test",
		AddedAt:   now,
		ExpiresAt: now,
	}
	bl.mu.Unlock()

	entry, found := bl.IsBlacklisted("exact-expiry")
	assert.True(t, found, "entry at exact expiry should still be considered blacklisted")
	assert.Equal(t, "boundary test", entry.Reason)
}

func TestAddOverwriteExisting(t *testing.T) {
	bl := newTestBlacklist(t)

	err := bl.Add("1.1.1.1", "first reason", time.Hour)
	require.NoError(t, err)

	err = bl.Add("1.1.1.1", "updated reason", 2*time.Hour)
	require.NoError(t, err)

	assert.Equal(t, 1, bl.Len())

	entry, found := bl.IsBlacklisted("1.1.1.1")
	require.True(t, found)
	assert.Equal(t, "updated reason", entry.Reason)
}

func TestCleanupExpired(t *testing.T) {
	bl := newTestBlacklist(t)

	_ = bl.Add("active-1", "still active", time.Hour)
	_ = bl.Add("active-2", "also active", 2*time.Hour)
	_ = bl.Add("expired-1", "gone", 1*time.Millisecond)
	_ = bl.Add("expired-2", "also gone", 1*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	count := bl.CleanupExpired()
	assert.Equal(t, 2, count)
	assert.Equal(t, 2, bl.Len())

	_, found1 := bl.IsBlacklisted("active-1")
	_, found2 := bl.IsBlacklisted("active-2")
	assert.True(t, found1)
	assert.True(t, found2)

	_, found3 := bl.IsBlacklisted("expired-1")
	_, found4 := bl.IsBlacklisted("expired-2")
	assert.False(t, found3)
	assert.False(t, found4)
}

func TestCleanupExpiredNone(t *testing.T) {
	bl := newTestBlacklist(t)

	_ = bl.Add("all-active", "no expiry yet", time.Hour)

	count := bl.CleanupExpired()
	assert.Equal(t, 0, count)
	assert.Equal(t, 1, bl.Len())
}

func TestCleanupExpiredAll(t *testing.T) {
	bl := newTestBlacklist(t)

	_ = bl.Add("e1", "r1", 1*time.Millisecond)
	_ = bl.Add("e2", "r2", 1*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	count := bl.CleanupExpired()
	assert.Equal(t, 2, count)
	assert.Equal(t, 0, bl.Len())
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := tempDir(t)
	bl, err := NewBlacklist(dir)
	require.NoError(t, err)

	entries := []struct {
		ip     string
		reason string
		ttl    time.Duration
	}{
		{"192.168.1.1", "attack A", time.Hour},
		{"10.0.0.42", "attack B", 2 * time.Hour},
		{"172.16.100.1", "attack C", 30 * time.Minute},
	}

	for _, e := range entries {
		err := bl.Add(e.ip, e.reason, e.ttl)
		require.NoError(t, err)
	}

	bl2, err := NewBlacklist(dir)
	require.NoError(t, err)

	for _, e := range entries {
		entry, found := bl2.IsBlacklisted(e.ip)
		assert.True(t, found, "should find %s after reload", e.ip)
		assert.Equal(t, e.reason, entry.Reason)
	}
	assert.Equal(t, len(entries), bl2.Len())
}

func TestSaveLoadFiltersExpired(t *testing.T) {
	dir := tempDir(t)
	bl, err := NewBlacklist(dir)
	require.NoError(t, err)

	_ = bl.Add("will-survive", "long TTL", time.Hour)
	_ = bl.Add("will-die", "short TTL", 1*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	bl2, err := NewBlacklist(dir)
	require.NoError(t, err)

	_, foundAlive := bl2.IsBlacklisted("will-survive")
	assert.True(t, foundAlive)

	_, foundDead := bl2.IsBlacklisted("will-die")
	assert.False(t, foundDead)
}

func TestLoadNonexistentFile(t *testing.T) {
	bl, err := NewBlacklist(tempDir(t))
	require.NoError(t, err)
	require.NotNil(t, bl)
	assert.Equal(t, 0, bl.Len())
}

func TestLoadEmptyFile(t *testing.T) {
	dir := tempDir(t)
	filePath := filepath.Join(dir, "blacklist.json")

	err := os.WriteFile(filePath, []byte{}, 0o644)
	require.NoError(t, err)

	bl, err := NewBlacklist(dir)
	require.NoError(t, err)
	assert.Equal(t, 0, bl.Len())
}

func TestLoadCorruptJSON(t *testing.T) {
	dir := tempDir(t)
	filePath := filepath.Join(dir, "blacklist.json")

	err := os.WriteFile(filePath, []byte("{invalid json"), 0o644)
	require.NoError(t, err)

	_, err = NewBlacklist(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse blacklist JSON")
}

func TestAtomicWrite(t *testing.T) {
	bl := newTestBlacklist(t)

	_ = bl.Add("atomic-test", "verify atomic write", time.Hour)

	data, err := os.ReadFile(bl.filePath)
	require.NoError(t, err)

	var parsed []BlacklistEntry
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, 1, len(parsed))
	assert.Equal(t, "atomic-test", parsed[0].IP)
	assert.NotContains(t, string(data), ".tmp")
}

func TestRemove(t *testing.T) {
	bl := newTestBlacklist(t)

	_ = bl.Add("to-remove", "temporary ban", time.Hour)
	assert.Equal(t, 1, bl.Len())

	removed := bl.Remove("to-remove")
	assert.True(t, removed)
	assert.Equal(t, 0, bl.Len())

	_, found := bl.IsBlacklisted("to-remove")
	assert.False(t, found)

	removedAgain := bl.Remove("to-remove")
	assert.False(t, removedAgain)
}

func TestRemoveNonexistent(t *testing.T) {
	bl := newTestBlacklist(t)

	removed := bl.Remove("nonexistent-ip")
	assert.False(t, removed)
}

func TestGetAll(t *testing.T) {
	bl := newTestBlacklist(t)

	ips := []string{"ip-a", "ip-b", "ip-c"}
	for i, ip := range ips {
		_ = bl.Add(ip, "reason"+string(rune('A'+i)), time.Hour)
	}

	all := bl.GetAll()
	assert.Equal(t, 3, len(all))

	ipSet := make(map[string]bool)
	for _, e := range all {
		ipSet[e.IP] = true
	}
	for _, ip := range ips {
		assert.True(t, ipSet[ip], " GetAll should contain %s", ip)
	}
}

func TestGetAllEmpty(t *testing.T) {
	bl := newTestBlacklist(t)
	all := bl.GetAll()
	assert.Equal(t, 0, len(all))
}

func TestStartAndStop(t *testing.T) {
	bl := newTestBlacklist(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = bl.Add("cleanup-target", "auto cleanup test", 1*time.Millisecond)

	bl.Start(ctx)
	defer bl.Stop()

	time.Sleep(50 * time.Millisecond)

	_, found := bl.IsBlacklisted("cleanup-target")
	assert.False(t, found, "background cleanup should have removed expired entry")
}

func TestStartContextCancel(t *testing.T) {
	bl := newTestBlacklist(t)

	ctx, cancel := context.WithCancel(context.Background())

	bl.Start(ctx)

	cancel()
	time.Sleep(50 * time.Millisecond)

	bl.Stop()
}

func TestConcurrentAdd(t *testing.T) {
	bl := newTestBlacklist(t)

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ip := fmt.Sprintf("10.0.%d.%d", idx/256, idx%256)
			if err := bl.Add(ip, "concurrent add", time.Hour); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent Add error: %v", err)
	}

	assert.Equal(t, 50, bl.Len())
}

func TestConcurrentReadWrite(t *testing.T) {
	bl := newTestBlacklist(t)

	for i := 0; i < 20; i++ {
		_ = bl.Add(fmt.Sprintf("192.168.1.%d", i), "init", time.Hour)
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				select {
				case <-ctx.Done():
					return
				default:
					ip := fmt.Sprintf("192.168.1.%d", idx)
					bl.IsBlacklisted(ip)
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestConcurrentAddAndCleanup(t *testing.T) {
	bl := newTestBlacklist(t)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = bl.Add(fmt.Sprintf("add-%d", idx), "race test", 1*time.Millisecond)
		}(i)
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bl.CleanupExpired()
		}()
	}

	wg.Wait()
}

func TestLargeScaleEntries(t *testing.T) {
	bl := newTestBlacklist(t)

	const total = 500
	for i := 0; i < total; i++ {
		ip := fmt.Sprintf("%d.%d.%d.%d", byte(i>>24&0xFF), byte(i>>16&0xFF), byte(i>>8&0xFF), byte(i&0xFF))
		bl.mu.Lock()
		bl.entries[ip] = &BlacklistEntry{
			IP:        ip,
			Reason:    fmt.Sprintf("reason-%d", i),
			AddedAt:   time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
		}
		bl.mu.Unlock()
	}

	err := bl.Save()
	require.NoError(t, err)

	assert.Equal(t, total, bl.Len())

	all := bl.GetAll()
	assert.Equal(t, total, len(all))

	bl2, err := NewBlacklist(filepath.Dir(bl.filePath))
	require.NoError(t, err)
	assert.Equal(t, total, bl2.Len())
}

func TestSpecialCharactersInReason(t *testing.T) {
	bl := newTestBlacklist(t)

	reasons := []string{
		"simple reason",
		"reason with spaces",
		"reason-with-dashes",
		"reason_with_underscores",
		"reason.with.dots",
		"中文原因测试",
		"emoji 🚫 test",
		"line\nbreak",
		"tab\there",
		`{"json":"like"}`,
	}

	for i, reason := range reasons {
		err := bl.Add(fmt.Sprintf("10.0.0.%d", i+1), reason, time.Hour)
		require.NoError(t, err)
	}

	bl2, err := NewBlacklist(filepath.Dir(bl.filePath))
	require.NoError(t, err)

	for i, expected := range reasons {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		entry, found := bl2.IsBlacklisted(ip)
		assert.True(t, found, "should find %s", ip)
		assert.Equal(t, expected, entry.Reason)
	}
}

func TestIPv6Address(t *testing.T) {
	bl := newTestBlacklist(t)

	ipv6Addr := "2001:0db8:85a3:0000:0000:8a2e:0370:7334"
	err := bl.Add(ipv6Addr, "ipv6 block", time.Hour)
	require.NoError(t, err)

	entry, found := bl.IsBlacklisted(ipv6Addr)
	assert.True(t, found)
	assert.Equal(t, ipv6Addr, entry.IP)
}

func TestPortInIPAddress(t *testing.T) {
	bl := newTestBlacklist(t)

	addrWithPort := "192.168.1.1:8333"
	err := bl.Add(addrWithPort, "p2p port ban", time.Hour)
	require.NoError(t, err)

	entry, found := bl.IsBlacklisted(addrWithPort)
	assert.True(t, found)
	assert.Equal(t, addrWithPort, entry.IP)

	differentPort := "192.168.1.1:8334"
	_, found2 := bl.IsBlacklisted(differentPort)
	assert.False(t, found2, "different port should be separate entry")
}

func TestRapidAddRemoveCycle(t *testing.T) {
	bl := newTestBlacklist(t)

	const cycles = 100
	for i := 0; i < cycles; i++ {
		ip := "cyclic-ip"
		err := bl.Add(ip, "cycle reason", time.Hour)
		require.NoError(t, err)

		removed := bl.Remove(ip)
		assert.True(t, removed)

		_, found := bl.IsBlacklisted(ip)
		assert.False(t, found)
	}

	assert.Equal(t, 0, bl.Len())
}

func TestMultipleSavesConsistency(t *testing.T) {
	bl := newTestBlacklist(t)

	for i := 0; i < 20; i++ {
		ip := fmt.Sprintf("save-consistency-%d", i)
		err := bl.Add(ip, "consistency test", time.Hour)
		require.NoError(t, err)

		err = bl.Save()
		require.NoError(t, err)
	}

	data, err := os.ReadFile(bl.filePath)
	require.NoError(t, err)

	var loaded []BlacklistEntry
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	assert.Equal(t, 20, len(loaded))

	bl2, err := NewBlacklist(filepath.Dir(bl.filePath))
	require.NoError(t, err)
	assert.Equal(t, 20, bl2.Len())
}

func TestBoundaryMaxInt64TTL(t *testing.T) {
	bl := newTestBlacklist(t)

	maxTTL := time.Duration(int64(^uint64(0)>>1)) * time.Nanosecond
	err := bl.Add("max-ttl", "max duration", maxTTL)
	require.NoError(t, err)

	entry, found := bl.IsBlacklisted("max-ttl")
	assert.True(t, found)
	assert.True(t, entry.ExpiresAt.After(time.Now().Add(maxTTL - time.Hour)))
}

func TestBoundaryNanoSecondTTL(t *testing.T) {
	bl := newTestBlacklist(t)

	err := bl.Add("nano-ttl", "nano second ttl", 1*time.Nanosecond)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	_, found := bl.IsBlacklisted("nano-ttl")
	assert.False(t, found, "nanosecond TTL should expire almost immediately")
}

func TestBoundaryVeryLongIPString(t *testing.T) {
	bl := newTestBlacklist(t)

	longIPBytes := make([]byte, 4096)
	for i := range longIPBytes {
		longIPBytes[i] = 'a' + byte(i%26)
	}
	longIP := string(longIPBytes)

	err := bl.Add(longIP, "very long IP", time.Hour)
	require.NoError(t, err)

	entry, found := bl.IsBlacklisted(longIP)
	assert.True(t, found)
	assert.Equal(t, longIP, entry.IP)
}

func TestBoundaryVeryLongReason(t *testing.T) {
	bl := newTestBlacklist(t)

	longReasonBytes := make([]byte, 8192)
	for i := range longReasonBytes {
		longReasonBytes[i] = 'x'
	}
	longReason := string(longReasonBytes)

	err := bl.Add("1.2.3.4", longReason, time.Hour)
	require.NoError(t, err)

	entry, found := bl.IsBlacklisted("1.2.3.4")
	assert.True(t, found)
	assert.Equal(t, longReason, entry.Reason)
}

func TestJSONFormatCompliance(t *testing.T) {
	bl := newTestBlacklist(t)

	baseTime := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	bl.mu.Lock()
	bl.entries["format-test"] = &BlacklistEntry{
		IP:        "1.2.3.4",
		Reason:    "format check",
		AddedAt:   baseTime,
		ExpiresAt: baseTime.Add(time.Hour),
	}
	bl.mu.Unlock()

	err := bl.Save()
	require.NoError(t, err)

	data, err := os.ReadFile(bl.filePath)
	require.NoError(t, err)

	var raw []map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)
	require.Equal(t, 1, len(raw))

	entry := raw[0]
	assert.Equal(t, "1.2.3.4", entry["ip"])
	assert.Equal(t, "format check", entry["reason"])
	assert.Equal(t, "2026-04-17T12:00:00Z", entry["added_at"])
	assert.Equal(t, "2026-04-17T13:00:00Z", entry["expires_at"])
}

func TestLenConsistency(t *testing.T) {
	bl := newTestBlacklist(t)

	assert.Equal(t, 0, bl.Len())

	for i := 0; i < 10; i++ {
		_ = bl.Add(fmt.Sprintf("len-test-%d", i), "r", time.Hour)
		assert.Equal(t, i+1, bl.Len())
	}

	for i := 0; i < 5; i++ {
		bl.Remove(fmt.Sprintf("len-test-%d", i))
		assert.Equal(t, 9-i, bl.Len())
	}
}
