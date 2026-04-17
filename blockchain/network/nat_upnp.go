// Copyright 2026 NogoChain Team
// Complete UPnP NAT Traversal for P2P connectivity
// Implements full UPnP protocol stack for automatic port forwarding

package network

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// UPnP service types
	UPNPServiceWANIP  = "urn:schemas-upnp-org:service:WANIPConnection:1"
	UPNPServiceWANIP2 = "urn:schemas-upnp-org:service:WANIPConnection:2"
	UPNPServiceWANPPP = "urn:schemas-upnp-org:service:WANPPPConnection:1"
	UPNPDeviceIGD     = "urn:schemas-upnp-org:device:InternetGatewayDevice:1"

	// SSDP multicast address and port
	SSDPMulticastAddr = "239.255.255.250:1900"

	// UPnP timeouts
	UPnPDiscoveryTimeout = 5 * time.Second
	UPnPRequestTimeout   = 10 * time.Second
	UPnPMapLeaseDuration = 3600 // 1 hour
	UPnPRefreshInterval  = 30 * time.Minute

	// Core-geth and Bitcoin-inspired configuration
	UPnPPortRetryAttempts    = 5     // Number of retry attempts with random ports
	UPnPPortRetryInterval   = 2 * time.Second
	UPnPRandomPortBase      = 9000  // Base port for random allocation
	UPnPRandomPortRange     = 1000   // Range for random allocation (9000-9999)
)

// SSDP search targets
var ssdpSearchTargets = []string{
	UPNPDeviceIGD,
	UPNPServiceWANIP,
	UPNPServiceWANIP2,
	UPNPServiceWANPPP,
}

// UPnP device description XML structures
type Device struct {
	DeviceType   string `xml:"deviceType"`
	FriendlyName string `xml:"friendlyName"`
	Manufacturer string `xml:"manufacturer"`
	ModelName    string `xml:"modelName"`
	ModelNumber  string `xml:"modelNumber"`
	SerialNumber string `xml:"serialNumber"`
	UDN          string `xml:"UDN"`
	ServiceList  []struct {
		Service struct {
			ServiceType string `xml:"serviceType"`
			ServiceID   string `xml:"serviceId"`
			ControlURL  string `xml:"controlURL"`
			EventSubURL string `xml:"eventSubURL"`
			SCPDURL     string `xml:"SCPDURL"`
		} `xml:"service"`
	} `xml:"serviceList>service"`
	DeviceList []struct {
		Device Device `xml:"device"`
	} `xml:"deviceList>device"`
}

type RootDevice struct {
	XMLName xml.Name `xml:"root"`
	Device  Device   `xml:"device"`
}

// SSDP response structure
type SSDPResponse struct {
	Location     string
	USN          string
	Server       string
	ST           string
	CacheControl string
}

// SOAP request/response structures
type GetExternalIPAddressResponse struct {
	XMLName              xml.Name `xml:"GetExternalIPAddressResponse"`
	NewExternalIPAddress string   `xml:"NewExternalIPAddress"`
}

type AddPortMappingResponse struct {
	XMLName xml.Name `xml:"AddPortMappingResponse"`
}

type DeletePortMappingResponse struct {
	XMLName xml.Name `xml:"DeletePortMappingResponse"`
}

// UPnP service information
type UPnPService struct {
	ServiceType string
	ControlURL  string
	BaseURL     string
}

// NATProtocol represents supported NAT traversal protocols
type NATProtocol int

const (
	NATProtocolUPnP NATProtocol = iota // 0: Universal Plug and Play
	NATProtocolPCP                   // 1: Port Control Protocol (RFC 6887)
	NATProtocolNATPMP               // 2: NAT Port Mapping Protocol (RFC 6886)
)

func (p NATProtocol) String() string {
	switch p {
	case NATProtocolUPnP:
		return "UPnP"
	case NATProtocolPCP:
		return "PCP"
	case NATProtocolNATPMP:
		return "NAT-PMP"
	default:
		return "Unknown"
	}
}

