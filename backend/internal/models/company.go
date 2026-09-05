package models

import "time"

// Company mirrors the "companies" table.
type Company struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Website   string    `json:"website,omitempty"`
	LogoURL   string    `json:"logo_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
