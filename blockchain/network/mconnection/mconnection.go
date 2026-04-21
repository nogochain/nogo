package mconnection

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// Packet type constants for the wire protocol.
const (
	// packetTypePing is sent periodically as a heartbeat.
	packetTypePing byte = 0x01

	// packetTypePong is sent in response to a ping.
	packetTypePong byte = 0x02

	// packetTypeMsg carries application data fragments.
	packetTypeMsg byte = 0x03
)

// Buffer size constants for I/O optimization.
const (
	// minReadBufferSize is the minimum buffered reader size (1KB).
	minReadBufferSize = 1024

	// minWriteBufferSize is the minimum buffered writer size (64KB).
	minWriteBufferSize = 65536

	// numBatchMsgPackets is the number of packets to send per batch before yielding.
	numBatchMsgPackets = 10

	// channelStatsInterval is how often to update channel statistics (decay recentlySent).
	channelStatsInterval = 2 * time.Second
)

// MConnection multiplexes multiple logical channels over a single net.Conn.
// It handles message fragmentation, priority scheduling, flow control,
// heartbeat detection, and rate monitoring.
//
// Usage:
//   mconn := NewMConnection(conn, descs, onReceive, onError, config)
//   mconn.Start()
//   mconn.Send(chID, data)
//   mconn.Stop()
type MConnection struct {
	// conn is the underlying network connection.
	conn net.Conn

	// bufReader buffers reads from the connection for efficiency.
	bufReader *bufio.Reader

	// bufWriter buffers writes to the connection for efficiency.
	bufWriter *bufio.Writer

	// writeMu protects bufWriter from concurrent access by sendRoutine and pingRoutine.
	writeMu sync.Mutex

	// channels holds all multiplexed channels indexed by their position.
	channels []*Channel

	// channelsIdx maps channel IDs to their Channel instances.
	channelsIdx map[byte]*Channel

	// sendMonitor tracks the outgoing byte rate.
	sendMonitor *FlowRate

	// recvMonitor tracks the incoming byte rate.
	recvMonitor *FlowRate

	// send signals the send routine that there is data to send.
	send chan struct{}

	// pong signals the send routine to send a pong response.
	pong chan struct{}

	// config holds connection configuration parameters.
	config MConnConfig

	// onReceive is called when a complete message is received on any channel.
	onReceive func(chID byte, msg []byte)

	// onError is called when a fatal error occurs.
	onError func(error)

	// errored tracks whether an error has been reported (atomic flag).
	errored uint32

	// quit signals all goroutines to terminate.
	quit chan struct{}

	// pingTicker sends periodic ping packets for liveness detection.
	pingTicker *time.Ticker

	// statsTicker periodically updates channel statistics.
	statsTicker *time.Ticker

	// running indicates whether the connection is active (atomic bool: 0=stopped, 1=running).
	running atomic.Int32

	// wg tracks all goroutines for graceful shutdown.
	wg sync.WaitGroup
}