// NATManager handles NAT traversal with multiple protocol support
// Inspired by Bitcoin's multi-protocol NAT support
type NATManager struct {
	localIP      string
	localPort    int
	externalIP   string
	externalPort int
	protocol     NATProtocol
	mu           sync.RWMutex
	gateway      string
	controlURL   string
	serviceType  string
	lastUpdated  time.Time
	forwarded    bool
	discovered   bool
	client       *http.Client
	cancel       context.CancelFunc

	// Gateway address for PCP/NAT-PMP
	gatewayIP   net.IP

	// Protocol detection results
	protocolsSupported map[NATProtocol]bool
}

// NewNATManager creates a new NAT manager with multi-protocol support
func NewNATManager(listenPort int) *NATManager {
	return &NATManager{
		localIP:          getLocalIP(),
		localPort:        listenPort,
		protocol:          NATProtocolUPnP, // Default to UPnP
		protocolsSupported: make(map[NATProtocol]bool),
		gatewayIP:        nil,
		externalIP:       "",
		externalPort:     0,
		mu:              sync.RWMutex{},
		lastUpdated:      time.Now(),
		forwarded:        false,
		discovered:       false,
		client:           &http.Client{Timeout: UPnPRequestTimeout},
	}
}

// getLocalIP returns the local IP address
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Printf("[NAT] Failed to get local interfaces: %v", err)
		return "127.0.0.1"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil && !ipnet.IP.IsLinkLocalMulticast() {
				return ipnet.IP.String()
			}
		}
	}

	return "127.0.0.1"
}

// DiscoverGateway discovers UPnP-enabled gateway using SSDP
func (nm *NATManager) DiscoverGateway() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if nm.discovered && nm.controlURL != "" {
		log.Printf("[NAT] Using discovered gateway: %s", nm.gateway)
		return nil
	}

	log.Printf("[NAT] Starting SSDP discovery for UPnP gateway...")

	// Create UDP connection for SSDP
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return fmt.Errorf("failed to create UDP socket: %w", err)
	}
	defer conn.Close()

	// Enable multicast
	addr, err := net.ResolveUDPAddr("udp4", SSDPMulticastAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve multicast address: %w", err)
	}

	// Send SSDP M-SEARCH requests
	for _, target := range ssdpSearchTargets {
		msg := fmt.Sprintf("M-SEARCH * HTTP/1.1\r\n"+
			"HOST: %s\r\n"+
			"MAN: \"ssdp:discover\"\r\n"+
			"MX: 3\r\n"+
			"ST: %s\r\n"+
			"\r\n", SSDPMulticastAddr, target)

		_, err := conn.WriteTo([]byte(msg), addr)
		if err != nil {
			log.Printf("[NAT] Failed to send M-SEARCH for %s: %v", target, err)
			continue
		}
		log.Printf("[NAT] Sent M-SEARCH for: %s", target)
	}

	// Read responses with timeout
	conn.SetReadDeadline(time.Now().Add(UPnPDiscoveryTimeout))
	buf := make([]byte, 4096)

	var devices []SSDPResponse
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			log.Printf("[NAT] Error reading SSDP response: %v", err)
			continue
		}

		response := parseSSDPResponse(string(buf[:n]))
		if response.Location != "" {
			log.Printf("[NAT] Found UPnP device: %s at %s", response.ST, response.Location)
			devices = append(devices, response)
		}
	}

	if len(devices) == 0 {
		return fmt.Errorf("no UPnP devices found on network")
	}

	log.Printf("[NAT] Discovered %d UPnP device(s), finding WAN IP service...", len(devices))

	// Find the WAN IP connection service
	for _, device := range devices {
		service, err := nm.findWANIPService(device.Location)
		if err != nil {
			log.Printf("[NAT] Failed to get service from %s: %v", device.Location, err)
			continue
		}

		if service != nil {
			nm.controlURL = service.ControlURL
			nm.serviceType = service.ServiceType
			nm.gateway = service.BaseURL
			nm.discovered = true

			log.Printf("[NAT] Successfully discovered WAN IP service:")
			log.Printf("[NAT]   Gateway: %s", nm.gateway)
			log.Printf("[NAT]   Control URL: %s", nm.controlURL)
			log.Printf("[NAT]   Service Type: %s", nm.serviceType)

			return nil
		}
	}

	return fmt.Errorf("no WAN IP connection service found")
}

