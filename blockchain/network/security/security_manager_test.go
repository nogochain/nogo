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
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func smTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "security-manager-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func newTestSecurityManager(t *testing.T) *SecurityManager {
	t.Helper()
	sm, err := NewSecurityManager(smTempDir(t))
	require.NoError(t, err)
	require.NotNil(t, sm)
	return sm
}

func TestNewSecurityManager(t *testing.T) {
	dir := smTempDir(t)
	sm, err := NewSecurityManager(dir)
	require.NoError(t, err)
	require.NotNil(t, sm)
	assert.NotNil(t, sm.blacklist)
	assert.NotNil(t, sm.ipFilter)
	assert.NotNil(t, sm.banScores)
	assert.Equal(t, dir, sm.dataDir)
}

func TestNewSecurityManagerEmptyDir(t *testing.T) {
	_, err := NewSecurityManager("")
	assert.ErrorIs(t, err, ErrEmptyDataDir)
}

func TestNewSecurityManagerCreatesBlacklistFile(t *testing.T) {
	dir := smTempDir(t)
	sm, err := NewSecurityManager(dir)
	require.NoError(t, err)
	require.NotNil(t, sm)

	err = sm.blacklist.Add("1.2.3.4", "init", time.Hour)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "blacklist.json"))
}

func TestShouldAcceptConnection_Normal(t *testing.T) {
	sm := newTestSecurityManager(t)

	accepted, reason := sm.ShouldAcceptConnection("192.168.1.1:8333")
	assert.True(t, accepted)
	assert.Empty(t, reason)
}

func TestShouldAcceptConnection_Loopback(t *testing.T) {
	sm := newTestSecurityManager(t)

	accepted, reason := sm.ShouldAcceptConnection("127.0.0.1:8333")
	assert.True(t, accepted)
	assert.Empty(t, reason)
}

func TestShouldAcceptConnection_BareIP(t *testing.T) {
	sm := newTestSecurityManager(t)

	accepted, reason := sm.ShouldAcceptConnection("10.0.0.1")
	assert.True(t, accepted)
	assert.Empty(t, reason)
}

func TestShouldAcceptConnection_InvalidAddr(t *testing.T) {
	sm := newTestSecurityManager(t)

	accepted, reason := sm.ShouldAcceptConnection("not-an-address")
	assert.False(t, accepted)
	assert.Contains(t, reason, "invalid")
}

func TestShouldAcceptConnection_EmptyAddr(t *testing.T) {
	sm := newTestSecurityManager(t)

	accepted, reason := sm.ShouldAcceptConnection("")
	assert.False(t, accepted)
	assert.Contains(t, reason, "invalid")
}

func TestShouldAcceptConnection_IPFilterDeny(t *testing.T) {
	sm := newTestSecurityManager(t)

	err := sm.ipFilter.SetMode("blacklist")
	require.NoError(t, err)

	err = sm.ipFilter.ParseConfig([]byte(`[
		{"action":"deny","cidr":"10.0.0.0/8","reason":"private network"}
	]`))
	require.NoError(t, err)

	accepted, reason := sm.ShouldAcceptConnection("10.0.0.1:8333")
	assert.False(t, accepted)
	assert.Contains(t, reason, "denied by IP filter")
}

func TestShouldAcceptConnection_Blacklisted(t *testing.T) {
	sm := newTestSecurityManager(t)

	err := sm.blacklist.Add("192.168.1.100", "spam", time.Hour)
	require.NoError(t, err)

	accepted, reason := sm.ShouldAcceptConnection("192.168.1.100:8333")
	assert.False(t, accepted)
	assert.Contains(t, reason, "blacklisted")
	assert.Contains(t, reason, "spam")
}

func TestShouldAcceptConnection_IPFilterFirstThenBlacklist(t *testing.T) {
	sm := newTestSecurityManager(t)

	err := sm.ipFilter.SetMode("blacklist")
	require.NoError(t, err)

	err = sm.ipFilter.ParseConfig([]byte(`[
		{"action":"deny","cidr":"10.0.0.0/8","reason":"blocked range"}
	]`))
	require.NoError(t, err)

	err = sm.blacklist.Add("10.0.0.5", "blacklisted peer", time.Hour)
	require.NoError(t, err)

	accepted, reason := sm.ShouldAcceptConnection("10.0.0.5:8333")
	assert.False(t, accepted)
	assert.Contains(t, reason, "denied by IP filter")
}

func TestOnPeerMisbehavior_CreatesScore(t *testing.T) {
	sm := newTestSecurityManager(t)

	sm.OnPeerMisbehavior("peer-1", 10, 20)

	score := sm.GetBanScore("peer-1")
	require.NotNil(t, score)
	assert.True(t, score.Score() >= 30)
}

