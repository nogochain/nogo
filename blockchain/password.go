package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxPasswordBytes = 1024 * 1024

func ReadWalletPassword(passwordArg string, prompt string, confirm bool) (string, error) {
	if pass, ok, err := passwordFromFileEnv(); err != nil {
		return "", err
	} else if ok {
		return pass, nil
	}

	if pass := strings.TrimSpace(os.Getenv("WALLET_PASSWORD")); pass != "" {
		if pass == "-" {
			return readPasswordFromPipe(os.Stdin)
		}
		return pass, nil
	}

	if strings.TrimSpace(passwordArg) != "" {
		fmt.Fprintln(os.Stderr, "warning: password provided on command line is insecure; prefer WALLET_PASSWORD_FILE, WALLET_PASSWORD, or stdin prompt")
		return passwordArg, nil
	}

	if isTerminalStdin() {
		return readPasswordFromPrompt(prompt, confirm)
	}

	return readPasswordFromPipe(os.Stdin)
}

func passwordFromFileEnv() (string, bool, error) {
	path := strings.TrimSpace(os.Getenv("WALLET_PASSWORD_FILE"))
	if path == "" {
		return "", false, nil
	}
	if path == "-" {
		pass, err := readPasswordFromPipe(os.Stdin)
		return pass, err == nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	pass := strings.TrimSpace(string(b))
	if pass == "" {
		return "", false, errors.New("WALLET_PASSWORD_FILE is empty")
	}
	return pass, true, nil
}

func readPasswordFromPrompt(prompt string, confirm bool) (string, error) {
	if prompt == "" {
		prompt = "Password: "
	}
	pass, ok, err := readPasswordNoEcho(prompt)
	if err != nil {
		return "", err
	}
	if !ok {
		pass, err = readPasswordEcho(prompt)
		if err != nil {
			return "", err
		}
	}
	if pass == "" {
		return "", errors.New("empty password")
	}
	if confirm {
		confirmPass, ok, err := readPasswordNoEcho("Confirm password: ")
		if err != nil {
			return "", err
		}
		if !ok {
			confirmPass, err = readPasswordEcho("Confirm password: ")
			if err != nil {
				return "", err
			}
		}
		if pass != confirmPass {
			return "", errors.New("passwords do not match")
		}
	}
	return pass, nil
}

func readPasswordEcho(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	r := bufio.NewReader(os.Stdin)
	s, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}

func readPasswordFromPipe(r io.Reader) (string, error) {
	lr := &io.LimitedReader{R: r, N: maxPasswordBytes + 1}
	b, err := io.ReadAll(lr)
	if err != nil {
		return "", err
	}
	if len(b) > maxPasswordBytes {
		return "", errors.New("password too large")
	}
	pass := strings.TrimSpace(string(b))
	if pass == "" {
		return "", errors.New("missing password (set WALLET_PASSWORD_FILE, WALLET_PASSWORD, or provide via stdin)")
	}
	return pass, nil
}

func isTerminalStdin() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
