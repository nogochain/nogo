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
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// contextKeyRequestID is the context key for request ID
type contextKeyRequestID struct{}

// GetRequestIDFromContext retrieves request ID from context
func GetRequestIDFromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value(contextKeyRequestID{}).(string); ok {
		return requestID
	}
	return generateRequestID()
}

// WithRequestID adds request ID to context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, contextKeyRequestID{}, requestID)
}

// ErrorHandler is a middleware that handles errors and ensures consistent error responses
type ErrorHandler struct {
	next http.Handler
}

// ServeHTTP implements http.Handler
func (eh *ErrorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestID(r)
	
	// Add request ID to context
	ctx := WithRequestID(r.Context(), requestID)
	r = r.WithContext(ctx)
	
	// Add request ID to response header
	w.Header().Set("X-Request-ID", requestID)
	
	eh.next.ServeHTTP(w, r)
}

// NewErrorHandler creates a new error handler middleware
func NewErrorHandler(next http.Handler) http.Handler {
	return &ErrorHandler{next: next}
}

// APIHandler is a wrapper for API handlers that provides structured error handling
type APIHandler func(w http.ResponseWriter, r *http.Request) error

// ServeHTTP implements http.Handler for APIHandler
func (h APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestID(r)
	
	if err := h(w, r); err != nil {
		// Convert to APIError if not already
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			_ = WriteError(w, apiErr, requestID)
		} else {
			// Wrap unknown errors
			apiErr = MapError(err, "internal server error")
			_ = WriteError(w, apiErr, requestID)
		}
	}
}

// Handle creates an APIHandler from a function
func Handle(fn APIHandler) http.Handler {
	return APIHandler(fn)
}

// Common error response helpers for use in handlers

// RespondWithValidationError returns a validation error response
func RespondWithValidationError(w http.ResponseWriter, message string, details map[string]any, requestID string) error {
	return WriteErrorResponse(w, ErrorCodeValidationGeneral, message, details, requestID)
}

// RespondWithNotFoundError returns a not found error response
func RespondWithNotFoundError(w http.ResponseWriter, resource string, details map[string]any, requestID string) error {
	if details == nil {
		details = make(map[string]any)
	}
	details["resource"] = resource
	return WriteErrorResponse(w, ErrorCodeNotFoundGeneral, "resource not found", details, requestID)
}

// RespondWithTxNotFound returns a transaction not found error response
func RespondWithTxNotFound(w http.ResponseWriter, txid string, requestID string) error {
	return WriteErrorResponse(w, ErrorCodeTxNotFound, "transaction not found", map[string]any{"txid": txid}, requestID)
}

// RespondWithBlockNotFound returns a block not found error response
func RespondWithBlockNotFound(w http.ResponseWriter, identifier string, requestID string) error {
	return WriteErrorResponse(w, ErrorCodeBlockNotFound, "block not found", map[string]any{"identifier": identifier}, requestID)
}

// RespondWithInternalError returns an internal server error response
func RespondWithInternalError(w http.ResponseWriter, message string, details map[string]any, requestID string) error {
	return WriteErrorResponse(w, ErrorCodeInternalGeneral, message, details, requestID)
}

// RespondWithUnauthorized returns an unauthorized error response
func RespondWithUnauthorized(w http.ResponseWriter, message string, requestID string) error {
	return WriteErrorResponse(w, ErrorCodeUnauthorized, message, nil, requestID)
}

// RespondWithForbidden returns a forbidden error response
func RespondWithForbidden(w http.ResponseWriter, message string, requestID string) error {
	return WriteErrorResponse(w, ErrorCodeForbidden, message, nil, requestID)
}

// RespondWithRateLimited returns a rate limited error response
func RespondWithRateLimited(w http.ResponseWriter, message string, details map[string]any, requestID string) error {
	return WriteErrorResponse(w, ErrorCodeRateLimitedGeneral, message, details, requestID)
}

// RespondWithSuccess returns a success response
func RespondWithSuccess(w http.ResponseWriter, data any, requestID string) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// RespondWithCreated returns a created response
func RespondWithCreated(w http.ResponseWriter, data any, requestID string) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// RespondWithNoContent returns a no content response
func RespondWithNoContent(w http.ResponseWriter, requestID string) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}
