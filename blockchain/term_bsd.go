//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func readPasswordNoEcho(prompt string) (password string, ok bool, err error) {
	if !isTerminalStdin() {
		return "", false, nil
	}

	fd := int(os.Stdin.Fd())
	oldState, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return "", false, nil
	}
	newState := *oldState
	newState.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, &newState); err != nil {
		return "", false, nil
	}
	defer func() { _ = unix.IoctlSetTermios(fd, unix.TIOCSETA, oldState) }()

	fmt.Fprint(os.Stderr, prompt)
	r := bufio.NewReader(os.Stdin)
	s, err := r.ReadString('\n')
	fmt.Fprintln(os.Stderr)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", false, nil
	}
	return strings.TrimRight(s, "\r\n"), true, nil
}
