package mconnection

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
)

// Helper to create a test channel descriptor.
func testChannelDesc() *ChannelDescriptor {
	return &ChannelDescriptor{
		ID:                  0x01,
		Priority:            1,
		SendQueueCapacity:   10,
		RecvBufferCapacity:  4096,
		RecvMessageCapacity: 22020096,
	}
}

// TestChannelNewChannel verifies channel creation with valid descriptor.
func TestChannelNewChannel(t *testing.T) {
	desc := testChannelDesc()
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}
	if ch == nil {
		t.Fatal("NewChannel returned nil")
	}
	if ch.desc != desc {
		t.Error("channel descriptor mismatch")
	}
	if ch.priority != desc.Priority {
		t.Errorf("expected priority %d, got %d", desc.Priority, ch.priority)
	}
}

// TestChannelNewChannelInvalidDescriptor verifies that invalid descriptors are rejected.
func TestChannelNewChannelInvalidDescriptor(t *testing.T) {
	invalidDescs := []*ChannelDescriptor{
		{ID: 0x00, Priority: 1, SendQueueCapacity: 10, RecvBufferCapacity: 100, RecvMessageCapacity: 100}, // ID zero.
		{ID: 0x01, Priority: 0, SendQueueCapacity: 10, RecvBufferCapacity: 100, RecvMessageCapacity: 100}, // Priority zero.
		{ID: 0x01, Priority: -1, SendQueueCapacity: 10, RecvBufferCapacity: 100, RecvMessageCapacity: 100}, // Negative priority.
		{ID: 0x01, Priority: 1, SendQueueCapacity: 0, RecvBufferCapacity: 100, RecvMessageCapacity: 100},  // Zero send capacity.
		{ID: 0x01, Priority: 1, SendQueueCapacity: 10, RecvBufferCapacity: 0, RecvMessageCapacity: 100},   // Zero recv buffer.
		{ID: 0x01, Priority: 1, SendQueueCapacity: 10, RecvBufferCapacity: 100, RecvMessageCapacity: 0},   // Zero recv message.
	}

	for i, desc := range invalidDescs {
		_, err := NewChannel(desc)
		if err == nil {
			t.Errorf("test %d: expected error for invalid descriptor", i)
		}
	}
}

// TestChannelCanSend verifies that canSend returns correct values.
func TestChannelCanSend(t *testing.T) {
	desc := testChannelDesc()
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	// Initially the queue is empty, so canSend should be true.
	if !ch.canSend() {
		t.Error("canSend should be true when queue is empty")
	}

	// Fill the queue.
	for i := 0; i < desc.SendQueueCapacity; i++ {
		ch.WriteMessage([]byte("fill"))
	}

	// Now the queue is full, canSend should be false.
	if ch.canSend() {
		t.Error("canSend should be false when queue is full")
	}
}

// TestChannelWriteAndTryWrite verifies both blocking and non-blocking write.
func TestChannelWriteAndTryWrite(t *testing.T) {
	desc := testChannelDesc()
	desc.SendQueueCapacity = 2
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	// WriteMessage should succeed (blocking).
	if !ch.WriteMessage([]byte("msg1")) {
		t.Error("WriteMessage failed")
	}

	// TryWriteMessage should succeed when queue has capacity.
	if !ch.TryWriteMessage([]byte("msg2")) {
		t.Error("TryWriteMessage should succeed when queue has capacity")
	}

	// TryWriteMessage should fail when queue is full.
	if ch.TryWriteMessage([]byte("msg3")) {
		t.Error("TryWriteMessage should fail when queue is full")
	}
}

// TestChannelSendQueueSizeAtomic verifies that send queue size is tracked atomically.
func TestChannelSendQueueSizeAtomic(t *testing.T) {
	desc := testChannelDesc()
	desc.SendQueueCapacity = 100
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	var wg sync.WaitGroup
	numWriters := 10
	msgsPerWriter := 5

	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < msgsPerWriter; j++ {
				ch.TryWriteMessage([]byte("concurrent"))
			}
		}()
	}

	wg.Wait()

	expected := numWriters * msgsPerWriter
	actual := ch.loadSendQueueSize()
	if actual != expected {
		t.Errorf("expected send queue size %d, got %d", expected, actual)
	}
}

