package ingestion

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// Dedup exists because the same opening routinely appears on several job boards
// at once (README §2/§7). Without a stable fingerprint we'd show the same role
// twice under one office. Two layers guard against that:
//
//  1. DedupHash — a deterministic fingerprint stored UNIQUE in the jobs table,
//     so even across separate ingestion runs the DB rejects a re-insert.
//  2. DedupBatch — an in-memory pass that collapses duplicates *within* a single
//     fetch before they ever reach the DB, saving pointless upsert round-trips.

var dedupNonAlnumRE = regexp.MustCompile(`[^a-z0-9]+`)

// DedupHash builds the fingerprint from the fields that identify a role
// independent of which board it came from: company, title and location. All are
// aggressively normalized (lower-cased, punctuation-stripped, whitespace
// collapsed) so "Sr. Engineer" and "senior engineer" at the same company/city
// collide as intended.
func DedupHash(company, title, city, country string) string {
	parts := []string{
		canonicalToken(company),
		canonicalToken(title),
		canonicalToken(city),
		canonicalToken(country),
	}
	joined := strings.Join(parts, "\x1f") // unit separator, can't occur in input
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}

// canonicalToken lower-cases, replaces any run of non-alphanumerics with a
// single space, and trims. This is what makes the hash resilient to cosmetic
// differences between sources.
func canonicalToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = dedupNonAlnumRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// DedupBatch removes duplicates from a slice of Normalized records in a single
// pass, keeping the first occurrence of each dedup hash. Order is preserved.
func DedupBatch(in []Normalized) []Normalized {
	seen := make(map[string]struct{}, len(in))
	out := in[:0:0] // new backing array; don't mutate caller's slice
	for _, n := range in {
		h := n.Job.DedupHash
		if h == "" {
			// Shouldn't happen (Normalize always sets it) but be defensive:
			// keep records without a hash rather than silently dropping them.
			out = append(out, n)
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, n)
	}
	return out
}
