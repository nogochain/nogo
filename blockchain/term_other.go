//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package main

func readPasswordNoEcho(prompt string) (password string, ok bool, err error) {
	return "", false, nil
}