// TestChannelNextMsgPacket verifies message fragmentation into packets.
func TestChannelNextMsgPacket(t *testing.T) {
	desc := testChannelDesc()
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	// Queue a small message (fits in one packet).
	ch.WriteMessage([]byte("small"))
	if !ch.isSendPending() {
		t.Fatal("isSendPending should be true")
	}

	packet := ch.nextMsgPacket()
	if !packet.EOF {
		t.Error("small message should have EOF=true")
	}
	if string(packet.Data) != "small" {
		t.Errorf("expected 'small', got %q", packet.Data)
	}

	// Queue a large message (requires fragmentation).
	largeMsg := make([]byte, 3000)
	for i := range largeMsg {
		largeMsg[i] = byte(i % 256)
	}
	ch.WriteMessage(largeMsg)

	// First fragment should not have EOF.
	if !ch.isSendPending() {
		t.Fatal("isSendPending should be true for large message")
	}
	packet1 := ch.nextMsgPacket()
	if packet1.EOF {
		t.Error("first fragment of large message should have EOF=false")
	}
	if len(packet1.Data) != maxMsgPacketPayloadSize {
		t.Errorf("first fragment should be %d bytes, got %d", maxMsgPacketPayloadSize, len(packet1.Data))
	}

	// Second fragment should not have EOF (3000 - 1024 = 1976 remaining).
	if !ch.isSendPending() {
		t.Fatal("isSendPending should still be true")
	}
	packet2 := ch.nextMsgPacket()
	if packet2.EOF {
		t.Error("second fragment should have EOF=false")
	}
	if len(packet2.Data) != maxMsgPacketPayloadSize {
		t.Errorf("second fragment should be %d bytes, got %d", maxMsgPacketPayloadSize, len(packet2.Data))
	}

	// Third fragment should have EOF (3000 - 2048 = 952 remaining).
	if !ch.isSendPending() {
		t.Fatal("isSendPending should still be true")
	}
	packet3 := ch.nextMsgPacket()
	if !packet3.EOF {
		t.Error("third fragment should have EOF=true")
	}
	expectedRemaining := 3000 - 2*maxMsgPacketPayloadSize
	if len(packet3.Data) != expectedRemaining {
		t.Errorf("third fragment should be %d bytes, got %d", expectedRemaining, len(packet3.Data))
	}

	// No more pending.
	if ch.isSendPending() {
		t.Error("isSendPending should be false after all fragments sent")
	}
}

// TestChannelRecvMsgPacket verifies message reassembly from fragments.
func TestChannelRecvMsgPacket(t *testing.T) {
	desc := testChannelDesc()
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	// Single packet message.
	packet1 := msgPacket{
		ChannelID: 0x01,
		Data:      []byte("complete"),
		EOF:       true,
	}
	msgBytes, err := ch.recvMsgPacket(packet1)
	if err != nil {
		t.Fatalf("recvMsgPacket failed: %v", err)
	}
	if string(msgBytes) != "complete" {
		t.Errorf("expected 'complete', got %q", msgBytes)
	}

	// Fragmented message.
	original := make([]byte, 3000)
	for i := range original {
		original[i] = byte(i % 256)
	}

	frag1 := msgPacket{
		ChannelID: 0x01,
		Data:      original[:1024],
		EOF:       false,
	}
	frag2 := msgPacket{
		ChannelID: 0x01,
		Data:      original[1024:2048],
		EOF:       false,
	}
	frag3 := msgPacket{
		ChannelID: 0x01,
		Data:      original[2048:],
		EOF:       true,
	}

	// First two fragments should return nil.
	msg1, err := ch.recvMsgPacket(frag1)
	if err != nil {
		t.Fatalf("recvMsgPacket frag1 failed: %v", err)
	}
	if msg1 != nil {
		t.Error("first fragment should return nil")
	}

	msg2, err := ch.recvMsgPacket(frag2)
	if err != nil {
		t.Fatalf("recvMsgPacket frag2 failed: %v", err)
	}
	if msg2 != nil {
		t.Error("second fragment should return nil")
	}

	// Third fragment (EOF) should return the complete message.
	msg3, err := ch.recvMsgPacket(frag3)
	if err != nil {
		t.Fatalf("recvMsgPacket frag3 failed: %v", err)
	}
	if msg3 == nil {
		t.Fatal("third fragment (EOF) should return complete message")
	}
	if !bytes.Equal(msg3, original) {
		t.Error("reassembled message does not match original")
	}
}