func TestOnPeerMisbehavior_AccumulatesScore(t *testing.T) {
	sm := newTestSecurityManager(t)

	sm.OnPeerMisbehavior("peer-1", 20, 0)
	sm.OnPeerMisbehavior("peer-1", 30, 0)

	score := sm.GetBanScore("peer-1")
	require.NotNil(t, score)
	assert.Equal(t, uint32(50), score.Score())
}

func TestOnPeerMisbehavior_TriggersBan(t *testing.T) {
	sm := newTestSecurityManager(t)

	var callbackInvoked atomic.Int32
	sm.SetOnBanCallback(func(peerID, ip, reason string) {
		callbackInvoked.Add(1)
	})

	sm.OnPeerMisbehavior("peer-1", BanThreshold, 0)

	assert.Equal(t, int32(1), callbackInvoked.Load())
}

func TestOnPeerMisbehavior_BelowThresholdNoBan(t *testing.T) {
	sm := newTestSecurityManager(t)

	var callbackInvoked atomic.Int32
	sm.SetOnBanCallback(func(peerID, ip, reason string) {
		callbackInvoked.Add(1)
	})

	sm.OnPeerMisbehavior("peer-1", BanThreshold-1, 0)

	assert.Equal(t, int32(0), callbackInvoked.Load())
}

func TestOnPeerMisbehavior_MultiplePeers(t *testing.T) {
	sm := newTestSecurityManager(t)

	sm.OnPeerMisbehavior("peer-a", 10, 0)
	sm.OnPeerMisbehavior("peer-b", 20, 0)

	scoreA := sm.GetBanScore("peer-a")
	scoreB := sm.GetBanScore("peer-b")

	require.NotNil(t, scoreA)
	require.NotNil(t, scoreB)
	assert.Equal(t, uint32(10), scoreA.Score())
	assert.Equal(t, uint32(20), scoreB.Score())
}

func TestOnPeerBanned_AddsToBlacklist(t *testing.T) {
	sm := newTestSecurityManager(t)

	sm.OnPeerBanned("peer-1", "192.168.1.50", "malicious behavior")

	entry, found := sm.blacklist.IsBlacklisted("192.168.1.50")
	assert.True(t, found)
	assert.Equal(t, "malicious behavior", entry.Reason)
}

func TestOnPeerBanned_EmptyIPNoBlacklist(t *testing.T) {
	sm := newTestSecurityManager(t)

	sm.OnPeerBanned("peer-1", "", "no IP provided")

	assert.Equal(t, 0, sm.blacklist.Len())
}

func TestOnPeerBanned_CallbackInvoked(t *testing.T) {
	sm := newTestSecurityManager(t)

	var capturedPeerID, capturedIP, capturedReason string
	sm.SetOnBanCallback(func(peerID, ip, reason string) {
		capturedPeerID = peerID
		capturedIP = ip
		capturedReason = reason
	})

	sm.OnPeerBanned("peer-1", "10.0.0.1", "DDoS attack")

	assert.Equal(t, "peer-1", capturedPeerID)
	assert.Equal(t, "10.0.0.1", capturedIP)
	assert.Equal(t, "DDoS attack", capturedReason)
}

func TestOnPeerBanned_CallbackNilSafe(t *testing.T) {
	sm := newTestSecurityManager(t)

	assert.NotPanics(t, func() {
		sm.OnPeerBanned("peer-1", "10.0.0.1", "test")
	})
}

func TestSetOnBanCallback(t *testing.T) {
	sm := newTestSecurityManager(t)

	callCount := 0
	sm.SetOnBanCallback(func(peerID, ip, reason string) {
		callCount++
	})

	sm.OnPeerBanned("peer-1", "1.2.3.4", "reason-1")
	sm.OnPeerBanned("peer-2", "5.6.7.8", "reason-2")

	assert.Equal(t, 2, callCount)
}

func TestSetOnBanCallback_Replace(t *testing.T) {
	sm := newTestSecurityManager(t)

	var firstCalled, secondCalled bool

	sm.SetOnBanCallback(func(peerID, ip, reason string) {
		firstCalled = true
	})

	sm.OnPeerBanned("peer-1", "1.2.3.4", "first")
	assert.True(t, firstCalled)

	sm.SetOnBanCallback(func(peerID, ip, reason string) {
		secondCalled = true
	})

	sm.OnPeerBanned("peer-2", "5.6.7.8", "second")
	assert.True(t, secondCalled)
}

func TestGetBanScore_Nonexistent(t *testing.T) {
	sm := newTestSecurityManager(t)

	score := sm.GetBanScore("nonexistent")
	assert.Nil(t, score)
}