// parseSSDPResponse parses SSDP response header
func parseSSDPResponse(data string) SSDPResponse {
	var response SSDPResponse
	lines := strings.Split(data, "\r\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, ":"); idx != -1 {
			key := strings.ToUpper(strings.TrimSpace(line[:idx]))
			value := strings.TrimSpace(line[idx+1:])

			switch key {
			case "LOCATION":
				response.Location = value
			case "USN":
				response.USN = value
			case "SERVER":
				response.Server = value
			case "ST":
				response.ST = value
			case "CACHE-CONTROL":
				response.CacheControl = value
			}
		}
	}
	return response
}

// findWANIPService finds WAN IP connection service from device description
func (nm *NATManager) findWANIPService(deviceURL string) (*UPnPService, error) {
	// Parse device URL to get base URL
	baseURL, err := url.Parse(deviceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse device URL: %w", err)
	}

	// Fetch device description
	resp, err := nm.client.Get(deviceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch device description: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device description returned status %d", resp.StatusCode)
	}

	// Parse XML
	var root RootDevice
	decoder := xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("failed to parse device description: %w", err)
	}

	log.Printf("[NAT] Device: %s (%s)", root.Device.FriendlyName, root.Device.Manufacturer)

	// Search for WAN IP connection service
	service := nm.searchForService(root.Device, baseURL.Scheme+"://"+baseURL.Host)
	if service != nil {
		return service, nil
	}

	return nil, nil
}

// searchForService recursively searches for WAN IP service
func (nm *NATManager) searchForService(device Device, baseURL string) *UPnPService {
	// Check device's services
	for _, svcList := range device.ServiceList {
		svc := svcList.Service
		if nm.isWANIPService(svc.ServiceType) {
			// Build full control URL
			controlURL := svc.ControlURL
			if strings.HasPrefix(controlURL, "/") {
				controlURL = baseURL + controlURL
			}

			return &UPnPService{
				ServiceType: svc.ServiceType,
				ControlURL:  controlURL,
				BaseURL:     baseURL,
			}
		}
	}

	// Recursively check embedded devices
	for _, devList := range device.DeviceList {
		if service := nm.searchForService(devList.Device, baseURL); service != nil {
			return service
		}
	}

	return nil
}

// isWANIPService checks if service type is a WAN IP connection service
func (nm *NATManager) isWANIPService(serviceType string) bool {
	return strings.Contains(serviceType, "WANIPConnection") ||
		strings.Contains(serviceType, "WANPPPConnection")
}

// GetExternalAddress retrieves the external IP address
func (nm *NATManager) GetExternalAddress() (string, int) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if nm.externalIP != "" && nm.externalPort > 0 {
		return nm.externalIP, nm.externalPort
	}

	return "", 0
}

// GetExternalIPAddress fetches external IP from UPnP gateway
func (nm *NATManager) GetExternalIPAddress() (string, error) {
	if !nm.discovered || nm.controlURL == "" {
		if err := nm.DiscoverGateway(); err != nil {
			return "", fmt.Errorf("gateway not discovered: %w", err)
		}
	}

	// Build SOAP request
	soapBody := fmt.Sprintf(`<?xml version="1.0"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
soap:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <soap:Body>
    <u:GetExternalIPAddress xmlns:u="%s"/>
  </soap:Body>
</soap:Envelope>`, nm.serviceType)

	req, err := http.NewRequest("POST", nm.controlURL, strings.NewReader(soapBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Set("SOAPAction", fmt.Sprintf("\"%s#GetExternalIPAddress\"", nm.serviceType))

	resp, err := nm.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SOAP request returned status %d", resp.StatusCode)
	}

	// Parse response
	var response GetExternalIPAddressResponse
	decoder := xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if response.NewExternalIPAddress == "" {
		return "", fmt.Errorf("empty external IP in response")
	}

	nm.mu.Lock()
	nm.externalIP = response.NewExternalIPAddress
	nm.lastUpdated = time.Now()
	nm.mu.Unlock()

	log.Printf("[NAT] External IP: %s", nm.externalIP)
	return nm.externalIP, nil
}

// randomPort generates a random port number in the allowed range
// Inspired by Core-geth's randomPort() and Bitcoin's port retry strategy
func randomPort() int {
	// Use crypto/rand for better randomness
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		// Fallback to simple hash-based random if crypto/rand fails
		h := hex.EncodeToString([]byte(time.Now().String()))
		// Use first 4 chars as a number and modulo the range
		n, _ := strconv.ParseInt(h[:4], 16, 64)
		return UPnPRandomPortBase + int(n%int64(UPnPRandomPortRange))
	}
	// Convert 2 bytes to a number and modulo the range
	n := uint16(b[0])<<8 | uint16(b[1])
	return UPnPRandomPortBase + int(n%uint16(UPnPRandomPortRange))
}

