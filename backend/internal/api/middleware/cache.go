package middleware

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Cache is a tiny in-memory TTL cache for idempotent GET responses. Per README
// §7, caching is a non-negotiable because /pins responses are expensive to
// build (PostGIS aggregation) and are hit repeatedly as users pan/zoom the map.
//
// This is intentionally process-local. For multi-instance production you'd put
// Redis or a CDN in front; the interface here stays the same either way.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	status    int
	body      []byte
	header    http.Header
	expiresAt time.Time
}

// NewCache builds a response cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
	go c.sweep()
	return c
}

func (c *Cache) sweep() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for k, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}

// Handler caches GET responses keyed on method+path+normalised-query.
// Non-GET requests and non-2xx responses bypass the cache entirely.
func (c *Cache) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		key := cacheKey(r)

		// Serve from cache on hit.
		c.mu.RLock()
		e, ok := c.entries[key]
		c.mu.RUnlock()
		if ok && time.Now().Before(e.expiresAt) {
			for k, vals := range e.header {
				for _, v := range vals {
					w.Header().Add(k, v)
				}
			}
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(e.status)
			_, _ = w.Write(e.body)
			return
		}

		// Miss: capture the response as it's written.
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK, buf: &bytes.Buffer{}}
		rec.Header().Set("X-Cache", "MISS")
		next.ServeHTTP(rec, r)

		// Only cache successful responses.
		if rec.status >= 200 && rec.status < 300 {
			hdr := make(http.Header, len(rec.Header()))
			for k, vals := range rec.Header() {
				if k == "X-Cache" {
					continue
				}
				hdr[k] = append([]string(nil), vals...)
			}
			c.mu.Lock()
			c.entries[key] = cacheEntry{
				status:    rec.status,
				body:      append([]byte(nil), rec.buf.Bytes()...),
				header:    hdr,
				expiresAt: time.Now().Add(c.ttl),
			}
			c.mu.Unlock()
		}
	})
}

// cacheKey hashes method + path + normalised query. The query is normalised
// so that bbox coordinates are rounded to 1 decimal place (~11 km grid) and
// keys are sorted — this prevents trivial map pans from always missing the
// cache.
func cacheKey(r *http.Request) string {
	h := sha1.New()
	h.Write([]byte(r.Method))
	h.Write([]byte{0})
	h.Write([]byte(r.URL.Path))
	h.Write([]byte{0})
	h.Write([]byte(normalisedQuery(r.URL.Query())))
	return hex.EncodeToString(h.Sum(nil))
}

// normalisedQuery rebuilds the query string with bbox coordinates rounded to
// 1 decimal place so nearby map viewports share the same cache entry instead
// of always missing. All other params are passed through unchanged. Keys are
// sorted via url.Values.Encode() so param order never affects the cache key.
func normalisedQuery(q url.Values) string {
	if bbox := q.Get("bbox"); bbox != "" {
		parts := strings.Split(bbox, ",")
		if len(parts) == 4 {
			rounded := make([]string, 4)
			ok := true
			for i, p := range parts {
				f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
				if err != nil {
					ok = false
					break
				}
				// Round to 1 decimal place: 0.1 degree ≈ 11 km.
				rounded[i] = strconv.FormatFloat(math.Round(f*10)/10, 'f', 1, 64)
			}
			if ok {
				q = cloneValues(q)
				q.Set("bbox", strings.Join(rounded, ","))
			}
		}
	}
	return q.Encode() // Encode sorts keys alphabetically.
}

// cloneValues shallow-copies a url.Values so we can mutate bbox without
// touching the original request.
func cloneValues(q url.Values) url.Values {
	out := make(url.Values, len(q))
	for k, v := range q {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// responseRecorder tees the handler's output into a buffer while still writing
// through to the real client.
type responseRecorder struct {
	http.ResponseWriter
	status      int
	buf         *bytes.Buffer
	wroteHeader bool
}

func (rr *responseRecorder) WriteHeader(code int) {
	if rr.wroteHeader {
		return
	}
	rr.status = code
	rr.wroteHeader = true
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if !rr.wroteHeader {
		rr.WriteHeader(http.StatusOK)
	}
	rr.buf.Write(b)
	return rr.ResponseWriter.Write(b)
}