// TestChannelRecvMsgPacketCapacityExceeded verifies that overflow is detected.
func TestChannelRecvMsgPacketCapacityExceeded(t *testing.T) {
	desc := testChannelDesc()
	desc.RecvMessageCapacity = 10 // Very small capacity for testing.
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	// First fragment within capacity.
	p1 := msgPacket{ChannelID: 0x01, Data: make([]byte, 5), EOF: false}
	_, err = ch.recvMsgPacket(p1)
	if err != nil {
		t.Fatalf("recvMsgPacket p1 failed: %v", err)
	}

	// Second fragment would exceed capacity.
	p2 := msgPacket{ChannelID: 0x01, Data: make([]byte, 6), EOF: false}
	_, err = ch.recvMsgPacket(p2)
	if err == nil {
		t.Error("expected capacity exceeded error")
	}
}

// TestChannelRecentlySent verifies that recentlySent tracking works correctly.
func TestChannelRecentlySent(t *testing.T) {
	desc := testChannelDesc()
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	// Initially zero.
	if ch.loadRecentlySent() != 0 {
		t.Error("recentlySent should be zero initially")
	}

	// Increment.
	ch.incrementRecentlySent(100)
	ch.incrementRecentlySent(200)
	if ch.loadRecentlySent() != 300 {
		t.Errorf("expected recentlySent=300, got %d", ch.loadRecentlySent())
	}

	// Decay (0.8 factor).
	ch.decayRecentlySent()
	expected := int64(float64(300) * 0.8) // 240
	if ch.loadRecentlySent() != expected {
		t.Errorf("expected recentlySent=%d after decay, got %d", expected, ch.loadRecentlySent())
	}
}

// TestChannelPriorityRatio verifies that priority ratio calculation is correct.
func TestChannelPriorityRatio(t *testing.T) {
	// High priority channel (priority 10).
	desc1 := testChannelDesc()
	desc1.Priority = 10
	ch1, err := NewChannel(desc1)
	if err != nil {
		t.Fatalf("NewChannel ch1 failed: %v", err)
	}

	// Low priority channel (priority 1).
	desc2 := testChannelDesc()
	desc2.Priority = 1
	ch2, err := NewChannel(desc2)
	if err != nil {
		t.Fatalf("NewChannel ch2 failed: %v", err)
	}

	// Both channels sent the same amount.
	ch1.incrementRecentlySent(100)
	ch2.incrementRecentlySent(100)

	// High priority channel should have lower ratio.
	ratio1 := ch1.getSendPriorityRatio()
	ratio2 := ch2.getSendPriorityRatio()

	// ratio1 = 100/10 = 10, ratio2 = 100/1 = 100.
	if ratio1 >= ratio2 {
		t.Errorf("high priority channel should have lower ratio: ch1=%f, ch2=%f", ratio1, ratio2)
	}
}

// TestChannelConcurrencySafety verifies concurrent operations are safe.
func TestChannelConcurrencySafety(t *testing.T) {
	desc := testChannelDesc()
	desc.SendQueueCapacity = 1000
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 20
	opsPerGoroutine := 100

	// Concurrent writes.
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				ch.TryWriteMessage([]byte("test"))
			}
		}()
	}

	// Concurrent reads of send queue size.
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				_ = ch.loadSendQueueSize()
			}
		}()
	}

	// Concurrent recentlySent operations.
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				ch.incrementRecentlySent(1)
				_ = ch.loadRecentlySent()
			}
		}()
	}

	// Concurrent decay operations.
	wg.Add(numGoroutines / 2)
	for i := 0; i < numGoroutines/2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				ch.decayRecentlySent()
			}
		}()
	}

	wg.Wait()
}

