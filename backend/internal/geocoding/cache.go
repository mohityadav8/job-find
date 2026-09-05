// Package geocoding turns a location string into coordinates exactly once per
// unique location, then caches the result permanently. This is the single most
// important cost-control measure in the system (README §3/§6/§7): every
// ingestion sync would otherwise re-geocode every job, which is both slow and,
// at scale, expensive.
package geocoding

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Coord is a resolved latitude/longitude pair.
type Coord struct {
	Lat float64
	Lng float64
}

// Cache is a two-tier permanent cache: a process-local map in front of the
// geocode_cache Postgres table. The DB tier survives restarts and is shared
// across every worker/instance; the memory tier avoids a DB round-trip for
// locations already seen in this process.
type Cache struct {
	pool *pgxpool.Pool

	mu  sync.RWMutex
	mem map[string]Coord
}

// NewCache builds a Cache over the given pool.
func NewCache(pool *pgxpool.Pool) *Cache {
	return &Cache{
		pool: pool,
		mem:  make(map[string]Coord),
	}
}

// Get returns a cached coordinate for key, checking memory first, then the DB.
// The bool is false when the key has never been geocoded.
func (c *Cache) Get(ctx context.Context, key string) (Coord, bool, error) {
	// Memory tier.
	c.mu.RLock()
	if coord, ok := c.mem[key]; ok {
		c.mu.RUnlock()
		return coord, true, nil
	}
	c.mu.RUnlock()

	// DB tier.
	const q = `SELECT latitude, longitude FROM geocode_cache WHERE location_key = $1`
	var coord Coord
	err := c.pool.QueryRow(ctx, q, key).Scan(&coord.Lat, &coord.Lng)
	if err != nil {
		// pgx returns ErrNoRows for a miss; treat that as "not cached", not a
		// hard error, so the caller falls through to a live geocode.
		if isNoRows(err) {
			return Coord{}, false, nil
		}
		return Coord{}, false, err
	}

	// Promote into the memory tier for next time.
	c.mu.Lock()
	c.mem[key] = coord
	c.mu.Unlock()
	return coord, true, nil
}

// Put stores a coordinate in both tiers. The DB write is an upsert so
// concurrent workers resolving the same new location don't collide.
func (c *Cache) Put(ctx context.Context, key string, coord Coord) error {
	const q = `
		INSERT INTO geocode_cache (location_key, latitude, longitude)
		VALUES ($1, $2, $3)
		ON CONFLICT (location_key) DO UPDATE
		  SET latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude`
	if _, err := c.pool.Exec(ctx, q, key, coord.Lat, coord.Lng); err != nil {
		return err
	}
	c.mu.Lock()
	c.mem[key] = coord
	c.mu.Unlock()
	return nil
}

// isNoRows reports whether err is pgx's "no rows" sentinel without importing
// the whole pgx error surface at every call site.
func isNoRows(err error) bool {
	return err != nil && err.Error() == "no rows in result set"
}
