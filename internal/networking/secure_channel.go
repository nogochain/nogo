package networking

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

var (
	ErrInvalidKeySize    = errors.New("invalid key size")
	ErrDecryptionFailed  = errors.New("decryption failed")
	ErrEncryptionFailed  = errors.New("encryption failed")
	ErrMaxMessageSize    = errors.New("message too large")
	ErrChannelClosed     = errors.New("channel closed")
	ErrHandshakeNotReady = errors.New("handshake not completed")
	ErrInvalidSaltSize   = errors.New("invalid salt size")
	ErrVersionMismatch   = errors.New("protocol version mismatch")
)

const (
	SecureChannelVersion = 1
	NonceSize            = 12
	KeySize              = 32
	MaxSecureMessageSize = 4 << 20
	HandshakeTimeoutSec  = 15
	DefaultTimeoutSec    = 30

	EphemeralKeySize = 32
	HandshakeMsgSize = 1 + EphemeralKeySize

	HKDFInfo = "nogo-secure-channel-v1"

	Argon2Time    = 3
	Argon2Memory  = 64 * 1024
	Argon2Threads = 4
	Argon2SaltLen = 32

	EncryptedMessageHeaderSize = 1 + 8 + 8
)

type SecureChannel struct {
	conn           net.Conn
	encKey         [KeySize]byte
	sendNonce      uint64
	recvNonce      uint64
	isInitiator    bool
	handshakeReady bool
	mu             sync.Mutex
	closed         bool
}

type SecureChannelConfig struct {
	Version        uint8
	KeySize        int
	NonceSize      int
	MaxMessageSize int
	Timeout        time.Duration
}

func DefaultSecureChannelConfig() SecureChannelConfig {
	return SecureChannelConfig{
		Version:        SecureChannelVersion,
		KeySize:        KeySize,
		NonceSize:      NonceSize,
		MaxMessageSize: MaxSecureMessageSize,
		Timeout:        time.Duration(HandshakeTimeoutSec) * time.Second,
	}
}

func NewSecureChannel(conn net.Conn, isInitiator bool) *SecureChannel {
	return &SecureChannel{
		conn:        conn,
		isInitiator: isInitiator,
	}
}

func NewSecureChannelWithKey(conn net.Conn, key []byte, isInitiator bool) (*SecureChannel, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}

	var keyArray [KeySize]byte
	copy(keyArray[:], key)

	return &SecureChannel{
		conn:           conn,
		encKey:         keyArray,
		isInitiator:    isInitiator,
		handshakeReady: true,
	}, nil
}

func NewSecureChannelFromKeyExchange(conn net.Conn, privKey, peerPubKey []byte, isInitiator bool) (*SecureChannel, error) {
	if len(privKey) != EphemeralKeySize {
		return nil, fmt.Errorf("invalid private key size %d: %w", len(privKey), ErrInvalidKeySize)
	}
	if len(peerPubKey) != EphemeralKeySize {
		return nil, fmt.Errorf("invalid peer public key size %d: %w", len(peerPubKey), ErrInvalidKeySize)
	}

	var priv [EphemeralKeySize]byte
	copy(priv[:], privKey)

	var peerPub [EphemeralKeySize]byte
	copy(peerPub[:], peerPubKey)

	ourPubSlice, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("compute public key from private: %w", err)
	}
	var ourPub [EphemeralKeySize]byte
	copy(ourPub[:], ourPubSlice)

	sharedSecret, err := computeX25519SharedSecret(priv, peerPub)
	if err != nil {
		return nil, fmt.Errorf("compute shared secret: %w", err)
	}

	var encKey [KeySize]byte
	if isInitiator {
		encKey, err = deriveEncryptionKey(sharedSecret, ourPub, peerPub)
	} else {
		encKey, err = deriveEncryptionKey(sharedSecret, peerPub, ourPub)
	}
	if err != nil {
		return nil, fmt.Errorf("derive encryption key: %w", err)
	}

	return &SecureChannel{
		conn:           conn,
		encKey:         encKey,
		isInitiator:    isInitiator,
		handshakeReady: true,
	}, nil
}

func generateEphemeralKeyPair() (privateKey [EphemeralKeySize]byte, publicKey [EphemeralKeySize]byte, err error) {
	if _, err = rand.Read(privateKey[:]); err != nil {
		return [EphemeralKeySize]byte{}, [EphemeralKeySize]byte{}, fmt.Errorf("generate ephemeral private key: %w", err)
	}

	pubKeySlice, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		return [EphemeralKeySize]byte{}, [EphemeralKeySize]byte{}, fmt.Errorf("compute ephemeral public key: %w", err)
	}
	copy(publicKey[:], pubKeySlice)

	return privateKey, publicKey, nil
}

