package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Jooble (https://jooble.org/api/about) is a keyed, worldwide job aggregator.
// Unlike the others it uses a POST endpoint with the API key embedded in the
// URL path and a JSON body carrying the query.
type Jooble struct {
	apiKey string
	client *http.Client
}

// NewJooble builds a Jooble source. An empty apiKey makes Fetch a no-op so the
// scheduler skips it.
func NewJooble(apiKey string, httpClient *http.Client) *Jooble {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 25 * time.Second}
	}
	return &Jooble{apiKey: apiKey, client: httpClient}
}

func (j *Jooble) Name() string { return "jooble" }

type joobleRequest struct {
	Keywords string `json:"keywords,omitempty"`
	Location string `json:"location,omitempty"`
	Page     string `json:"page,omitempty"`
}

type joobleResponse struct {
	TotalCount int          `json:"totalCount"`
	Jobs       []joobleItem `json:"jobs"`
}

type joobleItem struct {
	Title    string `json:"title"`
	Location string `json:"location"`
	Snippet  string `json:"snippet"`
	Salary   string `json:"salary"`
	Source   string `json:"source"`
	Type     string `json:"type"`
	Link     string `json:"link"`
	Company  string `json:"company"`
	Updated  string `json:"updated"`
	ID       int64  `json:"id"`
}

func (j *Jooble) Fetch(ctx context.Context, q Query) ([]RawJob, error) {
	if j.apiKey == "" {
		return nil, nil
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	reqBody := joobleRequest{
		Keywords: q.Keywords,
		Location: q.Location,
		Page:     fmt.Sprintf("%d", page),
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("jooble: marshaling request: %w", err)
	}

	endpoint := "https://jooble.org/api/" + j.apiKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("jooble: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jooble: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jooble: unexpected status %d", resp.StatusCode)
	}

	var body joobleResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("jooble: decoding response: %w", err)
	}

	limit := q.PerPage
	out := make([]RawJob, 0, len(body.Jobs))
	for _, it := range body.Jobs {
		city, country := splitLocation(it.Location)
		out = append(out, RawJob{
			Source:      j.Name(),
			SourceID:    fmt.Sprintf("%d", it.ID),
			SourceURL:   it.Link,
			CompanyName: strings.TrimSpace(it.Company),
			LocationRaw: it.Location,
			City:        city,
			Country:     country,
			Title:       strings.TrimSpace(it.Title),
			Description: it.Snippet,
			WorkTypeRaw: it.Type, // free text; normalizer classifies
			PostedAt:    it.Updated,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
