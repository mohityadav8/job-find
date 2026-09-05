// Package ingestion contains the core pipeline that turns raw job-board
// payloads into the Company/Office/Job shape stored in Postgres.
//
// Pipeline order (README §3):
//
//	source.Fetch  ->  Normalize  ->  Dedup  ->  Geocode  ->  DB upsert
//
// The normalizer is pure and side-effect-free: no network, no DB. That makes it
// trivially unit-testable (see tests/normalize_test.go) and keeps all the messy
// "clean up whatever the source gave us" logic in one place.
package ingestion

import (
	"regexp"
	"strings"
	"time"

	"github.com/mohityadav8/job-find/backend/internal/ingestion/sources"
	"github.com/mohityadav8/job-find/backend/internal/models"
)

// Normalized is the cleaned, structured result of processing one RawJob. It
// carries the three-entity shape plus the location key used for geocoding and
// the dedup hash used to suppress cross-source duplicates.
type Normalized struct {
	CompanyName string
	CompanyLogo string

	City        string
	Country     string
	LocationKey string // canonical key for the geocode cache, e.g. "bengaluru|india"

	Job models.Job // Skills, WorkType, salary, etc. already cleaned
}

// knownSkills is a small dictionary the normalizer uses to mine skills out of a
// free-text description when a source doesn't provide structured tags. It's
// intentionally not exhaustive — it's a pragmatic booster, not an NLP system.
var knownSkills = []string{
	"go", "golang", "python", "java", "javascript", "typescript", "react",
	"next.js", "node.js", "node", "vue", "angular", "svelte", "rust", "c++",
	"c#", ".net", "ruby", "rails", "php", "laravel", "django", "flask",
	"fastapi", "spring", "kotlin", "swift", "objective-c", "scala", "elixir",
	"postgres", "postgresql", "mysql", "mongodb", "redis", "elasticsearch",
	"kafka", "rabbitmq", "graphql", "rest", "grpc", "docker", "kubernetes",
	"k8s", "terraform", "ansible", "aws", "gcp", "azure", "ci/cd", "jenkins",
	"github actions", "linux", "bash", "sql", "nosql", "spark", "hadoop",
	"tensorflow", "pytorch", "pandas", "numpy", "machine learning", "ml",
	"data science", "html", "css", "tailwind", "sass", "figma", "flutter",
	"react native", "android", "ios",
}

var (
	// Strips HTML tags from source descriptions before skill mining.
	htmlTagRE = regexp.MustCompile(`<[^>]*>`)
	// Word-boundary matcher builder is done per-skill at match time.
	nonAlnumRE = regexp.MustCompile(`[^a-z0-9+#.]+`)
	// Common seniority markers, richest first so "senior" wins over "mid".
	seniorityRE = regexp.MustCompile(`(?i)\b(principal|staff|lead|senior|sr\.?|mid[- ]?level|mid|junior|jr\.?|intern|entry[- ]?level|graduate)\b`)
)

// Normalize converts a single RawJob into a Normalized record. It never errors:
// bad/missing fields degrade to sensible zero values rather than dropping the
// whole posting, because a job with a fuzzy location is still more useful than
// no job. The caller decides whether to drop records with, e.g., no company.
func Normalize(raw sources.RawJob) Normalized {
	companyName := cleanCompany(raw.CompanyName)
	city := strings.TrimSpace(raw.City)
	country := strings.TrimSpace(raw.Country)

	// Fall back to parsing the raw location string if the source didn't split it.
	if city == "" && country == "" && raw.LocationRaw != "" {
		city, country = parseLocation(raw.LocationRaw)
	}

	descText := stripHTML(raw.Description)

	n := Normalized{
		CompanyName: companyName,
		CompanyLogo: strings.TrimSpace(raw.CompanyLogo),
		City:        city,
		Country:     country,
		LocationKey: LocationKey(city, country),
		Job: models.Job{
			Title:           cleanTitle(raw.Title),
			Skills:          normalizeSkills(raw.SkillsRaw, descText),
			ExperienceLevel: classifyExperience(raw.Title, raw.ExperienceRaw, descText),
			WorkType:        classifyWorkType(raw.WorkTypeRaw, raw.LocationRaw, descText),
			SalaryMin:       nilIfZero(raw.SalaryMin),
			SalaryMax:       nilIfZero(raw.SalaryMax),
			SalaryCurrency:  strings.ToUpper(strings.TrimSpace(raw.SalaryCurrency)),
			PostedDate:      parseDate(raw.PostedAt),
			Source:          raw.Source,
			SourceURL:       raw.SourceURL,
		},
	}
	// The dedup hash is computed once here from normalized (not raw) fields so
	// the same posting from two sources produces the same fingerprint.
	n.Job.DedupHash = DedupHash(companyName, n.Job.Title, city, country)
	return n
}

