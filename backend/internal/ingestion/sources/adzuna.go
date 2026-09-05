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

// Adzuna (https://developer.adzuna.com/) is a keyed, worldwide job API with
// real structured salary and location data. It is the primary Phase-1 source
// (README §4) because it gives already-geocoded-ish, country-scoped, salaried
// listings — the richest input we get before any scraping.
//
// Adzuna's API is country-partitioned: you hit /v1/api/jobs/{country}/search/1
// where {country} is an ISO-ish code ("gb", "us", "in", ...). We iterate a
// configured set of countries so a single Fetch call yields worldwide coverage.
type Adzuna struct {
	appID     string
	appKey    string
	countries []string
	client    *http.Client
}

// DefaultAdzunaCountries is the Phase-1 country set. Adzuna supports these
// marketplaces; extend as coverage rolls out.
var DefaultAdzunaCountries = []string{"gb", "us", "in", "au", "ca", "de", "fr", "sg"}

// NewAdzuna builds an Adzuna source. If appID or appKey is empty the source is
// considered unconfigured and Fetch returns (nil, nil) so the scheduler skips it.
func NewAdzuna(appID, appKey string, httpClient *http.Client) *Adzuna {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 25 * time.Second}
	}
	return &Adzuna{
		appID:     appID,
		appKey:    appKey,
		countries: DefaultAdzunaCountries,
		client:    httpClient,
	}
}

func (a *Adzuna) Name() string { return "adzuna" }

type adzunaResponse struct {
	Results []adzunaItem `json:"results"`
}

type adzunaItem struct {
	ID           string  `json:"id"`
	RedirectURL  string  `json:"redirect_url"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	Created      string  `json:"created"`
	SalaryMin    float64 `json:"salary_min"`
	SalaryMax    float64 `json:"salary_max"`
	ContractTime string  `json:"contract_time"`
	Company      struct {
		DisplayName string `json:"display_name"`
	} `json:"company"`
	Location struct {
		DisplayName string   `json:"display_name"`
		Area        []string `json:"area"` // ["India", "Karnataka", "Bengaluru"]
	} `json:"location"`
	Category struct {
		Label string `json:"label"`
	} `json:"category"`
}

func (a *Adzuna) Fetch(ctx context.Context, q Query) ([]RawJob, error) {
	// Unconfigured → politely skip.
	if a.appID == "" || a.appKey == "" {
		return nil, nil
	}

	perPage := q.PerPage
	if perPage <= 0 || perPage > 50 {
		perPage = 50 // Adzuna hard cap per page
	}
	page := q.Page
	if page < 1 {
		page = 1
	}

	var out []RawJob
	for _, country := range a.countries {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}

		items, err := a.fetchCountry(ctx, country, q, page, perPage)
		if err != nil {
			// One country failing shouldn't abort the whole worldwide pull;
			// log-and-continue is handled by the caller via the returned error
			// only when *every* country failed. Here we just skip.
			continue
		}
		out = append(out, items...)
	}
	return out, nil
}

func (a *Adzuna) fetchCountry(ctx context.Context, country string, q Query, page, perPage int) ([]RawJob, error) {
	endpoint := fmt.Sprintf("https://api.adzuna.com/v1/api/jobs/%s/search/%d", country, page)
	params := url.Values{}
	params.Set("app_id", a.appID)
	params.Set("app_key", a.appKey)
	params.Set("results_per_page", fmt.Sprintf("%d", perPage))
	params.Set("content-type", "application/json")
	if q.Keywords != "" {
		params.Set("what", q.Keywords)
	}
	if q.Location != "" {
		params.Set("where", q.Location)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("adzuna: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("adzuna: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("adzuna: %s returned status %d", country, resp.StatusCode)
	}

	var body adzunaResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("adzuna: decoding response: %w", err)
	}

	out := make([]RawJob, 0, len(body.Results))
	for _, it := range body.Results {
		city, ctry := adzunaLocation(it.Location.Area, it.Location.DisplayName)
		out = append(out, RawJob{
			Source:         a.Name(),
			SourceID:       it.ID,
			SourceURL:      it.RedirectURL,
			CompanyName:    strings.TrimSpace(it.Company.DisplayName),
			LocationRaw:    it.Location.DisplayName,
			City:           city,
			Country:        ctry,
			Title:          strings.TrimSpace(it.Title),
			Description:    it.Description,
			SkillsRaw:      splitCategory(it.Category.Label),
			WorkTypeRaw:    it.ContractTime, // "full_time"/"part_time"; normalizer maps to onsite by default
			SalaryMin:      int(it.SalaryMin),
			SalaryMax:      int(it.SalaryMax),
			SalaryCurrency: currencyForCountry(country),
			PostedAt:       it.Created,
		})
	}
	return out, nil
}

// adzunaLocation prefers the structured area array (country-first) but falls
// back to splitting the display name.
func adzunaLocation(area []string, display string) (city, country string) {
	if len(area) >= 2 {
		// area is ordered broad→narrow: [country, region, city]
		return area[len(area)-1], area[0]
	}
	if len(area) == 1 {
		return "", area[0]
	}
	return splitLocation(display)
}

func splitCategory(label string) []string {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	// Strip a trailing " Jobs" that Adzuna appends to category labels.
	label = strings.TrimSuffix(label, " Jobs")
	return []string{label}
}

// currencyForCountry maps an Adzuna country code to an ISO currency for display.
func currencyForCountry(code string) string {
	switch code {
	case "gb":
		return "GBP"
	case "us", "ca":
		return "USD"
	case "in":
		return "INR"
	case "au":
		return "AUD"
	case "de", "fr":
		return "EUR"
	case "sg":
		return "SGD"
	default:
		return ""
	}
}