// addPortMappingInternal performs the actual SOAP AddPortMapping request
func (nm *NATManager) addPortMappingInternal(externalPort, internalPort int, protocol string) error {
	soapBody := fmt.Sprintf(`<?xml version="1.0"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
soap:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <soap:Body>
    <u:AddPortMapping xmlns:u="%s">
      <NewRemoteHost></NewRemoteHost>
      <NewExternalPort>%d</NewExternalPort>
      <NewProtocol>%s</NewProtocol>
      <NewInternalPort>%d</NewInternalPort>
      <NewInternalClient>%s</NewInternalClient>
      <NewEnabled>1</NewEnabled>
      <NewPortMappingDescription>%s</NewPortMappingDescription>
      <NewLeaseDuration>%d</NewLeaseDuration>
    </u:AddPortMapping>
  </soap:Body>
</soap:Envelope>`,
		nm.serviceType,
		externalPort,
		protocol,
		internalPort,
		nm.localIP,
		"NogoChain P2P",
		UPnPMapLeaseDuration)

	req, err := http.NewRequest("POST", nm.controlURL, strings.NewReader(soapBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Set("SOAPAction", fmt.Sprintf("\"%s#AddPortMapping\"", nm.serviceType))

	resp, err := nm.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send port mapping request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := readAll(resp.Body)
		return fmt.Errorf("port mapping request failed with status %d: %s", resp.StatusCode, body)
	}

	// Parse response (even though we don't need the content, we should check for errors)
	var response AddPortMappingResponse
	decoder := xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		log.Printf("[NAT] Warning: failed to parse AddPortMapping response: %v", err)
	}

	return nil
}

// ForwardPortMultiProtocol tries to create port mapping using multiple protocols
// Implements Bitcoin's strategy: try PCP first, then NAT-PMP, then UPnP
func (nm *NATManager) ForwardPortMultiProtocol() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Check if already forwarded
	if nm.forwarded && nm.externalIP != "" && nm.externalPort > 0 {
		log.Printf("[NAT] Port already forwarded: %d (local %d -> external %d)",
			nm.localPort, nm.localPort, nm.externalPort)
		return nil
	}

	log.Printf("[NAT] Attempting multi-protocol port mapping for local port %d", nm.localPort)

	// Try protocols in order: PCP -> NAT-PMP -> UPnP
	protocols := []NATProtocol{NATProtocolPCP, NATProtocolNATPMP, NATProtocolUPnP}

	var lastErr error
	for _, protocol := range protocols {
		log.Printf("[NAT] Trying protocol: %s", protocol.String())
		nm.protocol = protocol

		// Try forward with current protocol
		err := nm.tryForwardWithProtocol(protocol)
		if err == nil {
			// Success!
			log.Printf("[NAT] ✓ Port mapping successful using %s!", protocol.String())
			return nil
		}

		log.Printf("[NAT] Protocol %s failed: %v", protocol.String(), err)
		lastErr = err
	}

	// All protocols failed
	log.Printf("[NAT] ⚠ All NAT protocols failed")
	log.Printf("[NAT]   Last error: %v", lastErr)
	log.Printf("[NAT]   Node will continue to operate, but may not be reachable from outside")
	log.Printf("[NAT]   This is normal if: 1) Router doesn't support any protocol, 2) Protocols are disabled")

	return nil // Non-blocking - allow node to operate without NAT traversal
}

