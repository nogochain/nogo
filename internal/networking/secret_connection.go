package networking

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/secretbox"
)

// SecretConnection constants for frame-based encrypted communication.
const (
	SecretConnVersion      = 1
	SecretMaxPlaintext     = 1024
	SecretPoly1305TagSize  = 16
	SecretNonceSize        = 24
	SecretEphemeralKeySize = 32
	SecretSignatureSize    = 64
	SecretHandshakeTimeout = 30 * time.Second
)

// Errors for SecretConnection operations.
var (
	ErrSecretHandshakeFailed     = fmt.Errorf("secret connection handshake failed")
	ErrSecretInvalidEphemeralKey = fmt.Errorf("invalid ephemeral public key size")
	ErrSecretInvalidSignature    = fmt.Errorf("invalid ed25519 signature")
	ErrSecretSignatureVerify     = fmt.Errorf("remote signature verification failed")
	ErrSecretDecryptFailed       = fmt.Errorf("frame decryption failed")
	ErrSecretEncryptFailed       = fmt.Errorf("frame encryption failed")
	ErrSecretConnClosed          = fmt.Errorf("secret connection closed")
)

// SecretConnection wraps a net.Conn with XSalsa20-Poly1305 encryption providing
// forward secrecy through ephemeral Curve25519 key exchange and mutual authentication
// via Ed25519 signatures.
type SecretConnection struct {
	conn       io.ReadWriteCloser
	sendKey    [32]byte
	recvKey    [32]byte
	sendNonce  uint64
	recvNonce  uint64
	remotePub  ed25519.PublicKey
	localPub   ed25519.PublicKey
	muSend     sync.Mutex
	muRecv     sync.Mutex
	muClose    sync.Mutex
	closed     bool
	recvBuf    bytes.Buffer
}

