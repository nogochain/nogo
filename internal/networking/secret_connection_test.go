package networking

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// generateTestKeyPair generates an Ed25519 key pair for testing.
func generateTestKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate test key pair: %v", err)
	}
	return pub, priv
}

// pipeConn creates a pair of connected io.ReadWriteClosers for testing.
func pipeConn(t *testing.T) (io.ReadWriteCloser, io.ReadWriteCloser) {
	t.Helper()
	server, client := net.Pipe()
	return server, client
}

// TestSecretConnectionHandshake tests the complete handshake protocol between two parties.
func TestSecretConnectionHandshake(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	serverPub, serverPriv := generateTestKeyPair(t)
	clientPub, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection
	var serverErr, clientErr error

	// Server side handshake (responder = false)
	go func() {
		defer wg.Done()
		serverSC, serverErr = MakeSecretConnection(serverConn, serverPriv, false)
	}()

	// Client side handshake (initiator = true)
	go func() {
		defer wg.Done()
		clientSC, clientErr = MakeSecretConnection(clientConn, clientPriv, true)
	}()

	wg.Wait()

	if serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}
	if clientErr != nil {
		t.Fatalf("client handshake failed: %v", clientErr)
	}

	// Verify remote public keys are correctly authenticated
	if !bytes.Equal(serverSC.RemotePubKey(), clientPub) {
		t.Errorf("server remote pubkey mismatch: got %v, want %v", serverSC.RemotePubKey(), clientPub)
	}
	if !bytes.Equal(clientSC.RemotePubKey(), serverPub) {
		t.Errorf("client remote pubkey mismatch: got %v, want %v", clientSC.RemotePubKey(), serverPub)
	}

	// Cleanup
	serverSC.Close()
	clientSC.Close()
}

// TestSecretConnectionReadWrite tests basic encrypted read/write operations.
func TestSecretConnectionReadWrite(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	_, serverPriv := generateTestKeyPair(t)
	_, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		var err error
		serverSC, err = MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			t.Errorf("server handshake failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		clientSC, err = MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			t.Errorf("client handshake failed: %v", err)
		}
	}()

	wg.Wait()

	if serverSC == nil || clientSC == nil {
		t.Fatal("handshake failed, connections are nil")
	}

	// Test single frame write/read (less than 1024 bytes)
	testData := []byte("Hello, SecretConnection!")

	var writeWg sync.WaitGroup
	writeWg.Add(1)

	go func() {
		defer writeWg.Done()
		n, err := clientSC.Write(testData)
		if err != nil {
			t.Errorf("client write failed: %v", err)
		}
		if n != len(testData) {
			t.Errorf("client wrote %d bytes, want %d", n, len(testData))
		}
	}()

	// Server reads
	readBuf := make([]byte, 1024)
	n, err := serverSC.Read(readBuf)
	if err != nil {
		t.Fatalf("server read failed: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("server read %d bytes, want %d", n, len(testData))
	}
	if !bytes.Equal(readBuf[:n], testData) {
		t.Fatalf("server read mismatch: got %q, want %q", readBuf[:n], testData)
	}

	writeWg.Wait()

	serverSC.Close()
	clientSC.Close()
}

// TestSecretConnectionLargeData tests write/read with data larger than one frame (1024 bytes).
func TestSecretConnectionLargeData(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	_, serverPriv := generateTestKeyPair(t)
	_, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		var err error
		serverSC, err = MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			t.Errorf("server handshake failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		clientSC, err = MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			t.Errorf("client handshake failed: %v", err)
		}
	}()

	wg.Wait()

	if serverSC == nil || clientSC == nil {
		t.Fatal("handshake failed")
	}

	// Test data spanning multiple frames (3000 bytes = 3 frames)
	testData := make([]byte, 3000)
	if _, err := rand.Read(testData); err != nil {
		t.Fatalf("failed to generate random test data: %v", err)
	}

	var writeWg sync.WaitGroup
	writeWg.Add(1)

	go func() {
		defer writeWg.Done()
		n, err := clientSC.Write(testData)
		if err != nil {
			t.Errorf("client write failed: %v", err)
		}
		if n != len(testData) {
			t.Errorf("client wrote %d bytes, want %d", n, len(testData))
		}
	}()

	// Server reads all data
	readBuf := make([]byte, 4096)
	totalRead := 0
	for totalRead < len(testData) {
		n, err := serverSC.Read(readBuf[totalRead:])
		if err != nil && err != io.EOF {
			t.Fatalf("server read failed: %v", err)
		}
		totalRead += n
		if n == 0 {
			break
		}
	}

	if totalRead != len(testData) {
		t.Fatalf("server read %d bytes, want %d", totalRead, len(testData))
	}
	if !bytes.Equal(readBuf[:totalRead], testData) {
		t.Fatal("server read mismatch for large data")
	}

	writeWg.Wait()

	serverSC.Close()
	clientSC.Close()
}

