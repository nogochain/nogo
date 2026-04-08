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
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nogochain/nogo/blockchain"
)

func TestErrorCodeString(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrorCodeUnknown, "UNKNOWN"},
		{ErrorCodeValidationGeneral, "VALIDATION_ERROR"},
		{ErrorCodeInvalidJSON, "INVALID_JSON"},
		{ErrorCodeTxNotFound, "TX_NOT_FOUND"},
		{ErrorCodeBlockNotFound, "BLOCK_NOT_FOUND"},
		{ErrorCodeInternalGeneral, "INTERNAL_ERROR"},
		{ErrorCodeRateLimitedGeneral, "RATE_LIMITED"},
		{ErrorCodeAuthGeneral, "AUTH_ERROR"},
		{ErrorCode(9999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.code.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestErrorCodeHTTPStatusCode(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected int
	}{
		{ErrorCodeInvalidJSON, http.StatusBadRequest},
		{ErrorCodeInvalidAddress, http.StatusBadRequest},
		{ErrorCodeTxNotFound, http.StatusNotFound},
		{ErrorCodeBlockNotFound, http.StatusNotFound},
		{ErrorCodeProposalNotFound, http.StatusNotFound},
		{ErrorCodeInternalGeneral, http.StatusInternalServerError},
		{ErrorCodeDatabase, http.StatusInternalServerError},
		{ErrorCodeRateLimitedGeneral, http.StatusTooManyRequests},
		{ErrorCodeIPLimited, http.StatusTooManyRequests},
		{ErrorCodeUnauthorized, http.StatusUnauthorized},
		{ErrorCodeForbidden, http.StatusForbidden},
		{ErrorCodeInvalidToken, http.StatusUnauthorized},
		{ErrorCodeMethodNotAllowed, http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			if got := tt.code.HTTPStatusCode(); got != tt.expected {
				t.Errorf("HTTPStatusCode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestErrorCodeCategory(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrorCodeInvalidJSON, "VALIDATION_ERROR"},
		{ErrorCodeInsufficientBalance, "VALIDATION_ERROR"},
		{ErrorCodeTxNotFound, "NOT_FOUND"},
		{ErrorCodeBlockNotFound, "NOT_FOUND"},
		{ErrorCodeInternalGeneral, "INTERNAL_ERROR"},
		{ErrorCodeDatabase, "INTERNAL_ERROR"},
		{ErrorCodeRateLimitedGeneral, "RATE_LIMITED"},
		{ErrorCodeAuthGeneral, "AUTH_ERROR"},
		{ErrorCodeUnknown, "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			if got := tt.code.Category(); got != tt.expected {
				t.Errorf("Category() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNewAPIError(t *testing.T) {
	err := NewAPIError(ErrorCodeTxNotFound, "transaction not found")

	if err.Code() != ErrorCodeTxNotFound {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrorCodeTxNotFound)
	}

	if err.Message() != "transaction not found" {
		t.Errorf("Message() = %v, want %v", err.Message(), "transaction not found")
	}

	if err.HTTPStatus() != http.StatusNotFound {
		t.Errorf("HTTPStatus() = %v, want %v", err.HTTPStatus(), http.StatusNotFound)
	}

	if err.Details() == nil {
		t.Error("Details() should not be nil")
	}

	if len(err.Details()) != 0 {
		t.Error("Details() should be empty by default")
	}
}

func TestAPIErrorWithDetails(t *testing.T) {
	details := map[string]any{
		"txid":  "abc123",
		"field": "value",
	}

	err := NewAPIError(ErrorCodeTxNotFound, "transaction not found", WithDetails(details))

	if len(err.Details()) != 2 {
		t.Errorf("Details() length = %v, want %v", len(err.Details()), 2)
	}

	if err.Details()["txid"] != "abc123" {
		t.Errorf("Details['txid'] = %v, want %v", err.Details()["txid"], "abc123")
	}
}

func TestAPIErrorWithWrappedError(t *testing.T) {
	wrappedErr := errors.New("underlying error")
	err := NewAPIError(ErrorCodeDatabase, "database operation failed", WithWrappedError(wrappedErr))

	if err.Error() != "DATABASE_ERROR: underlying error" {
		t.Errorf("Error() = %v, want %v", err.Error(), "DATABASE_ERROR: underlying error")
	}

	if !errors.Is(err, wrappedErr) {
		t.Error("errors.Is() should return true for wrapped error")
	}

	unwrapped := errors.Unwrap(err)
	if unwrapped != wrappedErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, wrappedErr)
	}
}

func TestWrapError(t *testing.T) {
	underlyingErr := errors.New("connection refused")
	err := WrapError(ErrorCodeNetwork, underlyingErr, "network operation failed")

	if err.Code() != ErrorCodeNetwork {
		t.Errorf("Code() = %v, want %v", err.Code(), ErrorCodeNetwork)
	}

	if err.Message() != "network operation failed" {
		t.Errorf("Message() = %v, want %v", err.Message(), "network operation failed")
	}

	if err.Unwrap() != underlyingErr {
		t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), underlyingErr)
	}
}

func TestAPIErrorToResponse(t *testing.T) {
	details := map[string]any{
		"txid": "abc123",
	}

	err := NewAPIError(ErrorCodeTxNotFound, "transaction not found", WithDetails(details))
	response := err.ToResponse("req_123")

	if response.Error.Code != "TX_NOT_FOUND" {
		t.Errorf("Error.Code = %v, want %v", response.Error.Code, "TX_NOT_FOUND")
	}

	if response.Error.Message != "transaction not found" {
		t.Errorf("Error.Message = %v, want %v", response.Error.Message, "transaction not found")
	}

	if response.Error.RequestID != "req_123" {
		t.Errorf("Error.RequestID = %v, want %v", response.Error.RequestID, "req_123")
	}

	if response.Error.Details["txid"] != "abc123" {
		t.Errorf("Error.Details['txid'] = %v, want %v", response.Error.Details["txid"], "abc123")
	}
}

func TestErrorResponseJSONSerialization(t *testing.T) {
	details := map[string]any{
		"txid":    "abc123",
		"address": "NOGO1234567890",
		"count":   5,
	}

	err := NewAPIError(ErrorCodeTxNotFound, "transaction not found", WithDetails(details))
	response := err.ToResponse("req_123")

	jsonData, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		t.Fatalf("json.Marshal() error = %v", marshalErr)
	}

	var decoded ErrorResponse
	if unmarshalErr := json.Unmarshal(jsonData, &decoded); unmarshalErr != nil {
		t.Fatalf("json.Unmarshal() error = %v", unmarshalErr)
	}

	if decoded.Error.Code != response.Error.Code {
		t.Errorf("Decoded Error.Code = %v, want %v", decoded.Error.Code, response.Error.Code)
	}

	if decoded.Error.Message != response.Error.Message {
		t.Errorf("Decoded Error.Message = %v, want %v", decoded.Error.Message, response.Error.Message)
	}

	if decoded.Error.RequestID != response.Error.RequestID {
		t.Errorf("Decoded Error.RequestID = %v, want %v", decoded.Error.RequestID, response.Error.RequestID)
	}
}

func TestWriteError(t *testing.T) {
	err := NewAPIError(
		ErrorCodeTxNotFound,
		"transaction not found",
		WithDetails(map[string]any{"txid": "abc123"}),
	)

	recorder := httptest.NewRecorder()
	requestID := "test_req_123"

	writeErr := WriteError(recorder, err, requestID)
	if writeErr != nil {
		t.Fatalf("WriteError() error = %v", writeErr)
	}

	if recorder.Code != http.StatusNotFound {
		t.Errorf("Status code = %v, want %v", recorder.Code, http.StatusNotFound)
	}

	contentType := recorder.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %v, want %v", contentType, "application/json")
	}

	var response ErrorResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("JSON decode error = %v", err)
	}

	if response.Error.Code != "TX_NOT_FOUND" {
		t.Errorf("Response Error.Code = %v, want %v", response.Error.Code, "TX_NOT_FOUND")
	}

	if response.Error.RequestID != requestID {
		t.Errorf("Response Error.RequestID = %v, want %v", response.Error.RequestID, requestID)
	}
}

func TestWriteErrorResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	details := map[string]any{"field": "value"}
	requestID := "test_req_456"

	err := WriteErrorResponse(
		recorder,
		ErrorCodeInvalidAddress,
		"invalid address format",
		details,
		requestID,
	)
	if err != nil {
		t.Fatalf("WriteErrorResponse() error = %v", err)
	}

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %v, want %v", recorder.Code, http.StatusBadRequest)
	}

	var response ErrorResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("JSON decode error = %v", err)
	}

	if response.Error.Code != "INVALID_ADDRESS" {
		t.Errorf("Response Error.Code = %v, want %v", response.Error.Code, "INVALID_ADDRESS")
	}

	if response.Error.Message != "invalid address format" {
		t.Errorf("Response Error.Message = %v, want %v", response.Error.Message, "invalid address format")
	}
}

func TestErrorMapper(t *testing.T) {
	mapper := NewErrorMapper()

	testErr := errors.New("test error")
	mapper.Register(testErr, ErrorCodeValidationGeneral)

	mappedErr := mapper.Map(testErr, "default message")
	if mappedErr.Code() != ErrorCodeValidationGeneral {
		t.Errorf("Mapped code = %v, want %v", mappedErr.Code(), ErrorCodeValidationGeneral)
	}
}

func TestMapError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		defaultMessage string
		expectedCode   ErrorCode
	}{
		{"InvalidSignature", blockchain.ErrInvalidSignature, "signature invalid", ErrorCodeInvalidSignature},
		{"InsufficientFunds", blockchain.ErrInsufficientFunds, "insufficient funds", ErrorCodeInsufficientBalance},
		{"InvalidTransaction", blockchain.ErrInvalidTransaction, "invalid tx", ErrorCodeValidationGeneral},
		{"DuplicateTransaction", blockchain.ErrDuplicateTransaction, "duplicate tx", ErrorCodeDuplicateTx},
		{"NotFound", blockchain.ErrNotFound, "not found", ErrorCodeNotFoundGeneral},
		{"RateLimited", blockchain.ErrRateLimited, "rate limited", ErrorCodeRateLimitedGeneral},
		{"Unauthorized", blockchain.ErrUnauthorized, "unauthorized", ErrorCodeUnauthorized},
		{"Forbidden", blockchain.ErrForbidden, "forbidden", ErrorCodeForbidden},
		{"InvalidJSON", blockchain.ErrInvalidJSON, "invalid json", ErrorCodeInvalidJSON},
		{"Unknown", errors.New("unknown error"), "default", ErrorCodeInternalGeneral},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped := MapError(tt.err, tt.defaultMessage)
			if mapped.Code() != tt.expectedCode {
				t.Errorf("Mapped code = %v, want %v", mapped.Code(), tt.expectedCode)
			}
		})
	}
}

func TestMustGetErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCode
	}{
		{"NilError", nil, ErrorCodeUnknown},
		{"APIError", NewAPIError(ErrorCodeTxNotFound, "not found"), ErrorCodeTxNotFound},
		{"WrappedError", WrapError(ErrorCodeDatabase, errors.New("db error"), "db failed"), ErrorCodeDatabase},
		{"UnknownError", errors.New("unknown"), ErrorCodeInternalGeneral},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MustGetErrorCode(tt.err)
			if got != tt.expected {
				t.Errorf("MustGetErrorCode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAPIErrorSourceLocation(t *testing.T) {
	err := NewAPIError(ErrorCodeTxNotFound, "test error")

	if err.File() == "" || err.File() == "unknown" {
		t.Error("File() should contain source file path")
	}

	if err.Line() <= 0 {
		t.Error("Line() should be a positive number")
	}

	if !strings.HasSuffix(err.File(), "error_response_test.go") {
		t.Errorf("File() = %v, should end with error_response_test.go", err.File())
	}
}

func TestErrorResponseFormat(t *testing.T) {
	err := NewAPIError(
		ErrorCodeTxNotFound,
		"Transaction not found",
		WithDetails(map[string]any{
			"txid": "abc123...",
		}),
	)

	response := err.ToResponse("req_123")

	jsonData, marshalErr := json.MarshalIndent(response, "", "  ")
	if marshalErr != nil {
		t.Fatalf("json.MarshalIndent() error = %v", marshalErr)
	}

	expectedFormat := `{
  "error": {
    "code": "TX_NOT_FOUND",
    "message": "Transaction not found",
    "details": {
      "txid": "abc123..."
    },
    "request_id": "req_123"
  }
}`

	var expectedJSON map[string]any
	var actualJSON map[string]any

	if unmarshalErr := json.Unmarshal([]byte(expectedFormat), &expectedJSON); unmarshalErr != nil {
		t.Fatalf("Expected JSON unmarshal error = %v", unmarshalErr)
	}

	if unmarshalErr := json.Unmarshal(jsonData, &actualJSON); unmarshalErr != nil {
		t.Fatalf("Actual JSON unmarshal error = %v", unmarshalErr)
	}

	errorBody, ok := actualJSON["error"].(map[string]any)
	if !ok {
		t.Fatal("Response should have 'error' field")
	}

	if errorBody["code"] != "TX_NOT_FOUND" {
		t.Errorf("error.code = %v, want TX_NOT_FOUND", errorBody["code"])
	}

	if errorBody["message"] != "Transaction not found" {
		t.Errorf("error.message = %v, want Transaction not found", errorBody["message"])
	}

	details, ok := errorBody["details"].(map[string]any)
	if !ok {
		t.Fatal("error.details should be an object")
	}

	if details["txid"] != "abc123..." {
		t.Errorf("error.details.txid = %v, want abc123...", details["txid"])
	}

	if errorBody["request_id"] != "req_123" {
		t.Errorf("error.request_id = %v, want req_123", errorBody["request_id"])
	}
}

func TestErrorChainTraversal(t *testing.T) {
	baseErr := errors.New("base error")
	wrappedErr := WrapError(ErrorCodeDatabase, baseErr, "database failed")
	doubleWrappedErr := WrapError(ErrorCodeInternalGeneral, wrappedErr, "internal failed")

	var apiErr *APIError
	if !errors.As(doubleWrappedErr, &apiErr) {
		t.Fatal("errors.As should find APIError in chain")
	}

	if apiErr.Code() != ErrorCodeInternalGeneral {
		t.Errorf("Outer error code = %v, want %v", apiErr.Code(), ErrorCodeInternalGeneral)
	}

	if errors.Is(doubleWrappedErr, baseErr) {
		t.Log("errors.Is correctly traverses the chain")
	} else {
		t.Error("errors.Is should find base error in chain")
	}
}

func TestErrorResponseWithNilDetails(t *testing.T) {
	err := NewAPIError(ErrorCodeTxNotFound, "transaction not found")
	response := err.ToResponse("req_test")

	jsonData, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		t.Fatalf("json.Marshal() error = %v", marshalErr)
	}

	if bytes.Contains(jsonData, []byte("details")) {
		t.Error("JSON should not contain 'details' field when empty (omitempty)")
	}
}

func BenchmarkAPIErrorCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewAPIError(ErrorCodeTxNotFound, "transaction not found")
	}
}

func BenchmarkAPIErrorWithDetails(b *testing.B) {
	details := map[string]any{"txid": "abc123"}
	for i := 0; i < b.N; i++ {
		_ = NewAPIError(ErrorCodeTxNotFound, "transaction not found", WithDetails(details))
	}
}

func BenchmarkErrorResponseJSON(b *testing.B) {
	err := NewAPIError(ErrorCodeTxNotFound, "transaction not found", WithDetails(map[string]any{"txid": "abc123"}))
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(err.ToResponse("req_123"))
	}
}