// NewMConnection creates a new multiplexed connection wrapping the given net.Conn.
// chDescs defines the channels to multiplex, onReceive is called for each complete
// incoming message, onError is called for fatal errors, and config controls behavior.
//
// The connection is not started until Start() is called.
func NewMConnection(
	conn net.Conn,
	chDescs []*ChannelDescriptor,
	onReceive func(chID byte, msg []byte),
	onError func(error),
	config MConnConfig,
) (*MConnection, error) {
	if conn == nil {
		return nil, fmt.Errorf("nil connection")
	}
	if onReceive == nil {
		return nil, fmt.Errorf("nil onReceive callback")
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if err := ValidateDescriptors(chDescs); err != nil {
		return nil, fmt.Errorf("invalid channel descriptors: %w", err)
	}

	channels := make([]*Channel, 0, len(chDescs))
	channelsIdx := make(map[byte]*Channel, len(chDescs))

	for _, desc := range chDescs {
		// Copy the descriptor to prevent cross-connection data races.
		descCopy := *desc
		ch, err := NewChannel(&descCopy)
		if err != nil {
			return nil, fmt.Errorf("create channel 0x%02x: %w", desc.ID, err)
		}
		channels = append(channels, ch)
		channelsIdx[desc.ID] = ch
	}

	return &MConnection{
		conn:        conn,
		bufReader:   bufio.NewReaderSize(conn, minReadBufferSize),
		bufWriter:   bufio.NewWriterSize(conn, minWriteBufferSize),
		channels:    channels,
		channelsIdx: channelsIdx,
		sendMonitor: NewFlowRate(),
		recvMonitor: NewFlowRate(),
		send:        make(chan struct{}, 1),
		pong:        make(chan struct{}, 1),
		config:      config,
		onReceive:   onReceive,
		onError:     onError,
		errored:     0,
		quit:        make(chan struct{}),
		running:     atomic.Int32{},
	}, nil
}

// Start begins the send, receive, and ping goroutines.
// Returns an error if the connection is already running.
func (m *MConnection) Start() error {
	if !m.running.CompareAndSwap(0, 1) {
		return fmt.Errorf("connection already running")
	}

	m.quit = make(chan struct{})
	m.pingTicker = time.NewTicker(m.config.PingTimeout)
	m.statsTicker = time.NewTicker(channelStatsInterval)

	m.wg.Add(3)
	go m.sendRoutine()
	go m.recvRoutine()
	go m.pingRoutine()

	return nil
}

// Stop gracefully shuts down all goroutines and closes the connection.
// It is safe to call Stop multiple times.
func (m *MConnection) Stop() error {
	if !m.running.CompareAndSwap(1, 0) {
		return nil // Already stopped.
	}

	// Signal all goroutines to exit.
	close(m.quit)

	// Stop tickers to prevent further sends.
	if m.pingTicker != nil {
		m.pingTicker.Stop()
	}
	if m.statsTicker != nil {
		m.statsTicker.Stop()
	}

	// Close the connection to unblock any pending I/O operations.
	// This must happen before wg.Wait() to prevent goroutine deadlock.
	if m.conn != nil {
		_ = m.conn.Close()
	}

	// Wait for all goroutines to finish.
	m.wg.Wait()

	return nil
}

// IsRunning returns true if the connection is currently active.
func (m *MConnection) IsRunning() bool {
	return m.running.Load() == 1
}

// Send queues a message to the specified channel (blocking).
// Returns false if the channel does not exist or the connection is not running.
// Returns true if the message was successfully queued.
func (m *MConnection) Send(chID byte, msg []byte) bool {
	if !m.IsRunning() {
		return false
	}

	ch, ok := m.channelsIdx[chID]
	if !ok {
		return false
	}

	// Queue the message (blocking).
	ch.WriteMessage(msg)

	// Signal the send routine that there is data.
	select {
	case m.send <- struct{}{}:
	default:
		// Signal already pending, send routine will process.
	}

	return true
}

// TrySend queues a message to the specified channel (non-blocking).
// Returns false if the channel's send queue is full or the channel does not exist.
func (m *MConnection) TrySend(chID byte, msg []byte) bool {
	if !m.IsRunning() {
		return false
	}

	ch, ok := m.channelsIdx[chID]
	if !ok {
		return false
	}

	if !ch.TryWriteMessage(msg) {
		return false
	}

	// Signal the send routine.
	select {
	case m.send <- struct{}{}:
	default:
	}

	return true
}

// CanSend returns true if the specified channel has capacity for more messages.
// Returns false if the channel does not exist or the connection is not running.
func (m *MConnection) CanSend(chID byte) bool {
	if !m.IsRunning() {
		return false
	}

	ch, ok := m.channelsIdx[chID]
	if !ok {
		return false
	}

	return ch.canSend()
}

// TrafficStatus returns the current send and receive rates in bytes per second.
func (m *MConnection) TrafficStatus() (sendRate float64, recvRate float64) {
	sendRate = m.sendMonitor.Rate()
	recvRate = m.recvMonitor.Rate()
	return sendRate, recvRate
}

// RemoteAddr returns the remote network address of the connection.
func (m *MConnection) RemoteAddr() string {
	if m.conn == nil {
		return "nil"
	}
	return m.conn.RemoteAddr().String()
}

// String returns a human-readable representation for debugging.
func (m *MConnection) String() string {
	addr := "nil"
	if m.conn != nil {
		addr = m.conn.RemoteAddr().String()
	}
	return fmt.Sprintf("MConn{%s}", addr)
}

// sendRoutine handles priority-based message transmission.
// It selects the channel with the lowest recentlySent/priority ratio for fair scheduling.
// Flow control is enforced by sleeping when the send rate limit is exceeded.
func (m *MConnection) sendRoutine() {
	defer m.wg.Done()
	defer m._recover()

	for {
		select {
		case <-m.quit:
			return
		case <-m.statsTicker.C:
			// Decay channel statistics periodically.
			for _, ch := range m.channels {
				ch.decayRecentlySent()
			}
		case <-m.pingTicker.C:
			// Send a ping heartbeat packet.
			m.sendPing()
		case <-m.pong:
			// Send a pong response.
			m.sendPong()
		case <-m.send:
			// Send pending messages from channels.
			m.sendSomeMsgPackets()
		}

		if !m.IsRunning() {
			return
		}
	}
}

// recvRoutine reads packets from the connection and reassembles fragmented messages.
// When a complete message is assembled, it calls the onReceive callback.
// Flow control is enforced by checking the receive rate limit before each read.
func (m *MConnection) recvRoutine() {
	defer m.wg.Done()
	defer m._recover()

	for {
		// Check receive rate limit before reading.
		m.enforceRecvLimit()

		// Read the packet type byte.
		pktType, err := m.readByte()
		if err != nil {
			if m.IsRunning() {
				m.stopForError(fmt.Errorf("read packet type: %w", err))
			}
			return
		}
		m.recvMonitor.Update(1)

		switch pktType {
		case packetTypePing:
			// Received a ping, respond with a pong.
			select {
			case m.pong <- struct{}{}:
			default:
				// Pong already pending.
			}

		case packetTypePong:
			// Received a pong, connection is alive.

		case packetTypeMsg:
			// Read a message packet.
			packet, n, err := m.readMsgPacket()
			if err != nil {
				if m.IsRunning() {
					m.stopForError(fmt.Errorf("read msg packet: %w", err))
				}
				return
			}
			m.recvMonitor.Update(int64(n))

			// Find the channel for this packet.
			ch, ok := m.channelsIdx[packet.ChannelID]
			if !ok || ch == nil {
				if m.IsRunning() {
					m.stopForError(fmt.Errorf("unknown channel ID: 0x%02x", packet.ChannelID))
				}
				return
			}

			// Process the packet fragment.
			msgBytes, err := ch.recvMsgPacket(*packet)
			if err != nil {
				if m.IsRunning() {
					m.stopForError(fmt.Errorf("recv msg packet on channel 0x%02x: %w", packet.ChannelID, err))
				}
				return
			}

			// Complete message received, deliver to callback.
			if msgBytes != nil && m.onReceive != nil {
				m.onReceive(packet.ChannelID, msgBytes)
			}

		default:
			if m.IsRunning() {
				m.stopForError(fmt.Errorf("unknown packet type: 0x%02x", pktType))
			}
			return
		}

		if !m.IsRunning() {
			return
		}
	}
}

// pingRoutine sends periodic ping packets for liveness detection.
// This runs independently from the send routine to ensure pings are sent
// even when there is no application data to transmit.
func (m *MConnection) pingRoutine() {
	defer m.wg.Done()
	defer m._recover()

	for {
		select {
		case <-m.quit:
			return
		case <-m.pingTicker.C:
			m.sendPing()
		}
	}
}

// sendSomeMsgPackets sends a batch of message packets from channels.
// Uses priority scheduling: the channel with the lowest recentlySent/priority ratio
// is selected first to ensure fair bandwidth distribution.
// Returns true if all channels are exhausted, false if more data is pending.
func (m *MConnection) sendSomeMsgPackets() bool {
	// Enforce send rate limit.
	m.enforceSendLimit()

	for i := 0; i < numBatchMsgPackets; i++ {
		if m.sendMsgPacket() {
			return true // All channels exhausted.
		}
	}
	return false
}

// sendMsgPacket sends a single packet from the channel with the highest priority.
// Priority is calculated as recentlySent/priority ratio (lower ratio = higher priority).
// Returns true if all channels have no pending data, false if a packet was sent.
func (m *MConnection) sendMsgPacket() bool {
	var leastRatio float64 = math.MaxFloat64
	var leastChannel *Channel

	// Find the channel with the lowest recentlySent/priority ratio.
	for _, ch := range m.channels {
		if !ch.isSendPending() {
			continue
		}
		ratio := ch.getSendPriorityRatio()
		if ratio < leastRatio {
			leastRatio = ratio
			leastChannel = ch
		}
	}

	if leastChannel == nil {
		return true // No channels have pending data.
	}

	// Create the next fragment packet from the selected channel.
	packet := leastChannel.nextMsgPacket()

	// Serialize and write the packet.
	// First write the packet type byte, then the packet payload.
	m.writeMu.Lock()
	if err := m.bufWriter.WriteByte(packetTypeMsg); err != nil {
		m.writeMu.Unlock()
		if m.IsRunning() {
			m.stopForError(fmt.Errorf("write packet type: %w", err))
		}
		return true
	}

	n, err := packet.SerializeTo(m.bufWriter)
	n++ // Add the packet type byte to the count.
	if err != nil {
		m.writeMu.Unlock()
		if m.IsRunning() {
			m.stopForError(fmt.Errorf("write msg packet: %w", err))
		}
		return true
	}

	// Update traffic monitors and channel statistics.
	m.sendMonitor.Update(int64(n))
	leastChannel.incrementRecentlySent(int64(n))

	// Flush the buffer to ensure data is sent.
	if err := m.bufWriter.Flush(); err != nil {
		m.writeMu.Unlock()
		if m.IsRunning() {
			m.stopForError(fmt.Errorf("flush after send: %w", err))
		}
		return true
	}
	m.writeMu.Unlock()

	return false
}

// sendPing writes a ping packet to the connection.
func (m *MConnection) sendPing() {
	if !m.IsRunning() {
		return
	}

	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	if err := m.bufWriter.WriteByte(packetTypePing); err != nil {
		if m.IsRunning() {
			m.stopForError(fmt.Errorf("write ping packet: %w", err))
		}
		return
	}
	m.sendMonitor.Update(1)

	if err := m.bufWriter.Flush(); err != nil {
		if m.IsRunning() {
			m.stopForError(fmt.Errorf("flush after ping: %w", err))
		}
	}
}

// sendPong writes a pong packet to the connection.
func (m *MConnection) sendPong() {
	if !m.IsRunning() {
		return
	}

	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	if err := m.bufWriter.WriteByte(packetTypePong); err != nil {
		if m.IsRunning() {
			m.stopForError(fmt.Errorf("write pong packet: %w", err))
		}
		return
	}
	m.sendMonitor.Update(1)

	if err := m.bufWriter.Flush(); err != nil {
		if m.IsRunning() {
			m.stopForError(fmt.Errorf("flush after pong: %w", err))
		}
	}
}

// enforceSendLimit blocks until the send rate limit allows more data to be written.
// Calculates the sleep duration based on the configured SendRate.
func (m *MConnection) enforceSendLimit() {
	if m.config.SendRate <= 0 {
		return
	}

	totalBytes := m.sendMonitor.TotalBytes()
	if totalBytes <= 0 {
		return
	}

	// Calculate how long we should have slept to stay within the rate limit.
	// sleepTime = totalBytes / sendRate - elapsedSinceStart
	// If we're ahead, we don't need to sleep.
	elapsed := m.sendMonitor.ElapsedSinceStart()
	expectedTime := time.Duration(float64(totalBytes) / float64(m.config.SendRate) * float64(time.Second))

	if expectedTime > elapsed {
		sleepDuration := expectedTime - elapsed
		if sleepDuration > 0 && sleepDuration < time.Second {
			select {
			case <-time.After(sleepDuration):
			case <-m.quit:
				return
			}
		}
	}
}

// enforceRecvLimit blocks until the receive rate limit allows more data to be read.
func (m *MConnection) enforceRecvLimit() {
	if m.config.RecvRate <= 0 {
		return
	}

	totalBytes := m.recvMonitor.TotalBytes()
	if totalBytes <= 0 {
		return
	}

	elapsed := m.recvMonitor.ElapsedSinceStart()
	expectedTime := time.Duration(float64(totalBytes) / float64(m.config.RecvRate) * float64(time.Second))

	if expectedTime > elapsed {
		sleepDuration := expectedTime - elapsed
		if sleepDuration > 0 && sleepDuration < time.Second {
			select {
			case <-time.After(sleepDuration):
			case <-m.quit:
				return
			}
		}
	}
}

// readByte reads a single byte from the buffered reader.
func (m *MConnection) readByte() (byte, error) {
	b, err := m.bufReader.ReadByte()
	if err != nil {
		return 0, fmt.Errorf("read byte: %w", err)
	}
	return b, nil
}

// writeByte writes a single byte to the buffered writer.
func (m *MConnection) writeByte(b byte) error {
	err := m.bufWriter.WriteByte(b)
	if err != nil {
		return fmt.Errorf("write byte: %w", err)
	}
	return nil
}

// readMsgPacket reads a complete msgPacket from the connection.
func (m *MConnection) readMsgPacket() (*msgPacket, int, error) {
	packet, err := DeserializePacket(m.bufReader)
	if err != nil {
		return nil, 0, fmt.Errorf("deserialize packet: %w", err)
	}
	// Calculate approximate byte count for rate monitoring.
	n := msgPacketHeaderSize + len(packet.Data)
	return packet, n, nil
}

// _recover catches panics from goroutines, logs the stack trace,
// and triggers a graceful shutdown with the error.
func (m *MConnection) _recover() {
	if r := recover(); r != nil {
		stack := debug.Stack()
		err := fmt.Errorf("panic in MConnection goroutine: %v\nstack: %s", r, string(stack))
		m.stopForError(err)
	}
}

// stopForError gracefully stops the connection and reports the error to the onError callback.
// Uses atomic compare-and-swap to ensure the error is reported only once.
func (m *MConnection) stopForError(err error) {
	// Attempt to stop the connection.
	if m.IsRunning() {
		if stopErr := m.Stop(); stopErr != nil {
			// Log but don't override the original error.
			_ = fmt.Errorf("stop on error: %w, original: %w", stopErr, err)
		}
	}

	// Report the error callback exactly once.
	if atomic.CompareAndSwapUint32(&m.errored, 0, 1) && m.onError != nil {
		m.onError(err)
	}
}

// Context returns a context that is cancelled when the connection stops.
func (m *MConnection) Context() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Wait for the quit channel to close.
		if m.quit != nil {
			<-m.quit
		}
		cancel()
	}()
	return ctx
}