// TestSecretConnectionMultipleMessages tests multiple sequential messages.
func TestSecretConnectionMultipleMessages(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	_, serverPriv := generateTestKeyPair(t)
	_, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		var err error
		serverSC, err = MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			t.Errorf("server handshake failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		clientSC, err = MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			t.Errorf("client handshake failed: %v", err)
		}
	}()

	wg.Wait()

	if serverSC == nil || clientSC == nil {
		t.Fatal("handshake failed")
	}

	messages := [][]byte{
		[]byte("Message 1"),
		[]byte("Message 2 - longer content here"),
		make([]byte, 1024), // Exactly one frame
		make([]byte, 2048), // Two frames
	}

	// Fill random data for larger messages
	for _, msg := range messages {
		if len(msg) > 10 {
			rand.Read(msg)
		}
	}

	for i, msg := range messages {
		var writeErr error
		var written int

		var writeDone sync.WaitGroup
		writeDone.Add(1)

		go func() {
			defer writeDone.Done()
			written, writeErr = clientSC.Write(msg)
		}()

		// Read on server
		readBuf := make([]byte, len(msg)+SecretMaxPlaintext)
		totalRead := 0
		for totalRead < len(msg) {
			n, err := serverSC.Read(readBuf[totalRead:])
			if err != nil && err != io.EOF {
				t.Fatalf("failed to read message %d: %v", i, err)
			}
			totalRead += n
			if n == 0 {
				break
			}
		}

		writeDone.Wait()

		if writeErr != nil {
			t.Fatalf("failed to write message %d: %v", i, writeErr)
		}
		if written != len(msg) {
			t.Fatalf("message %d: wrote %d bytes, want %d", i, written, len(msg))
		}
		if totalRead != len(msg) {
			t.Fatalf("message %d: read %d bytes, want %d", i, totalRead, len(msg))
		}
		if !bytes.Equal(readBuf[:totalRead], msg) {
			t.Fatalf("message %d: content mismatch", i)
		}
	}

	serverSC.Close()
	clientSC.Close()
}

// TestSecretConnectionConcurrentReadWrite tests concurrent reads and writes.
func TestSecretConnectionConcurrentReadWrite(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	_, serverPriv := generateTestKeyPair(t)
	_, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		var err error
		serverSC, err = MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			t.Errorf("server handshake failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		clientSC, err = MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			t.Errorf("client handshake failed: %v", err)
		}
	}()

	wg.Wait()

	if serverSC == nil || clientSC == nil {
		t.Fatal("handshake failed")
	}

	const numMessages = 50
	messageSize := 512

	// Client sends messages
	var sendWg sync.WaitGroup
	sendWg.Add(numMessages)

	go func() {
		for i := 0; i < numMessages; i++ {
			msg := make([]byte, messageSize)
			msg[0] = byte(i)
			msg[1] = byte(i >> 8)
			rand.Read(msg[2:])

			go func(idx int, data []byte) {
				defer sendWg.Done()
				_, err := clientSC.Write(data)
				if err != nil {
					t.Errorf("failed to send message %d: %v", idx, err)
				}
			}(i, msg)
		}
	}()

	// Server receives messages
	var recvWg sync.WaitGroup
	recvWg.Add(numMessages)

	go func() {
		for i := 0; i < numMessages; i++ {
			go func() {
				defer recvWg.Done()
				buf := make([]byte, messageSize+SecretMaxPlaintext)
				totalRead := 0
				for totalRead < messageSize {
					n, err := serverSC.Read(buf[totalRead:])
					if err != nil && err != io.EOF {
						t.Errorf("failed to receive message: %v", err)
						return
					}
					totalRead += n
					if n == 0 {
						break
					}
				}
			}()
		}
	}()

	sendWg.Wait()
	recvWg.Wait()

	serverSC.Close()
	clientSC.Close()
}

