package models

import "time"

// User is a company-side account (README §3 Phase 2 — the authenticated
// dashboard). Users own a company and manage that company's offices and jobs
// directly, writing into the same tables the ingestion service populates.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // never serialized
	CompanyID    *int64    `json:"company_id,omitempty"`
	Role         string    `json:"role"` // "company" | "admin"
	CreatedAt    time.Time `json:"created_at"`
}
