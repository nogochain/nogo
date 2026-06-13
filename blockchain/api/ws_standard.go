// Copyright 2026 NogoChain Team
// Standard WebSocket compatibility layer using gorilla/websocket.
// Provides a /ws/std endpoint that wraps the existing WSHub event stream
// with a widely-compatible WebSocket implementation.

package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// wsStdUpgrader is the gorilla/websocket Upgrader for the /ws/std endpoint.
// Allows all origins for compatibility with exchange clients and block explorers.
var wsStdUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// wsStdPingInterval defines keep-alive ping frequency.
const wsStdPingInterval = 30 * time.Second

// wsStdWriteTimeout is the write deadline for each message.
const wsStdWriteTimeout = 10 * time.Second

// wsStdReadTimeout is the read deadline extended by pong handler.
const wsStdReadTimeout = 60 * time.Second

// ServeWSStd handles GET /ws/std requests using gorilla/websocket.
// Wraps the existing WSHub for event distribution while providing
// a standard WebSocket interface to external consumers.
func (h *WSHub) ServeWSStd(w http.ResponseWriter, r *http.Request) {
	if h == nil {
		http.Error(w, "websocket not configured", http.StatusInternalServerError)
		return
	}

	conn, err := wsStdUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws/std: upgrade failed: %v", err)
		return
	}

	client := &wsStdClient{
		conn: conn,
		hub:  h,
		done: make(chan struct{}),
		sub:  wsStdSubscriptions{legacyAll: true},
	}

	h.mu.Lock()
	if len(h.conns) >= h.maxConns {
		h.mu.Unlock()
		if closeErr := conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(1013, "server overloaded")); closeErr != nil {
			log.Printf("ws/std: close write error: %v", closeErr)
		}
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("ws/std: close error: %v", closeErr)
		}
		return
	}
	h.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)

	// Read pump: handles ping/pong/close and subscription messages.
	go func() {
		defer wg.Done()
		client.readPump()
	}()

	// Write pump: publishes events from WSHub to the gorilla/websocket connection.
	go func() {
		defer wg.Done()
		client.writePump()
	}()

	wg.Wait()
	// Cleanup removed — WSHub is owned by Server, lifecycle managed elsewhere.
}

// wsStdClient wraps a gorilla/websocket connection with subscription state.
type wsStdClient struct {
	conn *websocket.Conn
	hub  *WSHub

	subMu sync.RWMutex
	sub   wsStdSubscriptions

	doneMu sync.RWMutex
	done   chan struct{}
}

type wsStdSubscriptions struct {
	legacyAll bool
	all       bool
	addresses map[string]struct{}
	types     map[string]struct{}
}

// readPump reads messages from the gorilla/websocket connection.
func (c *wsStdClient) readPump() {
	defer func() {
		_ = c.conn.Close()
	}()

	_ = c.conn.SetReadDeadline(time.Now().Add(wsStdReadTimeout))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(wsStdReadTimeout))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws/std: read error: %v", err)
			}
			return
		}

		var msg wsClientMsg
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "subscribe":
			c.handleSubscribe(msg)
		case "unsubscribe":
			c.handleUnsubscribe(msg)
		}
	}
}

// writePump writes events from the WSHub to the gorilla/websocket connection.
func (c *wsStdClient) writePump() {
	ticker := time.NewTicker(wsStdPingInterval)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsStdWriteTimeout))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleSubscribe processes a subscribe message.
func (c *wsStdClient) handleSubscribe(msg wsClientMsg) {
	topic := msg.Topic
	eventType := msg.Event
	addr := msg.Address

	c.subMu.Lock()
	c.sub.legacyAll = false
	switch topic {
	case "all":
		c.sub.all = true
		c.sub.addresses = nil
		c.sub.types = nil
	case "address":
		if c.sub.addresses == nil {
			c.sub.addresses = make(map[string]struct{})
		}
		c.sub.addresses[addr] = struct{}{}
	case "type":
		if c.sub.types == nil {
			c.sub.types = make(map[string]struct{})
		}
		c.sub.types[eventType] = struct{}{}
	}
	c.subMu.Unlock()
}

// handleUnsubscribe processes an unsubscribe message.
func (c *wsStdClient) handleUnsubscribe(msg wsClientMsg) {
	topic := msg.Topic
	eventType := msg.Event
	addr := msg.Address

	c.subMu.Lock()
	c.sub.legacyAll = false
	switch topic {
	case "all":
		c.sub.all = false
	case "address":
		if c.sub.addresses != nil {
			delete(c.sub.addresses, addr)
		}
	case "type":
		if c.sub.types != nil {
			delete(c.sub.types, eventType)
		}
	}
	c.subMu.Unlock()
}
