package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Arbeitnow (https://www.arbeitnow.com/api/job-board-api) is a free,
// key-less public jobs API covering EU and international postings.
type Arbeitnow struct {
	client *http.Client
}

func NewArbeitnow(httpClient *http.Client) *Arbeitnow {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Arbeitnow{client: httpClient}
}

func (a *Arbeitnow) Name() string { return "arbeitnow" }

type arbeitnowResponse struct {
	Data []arbeitnowItem `json:"data"`
}

type arbeitnowItem struct {
	Slug        string   `json:"slug"`
	CompanyName string   `json:"company_name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Remote      bool     `json:"remote"`
	URL         string   `json:"url"`
	Tags        []string `json:"tags"`
	JobTypes    []string `json:"job_types"`
	Location    string   `json:"location"`
	CreatedAt   int64    `json:"created_at"`
}

func (a *Arbeitnow) Fetch(ctx context.Context, q Query) ([]RawJob, error) {
	page := q.Page
	if page < 1 {
		page = 1
	}

	endpoint := fmt.Sprintf("https://www.arbeitnow.com/api/job-board-api?page=%d", page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("arbeitnow: building request: %w", err)
	}
	req.Header.Set("User-Agent", "job-find/1.0 (+https://job-find.xyz)")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("arbeitnow: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arbeitnow: unexpected status %d", resp.StatusCode)
	}

	var body arbeitnowResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("arbeitnow: decoding response: %w", err)
	}

	kw := strings.ToLower(q.Keywords)
	out := make([]RawJob, 0, len(body.Data))
	for _, it := range body.Data {
		if strings.TrimSpace(it.Title) == "" || strings.TrimSpace(it.CompanyName) == "" {
			continue
		}
		if kw != "" && !matchesKeyword(kw, it.Title, it.Description, it.Tags) {
			continue
		}

		workType := "onsite"
		if it.Remote {
			workType = "remote"
		}

		city, country := splitLocation(it.Location)

		out = append(out, RawJob{
			Source:      a.Name(),
			SourceID:    it.Slug,
			SourceURL:   it.URL,
			CompanyName: strings.TrimSpace(it.CompanyName),
			LocationRaw: it.Location,
			City:        city,
			Country:     country,
			Title:       strings.TrimSpace(it.Title),
			Description: it.Description,
			SkillsRaw:   it.Tags,
			WorkTypeRaw: workType,
		})

		if q.PerPage > 0 && len(out) >= q.PerPage {
			break
		}
	}
	return out, nil
}