func computeX25519SharedSecret(ownPriv [EphemeralKeySize]byte, peerPub [EphemeralKeySize]byte) ([KeySize]byte, error) {
	shared, err := curve25519.X25519(ownPriv[:], peerPub[:])
	if err != nil {
		return [KeySize]byte{}, fmt.Errorf("x25519 scalar multiplication: %w", err)
	}

	var result [KeySize]byte
	copy(result[:], shared)
	return result, nil
}

func deriveEncryptionKey(sharedSecret [KeySize]byte, initiatorPub, responderPub [EphemeralKeySize]byte) ([KeySize]byte, error) {
	saltHash := sha256.New()
	saltHash.Write(initiatorPub[:])
	saltHash.Write(responderPub[:])
	salt := saltHash.Sum(nil)

	reader := hkdf.New(sha256.New, sharedSecret[:], salt, []byte(HKDFInfo))
	var encKey [KeySize]byte
	if _, err := io.ReadFull(reader, encKey[:]); err != nil {
		return [KeySize]byte{}, fmt.Errorf("hkdf expand: %w", err)
	}
	return encKey, nil
}

func (sc *SecureChannel) Handshake() error {
	if err := sc.conn.SetDeadline(time.Now().Add(time.Duration(HandshakeTimeoutSec) * time.Second)); err != nil {
		return fmt.Errorf("set handshake deadline: %w", err)
	}

	var handshakeErr error
	if sc.isInitiator {
		handshakeErr = sc.initiatorHandshake()
	} else {
		handshakeErr = sc.responderHandshake()
	}

	if handshakeErr != nil {
		return handshakeErr
	}

	sc.handshakeReady = true

	if err := sc.conn.SetDeadline(time.Now().Add(time.Duration(DefaultTimeoutSec) * time.Second)); err != nil {
		return fmt.Errorf("set post-handshake deadline: %w", err)
	}
	return nil
}

func (sc *SecureChannel) initiatorHandshake() error {
	ourPriv, ourPub, err := generateEphemeralKeyPair()
	if err != nil {
		return fmt.Errorf("initiator generate ephemeral key: %w", err)
	}

	outMsg := make([]byte, HandshakeMsgSize)
	outMsg[0] = SecureChannelVersion
	copy(outMsg[1:], ourPub[:])

	if _, err := sc.conn.Write(outMsg); err != nil {
		return fmt.Errorf("initiator write handshake: %w", err)
	}

	inMsg := make([]byte, HandshakeMsgSize)
	if _, err := io.ReadFull(sc.conn, inMsg); err != nil {
		return fmt.Errorf("initiator read handshake response: %w", err)
	}

	if inMsg[0] != SecureChannelVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrVersionMismatch, inMsg[0], SecureChannelVersion)
	}

	var peerPub [EphemeralKeySize]byte
	copy(peerPub[:], inMsg[1:])

	sharedSecret, err := computeX25519SharedSecret(ourPriv, peerPub)
	if err != nil {
		return fmt.Errorf("initiator compute shared secret: %w", err)
	}

	encKey, err := deriveEncryptionKey(sharedSecret, ourPub, peerPub)
	if err != nil {
		return fmt.Errorf("initiator derive encryption key: %w", err)
	}

	sc.encKey = encKey
	return nil
}

func (sc *SecureChannel) responderHandshake() error {
	inMsg := make([]byte, HandshakeMsgSize)
	if _, err := io.ReadFull(sc.conn, inMsg); err != nil {
		return fmt.Errorf("responder read handshake: %w", err)
	}

	if inMsg[0] != SecureChannelVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrVersionMismatch, inMsg[0], SecureChannelVersion)
	}

	var peerPub [EphemeralKeySize]byte
	copy(peerPub[:], inMsg[1:])

	ourPriv, ourPub, err := generateEphemeralKeyPair()
	if err != nil {
		return fmt.Errorf("responder generate ephemeral key: %w", err)
	}

	outMsg := make([]byte, HandshakeMsgSize)
	outMsg[0] = SecureChannelVersion
	copy(outMsg[1:], ourPub[:])

	if _, err := sc.conn.Write(outMsg); err != nil {
		return fmt.Errorf("responder write handshake response: %w", err)
	}

	sharedSecret, err := computeX25519SharedSecret(ourPriv, peerPub)
	if err != nil {
		return fmt.Errorf("responder compute shared secret: %w", err)
	}

	encKey, err := deriveEncryptionKey(sharedSecret, peerPub, ourPub)
	if err != nil {
		return fmt.Errorf("responder derive encryption key: %w", err)
	}

	sc.encKey = encKey
	return nil
}