// TestSecretConnectionClose tests proper connection cleanup.
func TestSecretConnectionClose(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	_, serverPriv := generateTestKeyPair(t)
	_, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		var err error
		serverSC, err = MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			t.Errorf("server handshake failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		clientSC, err = MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			t.Errorf("client handshake failed: %v", err)
		}
	}()

	wg.Wait()

	if serverSC == nil || clientSC == nil {
		t.Fatal("handshake failed")
	}

	// Test double close
	if err := serverSC.Close(); err != nil {
		t.Errorf("first close failed: %v", err)
	}
	if err := serverSC.Close(); err != ErrSecretConnClosed {
		t.Errorf("second close should return ErrSecretConnClosed, got: %v", err)
	}

	// Test operations after close
	_, err := serverSC.Write([]byte("test"))
	if err != ErrSecretConnClosed {
		t.Errorf("write after close should return ErrSecretConnClosed, got: %v", err)
	}

	_, err = serverSC.Read(make([]byte, 10))
	if err != ErrSecretConnClosed {
		t.Errorf("read after close should return ErrSecretConnClosed, got: %v", err)
	}

	clientSC.Close()
}

// TestSecretConnectionNetConnInterface verifies SecretConnection implements net.Conn.
func TestSecretConnectionNetConnInterface(t *testing.T) {
	var conn net.Conn = &SecretConnection{}
	_ = conn // Verify it compiles as net.Conn
}

// TestSecretConnectionDeadlines tests deadline methods.
func TestSecretConnectionDeadlines(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	_, serverPriv := generateTestKeyPair(t)
	_, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		var err error
		serverSC, err = MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			t.Errorf("server handshake failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		clientSC, err = MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			t.Errorf("client handshake failed: %v", err)
		}
	}()

	wg.Wait()

	if serverSC == nil || clientSC == nil {
		t.Fatal("handshake failed")
	}

	// Test SetDeadline
	future := time.Now().Add(5 * time.Second)
	if err := serverSC.SetDeadline(future); err != nil {
		t.Errorf("SetDeadline failed: %v", err)
	}

	// Test SetReadDeadline
	if err := serverSC.SetReadDeadline(future); err != nil {
		t.Errorf("SetReadDeadline failed: %v", err)
	}

	// Test SetWriteDeadline
	if err := serverSC.SetWriteDeadline(future); err != nil {
		t.Errorf("SetWriteDeadline failed: %v", err)
	}

	// Test past deadline (should cause timeout on next operation)
	past := time.Now().Add(-1 * time.Second)
	if err := serverSC.SetReadDeadline(past); err != nil {
		t.Errorf("SetReadDeadline with past time failed: %v", err)
	}

	serverSC.Close()
	clientSC.Close()
}

// TestSecretConnectionLocalRemoteAddr tests address methods.
func TestSecretConnectionLocalRemoteAddr(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	_, serverPriv := generateTestKeyPair(t)
	_, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		var err error
		serverSC, err = MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			t.Errorf("server handshake failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		clientSC, err = MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			t.Errorf("client handshake failed: %v", err)
		}
	}()

	wg.Wait()

	if serverSC == nil || clientSC == nil {
		t.Fatal("handshake failed")
	}

	// net.Pipe() returns pipe addresses, verify they're not nil
	localAddr := clientSC.LocalAddr()
	remoteAddr := clientSC.RemoteAddr()

	if localAddr == nil {
		t.Error("LocalAddr should not be nil for pipe connection")
	}
	if remoteAddr == nil {
		t.Error("RemoteAddr should not be nil for pipe connection")
	}

	serverSC.Close()
	clientSC.Close()
}

// TestSecretConnectionExactFrameSize tests writing exactly 1024 bytes (one frame).
func TestSecretConnectionExactFrameSize(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	_, serverPriv := generateTestKeyPair(t)
	_, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		var err error
		serverSC, err = MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			t.Errorf("server handshake failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		clientSC, err = MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			t.Errorf("client handshake failed: %v", err)
		}
	}()

	wg.Wait()

	if serverSC == nil || clientSC == nil {
		t.Fatal("handshake failed")
	}

	// Test exactly one frame size
	testData := make([]byte, SecretMaxPlaintext)
	rand.Read(testData)

	var writeDone sync.WaitGroup
	writeDone.Add(1)

	go func() {
		defer writeDone.Done()
		n, err := clientSC.Write(testData)
		if err != nil {
			t.Errorf("write failed: %v", err)
		}
		if n != SecretMaxPlaintext {
			t.Errorf("wrote %d bytes, want %d", n, SecretMaxPlaintext)
		}
	}()

	readBuf := make([]byte, SecretMaxPlaintext+SecretMaxPlaintext)
	totalRead := 0
	for totalRead < SecretMaxPlaintext {
		n, err := serverSC.Read(readBuf[totalRead:])
		if err != nil && err != io.EOF {
			t.Fatalf("read failed: %v", err)
		}
		totalRead += n
		if n == 0 {
			break
		}
	}

	writeDone.Wait()

	if totalRead != SecretMaxPlaintext {
		t.Fatalf("read %d bytes, want %d", totalRead, SecretMaxPlaintext)
	}
	if !bytes.Equal(readBuf[:totalRead], testData) {
		t.Fatal("data mismatch for exact frame size")
	}

	serverSC.Close()
	clientSC.Close()
}

