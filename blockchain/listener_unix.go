//go:build linux || darwin

package main

import (
	"context"
	"net"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func createListener(addr string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				// Set SO_REUSEADDR for socket reuse
				unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
			})
		},
		KeepAlive: 3 * time.Minute,
	}

	return lc.Listen(context.Background(), "tcp", addr)
}
