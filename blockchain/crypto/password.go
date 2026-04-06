// Copyright 2026 The NogoChain Authors
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

package crypto

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

const (
	// MaxPasswordBytes is the maximum password size in bytes
	// Security: prevents memory exhaustion attacks
	MaxPasswordBytes = 1024 * 1024

	// MinPasswordLength is the minimum recommended password length
	MinPasswordLength = 12

	// MaxPasswordLength is the maximum password length
	MaxPasswordLength = 512
)

var (
	// ErrPasswordTooShort is returned when password is too short
	ErrPasswordTooShort = errors.New("password too short")

	// ErrPasswordTooLong is returned when password is too long
	ErrPasswordTooLong = errors.New("password too long")

	// ErrPasswordEmpty is returned when password is empty
	ErrPasswordEmpty = errors.New("password cannot be empty")

	// ErrPasswordMismatch is returned when passwords don't match
	ErrPasswordMismatch = errors.New("passwords do not match")

	// ErrPasswordFileEmpty is returned when password file is empty
	ErrPasswordFileEmpty = errors.New("password file is empty")

	// ErrPasswordTooLarge is returned when password from pipe is too large
	ErrPasswordTooLarge = errors.New("password too large")

	// ErrPasswordMissing is returned when no password is provided
	ErrPasswordMissing = errors.New("missing password")
)

// ReadWalletPassword reads wallet password from multiple sources
// Priority: WALLET_PASSWORD_FILE env > WALLET_PASSWORD env > argument > stdin
// Security: warns about insecure command-line password usage
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

// passwordFromFileEnv reads password from file specified in WALLET_PASSWORD_FILE env
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
		return "", false, fmt.Errorf("failed to read password file: %w", err)
	}

	pass := strings.TrimSpace(string(b))
	if pass == "" {
		return "", false, ErrPasswordFileEmpty
	}

	return pass, true, nil
}

// readPasswordFromPrompt reads password from terminal with optional confirmation
func readPasswordFromPrompt(prompt string, confirm bool) (string, error) {
	if prompt == "" {
		prompt = "Password: "
	}

	pass, ok, err := readPasswordNoEcho(prompt)
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	if !ok {
		pass, err = readPasswordEcho(prompt)
		if err != nil {
			return "", err
		}
	}

	if pass == "" {
		return "", ErrPasswordEmpty
	}

	if confirm {
		confirmPass, ok, err := readPasswordNoEcho("Confirm password: ")
		if err != nil {
			return "", fmt.Errorf("failed to read confirmation: %w", err)
		}

		if !ok {
			confirmPass, err = readPasswordEcho("Confirm password: ")
			if err != nil {
				return "", err
			}
		}

		if pass != confirmPass {
			return "", ErrPasswordMismatch
		}
	}

	return pass, nil
}

// readPasswordEcho reads password with visible characters (fallback)
func readPasswordEcho(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	r := bufio.NewReader(os.Stdin)
	s, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("failed to read password: %w", err)
	}
	return strings.TrimRight(s, "\r\n"), nil
}

// readPasswordFromPipe reads password from pipe/stdin
// Security: limits input size to prevent memory exhaustion
func readPasswordFromPipe(r io.Reader) (string, error) {
	lr := &io.LimitedReader{R: r, N: MaxPasswordBytes + 1}
	b, err := io.ReadAll(lr)
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	if len(b) > MaxPasswordBytes {
		return "", ErrPasswordTooLarge
	}

	pass := strings.TrimSpace(string(b))
	if pass == "" {
		return "", ErrPasswordMissing
	}

	return pass, nil
}

// isTerminalStdin checks if stdin is a terminal
func isTerminalStdin() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// ValidatePasswordStrength validates password strength
// Production-grade: checks length and character requirements
func ValidatePasswordStrength(password string) error {
	if len(password) == 0 {
		return ErrPasswordEmpty
	}

	if len(password) < MinPasswordLength {
		return fmt.Errorf("%w: minimum %d characters", ErrPasswordTooShort, MinPasswordLength)
	}

	if len(password) > MaxPasswordLength {
		return fmt.Errorf("%w: maximum %d characters", ErrPasswordTooLong, MaxPasswordLength)
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		return errors.New("password must contain uppercase, lowercase, digit, and special character")
	}

	return nil
}

// ValidatePasswordBasic performs basic password validation
// Less strict than ValidatePasswordStrength, suitable for simple use cases
func ValidatePasswordBasic(password string) error {
	if len(password) == 0 {
		return ErrPasswordEmpty
	}

	if len(password) < 8 {
		return fmt.Errorf("%w: minimum 8 characters", ErrPasswordTooShort)
	}

	return nil
}

// ReadPasswordInteractive reads password interactively with validation
// Production-grade: includes strength validation and confirmation
func ReadPasswordInteractive(prompt, confirmPrompt string, requireStrong bool) (string, error) {
	if prompt == "" {
		prompt = "Enter password: "
	}
	if confirmPrompt == "" {
		confirmPrompt = "Confirm password: "
	}

	for attempt := 0; attempt < 3; attempt++ {
		pass, ok, err := readPasswordNoEcho(prompt)
		if err != nil {
			return "", fmt.Errorf("failed to read password: %w", err)
		}

		if !ok {
			pass, err = readPasswordEcho(prompt)
			if err != nil {
				return "", err
			}
		}

		if requireStrong {
			if err := ValidatePasswordStrength(pass); err != nil {
				fmt.Fprintf(os.Stderr, "weak password: %v\n", err)
				continue
			}
		} else {
			if err := ValidatePasswordBasic(pass); err != nil {
				fmt.Fprintf(os.Stderr, "invalid password: %v\n", err)
				continue
			}
		}

		confirmPass, ok, err := readPasswordNoEcho(confirmPrompt)
		if err != nil {
			return "", fmt.Errorf("failed to read confirmation: %w", err)
		}

		if !ok {
			confirmPass, err = readPasswordEcho(confirmPrompt)
			if err != nil {
				return "", err
			}
		}

		if pass != confirmPass {
			fmt.Fprintln(os.Stderr, "passwords do not match")
			continue
		}

		return pass, nil
	}

	return "", errors.New("failed to enter valid password after 3 attempts")
}

// SecureZero clears password from memory
// Security: zeros out password bytes to prevent memory leaks
func SecureZero(password string) {
	if len(password) == 0 {
		return
	}

	b := []byte(password)
	for i := range b {
		b[i] = 0
	}
}
