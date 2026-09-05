// Command ingest runs the background ingestion service (README §3/§7). It has
// three modes, selected by the first CLI arg (default: "worker"):
//
//	ingest worker      run the Asynq worker AND the cron scheduler together
//	                   (the normal long-running process in production)
//	ingest scheduler   run only the cron scheduler (enqueues sync tasks)
//	ingest once        run a single synchronous ingestion pass and exit
//	                   (handy for seeding a fresh DB or local testing)
//
// Keeping ingestion in its own binary is what keeps syncs off the API's
// request path.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mohityadav8/job-find/backend/internal/config"
	"github.com/mohityadav8/job-find/backend/internal/db"
	"github.com/mohityadav8/job-find/backend/internal/geocoding"
	"github.com/mohityadav8/job-find/backend/internal/ingestion"
)

func main() {
	mode := "worker"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

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

	store := db.NewStore(pool)
	geoCache := geocoding.NewCache(pool)
	geocoder := geocoding.NewGeocoder(geoCache, cfg.GoogleMapsAPIKey, nil)

	srcs := ingestion.BuildSources(ingestion.SourceConfig{
		AdzunaAppID:  cfg.AdzunaAppID,
		AdzunaAppKey: cfg.AdzunaAppKey,
		JoobleAPIKey: cfg.JoobleAPIKey,
	})
	log.Printf("ingest: %d source(s) enabled", len(srcs))
	runner := ingestion.NewRunner(store, geocoder, srcs, cfg.IngestPerQuery)

	switch mode {
	case "once":
		runOnce(ctx, runner)
	case "scheduler":
		runScheduler(cfg)
	case "worker":
		runWorker(cfg, runner)
	default:
		log.Fatalf("unknown mode %q (use: worker | scheduler | once)", mode)
	}
}

// runOnce performs a single synchronous pass and exits — used to seed data.
func runOnce(ctx context.Context, runner *ingestion.Runner) {
	log.Println("ingest: running one-shot sync...")
	res := runner.Run(ctx, ingestion.DefaultQueries())
	log.Printf("ingest: done — fetched=%d inserted=%d skipped=%d geocodeFail=%d errors=%d",
		res.Fetched, res.Inserted, res.Skipped, res.GeocodeFail, len(res.Errors))
}

// runScheduler runs only the cron scheduler.
func runScheduler(cfg *config.Config) {
	scheduler, err := ingestion.RegisterScheduler(cfg.RedisAddr, cfg.IngestCron)
	if err != nil {
		log.Fatalf("scheduler init: %v", err)
	}
	log.Printf("ingest: scheduler started (cron %q)", cfg.IngestCron)
	if err := scheduler.Run(); err != nil {
		log.Fatalf("scheduler error: %v", err)
	}
}

// runWorker runs the queue worker plus the scheduler in one process — the
// typical single-container deployment. Both are stopped cleanly on signal.
func runWorker(cfg *config.Config, runner *ingestion.Runner) {
	scheduler, err := ingestion.RegisterScheduler(cfg.RedisAddr, cfg.IngestCron)
	if err != nil {
		log.Fatalf("scheduler init: %v", err)
	}
	if err := scheduler.Start(); err != nil {
		log.Fatalf("scheduler start: %v", err)
	}
	defer scheduler.Shutdown()
	log.Printf("ingest: scheduler running (cron %q)", cfg.IngestCron)

	worker := ingestion.NewWorker(cfg.RedisAddr, runner, 4)

	// Run the worker in a goroutine so we can catch signals here.
	errCh := make(chan error, 1)
	go func() { errCh <- worker.Run() }()
	log.Println("ingest: worker running")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
		log.Println("ingest: shutting down...")
		worker.Shutdown()
	case err := <-errCh:
		if err != nil {
			log.Fatalf("worker error: %v", err)
		}
	}
}
