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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"

	"github.com/nogochain/nogo/blockchain"
)

// APIError represents a structured API error
// Production-grade: implements error interface, supports error wrapping
type APIError struct {
	code       ErrorCode
	message    string
	details    map[string]any
	wrappedErr error
	file       string
	line       int
}

// Error implements the error interface
func (e *APIError) Error() string {
	if e.wrappedErr != nil {
		return fmt.Sprintf("%s: %s", e.code.String(), e.wrappedErr.Error())
	}
	return e.message
}

// Unwrap returns the wrapped error for errors.Is/errors.As support
func (e *APIError) Unwrap() error {
	return e.wrappedErr
}

// Code returns the error code
func (e *APIError) Code() ErrorCode {
	return e.code
}

// Message returns the error message
func (e *APIError) Message() string {
	return e.message
}

// Details returns the error details
func (e *APIError) Details() map[string]any {
	return e.details
}

// File returns the source file where the error was created
func (e *APIError) File() string {
	return e.file
}

// Line returns the line number where the error was created
func (e *APIError) Line() int {
	return e.line
}

// HTTPStatus returns the HTTP status code for this error
func (e *APIError) HTTPStatus() int {
	return e.code.HTTPStatusCode()
}

// ErrorOption is a function that configures an APIError
type ErrorOption func(*APIError)

// WithDetails adds details to the error
func WithDetails(details map[string]any) ErrorOption {
	return func(e *APIError) {
		e.details = details
	}
}

// WithWrappedError wraps another error
func WithWrappedError(err error) ErrorOption {
	return func(e *APIError) {
		e.wrappedErr = err
	}
}

// NewAPIError creates a new API error with the given code and message
// Production-grade: captures source location, supports error wrapping
func NewAPIError(code ErrorCode, message string, opts ...ErrorOption) *APIError {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
		line = 0
	}

	e := &APIError{
		code:    code,
		message: message,
		details: make(map[string]any),
		file:    file,
		line:    line,
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// WrapError wraps an existing error with an error code
func WrapError(code ErrorCode, err error, message string, opts ...ErrorOption) *APIError {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
		line = 0
	}

	e := &APIError{
		code:       code,
		message:    message,
		details:    make(map[string]any),
		wrappedErr: err,
		file:       file,
		line:       line,
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// ErrorResponse represents the JSON response structure for errors
// Production-grade: matches the specified format exactly
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody represents the error body in the response
type ErrorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

// ToResponse converts an APIError to an ErrorResponse
func (e *APIError) ToResponse(requestID string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:      e.code.String(),
			Message:   e.message,
			Details:   e.details,
			RequestID: requestID,
		},
	}
}

// ErrorMapper maps internal errors to error codes
// Production-grade: supports errors.Is and errors.As for error chain traversal
type ErrorMapper struct {
	mappings map[string]ErrorCode
}

// NewErrorMapper creates a new error mapper
func NewErrorMapper() *ErrorMapper {
	return &ErrorMapper{
		mappings: make(map[string]ErrorCode),
	}
}

// Register registers an error to error code mapping
func (m *ErrorMapper) Register(err error, code ErrorCode) {
	if err == nil {
		return
	}
	mappings := m.mappings
	mappings[err.Error()] = code
	m.mappings = mappings
}