func (sc *SecureChannel) SendEncrypted(msg []byte) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.closed {
		return ErrChannelClosed
	}

	if !sc.handshakeReady {
		return ErrHandshakeNotReady
	}

	if len(msg) > MaxSecureMessageSize {
		return ErrMaxMessageSize
	}

	var nonceArr [NonceSize]byte
	binary.BigEndian.PutUint64(nonceArr[NonceSize-8:], sc.sendNonce)
	sc.sendNonce++

	block, err := aes.NewCipher(sc.encKey[:])
	if err != nil {
		return fmt.Errorf("create aes cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create gcm aead: %w", err)
	}

	ciphertext := aead.Seal(nil, nonceArr[:], msg, nil)

	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(ciphertext)))

	if _, err := sc.conn.Write(header); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if _, err := sc.conn.Write(ciphertext); err != nil {
		return fmt.Errorf("write ciphertext: %w", err)
	}

	return nil
}

func (sc *SecureChannel) RecvEncrypted() ([]byte, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.closed {
		return nil, ErrChannelClosed
	}

	if !sc.handshakeReady {
		return nil, ErrHandshakeNotReady
	}

	var header [4]byte
	if _, err := io.ReadFull(sc.conn, header[:]); err != nil {
		return nil, fmt.Errorf("read frame header: %w", err)
	}

	length := binary.BigEndian.Uint32(header[:])
	if length > MaxSecureMessageSize {
		return nil, ErrMaxMessageSize
	}

	ciphertext := make([]byte, length)
	if _, err := io.ReadFull(sc.conn, ciphertext); err != nil {
		return nil, fmt.Errorf("read ciphertext: %w", err)
	}

	var nonceArr [NonceSize]byte
	binary.BigEndian.PutUint64(nonceArr[NonceSize-8:], sc.recvNonce)
	sc.recvNonce++

	block, err := aes.NewCipher(sc.encKey[:])
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm aead: %w", err)
	}

	plaintext, err := aead.Open(nil, nonceArr[:], ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}

func (sc *SecureChannel) Close() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.closed = true
	return sc.conn.Close()
}

func (sc *SecureChannel) SetDeadline(t time.Time) error {
	return sc.conn.SetDeadline(t)
}

func (sc *SecureChannel) SetReadDeadline(t time.Time) error {
	return sc.conn.SetReadDeadline(t)
}

func (sc *SecureChannel) SetWriteDeadline(t time.Time) error {
	return sc.conn.SetWriteDeadline(t)
}

func (sc *SecureChannel) LocalAddr() net.Addr {
	return sc.conn.LocalAddr()
}

func (sc *SecureChannel) RemoteAddr() net.Addr {
	return sc.conn.RemoteAddr()
}

type SecureChannelServer struct {
	listener net.Listener
	config   SecureChannelConfig
	done     chan struct{}
}

func NewSecureChannelServer(listenAddr string) (*SecureChannelServer, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", listenAddr, err)
	}

	return &SecureChannelServer{
		listener: ln,
		config:   DefaultSecureChannelConfig(),
		done:     make(chan struct{}),
	}, nil
}

func (s *SecureChannelServer) Accept() (*SecureChannel, error) {
	conn, err := s.listener.Accept()
	if err != nil {
		return nil, fmt.Errorf("accept connection: %w", err)
	}

	sc := NewSecureChannel(conn, false)
	if err := sc.Handshake(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ecdh handshake: %w", err)
	}

	return sc, nil
}

func (s *SecureChannelServer) Close() error {
	close(s.done)
	return s.listener.Close()
}

func (s *SecureChannelServer) Addr() net.Addr {
	return s.listener.Addr()
}

func DeriveKeyFromPassword(password string, salt []byte) ([]byte, error) {
	if len(salt) != Argon2SaltLen {
		return nil, fmt.Errorf("expected salt size %d: %w", Argon2SaltLen, ErrInvalidSaltSize)
	}

	derived := argon2.IDKey(
		[]byte(password),
		salt,
		Argon2Time,
		Argon2Memory,
		uint8(Argon2Threads),
		KeySize,
	)

	return derived, nil
}

func GenerateSalt() ([]byte, error) {
	salt := make([]byte, Argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	return salt, nil
}

type EncryptedMessage struct {
	Version    uint8
	Timestamp  int64
	Nonce      uint64
	Ciphertext []byte
}

func (em *EncryptedMessage) Serialize() ([]byte, error) {
	result := make([]byte, EncryptedMessageHeaderSize+len(em.Ciphertext))
	result[0] = em.Version
	binary.BigEndian.PutUint64(result[1:9], uint64(em.Timestamp))
	binary.BigEndian.PutUint64(result[9:17], em.Nonce)
	copy(result[EncryptedMessageHeaderSize:], em.Ciphertext)
	return result, nil
}

func DeserializeEncryptedMessage(data []byte) (*EncryptedMessage, error) {
	if len(data) < EncryptedMessageHeaderSize {
		return nil, errors.New("data too short for encrypted message header")
	}

	return &EncryptedMessage{
		Version:    data[0],
		Timestamp:  int64(binary.BigEndian.Uint64(data[1:9])),
		Nonce:      binary.BigEndian.Uint64(data[9:17]),
		Ciphertext: data[EncryptedMessageHeaderSize:],
	}, nil
}
