package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// WSEvent is an alias for core.WSEvent for backward compatibility
type WSEvent = core.WSEvent

// EventSink is an alias for core.EventSink for backward compatibility
type EventSink = core.EventSink

type WSHub struct {
	maxConns int

	mu    sync.Mutex
	conns map[*wsClient]struct{}
}

// Stop stops the WebSocket server
// Production-grade: gracefully closes all WebSocket connections
func (h *WSHub) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.conns {
		c.close()
	}
	return nil
}

func NewWSHub(maxConns int) *WSHub {
	if maxConns <= 0 {
		maxConns = 100
	}
	return &WSHub{
		maxConns: maxConns,
		conns:    map[*wsClient]struct{}{},
	}
}

func (h *WSHub) Publish(e WSEvent) {
	if h == nil {
		return
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	h.mu.Lock()
	for c := range h.conns {
		if !c.acceptsEvent(e) {
			continue
		}
		select {
		case c.send <- b:
		default:
			// slow consumer; drop it
			go c.close()
			delete(h.conns, c)
		}
	}
	h.mu.Unlock()
}

func (h *WSHub) ServeWS(w http.ResponseWriter, r *http.Request) {
	if h == nil {
		http.Error(w, "websocket not configured", http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	conn, rw, err := wsUpgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	client := &wsClient{
		conn: conn,
		rw:   rw,
		send: make(chan []byte, 64),
		done: make(chan struct{}),
		sub:  wsSubscriptions{legacyAll: true},
	}

	h.mu.Lock()
	if len(h.conns) >= h.maxConns {
		h.mu.Unlock()
		if closeErr := client.writeClose(1013, "server overloaded"); closeErr != nil {
			log.Printf("websocket: failed to write close message: %v", closeErr)
		}
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("websocket: failed to close connection: %v", closeErr)
		}
		return
	}
	h.conns[client] = struct{}{}
	h.mu.Unlock()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		<-client.done
		cancel()
	}()

	// writer loop
	go client.writeLoop(ctx)
	// read loop (handles ping/pong/close; ignores messages)
	client.readLoop(ctx)

	h.mu.Lock()
	delete(h.conns, client)
	h.mu.Unlock()
	_ = client.close()
}

type wsClient struct {
	conn net.Conn
	rw   *bufio.ReadWriter

	send chan []byte
	done chan struct{}

	subMu sync.RWMutex
	sub   wsSubscriptions

	closeOnce sync.Once
	writeMu   sync.Mutex // protects rw.Writer from concurrent access
}

type wsSubscriptions struct {
	// Legacy behavior: if no subscription message is ever received, we broadcast everything.
	legacyAll bool

	// Once a subscription message is received, events are filtered unless "all" is subscribed.
	all       bool
	addresses map[string]struct{}
	types     map[string]struct{}
}

func (c *wsClient) close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.done)
		err = c.conn.Close()
	})
	return err
}

func (c *wsClient) writeLoop(ctx context.Context) {
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = c.writeClose(1000, "bye")
			_ = c.close()
			return
		case b := <-c.send:
			if err := c.writeText(b); err != nil {
				_ = c.close()
				return
			}
		case <-ping.C:
			if err := c.writePing(); err != nil {
				_ = c.close()
				return
			}
		}
	}
}

func (c *wsClient) readLoop(ctx context.Context) {
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		op, payload, err := wsReadFrame(c.rw.Reader)
		if err != nil {
			return
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		switch op {
		case 0x8: // close
			_ = c.writeClose(1000, "bye")
			return
		case 0x9: // ping
			_ = c.writePong(payload)
		case 0xA: // pong
			// ignore
		case 0x1: // text
			c.handleText(payload)
		default:
			// ignore text/binary/continuation
		}
	}
}

type wsClientMsg struct {
	Type    string `json:"type"`
	Topic   string `json:"topic,omitempty"`
	Address string `json:"address,omitempty"`
	Event   string `json:"event,omitempty"`
}