// Map maps an error to an APIError
// Production-grade: traverses error chain using errors.Is/errors.As
func (m *ErrorMapper) Map(err error, defaultMessage string) *APIError {
	if err == nil {
		return nil
	}

	// Check if it's already an APIError
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}

	// Check for known error patterns
	for errStr, code := range m.mappings {
		if errors.Is(err, err) || err.Error() == errStr {
			return NewAPIError(code, defaultMessage, WithWrappedError(err))
		}
	}

	// Check for specific error types
	switch {
	case errors.Is(err, blockchain.ErrInvalidSignature):
		return NewAPIError(ErrorCodeInvalidSignature, "invalid signature", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInsufficientFunds):
		return NewAPIError(ErrorCodeInsufficientBalance, "insufficient funds", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidBlock):
		return NewAPIError(ErrorCodeBlockchain, "invalid block", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidTransaction):
		return NewAPIError(ErrorCodeValidationGeneral, "invalid transaction", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrDuplicateTransaction):
		return NewAPIError(ErrorCodeDuplicateTx, "duplicate transaction", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrNotFound):
		return NewAPIError(ErrorCodeNotFoundGeneral, "resource not found", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrPeerNotConnected):
		return NewAPIError(ErrorCodePeerNotFound, "peer not connected", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrBroadcastFailed):
		return NewAPIError(ErrorCodeNetwork, "broadcast failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrConnectionClosed):
		return NewAPIError(ErrorCodeNetwork, "connection closed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrHandshakeFailed):
		return NewAPIError(ErrorCodeNetwork, "handshake failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidMessage):
		return NewAPIError(ErrorCodeInvalidFieldFormat, "invalid message", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrRateLimited):
		return NewAPIError(ErrorCodeRateLimitedGeneral, "rate limited", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrTimeout):
		return NewAPIError(ErrorCodeNetwork, "operation timeout", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidPeer):
		return NewAPIError(ErrorCodeInvalidFieldFormat, "invalid peer", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrPeerBlacklisted):
		return NewAPIError(ErrorCodeForbidden, "peer blacklisted", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidHeight):
		return NewAPIError(ErrorCodeInvalidHeight, "invalid height", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidHash):
		return NewAPIError(ErrorCodeInvalidHash, "invalid hash", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidMerkleRoot):
		return NewAPIError(ErrorCodeMerkleProof, "invalid merkle root", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidTimestamp):
		return NewAPIError(ErrorCodeInvalidFieldFormat, "invalid timestamp", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrForkDetected):
		return NewAPIError(ErrorCodeFork, "fork detected", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrOrphanBlock):
		return NewAPIError(ErrorCodeOrphanBlock, "orphan block", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrGenesisMismatch):
		return NewAPIError(ErrorCodeConsensus, "genesis mismatch", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrRulesMismatch):
		return NewAPIError(ErrorCodeConsensus, "consensus rules mismatch", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrDatabaseOpen):
		return NewAPIError(ErrorCodeDatabase, "database open failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrDatabaseRead):
		return NewAPIError(ErrorCodeDatabase, "database read failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrDatabaseWrite):
		return NewAPIError(ErrorCodeDatabase, "database write failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrDatabaseClose):
		return NewAPIError(ErrorCodeDatabase, "database close failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrCorruptedData):
		return NewAPIError(ErrorCodeDatabase, "corrupted data", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrHTTPRequest):
		return NewAPIError(ErrorCodeNetwork, "HTTP request failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrHTTPResponse):
		return NewAPIError(ErrorCodeNetwork, "HTTP response failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidJSON):
		return NewAPIError(ErrorCodeInvalidJSON, "invalid JSON", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrUnauthorized):
		return NewAPIError(ErrorCodeUnauthorized, "unauthorized", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrForbidden):
		return NewAPIError(ErrorCodeForbidden, "forbidden", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrBadRequest):
		return NewAPIError(ErrorCodeValidationGeneral, "bad request", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInternalServer):
		return NewAPIError(ErrorCodeInternalGeneral, "internal server error", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrVMExecution):
		return NewAPIError(ErrorCodeVM, "VM execution failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidContract):
		return NewAPIError(ErrorCodeInvalidContract, "invalid contract", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrContractExecution):
		return NewAPIError(ErrorCodeContract, "contract execution failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrOutOfGas):
		return NewAPIError(ErrorCodeVM, "out of gas", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidOpcode):
		return NewAPIError(ErrorCodeVM, "invalid opcode", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrKeyGeneration):
		return NewAPIError(ErrorCodeCrypto, "key generation failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrKeyImport):
		return NewAPIError(ErrorCodeCrypto, "key import failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrEncryption):
		return NewAPIError(ErrorCodeCrypto, "encryption failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrDecryption):
		return NewAPIError(ErrorCodeCrypto, "decryption failed", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidAddress):
		return NewAPIError(ErrorCodeInvalidAddress, "invalid address", WithWrappedError(err))
	case errors.Is(err, blockchain.ErrInvalidMnemonic):
		return NewAPIError(ErrorCodeInvalidMnemonic, "invalid mnemonic", WithWrappedError(err))
	}

	// Default to internal error
	return NewAPIError(ErrorCodeInternalGeneral, defaultMessage, WithWrappedError(err))
}

// Global error mapper instance
var globalErrorMapper = NewErrorMapper()

// MapError maps an error to an APIError using the global mapper
func MapError(err error, defaultMessage string) *APIError {
	return globalErrorMapper.Map(err, defaultMessage)
}

// WriteError writes an error response to the HTTP response writer
func WriteError(w http.ResponseWriter, err *APIError, requestID string) error {
	response := err.ToResponse(requestID)
	status := err.HTTPStatus()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(response)
}

// WriteErrorResponse writes an error response directly
func WriteErrorResponse(w http.ResponseWriter, code ErrorCode, message string, details map[string]any, requestID string) error {
	err := NewAPIError(code, message, WithDetails(details))
	return WriteError(w, err, requestID)
}

// MustGetErrorCode returns the error code for a known error, or ErrorCodeInternalGeneral if unknown
func MustGetErrorCode(err error) ErrorCode {
	if err == nil {
		return ErrorCodeUnknown
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code()
	}

	mapped := MapError(err, "unknown error")
	return mapped.Code()
}

// generateRequestID generates a unique request ID
// Production-grade: uses crypto/rand for secure random generation
func generateRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "req_unknown"
	}
	return "req_" + hex.EncodeToString(bytes[:8])
}

// getRequestID extracts request ID from headers or generates a new one
func getRequestID(r *http.Request) string {
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = generateRequestID()
	}
	return requestID
}
