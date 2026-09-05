// Package sources defines the contract every job-board integration implements,
// plus the neutral RawJob shape they all emit.
//
// Design note (README §3/§4): each external API has its own payload format.
// Rather than let those formats leak into the rest of the system, every source
// maps its response into a single RawJob struct. Everything downstream — the
// normalizer, deduper, geocoder and DB writer — only ever sees RawJob, so
// adding a new source (Phase 3 scraping included) never touches ingestion core.
package sources

import "context"

// RawJob is the lowest-common-denominator representation of a job posting as it
// comes off an external source, *before* normalization, dedup and geocoding.
//
// Fields are intentionally plain strings where the upstream data is messy
// (LocationRaw, ExperienceRaw, WorkTypeRaw) — cleaning them up is the
// normalizer's job, not the source adapter's.
type RawJob struct {
	// Identity / provenance
	Source    string // "adzuna" | "jooble" | "remotive" | "remoteok"
	SourceID  string // the source's own ID for this posting, if any
	SourceURL string // canonical link back to the posting

	// Company + location (still raw text; geocoding happens later)
	CompanyName string
	CompanyLogo string
	LocationRaw string // e.g. "Bengaluru, India" or "Remote"
	City        string // best-effort split; may be empty
	Country     string // best-effort split; may be empty

	// Job content
	Title         string
	Description   string
	SkillsRaw     []string // may be empty; normalizer also mines the description
	ExperienceRaw string   // free text, e.g. "Senior", "3+ years"
	WorkTypeRaw   string   // free text, e.g. "Full remote", "On-site", "Hybrid"

	// Salary (any/all may be zero/empty when the source omits it)
	SalaryMin      int
	SalaryMax      int
	SalaryCurrency string

	// Timing
	PostedAt string // source-formatted date string; normalizer parses it
}

// Source is one job-board integration. Fetch pulls a batch of raw postings for
// the given query. Implementations must respect ctx cancellation/timeouts so a
// slow upstream can't wedge the whole ingestion run.
type Source interface {
	// Name returns the stable source identifier ("adzuna", ...). It must match
	// the Source field the adapter stamps onto every RawJob it returns.
	Name() string

	// Fetch retrieves postings matching q. A source with no credentials
	// configured should return (nil, nil) rather than an error, so the
	// scheduler can simply skip it.
	Fetch(ctx context.Context, q Query) ([]RawJob, error)
}

// Query parameters shared across sources. Not every source honours every field;
// each adapter maps what it can and ignores the rest.
type Query struct {
	Keywords string // free-text search, e.g. "backend engineer"
	Location string // e.g. "India"; empty means "wherever the source defaults"
	Page     int    // 1-based
	PerPage  int    // upper bound on results; sources may return fewer
}