// tryForwardWithProtocol attempts port forwarding with a specific protocol
func (nm *NATManager) tryForwardWithProtocol(protocol NATProtocol) error {
	switch protocol {
	case NATProtocolPCP:
		return nm.forwardViaPCP()
	case NATProtocolNATPMP:
		return nm.forwardViaNATPMP()
	case NATProtocolUPnP:
		return nm.forwardViaUPnP()
	default:
		return fmt.Errorf("unsupported protocol: %s", protocol.String())
	}
}

// forwardViaPCP attempts port mapping using PCP (RFC 6887)
func (nm *NATManager) forwardViaPCP() error {
	nm.mu.Unlock()
	defer nm.mu.Lock()

	// Discover default gateway first
	gateway, err := nm.discoverDefaultGateway()
	if err != nil {
		return fmt.Errorf("failed to discover default gateway: %w", err)
	}

	nm.gatewayIP = gateway

	// Create PCP client
	pcpClient := NewPCPClient(gateway, net.ParseIP(nm.localIP), 6) // TCP=6

	// Request port mapping
	extPort, err := pcpClient.RequestPortMapping(nm.localPort, PCPDefaultLifetime)
	if err != nil {
		return err
	}

	// Success
	nm.forwarded = true
	nm.externalPort = int(extPort)
	nm.lastUpdated = time.Now()

	return nil
}

// forwardViaNATPMP attempts port mapping using NAT-PMP (RFC 6886)
func (nm *NATManager) forwardViaNATPMP() error {
	nm.mu.Unlock()
	defer nm.mu.Lock()

	// Discover default gateway first
	gateway, err := nm.discoverDefaultGateway()
	if err != nil {
		return fmt.Errorf("failed to discover default gateway: %w", err)
	}

	nm.gatewayIP = gateway

	// Create NAT-PMP client
	natpmpClient := NewNATPMPClient(gateway)

	// Request external address first
	extIP, _, err := natpmpClient.GetExternalAddress()
	if err != nil {
		return fmt.Errorf("failed to get external address: %w", err)
	}

	// Request port mapping
	extPort, err := natpmpClient.MapPort("TCP", nm.localPort, 0, NATPMPDefaultTTL)
	if err != nil {
		return err
	}

	// Success
	nm.forwarded = true
	nm.externalIP = extIP.String()
	nm.externalPort = int(extPort)
	nm.lastUpdated = time.Now()

	return nil
}

// forwardViaUPnP attempts port mapping using UPnP
func (nm *NATManager) forwardViaUPnP() error {
	// Discover gateway if needed
	if !nm.discovered || nm.controlURL == "" {
		nm.mu.Unlock()
		err := nm.DiscoverGateway()
		nm.mu.Lock()
		if err != nil {
			return fmt.Errorf("UPnP gateway discovery failed: %w", err)
		}
	}

	// Get external IP first
	nm.mu.Unlock()
	extIP, err := nm.GetExternalIPAddress()
	nm.mu.Lock()
	if err != nil {
		log.Printf("[UPnP] Warning: failed to get external IP: %v", err)
		extIP = ""
	}

	// Try requested port first, then random ports as fallback
	portsToTry := []int{nm.localPort}
	for i := 0; i < UPnPPortRetryAttempts; i++ {
		portsToTry = append(portsToTry, randomPort())
	}

	var lastErr error
	for attempt, portToTry := range portsToTry {
		log.Printf("[UPnP] Attempt %d/%d: mapping external port %d to internal port %d",
			attempt+1, len(portsToTry), portToTry, nm.localPort)

		// Release lock during network operation
		nm.mu.Unlock()
		mapErr := nm.addPortMappingInternal(portToTry, nm.localPort, "TCP")
		nm.mu.Lock()

		if mapErr == nil {
			// Success!
			nm.forwarded = true
			nm.externalPort = portToTry
			nm.lastUpdated = time.Now()
			if extIP != "" {
				nm.externalIP = extIP
			}

			log.Printf("[UPnP] ✓ Port mapping successful (attempt %d)!", attempt+1)
			log.Printf("[UPnP]   External: %s:%d", nm.externalIP, nm.externalPort)
			log.Printf("[UPnP]   Internal: %s:%d", nm.localIP, nm.localPort)
			log.Printf("[UPnP]   Lease: %d seconds", UPnPMapLeaseDuration)

			return nil
		}

		log.Printf("[UPnP] Port mapping attempt %d failed: %v", attempt+1, mapErr)
		lastErr = mapErr

		// Wait before retrying (except for first attempt)
		if attempt > 0 {
			nm.mu.Unlock()
			time.Sleep(UPnPPortRetryInterval)
			nm.mu.Lock()
		}
	}

	// All attempts failed
	return lastErr
}

