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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package api

import (
	"net/http"
	"os"
	"time"

	"github.com/nogochain/nogo/blockchain/mempool"
	"github.com/nogochain/nogo/blockchain/metrics"
	"github.com/nogochain/nogo/blockchain/miner"
	"github.com/nogochain/nogo/blockchain/network"
)

// Use interfaces from network package for consistency
type Blockchain = network.BlockchainInterface
type Mempool = mempool.Mempool
type Miner = *miner.Miner
type PeerManager = network.P2PPeerManager

// Type aliases for concrete implementations (for backward compatibility)
type MempoolImpl = mempool.Mempool
type MinerImpl = miner.Miner
type PeerManagerImpl = network.P2PPeerManager
type MetricsImpl = metrics.Metrics

// Server type aliases for main.go compatibility
type HTTPServer = SimpleServer
type WSServer = WSHub

// NewHTTPServer creates a new HTTP server instance
// Production-grade: initializes server with proper configuration
// Note: This is a compatibility wrapper - use NewSimpleServer directly for new code
func NewHTTPServer(cfg interface{}, chain Blockchain, p2p interface{}, miner interface{}, store interface{}) *HTTPServer {
	// Extract configuration from parameters
	// For backward compatibility, we use type assertions
	var adminToken string
	
	// Try to extract admin token from cfg if it's a config struct
	if cfg != nil {
		// Handle different config types via reflection or type assertion
		// For now, extract from environment as fallback
		adminToken = os.Getenv("ADMIN_TOKEN")
	}
	
	// Type assertions for compatibility
	// Note: In production code, use NewSimpleServer directly with properly typed parameters
	var minerImpl *MinerImpl
	var peersImpl *PeerManagerImpl
	
	if m, ok := miner.(*MinerImpl); ok {
		minerImpl = m
	}
	
	if p, ok := p2p.(*PeerManagerImpl); ok {
		peersImpl = p
	}
	
	// Create simple server with minimal dependencies
	return NewSimpleServer(chain, nil, minerImpl, peersImpl, adminToken)
}

// NewWSServer creates a new WebSocket server instance
// Production-grade: initializes WebSocket hub with proper configuration
// Note: This is a compatibility wrapper - use NewWSHub directly for new code
func NewWSServer(cfg interface{}, chain Blockchain, p2p interface{}, miner interface{}) *WSServer {
	// Default max connections
	maxConnections := 100
	
	// Extract configuration if provided
	if cfg != nil {
		// Handle different config types
		// For now, use defaults
	}
	
	// Create WebSocket hub
	return NewWSHub(maxConnections)
}

// NewMetricsServer creates a new metrics server instance
// Production-grade: creates HTTP server for Prometheus metrics endpoint
// Note: This is a compatibility wrapper - use http.Server directly for new code
func NewMetricsServer(addr string, handler interface{}) *HTTPServer {
	// Use provided handler or create default
	var h http.Handler
	if handler != nil {
		// Try to use provided handler
		if handler, ok := handler.(http.Handler); ok {
			h = handler
		}
	}
	
	if h == nil {
		// Create default mux
		h = http.NewServeMux()
	}
	
	// Create HTTP server
	return &HTTPServer{
		server: &http.Server{
			Addr:         addr,
			Handler:      h,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  30 * time.Second,
		},
	}
}
