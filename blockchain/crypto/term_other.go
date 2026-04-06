//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package crypto

func readPasswordNoEcho(prompt string) (password string, ok bool, err error) {
	return "", false, nil
}