// discoverDefaultGateway discovers the default gateway (IPv4)
// Common to all protocols
func (nm *NATManager) discoverDefaultGateway() (net.IP, error) {
	// Simple method: try common gateway addresses
	commonGateways := []string{
		"192.168.1.1",
		"192.168.0.1",
		"192.168.2.1",
		"10.0.0.1",
		"10.0.0.138",
		"172.16.0.1",
	}

	// Try each gateway
	for _, gwAddr := range commonGateways {
		gw := net.ParseIP(gwAddr)
		if gw == nil {
			continue
		}

		// Quick connectivity test
		conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:%d", gwAddr, NATPMPClientPort), 1*time.Second)
		if err == nil {
			conn.Close()
			log.Printf("[NAT] Discovered default gateway: %s", gwAddr)
			return gw, nil
		}
	}

	return nil, errors.New("unable to discover default gateway")
}

// ForwardPort creates a port mapping on the UPnP gateway with fallback and retry
// Implements Core-geth and Bitcoin-inspired retry strategy:
// 1. Try requested port first
// 2. If fails, retry with random ports (up to UPnPPortRetryAttempts times)
// 3. If all fails, log warning but continue (non-blocking)
// DEPRECATED: Use ForwardPortMultiProtocol() instead
func (nm *NATManager) ForwardPort() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Check if already forwarded
	if nm.forwarded && nm.externalIP != "" && nm.externalPort > 0 {
		log.Printf("[NAT] Port already forwarded: %d (local %d -> external %d)",
			nm.localPort, nm.localPort, nm.externalPort)
		return nil
	}

	// Discover gateway if needed
	if !nm.discovered || nm.controlURL == "" {
		nm.mu.Unlock()
		err := nm.DiscoverGateway()
		nm.mu.Lock()
		if err != nil {
			log.Printf("[NAT] Gateway discovery failed: %v", err)
			return fmt.Errorf("gateway discovery failed: %w", err)
		}
	}

	log.Printf("[NAT] Creating port mapping: local %s:%d -> external ?",
		nm.localIP, nm.localPort)

	// Get external IP first
	nm.mu.Unlock()
	extIP, err := nm.GetExternalIPAddress()
	nm.mu.Lock()
	if err != nil {
		log.Printf("[NAT] Warning: failed to get external IP: %v", err)
		extIP = ""
	}

	// Try requested port first, then random ports as fallback
	portsToTry := []int{nm.localPort}
	for i := 0; i < UPnPPortRetryAttempts; i++ {
		portsToTry = append(portsToTry, randomPort())
	}

	var lastErr error
	for attempt, portToTry := range portsToTry {
		log.Printf("[NAT] Attempt %d/%d: mapping external port %d to internal port %d",
			attempt+1, len(portsToTry), portToTry, nm.localPort)

		// Release lock during network operation
		nm.mu.Unlock()
		mapErr := nm.addPortMappingInternal(portToTry, nm.localPort, "TCP")
		nm.mu.Lock()

		if mapErr == nil {
			// Success!
			nm.forwarded = true
			nm.externalPort = portToTry
			nm.lastUpdated = time.Now()
			if extIP != "" {
				nm.externalIP = extIP
			}

			log.Printf("[NAT] ✓ Port mapping successful (attempt %d)!", attempt+1)
			log.Printf("[NAT]   External: %s:%d", nm.externalIP, nm.externalPort)
			log.Printf("[NAT]   Internal: %s:%d", nm.localIP, nm.localPort)
			log.Printf("[NAT]   Lease: %d seconds", UPnPMapLeaseDuration)

			return nil
		}

		log.Printf("[NAT] Port mapping attempt %d failed: %v", attempt+1, mapErr)
		lastErr = mapErr

		// Wait before retrying (except for the first attempt)
		if attempt > 0 {
			nm.mu.Unlock()
			time.Sleep(UPnPPortRetryInterval)
			nm.mu.Lock()
		}
	}

	// All attempts failed - log but don't return error (non-blocking)
	log.Printf("[NAT] ⚠ All port mapping attempts failed (tried %d ports)", len(portsToTry))
	log.Printf("[NAT]   Last error: %v", lastErr)
	log.Printf("[NAT]   Node will continue to operate, but may not be reachable from outside")
	log.Printf("[NAT]   This is normal if: 1) Router doesn't support UPnP, 2) UPnP is disabled, 3) Behind multiple NAT layers")

	// Don't return error - allow node to operate without NAT traversal
	return nil
}

