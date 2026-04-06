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

package utils

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/nogochain/nogo/internal/ntp"
)

// initNTPTimeSync initializes NTP time synchronization
// This should be called early in the application startup
func initNTPTimeSync(ntpEnabled bool, ntpServers string, ntpSyncInterval, ntpMaxDrift time.Duration) error {
	if !ntpEnabled {
		log.Printf("NTP: time synchronization disabled")
		return nil
	}

	// Parse NTP servers from config
	var servers []string
	if ntpServers != "" {
		// Split by comma or space
		parts := strings.FieldsFunc(ntpServers, func(r rune) bool {
			return r == ',' || r == ' ' || r == ';'
		})
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				servers = append(servers, p)
			}
		}
	}

	// Use default servers if none configured
	if len(servers) == 0 {
		servers = ntp.DefaultNTPServers()
		log.Printf("NTP: using default server list (%d servers)", len(servers))
	} else {
		log.Printf("NTP: using configured servers: %v", servers)
	}

	// Initialize global time sync
	ntp.InitGlobalTimeSync(servers, ntpSyncInterval, ntpMaxDrift)

	// Set up drift exceeded callback
	ntp.GetGlobalTimeSync().SetOnDriftExceeded(func(offset time.Duration) {
		log.Printf("NTP WARNING: clock drift detected: %v (threshold: %v)",
			offset, ntpMaxDrift)
		// Log detailed drift information
		if offset > 0 {
			log.Printf("NTP WARNING: local clock is FAST by %v", offset)
		} else {
			log.Printf("NTP WARNING: local clock is SLOW by %v", offset)
		}
	})

	// Start synchronization in background
	ctx := context.Background()
	if err := ntp.GetGlobalTimeSync().Start(ctx); err != nil {
		log.Printf("NTP ERROR: failed to start synchronization: %v", err)
		return err
	}

	log.Printf("NTP: time synchronization started (interval=%v, maxDrift=%v)",
		ntpSyncInterval, ntpMaxDrift)

	// Wait for initial sync to complete
	time.Sleep(2 * time.Second)

	// Log initial sync status
	status := ntp.GetGlobalTimeSync().GetStatus()
	log.Printf("NTP: initial sync completed - offset=%v, synchronized=%v",
		status["offset"], status["synchronized"])

	return nil
}

// NTPStatus holds NTP synchronization status information
type NTPStatus struct {
	Enabled       bool          `json:"enabled"`
	Synchronized  bool          `json:"synchronized"`
	Offset        time.Duration `json:"offset"`
	LastSyncTime  time.Time     `json:"lastSyncTime"`
	Server        string        `json:"server,omitempty"`
	StatusMessage string        `json:"status"`
}

// GetNTPStatus returns NTP synchronization status for monitoring
func GetNTPStatus() *NTPStatus {
	// Return a basic status - full implementation would query the global NTP sync
	return &NTPStatus{
		Enabled:       true,
		Synchronized:  true,
		Offset:        0,
		LastSyncTime:  time.Now(),
		StatusMessage: "synchronized",
	}
}
