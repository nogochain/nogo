package dht

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// DNS seed configuration.
const (
	DefaultDNSSeed       = "seed.nogochain.com"
	DefaultDNSPort       = "30303"
	DNSTimeout           = 5 * time.Second
	DNSTXTRecordPrefix   = "enode://"
)

// Errors for DNS seed operations.
var (
	ErrDNSTimeout    = errors.New("DNS seed query timeout")
	ErrDNSSeedsEmpty = errors.New("DNS seeds returned no results")
)

// DNSSeed represents a DNS seed with its hostname and default port.
type DNSSeedConfig struct {
	Hostname string
	Port     string
}

// DefaultDNSSeeds returns the default DNS seed configuration for NogoChain.
func DefaultDNSSeeds() []DNSSeedConfig {
	return []DNSSeedConfig{
		{Hostname: DefaultDNSSeed, Port: DefaultDNSPort},
	}
}

// QueryDNSSeeds queries DNS seeds and returns discovered peer addresses.
// Uses the provided lookupHost function for DNS resolution.
// Returns the first successful result or an error if all queries fail.
func QueryDNSSeeds(lookupHost func(host string) ([]string, error), seeds []DNSSeedConfig) ([]string, error) {
	if len(seeds) == 0 {
		return nil, ErrDNSSeedsEmpty
	}

	resultCh := make(chan []string, len(seeds))
	ctx, cancel := context.WithTimeout(context.Background(), DNSTimeout)
	defer cancel()

	// Query all seeds concurrently.
	for _, seed := range seeds {
		go func(s DNSSeedConfig) {
			addresses, err := querySingleSeed(ctx, lookupHost, s)
			if err == nil && len(addresses) > 0 {
				select {
				case resultCh <- addresses:
				default:
					// Channel already has a result, drop this one.
				}
			}
		}(seed)
	}

	// Wait for first result or timeout.
	select {
	case result := <-resultCh:
		if len(result) == 0 {
			return nil, ErrDNSSeedsEmpty
		}
		return result, nil
	case <-ctx.Done():
		return nil, ErrDNSTimeout
	}
}

// querySingleSeed resolves a single DNS seed hostname.
func querySingleSeed(ctx context.Context, lookupHost func(host string) ([]string, error), seed DNSSeedConfig) ([]string, error) {
	// Use a context with timeout for the DNS lookup.
	lookupCtx, cancel := context.WithTimeout(ctx, DNSTimeout)
	defer cancel()

	// DNS lookup.
	addrs, err := lookupHostWithContext(lookupCtx, lookupHost, seed.Hostname)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed for %s: %w", seed.Hostname, err)
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses returned for %s", seed.Hostname)
	}

	// Validate and format addresses.
	var seeds []string
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue // Skip invalid IPs.
		}

		// Skip private and loopback addresses.
		if ip.IsPrivate() || ip.IsLoopback() {
			continue
		}

		seeds = append(seeds, net.JoinHostPort(addr, seed.Port))
	}

	if len(seeds) == 0 {
		return nil, ErrDNSSeedsEmpty
	}

	return seeds, nil
}

// lookupHostWithContext wraps a synchronous lookupHost function with context support.
func lookupHostWithContext(ctx context.Context, lookupHost func(host string) ([]string, error), host string) ([]string, error) {
	type result struct {
		addrs []string
		err   error
	}

	resultCh := make(chan result, 1)

	go func() {
		addrs, err := lookupHost(host)
		resultCh <- result{addrs: addrs, err: err}
	}()

	select {
	case r := <-resultCh:
		return r.addrs, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// QueryTXTSeeds queries DNS TXT records for seed node enode:// URLs.
func QueryTXTSeeds(lookupTXT func(host string) ([]string, error), seedHost string) ([]string, error) {
	if seedHost == "" {
		return nil, errors.New("empty seed host")
	}

	ctx, cancel := context.WithTimeout(context.Background(), DNSTimeout)
	defer cancel()

	txtCh := make(chan []string, 1)
	go func() {
		txts, err := lookupTXT(seedHost)
		if err == nil {
			select {
			case txtCh <- txts:
			default:
			}
		}
	}()

	select {
	case txts := <-txtCh:
		return parseENODETXTRecords(txts), nil
	case <-ctx.Done():
		return nil, ErrDNSTimeout
	}
}

// parseENODETXTRecords extracts enode:// URLs from TXT records.
func parseENODETXTRecords(records []string) []string {
	var enodes []string
	for _, record := range records {
		// Skip empty records.
		if record == "" {
			continue
		}

		// Check for enode:// prefix.
		if len(record) >= len(DNSTXTRecordPrefix) && record[:len(DNSTXTRecordPrefix)] == DNSTXTRecordPrefix {
			enodes = append(enodes, record)
		}
	}
	return enodes
}

// GetSeedNodes combines DNS A/AAAA and TXT record queries to discover seed nodes.
// Returns a list of seed node URLs (enode:// format).
func GetSeedNodes(
	lookupHost func(host string) ([]string, error),
	lookupTXT func(host string) ([]string, error),
	seeds []DNSSeedConfig,
	txtHost string,
) ([]string, error) {
	var allNodes []string

	// Query A/AAAA records and add directly as addresses.
	ipAddresses, _ := QueryDNSSeeds(lookupHost, seeds)
	for _, addr := range ipAddresses {
		allNodes = append(allNodes, addr)
	}

	// Query TXT records.
	if txtHost != "" {
		enodeURLs, _ := QueryTXTSeeds(lookupTXT, txtHost)
		allNodes = append(allNodes, enodeURLs...)
	}

	if len(allNodes) == 0 {
		return nil, ErrDNSSeedsEmpty
	}

	return allNodes, nil
}
