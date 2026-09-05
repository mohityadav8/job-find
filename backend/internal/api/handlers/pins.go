package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/mohityadav8/job-find/backend/internal/db"
	"github.com/mohityadav8/job-find/backend/internal/models"
)

// GetPins handles GET /api/pins — the primary map endpoint (README §3).
//
// Query params (all optional):
//
//	bbox        minLng,minLat,maxLng,maxLat  — map viewport
//	keyword     free-text title match
//	skills      comma-separated; job must have ALL
//	work_type   comma-separated: remote,onsite,hybrid
//	experience  e.g. "senior"
//	salary_min  integer
//	salary_max  integer
//	source      adzuna|jooble|remotive|remoteok|self-posted
//	company     company name substring
//	limit       max pins (default 2000)
//
// The response is a flat array of pins, each carrying a job_count. Jobs
// themselves are fetched lazily via GetOffice on a pin click, keeping the map
// payload small even with thousands of pins.
func (h *Handlers) GetPins(w http.ResponseWriter, r *http.Request) {
	f := pinFilterFromQuery(r)

	pins, err := h.Store.GetPins(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load pins")
		return
	}
	if pins == nil {
		pins = []models.Pin{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pins":  pins,
		"count": len(pins),
	})
}

// GetOffice handles GET /api/offices/{id} — full detail for one pin, including
// the list of jobs matching the same filters the map was showing.
func (h *Handlers) GetOffice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid office id")
		return
	}
	f := pinFilterFromQuery(r)
	office, err := h.Store.GetOfficeDetail(r.Context(), id, f)
	if err != nil {
		if err == db.ErrNotFound {
			writeError(w, http.StatusNotFound, "office not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load office")
		return
	}
	writeJSON(w, http.StatusOK, office)
}

// pinFilterFromQuery builds a db.PinFilter from request query params. Shared by
// GetPins and GetOffice so both interpret filters identically.
func pinFilterFromQuery(r *http.Request) db.PinFilter {
	q := map[string][]string(r.URL.Query())
	f := db.PinFilter{
		Keyword:         firstVal(q, "keyword"),
		Skills:          parseCSV(q, "skills"),
		WorkTypes:       parseCSV(q, "work_type"),
		ExperienceLevel: firstVal(q, "experience"),
		Source:          firstVal(q, "source"),
		Company:         firstVal(q, "company"),
	}
	if v, ok := parseInt(q, "salary_min"); ok {
		f.SalaryMin = &v
	}
	if v, ok := parseInt(q, "salary_max"); ok {
		f.SalaryMax = &v
	}
	if v, ok := parseInt(q, "limit"); ok {
		f.Limit = v
	}

	// bbox = minLng,minLat,maxLng,maxLat
	if bbox := parseCSV(q, "bbox"); len(bbox) == 4 {
		vals := make([]float64, 4)
		ok := true
		for i, s := range bbox {
			parsed, err := strconv.ParseFloat(s, 64)
			if err != nil {
				ok = false
				break
			}
			vals[i] = parsed
		}
		if ok {
			f.MinLng, f.MinLat, f.MaxLng, f.MaxLat = vals[0], vals[1], vals[2], vals[3]
			f.HasViewport = true
		}
	}
	return f
}
