// Command api runs the HTTP API server that the frontend talks to. It wires the
// DB, geocoder and router together, applies migrations, and serves with a
// graceful shutdown. Ingestion runs in a SEPARATE process (cmd/ingest) so a
// heavy sync never competes with user traffic (README §7).
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mohityadav8/job-find/backend/internal/api"
	"github.com/mohityadav8/job-find/backend/internal/api/middleware"
	"github.com/mohityadav8/job-find/backend/internal/config"
	"github.com/mohityadav8/job-find/backend/internal/db"
	"github.com/mohityadav8/job-find/backend/internal/geocoding"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()
	log.Println("connected to database successfully")

	// Apply migrations on boot (idempotent). Path is relative to the repo root;
	// override with MIGRATIONS_DIR when running from elsewhere.
	migDir := os.Getenv("MIGRATIONS_DIR")
	if migDir == "" {
		migDir = "migrations"
	}
	if err := db.RunMigrations(ctx, pool, migDir); err != nil {
		log.Printf("warning: migrations not applied (%v) — continuing; run them manually if needed", err)
	} else {
		log.Println("migrations up to date")
	}

	store := db.NewStore(pool)
	geoCache := geocoding.NewCache(pool)
	geocoder := geocoding.NewGeocoder(geoCache, cfg.GoogleMapsAPIKey, nil)
	auth := middleware.NewAuthenticator(cfg.JWTSecret, cfg.JWTTTL)

	router := api.NewRouter(api.RouterConfig{
		Store:        store,
		Geo:          geocoder,
		Auth:         auth,
		AllowOrigins: cfg.AllowOrigins,
		CacheTTL:     cfg.CacheTTL,
		RateRPS:      cfg.RateRPS,
		RateBurst:    cfg.RateBurst,
		RedisAddr:    cfg.RedisAddr,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		log.Printf("api server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	log.Println("bye")
}
