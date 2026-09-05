package geocoding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Geocoder resolves a "city, country" string to coordinates, always consulting
// the permanent Cache first. Only genuine cache misses ever hit an external
// provider, which is what keeps geocoding spend near zero (README §6).
//
// Provider strategy:
//   - If a Google Maps API key is configured, use the Google Geocoding API
//     (best accuracy, metered).
//   - Otherwise fall back to OpenStreetMap Nominatim (free, rate-limited to
//     ~1 req/s by their usage policy — we enforce that with a limiter).
type Geocoder struct {
	cache      *Cache
	googleKey  string
	client     *http.Client
	limiter    *rate.Limiter
	singledoer *singleflightGroup // collapses concurrent lookups of the same key
}

// NewGeocoder builds a Geocoder. googleKey may be empty, in which case Nominatim
// is used. httpClient may be nil.
func NewGeocoder(cache *Cache, googleKey string, httpClient *http.Client) *Geocoder {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	// Nominatim's policy is 1 req/s. Google tolerates far more, but a modest
	// cap protects our own quota/bill regardless of provider.
	lim := rate.NewLimiter(rate.Every(time.Second), 1)
	return &Geocoder{
		cache:      cache,
		googleKey:  googleKey,
		client:     httpClient,
		limiter:    lim,
		singledoer: &singleflightGroup{calls: map[string]*singleflightCall{}},
	}
}

// Geocode returns coordinates for the given location key + human display string.
// key is the canonical cache key (from ingestion.LocationKey); display is the
// human string actually sent to the provider (e.g. "Bengaluru, India").
func (g *Geocoder) Geocode(ctx context.Context, key, display string) (Coord, error) {
	// 1. Cache.
	if coord, ok, err := g.cache.Get(ctx, key); err != nil {
		return Coord{}, err
	} else if ok {
		return coord, nil
	}

	// 2. Collapse concurrent misses for the same key into one live lookup.
	coord, err := g.singledoer.Do(key, func() (Coord, error) {
		// Re-check the cache inside the singleflight in case another goroutine
		// resolved it while we queued.
		if c, ok, cerr := g.cache.Get(ctx, key); cerr == nil && ok {
			return c, nil
		}

		c, err := g.lookup(ctx, display)
		if err != nil {
			return Coord{}, err
		}
		// 3. Persist permanently.
		if err := g.cache.Put(ctx, key, c); err != nil {
			// A cache-write failure shouldn't fail the geocode itself; the
			// coordinate is still valid, we'll just re-resolve next time.
			return c, nil
		}
		return c, nil
	})
	return coord, err
}

// lookup performs the live external call, honoring the rate limiter.
func (g *Geocoder) lookup(ctx context.Context, display string) (Coord, error) {
	if strings.TrimSpace(display) == "" {
		return Coord{}, fmt.Errorf("geocode: empty location")
	}
	if err := g.limiter.Wait(ctx); err != nil {
		return Coord{}, err
	}
	if g.googleKey != "" {
		return g.lookupGoogle(ctx, display)
	}
	return g.lookupNominatim(ctx, display)
}

type googleGeocodeResponse struct {
	Status  string `json:"status"`
	Results []struct {
		Geometry struct {
			Location struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"location"`
		} `json:"geometry"`
	} `json:"results"`
	ErrorMessage string `json:"error_message"`
}

func (g *Geocoder) lookupGoogle(ctx context.Context, display string) (Coord, error) {
	params := url.Values{}
	params.Set("address", display)
	params.Set("key", g.googleKey)
	endpoint := "https://maps.googleapis.com/maps/api/geocode/json?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Coord{}, fmt.Errorf("geocode(google): building request: %w", err)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return Coord{}, fmt.Errorf("geocode(google): request failed: %w", err)
	}
	defer resp.Body.Close()

	var body googleGeocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Coord{}, fmt.Errorf("geocode(google): decode: %w", err)
	}
	if body.Status != "OK" || len(body.Results) == 0 {
		return Coord{}, fmt.Errorf("geocode(google): status %q for %q: %s", body.Status, display, body.ErrorMessage)
	}
	loc := body.Results[0].Geometry.Location
	return Coord{Lat: loc.Lat, Lng: loc.Lng}, nil
}

type nominatimResult struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

func (g *Geocoder) lookupNominatim(ctx context.Context, display string) (Coord, error) {
	params := url.Values{}
	params.Set("q", display)
	params.Set("format", "json")
	params.Set("limit", "1")
	endpoint := "https://nominatim.openstreetmap.org/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Coord{}, fmt.Errorf("geocode(nominatim): building request: %w", err)
	}
	// Nominatim requires a descriptive User-Agent identifying the application.
	req.Header.Set("User-Agent", "job-find/1.0 (+https://job-find.xyz)")

	resp, err := g.client.Do(req)
	if err != nil {
		return Coord{}, fmt.Errorf("geocode(nominatim): request failed: %w", err)
	}
	defer resp.Body.Close()

	var results []nominatimResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return Coord{}, fmt.Errorf("geocode(nominatim): decode: %w", err)
	}
	if len(results) == 0 {
		return Coord{}, fmt.Errorf("geocode(nominatim): no result for %q", display)
	}
	var c Coord
	if _, err := fmt.Sscanf(results[0].Lat, "%f", &c.Lat); err != nil {
		return Coord{}, fmt.Errorf("geocode(nominatim): bad lat %q", results[0].Lat)
	}
	if _, err := fmt.Sscanf(results[0].Lon, "%f", &c.Lng); err != nil {
		return Coord{}, fmt.Errorf("geocode(nominatim): bad lon %q", results[0].Lon)
	}
	return c, nil
}

// ---- minimal singleflight (avoids an extra dependency) ---------------------
//
// We only need "collapse concurrent calls with the same key into one"; the
// stdlib doesn't ship this and pulling golang.org/x/sync/singleflight would add
// another x/* replace to manage, so a tiny local version is used instead.

type singleflightCall struct {
	wg  sync.WaitGroup
	val Coord
	err error
}

type singleflightGroup struct {
	mu    sync.Mutex
	calls map[string]*singleflightCall
}

func (g *singleflightGroup) Do(key string, fn func() (Coord, error)) (Coord, error) {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := &singleflightCall{}
	c.wg.Add(1)
	g.calls[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	return c.val, c.err
}
