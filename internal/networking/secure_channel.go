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
)

var (
	ErrInvalidKeySize   = errors.New("invalid key size")
	ErrDecryptionFailed = errors.New("decryption failed")
	ErrEncryptionFailed = errors.New("encryption failed")
	ErrMaxMessageSize   = errors.New("message too large")
)

const (
	SecureChannelVersion = 1
	NonceSize            = 12
	KeySize              = 32
	MaxSecureMessageSize = 4 << 20
	HandshakeTimeoutSec  = 15
)

type SecureChannel struct {
	conn        net.Conn
	key         [KeySize]byte
	peerKey     [KeySize]byte
	isInitiator bool
	nonce       uint64
	mu          sync.Mutex
	closed      bool
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
		Timeout:        15 * time.Second,
	}
}

func NewSecureChannel(conn net.Conn, key []byte, isInitiator bool) (*SecureChannel, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}

	var keyArray [KeySize]byte
	copy(keyArray[:], key)

	return &SecureChannel{
		conn:        conn,
		key:         keyArray,
		isInitiator: isInitiator,
	}, nil
}

func NewSecureChannelFromKeyExchange(conn net.Conn, privKey, pubKey []byte, isInitiator bool) (*SecureChannel, error) {
	sharedSecret, err := deriveSharedSecret(privKey, pubKey)
	if err != nil {
		return nil, err
	}

	return NewSecureChannel(conn, sharedSecret, isInitiator)
}

func deriveSharedSecret(privKey, pubKey []byte) ([]byte, error) {
	h := sha256.New()
	h.Write(privKey)
	h.Write(pubKey)
	sum := h.Sum(nil)

	if len(sum) < KeySize {
		return nil, ErrInvalidKeySize
	}

	return sum[:KeySize], nil
}

func (sc *SecureChannel) Handshake() error {
	_ = sc.conn.SetDeadline(time.Now().Add(HandshakeTimeoutSec * time.Second))

	if sc.isInitiator {
		return sc.initiatorHandshake()
	}
	return sc.responderHandshake()
}

func (sc *SecureChannel) initiatorHandshake() error {
	msg := make([]byte, 1+KeySize+NonceSize)
	msg[0] = SecureChannelVersion
	rand.Read(msg[1 : 1+KeySize])
	rand.Read(msg[1+KeySize:])

	if _, err := sc.conn.Write(msg); err != nil {
		return fmt.Errorf("write handshake: %w", err)
	}

	resp := make([]byte, 1+KeySize+NonceSize)
	if _, err := io.ReadFull(sc.conn, resp); err != nil {
		return fmt.Errorf("read handshake response: %w", err)
	}

	if resp[0] != SecureChannelVersion {
		return fmt.Errorf("version mismatch")
	}

	copy(sc.peerKey[:], resp[1:1+KeySize])

	combined := append(sc.key[:], resp[1+KeySize:]...)
	h := sha256.Sum256(combined)
	copy(sc.key[:], h[:])

	_ = sc.conn.SetDeadline(time.Now().Add(30 * time.Second))
	return nil
}

func (sc *SecureChannel) responderHandshake() error {
	msg := make([]byte, 1+KeySize+NonceSize)
	if _, err := io.ReadFull(sc.conn, msg); err != nil {
		return fmt.Errorf("read handshake: %w", err)
	}

	if msg[0] != SecureChannelVersion {
		return fmt.Errorf("version mismatch")
	}

	peerKey := msg[1 : 1+KeySize]
	peerNonce := msg[1+KeySize:]

	resp := make([]byte, 1+KeySize+NonceSize)
	resp[0] = SecureChannelVersion
	rand.Read(resp[1 : 1+KeySize])
	rand.Read(resp[1+KeySize:])

	if _, err := sc.conn.Write(resp); err != nil {
		return fmt.Errorf("write handshake response: %w", err)
	}

	copy(sc.peerKey[:], peerKey)

	combined := append(resp[1:1+KeySize], peerNonce...)
	h := sha256.Sum256(combined)
	copy(sc.key[:], h[:])

	_ = sc.conn.SetDeadline(time.Now().Add(30 * time.Second))
	return nil
}

func (sc *SecureChannel) SendEncrypted(msg []byte) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.closed {
		return errors.New("channel closed")
	}

	if len(msg) > MaxSecureMessageSize {
		return ErrMaxMessageSize
	}

	var nonceArr [NonceSize]byte
	binary.BigEndian.PutUint64(nonceArr[4:], sc.nonce)
	sc.nonce++

	block, err := aes.NewCipher(sc.key[:])
	if err != nil {
		return fmt.Errorf("create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create aead: %w", err)
	}

	ciphertext := aead.Seal(nil, nonceArr[:], msg, nil)

	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(ciphertext)))

	if _, err := sc.conn.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
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
		return nil, errors.New("channel closed")
	}

	var header [4]byte
	if _, err := io.ReadFull(sc.conn, header[:]); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
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
	binary.BigEndian.PutUint64(nonceArr[4:], sc.nonce)
	sc.nonce++

	block, err := aes.NewCipher(sc.key[:])
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create aead: %w", err)
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
	privKey  []byte
	peerKeys map[string]struct{}
	mu       sync.RWMutex
	done     chan struct{}
}

func NewSecureChannelServer(listenAddr string, privKey []byte) (*SecureChannelServer, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	return &SecureChannelServer{
		listener: ln,
		config:   DefaultSecureChannelConfig(),
		privKey:  privKey,
		peerKeys: make(map[string]struct{}),
		done:     make(chan struct{}),
	}, nil
}

func (s *SecureChannelServer) Accept() (*SecureChannel, error) {
	conn, err := s.listener.Accept()
	if err != nil {
		return nil, fmt.Errorf("accept: %w", err)
	}

	sc, err := NewSecureChannel(conn, s.privKey, false)
	if err != nil {
		conn.Close()
		return nil, err
	}

	if err := sc.Handshake(); err != nil {
		sc.Close()
		return nil, fmt.Errorf("handshake: %w", err)
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
	h := sha256.New()
	h.Write([]byte(password))
	h.Write(salt)
	sum := h.Sum(nil)

	for i := 1; i < 10000; i++ {
		h.Reset()
		h.Write(sum)
		h.Write([]byte{byte(i & 0xff), byte((i >> 8) & 0xff), byte((i >> 16) & 0xff)})
		sum = h.Sum(nil)
	}

	return sum[:KeySize], nil
}

func GenerateSalt() ([]byte, error) {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
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
	result := make([]byte, 1+8+8+len(em.Ciphertext))
	result[0] = em.Version
	binary.BigEndian.PutUint64(result[1:9], uint64(em.Timestamp))
	binary.BigEndian.PutUint64(result[9:17], em.Nonce)
	copy(result[17:], em.Ciphertext)
	return result, nil
}

func DeserializeEncryptedMessage(data []byte) (*EncryptedMessage, error) {
	if len(data) < 17 {
		return nil, errors.New("data too short")
	}

	return &EncryptedMessage{
		Version:    data[0],
		Timestamp:  int64(binary.BigEndian.Uint64(data[1:9])),
		Nonce:      binary.BigEndian.Uint64(data[9:17]),
		Ciphertext: data[17:],
	}, nil
}