func TestGetBanScore_Existing(t *testing.T) {
	sm := newTestSecurityManager(t)

	sm.OnPeerMisbehavior("peer-1", 42, 0)

	score := sm.GetBanScore("peer-1")
	require.NotNil(t, score)
	assert.Equal(t, uint32(42), score.Score())
}

func TestRemovePeer(t *testing.T) {
	sm := newTestSecurityManager(t)

	sm.OnPeerMisbehavior("peer-1", 10, 0)
	require.NotNil(t, sm.GetBanScore("peer-1"))

	sm.RemovePeer("peer-1")
	assert.Nil(t, sm.GetBanScore("peer-1"))
}

func TestRemovePeer_Nonexistent(t *testing.T) {
	sm := newTestSecurityManager(t)

	assert.NotPanics(t, func() {
		sm.RemovePeer("nonexistent")
	})
}

func TestSecurityManagerStartAndStop(t *testing.T) {
	sm := newTestSecurityManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := sm.Start(ctx)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	sm.Stop()
}

func TestStart_CancelContext(t *testing.T) {
	sm := newTestSecurityManager(t)

	ctx, cancel := context.WithCancel(context.Background())

	err := sm.Start(ctx)
	require.NoError(t, err)

	cancel()
	time.Sleep(50 * time.Millisecond)

	sm.Stop()
}

func TestStop_Idempotent(t *testing.T) {
	sm := newTestSecurityManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := sm.Start(ctx)
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		sm.Stop()
		sm.Stop()
	})
}

func TestLoadIPFilterConfig_FromEnv(t *testing.T) {
	dir := smTempDir(t)

	t.Setenv(envIPFilterConfig, `[{"action":"deny","cidr":"203.0.113.0/24","reason":"test range"}]`)

	sm, err := NewSecurityManager(dir)
	require.NoError(t, err)

	parsedIP := net.ParseIP("203.0.113.5")
	require.NotNil(t, parsedIP)
	assert.False(t, sm.ipFilter.Allow(parsedIP))
}

func TestLoadIPFilterConfig_EmptyEnv(t *testing.T) {
	dir := smTempDir(t)

	t.Setenv(envIPFilterConfig, "")

	sm, err := NewSecurityManager(dir)
	require.NoError(t, err)

	parsedIP := net.ParseIP("10.0.0.1")
	require.NotNil(t, parsedIP)
	assert.True(t, sm.ipFilter.Allow(parsedIP))
}

func TestLoadIPFilterConfig_InvalidJSON(t *testing.T) {
	dir := smTempDir(t)

	t.Setenv(envIPFilterConfig, `{invalid json}`)

	sm, err := NewSecurityManager(dir)
	require.NoError(t, err)
	require.NotNil(t, sm)
}

func TestExtractIPFromAddr_HostPort(t *testing.T) {
	assert.Equal(t, "192.168.1.1", extractIPFromAddr("192.168.1.1:8333"))
}

func TestExtractIPFromAddr_BareIPv4(t *testing.T) {
	assert.Equal(t, "10.0.0.1", extractIPFromAddr("10.0.0.1"))
}

func TestExtractIPFromAddr_BareIPv6(t *testing.T) {
	assert.Equal(t, "2001:db8::1", extractIPFromAddr("2001:db8::1"))
}

func TestExtractIPFromAddr_Invalid(t *testing.T) {
	assert.Equal(t, "", extractIPFromAddr("not-an-ip"))
}

func TestExtractIPFromAddr_Empty(t *testing.T) {
	assert.Equal(t, "", extractIPFromAddr(""))
}

func TestIntegration_MisbehaviorToBan(t *testing.T) {
	sm := newTestSecurityManager(t)

	var bannedPeer, bannedReason string
	var bannedIP string
	sm.SetOnBanCallback(func(peerID, ip, reason string) {
		bannedPeer = peerID
		bannedIP = ip
		bannedReason = reason
	})

	sm.OnPeerMisbehavior("peer-1", BanThreshold-1, 0)
	assert.Equal(t, "", bannedPeer)

	sm.OnPeerMisbehavior("peer-1", 1, 0)

	assert.Equal(t, "peer-1", bannedPeer)
	assert.Equal(t, "", bannedIP)
	assert.Contains(t, bannedReason, "threshold")
}

func TestIntegration_BannedPeerBlacklisted(t *testing.T) {
	sm := newTestSecurityManager(t)

	sm.OnPeerBanned("peer-1", "192.168.1.200", "protocol violation")

	accepted, reason := sm.ShouldAcceptConnection("192.168.1.200:8333")
	assert.False(t, accepted)
	assert.Contains(t, reason, "blacklisted")
	assert.Contains(t, reason, "protocol violation")
}

