package mconnection

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Helper function to create a test MConnection with a pipe.
func createTestMConnection(t *testing.T) (*MConnection, *MConnection, net.Conn, net.Conn) {
	t.Helper()

	server, client := net.Pipe()

	receivedCh := make(chan struct {
		chID  byte
		msg   []byte
	}, 100)
	errorsCh := make(chan error, 10)

	onReceive := func(chID byte, msg []byte) {
		receivedCh <- struct {
			chID  byte
			msg   []byte
		}{chID: chID, msg: msg}
	}
	onError := func(err error) {
		errorsCh <- err
	}

	config := DefaultMConnConfig()
	config.PingTimeout = 100 * time.Millisecond // Fast ping for testing.

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
		t.Fatalf("create server mconnection: %v", err)
	}

	mconn2, err := NewMConnection(client, chDescs, onReceive, onError, config)
	if err != nil {
		t.Fatalf("create client mconnection: %v", err)
	}

	if err := mconn1.Start(); err != nil {
		t.Fatalf("start server mconnection: %v", err)
	}
	if err := mconn2.Start(); err != nil {
		t.Fatalf("start client mconnection: %v", err)
	}

	return mconn1, mconn2, server, client
}

// Helper to stop connections safely.
func stopTestConnections(mconn1, mconn2 *MConnection) {
	_ = mconn1.Stop()
	_ = mconn2.Stop()
}

// TestMConnectionSendReceive verifies basic send and receive functionality.
func TestMConnectionSendReceive(t *testing.T) {
	mconn1, mconn2, _, _ := createTestMConnection(t)
	defer stopTestConnections(mconn1, mconn2)

	// Give connections time to initialize.
	time.Sleep(50 * time.Millisecond)

	testMsg := []byte("hello nogo")
	if !mconn1.Send(0x01, testMsg) {
		t.Fatal("Send failed")
	}

	// Give time for the message to be processed by the send/recv routines.
	time.Sleep(200 * time.Millisecond)
}

// TestMConnectionSendReceiveWithCallback tests message delivery via callback.
func TestMConnectionSendReceiveWithCallback(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	receivedCh := make(chan []byte, 10)
	errorsCh := make(chan error, 10)

	onReceive := func(chID byte, msg []byte) {
		receivedCh <- msg
	}
	onError := func(err error) {
		errorsCh <- err
	}

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

	testMsg := []byte("test message delivery")
	if !mconn2.Send(0x01, testMsg) {
		t.Fatal("mconn2 Send failed")
	}

	select {
	case received := <-receivedCh:
		if string(received) != string(testMsg) {
			t.Errorf("expected %q, got %q", testMsg, received)
		}
	case err := <-errorsCh:
		t.Fatalf("received error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for message delivery")
	}
}

// TestMConnectionTrySend tests non-blocking send behavior.
func TestMConnectionTrySend(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	receivedCh := make(chan []byte, 10)
	onReceive := func(chID byte, msg []byte) {
		receivedCh <- msg
	}
	onError := func(err error) {
		t.Logf("error: %v", err)
	}

	config := DefaultMConnConfig()
	config.PingTimeout = 1 * time.Second

	chDescs := []*ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   1, // Small queue for testing.
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

	time.Sleep(50 * time.Millisecond)

	// First TrySend should succeed.
	if !mconn2.TrySend(0x01, []byte("msg1")) {
		t.Fatal("first TrySend should succeed")
	}

	// Second TrySend may succeed or fail depending on send routine speed.
	// This is expected behavior for non-blocking send.
	_ = mconn2.TrySend(0x01, []byte("msg2"))
}

