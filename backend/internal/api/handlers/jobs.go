package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mohityadav8/job-find/backend/internal/api/middleware"
	"github.com/mohityadav8/job-find/backend/internal/db"
	"github.com/mohityadav8/job-find/backend/internal/ingestion"
	"github.com/mohityadav8/job-find/backend/internal/models"
)

// GetStats handles GET /api/stats — aggregate counts for the landing page.
func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Store.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// GetSkills handles GET /api/skills — most common skills, for the filter UI.
func (h *Handlers) GetSkills(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v, ok := parseInt(map[string][]string(r.URL.Query()), "limit"); ok {
		limit = v
	}
	skills, err := h.Store.DistinctSkills(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load skills")
		return
	}
	if skills == nil {
		skills = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
}

// createJobRequest is the dashboard payload for posting a job (Phase 2).
type createJobRequest struct {
	OfficeID        int64    `json:"office_id"`
	Title           string   `json:"title"`
	Skills          []string `json:"skills"`
	ExperienceLevel string   `json:"experience_level"`
	WorkType        string   `json:"work_type"`
	SalaryMin       *int     `json:"salary_min"`
	SalaryMax       *int     `json:"salary_max"`
	SalaryCurrency  string   `json:"salary_currency"`
	SourceURL       string   `json:"source_url"`
}

// CreateJob handles POST /api/dashboard/jobs — a company posting a role
// directly (README §4 Phase 2). Requires auth; the job is stamped source
// "self-posted" and a dedup hash derived from the company/office.
func (h *Handlers) CreateJob(w http.ResponseWriter, r *http.Request) {
	companyID, ok := middleware.CompanyIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "account is not linked to a company")
		return
	}

	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Title) == "" || req.OfficeID == 0 {
		writeError(w, http.StatusBadRequest, "title and office_id are required")
		return
	}

	// Verify the office belongs to the authenticated company (ownership check).
	office, err := h.Store.GetOfficeDetail(r.Context(), req.OfficeID, db.PinFilter{})
	if err != nil || office.CompanyID != companyID {
		writeError(w, http.StatusForbidden, "office does not belong to your company")
		return
	}

	company, err := h.Store.GetCompany(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve company")
		return
	}

	wt := models.WorkType(strings.ToLower(strings.TrimSpace(req.WorkType)))
	if wt != models.WorkTypeRemote && wt != models.WorkTypeOnsite && wt != models.WorkTypeHybrid {
		wt = models.WorkTypeOnsite
	}

	skills := make([]string, 0, len(req.Skills))
	for _, s := range req.Skills {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			skills = append(skills, s)
		}
	}

	job := models.Job{
		OfficeID:        req.OfficeID,
		Title:           strings.TrimSpace(req.Title),
		Skills:          skills,
		ExperienceLevel: strings.TrimSpace(req.ExperienceLevel),
		WorkType:        wt,
		SalaryMin:       req.SalaryMin,
		SalaryMax:       req.SalaryMax,
		SalaryCurrency:  strings.ToUpper(strings.TrimSpace(req.SalaryCurrency)),
		Source:          "self-posted",
		SourceURL:       strings.TrimSpace(req.SourceURL),
		DedupHash: ingestion.DedupHash(
			company.Name, req.Title, office.City, office.Country,
		),
	}

	inserted, err := h.Store.UpsertJob(r.Context(), job)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create job")
		return
	}
	status := http.StatusCreated
	if !inserted {
		status = http.StatusOK // idempotent re-post
	}
	writeJSON(w, status, map[string]any{"created": inserted, "job": job})
}

// DeleteJob handles DELETE /api/dashboard/jobs/{id}.
func (h *Handlers) DeleteJob(w http.ResponseWriter, r *http.Request) {
	companyID, ok := middleware.CompanyIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "account is not linked to a company")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	if err := h.Store.DeleteJob(r.Context(), id, companyID); err != nil {
		if err == db.ErrNotFound {
			writeError(w, http.StatusNotFound, "job not found or not yours")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete job")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
