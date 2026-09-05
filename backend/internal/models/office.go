package models

import "time"

// Office mirrors the "offices" table. This is what becomes a map pin —
// see README §2 for why pins are offices, not individual jobs.
type Office struct {
	ID        int64     `json:"id"`
	CompanyID int64     `json:"company_id"`
	City      string    `json:"city"`
	Country   string    `json:"country"`
	Address   string    `json:"address,omitempty"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	CreatedAt time.Time `json:"created_at"`

	// Populated by the API layer when building pin responses.
	// Not DB columns — filled in manually after separate queries/joins.
	Company *Company `json:"company,omitempty"`
	Jobs    []Job    `json:"jobs,omitempty"`
}