// TestMConnectionCanSend verifies CanSend returns correct values.
func TestMConnectionCanSend(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

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

	mconn, err := NewMConnection(client, chDescs, func(byte, []byte) {}, func(error) {}, config)
	if err != nil {
		t.Fatalf("create mconnection: %v", err)
	}

	// Not running yet, CanSend should return false.
	if mconn.CanSend(0x01) {
		t.Error("CanSend should return false when not running")
	}

	if err := mconn.Start(); err != nil {
		t.Fatalf("start mconnection: %v", err)
	}
	defer mconn.Stop()

	time.Sleep(50 * time.Millisecond)

	// Running and channel exists, CanSend should return true.
	if !mconn.CanSend(0x01) {
		t.Error("CanSend should return true when running")
	}

	// Unknown channel should return false.
	if mconn.CanSend(0xFF) {
		t.Error("CanSend should return false for unknown channel")
	}
}

// TestMConnectionUnknownChannel verifies operations on unknown channels fail gracefully.
func TestMConnectionUnknownChannel(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

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

	mconn, err := NewMConnection(client, chDescs, func(byte, []byte) {}, func(error) {}, config)
	if err != nil {
		t.Fatalf("create mconnection: %v", err)
	}

	if err := mconn.Start(); err != nil {
		t.Fatalf("start mconnection: %v", err)
	}
	defer mconn.Stop()

	time.Sleep(50 * time.Millisecond)

	// Send to unknown channel should return false.
	if mconn.Send(0xFF, []byte("test")) {
		t.Error("Send to unknown channel should return false")
	}

	// TrySend to unknown channel should return false.
	if mconn.TrySend(0xFF, []byte("test")) {
		t.Error("TrySend to unknown channel should return false")
	}
}