// DeletePortMapping removes the port mapping
func (nm *NATManager) DeletePortMapping() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.forwarded || nm.controlURL == "" {
		return nil
	}

	log.Printf("[NAT] Deleting port mapping: %d", nm.localPort)

	soapBody := fmt.Sprintf(`<?xml version="1.0"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
soap:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <soap:Body>
    <u:DeletePortMapping xmlns:u="%s">
      <NewRemoteHost></NewRemoteHost>
      <NewExternalPort>%d</NewExternalPort>
      <NewProtocol>TCP</NewProtocol>
    </u:DeletePortMapping>
  </soap:Body>
</soap:Envelope>`, nm.serviceType, nm.localPort)

	req, err := http.NewRequest("POST", nm.controlURL, strings.NewReader(soapBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Set("SOAPAction", fmt.Sprintf("\"%s#DeletePortMapping\"", nm.serviceType))

	resp, err := nm.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send delete request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete mapping request failed with status %d", resp.StatusCode)
	}

	nm.forwarded = false
	log.Printf("[NAT] Port mapping deleted successfully")
	return nil
}

// RefreshConnection refreshes the port mapping
func (nm *NATManager) RefreshConnection() error {
	// Check if mapping is stale
	nm.mu.RLock()
	stale := time.Since(nm.lastUpdated) > UPnPRefreshInterval
	nm.mu.RUnlock()

	if stale {
		log.Printf("[NAT] Port mapping might be stale, refreshing...")
		// Re-forward the port to refresh the lease
		return nm.ForwardPort()
	}

	return nil
}

// GetNATStatus returns current NAT status
func (nm *NATManager) GetNATStatus() string {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	if nm.externalIP != "" && nm.forwarded {
		return fmt.Sprintf("UPnP: %s:%d (mapped)", nm.externalIP, nm.externalPort)
	}

	if nm.gateway != "" {
		return "UPnP: Gateway found, port mapping in progress..."
	}

	return "NAT: No external access (UPnP not discovered)"
}

// StartAutoRefresh starts automatic port mapping refresh
func (nm *NATManager) StartAutoRefresh(ctx context.Context) {
	nm.mu.Lock()
	if nm.cancel != nil {
		nm.mu.Unlock()
		return
	}
	ctx, nm.cancel = context.WithCancel(ctx)
	nm.mu.Unlock()

	ticker := time.NewTicker(UPnPRefreshInterval)
	defer ticker.Stop()

	log.Printf("[NAT] Started auto-refresh (interval: %v)", UPnPRefreshInterval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[NAT] Stopping auto-refresh")
			return
		case <-ticker.C:
			if err := nm.RefreshConnection(); err != nil {
				log.Printf("[NAT] Auto-refresh error: %v", err)
			}
		}
	}
}

// StopAutoRefresh stops automatic port mapping refresh
func (nm *NATManager) StopAutoRefresh() {
	nm.mu.Lock()
	if nm.cancel != nil {
		nm.cancel()
		nm.cancel = nil
	}
	nm.mu.Unlock()
}

// Cleanup removes port mapping and stops auto-refresh
func (nm *NATManager) Cleanup() {
	log.Printf("[NAT] Cleaning up NAT manager...")
	nm.StopAutoRefresh()
	if err := nm.DeletePortMapping(); err != nil {
		log.Printf("[NAT] Error deleting port mapping: %v", err)
	}
}

// readAll reads entire response body
func readAll(r io.Reader) (string, error) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r)
	return buf.String(), err
}
