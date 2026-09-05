# Architecture

This document describes how the pieces of job-find fit together in practice.
For the *why* behind these choices, see the root `Readme.md`; this is the
implementation-level map.

## Processes

The system runs as three deployable processes plus two backing stores:

| Process    | Binary / image        | Responsibility                                   |
|------------|-----------------------|--------------------------------------------------|
| API        | `cmd/api`             | Serves the frontend's HTTP requests only.        |
| Ingestion  | `cmd/ingest`          | Pulls job boards, normalizes, geocodes, writes.  |
| Frontend   | Next.js               | Map UI + filter sidebar + company dashboard.     |
| Postgres   | `postgis/postgis`     | Geo-indexed storage.                             |
| Redis      | `redis`               | Asynq broker for the background queue.           |

Keeping API and ingestion as **separate processes** is the concrete form of the
README's "background job queue for ingestion" non-negotiable: a heavy sync
never competes with user traffic for CPU or a DB connection.

## Request path (read)

```
Browser ──GET /api/pins?bbox=…&skill=…──▶ API
                                           │  rate limit → response cache → handler
                                           ▼
                                     Postgres/PostGIS
                                       (ST_Intersects on the viewport,
                                        GIN match on skills, agg job_count)
                                           │
                                           ▼
                                     [ ]Pin  ──JSON──▶ Browser ──▶ MapLibre source
```

A pin click issues `GET /api/offices/{id}` (same filters) to fetch the full job
list for that one office — the map payload itself stays small.

## Ingestion path (write)

```
cron (Asynq scheduler) ──enqueue "ingest:sync"──▶ Redis
                                                    │
                                    Asynq worker dequeues
                                                    ▼
   sources.Fetch ─▶ Normalize ─▶ DedupBatch ─▶ Geocode(cache) ─▶ Upsert
   (Adzuna, Jooble,   (clean,       (drop        (1 lookup per    (company →
    Remotive,          classify,     in-batch     unique location   office →
    RemoteOK)          mine skills)  dupes)       ever)             job)
```

Cross-run dedup is additionally enforced at the DB by a `UNIQUE (dedup_hash)`
constraint on `jobs`, so even a duplicate that slips past the in-batch pass is
rejected on insert.

## Data model

```
companies ──1:N──▶ offices ──1:N──▶ jobs
                      │                 └── dedup_hash UNIQUE
                      └── location GEOGRAPHY(Point,4326)   ← the map pin
geocode_cache (location_key PK)   ← permanent "string → lat/lng" cache
users ──N:1──▶ companies          ← dashboard accounts (Phase 2)
schema_migrations                 ← applied-migration tracking
```

## Key modules (backend)

- `internal/ingestion/sources` — one adapter per job board; all emit `RawJob`.
- `internal/ingestion/normalize.go` — pure `RawJob → Normalized` transform.
- `internal/ingestion/dedup.go` — fingerprinting + in-batch suppression.
- `internal/geocoding` — two-tier permanent cache + Google/Nominatim providers.
- `internal/db/queries.go` — every SQL statement, as typed `Store` methods.
- `internal/api` — chi router, handlers, and middleware (auth/cache/ratelimit).

## Swapping the map provider

The frontend uses MapLibre GL so it runs with no API key. The README specifies
Google Maps; that swap is isolated to `components/map/MapView.tsx` — the pins
data flow, clustering config (`MarkerCluster.tsx`) and detail drawer are
provider-agnostic.
