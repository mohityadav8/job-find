package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mohityadav8/job-find/backend/internal/api/middleware"
	"github.com/mohityadav8/job-find/backend/internal/db"
	"github.com/mohityadav8/job-find/backend/internal/geocoding"
	"github.com/mohityadav8/job-find/backend/internal/ingestion"
	"github.com/mohityadav8/job-find/backend/internal/models"
)

// CompanyHandlers extends Handlers with the geocoder, needed when a company
// adds an office (its city/country must be resolved to coordinates before it
// can become a pin).
type CompanyHandlers struct {
	*Handlers
	Geo *geocoding.Geocoder
}

// NewCompanyHandlers builds company/dashboard handlers.
func NewCompanyHandlers(h *Handlers, geo *geocoding.Geocoder) *CompanyHandlers {
	return &CompanyHandlers{Handlers: h, Geo: geo}
}

// ListCompanies handles GET /api/companies.
func (h *Handlers) ListCompanies(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v, ok := parseInt(map[string][]string(r.URL.Query()), "limit"); ok {
		limit = v
	}
	companies, err := h.Store.ListCompanies(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load companies")
		return
	}
	if companies == nil {
		companies = []models.Company{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"companies": companies})
}

// GetCompany handles GET /api/companies/{id}.
func (h *Handlers) GetCompany(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid company id")
		return
	}
	company, err := h.Store.GetCompany(r.Context(), id)
	if err != nil {
		if err == db.ErrNotFound {
			writeError(w, http.StatusNotFound, "company not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load company")
		return
	}
	writeJSON(w, http.StatusOK, company)
}

// createOfficeRequest is the dashboard payload for adding an office pin.
type createOfficeRequest struct {
	City    string `json:"city"`
	Country string `json:"country"`
	Address string `json:"address"`
	// Optional explicit coordinates; when omitted the server geocodes.
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// CreateOffice handles POST /api/dashboard/offices — a company adding an office
// pin. Requires auth. Geocodes the location (cache-first) unless coordinates
// are supplied directly.
func (h *CompanyHandlers) CreateOffice(w http.ResponseWriter, r *http.Request) {
	companyID, ok := middleware.CompanyIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "account is not linked to a company")
		return
	}

	var req createOfficeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.City = strings.TrimSpace(req.City)
	req.Country = strings.TrimSpace(req.Country)
	if req.City == "" && req.Country == "" {
		writeError(w, http.StatusBadRequest, "city or country is required")
		return
	}

	var lat, lng float64
	if req.Latitude != nil && req.Longitude != nil {
		lat, lng = *req.Latitude, *req.Longitude
	} else {
		key := ingestion.LocationKey(req.City, req.Country)
		display := geocodeDisplay(req.City, req.Country)
		coord, err := h.Geo.Geocode(r.Context(), key, display)
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not geocode that location")
			return
		}
		lat, lng = coord.Lat, coord.Lng
	}

	officeID, err := h.Store.CreateOfficeForCompany(r.Context(), companyID, req.City, req.Country, req.Address, lat, lng)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create office")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"office_id": officeID,
		"latitude":  lat,
		"longitude": lng,
	})
}

// geocodeDisplay mirrors the ingestion helper for the dashboard path.
func geocodeDisplay(city, country string) string {
	switch {
	case city != "" && country != "":
		return city + ", " + country
	case country != "":
		return country
	case city != "":
		return city
	default:
		return "Remote"
	}
}
