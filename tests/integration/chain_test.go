package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
)

const (
	Node1URL = "http://127.0.0.1:8081"
	Node2URL = "http://127.0.0.1:8082"
)

func TestMain(m *testing.M) {
	fmt.Println("Starting integration test environment...")

	if os.Getenv("RUN_INTEGRATION") != "1" {
		fmt.Println("Skipping integration tests (set RUN_INTEGRATION=1 to run)")
		os.Exit(0)
	}

	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestChainInfo(t *testing.T) {
	resp, err := http.Get(Node1URL + "/chain/info")
	if err != nil {
		t.Skipf("Node not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	if _, ok := info["height"]; !ok {
		t.Error("Response missing 'height' field")
	}
}

func TestWalletCreate(t *testing.T) {
	resp, err := http.Post(Node1URL+"/wallet/create", "application/json", bytes.NewBuffer([]byte("{}")))
	if err != nil {
		t.Skipf("Node not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	if _, ok := result["address"]; !ok {
		t.Error("Response missing 'address' field")
	}
}

func TestBalanceEndpoint(t *testing.T) {
	resp, err := http.Post(Node1URL+"/wallet/create", "application/json", bytes.NewBuffer([]byte("{}")))
	if err != nil {
		t.Skipf("Node not available: %v", err)
	}
	defer resp.Body.Close()

	var wallet map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&wallet); err != nil {
		t.Skipf("Failed to create wallet: %v", err)
	}

	addr := wallet["address"].(string)

	balResp, err := http.Get(fmt.Sprintf("%s/balance/%s", Node1URL, addr))
	if err != nil {
		t.Skipf("Node not available: %v", err)
	}
	defer balResp.Body.Close()

	if balResp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", balResp.StatusCode)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	resp, err := http.Get(Node1URL + "/metrics")
	if err != nil {
		t.Skipf("Node not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	body := buf.String()

	if !contains(body, "neocoin_chain_height") {
		t.Error("Metrics missing neocoin_chain_height")
	}
}

func TestMempoolEndpoint(t *testing.T) {
	resp, err := http.Get(Node1URL + "/mempool")
	if err != nil {
		t.Skipf("Node not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestNode2Health(t *testing.T) {
	resp, err := http.Get(Node2URL + "/chain/info")
	if err != nil {
		t.Skipf("Node2 not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
