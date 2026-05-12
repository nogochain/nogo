package mconnection

import (
	"net"
	"testing"
	"time"
)

// TestPongTimeout_NormalPong verifies that normal ping/pong exchange
// keeps the connection alive. Two connections exchange heartbeats;
// neither should trigger a pong timeout.
func TestPongTimeout_NormalPong(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	errorCh := make(chan error, 10)

	onReceive := func(chID byte, msg []byte) {}
	onError := func(err error) {
		select {
		case errorCh <- err:
		default:
		}
	}

	config := DefaultMConnConfig()
	config.PingTimeout = 50 * time.Millisecond
	config.PongTimeout = 500 * time.Millisecond

	chDescs := []*ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   10,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	mconn1, err := NewMConnection(server, chDescs, onReceive, onError, config)
	if err != nil {
		t.Fatalf("create mconn1: %v", err)
	}
	mconn2, err := NewMConnection(client, chDescs, func(byte, []byte) {}, func(error) {}, config)
	if err != nil {
		t.Fatalf("create mconn2: %v", err)
	}

	if err := mconn1.Start(); err != nil {
		t.Fatalf("start mconn1: %v", err)
	}
	defer mconn1.Stop()

	if err := mconn2.Start(); err != nil {
		t.Fatalf("start mconn2: %v", err)
	}
	defer mconn2.Stop()

	time.Sleep(300 * time.Millisecond)

	// If either connection stopped, check the error.
	if !mconn1.IsRunning() {
		select {
		case err := <-errorCh:
			t.Fatalf("mconn1 stopped unexpectedly: %v", err)
		default:
			t.Fatal("mconn1 stopped unexpectedly without error")
		}
	}
	if !mconn2.IsRunning() {
		t.Fatal("mconn2 stopped unexpectedly")
	}
}

// TestPongTimeout_MissingPong verifies that a connection is terminated
// when pong responses cease for longer than PongTimeout.
func TestPongTimeout_MissingPong(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	errorCh := make(chan error, 10)

	onReceive := func(chID byte, msg []byte) {}
	onError := func(err error) {
		select {
		case errorCh <- err:
		default:
		}
	}

	config := DefaultMConnConfig()
	config.PingTimeout = 30 * time.Millisecond
	config.PongTimeout = 100 * time.Millisecond

	chDescs := []*ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   10,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	mconn, err := NewMConnection(server, chDescs, onReceive, onError, config)
	if err != nil {
		t.Fatalf("create mconnection: %v", err)
	}

	if err := mconn.Start(); err != nil {
		t.Fatalf("start mconnection: %v", err)
	}
	defer mconn.Stop()

	// Drain pings on the raw client side so the pipe buffer does not block.
	done := make(chan struct{})
	defer close(done)
	go func() {
		buf := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
				_, err := client.Read(buf)
				if err != nil {
					return
				}
			}
		}
	}()

	// Send a single pong so lastRecvPongTime becomes non-zero.
	_, err = client.Write([]byte{packetTypePong})
	if err != nil {
		t.Fatalf("write initial pong: %v", err)
	}

	// Wait for the pong timeout (PongTimeout + some margin).
	// The pongMonitorRoutine checks every PingTimeout (30ms) and should detect
	// that the last pong was received more than PongTimeout (100ms) ago.
	maxWait := config.PingTimeout + config.PongTimeout + 300*time.Millisecond
	select {
	case err := <-errorCh:
		if err == nil {
			t.Fatal("expected non-nil pong timeout error")
		}
	case <-time.After(maxWait):
		// Check before giving up.
		if mconn.IsRunning() {
			t.Fatalf("connection still running after %v (should have timed out)", maxWait)
		}
	}

	if mconn.IsRunning() {
		t.Error("connection should not be running after pong timeout")
	}
}

// TestPongTimeout_InitialPongNotRequired verifies that the pong monitor
// does not trigger before any pong has been received. This prevents
// premature disconnection during initial handshake.
func TestPongTimeout_InitialPongNotRequired(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	errorCh := make(chan error, 10)

	onReceive := func(chID byte, msg []byte) {}
	onError := func(err error) {
		select {
		case errorCh <- err:
		default:
		}
	}

	config := DefaultMConnConfig()
	config.PingTimeout = 10 * time.Millisecond
	config.PongTimeout = 30 * time.Millisecond

	chDescs := []*ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   10,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	mconn, err := NewMConnection(server, chDescs, onReceive, onError, config)
	if err != nil {
		t.Fatalf("create mconnection: %v", err)
	}

	if err := mconn.Start(); err != nil {
		t.Fatalf("start mconnection: %v", err)
	}
	defer mconn.Stop()

	// Drain incoming pings on the raw client side to prevent pipe buffer from
	// blocking mconn's send routine.
	done := make(chan struct{})
	defer close(done)
	go func() {
		buf := make([]byte, 1024)
		for {
			select {
			case <-done:
				return
			default:
				_, err := client.Read(buf)
				if err != nil {
					return
				}
			}
		}
	}()

	time.Sleep(150 * time.Millisecond)

	select {
	case err := <-errorCh:
		t.Fatalf("unexpected error before any pong received: %v", err)
	default:
	}

	if !mconn.IsRunning() {
		t.Fatal("connection should still be running with no pong timeout triggered")
	}
}

// TestPongTimeout_LastRecvTime verifies that LastRecvTime() returns a non-zero
// time after a message has been received.
func TestPongTimeout_LastRecvTime(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	receivedCh := make(chan []byte, 10)

	onReceive := func(chID byte, msg []byte) {
		receivedCh <- msg
	}
	onError := func(err error) {}

	config := DefaultMConnConfig()
	config.PingTimeout = 1 * time.Second

	chDescs := []*ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   10,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	mconn, err := NewMConnection(server, chDescs, onReceive, onError, config)
	if err != nil {
		t.Fatalf("create mconnection: %v", err)
	}

	if err := mconn.Start(); err != nil {
		t.Fatalf("start mconnection: %v", err)
	}
	defer mconn.Stop()

	// Before any message, LastRecvTime should be zero.
	if !mconn.LastRecvTime().IsZero() {
		t.Error("LastRecvTime should be zero before any message received")
	}

	// Build and send a valid message packet.
	payload := make([]byte, msgPacketHeaderSize+5)
	payload[0] = 0x01                                          // channelID
	payload[1] = 0                                              // length high
	payload[2] = 5                                              // length low
	payload[3] = 0x01                                           // EOF flag
	copy(payload[4:], []byte("hello"))                          // data

	fullPkt := append([]byte{packetTypeMsg}, payload...)
	_, err = client.Write(fullPkt)
	if err != nil {
		t.Fatalf("write message: %v", err)
	}

	select {
	case <-receivedCh:
		if mconn.LastRecvTime().IsZero() {
			t.Error("LastRecvTime should be non-zero after message received")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}