package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/mohityadav8/job-find/backend/internal/api/handlers"
	"github.com/mohityadav8/job-find/backend/internal/api/middleware"
	"github.com/mohityadav8/job-find/backend/internal/db"
	"github.com/mohityadav8/job-find/backend/internal/geocoding"
)

// RouterConfig carries everything the router needs to wire handlers +
// middleware. Keeping it a struct (rather than a long param list) makes it easy
// to extend as new endpoints are added.
type RouterConfig struct {
	Store        *db.Store
	Geo          *geocoding.Geocoder
	Auth         *middleware.Authenticator
	AllowOrigins []string      // CORS allow-list (frontend origins)
	CacheTTL     time.Duration // /pins response cache TTL
	RateRPS      float64       // per-IP requests/sec
	RateBurst    int
	RedisAddr    string // used by the admin "trigger sync" endpoint
}

// NewRouter builds the fully-wired HTTP handler. This is the single place that
// declares the API surface, so the route table doubles as documentation.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// --- Global middleware ---------------------------------------------------
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))

	allowOrigins := cfg.AllowOrigins
	if len(allowOrigins) == 0 {
		allowOrigins = []string{"*"}
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Authorization"},
		ExposedHeaders:   []string{"X-Cache"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	rateLimiter := middleware.NewRateLimiter(cfg.RateRPS, cfg.RateBurst)
	r.Use(rateLimiter.Handler)

	h := handlers.New(cfg.Store)
	companyH := handlers.NewCompanyHandlers(h, cfg.Geo)
	authH := handlers.NewAuthHandlers(cfg.Store, cfg.Auth)

	// Response cache applied only to the read-heavy map endpoints.
	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = 60 * time.Second
	}
	respCache := middleware.NewCache(cacheTTL)

	// --- Health --------------------------------------------------------------
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := cfg.Store.Pool().Ping(req.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"degraded","db":"down"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// --- Public API ----------------------------------------------------------
	r.Route("/api", func(api chi.Router) {
		// Cached, read-only map + facet endpoints.
		api.Group(func(cached chi.Router) {
			cached.Use(respCache.Handler)
			cached.Get("/pins", h.GetPins)
			cached.Get("/offices/{id}", h.GetOffice)
			cached.Get("/stats", h.GetStats)
			cached.Get("/skills", h.GetSkills)
			cached.Get("/companies", h.ListCompanies)
			cached.Get("/companies/{id}", h.GetCompany)
		})

		// Auth (uncached).
		api.Post("/auth/register", authH.Register)
		api.Post("/auth/login", authH.Login)

		// Authenticated routes.
		api.Group(func(priv chi.Router) {
			priv.Use(cfg.Auth.Require)
			priv.Get("/auth/me", authH.Me)

			// Company dashboard (Phase 2): write into the same tables ingestion
			// populates, but scoped to the authenticated company.
			priv.Post("/dashboard/offices", companyH.CreateOffice)
			priv.Post("/dashboard/jobs", h.CreateJob)
			priv.Delete("/dashboard/jobs/{id}", h.DeleteJob)
		})
	})

	return r
}
