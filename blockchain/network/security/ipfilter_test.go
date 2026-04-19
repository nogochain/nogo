package security

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIPFilter(t *testing.T) {
	f := NewIPFilter()
	require.NotNil(t, f)
	assert.Equal(t, "blacklist", f.Mode())
	assert.Empty(t, f.Rules())
}

func TestParseConfig_ValidRules(t *testing.T) {
	f := NewIPFilter()

	jsonRules := []byte(`[
		{"action": "deny", "cidr": "192.168.1.0/24", "reason": "block internal scan"},
		{"action": "allow", "cidr": "10.0.0.0/8", "reason": "trusted network"}
	]`)

	err := f.ParseConfig(jsonRules)
	require.NoError(t, err)

	rules := f.Rules()
	require.Len(t, rules, 2)
	assert.Equal(t, "deny", rules[0].Action)
	assert.Equal(t, "192.168.1.0/24", rules[0].CIDR.String())
	assert.Equal(t, "block internal scan", rules[0].Reason)
	assert.Equal(t, "allow", rules[1].Action)
	assert.Equal(t, "10.0.0.0/8", rules[1].CIDR.String())
}

func TestParseConfig_EmptyInput(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte{})
	assert.ErrorIs(t, err, ErrInvalidJSON)
}

func TestParseConfig_InvalidJSON(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`{invalid json`))
	assert.ErrorIs(t, err, ErrInvalidJSON)
}

func TestParseConfig_EmptyRuleArray(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[]`))
	assert.ErrorIs(t, err, ErrEmptyRules)
}

func TestParseConfig_InvalidAction(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "block", "cidr": "1.2.3.4/32"}]`))
	assert.ErrorIs(t, err, ErrInvalidAction)
}

func TestParseConfig_InvalidCIDR(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "999.999.999.999/32"}]`))
	assert.ErrorIs(t, err, ErrInvalidCIDR)
}

func TestParseConfig_EmptyCIDR(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": ""}]`))
	assert.ErrorIs(t, err, ErrInvalidCIDR)
}

func TestParseConfig_CaseInsensitiveAction(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "DENY", "cidr": "1.2.3.0/24"}, {"action": "Allow", "cidr": "5.6.7.0/24"}]`))
	require.NoError(t, err)

	rules := f.Rules()
	assert.Equal(t, "deny", rules[0].Action)
	assert.Equal(t, "allow", rules[1].Action)
}

func TestAllow_BlacklistMode_DenyMatched(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "192.168.1.0/24", "reason": "blocked"}]`))
	require.NoError(t, err)

	assert.False(t, f.Allow(net.ParseIP("192.168.1.50")))
	assert.False(t, f.Allow(net.ParseIP("192.168.1.0")))
	assert.False(t, f.Allow(net.ParseIP("192.168.1.255")))
}

func TestAllow_BlacklistMode_DenyNotMatched(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "192.168.1.0/24"}]`))
	require.NoError(t, err)

	assert.True(t, f.Allow(net.ParseIP("192.168.2.1")))
	assert.True(t, f.Allow(net.ParseIP("10.0.0.1")))
	assert.True(t, f.Allow(net.ParseIP("8.8.8.8")))
}

func TestAllow_WhitelistMode_AllowMatched(t *testing.T) {
	f := NewIPFilter()
	err := f.SetMode("whitelist")
	require.NoError(t, err)

	err = f.ParseConfig([]byte(`[{"action": "allow", "cidr": "10.0.0.0/8", "reason": "trusted"}]`))
	require.NoError(t, err)

	assert.True(t, f.Allow(net.ParseIP("10.0.0.1")))
	assert.True(t, f.Allow(net.ParseIP("10.255.255.255")))
}

func TestAllow_WhitelistMode_AllowNotMatched(t *testing.T) {
	f := NewIPFilter()
	err := f.SetMode("whitelist")
	require.NoError(t, err)

	err = f.ParseConfig([]byte(`[{"action": "allow", "cidr": "10.0.0.0/8"}]`))
	require.NoError(t, err)

	assert.False(t, f.Allow(net.ParseIP("192.168.1.1")))
	assert.False(t, f.Allow(net.ParseIP("8.8.8.8")))
}

func TestBuiltinWhitelist_Loopback_AlwaysAllowed(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "127.0.0.0/8", "reason": "try block loopback"}]`))
	require.NoError(t, err)

	assert.True(t, f.Allow(net.ParseIP("127.0.0.1")))
	assert.True(t, f.Allow(net.ParseIP("127.0.0.5")))
	assert.True(t, f.Allow(net.ParseIP("127.255.255.255")))

	f.SetMode("whitelist")
	f.ParseConfig([]byte(`[{"action": "allow", "cidr": "10.0.0.0/8"}]`))

	assert.True(t, f.Allow(net.ParseIP("127.0.0.1")))
}

