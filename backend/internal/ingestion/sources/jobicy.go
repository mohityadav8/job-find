package sources

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"
)

// Jobicy (https://jobicy.com/api/v2/remote-jobs) is a free, key-less
// remote jobs feed. We fetch all jobs and filter client-side to avoid
// the tag parameter returning 400 on multi-word queries.
type Jobicy struct {
    client *http.Client
}

func NewJobicy(httpClient *http.Client) *Jobicy {
    if httpClient == nil {
        httpClient = &http.Client{Timeout: 20 * time.Second}
    }
    return &Jobicy{client: httpClient}
}

func (j *Jobicy) Name() string { return "jobicy" }

type jobicyResponse struct {
    Jobs []jobicyItem `json:"jobs"`
}

type jobicyItem struct {
    ID          int             `json:"id"`
    URL         string          `json:"url"`
    JobTitle    string          `json:"jobTitle"`
    CompanyName string          `json:"companyName"`
    JobGeo      string          `json:"jobGeo"`
    JobType     json.RawMessage `json:"jobType"` // string or []string depending on API version
    JobExcerpt  string          `json:"jobExcerpt"`
    JobIndustry []string        `json:"jobIndustry"`
    PubDate     string          `json:"pubDate"`
}

func (j *Jobicy) Fetch(ctx context.Context, q Query) ([]RawJob, error) {
    // Fetch all jobs; Jobicy tag param returns 400 on multi-word queries
    // so we filter client-side like RemoteOK.
    endpoint := "https://jobicy.com/api/v2/remote-jobs?count=50"

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
    if err != nil {
        return nil, fmt.Errorf("jobicy: building request: %w", err)
    }
    req.Header.Set("User-Agent", "job-find/1.0 (+https://job-find.xyz)")
    req.Header.Set("Accept", "application/json")

    resp, err := j.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("jobicy: request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("jobicy: unexpected status %d", resp.StatusCode)
    }

    var body jobicyResponse
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
        return nil, fmt.Errorf("jobicy: decoding response: %w", err)
    }

    kw := strings.ToLower(q.Keywords)
    limit := q.PerPage
    out := make([]RawJob, 0, len(body.Jobs))
    for _, it := range body.Jobs {
        if strings.TrimSpace(it.JobTitle) == "" {
            continue
        }
        // Client-side keyword filter.
        if kw != "" && !matchesKeyword(kw, it.JobTitle, it.JobExcerpt, it.JobIndustry) {
            continue
        }

        city, country := splitLocation(it.JobGeo)
        if it.JobGeo == "" || strings.EqualFold(it.JobGeo, "worldwide") {
            city, country = "", "Remote"
        }

        out = append(out, RawJob{
            Source:      j.Name(),
            SourceID:    fmt.Sprintf("%d", it.ID),
            SourceURL:   it.URL,
            CompanyName: strings.TrimSpace(it.CompanyName),
            LocationRaw: it.JobGeo,
            City:        city,
            Country:     country,
            Title:       strings.TrimSpace(it.JobTitle),
            Description: it.JobExcerpt,
            SkillsRaw:   it.JobIndustry,
            WorkTypeRaw: "remote",
            PostedAt:    it.PubDate,
        })

        if limit > 0 && len(out) >= limit {
            break
        }
    }
    return out, nil
}
