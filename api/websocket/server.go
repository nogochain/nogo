package websocket

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Hub struct {
	maxConns int

	mu    sync.Mutex
	conns map[*Client]struct{}
}

func NewHub(maxConns int) *Hub {
	if maxConns <= 0 {
		maxConns = 100
	}
	return &Hub{
		maxConns: maxConns,
		conns:    map[*Client]struct{}{},
	}
}

func (h *Hub) Publish(e WSEvent) {
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
			go c.close()
			delete(h.conns, c)
		}
	}
	h.mu.Unlock()
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
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

	client := &Client{
		conn: conn,
		rw:   rw,
		send: make(chan []byte, 64),
		done: make(chan struct{}),
		sub:  subscriptions{legacyAll: true},
	}

	h.mu.Lock()
	if len(h.conns) >= h.maxConns {
		h.mu.Unlock()
		_ = client.writeClose(1013, "server overloaded")
		_ = conn.Close()
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

	go client.writeLoop(ctx)
	client.readLoop(ctx)

	h.mu.Lock()
	delete(h.conns, client)
	h.mu.Unlock()
	_ = client.close()
}

type Client struct {
	conn net.Conn
	rw   *bufio.ReadWriter

	send chan []byte
	done chan struct{}

	subMu sync.RWMutex
	sub   subscriptions

	closeOnce sync.Once
}

type subscriptions struct {
	legacyAll bool
	all       bool
	addresses map[string]struct{}
	types     map[string]struct{}
}

func (c *Client) close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.done)
		err = c.conn.Close()
	})
	return err
}

func (c *Client) writeLoop(ctx context.Context) {
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

func (c *Client) readLoop(ctx context.Context) {
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
		case 0x8:
			_ = c.writeClose(1000, "bye")
			return
		case 0x9:
			_ = c.writePong(payload)
		case 0xA:
		case 0x1:
			c.handleText(payload)
		}
	}
}

type clientMsg struct {
	Type    string `json:"type"`
	Topic   string `json:"topic,omitempty"`
	Address string `json:"address,omitempty"`
	Event   string `json:"event,omitempty"`
}

func (c *Client) handleText(payload []byte) {
	var msg clientMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(msg.Type)) {
	case "subscribe":
		c.subscribe(msg)
	case "unsubscribe":
		c.unsubscribe(msg)
	}
}

