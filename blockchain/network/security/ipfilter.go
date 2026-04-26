package security

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
)

var (
	ErrInvalidMode    = errors.New("invalid mode, must be 'blacklist' or 'whitelist'")
	ErrInvalidAction  = errors.New("invalid action, must be 'allow' or 'deny'")
	ErrInvalidCIDR    = errors.New("invalid CIDR notation")
	ErrEmptyRules     = errors.New("empty rules configuration")
	ErrInvalidJSON    = errors.New("invalid JSON format")

	builtinWhitelist = buildBuiltinWhitelist()
)

func buildBuiltinWhitelist() []*net.IPNet {
	// These CIDRs are constant and should always parse successfully.
	// If parsing fails for any reason, fall back to an empty allowlist instead of panicking.
	cidrs := []string{
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fe80::/10",
	}

	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, ipNet, err := net.ParseCIDR(c)
		if err != nil || ipNet == nil {
			continue
		}
		out = append(out, ipNet)
	}
	return out
}

type FilterRule struct {
	Action string    `json:"action"`
	CIDR   *net.IPNet `json:"cidr"`
	Reason string    `json:"reason"`
}

type jsonRule struct {
	Action string `json:"action"`
	CIDR   string `json:"cidr"`
	Reason string `json:"reason"`
}

type IPFilter struct {
	rules []FilterRule
	mode  string
	mu    sync.RWMutex
}

func NewIPFilter() *IPFilter {
	return &IPFilter{
		rules: make([]FilterRule, 0),
		mode:  "blacklist",
	}
}

func (f *IPFilter) ParseConfig(jsonRules []byte) error {
	if len(jsonRules) == 0 {
		return fmt.Errorf("%w: input is empty", ErrInvalidJSON)
	}

	var rawRules []jsonRule
	if err := json.Unmarshal(jsonRules, &rawRules); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	if len(rawRules) == 0 {
		return ErrEmptyRules
	}

	parsedRules := make([]FilterRule, 0, len(rawRules))
	for i, r := range rawRules {
		action := strings.ToLower(strings.TrimSpace(r.Action))
		if action != "allow" && action != "deny" {
			return fmt.Errorf("%w: rule[%d] action=%q", ErrInvalidAction, i, action)
		}

		cidrStr := strings.TrimSpace(r.CIDR)
		if cidrStr == "" {
			return fmt.Errorf("%w: rule[%d] cidr is empty", ErrInvalidCIDR, i)
		}

		_, ipNet, err := net.ParseCIDR(cidrStr)
		if err != nil {
			return fmt.Errorf("%w: rule[%d] cidr=%q: %v", ErrInvalidCIDR, i, cidrStr, err)
		}

		parsedRules = append(parsedRules, FilterRule{
			Action: action,
			CIDR:   ipNet,
			Reason: r.Reason,
		})
	}

	f.mu.Lock()
	f.rules = parsedRules
	f.mu.Unlock()

	return nil
}

func (f *IPFilter) Allow(ip net.IP) bool {
	if ip == nil {
		return false
	}

	normalizedIP := ip.To4()
	if normalizedIP == nil {
		normalizedIP = ip
	}

	for _, builtin := range builtinWhitelist {
		if builtin.Contains(normalizedIP) {
			return true
		}
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	switch f.mode {
	case "blacklist":
		for _, rule := range f.rules {
			if rule.Action == "deny" && rule.CIDR.Contains(normalizedIP) {
				return false
			}
		}
		return true
	case "whitelist":
		for _, rule := range f.rules {
			if rule.Action == "allow" && rule.CIDR.Contains(normalizedIP) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func (f *IPFilter) SetMode(mode string) error {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized != "blacklist" && normalized != "whitelist" {
		return fmt.Errorf("%w: got=%q", ErrInvalidMode, mode)
	}

	f.mu.Lock()
	f.mode = normalized
	f.mu.Unlock()

	return nil
}

func (f *IPFilter) Mode() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.mode
}

func (f *IPFilter) Rules() []FilterRule {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make([]FilterRule, len(f.rules))
	copy(result, f.rules)
	return result
}

// parseCIDRMustSucceed is kept for backward compatibility with older code paths.
// It never panics and returns nil on parse failure.
func parseCIDRMustSucceed(cidr string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil
	}
	return ipNet
}