func (c *wsClient) handleText(payload []byte) {
	var msg wsClientMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(msg.Type)) {
	case "subscribe":
		c.subscribe(msg)
	case "unsubscribe":
		c.unsubscribe(msg)
	default:
		// ignore unknown messages for forwards/backwards compatibility
	}
}

func (c *wsClient) subscribe(msg wsClientMsg) {
	topic := strings.ToLower(strings.TrimSpace(msg.Topic))
	eventType := strings.TrimSpace(msg.Event)
	addr := strings.ToLower(strings.TrimSpace(msg.Address))

	c.subMu.Lock()
	c.sub.legacyAll = false
	switch topic {
	case "all":
		c.sub.all = true
		c.sub.addresses = nil
		c.sub.types = nil
		c.subMu.Unlock()
		c.sendControl("subscribed", map[string]any{"topic": "all"})
		return
	case "address":
		if err := validateAddress(addr); err != nil {
			c.subMu.Unlock()
			c.sendControl("error", map[string]any{"message": "invalid address"})
			return
		}
		if c.sub.addresses == nil {
			c.sub.addresses = map[string]struct{}{}
		}
		c.sub.addresses[addr] = struct{}{}
		c.subMu.Unlock()
		c.sendControl("subscribed", map[string]any{"topic": "address", "address": addr})
		return
	case "type":
		if eventType == "" {
			c.subMu.Unlock()
			c.sendControl("error", map[string]any{"message": "missing event for topic=type"})
			return
		}
		if c.sub.types == nil {
			c.sub.types = map[string]struct{}{}
		}
		c.sub.types[eventType] = struct{}{}
		c.subMu.Unlock()
		c.sendControl("subscribed", map[string]any{"topic": "type", "event": eventType})
		return
	default:
		c.subMu.Unlock()
		c.sendControl("error", map[string]any{"message": "unknown topic"})
		return
	}
}

func (c *wsClient) unsubscribe(msg wsClientMsg) {
	topic := strings.ToLower(strings.TrimSpace(msg.Topic))
	eventType := strings.TrimSpace(msg.Event)
	addr := strings.ToLower(strings.TrimSpace(msg.Address))

	c.subMu.Lock()
	c.sub.legacyAll = false
	switch topic {
	case "all":
		c.sub.all = false
		c.subMu.Unlock()
		c.sendControl("unsubscribed", map[string]any{"topic": "all"})
		return
	case "address":
		if c.sub.addresses != nil {
			delete(c.sub.addresses, addr)
			if len(c.sub.addresses) == 0 {
				c.sub.addresses = nil
			}
		}
		c.subMu.Unlock()
		c.sendControl("unsubscribed", map[string]any{"topic": "address", "address": addr})
		return
	case "type":
		if c.sub.types != nil {
			delete(c.sub.types, eventType)
			if len(c.sub.types) == 0 {
				c.sub.types = nil
			}
		}
		c.subMu.Unlock()
		c.sendControl("unsubscribed", map[string]any{"topic": "type", "event": eventType})
		return
	default:
		c.subMu.Unlock()
		c.sendControl("error", map[string]any{"message": "unknown topic"})
		return
	}
}

func (c *wsClient) sendControl(typ string, data any) {
	b, err := json.Marshal(WSEvent{Type: typ, Data: data})
	if err != nil {
		return
	}
	select {
	case c.send <- b:
	default:
		// best effort
	}
}

func (c *wsClient) acceptsEvent(e WSEvent) bool {
	c.subMu.RLock()
	sub := c.sub
	c.subMu.RUnlock()

	if sub.legacyAll {
		return true
	}
	if sub.all {
		return true
	}
	if len(sub.types) > 0 {
		if _, ok := sub.types[e.Type]; ok {
			return true
		}
	}
	if len(sub.addresses) == 0 {
		return false
	}
	addrs := wsEventAddresses(e)
	for _, a := range addrs {
		if _, ok := sub.addresses[a]; ok {
			return true
		}
	}
	return false
}

