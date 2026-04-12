package api

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/metrics"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// requestIDFromContext reserved for future use //nolint:unused
func requestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	return h.Hijack()
}

type RouteMiddleware struct {
	adminToken string
	trustProxy bool
	limiter    *IPRateLimiter
	metrics    *metrics.Metrics
}

func (mw *RouteMiddleware) Wrap(route string, admin bool, maxBodyBytes int64, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Set CORS headers for all responses (wallet compatibility)
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With, X-Request-ID, X-Relay-Hops")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Handle preflight OPTIONS request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		reqID := newRequestID()
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", reqID)

		if mw.limiter != nil {
			ip := clientIP(r, mw.trustProxy)
			if ok, retryAfter := mw.limiter.Allow(ip); !ok {
				if retryAfter > 0 {
					w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
				}
				writeJSON(w, http.StatusTooManyRequests, map[string]any{
					"error":     "rate_limited",
					"message":   "too many requests",
					"requestId": reqID,
				})
				mw.observe(route, http.StatusTooManyRequests, start)
				return
			}
		}

		if admin {
			if mw.adminToken == "" {
				writeJSON(w, http.StatusForbidden, map[string]any{
					"error":     "admin_disabled",
					"message":   "admin endpoints are disabled (set ADMIN_TOKEN to enable)",
					"requestId": reqID,
				})
				mw.observe(route, http.StatusForbidden, start)
				return
			}
			if !hasBearerToken(r, mw.adminToken) {
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"error":     "unauthorized",
					"message":   "missing or invalid admin token",
					"requestId": reqID,
				})
				mw.observe(route, http.StatusUnauthorized, start)
				return
			}
		}

		if maxBodyBytes > 0 && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h(rec, r)
		mw.observe(route, rec.status, start)
	}
}

func (mw *RouteMiddleware) observe(route string, status int, start time.Time) {
	// Metrics observation disabled in simple server
}

func hasBearerToken(r *http.Request, expected string) bool {
	if expected == "" {
		return true
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	return got == expected
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	// fallback: time-based (still unique enough for debugging)
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
		if xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				ip := strings.TrimSpace(parts[0])
				if net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

type IPRateLimiter struct {
	mu sync.Mutex

	rps   float64
	burst float64
	ttl   time.Duration

	buckets map[string]*ipBucket

	lastSweep time.Time
}

type ipBucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

func NewIPRateLimiter(rps int, burst int) *IPRateLimiter {
	if rps <= 0 || burst <= 0 {
		return nil
	}
	return &IPRateLimiter{
		rps:       float64(rps),
		burst:     float64(burst),
		ttl:       10 * time.Minute,
		buckets:   map[string]*ipBucket{},
		lastSweep: time.Now(),
	}
}

func (l *IPRateLimiter) Allow(ip string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	if ip == "" {
		ip = "unknown"
	}
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.buckets[ip]
	if b == nil {
		b = &ipBucket{tokens: l.burst, last: now, lastSeen: now}
		l.buckets[ip] = b
	}

	// refill
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.rps
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.last = now
	}
	b.lastSeen = now

	if b.tokens >= 1 {
		b.tokens -= 1
		l.sweepLocked(now)
		return true, 0
	}

	need := 1 - b.tokens
	sec := need / l.rps
	if sec < 0 {
		sec = 0
	}
	l.sweepLocked(now)
	return false, time.Duration(sec * float64(time.Second))
}

func (l *IPRateLimiter) sweepLocked(now time.Time) {
	// Opportunistic cleanup.
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	cutoff := now.Add(-l.ttl)
	for ip, b := range l.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(l.buckets, ip)
		}
	}
}