func TestBuiltinWhitelist_LinkLocal_AlwaysAllowed(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "169.254.0.0/16", "reason": "try block link-local"}]`))
	require.NoError(t, err)

	assert.True(t, f.Allow(net.ParseIP("169.254.1.1")))
	assert.True(t, f.Allow(net.ParseIP("169.254.255.255")))

	f.SetMode("whitelist")
	f.ParseConfig([]byte(`[{"action": "allow", "cidr": "10.0.0.0/8"}]`))

	assert.True(t, f.Allow(net.ParseIP("169.254.100.50")))
}

func TestCIDRBoundary_NetworkAddress(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "10.1.2.0/24"}]`))
	require.NoError(t, err)

	assert.False(t, f.Allow(net.ParseIP("10.1.2.0")), "network address should be denied")
	assert.True(t, f.Allow(net.ParseIP("10.1.3.0")), "outside network should be allowed")
}

func TestCIDRBoundary_BroadcastAddress(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "10.1.2.0/24"}]`))
	require.NoError(t, err)

	assert.False(t, f.Allow(net.ParseIP("10.1.2.255")), "broadcast address should be denied")
}

func TestCIDRBoundary_SingleHost(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "1.2.3.4/32"}]`))
	require.NoError(t, err)

	assert.False(t, f.Allow(net.ParseIP("1.2.3.4")), "/32 exact match should be denied")
	assert.True(t, f.Allow(net.ParseIP("1.2.3.5")), "adjacent IP should be allowed")
}

func TestCIDRBoundary_LargeRange(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "10.0.0.0/8"}]`))
	require.NoError(t, err)

	assert.False(t, f.Allow(net.ParseIP("10.0.0.0")))
	assert.False(t, f.Allow(net.ParseIP("10.255.255.255")))
	assert.True(t, f.Allow(net.ParseIP("9.255.255.255")))
	assert.True(t, f.Allow(net.ParseIP("11.0.0.0")))
}

func TestSetMode_ValidModes(t *testing.T) {
	f := NewIPFilter()

	err := f.SetMode("whitelist")
	require.NoError(t, err)
	assert.Equal(t, "whitelist", f.Mode())

	err = f.SetMode("blacklist")
	require.NoError(t, err)
	assert.Equal(t, "blacklist", f.Mode())
}

func TestSetMode_InvalidMode(t *testing.T) {
	f := NewIPFilter()

	err := f.SetMode("graylist")
	assert.ErrorIs(t, err, ErrInvalidMode)

	err = f.SetMode("")
	assert.ErrorIs(t, err, ErrInvalidMode)

	assert.Equal(t, "blacklist", f.Mode(), "mode should remain unchanged on error")
}

func TestSetMode_CaseInsensitive(t *testing.T) {
	f := NewIPFilter()

	err := f.SetMode("WHITElist")
	require.NoError(t, err)
	assert.Equal(t, "whitelist", f.Mode())

	err = f.SetMode("BlackList")
	require.NoError(t, err)
	assert.Equal(t, "blacklist", f.Mode())
}

func TestAllow_NilIP(t *testing.T) {
	f := NewIPFilter()
	assert.False(t, f.Allow(nil))
}

func TestAllow_IPv6Address(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "2001:db8::/32"}]`))
	require.NoError(t, err)

	deniedIP := net.ParseIP("2001:db8::1")
	require.NotNil(t, deniedIP)
	assert.False(t, f.Allow(deniedIP))

	allowedIP := net.ParseIP("2001:db9::1")
	require.NotNil(t, allowedIP)
	assert.True(t, f.Allow(allowedIP))
}