// MakeSecretConnection performs the complete handshake protocol to establish
// an encrypted connection with forward secrecy and mutual authentication.
// isInitiator determines the handshake order to avoid deadlock:
//   - Initiator: write first, then read
//   - Responder: read first, then write
//
// Handshake protocol:
// 1. Generate ephemeral Curve25519 key pair (forward secrecy)
// 2. Exchange ephemeral public keys (32 bytes each, binary encoding)
// 3. Compute shared secret via X25519 ECDH
// 4. Sort public keys lexicographically to derive send/recv nonces
// 5. Sign challenge (SHA256 of both ephemeral pub keys concatenated)
// 6. Exchange Ed25519 signatures for mutual authentication
// 7. Verify remote signature
// 8. Initialize XSalsa20-Poly1305 ciphers for send/recv
func MakeSecretConnection(conn io.ReadWriteCloser, locPrivKey ed25519.PrivateKey, isInitiator bool) (*SecretConnection, error) {
	sc := &SecretConnection{
		conn:     conn,
		localPub: locPrivKey.Public().(ed25519.PublicKey),
	}

	// Set handshake timeout
	if err := sc.setDeadline(time.Now().Add(SecretHandshakeTimeout)); err != nil {
		return nil, fmt.Errorf("set handshake deadline: %w", err)
	}

	// Step 1: Generate ephemeral Curve25519 key pair
	var ephemeralPriv [SecretEphemeralKeySize]byte
	if _, err := rand.Read(ephemeralPriv[:]); err != nil {
		return nil, fmt.Errorf("generate ephemeral private key: %w", err)
	}

	// Clamp the private key for Curve25519 compatibility
	ephemeralPriv[0] &= 248
	ephemeralPriv[31] &= 127
	ephemeralPriv[31] |= 64

	ephemeralPubSlice, err := curve25519.X25519(ephemeralPriv[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("compute ephemeral public key: %w", err)
	}
	var ephemeralPub [SecretEphemeralKeySize]byte
	copy(ephemeralPub[:], ephemeralPubSlice)

	// Step 2: Exchange ephemeral public keys
	var remoteEphemeralPub [SecretEphemeralKeySize]byte
	if isInitiator {
		// Initiator writes first, then reads (avoids deadlock)
		if _, err := conn.Write(ephemeralPub[:]); err != nil {
			return nil, fmt.Errorf("write ephemeral public key: %w", err)
		}
		if _, err := io.ReadFull(conn, remoteEphemeralPub[:]); err != nil {
			return nil, fmt.Errorf("read remote ephemeral public key: %w", err)
		}
	} else {
		// Responder reads first, then writes (avoids deadlock)
		if _, err := io.ReadFull(conn, remoteEphemeralPub[:]); err != nil {
			return nil, fmt.Errorf("read remote ephemeral public key: %w", err)
		}
		if _, err := conn.Write(ephemeralPub[:]); err != nil {
			return nil, fmt.Errorf("write ephemeral public key: %w", err)
		}
	}

	// Step 3: Compute shared secret via X25519 ECDH
	sharedSecretSlice, err := curve25519.X25519(ephemeralPriv[:], remoteEphemeralPub[:])
	if err != nil {
		return nil, fmt.Errorf("compute x25519 shared secret: %w", err)
	}
	var sharedSecret [32]byte
	copy(sharedSecret[:], sharedSecretSlice)

	// Step 4: Compute challenge hash deterministically
	// Both parties must compute the same challenge hash, so we sort ephemeral pub keys
	var challengeData [SecretEphemeralKeySize * 2]byte
	if bytes.Compare(ephemeralPub[:], remoteEphemeralPub[:]) < 0 {
		copy(challengeData[:SecretEphemeralKeySize], ephemeralPub[:])
		copy(challengeData[SecretEphemeralKeySize:], remoteEphemeralPub[:])
	} else {
		copy(challengeData[:SecretEphemeralKeySize], remoteEphemeralPub[:])
		copy(challengeData[SecretEphemeralKeySize:], ephemeralPub[:])
	}

	challengeHash := sha256.Sum256(challengeData[:])

	// Step 5: Sign the challenge
	ourPubKey := sc.localPub
	signature := ed25519.Sign(locPrivKey, challengeHash[:])

	// Step 6: Exchange Ed25519 signatures and public keys
	// Protocol: [64 bytes signature][2 bytes pubkey length][32 bytes pubkey]
	// Initiator writes first (signature + pubkey), then reads (signature + pubkey)
	// Responder reads first (signature + pubkey), then writes (signature + pubkey)
	var remotePubKeySize [2]byte
	var remoteSignature [SecretSignatureSize]byte

	if isInitiator {
		// Send our signature
		if _, err := conn.Write(signature); err != nil {
			return nil, fmt.Errorf("write ed25519 signature: %w", err)
		}
		// Send our Ed25519 public key
		pubKeySize := uint16(len(ourPubKey))
		pubKeySizeBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(pubKeySizeBuf, pubKeySize)
		if _, err := conn.Write(pubKeySizeBuf); err != nil {
			return nil, fmt.Errorf("write local pub key size: %w", err)
		}
		if _, err := conn.Write(ourPubKey); err != nil {
			return nil, fmt.Errorf("write local public key: %w", err)
		}

		// Read remote signature
		if _, err := io.ReadFull(conn, remoteSignature[:]); err != nil {
			return nil, fmt.Errorf("read remote signature: %w", err)
		}
		if _, err := io.ReadFull(conn, remotePubKeySize[:]); err != nil {
			return nil, fmt.Errorf("read remote pub key size: %w", err)
		}
		remotePubLen := binary.BigEndian.Uint16(remotePubKeySize[:])
		if remotePubLen != uint16(ed25519.PublicKeySize) {
			return nil, fmt.Errorf("invalid remote public key size: %d", remotePubLen)
		}
		remotePubKey := make([]byte, ed25519.PublicKeySize)
		if _, err := io.ReadFull(conn, remotePubKey); err != nil {
			return nil, fmt.Errorf("read remote public key: %w", err)
		}

		// Step 7: Verify remote signature
		if !ed25519.Verify(remotePubKey, challengeHash[:], remoteSignature[:]) {
			return nil, ErrSecretSignatureVerify
		}

		sc.remotePub = remotePubKey
	} else {
		// Read remote signature first
		if _, err := io.ReadFull(conn, remoteSignature[:]); err != nil {
			return nil, fmt.Errorf("read remote signature: %w", err)
		}
		if _, err := io.ReadFull(conn, remotePubKeySize[:]); err != nil {
			return nil, fmt.Errorf("read remote pub key size: %w", err)
		}
		remotePubLen := binary.BigEndian.Uint16(remotePubKeySize[:])
		if remotePubLen != uint16(ed25519.PublicKeySize) {
			return nil, fmt.Errorf("invalid remote public key size: %d", remotePubLen)
		}
		remotePubKey := make([]byte, ed25519.PublicKeySize)
		if _, err := io.ReadFull(conn, remotePubKey); err != nil {
			return nil, fmt.Errorf("read remote public key: %w", err)
		}

		// Verify remote signature before sending our own
		if !ed25519.Verify(remotePubKey, challengeHash[:], remoteSignature[:]) {
			return nil, ErrSecretSignatureVerify
		}

		// Send our signature
		if _, err := conn.Write(signature); err != nil {
			return nil, fmt.Errorf("write ed25519 signature: %w", err)
		}
		// Send our Ed25519 public key
		pubKeySize := uint16(len(ourPubKey))
		pubKeySizeBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(pubKeySizeBuf, pubKeySize)
		if _, err := conn.Write(pubKeySizeBuf); err != nil {
			return nil, fmt.Errorf("write local pub key size: %w", err)
		}
		if _, err := conn.Write(ourPubKey); err != nil {
			return nil, fmt.Errorf("write local public key: %w", err)
		}

		sc.remotePub = remotePubKey
	}

	// Step 8: Initialize XSalsa20-Poly1305 ciphers
	// Derive send and recv keys from shared secret
	// The initiator's send key = responder's recv key and vice versa
	sendKey, recvKey := deriveKeys(sharedSecret, ephemeralPub, remoteEphemeralPub, isInitiator)

	sc.sendKey = sendKey
	sc.recvKey = recvKey
	sc.sendNonce = 0
	sc.recvNonce = 0

	// Reset deadline after successful handshake
	if err := sc.setDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("reset deadline after handshake: %w", err)
	}

	return sc, nil
}

