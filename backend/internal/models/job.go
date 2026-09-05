package models

import "time"

// WorkType mirrors the "work_type" Postgres enum.
type WorkType string

const (
	WorkTypeRemote WorkType = "remote"
	WorkTypeOnsite WorkType = "onsite"
	WorkTypeHybrid WorkType = "hybrid"
)

// Job mirrors the "jobs" table — the listings shown when a pin is clicked.
type Job struct {
	ID              int64      `json:"id"`
	OfficeID        int64      `json:"office_id"`
	Title           string     `json:"title"`
	Skills          []string   `json:"skills"`
	ExperienceLevel string     `json:"experience_level,omitempty"`
	WorkType        WorkType   `json:"work_type"`
	SalaryMin       *int       `json:"salary_min,omitempty"`
	SalaryMax       *int       `json:"salary_max,omitempty"`
	SalaryCurrency  string     `json:"salary_currency,omitempty"`
	PostedDate      *time.Time `json:"posted_date,omitempty"`
	Source          string     `json:"source"`
	SourceURL       string     `json:"source_url,omitempty"`
	DedupHash       string     `json:"-"` // internal only, never serialized in API responses
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