func wsEventAddresses(e WSEvent) []string {
	m, ok := e.Data.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	set := map[string]struct{}{}
	addIfAddr := func(v any) {
		s, ok := v.(string)
		if !ok {
			return
		}
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return
		}
		set[s] = struct{}{}
	}

	addIfAddr(m["fromAddr"])
	addIfAddr(m["toAddress"])
	addIfAddr(m["address"])

	if raw, ok := m["addresses"]; ok {
		switch v := raw.(type) {
		case []string:
			for _, s := range v {
				addIfAddr(s)
			}
		case []any:
			for _, s := range v {
				addIfAddr(s)
			}
		}
	}

	out := make([]string, 0, len(set))
	for addr := range set {
		out = append(out, addr)
	}
	sort.Strings(out)
	return out
}

func (c *wsClient) writeText(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return wsWriteFrame(c.rw.Writer, 0x1, payload)
}

func (c *wsClient) writePing() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return wsWriteFrame(c.rw.Writer, 0x9, nil)
}

func (c *wsClient) writePong(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return wsWriteFrame(c.rw.Writer, 0xA, payload)
}

func (c *wsClient) writeClose(code uint16, reason string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	var b []byte
	if code != 0 {
		b = make([]byte, 2+len(reason))
		binary.BigEndian.PutUint16(b[:2], code)
		copy(b[2:], []byte(reason))
	}
	return wsWriteFrame(c.rw.Writer, 0x8, b)
}

func wsUpgrade(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	if !strings.EqualFold(r.Header.Get("Connection"), "Upgrade") && !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return nil, nil, errors.New("missing Connection: Upgrade")
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, nil, errors.New("missing Upgrade: websocket")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		return nil, nil, errors.New("unsupported websocket version")
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		return nil, nil, errors.New("missing Sec-WebSocket-Key")
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking not supported")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, nil, err
	}

	accept := wsAcceptKey(key)
	var resp bytes.Buffer
	resp.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	resp.WriteString("Upgrade: websocket\r\n")
	resp.WriteString("Connection: Upgrade\r\n")
	resp.WriteString("Sec-WebSocket-Accept: " + accept + "\r\n")
	resp.WriteString("\r\n")
	if _, err := conn.Write(resp.Bytes()); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("websocket: failed to close connection on handshake error: %v", closeErr)
		}
		return nil, nil, err
	}
	return conn, rw, nil
}

func wsAcceptKey(key string) string {
	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	sum := sha1.Sum([]byte(key + magic))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func wsReadFrame(r *bufio.Reader) (opcode byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	fin := (hdr[0] & 0x80) != 0
	_ = fin // we ignore fragmentation and read full frames only
	opcode = hdr[0] & 0x0F
	masked := (hdr[1] & 0x80) != 0
	if !masked {
		return 0, nil, errors.New("client frames must be masked")
	}
	plen := int(hdr[1] & 0x7F)
	switch plen {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		plen = int(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		n := binary.BigEndian.Uint64(b[:])
		if n > 1<<20 {
			return 0, nil, errors.New("frame too large")
		}
		plen = int(n)
	}

	var mask [4]byte
	if _, err = io.ReadFull(r, mask[:]); err != nil {
		return 0, nil, err
	}
	payload = make([]byte, plen)
	if plen > 0 {
		if _, err = io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
		for i := 0; i < plen; i++ {
			payload[i] ^= mask[i%4]
		}
	}
	return opcode, payload, nil
}

func wsWriteFrame(w *bufio.Writer, opcode byte, payload []byte) error {
	if opcode == 0 {
		return errors.New("missing opcode")
	}
	var hdr bytes.Buffer
	hdr.WriteByte(0x80 | (opcode & 0x0F)) // FIN set
	n := len(payload)
	switch {
	case n <= 125:
		hdr.WriteByte(byte(n))
	case n <= 65535:
		hdr.WriteByte(126)
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(n))
		hdr.Write(b[:])
	default:
		hdr.WriteByte(127)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		hdr.Write(b[:])
	}
	if _, err := w.Write(hdr.Bytes()); err != nil {
		return err
	}
	if n > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return w.Flush()
}
