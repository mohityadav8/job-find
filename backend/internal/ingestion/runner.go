package ingestion

import (
	"context"
	"log"

	"github.com/mohityadav8/job-find/backend/internal/db"
	"github.com/mohityadav8/job-find/backend/internal/geocoding"
	"github.com/mohityadav8/job-find/backend/internal/ingestion/sources"
)

// Runner executes one full ingestion pass across all configured sources. It is
// the concrete wiring of the pipeline documented at the top of normalize.go:
//
//	Fetch -> Normalize -> DedupBatch -> Geocode -> DB upsert
//
// It is deliberately decoupled from *how* it's triggered (cron, Asynq task, or
// the one-shot `ingest` CLI) — those just call Run.
type Runner struct {
	store    *db.Store
	geo      *geocoding.Geocoder
	sources  []sources.Source
	perQuery int
}

// NewRunner assembles a Runner. perQuery caps how many postings each source
// returns per query (keeps free-tier API usage bounded — README §6).
func NewRunner(store *db.Store, geo *geocoding.Geocoder, srcs []sources.Source, perQuery int) *Runner {
	if perQuery <= 0 {
		perQuery = 100
	}
	return &Runner{store: store, geo: geo, sources: srcs, perQuery: perQuery}
}

// Result summarizes a run for logging/metrics.
type Result struct {
	Fetched     int
	AfterDedup  int
	Inserted    int
	Skipped     int
	GeocodeFail int
	Errors      []error
}

// Run performs a single ingestion pass for the given queries. Each query is run
// against every source; results are pooled, deduped once across all sources,
// then persisted. Individual source failures are collected, not fatal — one bad
// upstream must never sink the whole run.
func (r *Runner) Run(ctx context.Context, queries []sources.Query) Result {
	var res Result

	// 1. FETCH — gather raw postings from every source × every query.
	var raw []sources.RawJob
	for _, src := range r.sources {
		for _, q := range queries {
			if q.PerPage == 0 {
				q.PerPage = r.perQuery
			}
			batch, err := src.Fetch(ctx, q)
			if err != nil {
				res.Errors = append(res.Errors, err)
				log.Printf("ingest: source %s query %q: %v", src.Name(), q.Keywords, err)
				continue
			}
			raw = append(raw, batch...)
		}
	}
	res.Fetched = len(raw)

	// 2. NORMALIZE.
	normalized := make([]Normalized, 0, len(raw))
	for _, rj := range raw {
		n := Normalize(rj)
		// Drop records too incomplete to be a usable pin.
		if n.CompanyName == "" || n.Job.Title == "" {
			continue
		}
		normalized = append(normalized, n)
	}

	// 3. DEDUP (in-batch).
	normalized = DedupBatch(normalized)
	res.AfterDedup = len(normalized)

	// 4 + 5. GEOCODE and UPSERT, per record.
	for _, n := range normalized {
		if err := r.persist(ctx, n, &res); err != nil {
			res.Errors = append(res.Errors, err)
		}
	}

	log.Printf("ingest run: fetched=%d afterDedup=%d inserted=%d skipped=%d geocodeFail=%d errors=%d",
		res.Fetched, res.AfterDedup, res.Inserted, res.Skipped, res.GeocodeFail, len(res.Errors))
	return res
}

// persist geocodes one record and writes company/office/job.
func (r *Runner) persist(ctx context.Context, n Normalized, res *Result) error {
	// Geocode (cache-first). A geocode failure means we can't place a pin, so
	// the record is skipped rather than stored at (0,0) in the ocean.
	display := geocodeDisplay(n.City, n.Country)
	coord, err := r.geo.Geocode(ctx, n.LocationKey, display)
	if err != nil {
		res.GeocodeFail++
		log.Printf("ingest: geocode %q failed: %v", display, err)
		return nil // non-fatal
	}

	companyID, err := r.store.UpsertCompany(ctx, n.CompanyName, "", n.CompanyLogo)
	if err != nil {
		return err
	}

	city, country := n.City, n.Country
	if city == "" && country == "" {
		country = "Remote"
	}
	officeID, err := r.store.UpsertOffice(ctx, companyID, city, country, "", coord.Lat, coord.Lng)
	if err != nil {
		return err
	}

	job := n.Job
	job.OfficeID = officeID
	inserted, err := r.store.UpsertJob(ctx, job)
	if err != nil {
		return err
	}
	if inserted {
		res.Inserted++
	} else {
		res.Skipped++
	}
	return nil
}

// geocodeDisplay builds the human string sent to the geocoder.
func geocodeDisplay(city, country string) string {
	switch {
	case city != "" && country != "":
		return city + ", " + country
	case country != "":
		return country
	case city != "":
		return city
	default:
		return "Remote"
	}
}

// DefaultQueries is a small seed set of searches used by the scheduled run when
// no explicit queries are supplied. Broad terms maximize coverage from the free
// APIs on each pass.
func DefaultQueries() []sources.Query {
	terms := []string{
		"software engineer", "backend", "frontend", "data scientist",
		"devops", "product manager", "designer", "full stack",
	}
	out := make([]sources.Query, 0, len(terms))
	for _, t := range terms {
		out = append(out, sources.Query{Keywords: t, PerPage: 100})
	}
	return out
}
