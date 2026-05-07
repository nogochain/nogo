package discover

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/nogochain/nogo/blockchain/p2p/discover/dht"
)

const (
	dnsTimeout   = 5 * time.Second
	defaultUDP   = uint16(30303)
	defaultTCP   = uint16(30303)
)

// DNSParse resolves DNS seed domain names to DHT nodes via TXT and A records.
// TXT records: "enode://<hex_id>@<ip>:<tcp>?discport=<udp>"
func DNSParse(domains []string) []*dht.Node {
	var all []*dht.Node
	for _, domain := range domains {
		nodes, err := resolveDNS(domain)
		if err != nil {
			log.Printf("[DNS] seed %s: %v", domain, err)
			continue
		}
		all = append(all, nodes...)
	}
	return all
}

func resolveDNS(domain string) ([]*dht.Node, error) {
	resolver := &net.Resolver{PreferGo: true}
	ctx, cancel := context.WithTimeout(context.Background(), dnsTimeout)
	defer cancel()

	txts, err := resolver.LookupTXT(ctx, domain)
	if err == nil {
		var nodes []*dht.Node
		for _, txt := range txts {
			if n := parseEnode(txt); n != nil {
				nodes = append(nodes, n)
			}
		}
		if len(nodes) > 0 {
			return nodes, nil
		}
	}

	// Fallback: A record with default ports
	ips, err := resolver.LookupHost(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("dns: no TXT or A records for %s", domain)
	}
	var nodes []*dht.Node
	for _, ip := range ips {
		nodes = append(nodes, dht.NewNode(dht.ZeroNodeID, net.ParseIP(ip), defaultUDP, defaultTCP))
	}
	return nodes, nil
}

// parseEnode parses an enode:// URL into a DHT Node.
func parseEnode(raw string) *dht.Node {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "enode://") {
		return nil
	}

	s := raw[len("enode://"):]
	atIdx := strings.Index(s, "@")
	if atIdx < 0 {
		return nil
	}

	idHex := s[:atIdx]
	idBytes, err := hexDecode(idHex)
	if err != nil || len(idBytes) != 32 {
		return nil
	}
	var id dht.NodeID
	copy(id[:], idBytes)

	addr := s[atIdx+1:]
	udpPort := 0

	if qIdx := strings.Index(addr, "?"); qIdx >= 0 {
		params := addr[qIdx+1:]
		addr = addr[:qIdx]
		for _, p := range strings.Split(params, "&") {
			if strings.HasPrefix(p, "discport=") {
				udpPort, _ = strconv.Atoi(p[len("discport="):])
			}
		}
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil
	}

	ip := net.ParseIP(host)
	if ip == nil {
		ips, _ := net.LookupIP(host)
		if len(ips) == 0 {
			return nil
		}
		ip = ips[0]
	}

	tcp, _ := strconv.Atoi(portStr)
	udp := uint16(tcp)
	if udpPort > 0 {
		udp = uint16(udpPort)
	}

	n := dht.NewNode(id, ip, udp, uint16(tcp))
	log.Printf("[DNS] Parsed seed: enode://%s@%s:%d", idHex[:16]+"...", ip, tcp)
	return n
}

func hexDecode(s string) ([]byte, error) {
	n := len(s) / 2
	b := make([]byte, n)
	for i := 0; i < len(s)-1; i += 2 {
		var v byte
		if _, err := fmt.Sscanf(s[i:i+2], "%02x", &v); err != nil {
			return nil, err
		}
		b[i/2] = v
	}
	return b, nil
}