// TestChannelIsSendPending verifies isSendPending behavior.
func TestChannelIsSendPending(t *testing.T) {
	desc := testChannelDesc()
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	// No messages, should be false.
	if ch.isSendPending() {
		t.Error("isSendPending should be false with empty queue")
	}

	// Add a message, should be true.
	ch.WriteMessage([]byte("test"))
	if !ch.isSendPending() {
		t.Error("isSendPending should be true with message in queue")
	}

	// After consuming, should be false again.
	_ = ch.nextMsgPacket()
	if ch.isSendPending() {
		t.Error("isSendPending should be false after consuming the message")
	}
}

// TestChannelDecrementSendQueue verifies atomic decrement.
func TestChannelDecrementSendQueue(t *testing.T) {
	desc := testChannelDesc()
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	ch.WriteMessage([]byte("msg1"))
	ch.WriteMessage([]byte("msg2"))

	initial := ch.loadSendQueueSize()
	if initial != 2 {
		t.Errorf("expected send queue size 2, got %d", initial)
	}

	// Simulate what nextMsgPacket does when EOF.
	ch.decrementSendQueue()
	ch.decrementSendQueue()

	after := ch.loadSendQueueSize()
	if after != 0 {
		t.Errorf("expected send queue size 0 after decrements, got %d", after)
	}
}

// TestChannelIncrementSendQueue verifies atomic increment.
func TestChannelIncrementSendQueue(t *testing.T) {
	desc := testChannelDesc()
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	if ch.loadSendQueueSize() != 0 {
		t.Error("initial send queue size should be 0")
	}

	ch.incrementSendQueue()
	ch.incrementSendQueue()
	ch.incrementSendQueue()

	if ch.loadSendQueueSize() != 3 {
		t.Errorf("expected send queue size 3, got %d", ch.loadSendQueueSize())
	}
}

// TestChannelDecayRecentlySent verifies exponential decay.
func TestChannelDecayRecentlySent(t *testing.T) {
	desc := testChannelDesc()
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	ch.incrementRecentlySent(1000)

	// First decay: 1000 * 0.8 = 800.
	ch.decayRecentlySent()
	if ch.loadRecentlySent() != 800 {
		t.Errorf("expected 800 after first decay, got %d", ch.loadRecentlySent())
	}

	// Second decay: 800 * 0.8 = 640.
	ch.decayRecentlySent()
	if ch.loadRecentlySent() != 640 {
		t.Errorf("expected 640 after second decay, got %d", ch.loadRecentlySent())
	}

	// Third decay: 640 * 0.8 = 512.
	ch.decayRecentlySent()
	if ch.loadRecentlySent() != 512 {
		t.Errorf("expected 512 after third decay, got %d", ch.loadRecentlySent())
	}
}

// TestChannelSendPriorityRatioEdgeCases verifies edge cases in priority ratio.
func TestChannelSendPriorityRatioEdgeCases(t *testing.T) {
	// Test with zero recentlySent (ratio should be 0).
	desc := testChannelDesc()
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	ratio := ch.getSendPriorityRatio()
	if ratio != 0 {
		t.Errorf("expected ratio 0 with no bytes sent, got %f", ratio)
	}

	// Test with high bytes sent.
	ch.incrementRecentlySent(1000000)
	ratio = ch.getSendPriorityRatio()
	expected := float64(1000000) / float64(desc.Priority)
	if ratio != expected {
		t.Errorf("expected ratio %f, got %f", expected, ratio)
	}
}

// TestChannelConcurrentWriteAndRead verifies concurrent write and read safety.
func TestChannelConcurrentWriteAndRead(t *testing.T) {
	desc := testChannelDesc()
	desc.SendQueueCapacity = 100
	ch, err := NewChannel(desc)
	if err != nil {
		t.Fatalf("NewChannel failed: %v", err)
	}

	var writeDone atomic.Bool
	var readCount atomic.Int32
	var wg sync.WaitGroup

	// Writer goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			ch.TryWriteMessage([]byte("data"))
		}
		writeDone.Store(true)
	}()

	// Reader goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if ch.isSendPending() {
				_ = ch.nextMsgPacket()
				readCount.Add(1)
			}
			if writeDone.Load() && !ch.isSendPending() {
				break
			}
		}
	}()

	wg.Wait()

	t.Logf("read count: %d", readCount.Load())
}
