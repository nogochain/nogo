package airdrop

import (
	"net"
	"sync"
	"time"
)

type AntiSybil struct {
	mu          sync.RWMutex
	ipClaims    map[string][]time.Time
	emailClaims map[string]time.Time
	blacklist   map[string]time.Time

	maxClaimsPerIP    int
	maxClaimsPerEmail int
	blacklistDuration time.Duration
	windowDuration    time.Duration
}

func NewAntiSybil() *AntiSybil {
	return &AntiSybil{
		ipClaims:          make(map[string][]time.Time),
		emailClaims:       make(map[string]time.Time),
		blacklist:         make(map[string]time.Time),
		maxClaimsPerIP:    3,
		maxClaimsPerEmail: 1,
		blacklistDuration: 24 * time.Hour,
		windowDuration:    1 * time.Hour,
	}
}

func (as *AntiSybil) CheckClaim(ip, email string) (bool, string) {
	as.mu.Lock()
	defer as.mu.Unlock()

	now := time.Now()

	// Check blacklist
	if banTime, exists := as.blacklist[ip]; exists {
		if now.Before(banTime) {
			return false, "ip_blacklisted"
		}
		delete(as.blacklist, ip)
	}

	// Check IP claims in window
	if claims, exists := as.ipClaims[ip]; exists {
		validClaims := 0
		for _, t := range claims {
			if now.Sub(t) < as.windowDuration {
				validClaims++
			}
		}
		if validClaims >= as.maxClaimsPerIP {
			as.blacklist[ip] = now.Add(as.blacklistDuration)
			return false, "too_many_claims_from_ip"
		}
	}

	// Check email claims
	if email != "" {
		if _, exists := as.emailClaims[email]; exists {
			return false, "email_already_used"
		}
	}

	return true, ""
}

func (as *AntiSybil) RecordClaim(ip, email string) {
	as.mu.Lock()
	defer as.mu.Unlock()

	now := time.Now()

	// Record IP claim
	as.ipClaims[ip] = append(as.ipClaims[ip], now)

	// Clean old IP claims
	if len(as.ipClaims[ip]) > 100 {
		keep := 50
		if len(as.ipClaims[ip]) < 50 {
			keep = len(as.ipClaims[ip])
		}
		as.ipClaims[ip] = as.ipClaims[ip][len(as.ipClaims[ip])-keep:]
	}

	// Record email claim
	if email != "" {
		as.emailClaims[email] = now
	}
}

func (as *AntiSybil) IsBlacklisted(ip string) bool {
	as.mu.RLock()
	defer as.mu.RUnlock()

	banTime, exists := as.blacklist[ip]
	if !exists {
		return false
	}

	return time.Now().Before(banTime)
}

func (as *AntiSybil) GetStats() map[string]interface{} {
	as.mu.RLock()
	defer as.mu.RUnlock()

	return map[string]interface{}{
		"uniqueIPs":      len(as.ipClaims),
		"uniqueEmails":   len(as.emailClaims),
		"blacklistedIPs": len(as.blacklist),
	}
}

func (as *AntiSybil) cleanupOldEntries() {
	as.mu.Lock()
	defer as.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-as.windowDuration)

	for ip, claims := range as.ipClaims {
		validClaims := 0
		for _, t := range claims {
			if t.After(cutoff) {
				validClaims++
			}
		}
		if validClaims == 0 {
			delete(as.ipClaims, ip)
		} else {
			as.ipClaims[ip] = claims[len(claims)-validClaims:]
		}
	}

	for ip, banTime := range as.blacklist {
		if now.After(banTime) {
			delete(as.blacklist, ip)
		}
	}
}

func isValidIP(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && !parsed.IsUnspecified()
}