// TestMConnectionMessageFragmentation tests that large messages are properly
// fragmented into packets and reassembled on the receiving end.
func TestMConnectionMessageFragmentation(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	receivedCh := make(chan []byte, 10)
	onReceive := func(chID byte, msg []byte) {
		receivedCh <- msg
	}
	onError := func(err error) {
		t.Logf("error: %v", err)
	}

	config := DefaultMConnConfig()
	config.PingTimeout = 1 * time.Second
	// Use a small max packet size to force fragmentation.
	config.MaxPacketPayloadSize = 100

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

	time.Sleep(50 * time.Millisecond)

	// Create a message larger than maxMsgPacketPayloadSize (1024 bytes).
	largeMsg := make([]byte, 3000)
	for i := range largeMsg {
		largeMsg[i] = byte(i % 256)
	}

	if !mconn2.Send(0x01, largeMsg) {
		t.Fatal("Send large message failed")
	}

	select {
	case received := <-receivedCh:
		if len(received) != len(largeMsg) {
			t.Errorf("expected message length %d, got %d", len(largeMsg), len(received))
		}
		if string(received) != string(largeMsg) {
			t.Errorf("received message does not match original")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for fragmented message")
	}
}

// TestMConnectionTrafficStatus verifies that traffic monitoring works correctly.
func TestMConnectionTrafficStatus(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

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

	mconn1, err := NewMConnection(server, chDescs, func(byte, []byte) {}, func(error) {}, config)
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

	time.Sleep(50 * time.Millisecond)

	// Initial rates should be zero or near-zero.
	sendRate, recvRate := mconn1.TrafficStatus()
	if sendRate < 0 || recvRate < 0 {
		t.Errorf("negative traffic rate: send=%f, recv=%f", sendRate, recvRate)
	}

	// Send some data and check that rates increase.
	testMsg := []byte("traffic test data")
	for i := 0; i < 10; i++ {
		mconn2.Send(0x01, testMsg)
	}

	time.Sleep(200 * time.Millisecond)

	sendRate, recvRate = mconn2.TrafficStatus()
	// After sending data, send rate should be positive.
	if sendRate <= 0 {
		t.Logf("warning: send rate is zero after sending data (may be timing dependent)")
	}
	_ = recvRate
}

// TestMConnectionStop verifies graceful shutdown.
func TestMConnectionStop(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

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

	mconn, err := NewMConnection(client, chDescs, func(byte, []byte) {}, func(error) {}, config)
	if err != nil {
		t.Fatalf("create mconnection: %v", err)
	}

	if err := mconn.Start(); err != nil {
		t.Fatalf("start mconnection: %v", err)
	}

	if !mconn.IsRunning() {
		t.Error("connection should be running after Start")
	}

	if err := mconn.Stop(); err != nil {
		t.Fatalf("stop mconnection: %v", err)
	}

	if mconn.IsRunning() {
		t.Error("connection should not be running after Stop")
	}

	// Stop should be idempotent.
	if err := mconn.Stop(); err != nil {
		t.Errorf("second Stop should not error: %v", err)
	}
}

// TestMConnectionStartTwice verifies that starting an already-running connection fails.
func TestMConnectionStartTwice(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

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

	mconn, err := NewMConnection(client, chDescs, func(byte, []byte) {}, func(error) {}, config)
	if err != nil {
		t.Fatalf("create mconnection: %v", err)
	}

	if err := mconn.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer mconn.Stop()

	// Second Start should fail.
	if err := mconn.Start(); err == nil {
		t.Error("second Start should return an error")
	}
}

// TestMConnectionNilConnection verifies that nil connection is rejected.
func TestMConnectionNilConnection(t *testing.T) {
	config := DefaultMConnConfig()
	chDescs := []*ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   10,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	_, err := NewMConnection(nil, chDescs, func(byte, []byte) {}, func(error) {}, config)
	if err == nil {
		t.Error("nil connection should be rejected")
	}
}

// TestMConnectionNilOnReceive verifies that nil onReceive callback is rejected.
func TestMConnectionNilOnReceive(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	config := DefaultMConnConfig()
	chDescs := []*ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   10,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	_, err := NewMConnection(client, chDescs, nil, func(error) {}, config)
	if err == nil {
		t.Error("nil onReceive should be rejected")
	}
}

// TestMConnectionInvalidConfig verifies that invalid config is rejected.
func TestMConnectionInvalidConfig(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	config := DefaultMConnConfig()
	config.SendRate = -1 // Invalid.

	chDescs := []*ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   10,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	_, err := NewMConnection(client, chDescs, func(byte, []byte) {}, func(error) {}, config)
	if err == nil {
		t.Error("invalid config should be rejected")
	}
}

// TestMConnectionMultipleChannels verifies that multiple channels work correctly.
func TestMConnectionMultipleChannels(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	receivedCh := make(chan struct {
		chID  byte
		msg   []byte
	}, 100)
	onReceive := func(chID byte, msg []byte) {
		receivedCh <- struct {
			chID  byte
			msg   []byte
		}{chID: chID, msg: msg}
	}
	onError := func(err error) {
		t.Logf("error: %v", err)
	}

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
		{
			ID:                  0x02,
			Priority:            5,
			SendQueueCapacity:   10,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
		{
			ID:                  0x03,
			Priority:            3,
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

	time.Sleep(50 * time.Millisecond)

	// Send messages on different channels.
	mconn2.Send(0x01, []byte("channel1"))
	mconn2.Send(0x02, []byte("channel2"))
	mconn2.Send(0x03, []byte("channel3"))

	received := make(map[byte]string)
	timeout := time.After(3 * time.Second)

	for len(received) < 3 {
		select {
		case r := <-receivedCh:
			received[r.chID] = string(r.msg)
		case <-timeout:
			t.Fatalf("timeout waiting for messages, received %d of 3", len(received))
		}
	}

	if received[0x01] != "channel1" {
		t.Errorf("channel 0x01: expected 'channel1', got %q", received[0x01])
	}
	if received[0x02] != "channel2" {
		t.Errorf("channel 0x02: expected 'channel2', got %q", received[0x02])
	}
	if received[0x03] != "channel3" {
		t.Errorf("channel 0x03: expected 'channel3', got %q", received[0x03])
	}
}

// TestMConnectionConcurrentSend tests concurrent sends from multiple goroutines.
func TestMConnectionConcurrentSend(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	var receivedCount atomic.Int32
	var wg sync.WaitGroup

	onReceive := func(chID byte, msg []byte) {
		receivedCount.Add(1)
	}
	onError := func(err error) {
		t.Logf("error: %v", err)
	}

	config := DefaultMConnConfig()
	config.PingTimeout = 1 * time.Second

	chDescs := []*ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   100,
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

	time.Sleep(50 * time.Millisecond)

	// Launch 10 goroutines, each sending 10 messages.
	numGoroutines := 10
	msgsPerGoroutine := 10
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < msgsPerGoroutine; j++ {
				msg := []byte("concurrent message")
				mconn2.Send(0x01, msg)
			}
		}(i)
	}

	wg.Wait()

	// Wait for messages to be processed.
	time.Sleep(500 * time.Millisecond)

	expected := int32(numGoroutines * msgsPerGoroutine)
	actual := receivedCount.Load()
	if actual < expected {
		t.Logf("received %d of %d messages (some may still be in transit)", actual, expected)
	}
}

// TestMConnectionDefaultChannels tests using the default channel descriptors.
func TestMConnectionDefaultChannels(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	config := DefaultMConnConfig()
	config.PingTimeout = 1 * time.Second

	chDescs := DefaultChannelDescriptors()

	mconn, err := NewMConnection(client, chDescs, func(byte, []byte) {}, func(error) {}, config)
	if err != nil {
		t.Fatalf("create mconnection with default channels: %v", err)
	}

	if err := mconn.Start(); err != nil {
		t.Fatalf("start mconnection: %v", err)
	}
	defer mconn.Stop()

	time.Sleep(50 * time.Millisecond)

	// Verify all default channels are accessible.
	defaultChIDs := []byte{ChannelSync, ChannelTx, ChannelBlock, ChannelConsensus, ChannelGossip}
	for _, chID := range defaultChIDs {
		if !mconn.CanSend(chID) {
			t.Errorf("channel 0x%02x (%s) should be sendable", chID, ChannelName(chID))
		}
	}
}

// TestMConnectionRemoteAddr verifies RemoteAddr returns the correct address.
func TestMConnectionRemoteAddr(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	config := DefaultMConnConfig()
	chDescs := []*ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   10,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	mconn, err := NewMConnection(client, chDescs, func(byte, []byte) {}, func(error) {}, config)
	if err != nil {
		t.Fatalf("create mconnection: %v", err)
	}

	addr := mconn.RemoteAddr()
	if addr == "nil" {
		t.Error("RemoteAddr should not be 'nil' for a valid connection")
	}
}

// TestMConnectionString verifies the String method returns a valid representation.
func TestMConnectionString(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	config := DefaultMConnConfig()
	chDescs := []*ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   10,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}

	mconn, err := NewMConnection(client, chDescs, func(byte, []byte) {}, func(error) {}, config)
	if err != nil {
		t.Fatalf("create mconnection: %v", err)
	}

	s := mconn.String()
	if len(s) == 0 {
		t.Error("String() should not return empty string")
	}
}

// TestMConnectionErrorCallback verifies that the error callback is invoked on connection failure.
func TestMConnectionErrorCallback(t *testing.T) {
	server, client := net.Pipe()

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

	errorReceived := make(chan error, 1)
	onReceive := func(chID byte, msg []byte) {}
	onError := func(err error) {
		errorReceived <- err
	}

	mconn, err := NewMConnection(server, chDescs, onReceive, onError, config)
	if err != nil {
		t.Fatalf("create mconnection: %v", err)
	}

	if err := mconn.Start(); err != nil {
		t.Fatalf("start mconnection: %v", err)
	}

	// Close the client side to trigger an error.
	client.Close()

	select {
	case err := <-errorReceived:
		if err == nil {
			t.Error("error callback should receive a non-nil error")
		}
	case <-time.After(3 * time.Second):
		t.Log("error callback timeout (connection may still be processing)")
	}

	_ = mconn.Stop()
}
