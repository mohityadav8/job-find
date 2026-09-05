// Package middleware holds cross-cutting HTTP concerns for the API layer:
// rate limiting, response caching and auth. README §7 lists rate limiting and
// caching as production non-negotiables — both protect metered cost drivers
// (job-API quotas, Maps/geocoding usage) from runaway or abusive traffic.
package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter is a per-client-IP token-bucket limiter. Each IP gets its own
// bucket; idle buckets are swept periodically so memory doesn't grow unbounded.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*ipBucket
	rps      rate.Limit
	burst    int
	ttl      time.Duration
	lastSeen map[string]time.Time
}

type ipBucket struct {
	limiter *rate.Limiter
}

// NewRateLimiter builds a limiter allowing rps requests/second with the given
// burst. A background sweeper evicts IPs not seen for 10 minutes.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*ipBucket),
		lastSeen: make(map[string]time.Time),
		rps:      rate.Limit(rps),
		burst:    burst,
		ttl:      10 * time.Minute,
	}
	go rl.sweep()
	return rl
}

func (rl *RateLimiter) sweep() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.ttl)
		for ip, seen := range rl.lastSeen {
			if seen.Before(cutoff) {
				delete(rl.buckets, ip)
				delete(rl.lastSeen, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) limiterFor(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.lastSeen[ip] = time.Now()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &ipBucket{limiter: rate.NewLimiter(rl.rps, rl.burst)}
		rl.buckets[ip] = b
	}
	return b.limiter
}

// Handler is the chi-compatible middleware. Over-limit requests get 429 with a
// Retry-After hint.
func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !rl.limiterFor(ip).Allow() {
			w.Header().Set("Retry-After", "1")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP extracts the best-guess client IP, honoring X-Forwarded-For (set by
// a load balancer / reverse proxy in production) before falling back to the
// socket address.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First entry is the original client.
		if comma := indexByte(xff, ','); comma >= 0 {
			return trimSpace(xff[:comma])
		}
		return trimSpace(xff)
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// tiny local helpers avoid importing strings for two trivial ops.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