// TestSecretConnectionOneBytePerFrame tests single byte messages.
func TestSecretConnectionOneBytePerFrame(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	_, serverPriv := generateTestKeyPair(t)
	_, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		var err error
		serverSC, err = MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			t.Errorf("server handshake failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		clientSC, err = MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			t.Errorf("client handshake failed: %v", err)
		}
	}()

	wg.Wait()

	if serverSC == nil || clientSC == nil {
		t.Fatal("handshake failed")
	}

	testByte := []byte{0xAB}

	var writeDone sync.WaitGroup
	writeDone.Add(1)

	go func() {
		defer writeDone.Done()
		n, err := clientSC.Write(testByte)
		if err != nil {
			t.Errorf("write failed: %v", err)
		}
		if n != 1 {
			t.Errorf("wrote %d bytes, want 1", n)
		}
	}()

	readBuf := make([]byte, 10)
	n, err := serverSC.Read(readBuf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("read %d bytes, want 1", n)
	}
	if readBuf[0] != 0xAB {
		t.Fatalf("read byte 0x%02x, want 0xAB", readBuf[0])
	}

	writeDone.Wait()

	serverSC.Close()
	clientSC.Close()
}

// TestSecretConnectionBidirectional tests bidirectional communication.
func TestSecretConnectionBidirectional(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	_, serverPriv := generateTestKeyPair(t)
	_, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		var err error
		serverSC, err = MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			t.Errorf("server handshake failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		clientSC, err = MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			t.Errorf("client handshake failed: %v", err)
		}
	}()

	wg.Wait()

	if serverSC == nil || clientSC == nil {
		t.Fatal("handshake failed")
	}

	serverMsg := []byte("Server to Client")
	clientMsg := []byte("Client to Server")

	var serverWriteDone, clientWriteDone sync.WaitGroup
	serverWriteDone.Add(1)
	clientWriteDone.Add(1)

	go func() {
		defer serverWriteDone.Done()
		_, err := serverSC.Write(serverMsg)
		if err != nil {
			t.Errorf("server write failed: %v", err)
		}
	}()

	go func() {
		defer clientWriteDone.Done()
		_, err := clientSC.Write(clientMsg)
		if err != nil {
			t.Errorf("client write failed: %v", err)
		}
	}()

	// Client reads from server
	clientReadBuf := make([]byte, len(serverMsg)+SecretMaxPlaintext)
	clientTotalRead := 0
	for clientTotalRead < len(serverMsg) {
		n, err := clientSC.Read(clientReadBuf[clientTotalRead:])
		if err != nil && err != io.EOF {
			t.Fatalf("client read failed: %v", err)
		}
		clientTotalRead += n
		if n == 0 {
			break
		}
	}

	// Server reads from client
	serverReadBuf := make([]byte, len(clientMsg)+SecretMaxPlaintext)
	serverTotalRead := 0
	for serverTotalRead < len(clientMsg) {
		n, err := serverSC.Read(serverReadBuf[serverTotalRead:])
		if err != nil && err != io.EOF {
			t.Fatalf("server read failed: %v", err)
		}
		serverTotalRead += n
		if n == 0 {
			break
		}
	}

	serverWriteDone.Wait()
	clientWriteDone.Wait()

	if clientTotalRead != len(serverMsg) {
		t.Fatalf("client read %d bytes, want %d", clientTotalRead, len(serverMsg))
	}
	if !bytes.Equal(clientReadBuf[:clientTotalRead], serverMsg) {
		t.Fatal("client received wrong message from server")
	}

	if serverTotalRead != len(clientMsg) {
		t.Fatalf("server read %d bytes, want %d", serverTotalRead, len(clientMsg))
	}
	if !bytes.Equal(serverReadBuf[:serverTotalRead], clientMsg) {
		t.Fatal("server received wrong message from client")
	}

	serverSC.Close()
	clientSC.Close()
}

// TestSecretConnectionEmptyWrite tests writing empty data.
func TestSecretConnectionEmptyWrite(t *testing.T) {
	serverConn, clientConn := pipeConn(t)

	_, serverPriv := generateTestKeyPair(t)
	_, clientPriv := generateTestKeyPair(t)

	var wg sync.WaitGroup
	wg.Add(2)

	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		var err error
		serverSC, err = MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			t.Errorf("server handshake failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		clientSC, err = MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			t.Errorf("client handshake failed: %v", err)
		}
	}()

	wg.Wait()

	if serverSC == nil || clientSC == nil {
		t.Fatal("handshake failed")
	}

	// Empty write should succeed
	n, err := clientSC.Write([]byte{})
	if err != nil {
		t.Errorf("empty write failed: %v", err)
	}
	if n != 0 {
		t.Errorf("empty write returned %d, want 0", n)
	}

	serverSC.Close()
	clientSC.Close()
}

