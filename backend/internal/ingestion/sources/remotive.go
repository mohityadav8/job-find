package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Remotive (https://remotive.com/api/remote-jobs) is a free, key-less API of
// curated remote jobs. It supports a server-side search parameter, so unlike
// RemoteOK we can push the keyword filter upstream.
type Remotive struct {
	client *http.Client
}

func NewRemotive(httpClient *http.Client) *Remotive {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Remotive{client: httpClient}
}

func (r *Remotive) Name() string { return "remotive" }

type remotiveResponse struct {
	JobCount int            `json:"job-count"`
	Jobs     []remotiveItem `json:"jobs"`
}

type remotiveItem struct {
	ID           int      `json:"id"`
	URL          string   `json:"url"`
	Title        string   `json:"title"`
	CompanyName  string   `json:"company_name"`
	CompanyLogo  string   `json:"company_logo"`
	Category     string   `json:"category"`
	Tags         []string `json:"tags"`
	JobType      string   `json:"job_type"`
	CandidateLoc string   `json:"candidate_required_location"`
	Salary       string   `json:"salary"`
	Description  string   `json:"description"`
	PubDate      string   `json:"publication_date"`
}

func (r *Remotive) Fetch(ctx context.Context, q Query) ([]RawJob, error) {
	u := "https://remotive.com/api/remote-jobs"
	params := url.Values{}
	if q.Keywords != "" {
		params.Set("search", q.Keywords)
	}
	if q.PerPage > 0 {
		params.Set("limit", fmt.Sprintf("%d", q.PerPage))
	}
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("remotive: building request: %w", err)
	}
	req.Header.Set("User-Agent", "job-find/1.0 (+https://job-find.xyz)")
	req.Header.Set("Accept", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remotive: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remotive: unexpected status %d", resp.StatusCode)
	}

	var body remotiveResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("remotive: decoding response: %w", err)
	}

	out := make([]RawJob, 0, len(body.Jobs))
	for _, it := range body.Jobs {
		loc := strings.TrimSpace(it.CandidateLoc)
		if loc == "" {
			loc = "Remote"
		}
		city, country := splitLocation(loc)

		skills := it.Tags
		if it.Category != "" {
			skills = append(skills, it.Category)
		}

		out = append(out, RawJob{
			Source:         r.Name(),
			SourceID:       fmt.Sprintf("%d", it.ID),
			SourceURL:      it.URL,
			CompanyName:    strings.TrimSpace(it.CompanyName),
			CompanyLogo:    it.CompanyLogo,
			LocationRaw:    loc,
			City:           city,
			Country:        country,
			Title:          strings.TrimSpace(it.Title),
			Description:    it.Description,
			SkillsRaw:      skills,
			ExperienceRaw:  "",
			WorkTypeRaw:    "remote",
			SalaryCurrency: "", // Remotive salary is free text; left to normalizer
			PostedAt:       it.PubDate,
		})
	}
	return out, nil
}
