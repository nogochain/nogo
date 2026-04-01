package networking

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const (
	ProtocolVersionNumber = 1
	HandshakeTimeout      = 15 * time.Second
	HelloTimeout          = 30 * time.Second
	ChallengeTTL          = 30 * time.Second
)

var (
	ErrUnsupportedVersion = errors.New("unsupported protocol version")
	ErrWrongChain         = errors.New("wrong chain ID")
	ErrRulesHashMismatch  = errors.New("rules hash mismatch")
	ErrStaleHello         = errors.New("stale hello timestamp")
	ErrInvalidSignature   = errors.New("invalid signature")
	ErrInvalidChallenge   = errors.New("invalid challenge")
	ErrEmptyChallenge     = errors.New("empty challenge")
)

type SecureHello struct {
	Version     uint8    `json:"version"`
	Protocol    uint32   `json:"protocol"`
	ChainID     uint64   `json:"chainId"`
	RulesHash   string   `json:"rulesHash"`
	NodeID      string   `json:"nodeId"`
	TimeUnix    int64    `json:"timeUnix"`
	PubKey      string   `json:"pubKey"`
	Signature   string   `json:"signature"`
	ListenAddrs []string `json:"listenAddrs,omitempty"`
	Port        uint16   `json:"port,omitempty"`
	Nonce       uint64   `json:"nonce,omitempty"`
}

type AuthChallenge struct {
	Challenge string `json:"challenge"`
	NodeID    string `json:"nodeId"`
}

type AuthResponse struct {
	Response string `json:"response"`
	NodeID   string `json:"nodeId"`
	PubKey   string `json:"pubKey"`
}

type Auth struct {
	nodeKey        ed25519.PrivateKey
	nodeID         string
	trustedPubKeys map[string]struct{}
}

func NewAuth(nodeKey ed25519.PrivateKey, nodeID string) *Auth {
	auth := &Auth{
		nodeKey:        nodeKey,
		trustedPubKeys: make(map[string]struct{}),
	}
	if nodeID == "" && len(nodeKey) > 0 {
		pubKey := nodeKey.Public().(ed25519.PublicKey)
		auth.nodeID = hex.EncodeToString(pubKey)
	} else {
		auth.nodeID = nodeID
	}
	return auth
}

func (a *Auth) GenerateChallenge() (string, error) {
	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(challengeBytes), nil
}

func (a *Auth) SignChallenge(challenge string) (string, error) {
	if a.nodeKey == nil {
		return "", errors.New("no node key configured")
	}
	sig := ed25519.Sign(a.nodeKey, []byte(challenge))
	return base64.StdEncoding.EncodeToString(sig), nil
}

func (a *Auth) VerifyResponse(challenge, response, pubKeyHex string) bool {
	if len(a.trustedPubKeys) > 0 {
		if _, ok := a.trustedPubKeys[pubKeyHex]; !ok {
			return false
		}
	}

	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return false
	}

	pubKey := ed25519.PublicKey(pubKeyBytes)
	return ed25519.Verify(pubKey, []byte(challenge), sigFromBase64(response))
}

func sigFromBase64(sigStr string) []byte {
	sigBytes, _ := base64.StdEncoding.DecodeString(sigStr)
	return sigBytes
}

func (a *Auth) AddTrustedPeer(pubKeyHex string) {
	a.trustedPubKeys[pubKeyHex] = struct{}{}
}

func (a *Auth) GetNodeID() string {
	return a.nodeID
}

func (a *Auth) GetPubKeyHex() string {
	if a.nodeKey == nil {
		return ""
	}
	return hex.EncodeToString(a.nodeKey.Public().(ed25519.PublicKey))
}

type ProtocolHandshake struct {
	chainID   uint64
	rulesHash string
	nodeID    string
	port      uint16
	services  uint64
	privKey   ed25519.PrivateKey
	auth      *Auth
}

func NewProtocolHandshake(chainID uint64, rulesHash, nodeID string, privKey ed25519.PrivateKey) *ProtocolHandshake {
	return &ProtocolHandshake{
		chainID:   chainID,
		rulesHash: rulesHash,
		nodeID:    nodeID,
		auth:      NewAuth(privKey, nodeID),
		privKey:   privKey,
	}
}

func (h *ProtocolHandshake) CreateHello() (SecureHello, error) {
	nonceBytes := make([]byte, 8)
	rand.Read(nonceBytes)
	nonce := binary.LittleEndian.Uint64(nonceBytes)

	pubKey := h.privKey.Public().(ed25519.PublicKey)
	sigData := fmt.Sprintf("%d|%d|%s|%s|%d|%d",
		ProtocolVersionNumber, h.chainID, h.rulesHash, h.nodeID, time.Now().Unix(), nonce)
	sig := ed25519.Sign(h.privKey, []byte(sigData))

	return SecureHello{
		Version:   ProtocolVersionNumber,
		Protocol:  ProtocolVersionNumber,
		ChainID:   h.chainID,
		RulesHash: h.rulesHash,
		NodeID:    h.nodeID,
		TimeUnix:  time.Now().Unix(),
		PubKey:    base64.StdEncoding.EncodeToString(pubKey),
		Signature: base64.StdEncoding.EncodeToString(sig),
		Port:      h.port,
		Nonce:     nonce,
	}, nil
}

func (h *ProtocolHandshake) ValidateHello(hello *SecureHello) error {
	if hello.Version != ProtocolVersionNumber {
		return ErrUnsupportedVersion
	}
	if hello.Protocol != ProtocolVersionNumber {
		return ErrUnsupportedVersion
	}
	if hello.ChainID != h.chainID {
		return ErrWrongChain
	}
	if hello.RulesHash == "" || hello.RulesHash != h.rulesHash {
		return ErrRulesHashMismatch
	}
	if time.Now().Unix()-hello.TimeUnix > 300 {
		return ErrStaleHello
	}
	return nil
}

func (h *ProtocolHandshake) VerifySignature(hello *SecureHello) error {
	pubKeyBytes, err := base64.StdEncoding.DecodeString(hello.PubKey)
	if err != nil {
		return fmt.Errorf("decode pubkey: %w", err)
	}

	sig, err := base64.StdEncoding.DecodeString(hello.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	sigData := fmt.Sprintf("%d|%d|%s|%s|%d|%d",
		hello.Version, hello.ChainID, hello.RulesHash, hello.NodeID, hello.TimeUnix, hello.Nonce)

	if !ed25519.Verify(pubKeyBytes, []byte(sigData), sig) {
		return ErrInvalidSignature
	}

	return nil
}

func (h *ProtocolHandshake) CreateSignedHello() (SecureHello, error) {
	return h.CreateHello()
}

func (h *ProtocolHandshake) GetNodeID() string {
	return h.nodeID
}

func (h *ProtocolHandshake) GetChainID() uint64 {
	return h.chainID
}

func (h *ProtocolHandshake) GetRulesHash() string {
	return h.rulesHash
}

func computeHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