// deriveKeys derives send and receive keys from the shared secret and ephemeral public keys.
// Both parties derive keys using the same sorted ordering of ephemeral pub keys,
// ensuring that initiator.sendKey == responder.recvKey and vice versa.
func deriveKeys(sharedSecret [32]byte, localEphPub, remoteEphPub [SecretEphemeralKeySize]byte, isInitiator bool) ([32]byte, [32]byte) {
	// Sort ephemeral pub keys deterministically so both parties derive the same keys
	var firstPub, secondPub [SecretEphemeralKeySize]byte
	if bytes.Compare(localEphPub[:], remoteEphPub[:]) < 0 {
		firstPub = localEphPub
		secondPub = remoteEphPub
	} else {
		firstPub = remoteEphPub
		secondPub = localEphPub
	}

	// Derive two distinct keys using sorted ordering
	keyMaterialA := make([]byte, 0, 32+SecretEphemeralKeySize*2)
	keyMaterialA = append(keyMaterialA, firstPub[:]...)
	keyMaterialA = append(keyMaterialA, secondPub[:]...)
	keyMaterialA = append(keyMaterialA, sharedSecret[:]...)

	keyMaterialB := append([]byte{0x01}, keyMaterialA...)

	// Hash both to derive two distinct 32-byte keys
	hashA := sha256.Sum256(keyMaterialA)
	hashB := sha256.Sum256(keyMaterialB)

	// Initiator uses hashA for sending, hashB for receiving
	// Responder uses hashB for sending, hashA for receiving
	// This ensures initiator.sendKey == responder.recvKey and vice versa
	if isInitiator {
		return hashA, hashB
	}
	return hashB, hashA
}

// Read decrypts incoming frames and copies plaintext to data.
// Frame format: [2 bytes length][ciphertext + 16 bytes Poly1305 tag]
// Max plaintext per frame: 1024 bytes
// Increments recvNonce by 2 after each frame.
func (sc *SecretConnection) Read(data []byte) (int, error) {
	sc.muRecv.Lock()
	defer sc.muRecv.Unlock()

	if sc.isClosed() {
		return 0, ErrSecretConnClosed
	}

	// If we have buffered data from a previous frame, copy from buffer first
	if sc.recvBuf.Len() > 0 {
		return sc.recvBuf.Read(data)
	}

	// Read frame length (2 bytes, big-endian)
	var lengthBuf [2]byte
	if _, err := io.ReadFull(sc.conn, lengthBuf[:]); err != nil {
		return 0, fmt.Errorf("read frame length: %w", err)
	}

	frameLength := int(binary.BigEndian.Uint16(lengthBuf[:]))

	// Validate frame length (max plaintext + Poly1305 tag)
	maxFrameLength := SecretMaxPlaintext + SecretPoly1305TagSize
	if frameLength <= 0 || frameLength > maxFrameLength {
		return 0, fmt.Errorf("invalid frame length: %d", frameLength)
	}

	// Read ciphertext (includes Poly1305 tag)
	ciphertextWithTag := make([]byte, frameLength)
	if _, err := io.ReadFull(sc.conn, ciphertextWithTag); err != nil {
		return 0, fmt.Errorf("read ciphertext: %w", err)
	}

	// Build 24-byte nonce: first 8 bytes = counter as big-endian uint64, remaining 16 bytes = zero
	var nonce [SecretNonceSize]byte
	binary.BigEndian.PutUint64(nonce[:8], sc.recvNonce)
	sc.recvNonce += 2

	// Decrypt using XSalsa20-Poly1305
	plaintext, ok := secretbox.Open(nil, ciphertextWithTag, &nonce, &sc.recvKey)
	if !ok {
		return 0, ErrSecretDecryptFailed
	}

	// Copy plaintext to user buffer, buffer any remainder
	n := copy(data, plaintext)
	if n < len(plaintext) {
		// Buffer the remaining plaintext for next read
		sc.recvBuf.Write(plaintext[n:])
	}

	return n, nil
}

