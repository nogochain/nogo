package explorer

import (
	"encoding/json"
	"net/http"
)

type ExplorerAPI struct {
	nodeURL string
}

func NewExplorerAPI(nodeURL string) *ExplorerAPI {
	if nodeURL == "" {
		nodeURL = "http://localhost:8080"
	}
	return &ExplorerAPI{nodeURL: nodeURL}
}

func (e *ExplorerAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.URL.Path {
	case "/explorer/search":
		e.handleSearch(w, r)
	case "/explorer/blocks":
		e.handleBlocks(w, r)
	case "/explorer/block":
		e.handleBlock(w, r)
	case "/explorer/tx":
		e.handleTransaction(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (e *ExplorerAPI) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing query"})
		return
	}

	resp, err := http.Get(e.nodeURL + "/block/" + q)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	json.NewEncoder(w).Encode(result)
}

func (e *ExplorerAPI) handleBlocks(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get(e.nodeURL + "/blocks?limit=20")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	var blocks []interface{}
	json.NewDecoder(resp.Body).Decode(&blocks)
	json.NewEncoder(w).Encode(blocks)
}

func (e *ExplorerAPI) handleBlock(w http.ResponseWriter, r *http.Request) {
	height := r.URL.Query().Get("height")
	if height == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing height"})
		return
	}

	resp, err := http.Get(e.nodeURL + "/block/height/" + height)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	var block map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&block)
	json.NewEncoder(w).Encode(block)
}

func (e *ExplorerAPI) handleTransaction(w http.ResponseWriter, r *http.Request) {
	txid := r.URL.Query().Get("id")
	if txid == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "missing txid"})
		return
	}

	resp, err := http.Get(e.nodeURL + "/tx/" + txid)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	var tx map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tx)
	json.NewEncoder(w).Encode(tx)
}
