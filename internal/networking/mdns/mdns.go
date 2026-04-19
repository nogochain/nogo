package mdns

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/grandcat/zeroconf"
)

const (
	serviceType  = "_nogochain"
	serviceProto = "_tcp"
	domain       = "local."

	txtVersionKey = "version"
	txtNodeIDKey  = "node_id"
	txtChainIDKey = "chain_id"
	txtPortKey    = "tcp_port"

	eventBufferSize = 100
)

var (
	ErrInvalidPort     = fmt.Errorf("port must be between 1 and 65535")
	ErrInvalidNodeID   = fmt.Errorf("node ID cannot be empty")
	ErrInvalidChainID  = fmt.Errorf("chain ID cannot be empty")
	ErrNilServiceEntry = fmt.Errorf("nil service entry")
	ErrNoIPv4Address   = fmt.Errorf("no suitable IPv4 address found")
)

type Service struct {
	mu      sync.RWMutex
	server  *zeroconf.Server
	running bool

	chainID string
	nodeID  string
	version string
	port    int

	ctx    context.Context
	cancel context.CancelFunc
}

func NewService(chainID, nodeID, version string) *Service {
	return &Service{
		chainID: chainID,
		nodeID:  nodeID,
		version: version,
	}
}

func serviceName(chainID string) string {
	return fmt.Sprintf("%s-%s", serviceType, chainID)
}

func serviceFullName(chainID string) string {
	return fmt.Sprintf("%s.%s.%s", serviceName(chainID), serviceProto, domain)
}

func (s *Service) Register(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("register service: %w, got %d", ErrInvalidPort, port)
	}
	if s.nodeID == "" {
		return fmt.Errorf("register service: %w", ErrInvalidNodeID)
	}
	if s.chainID == "" {
		return fmt.Errorf("register service: %w", ErrInvalidChainID)
	}

	hostIP, err := s.getLocalIP()
	if err != nil {
		return fmt.Errorf("register service: get local IP: %w", err)
	}

	s.port = port

	s.ctx, s.cancel = context.WithCancel(context.Background())

	info := map[string]string{
		txtVersionKey: s.version,
		txtNodeIDKey:  s.nodeID,
		txtChainIDKey: s.chainID,
		txtPortKey:    fmt.Sprintf("%d", port),
	}

	txtRecords := make([]string, 0, len(info))
	for k, v := range info {
		txtRecords = append(txtRecords, fmt.Sprintf("%s=%s", k, v))
	}

	server, err := zeroconf.RegisterProxy(
		s.nodeID,
		serviceName(s.chainID),
		domain,
		port,
		hostIP,
		[]string{hostIP},
		txtRecords,
		nil,
	)
	if err != nil {
		return fmt.Errorf("register service: zeroconf register: %w", err)
	}

	s.mu.Lock()
	s.server = server
	s.running = true
	s.mu.Unlock()

	return nil
}

func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

func (s *Service) GetPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.port
}

func (s *Service) GetServiceName() string {
	return serviceName(s.chainID)
}

func (s *Service) GetServiceFullName() string {
	return serviceFullName(s.chainID)
}

func (s *Service) getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("get local IP: enumerate interfaces: %w", err)
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipNet.IP
		if ip.IsLoopback() {
			continue
		}

		if ip.To4() != nil {
			return ip.String(), nil
		}
	}

	return "", fmt.Errorf("get local IP: %w", ErrNoIPv4Address)
}

func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}

	if s.server != nil {
		s.server.Shutdown()
		s.server = nil
	}

	s.running = false

	return nil
}
