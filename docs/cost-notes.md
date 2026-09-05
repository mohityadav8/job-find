# Cost Notes

Operational cost notes to complement README §6. The theme throughout: **caching
is what keeps the metered services cheap.**

## The three metered cost drivers

1. **Map loads (Google Maps JS API)** — only relevant if you swap the frontend
   from MapLibre (free) to Google Maps. Billed per map load once past the
   $200/month Google Cloud credit.
2. **Geocoding** — billed per lookup *only on a cache miss*. The two-tier
   permanent cache (`geocode_cache` table + in-process map) means each unique
   "city, country" is resolved exactly once, ever. Steady-state geocoding spend
   trends to zero even as traffic grows, because traffic doesn't create new
   *locations*.
3. **Job-board API quotas** — free tiers cap daily calls. `INGEST_PER_QUERY`
   and the cron cadence (`INGEST_CRON`) bound how many calls each sync makes.

## What each cache buys

| Cache                        | Where                          | Protects            |
|------------------------------|--------------------------------|---------------------|
| Geocode cache                | Postgres + memory              | Geocoding bill      |
| API response cache           | `middleware/cache.go` (TTL)    | DB load, latency    |
| Rate limiter                 | `middleware/ratelimit.go`      | DB + job-API quotas |

## Free-tier posture (current defaults)

- **MapLibre + CARTO tiles + Nominatim geocoding**: $0, no keys required. This
  is the default the code ships with, so a fresh clone costs nothing to run.
- **Adzuna / Jooble**: optional; enabled only when keys are set. Without them,
  RemoteOK + Remotive still provide real worldwide (remote) data for free.

## Before enabling any Google key

Set a Google Cloud **budget alert** first (README §6 calls this mandatory).
Suggested tripwires: $50 / $150 / $190. Five-minute setup; turns a spend spike
into an email instead of an invoice.

## Rough scaling shape

- Ingestion cost scales with **number of unique locations and sources**, not
  with user traffic (thanks to the geocode cache).
- Serving cost scales with **concurrent users and map interactions**, blunted by
  the response cache TTL.
- DB cost scales with **total jobs stored + query volume**; the PostGIS GIST
  index (viewport) and GIN index (skills) keep query cost sub-linear.