func (c *Client) subscribe(msg clientMsg) {
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

func (c *Client) unsubscribe(msg clientMsg) {
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

func (c *Client) sendControl(typ string, data any) {
	b, err := json.Marshal(WSEvent{Type: typ, Data: data})
	if err != nil {
		return
	}
	select {
	case c.send <- b:
	default:
	}
}

func (c *Client) acceptsEvent(e WSEvent) bool {
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
	addrs := eventAddresses(e)
	for _, a := range addrs {
		if _, ok := sub.addresses[a]; ok {
			return true
		}
	}
	return false
}

func eventAddresses(e WSEvent) []string {
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

func validateAddress(addr string) error {
	if addr == "" {
		return errors.New("empty address")
	}
	if !strings.HasPrefix(addr, "NOGO") && !strings.HasPrefix(addr, "0x") && len(addr) != 64 {
		return errors.New("invalid address format")
	}
	return nil
}

func (c *Client) writeText(payload []byte) error {
	return wsWriteFrame(c.rw.Writer, 0x1, payload)
}

func (c *Client) writePing() error {
	return wsWriteFrame(c.rw.Writer, 0x9, nil)
}

func (c *Client) writePong(payload []byte) error {
	return wsWriteFrame(c.rw.Writer, 0xA, payload)
}

func (c *Client) writeClose(code uint16, reason string) error {
	var b []byte
	if code != 0 {
		b = make([]byte, 2+len(reason))
		putUint16BE(b[:2], code)
		copy(b[2:], []byte(reason))
	}
	return wsWriteFrame(c.rw.Writer, 0x8, b)
}

func putUint16BE(b []byte, v uint16) {
	b[0] = byte(v >> 8)
	b[1] = byte(v)
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
	var resp strings.Builder
	resp.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	resp.WriteString("Upgrade: websocket\r\n")
	resp.WriteString("Connection: Upgrade\r\n")
	resp.WriteString("Sec-WebSocket-Accept: " + accept + "\r\n")
	resp.WriteString("\r\n")
	if _, err := conn.Write([]byte(resp.String())); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return conn, rw, nil
}

func wsAcceptKey(key string) string {
	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.Sum([]byte(key + magic))
	return base64.StdEncoding.EncodeToString(h[:])
}

func sha1Hash(data []byte) [20]byte {
	h := sha1New()
	h.Write(data)
	var out [20]byte
	copy(out[:], h.Sum(nil))
	return out
}

func sha1New() interface {
	Write([]byte) (int, error)
	Sum(b []byte) []byte
} {
	return newSHA1()
}

type sha1Ctx struct {
	h   [5]uint32
	n   int
	buf [64]byte
}

func newSHA1() *sha1Ctx {
	return &sha1Ctx{h: [5]uint32{0x67452301, 0xEFCDAB89, 0x98BADCFE, 0x10325476, 0xC3D2E1F0}}
}

func (c *sha1Ctx) Write(p []byte) (int, error) {
	n := len(p)
	c.n += n
	for len(p) > 0 {
		if c.n == len(p) && len(c.buf) == 0 {
			c.process(p)
			return n, nil
		}
		i := copy(c.buf[c.n:], p)
		p = p[i:]
		c.n -= i
		if c.n == 0 {
			c.process(c.buf[:])
		}
	}
	return n, nil
}

func (c *sha1Ctx) Sum(b []byte) []byte {
	d := *c
	rem := d.n % 64
	pad := uint64(1<<3) + uint64(d.n)
	var lenB [8]byte
	for i := 7; i >= 0; i-- {
		lenB[i] = byte(pad >> uint(56-i*8))
	}
	d.process(d.buf[:rem])
	d.process(append([]byte{0x80}, make([]byte, 63-rem/64*64)...))
	d.process(lenB[:])
	for _, v := range d.h {
		b = append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
	return b
}

func (c *sha1Ctx) process(p []byte) {
	for i := 0; i < len(p); i += 64 {
		var w [80]uint32
		for j := 0; j < 16; j++ {
			w[j] = uint32(p[i+j*4])<<24 | uint32(p[i+j*4+1])<<16 | uint32(p[i+j*4+2])<<8 | uint32(p[i+j*4+3])
		}
		for j := 16; j < 80; j++ {
			t := w[j-3] ^ w[j-8] ^ w[j-14] ^ w[j-16]
			w[j] = t<<1 | t>>31
		}
		a, b, cc, d, e := c.h[0], c.h[1], c.h[2], c.h[3], c.h[4]
		for i := 0; i < 80; i++ {
			var f uint32
			var k uint32
			switch {
			case i < 20:
				f = (b & cc) | (^b & d)
				k = 0x5A827999
			case i < 40:
				f = b ^ cc ^ d
				k = 0x6ED9EBA1
			case i < 60:
				f = (b & cc) | (b & d) | (cc & d)
				k = 0x8F1BBCDC
			default:
				f = b ^ cc ^ d
				k = 0xCA62C1D6
			}
			t := (a<<5 | a>>27) + f + e + k + w[i]
			e, d, cc, b, a = d, cc, b<<30|cc>>2, a, t
		}
		c.h[0] += a
		c.h[1] += b
		c.h[2] += cc
		c.h[3] += d
		c.h[4] += e
	}
}

func base64Encode(data []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	n := (len(data) + 2) / 3 * 4
	var out strings.Builder
	out.Grow(n)
	for i := 0; i < len(data); i += 3 {
		var v uint32
		remaining := len(data) - i
		switch remaining {
		case 1:
			v = uint32(data[i]) << 16
		case 2:
			v = uint32(data[i])<<16 | uint32(data[i+1])<<8
		default:
			v = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
		}
		out.WriteByte(alphabet[v>>18&63])
		out.WriteByte(alphabet[v>>12&63])
		if remaining > 1 {
			out.WriteByte(alphabet[v>>6&63])
		} else {
			out.WriteByte('=')
		}
		if remaining > 2 {
			out.WriteByte(alphabet[v&63])
		} else {
			out.WriteByte('=')
		}
	}
	return out.String()
}

func wsReadFrame(r *bufio.Reader) (opcode byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
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
		plen = int(binaryBE16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		n := binaryBE64(b[:])
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

func binaryBE16(b []byte) uint16 {
	return uint16(b[0])<<8 | uint16(b[1])
}

func binaryBE64(b []byte) uint64 {
	return uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
}

func wsWriteFrame(w *bufio.Writer, opcode byte, payload []byte) error {
	if opcode == 0 {
		return errors.New("missing opcode")
	}
	var hdr [10]byte
	hdr[0] = 0x80 | (opcode & 0x0F)
	n := len(payload)
	idx := 1
	switch {
	case n <= 125:
		hdr[idx] = byte(n)
	case n <= 65535:
		hdr[idx] = 126
		hdr[idx+1] = byte(n >> 8)
		hdr[idx+2] = byte(n)
		idx += 2
	default:
		hdr[idx] = 127
		hdr[idx+1] = byte(n >> 56)
		hdr[idx+2] = byte(n >> 48)
		hdr[idx+3] = byte(n >> 40)
		hdr[idx+4] = byte(n >> 32)
		hdr[idx+5] = byte(n >> 24)
		hdr[idx+6] = byte(n >> 16)
		hdr[idx+7] = byte(n >> 8)
		hdr[idx+8] = byte(n)
		idx += 8
	}
	if _, err := w.Write(hdr[:idx+1]); err != nil {
		return err
	}
	if n > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return w.Flush()
}
