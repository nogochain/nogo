// Copyright 2026 NogoChain Team
// Webhook notification system for exchange integration.
// Supports HMAC-SHA256 signing, BoltDB-backed event queue,
// exponential retry with jitter, and chain reorganization events.

package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// webhookDBBucket is the BoltDB bucket name for webhook event queues.
const webhookDBBucket = "webhook_queue"

// webhook registration and queue configuration.
const (
	maxWebhookSubscriptions = 50
	webhookMaxRetries       = 5
	webhookBaseDelay        = 2 * time.Second
	webhookMaxDelay         = 5 * time.Minute
	webhookQueueFile        = "webhooks.db"
	webhookWorkerCount      = 4
)

// WebhookEventType categorizes blockchain events for external consumers.
type WebhookEventType string

const (
	WebhookNewTx       WebhookEventType = "new_transaction"
	WebhookNewBlock    WebhookEventType = "new_block"
	WebhookTxConfirmed WebhookEventType = "tx_confirmed"
	WebhookTxRollback  WebhookEventType = "tx_rollback"
	WebhookReorg       WebhookEventType = "chain_reorg"
)

// WebhookRegistration stores a registered webhook endpoint configuration.
type WebhookRegistration struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Secret    string   `json:"-"` // HMAC secret, never serialized in responses
	Events    []string `json:"events"`
	CreatedAt int64    `json:"created_at"`
	Active    bool     `json:"active"`
}

// WebhookPayload is the standardized webhook delivery envelope.
type WebhookPayload struct {
	ID        string           `json:"id"`
	Event     WebhookEventType `json:"event"`
	Timestamp int64            `json:"timestamp"`
	Data      json.RawMessage  `json:"data"`
	Attempt   int              `json:"attempt,omitempty"`
}

// queuedWebhookEvent represents a pending delivery stored in BoltDB.
type queuedWebhookEvent struct {
	ID           string           `json:"id"`
	HookID       string           `json:"hook_id"`
	URL          string           `json:"url"`
	Secret       string           `json:"secret"`
	Payload      WebhookPayload   `json:"payload"`
	NextAttempt  int64            `json:"next_attempt"`
	AttemptCount int              `json:"attempt_count"`
	CreatedAt    int64            `json:"created_at"`
}

// WebhookManager manages webhook subscriptions, delivery, and retry logic.
type WebhookManager struct {
	mu      sync.RWMutex
	hooks   map[string]*WebhookRegistration
	queueDB *bolt.DB
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewWebhookManager creates a new webhook manager with BoltDB-backed queue.
func NewWebhookManager(dataDir string) (*WebhookManager, error) {
	if dataDir == "" {
		dataDir = os.TempDir()
	}
	dbPath := filepath.Join(dataDir, webhookQueueFile)
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open webhook db: %w", err)
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(webhookDBBucket))
		return err
	}); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("webhook: close db after init error: %v", closeErr)
		}
		return nil, fmt.Errorf("create webhook bucket: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	wm := &WebhookManager{
		hooks:   make(map[string]*WebhookRegistration),
		queueDB: db,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Start background delivery workers.
	for i := 0; i < webhookWorkerCount; i++ {
		wm.wg.Add(1)
		go wm.deliveryWorker()
	}

	return wm, nil
}

// Close gracefully shuts down the webhook manager.
func (wm *WebhookManager) Close() error {
	wm.cancel()
	wm.wg.Wait()
	return wm.queueDB.Close()
}

// RegisterWebhook registers a new webhook endpoint.
func (wm *WebhookManager) RegisterWebhook(url, secret string, events []string) (*WebhookRegistration, error) {
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("at least one event type is required")
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	if len(wm.hooks) >= maxWebhookSubscriptions {
		return nil, fmt.Errorf("max %d subscriptions reached", maxWebhookSubscriptions)
	}

	id := generateWebhookID()
	reg := &WebhookRegistration{
		ID:        id,
		URL:       url,
		Secret:    secret,
		Events:    events,
		CreatedAt: time.Now().Unix(),
		Active:    true,
	}
	wm.hooks[id] = reg
	log.Printf("webhook: registered id=%s url=%s events=%v", id, url, events)
	return reg, nil
}

// UnregisterWebhook removes a webhook subscription.
func (wm *WebhookManager) UnregisterWebhook(id string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	if _, ok := wm.hooks[id]; !ok {
		return fmt.Errorf("webhook not found: %s", id)
	}
	delete(wm.hooks, id)
	log.Printf("webhook: unregistered id=%s", id)
	return nil
}

// ListWebhooks returns all registered webhooks.
func (wm *WebhookManager) ListWebhooks() []*WebhookRegistration {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	result := make([]*WebhookRegistration, 0, len(wm.hooks))
	for _, h := range wm.hooks {
		result = append(result, h)
	}
	return result
}

// PublishEvent queues webhook events for all matching subscriptions.
func (wm *WebhookManager) PublishEvent(event WebhookEventType, data json.RawMessage) {
	wm.mu.RLock()
	matching := make([]*WebhookRegistration, 0)
	for _, h := range wm.hooks {
		if !h.Active {
			continue
		}
		for _, e := range h.Events {
			if e == string(event) {
				matching = append(matching, h)
				break
			}
		}
	}
	wm.mu.RUnlock()

	now := time.Now().Unix()
	for _, hook := range matching {
		payload := WebhookPayload{
			ID:        generateWebhookEventID(),
			Event:     event,
			Timestamp: now,
			Data:      data,
		}

		queued := queuedWebhookEvent{
			ID:           payload.ID,
			HookID:       hook.ID,
			URL:          hook.URL,
			Secret:       hook.Secret,
			Payload:      payload,
			NextAttempt:  now,
			AttemptCount: 0,
			CreatedAt:    now,
		}

		if err := wm.enqueue(queued); err != nil {
			log.Printf("webhook: failed to enqueue event %s for hook %s: %v", event, hook.ID, err)
		}
	}
}

