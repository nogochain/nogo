package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPClient provides a simple HTTP client for CLI-to-node communication.
// Connects to the running node API at the configured URL.
type HTTPClient struct {
	baseURL string
	client  *http.Client
}

// NewHTTPClient creates a new HTTP client with the given API URL.
func NewHTTPClient(apiURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: apiURL,
		client:  &http.Client{},
	}
}

// post sends a POST request to the given endpoint with the provided body.
func (h *HTTPClient) post(endpoint string, body interface{}) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := h.client.Post(h.baseURL+endpoint, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("connection refused: is the node running at %s? %w", h.baseURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// get sends a GET request to the given endpoint.
func (h *HTTPClient) get(endpoint string) ([]byte, error) {
	resp, err := h.client.Get(h.baseURL + endpoint)
	if err != nil {
		return nil, fmt.Errorf("connection refused: is the node running at %s? %w", h.baseURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// prettyJSON formats a JSON response for display.
func prettyJSON(data []byte) string {
	var obj interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return string(data)
	}
	formatted, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return string(data)
	}
	return string(formatted)
}

// printResponse prints a JSON response to stdout.
func printResponse(resp []byte) {
	fmt.Println(prettyJSON(resp))
}

// CreateWallet creates a new wallet via the node API.
func (h *HTTPClient) CreateWallet() error {
	resp, err := h.post("/wallet/create", nil)
	if err != nil {
		return err
	}
	printResponse(resp)
	return nil
}

// SignTransaction signs a transaction via the node API.
func (h *HTTPClient) SignTransaction(txJSON string) error {
	var tx interface{}
	if err := json.Unmarshal([]byte(txJSON), &tx); err != nil {
		return fmt.Errorf("invalid transaction JSON: %w", err)
	}
	resp, err := h.post("/wallet/sign", tx)
	if err != nil {
		return err
	}
	printResponse(resp)
	return nil
}

// SubmitTransaction submits a signed transaction via the node API.
func (h *HTTPClient) SubmitTransaction(txJSON string) error {
	var tx interface{}
	if err := json.Unmarshal([]byte(txJSON), &tx); err != nil {
		return fmt.Errorf("invalid transaction JSON: %w", err)
	}
	resp, err := h.post("/tx", tx)
	if err != nil {
		return err
	}
	printResponse(resp)
	return nil
}

// GetBlock retrieves block information by height or hash.
func (h *HTTPClient) GetBlock(identifier string) error {
	var endpoint string
	if strings.HasPrefix(identifier, "0x") || len(identifier) > 20 {
		endpoint = "/block/hash/" + identifier
	} else {
		endpoint = "/block/height/" + identifier
	}
	resp, err := h.get(endpoint)
	if err != nil {
		return err
	}
	printResponse(resp)
	return nil
}

// GetBalance retrieves the balance for an address.
func (h *HTTPClient) GetBalance(address string) error {
	if !strings.HasPrefix(address, "NOGO") {
		return fmt.Errorf("invalid address format: must start with 'NOGO'")
	}
	resp, err := h.get("/balance/" + address)
	if err != nil {
		return err
	}
	printResponse(resp)
	return nil
}
