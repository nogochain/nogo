package mconnection

import (
	"fmt"
	"sync/atomic"
)

// Channel represents a multiplexed communication channel over a single connection.
// It manages send/receive queues and tracks traffic statistics.
// All public methods are concurrency-safe.
type Channel struct {
	// sendQueue holds messages waiting to be transmitted.
	sendQueue chan []byte

	// sendQueueSize tracks the number of pending messages atomically.
	sendQueueSize int32

	// sending holds the current message being fragmented for transmission.
	sending []byte

	// recving accumulates fragments until a complete message is assembled.
	recving []byte

	// desc is the channel configuration descriptor.
	desc *ChannelDescriptor

	// priority is the scheduling priority (copied from desc for convenience).
	priority int

	// recentlySent tracks exponential moving average of bytes sent for fair scheduling.
	recentlySent int64
}

// NewChannel creates a new Channel from the given descriptor.
// The send queue is initialized with the descriptor's capacity.
func NewChannel(desc *ChannelDescriptor) (*Channel, error) {
	if err := desc.Validate(); err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}

	return &Channel{
		sendQueue:    make(chan []byte, desc.SendQueueCapacity),
		sendQueueSize: 0,
		sending:      nil,
		recving:      make([]byte, 0, desc.RecvBufferCapacity),
		desc:         desc,
		priority:     desc.Priority,
		recentlySent: 0,
	}, nil
}

// isSendPending returns true if there is data waiting to be sent.
// If the current sending buffer is empty, it dequeues the next message.
// This method is goroutine-safe.
func (ch *Channel) isSendPending() bool {
	if len(ch.sending) == 0 {
		select {
		case msg := <-ch.sendQueue:
			ch.sending = msg
		default:
			return false
		}
	}
	return true
}

// nextMsgPacket creates the next fragment packet from the current sending buffer.
// Returns a msgPacket containing up to maxMsgPacketPayloadSize bytes.
// Sets EOF=true if this is the last fragment of the message.
// This method is NOT goroutine-safe and should only be called from the send routine.
func (ch *Channel) nextMsgPacket() msgPacket {
	payloadSize := len(ch.sending)
	if payloadSize > maxMsgPacketPayloadSize {
		payloadSize = maxMsgPacketPayloadSize
	}

	data := ch.sending[:payloadSize]
	eof := len(ch.sending) <= maxMsgPacketPayloadSize

	packet := msgPacket{
		ChannelID: ch.desc.ID,
		Data:      data,
		EOF:       eof,
	}

	if eof {
		ch.sending = nil
		ch.decrementSendQueue()
	} else {
		ch.sending = ch.sending[payloadSize:]
	}

	return packet
}

// recvMsgPacket processes an incoming packet fragment.
// Returns the complete message bytes if this fragment completes the message, nil otherwise.
// Returns an error if the reassembled message would exceed capacity.
// This method is NOT goroutine-safe and should only be called from the receive routine.
func (ch *Channel) recvMsgPacket(packet msgPacket) ([]byte, error) {
	// Check if adding this fragment would exceed the message capacity.
	newLen := len(ch.recving) + len(packet.Data)
	if newLen > ch.desc.RecvMessageCapacity {
		return nil, fmt.Errorf("recv message capacity exceeded: %d > %d", newLen, ch.desc.RecvMessageCapacity)
	}

	ch.recving = append(ch.recving, packet.Data...)

	if packet.EOF {
		msgBytes := ch.recving
		// Reset the receive buffer for the next message.
		ch.recving = ch.recving[:0]
		return msgBytes, nil
	}

	return nil, nil
}

// WriteMessage queues a message for sending. Blocks until the message is queued
// or the channel is closed. Returns false if the queue operation fails.
// This method is goroutine-safe.
func (ch *Channel) WriteMessage(msg []byte) bool {
	ch.sendQueue <- msg
	atomic.AddInt32(&ch.sendQueueSize, 1)
	return true
}

// TryWriteMessage attempts to queue a message for sending without blocking.
// Returns true if the message was successfully queued, false if the queue is full.
// This method is goroutine-safe.
func (ch *Channel) TryWriteMessage(msg []byte) bool {
	select {
	case ch.sendQueue <- msg:
		atomic.AddInt32(&ch.sendQueueSize, 1)
		return true
	default:
		return false
	}
}

// incrementSendQueue atomically increments the send queue size counter.
func (ch *Channel) incrementSendQueue() {
	atomic.AddInt32(&ch.sendQueueSize, 1)
}

// decrementSendQueue atomically decrements the send queue size counter.
func (ch *Channel) decrementSendQueue() {
	atomic.AddInt32(&ch.sendQueueSize, -1)
}

// loadSendQueueSize returns the current send queue size atomically.
func (ch *Channel) loadSendQueueSize() int {
	return int(atomic.LoadInt32(&ch.sendQueueSize))
}

// canSend returns true if the send queue has capacity for more messages.
// This method is goroutine-safe.
func (ch *Channel) canSend() bool {
	return ch.loadSendQueueSize() < ch.desc.SendQueueCapacity
}

// incrementRecentlySent adds n to the recentlySent counter atomically.
// Used by the send routine to track traffic for priority scheduling.
func (ch *Channel) incrementRecentlySent(n int64) {
	atomic.AddInt64(&ch.recentlySent, n)
}

// loadRecentlySent returns the current recentlySent value atomically.
func (ch *Channel) loadRecentlySent() int64 {
	return atomic.LoadInt64(&ch.recentlySent)
}

// decayRecentlySent applies exponential decay to the recentlySent counter.
// Factor of 0.8 means 20% decay per call. Used for fair scheduling.
// This method is goroutine-safe.
func (ch *Channel) decayRecentlySent() {
	current := atomic.LoadInt64(&ch.recentlySent)
	decayed := int64(float64(current) * 0.8)
	atomic.StoreInt64(&ch.recentlySent, decayed)
}

// getSendPriorityRatio calculates the ratio of recentlySent to priority.
// Lower ratios indicate channels that should be scheduled sooner.
// Returns 0 if priority is invalid (should never happen after validation).
func (ch *Channel) getSendPriorityRatio() float64 {
	if ch.priority <= 0 {
		return 0
	}
	sent := atomic.LoadInt64(&ch.recentlySent)
	return float64(sent) / float64(ch.priority)
}