// BenchmarkSecretConnectionHandshake benchmarks the handshake protocol.
func BenchmarkSecretConnectionHandshake(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		serverConn, clientConn := net.Pipe()

		_, serverPriv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			b.Fatalf("generate server key: %v", err)
		}
		_, clientPriv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			b.Fatalf("generate client key: %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(2)

		var serverSC, clientSC *SecretConnection
		var serverErr, clientErr error

		go func() {
			defer wg.Done()
			serverSC, serverErr = MakeSecretConnection(serverConn, serverPriv, false)
		}()

		go func() {
			defer wg.Done()
			clientSC, clientErr = MakeSecretConnection(clientConn, clientPriv, true)
		}()

		wg.Wait()

		if serverErr != nil {
			b.Errorf("server handshake failed: %v", serverErr)
		}
		if clientErr != nil {
			b.Errorf("client handshake failed: %v", clientErr)
		}

		if serverSC != nil {
			serverSC.Close()
		}
		if clientSC != nil {
			clientSC.Close()
		}
	}
}

// BenchmarkSecretConnectionWrite benchmarks encrypted write operations.
func BenchmarkSecretConnectionWrite(b *testing.B) {
	serverConn, clientConn := net.Pipe()

	_, serverPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatalf("generate server key: %v", err)
	}
	_, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatalf("generate client key: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	var serverErr, clientErr error
	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		sc, err := MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			serverErr = err
			return
		}
		serverSC = sc
	}()

	go func() {
		defer wg.Done()
		sc, err := MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			clientErr = err
			return
		}
		clientSC = sc
	}()

	wg.Wait()

	if serverErr != nil {
		b.Fatalf("server handshake failed: %v", serverErr)
	}
	if clientErr != nil {
		b.Fatalf("client handshake failed: %v", clientErr)
	}

	testData := make([]byte, 1024)
	rand.Read(testData)

	// Start a reader goroutine to consume written data
	var readDone sync.WaitGroup
	readDone.Add(1)
	go func() {
		defer readDone.Done()
		readBuf := make([]byte, 2048)
		for i := 0; i < b.N; i++ {
			totalRead := 0
			for totalRead < len(testData) {
				n, err := serverSC.Read(readBuf[totalRead:])
				if err != nil {
					return
				}
				totalRead += n
			}
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := clientSC.Write(testData)
		if err != nil {
			b.Fatalf("write failed: %v", err)
		}
	}

	b.StopTimer()
	readDone.Wait()
	serverSC.Close()
	clientSC.Close()
}

// BenchmarkSecretConnectionRead benchmarks encrypted read operations.
func BenchmarkSecretConnectionRead(b *testing.B) {
	serverConn, clientConn := net.Pipe()

	_, serverPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatalf("generate server key: %v", err)
	}
	_, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatalf("generate client key: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	var srvErr, cliErr error
	var serverSC, clientSC *SecretConnection

	go func() {
		defer wg.Done()
		sc, err := MakeSecretConnection(serverConn, serverPriv, false)
		if err != nil {
			srvErr = err
			return
		}
		serverSC = sc
	}()

	go func() {
		defer wg.Done()
		sc, err := MakeSecretConnection(clientConn, clientPriv, true)
		if err != nil {
			cliErr = err
			return
		}
		clientSC = sc
	}()

	wg.Wait()

	if srvErr != nil {
		b.Fatalf("server handshake failed: %v", srvErr)
	}
	if cliErr != nil {
		b.Fatalf("client handshake failed: %v", cliErr)
	}

	testData := make([]byte, 1024)
	rand.Read(testData)

	// Start a writer goroutine to produce data
	var writeDone sync.WaitGroup
	writeDone.Add(1)
	go func() {
		defer writeDone.Done()
		for i := 0; i < b.N; i++ {
			_, err := clientSC.Write(testData)
			if err != nil {
				return
			}
		}
	}()

	readBuf := make([]byte, 2048)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		totalRead := 0
		for totalRead < len(testData) {
			n, err := serverSC.Read(readBuf[totalRead:])
			if err != nil {
				b.Fatalf("read failed: %v", err)
			}
			totalRead += n
		}
	}

	b.StopTimer()
	writeDone.Wait()
	serverSC.Close()
	clientSC.Close()
}
