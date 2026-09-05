// Package handlers implements the HTTP endpoints the frontend talks to. Per
// README §3 the API layer is the *only* thing the frontend calls — it never
// touches job-board APIs or the DB directly.
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/mohityadav8/job-find/backend/internal/db"
)

// Handlers bundles the dependencies every endpoint needs.
type Handlers struct {
	Store *db.Store
}

// New builds a Handlers.
func New(store *db.Store) *Handlers { return &Handlers{Store: store} }

// writeJSON serializes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error envelope.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// parseFloat reads a float query param, returning ok=false when absent/blank.
func parseFloat(vals map[string][]string, key string) (float64, bool) {
	s := firstVal(vals, key)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// parseInt reads an int query param.
func parseInt(vals map[string][]string, key string) (int, bool) {
	s := firstVal(vals, key)
	if s == "" {
		return 0, false
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return i, true
}

// parseCSV reads a comma-separated query param into a trimmed, non-empty slice.
func parseCSV(vals map[string][]string, key string) []string {
	s := firstVal(vals, key)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstVal(vals map[string][]string, key string) string {
	if v, ok := vals[key]; ok && len(v) > 0 {
		return strings.TrimSpace(v[0])
	}
	return ""
}
