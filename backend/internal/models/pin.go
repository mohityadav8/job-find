package models

// Pin is the API response shape for the map. It is *not* a DB table — it's an
// office joined to its company and its matching jobs, assembled by the API
// layer. This is the payload the frontend renders as a single map marker
// (README §2: pins are offices, and a pin with N roles is still one marker).
type Pin struct {
	OfficeID  int64   `json:"office_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	City      string  `json:"city"`
	Country   string  `json:"country"`
	Address   string  `json:"address,omitempty"`

	CompanyID   int64  `json:"company_id"`
	CompanyName string `json:"company_name"`
	CompanyLogo string `json:"company_logo,omitempty"`

	// JobCount is the number of jobs at this office matching the active filters.
	// The marker badge shows this; Jobs is populated only when the client
	// requests the office detail (a pin click), to keep the map payload small.
	JobCount int   `json:"job_count"`
	Jobs     []Job `json:"jobs,omitempty"`
}
