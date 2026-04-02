//go:build windows

package main

import (
	"context"
	"net"
	"syscall"
	"time"
)

func createListener(addr string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// Set SO_REUSEADDR for socket reuse
				syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
		},
		KeepAlive: 3 * time.Minute,
	}

	return lc.Listen(context.Background(), "tcp", addr)
}
