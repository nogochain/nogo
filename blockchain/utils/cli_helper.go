package utils

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/crypto"
)

// BuildSignedTransferTx builds a signed transfer transaction
func BuildSignedTransferTx(server, privB64, to string, amount, fee, nonce uint64, data string) (*core.Transaction, error) {
	w, err := crypto.WalletFromPrivateKeyBase64(privB64)
	if err != nil {
		return nil, err
	}
	return BuildSignedTransferTxWithWallet(server, w, to, amount, fee, nonce, data)
}

// BuildSignedTransferTxWithWallet builds a signed transfer transaction with a wallet
func BuildSignedTransferTxWithWallet(server string, w *crypto.Wallet, to string, amount, fee, nonce uint64, data string) (*core.Transaction, error) {
	if w == nil {
		return nil, errors.New("nil wallet")
	}

	info, err := fetchChainInfo(server)
	if err != nil {
		return nil, err
	}
	chainID := info.ChainID

	// If nonce not provided, fetch it from chain
	if nonce == 0 {
		acct, err := fetchAccount(server, w.Address)
		if err != nil {
			return nil, err
		}
		nonce = acct.Nonce + 1
	}

	tx := core.Transaction{
		Type:       core.TxTransfer,
		ChainID:    chainID,
		FromPubKey: w.PublicKey,
		ToAddress:  to,
		Amount:     amount,
		Fee:        fee,
		Nonce:      nonce,
		Data:       data,
	}
	p := config.ConsensusParams{
		BinaryEncodingEnable:           info.BinaryEncodingEnable,
		BinaryEncodingActivationHeight: info.BinaryEncodingActivationHeight,
	}
	nextHeight := info.Height + 1
	h, err := core.TxSigningHashForConsensus(tx, p, nextHeight)
	if err != nil {
		return nil, err
	}
	tx.Signature = ed25519.Sign(w.PrivateKey, h)

	return &tx, nil
}

type chainInfoView struct {
	ChainID                        uint64 `json:"chainId"`
	Height                         uint64 `json:"height"`
	BinaryEncodingEnable           bool   `json:"binaryEncodingEnable"`
	BinaryEncodingActivationHeight uint64 `json:"binaryEncodingActivationHeight"`
}

func fetchChainInfo(server string) (*chainInfoView, error) {
	if raw := strings.TrimSpace(os.Getenv("CHAIN_ID")); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 {
			return nil, errors.New("CHAIN_ID must be a uint64 > 0")
		}
		return &chainInfoView{ChainID: id}, nil
	}

	if strings.TrimSpace(server) == "" {
		return nil, errors.New("cannot determine chain info: set CHAIN_ID or provide server_url")
	}

	req, _ := http.NewRequest(http.MethodGet, server+"/chain/info", nil)
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chain info request failed with status: %s", resp.Status)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var info chainInfoView
	if err := json.Unmarshal(b, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

type accountView struct {
	Balance uint64 `json:"balance"`
	Nonce   uint64 `json:"nonce"`
}

func fetchAccount(server, addr string) (*accountView, error) {
	req, _ := http.NewRequest(http.MethodGet, server+"/account/"+addr, nil)
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("account request failed with status: %s", resp.Status)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var acct accountView
	if err := json.Unmarshal(b, &acct); err != nil {
		return nil, err
	}

	return &acct, nil
}

func addAdminAuth(req *http.Request) {
	if token := os.Getenv("ADMIN_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func httpClient() *http.Client {
	return &http.Client{}
}

// DefaultDataDir returns the default data directory
func DefaultDataDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "NogoChain")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(homeDir, ".nogochain")
}
