package network

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// DefaultIPDetectTimeout is the default timeout for IP detection operations
	DefaultIPDetectTimeout = 5 * time.Second
	// DefaultIPServiceURL is the default external IP service
	DefaultIPServiceURL = "https://api.ipify.org?format=json"
)

// IPServiceResponse represents the JSON response from ipify.org
type IPServiceResponse struct {
	IP string `json:"ip"`
}

// detectPublicIP attempts to detect the public IP address using multiple methods in priority order:
// 1. P2P_PUBLIC_IP environment variable if set
// 2. Query external service (ipify.org) with configurable timeout
// 3. Extract from outbound connection as fallback
// Returns empty string on failure (graceful degradation, no panic)
func detectPublicIP() (string, error) {
	// Method 1: Check environment variable (highest priority)
	if ip := os.Getenv("P2P_PUBLIC_IP"); ip != "" {
		ip = strings.TrimSpace(ip)
		if err := validatePublicIP(ip); err == nil {
			return ip, nil
		}
	}

	// Method 2: Query external IP service
	timeout := getIPDetectTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if ip, err := queryIPService(ctx, DefaultIPServiceURL); err == nil {
		if err := validatePublicIP(ip); err == nil {
			return ip, nil
		}
	}

	// Method 3: Extract from outbound connection (fallback)
	if ip, err := detectIPOutbound(); err == nil {
		if err := validatePublicIP(ip); err == nil {
			return ip, nil
		}
	}

	// All methods failed - graceful degradation
	return "", fmt.Errorf("all IP detection methods failed")
}

// queryIPService queries an external HTTP service to get the public IP address
// Uses context for timeout control and proper error handling
func queryIPService(ctx context.Context, url string) (string, error) {
	client := &http.Client{
		Timeout: DefaultIPDetectTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   DefaultIPDetectTimeout,
				KeepAlive: 0,
			}).DialContext,
			MaxIdleConns:        1,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: DefaultIPDetectTimeout,
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create IP service request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "NogoChain-P2P/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("query IP service: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("p2p ip detect: failed to close IP service response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IP service returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read IP service response: %w", err)
	}

	var ipResp IPServiceResponse
	if err := json.Unmarshal(body, &ipResp); err != nil {
		return "", fmt.Errorf("parse IP service response: %w", err)
	}

	if ipResp.IP == "" {
		return "", fmt.Errorf("empty IP from service")
	}

	return ipResp.IP, nil
}

// detectIPOutbound extracts the public IP from an outbound connection
// This is a fallback method that connects to a public server and reads the local address
func detectIPOutbound() (string, error) {
	conn, err := net.DialTimeout("udp", "8.8.8.8:53", DefaultIPDetectTimeout)
	if err != nil {
		return "", fmt.Errorf("create outbound connection: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// validatePublicIP validates that an IP address is a valid public IPv4 address
// Rejects private, loopback, link-local, and other reserved ranges:
// - 10.0.0.0/8 (private)
// - 172.16.0.0/12 (private)
// - 192.168.0.0/16 (private)
// - 127.0.0.0/8 (loopback)
// - 169.254.0.0/16 (link-local)
// - 0.0.0.0 (unspecified)
func validatePublicIP(ipStr string) error {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("invalid IP format: %s", ipStr)
	}

	// Convert to IPv4 if it's an IPv4-mapped IPv6 address
	ip = ip.To4()
	if ip == nil {
		return fmt.Errorf("IPv6 not supported: %s", ipStr)
	}

	// Reject unspecified address (0.0.0.0)
	if ip.Equal(net.IPv4zero) {
		return fmt.Errorf("unspecified address (0.0.0.0) not allowed")
	}

	// Check for private and reserved ranges
	if isPrivateIP(ip) {
		return fmt.Errorf("private/reserved IP not allowed: %s", ipStr)
	}

	return nil
}

// isPrivateIP checks if an IP address falls within private or reserved ranges
func isPrivateIP(ip net.IP) bool {
	// 10.0.0.0/8
	if ip[0] == 10 {
		return true
	}

	// 172.16.0.0/12 (172.16.0.0 - 172.31.255.255)
	if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		return true
	}

	// 192.168.0.0/16
	if ip[0] == 192 && ip[1] == 168 {
		return true
	}

	// 127.0.0.0/8 (loopback)
	if ip[0] == 127 {
		return true
	}

	// 169.254.0.0/16 (link-local)
	if ip[0] == 169 && ip[1] == 254 {
		return true
	}

	// 0.0.0.0/8 (this network)
	if ip[0] == 0 {
		return true
	}

	// 224.0.0.0/4 (multicast)
	if ip[0] >= 224 && ip[0] <= 239 {
		return true
	}

	// 240.0.0.0/4 (reserved for future use)
	if ip[0] >= 240 {
		return true
	}

	return false
}

// getIPDetectTimeout returns the IP detection timeout from environment or default
func getIPDetectTimeout() time.Duration {
	if val := os.Getenv("P2P_IP_DETECT_TIMEOUT"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			if d <= 0 {
				return DefaultIPDetectTimeout
			}
			// Cap at 30 seconds maximum
			if d > 30*time.Second {
				return 30 * time.Second
			}
			return d
		}
	}
	return DefaultIPDetectTimeout
}

// GetPublicIPWithFallback is the main entry point for public IP detection
// Returns empty string on failure (graceful degradation, no panic)
// Logs warnings but never panics
func GetPublicIPWithFallback() (string, error) {
	ip, err := detectPublicIP()
	if err != nil {
		// Graceful degradation - return empty string, caller decides what to do
		return "", err
	}
	return ip, nil
}

// IsPublicIPValid is a convenience function to validate an IP address
// Returns true if the IP is a valid public IPv4 address
func IsPublicIPValid(ipStr string) bool {
	return validatePublicIP(ipStr) == nil
}