// Write encrypts data into frames and writes to the underlying connection.
// Data is split into 1024-byte chunks, each encrypted separately.
// Frame format: [2 bytes length][ciphertext + 16 bytes Poly1305 tag]
// Increments sendNonce by 2 after each frame.
func (sc *SecretConnection) Write(data []byte) (int, error) {
	sc.muSend.Lock()
	defer sc.muSend.Unlock()

	if sc.isClosed() {
		return 0, ErrSecretConnClosed
	}

	totalWritten := 0
	dataLen := len(data)

	for offset := 0; offset < dataLen; {
		// Determine chunk size (max 1024 bytes per frame)
		chunkSize := dataLen - offset
		if chunkSize > SecretMaxPlaintext {
			chunkSize = SecretMaxPlaintext
		}

		chunk := data[offset : offset+chunkSize]

		// Build 24-byte nonce: first 8 bytes = counter as big-endian uint64, remaining 16 bytes = zero
		var nonce [SecretNonceSize]byte
		binary.BigEndian.PutUint64(nonce[:8], sc.sendNonce)
		sc.sendNonce += 2

		// Encrypt using XSalsa20-Poly1305
		ciphertextWithTag := secretbox.Seal(nil, chunk, &nonce, &sc.sendKey)

		// Build frame: [2 bytes length][ciphertext + 16 bytes Poly1305 tag]
		frameLength := uint16(len(ciphertextWithTag))
		lengthBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(lengthBuf, frameLength)

		// Write frame to underlying connection
		if _, err := sc.conn.Write(lengthBuf); err != nil {
			return totalWritten, fmt.Errorf("write frame length: %w", err)
		}
		if _, err := sc.conn.Write(ciphertextWithTag); err != nil {
			return totalWritten, fmt.Errorf("write ciphertext: %w", err)
		}

		totalWritten += chunkSize
		offset += chunkSize
	}

	return totalWritten, nil
}

// RemotePubKey returns the authenticated remote Ed25519 public key.
func (sc *SecretConnection) RemotePubKey() ed25519.PublicKey {
	return sc.remotePub
}

// Close closes the SecretConnection and underlying connection.
func (sc *SecretConnection) Close() error {
	sc.muClose.Lock()
	defer sc.muClose.Unlock()

	if sc.closed {
		return ErrSecretConnClosed
	}

	sc.closed = true

	// Clear sensitive key material
	sc.sendKey = [32]byte{}
	sc.recvKey = [32]byte{}

	if closer, ok := sc.conn.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// LocalAddr returns the local network address.
func (sc *SecretConnection) LocalAddr() net.Addr {
	if addrer, ok := sc.conn.(interface{ LocalAddr() net.Addr }); ok {
		return addrer.LocalAddr()
	}
	return nil
}

// RemoteAddr returns the remote network address.
func (sc *SecretConnection) RemoteAddr() net.Addr {
	if addrer, ok := sc.conn.(interface{ RemoteAddr() net.Addr }); ok {
		return addrer.RemoteAddr()
	}
	return nil
}

// SetDeadline sets the read and write deadlines.
func (sc *SecretConnection) SetDeadline(t time.Time) error {
	if sc.isClosed() {
		return ErrSecretConnClosed
	}

	// We need to set deadlines on both read and write operations
	// Since we have separate mutexes for send and recv, we set deadline on the underlying connection
	if deadlineSetter, ok := sc.conn.(interface{ SetDeadline(t time.Time) error }); ok {
		return deadlineSetter.SetDeadline(t)
	}
	return nil
}

// SetReadDeadline sets the read deadline.
func (sc *SecretConnection) SetReadDeadline(t time.Time) error {
	if sc.isClosed() {
		return ErrSecretConnClosed
	}
	if deadlineSetter, ok := sc.conn.(interface{ SetReadDeadline(t time.Time) error }); ok {
		return deadlineSetter.SetReadDeadline(t)
	}
	return nil
}

// SetWriteDeadline sets the write deadline.
func (sc *SecretConnection) SetWriteDeadline(t time.Time) error {
	if sc.isClosed() {
		return ErrSecretConnClosed
	}
	if deadlineSetter, ok := sc.conn.(interface{ SetWriteDeadline(t time.Time) error }); ok {
		return deadlineSetter.SetWriteDeadline(t)
	}
	return nil
}

// setDeadline is an internal helper to set deadline on the underlying connection.
func (sc *SecretConnection) setDeadline(t time.Time) error {
	if deadlineSetter, ok := sc.conn.(interface{ SetDeadline(t time.Time) error }); ok {
		return deadlineSetter.SetDeadline(t)
	}
	return nil
}

// isClosed checks if the connection is closed (must be called with muClose held or during initialization).
func (sc *SecretConnection) isClosed() bool {
	sc.muClose.Lock()
	defer sc.muClose.Unlock()
	return sc.closed
}

// Ensure SecretConnection implements net.Conn interface at compile time.
var _ net.Conn = (*SecretConnection)(nil)
