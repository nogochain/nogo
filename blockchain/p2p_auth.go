package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"
)

type P2PAuth struct {
	mu             sync.RWMutex
	nodeKey        ed25519.PrivateKey
	nodeID         string
	trustedPubKeys map[string]struct{}
	challengeTTL   time.Duration
}

func NewP2PAuth(nodeKey ed25519.PrivateKey, nodeID string) *P2PAuth {
	auth := &P2PAuth{
		nodeKey:        nodeKey,
		nodeID:         nodeID,
		trustedPubKeys: make(map[string]struct{}),
		challengeTTL:   30 * time.Second,
	}
	if nodeID == "" && len(nodeKey) > 0 {
		pubKey := nodeKey.Public().(ed25519.PublicKey)
		auth.nodeID = hex.EncodeToString(pubKey)
	}
	return auth
}

func (auth *P2PAuth) GenerateChallenge() (string, error) {
	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(challengeBytes), nil
}

func (auth *P2PAuth) SignChallenge(challenge string) (string, error) {
	if auth.nodeKey == nil {
		return "", errors.New("no node key configured")
	}
	sig := ed25519.Sign(auth.nodeKey, []byte(challenge))
	return base64.StdEncoding.EncodeToString(sig), nil
}

func (auth *P2PAuth) VerifyResponse(challenge, response, pubKeyHex string) bool {
	auth.mu.RLock()
	defer auth.mu.RUnlock()

	if len(auth.trustedPubKeys) > 0 {
		if _, ok := auth.trustedPubKeys[pubKeyHex]; !ok {
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

func (auth *P2PAuth) AddTrustedPeer(pubKeyHex string) {
	auth.mu.Lock()
	defer auth.mu.Unlock()
	auth.trustedPubKeys[pubKeyHex] = struct{}{}
}

func (auth *P2PAuth) GetNodeID() string {
	return auth.nodeID
}

func (auth *P2PAuth) GetPubKeyHex() string {
	if auth.nodeKey == nil {
		return ""
	}
	return hex.EncodeToString(auth.nodeKey.Public().(ed25519.PublicKey))
}

type P2PServerAuth struct {
	auth     *P2PAuth
	peers    map[string]*PeerScore
	scorer   *PeerScorer
	authMode bool
}

func NewP2PServerAuth(auth *P2PAuth, scorer *PeerScorer, authMode bool) *P2PServerAuth {
	return &P2PServerAuth{
		auth:     auth,
		peers:    make(map[string]*PeerScore),
		scorer:   scorer,
		authMode: authMode,
	}
}

// handleAuth reserved for future use //nolint:unused
func (sa *P2PServerAuth) handleAuth(c net.Conn, env p2pEnvelope, start time.Time) error {
	var challengeReq p2pAuthChallenge
	if err := json.Unmarshal(env.Payload, &challengeReq); err != nil {
		return errors.New("invalid auth payload")
	}

	challenge := challengeReq.Challenge
	if challenge == "" {
		return errors.New("empty challenge")
	}

	response, err := sa.auth.SignChallenge(challenge)
	if err != nil {
		return errors.New("failed to sign challenge")
	}

	authResp := p2pAuthResponse{
		Response: response,
		NodeID:   sa.auth.GetNodeID(),
		PubKey:   sa.auth.GetPubKeyHex(),
	}

	if err := p2pWriteJSON(c, p2pEnvelope{Type: "auth_response", Payload: mustJSON(authResp)}); err != nil {
		return err
	}

	latency := time.Since(start).Milliseconds()
	sa.scorer.RecordSuccess(c.RemoteAddr().String(), latency)

	return nil
}

type P2PClientAuth struct {
	auth         *P2PAuth
	scorer       *PeerScorer
	trustedPeers map[string]struct{}
}

func NewP2PClientAuth(auth *P2PAuth, scorer *PeerScorer) *P2PClientAuth {
	return &P2PClientAuth{
		auth:         auth,
		scorer:       scorer,
		trustedPeers: make(map[string]struct{}),
	}
}

func (ca *P2PClientAuth) Authenticate(_ context.Context, conn net.Conn, challenge string, start time.Time) error {
	response, err := ca.auth.SignChallenge(challenge)
	if err != nil {
		return err
	}

	authResp := p2pAuthResponse{
		Response: response,
		NodeID:   ca.auth.GetNodeID(),
		PubKey:   ca.auth.GetPubKeyHex(),
	}

	if err := p2pWriteJSON(conn, p2pEnvelope{Type: "auth_response", Payload: mustJSON(authResp)}); err != nil {
		return err
	}

	latency := time.Since(start).Milliseconds()
	ca.scorer.RecordSuccess(conn.RemoteAddr().String(), latency)

	return nil
}

func (ca *P2PClientAuth) VerifyPeer(pubKeyHex string) bool {
	if len(ca.trustedPeers) == 0 {
		return true
	}
	_, ok := ca.trustedPeers[pubKeyHex]
	return ok
}

func (ca *P2PClientAuth) AddTrustedPeer(pubKeyHex string) {
	ca.trustedPeers[pubKeyHex] = struct{}{}
}

// sha256Hash reserved for future use //nolint:unused
func sha256Hash(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