// enqueue persists a webhook event to the BoltDB queue.
func (wm *WebhookManager) enqueue(event queuedWebhookEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return wm.queueDB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(webhookDBBucket))
		return b.Put([]byte(event.ID), data)
	})
}

// dequeue removes a successfully delivered event from the queue.
func (wm *WebhookManager) dequeue(id string) error {
	return wm.queueDB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(webhookDBBucket))
		return b.Delete([]byte(id))
	})
}

// requeue updates an event for the next retry attempt.
func (wm *WebhookManager) requeue(event queuedWebhookEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal retry event: %w", err)
	}
	return wm.queueDB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(webhookDBBucket))
		return b.Put([]byte(event.ID), data)
	})
}

// deliveryWorker processes webhook events from the queue with exponential backoff.
func (wm *WebhookManager) deliveryWorker() {
	defer wm.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wm.ctx.Done():
			return
		case <-ticker.C:
			wm.processQueue()
		}
	}
}

// processQueue iterates over pending events and attempts delivery.
func (wm *WebhookManager) processQueue() {
	var pending []queuedWebhookEvent
	now := time.Now().Unix()

	if err := wm.queueDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(webhookDBBucket))
		return b.ForEach(func(k, v []byte) error {
			var event queuedWebhookEvent
			if err := json.Unmarshal(v, &event); err != nil {
				return nil // skip malformed entries
			}
			if event.NextAttempt <= now {
				pending = append(pending, event)
			}
			return nil
		})
	}); err != nil {
		log.Printf("webhook: queue scan error: %v", err)
		return
	}

	for _, event := range pending {
		if err := wm.deliver(event); err != nil {
			event.AttemptCount++
			if event.AttemptCount >= webhookMaxRetries {
				log.Printf("webhook: max retries reached for event %s, discarding", event.ID)
				if delErr := wm.dequeue(event.ID); delErr != nil {
					log.Printf("webhook: failed to dequeue exhausted event %s: %v", event.ID, delErr)
				}
				continue
			}
			// Exponential backoff with jitter: base * 2^attempt + random jitter.
			backoff := float64(webhookBaseDelay) * math.Pow(2, float64(event.AttemptCount))
			jitter := time.Duration(backoff*0.25) * time.Duration(randDurationFactor())
			delay := time.Duration(backoff) + jitter
			if delay > webhookMaxDelay {
				delay = webhookMaxDelay
			}
			event.NextAttempt = time.Now().Add(delay).Unix()
			if reqErr := wm.requeue(event); reqErr != nil {
				log.Printf("webhook: requeue error for event %s: %v", event.ID, reqErr)
			}
			log.Printf("webhook: delivery failed for event %s (attempt %d/%d), retry in %v", event.ID, event.AttemptCount, webhookMaxRetries, delay)
		} else {
			if delErr := wm.dequeue(event.ID); delErr != nil {
				log.Printf("webhook: dequeue error for event %s: %v", event.ID, delErr)
			}
		}
	}
}

// deliver sends a single webhook event with HMAC-SHA256 signing.
func (wm *WebhookManager) deliver(event queuedWebhookEvent) error {
	payload := event.Payload
	payload.Attempt = event.AttemptCount + 1
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(wm.ctx, http.MethodPost, event.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NogoChain-Webhook/1.0")
	req.Header.Set("X-Webhook-ID", event.ID)
	req.Header.Set("X-Webhook-Event", string(payload.Event))
	req.Header.Set("X-Webhook-Attempt", fmt.Sprintf("%d", payload.Attempt))

	// Compute HMAC-SHA256 signature if secret is configured.
	if event.Secret != "" {
		mac := hmac.New(sha256.New, []byte(event.Secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", "sha256="+sig)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("webhook: response body close error: %v", closeErr)
		}
	}()
	// Drain the body to allow connection reuse.
	if _, drainErr := io.Copy(io.Discard, resp.Body); drainErr != nil {
		log.Printf("webhook: response body drain error: %v", drainErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// randDurationFactor returns a random factor between 0 and 1 for jitter.
func randDurationFactor() float64 {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0.5 // fallback
	}
	return float64(b[0])/256.0 + float64(b[1])/65536.0
}

// generateWebhookID generates a unique webhook registration ID.
func generateWebhookID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		log.Printf("webhook: random ID generation failed, using fallback: %v", err)
	}
	return "wh_" + hex.EncodeToString(b[:])
}

// generateWebhookEventID generates a unique webhook event delivery ID.
func generateWebhookEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		log.Printf("webhook: random event ID generation failed, using fallback: %v", err)
	}
	return "whev_" + hex.EncodeToString(b[:])
}

// Webhook HTTP handlers for the API server.

// handleRegisterWebhook handles POST /webhook/register.
func (s *Server) handleRegisterWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.webhookMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "webhook manager not configured"})
		return
	}

	var req struct {
		URL    string   `json:"url"`
		Secret string   `json:"secret"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	reg, err := s.webhookMgr.RegisterWebhook(req.URL, req.Secret, req.Events)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, reg)
}

// handleUnregisterWebhook handles POST /webhook/unregister.
func (s *Server) handleUnregisterWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.webhookMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "webhook manager not configured"})
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if err := s.webhookMgr.UnregisterWebhook(req.ID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "unregistered"})
}

// handleListWebhooks handles GET /webhook/list.
func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.webhookMgr == nil {
		writeJSON(w, http.StatusOK, map[string]any{"webhooks": []any{}})
		return
	}
	hooks := s.webhookMgr.ListWebhooks()
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": hooks})
}