func TestAllow_IPv4MappedIPv6(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "192.168.1.0/24"}]`))
	require.NoError(t, err)

	mappedIP := net.ParseIP("::ffff:192.168.1.50")
	require.NotNil(t, mappedIP)
	assert.False(t, f.Allow(mappedIP), "IPv4-mapped IPv6 should match IPv4 rule")
}

func TestRules_ReturnsCopy(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "1.2.3.0/24"}]`))
	require.NoError(t, err)

	rules1 := f.Rules()
	rules2 := f.Rules()

	assert.Equal(t, rules1, rules2)
	if len(rules1) > 0 {
		rules1[0].Reason = "mutated"
		assert.NotEqual(t, "mutated", f.Rules()[0].Reason, "Rules() should return a copy")
	}
}

func TestParseConfig_RoundTrip(t *testing.T) {
	original := []jsonRule{
		{Action: "deny", CIDR: "203.0.113.0/24", Reason: "test-net deny"},
		{Action: "allow", CIDR: "198.51.100.0/24", Reason: "test-net allow"},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	f := NewIPFilter()
	err = f.ParseConfig(data)
	require.NoError(t, err)

	rules := f.Rules()
	require.Len(t, rules, 2)
	assert.Equal(t, "deny", rules[0].Action)
	assert.Equal(t, "203.0.113.0/24", rules[0].CIDR.String())
	assert.Equal(t, "test-net deny", rules[0].Reason)
	assert.Equal(t, "allow", rules[1].Action)
	assert.Equal(t, "198.51.100.0/24", rules[1].CIDR.String())
}

func TestMultipleRules_FirstMatchWins_Blacklist(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[
		{"action": "deny", "cidr": "10.0.0.0/8"},
		{"action": "allow", "cidr": "10.1.0.0/16"}
	]`))
	require.NoError(t, err)

	assert.False(t, f.Allow(net.ParseIP("10.1.0.1")), "deny should take precedence in blacklist mode when both match")
}

func TestDefaultMode_IsBlacklist(t *testing.T) {
	f := NewIPFilter()
	assert.Equal(t, "blacklist", f.Mode())

	err := f.ParseConfig([]byte(`[{"action": "allow", "cidr": "10.0.0.0/8"}]`))
	require.NoError(t, err)

	assert.True(t, f.Allow(net.ParseIP("8.8.8.8")), "default blacklist allows non-matched IPs")
}

func TestBuiltinWhitelist_IPv6Loopback_AlwaysAllowed(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "::1/128", "reason": "try block IPv6 loopback"}]`))
	require.NoError(t, err)

	assert.True(t, f.Allow(net.ParseIP("::1")), "IPv6 loopback should always be allowed")

	f.SetMode("whitelist")
	f.ParseConfig([]byte(`[{"action": "allow", "cidr": "10.0.0.0/8"}]`))

	assert.True(t, f.Allow(net.ParseIP("::1")), "IPv6 loopback should be allowed even in whitelist mode")
}

func TestBuiltinWhitelist_IPv6LinkLocal_AlwaysAllowed(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "fe80::/10", "reason": "try block IPv6 link-local"}]`))
	require.NoError(t, err)

	assert.True(t, f.Allow(net.ParseIP("fe80::1")), "IPv6 link-local should always be allowed")
	assert.True(t, f.Allow(net.ParseIP("fe80::ffff")), "IPv6 link-local should always be allowed")

	f.SetMode("whitelist")
	f.ParseConfig([]byte(`[{"action": "allow", "cidr": "10.0.0.0/8"}]`))

	assert.True(t, f.Allow(net.ParseIP("fe80::1")), "IPv6 link-local should be allowed even in whitelist mode")
}

func TestBuiltinWhitelist_IPv6NonBuiltin_DeniedWhenRuleMatches(t *testing.T) {
	f := NewIPFilter()
	err := f.ParseConfig([]byte(`[{"action": "deny", "cidr": "2001:db8::/32", "reason": "documentation range"}]`))
	require.NoError(t, err)

	assert.False(t, f.Allow(net.ParseIP("2001:db8::1")), "non-builtin IPv6 should be denied by rule")
	assert.True(t, f.Allow(net.ParseIP("::1")), "IPv6 loopback is builtin whitelisted")
	assert.True(t, f.Allow(net.ParseIP("fe80::1")), "IPv6 link-local is builtin whitelisted")
}
