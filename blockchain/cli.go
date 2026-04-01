package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultServerURL = "http://localhost:8080"

func runCLI(args []string) error {
	if len(args) == 0 {
		return errors.New("missing command")
	}

	switch args[0] {
	case "create_wallet":
		w, err := NewWallet()
		if err != nil {
			return err
		}
		out := map[string]any{
			"address":    w.Address,
			"publicKey":  w.PublicKeyBase64(),
			"privateKey": w.PrivateKeyBase64(),
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return nil
	
	case "create_wallet_mnemonic":
		// usage: blockchain create_wallet_mnemonic [mnemonic] [passphrase]
		// If mnemonic not provided, generates a new one
		mnemonic := ""
		passphrase := ""
		if len(args) >= 2 {
			mnemonic = args[1]
		}
		if len(args) >= 3 {
			passphrase = args[2]
		}
		
		w, generatedMnemonic, err := CreateWalletFromMnemonic(mnemonic, passphrase)
		if err != nil {
			return err
		}
		
		out := map[string]any{
			"address":   w.Address,
			"publicKey": w.PublicKeyBase64(),
			"mnemonic":  generatedMnemonic,
			"warning":   "IMPORTANT: Store mnemonic securely. Cannot be recovered if lost!",
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return nil
	
	case "create_keystore_mnemonic":
		// usage: blockchain create_keystore_mnemonic <file> [password] [mnemonic] [passphrase]
		// Creates a keystore with encrypted mnemonic backup
		if len(args) < 2 {
			return errors.New("usage: blockchain create_keystore_mnemonic <file> [password] [mnemonic] [passphrase]")
		}
		
		passArg := ""
		if len(args) >= 3 {
			passArg = args[2]
		}
		mnemonic := ""
		if len(args) >= 4 {
			mnemonic = args[3]
		}
		passphrase := ""
		if len(args) >= 5 {
			passphrase = args[4]
		}
		
		pass, err := ReadWalletPassword(passArg, "Keystore password: ", true)
		if err != nil {
			return err
		}
		
		w, finalMnemonic, err := CreateWalletFromMnemonic(mnemonic, passphrase)
		if err != nil {
			return err
		}
		
		if err := WriteKeystoreWithMnemonic(args[1], w, finalMnemonic, pass, DefaultKeystoreParams()); err != nil {
			return err
		}
		
		out := map[string]any{
			"keystore": args[1],
			"address":  w.Address,
			"message":  "Keystore created with encrypted mnemonic backup",
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return nil
	
	case "export_mnemonic":
		// usage: blockchain export_mnemonic <file> [password]
		// Exports mnemonic from a keystore file
		if len(args) < 2 {
			return errors.New("usage: blockchain export_mnemonic <file> [password]")
		}
		
		passArg := ""
		if len(args) >= 3 {
			passArg = args[2]
		}
		
		pass, err := ReadWalletPassword(passArg, "Keystore password: ", false)
		if err != nil {
			return err
		}
		
		mnemonic, err := ExportMnemonicFromKeystore(args[1], pass)
		if err != nil {
			return err
		}
		
		out := map[string]any{
			"keystore": args[1],
			"mnemonic": mnemonic,
			"warning":  "IMPORTANT: Keep mnemonic secure. Anyone with mnemonic can access your funds!",
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return nil

	case "create_keystore":
		// usage: blockchain create_keystore <file> [password]
		if len(args) < 2 {
			return errors.New("usage: blockchain create_keystore <file> [password]")
		}
		passArg := ""
		if len(args) >= 3 {
			passArg = args[2]
		}
		pass, err := ReadWalletPassword(passArg, "Keystore password: ", true)
		if err != nil {
			return err
		}
		w, err := NewWallet()
		if err != nil {
			return err
		}
		if err := WriteKeystore(args[1], w, pass, DefaultKeystoreParams()); err != nil {
			return err
		}
		out := map[string]any{
			"keystore": args[1],
			"address":  w.Address,
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return nil

	case "keystore_info":
		// usage: blockchain keystore_info <file>
		if len(args) < 2 {
			return errors.New("usage: blockchain keystore_info <file>")
		}
		ks, err := ReadKeystore(args[1])
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(map[string]any{
			"keystore": args[1],
			"version":  ks.Version,
			"address":  ks.Address,
			"kdf":      ks.KDF,
			"cipher":   ks.Cipher.Name,
		}, "", "  ")
		fmt.Println(string(b))
		return nil

	case "get_balance":
		if len(args) < 2 {
			return errors.New("usage: blockchain get_balance <address> [server_url]")
		}
		server := pickServer(args, 2)
		return cliGetBalance(server, args[1])

	case "sign_tx":
		// usage: blockchain sign_tx <private_key_b64> <to_address> <amount> [fee] [nonce] [data] [server_url]
		if len(args) < 4 {
			return errors.New("usage: blockchain sign_tx <private_key_b64> <to_address> <amount> [fee] [nonce] [data] [server_url]")
		}
		privB64 := args[1]
		to := args[2]
		amount, err := strconv.ParseUint(args[3], 10, 64)
		if err != nil {
			return errors.New("amount must be uint")
		}
		fee := uint64(minFee)
		if len(args) >= 5 && args[4] != "" {
			fee, err = strconv.ParseUint(args[4], 10, 64)
			if err != nil {
				return errors.New("fee must be uint")
			}
		}
		nonce := uint64(0)
		if len(args) >= 6 && args[5] != "" {
			nonce, err = strconv.ParseUint(args[5], 10, 64)
			if err != nil {
				return errors.New("nonce must be uint")
			}
		}
		data := ""
		if len(args) >= 7 {
			data = args[6]
		}
		server := pickServer(args, 7)
		return cliSignTx(server, privB64, to, amount, fee, nonce, data)

	case "sign_tx_keystore":
		// usage: blockchain sign_tx_keystore <keystore_file> <to_address> <amount> [fee] [nonce] [data] [server_url] [password]
		if len(args) < 4 {
			return errors.New("usage: blockchain sign_tx_keystore <keystore_file> <to_address> <amount> [fee] [nonce] [data] [server_url] [password]")
		}
		ksPath := args[1]
		to := args[2]
		amount, err := strconv.ParseUint(args[3], 10, 64)
		if err != nil {
			return errors.New("amount must be uint")
		}
		fee := uint64(minFee)
		if len(args) >= 5 && args[4] != "" {
			fee, err = strconv.ParseUint(args[4], 10, 64)
			if err != nil {
				return errors.New("fee must be uint")
			}
		}
		nonce := uint64(0)
		if len(args) >= 6 && args[5] != "" {
			nonce, err = strconv.ParseUint(args[5], 10, 64)
			if err != nil {
				return errors.New("nonce must be uint")
			}
		}
		data := ""
		if len(args) >= 7 {
			data = args[6]
		}
		server := pickServer(args, 7)
		passArg := ""
		if len(args) >= 9 && args[8] != "" {
			passArg = args[8]
		}
		pass, err := ReadWalletPassword(passArg, "Keystore password: ", false)
		if err != nil {
			return err
		}
		return cliSignTxKeystore(server, ksPath, pass, to, amount, fee, nonce, data)

	case "send":
		// usage: blockchain send <private_key_b64> <to_address> <amount> [fee] [nonce] [data] [server_url]
		if len(args) < 4 {
			return errors.New("usage: blockchain send <private_key_b64> <to_address> <amount> [fee] [nonce] [data] [server_url]")
		}
		privB64 := args[1]
		to := args[2]
		amount, err := strconv.ParseUint(args[3], 10, 64)
		if err != nil {
			return errors.New("amount must be uint")
		}
		fee := uint64(minFee)
		if len(args) >= 5 && args[4] != "" {
			fee, err = strconv.ParseUint(args[4], 10, 64)
			if err != nil {
				return errors.New("fee must be uint")
			}
		}
		nonce := uint64(0)
		if len(args) >= 6 && args[5] != "" {
			nonce, err = strconv.ParseUint(args[5], 10, 64)
			if err != nil {
				return errors.New("nonce must be uint")
			}
		}
		data := ""
		if len(args) >= 7 {
			data = args[6]
		}
		server := pickServer(args, 7)

		return cliSend(server, privB64, to, amount, fee, nonce, data)

	case "send_keystore":
		// usage: blockchain send_keystore <keystore_file> <to_address> <amount> [fee] [nonce] [data] [server_url] [password]
		if len(args) < 4 {
			return errors.New("usage: blockchain send_keystore <keystore_file> <to_address> <amount> [fee] [nonce] [data] [server_url] [password]")
		}
		ksPath := args[1]
		to := args[2]
		amount, err := strconv.ParseUint(args[3], 10, 64)
		if err != nil {
			return errors.New("amount must be uint")
		}
		fee := uint64(minFee)
		if len(args) >= 5 && args[4] != "" {
			fee, err = strconv.ParseUint(args[4], 10, 64)
			if err != nil {
				return errors.New("fee must be uint")
			}
		}
		nonce := uint64(0)
		if len(args) >= 6 && args[5] != "" {
			nonce, err = strconv.ParseUint(args[5], 10, 64)
			if err != nil {
				return errors.New("nonce must be uint")
			}
		}
		data := ""
		if len(args) >= 7 {
			data = args[6]
		}
		server := pickServer(args, 7)
		passArg := ""
		if len(args) >= 9 && args[8] != "" {
			passArg = args[8]
		}
		pass, err := ReadWalletPassword(passArg, "Keystore password: ", false)
		if err != nil {
			return err
		}

		return cliSendKeystore(server, ksPath, pass, to, amount, fee, nonce, data)

	case "audit_chain":
		server := pickServer(args, 1)
		return cliAuditChain(server)

	case "chain_info":
		server := pickServer(args, 1)
		return cliChainInfo(server)

	case "mempool":
		server := pickServer(args, 1)
		return cliMempool(server)

	case "tx":
		if len(args) < 2 {
			return errors.New("usage: blockchain tx <txid_hex> [server_url]")
		}
		server := pickServer(args, 2)
		return cliTx(server, args[1])

	case "block_height":
		if len(args) < 2 {
			return errors.New("usage: blockchain block_height <height> [server_url]")
		}
		server := pickServer(args, 2)
		h, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil {
			return errors.New("height must be uint")
		}
		return cliBlockByHeight(server, h)

	case "block_hash":
		if len(args) < 2 {
			return errors.New("usage: blockchain block_hash <hash_hex> [server_url]")
		}
		server := pickServer(args, 2)
		return cliBlockByHash(server, args[1])

	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func pickServer(args []string, idx int) string {
	if len(args) > idx && strings.HasPrefix(args[idx], "http") {
		return strings.TrimRight(args[idx], "/")
	}
	return defaultServerURL
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func require2xx(resp *http.Response, body []byte) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("http %d: %s", resp.StatusCode, msg)
}

func addAdminAuth(req *http.Request) {
	token := strings.TrimSpace(os.Getenv("ADMIN_TOKEN"))
	if token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
}

func cliGetBalance(server, address string) error {
	req, _ := http.NewRequest(http.MethodGet, server+"/balance/"+address, nil)
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if err := require2xx(resp, b); err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func cliAuditChain(server string) error {
	req, _ := http.NewRequest(http.MethodPost, server+"/audit/chain", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if err := require2xx(resp, b); err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func cliChainInfo(server string) error {
	req, _ := http.NewRequest(http.MethodGet, server+"/chain/info", nil)
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if err := require2xx(resp, b); err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func cliMempool(server string) error {
	req, _ := http.NewRequest(http.MethodGet, server+"/mempool", nil)
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if err := require2xx(resp, b); err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func cliSend(server, privB64, to string, amount, fee, nonce uint64, data string) error {
	tx, err := buildSignedTransferTx(server, privB64, to, amount, fee, nonce, data)
	if err != nil {
		return err
	}

	b, _ := json.Marshal(tx)
	req, _ := http.NewRequest(http.MethodPost, server+"/tx", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if err := require2xx(resp, out); err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func cliSignTx(server, privB64, to string, amount, fee, nonce uint64, data string) error {
	tx, err := buildSignedTransferTx(server, privB64, to, amount, fee, nonce, data)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(tx, "", "  ")
	fmt.Println(string(b))
	return nil
}

func buildSignedTransferTx(server, privB64, to string, amount, fee, nonce uint64, data string) (*Transaction, error) {
	w, err := WalletFromPrivateKeyBase64(privB64)
	if err != nil {
		return nil, err
	}
	return buildSignedTransferTxWithWallet(server, w, to, amount, fee, nonce, data)
}

func buildSignedTransferTxWithWallet(server string, w *Wallet, to string, amount, fee, nonce uint64, data string) (*Transaction, error) {
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

	tx := Transaction{
		Type:       TxTransfer,
		ChainID:    chainID,
		FromPubKey: w.PublicKey,
		ToAddress:  to,
		Amount:     amount,
		Fee:        fee,
		Nonce:      nonce,
		Data:       data,
	}
	p := ConsensusParams{
		BinaryEncodingEnable:           info.BinaryEncodingEnable,
		BinaryEncodingActivationHeight: info.BinaryEncodingActivationHeight,
	}
	nextHeight := info.Height + 1
	h, err := txSigningHashForConsensus(tx, p, nextHeight)
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
	body, _ := io.ReadAll(resp.Body)
	if err := require2xx(resp, body); err != nil {
		return nil, err
	}
	var v chainInfoView
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&v); err != nil {
		return nil, err
	}
	if v.ChainID == 0 {
		return nil, errors.New("server returned invalid chainId=0")
	}
	return &v, nil
}

// determineChainID reserved for future use //nolint:unused
func determineChainID(server string) (uint64, error) {
	info, err := fetchChainInfo(server)
	if err != nil {
		return 0, err
	}
	return info.ChainID, nil
}

type accountView struct {
	Address string `json:"address"`
	Balance uint64 `json:"balance"`
	Nonce   uint64 `json:"nonce"`
}

func fetchAccount(server, address string) (*accountView, error) {
	req, _ := http.NewRequest(http.MethodGet, server+"/balance/"+address, nil)
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if err := require2xx(resp, body); err != nil {
		return nil, err
	}
	var v accountView
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&v); err != nil {
		return nil, err
	}
	return &v, nil
}

func cliSignTxKeystore(server, ksPath, password, to string, amount, fee, nonce uint64, data string) error {
	w, err := WalletFromKeystore(ksPath, password)
	if err != nil {
		return err
	}
	tx, err := buildSignedTransferTxWithWallet(server, w, to, amount, fee, nonce, data)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(tx, "", "  ")
	fmt.Println(string(b))
	return nil
}

func cliSendKeystore(server, ksPath, password, to string, amount, fee, nonce uint64, data string) error {
	w, err := WalletFromKeystore(ksPath, password)
	if err != nil {
		return err
	}
	tx, err := buildSignedTransferTxWithWallet(server, w, to, amount, fee, nonce, data)
	if err != nil {
		return err
	}

	b, _ := json.Marshal(tx)
	req, _ := http.NewRequest(http.MethodPost, server+"/tx", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if err := require2xx(resp, out); err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func cliTx(server, txid string) error {
	req, _ := http.NewRequest(http.MethodGet, server+"/tx/"+txid, nil)
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if err := require2xx(resp, b); err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func cliBlockByHeight(server string, height uint64) error {
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/block/height/%d", server, height), nil)
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if err := require2xx(resp, b); err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func cliBlockByHash(server, hashHex string) error {
	req, _ := http.NewRequest(http.MethodGet, server+"/blocks/hash/"+hashHex, nil)
	addAdminAuth(req)
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if err := require2xx(resp, b); err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  blockchain server")
	fmt.Fprintln(os.Stderr, "  blockchain create_wallet")
	fmt.Fprintln(os.Stderr, "  blockchain create_keystore <file> [password]")
	fmt.Fprintln(os.Stderr, "  blockchain keystore_info <file>")
	fmt.Fprintln(os.Stderr, "  blockchain chain_info [server_url]")
	fmt.Fprintln(os.Stderr, "  blockchain get_balance <address> [server_url]")
	fmt.Fprintln(os.Stderr, "  blockchain sign_tx <private_key_b64> <to_address> <amount> [fee] [nonce] [data] [server_url]")
	fmt.Fprintln(os.Stderr, "  blockchain sign_tx_keystore <keystore_file> <to_address> <amount> [fee] [nonce] [data] [server_url] [password]")
	fmt.Fprintln(os.Stderr, "  blockchain send <private_key_b64> <to_address> <amount> [fee] [nonce] [data] [server_url]")
	fmt.Fprintln(os.Stderr, "  blockchain send_keystore <keystore_file> <to_address> <amount> [fee] [nonce] [data] [server_url] [password]")
	fmt.Fprintln(os.Stderr, "  blockchain mempool [server_url]")
	fmt.Fprintln(os.Stderr, "  blockchain tx <txid_hex> [server_url]")
	fmt.Fprintln(os.Stderr, "  blockchain block_height <height> [server_url]")
	fmt.Fprintln(os.Stderr, "  blockchain block_hash <hash_hex> [server_url]")
	fmt.Fprintln(os.Stderr, "  blockchain audit_chain [server_url]")
}