// LocationKey builds the canonical, lower-cased geocode-cache key. Using a
// stable key here is what guarantees "one geocode per unique location, ever"
// (README §6): every job in Bengaluru hashes to the same key and hits the cache.
func LocationKey(city, country string) string {
	c := strings.ToLower(strings.TrimSpace(city))
	k := strings.ToLower(strings.TrimSpace(country))
	switch {
	case c == "" && k == "":
		return "remote"
	case c == "":
		return k
	case k == "":
		return c
	default:
		return c + "|" + k
	}
}

func cleanCompany(s string) string {
	s = strings.TrimSpace(s)
	// Collapse internal whitespace.
	return strings.Join(strings.Fields(s), " ")
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func stripHTML(s string) string {
	return htmlTagRE.ReplaceAllString(s, " ")
}

// normalizeSkills de-dupes and lower-cases source tags, then augments them with
// any known skills found in the description text.
func normalizeSkills(tags []string, descText string) []string {
	seen := map[string]struct{}{}
	var out []string

	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.Trim(s, ".,;:")
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	for _, t := range tags {
		add(t)
	}

	if descText != "" {
		lowered := strings.ToLower(descText)
		// Tokenize once for word-boundary-ish matching.
		tokens := nonAlnumRE.Split(lowered, -1)
		tokenSet := make(map[string]struct{}, len(tokens))
		for _, tk := range tokens {
			if tk != "" {
				tokenSet[tk] = struct{}{}
			}
		}
		for _, sk := range knownSkills {
			if strings.Contains(sk, " ") {
				// multi-word skill: substring match
				if strings.Contains(lowered, sk) {
					add(sk)
				}
			} else if _, ok := tokenSet[sk]; ok {
				add(sk)
			}
		}
	}

	if out == nil {
		out = []string{}
	}
	return out
}

// classifyExperience picks a coarse seniority bucket from the title first
// (most reliable), then the explicit experience field, then the description.
func classifyExperience(title, expRaw, descText string) string {
	for _, src := range []string{title, expRaw, descText} {
		if m := seniorityRE.FindString(src); m != "" {
			return canonicalSeniority(m)
		}
	}
	return ""
}

func canonicalSeniority(m string) string {
	m = strings.ToLower(strings.TrimSpace(m))
	m = strings.TrimSuffix(m, ".")
	switch m {
	case "sr":
		return "senior"
	case "jr":
		return "junior"
	case "mid-level", "mid level", "midlevel":
		return "mid"
	case "entry-level", "entry level":
		return "entry"
	default:
		return m
	}
}

// classifyWorkType maps messy source signals to the remote/onsite/hybrid enum.
// Defaults to onsite when nothing indicates otherwise, since that's the safest
// assumption for a physical office pin.
func classifyWorkType(workRaw, locationRaw, descText string) models.WorkType {
	hay := strings.ToLower(workRaw + " " + locationRaw + " " + descText)
	switch {
	case strings.Contains(hay, "hybrid"):
		return models.WorkTypeHybrid
	case strings.Contains(hay, "remote"),
		strings.Contains(hay, "work from home"),
		strings.Contains(hay, "wfh"),
		strings.Contains(hay, "anywhere"):
		return models.WorkTypeRemote
	default:
		return models.WorkTypeOnsite
	}
}

// parseLocation is the normalizer's own splitter, mirroring the sources helper
// but kept here so the package doesn't depend on unexported source internals.
func parseLocation(raw string) (city, country string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ""
	}
	switch strings.ToLower(s) {
	case "remote", "worldwide", "anywhere", "global":
		return "", strings.Title(strings.ToLower(s)) //nolint:staticcheck
	}
	parts := strings.Split(s, ",")
	var cleaned []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			cleaned = append(cleaned, p)
		}
	}
	switch len(cleaned) {
	case 0:
		return "", ""
	case 1:
		return "", cleaned[0]
	default:
		return cleaned[0], cleaned[len(cleaned)-1]
	}
}

// parseDate tries the handful of formats the sources emit and returns nil on
// failure rather than guessing.
func parseDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"2006/01/02",
		"02.01.2006", // Jooble style
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return &t
		}
	}
	return nil
}

func nilIfZero(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}
