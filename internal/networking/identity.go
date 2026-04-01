package networking

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	ErrNoIdentity       = errors.New("no node identity configured")
	ErrIdentityNotFound = errors.New("identity not found")
)

type Identity struct {
	mu      sync.RWMutex
	nodeID  string
	pubKey  ed25519.PublicKey
	privKey ed25519.PrivateKey
	seed    [32]byte
}

var globalIdentity *Identity
var identityOnce sync.Once

func GetIdentity() *Identity {
	identityOnce.Do(func() {
		globalIdentity = NewIdentity()
	})
	return globalIdentity
}

func NewIdentity() *Identity {
	return &Identity{}
}

func GenerateIdentity() (*Identity, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, fmt.Errorf("failed to generate seed: %w", err)
	}

	nodeID := deriveNodeID(pubKey)

	return &Identity{
		nodeID:  nodeID,
		pubKey:  pubKey,
		privKey: privKey,
		seed:    seed,
	}, nil
}

func LoadOrGenerateIdentity(keyPath string) (*Identity, error) {
	identity := &Identity{}

	privKeyPath := keyPath + ".key"
	pubKeyPath := keyPath + ".pub"

	privKeyBytes, err := os.ReadFile(privKeyPath)
	if err == nil {
		if len(privKeyBytes) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("invalid private key size: %d", len(privKeyBytes))
		}
		identity.privKey = ed25519.PrivateKey(privKeyBytes)
		identity.pubKey = identity.privKey.Public().(ed25519.PublicKey)
		identity.nodeID = deriveNodeID(identity.pubKey)

		seed, err := deriveSeed(identity.privKey)
		if err != nil {
			return nil, err
		}
		identity.seed = seed

		return identity, nil
	}

	newIdentity, err := GenerateIdentity()
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(privKeyPath, newIdentity.privKey, 0600); err != nil {
		return nil, fmt.Errorf("failed to write private key: %w", err)
	}

	pubKeyBytes, _ := hex.DecodeString(newIdentity.nodeID)
	if err := os.WriteFile(pubKeyPath, pubKeyBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write public key: %w", err)
	}

	return newIdentity, nil
}

func deriveNodeID(pubKey ed25519.PublicKey) string {
	h := sha256.Sum256(pubKey)
	return hex.EncodeToString(h[:])
}

func deriveSeed(privKey ed25519.PrivateKey) ([32]byte, error) {
	h := sha256.Sum256(privKey)
	var seed [32]byte
	copy(seed[:], h[:])
	return seed, nil
}

func (i *Identity) NodeID() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.nodeID
}

func (i *Identity) PubKey() ed25519.PublicKey {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.pubKey
}

func (i *Identity) PrivKey() ed25519.PrivateKey {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.privKey
}

func (i *Identity) PubKeyHex() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.pubKey == nil {
		return ""
	}
	return hex.EncodeToString(i.pubKey)
}

func (i *Identity) PubKeyBase64() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.pubKey == nil {
		return ""
	}
	return base64Encode(i.pubKey)
}

func (i *Identity) HasPrivateKey() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.privKey != nil
}

func (i *Identity) SignMessage(msg []byte) ([]byte, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if i.privKey == nil {
		return nil, ErrNoIdentity
	}

	signature := ed25519.Sign(i.privKey, msg)
	return signature, nil
}

func (i *Identity) SignMessageHex(msg []byte) (string, error) {
	sig, err := i.SignMessage(msg)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sig), nil
}

func (i *Identity) SignMessageBase64(msg []byte) (string, error) {
	sig, err := i.SignMessage(msg)
	if err != nil {
		return "", err
	}
	return base64Encode(sig), nil
}

func (i *Identity) VerifyMessage(msg []byte, signature []byte) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if i.pubKey == nil {
		return false
	}

	return ed25519.Verify(i.pubKey, msg, signature)
}

func (i *Identity) VerifyMessageWithPubKey(pubKey []byte, msg []byte, signature []byte) bool {
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}

	publicKey := ed25519.PublicKey(pubKey)
	return ed25519.Verify(publicKey, msg, signature)
}

func (i *Identity) SignData(data []byte) ([]byte, error) {
	h := sha256.Sum256(data)
	return i.SignMessage(h[:])
}

func (i *Identity) SignDataHex(data []byte) (string, error) {
	sig, err := i.SignData(data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sig), nil
}

func (i *Identity) CreateSignedHello(chainID uint64, rulesHash string, protocol uint32) (SecureHello, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if i.privKey == nil {
		return SecureHello{}, ErrNoIdentity
	}

	var nonce uint64
	nonceBytes := make([]byte, 8)
	rand.Read(nonceBytes)
	for j := 0; j < 8; j++ {
		nonce = nonce<<8 | uint64(nonceBytes[j])
	}

	sigData := fmt.Sprintf("%d|%d|%s|%s|%d|%d",
		protocol, chainID, rulesHash, i.nodeID, time.Now().Unix(), nonce)
	sig := ed25519.Sign(i.privKey, []byte(sigData))

	return SecureHello{
		Version:   uint8(protocol),
		Protocol:  protocol,
		ChainID:   chainID,
		RulesHash: rulesHash,
		NodeID:    i.nodeID,
		TimeUnix:  time.Now().Unix(),
		PubKey:    base64Encode(i.pubKey),
		Signature: base64Encode(sig),
		Nonce:     nonce,
	}, nil
}

func (i *Identity) VerifySignedHello(hello *SecureHello) error {
	if hello == nil {
		return errors.New("nil hello")
	}

	pubKeyBytes, err := base64Decode(hello.PubKey)
	if err != nil {
		return fmt.Errorf("decode pubkey: %w", err)
	}

	sig, err := base64Decode(hello.Signature)
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

func (i *Identity) DeriveSharedSecret(peerPubKey []byte) ([]byte, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if i.privKey == nil {
		return nil, ErrNoIdentity
	}

	if len(peerPubKey) != ed25519.PublicKeySize {
		return nil, errors.New("invalid peer public key size")
	}

	peerKey := ed25519.PublicKey(peerPubKey)
	shared := ed25519.PrivateKey(i.privKey).Public().(ed25519.PublicKey)

	h := sha256.Sum256(append(shared, peerKey...))
	return h[:], nil
}

func (i *Identity) Seed() [32]byte {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.seed
}

func VerifyPeerIdentity(nodeID, pubKeyHex, signature, message string) bool {
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return false
	}

	derivedNodeID := deriveNodeID(ed25519.PublicKey(pubKeyBytes))
	if derivedNodeID != nodeID {
		return false
	}

	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}

	pubKey := ed25519.PublicKey(pubKeyBytes)
	return ed25519.Verify(pubKey, []byte(message), sigBytes)
}

func base64Encode(data []byte) string {
	return "base64:" + hex.EncodeToString(data)
}

func base64Decode(s string) ([]byte, error) {
	if len(s) > 7 && s[:7] == "base64:" {
		s = s[7:]
	}
	return hex.DecodeString(s)
}