func TestIntegration_FullFlow(t *testing.T) {
	sm := newTestSecurityManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := sm.Start(ctx)
	require.NoError(t, err)
	defer sm.Stop()

	accepted, _ := sm.ShouldAcceptConnection("10.0.0.1:8333")
	assert.True(t, accepted)

	sm.OnPeerMisbehavior("peer-1", 50, 0)
	assert.False(t, sm.GetBanScore("peer-1").IsBanned())

	sm.OnPeerMisbehavior("peer-1", 50, 0)
	assert.True(t, sm.GetBanScore("peer-1").IsBanned())

	sm.RemovePeer("peer-1")
	assert.Nil(t, sm.GetBanScore("peer-1"))
}

func TestConcurrent_OnPeerMisbehavior(t *testing.T) {
	sm := newTestSecurityManager(t)

	var wg sync.WaitGroup
	const goroutines = 50
	const increments = 20

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < increments; j++ {
				sm.OnPeerMisbehavior("peer-1", 1, 0)
			}
		}()
	}
	wg.Wait()

	score := sm.GetBanScore("peer-1")
	require.NotNil(t, score)
	expectedTotal := uint32(goroutines * increments)
	assert.Equal(t, expectedTotal, score.Score())
}

func TestConcurrent_ShouldAcceptConnection(t *testing.T) {
	sm := newTestSecurityManager(t)

	err := sm.blacklist.Add("10.0.0.1", "banned", time.Hour)
	require.NoError(t, err)

	var wg sync.WaitGroup
	const readers = 50

	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			accepted, _ := sm.ShouldAcceptConnection("10.0.0.1:8333")
			assert.False(t, accepted)
		}()
	}
	wg.Wait()
}

func TestConcurrent_MixedOperations(t *testing.T) {
	sm := newTestSecurityManager(t)

	var wg sync.WaitGroup
	const workers = 30
	const iterations = 50

	var banCallbackCount atomic.Int32
	sm.SetOnBanCallback(func(peerID, ip, reason string) {
		banCallbackCount.Add(1)
	})

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			peerID := "peer-mixed"
			for j := 0; j < iterations; j++ {
				switch id % 4 {
				case 0:
					sm.OnPeerMisbehavior(peerID, 1, 0)
				case 1:
					sm.GetBanScore(peerID)
				case 2:
					sm.ShouldAcceptConnection("192.168.1.1:8333")
				case 3:
					if j == iterations/2 {
						sm.RemovePeer(peerID)
					}
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_SetCallbackAndBan(t *testing.T) {
	sm := newTestSecurityManager(t)

	var wg sync.WaitGroup
	const workers = 20

	var callbackCount atomic.Int32

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				sm.SetOnBanCallback(func(peerID, ip, reason string) {
					callbackCount.Add(1)
				})
			} else {
				sm.OnPeerBanned("peer-1", "", "test")
			}
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_RemoveAndAccess(t *testing.T) {
	sm := newTestSecurityManager(t)

	sm.OnPeerMisbehavior("peer-1", 10, 0)

	var wg sync.WaitGroup
	const workers = 20

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			if id%3 == 0 {
				sm.RemovePeer("peer-1")
			} else if id%3 == 1 {
				sm.GetBanScore("peer-1")
			} else {
				sm.OnPeerMisbehavior("peer-1", 1, 0)
			}
		}(i)
	}
	wg.Wait()
}

func TestShouldAcceptConnection_IPv6WithPort(t *testing.T) {
	sm := newTestSecurityManager(t)

	accepted, reason := sm.ShouldAcceptConnection("[::1]:8333")
	assert.True(t, accepted)
	assert.Empty(t, reason)
}

func TestShouldAcceptConnection_IPv6Bare(t *testing.T) {
	sm := newTestSecurityManager(t)

	accepted, reason := sm.ShouldAcceptConnection("::1")
	assert.True(t, accepted)
	assert.Empty(t, reason)
}

func TestOnPeerBanned_DefaultTTL(t *testing.T) {
	sm := newTestSecurityManager(t)

	sm.OnPeerBanned("peer-1", "192.168.1.1", "test ban")

	entry, found := sm.blacklist.IsBlacklisted("192.168.1.1")
	require.True(t, found)

	expectedExpiry := time.Now().Add(defaultBanTTL)
	diff := entry.ExpiresAt.Sub(expectedExpiry)
	assert.Less(t, diff.Abs(), 10*time.Second)
}

func TestMultipleStartStop(t *testing.T) {
	sm := newTestSecurityManager(t)

	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		err := sm.Start(ctx)
		require.NoError(t, err)
		time.Sleep(20 * time.Millisecond)
		sm.Stop()
		cancel()
	}
}
