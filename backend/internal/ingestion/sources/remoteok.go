package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// RemoteOK (https://remoteok.com/api) is a free, key-less JSON feed of remote
// jobs. It's the cheapest source to run and needs no credentials, so it's the
// default "always on" integration used in dev and smoke tests.
//
// Quirk: the very first element of the returned array is a legal/metadata
// notice, not a job — it must be skipped.
type RemoteOK struct {
	client *http.Client
}

// NewRemoteOK builds a RemoteOK source. httpClient may be nil, in which case a
// sensible defaulted client is used.
func NewRemoteOK(httpClient *http.Client) *RemoteOK {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &RemoteOK{client: httpClient}
}

func (r *RemoteOK) Name() string { return "remoteok" }

// remoteOKItem mirrors the fields we care about from a RemoteOK feed entry.
// The feed types some numeric fields inconsistently, so salary is read
// leniently via json.Number.
type remoteOKItem struct {
	Legal       string      `json:"legal"` // only present on the header element
	ID          string      `json:"id"`
	URL         string      `json:"url"`
	Company     string      `json:"company"`
	CompanyLogo string      `json:"company_logo"`
	Position    string      `json:"position"`
	Description string      `json:"description"`
	Location    string      `json:"location"`
	Tags        []string    `json:"tags"`
	Date        string      `json:"date"`
	SalaryMin   json.Number `json:"salary_min"`
	SalaryMax   json.Number `json:"salary_max"`
}

func (r *RemoteOK) Fetch(ctx context.Context, q Query) ([]RawJob, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://remoteok.com/api", nil)
	if err != nil {
		return nil, fmt.Errorf("remoteok: building request: %w", err)
	}
	// RemoteOK blocks default Go user-agents; a browser-ish UA is required.
	req.Header.Set("User-Agent", "job-find/1.0 (+https://job-find.xyz)")
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remoteok: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remoteok: unexpected status %d", resp.StatusCode)
	}

	var items []remoteOKItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("remoteok: decoding response: %w", err)
	}

	kw := strings.ToLower(q.Keywords)
	out := make([]RawJob, 0, len(items))
	for _, it := range items {
		// Skip the legal/header element and anything without a title.
		if it.Legal != "" || strings.TrimSpace(it.Position) == "" {
			continue
		}
		// Client-side keyword filter (the feed has no server-side search).
		if kw != "" && !matchesKeyword(kw, it.Position, it.Description, it.Tags) {
			continue
		}

		city, country := splitLocation(it.Location)
		out = append(out, RawJob{
			Source:      r.Name(),
			SourceID:    it.ID,
			SourceURL:   it.URL,
			CompanyName: strings.TrimSpace(it.Company),
			CompanyLogo: it.CompanyLogo,
			LocationRaw: it.Location,
			City:        city,
			Country:     country,
			Title:       strings.TrimSpace(it.Position),
			Description: it.Description,
			SkillsRaw:   it.Tags,
			WorkTypeRaw: "remote", // RemoteOK is remote-only by definition
			SalaryMin:   parseJSONInt(it.SalaryMin),
			SalaryMax:   parseJSONInt(it.SalaryMax),
			PostedAt:    it.Date,
		})

		if q.PerPage > 0 && len(out) >= q.PerPage {
			break
		}
	}
	return out, nil
}

// matchesKeyword does a cheap case-insensitive contains check across the fields
// a user would expect a keyword to hit.
func matchesKeyword(kw, title, desc string, tags []string) bool {
	if strings.Contains(strings.ToLower(title), kw) {
		return true
	}
	if strings.Contains(strings.ToLower(desc), kw) {
		return true
	}
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), kw) {
			return true
		}
	}
	return false
}

// parseJSONInt reads a json.Number leniently, returning 0 on anything unparseable.
func parseJSONInt(n json.Number) int {
	if n == "" {
		return 0
	}
	i, err := n.Int64()
	if err != nil {
		// Some feeds emit floats like "90000.0"; fall back to float parse.
		f, ferr := n.Float64()
		if ferr != nil {
			return 0
		}
		return int(f)
	}
	return int(i)
}
