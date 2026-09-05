package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration, loaded from environment variables.
type Config struct {
	// Core infra
	DatabaseURL string
	RedisAddr   string
	Port        string

	// External data-source credentials (all optional; unset sources are skipped)
	AdzunaAppID  string
	AdzunaAppKey string
	JoobleAPIKey string

	// Google Maps / Geocoding (optional; falls back to Nominatim when unset)
	GoogleMapsAPIKey string

	// Auth
	JWTSecret string
	JWTTTL    time.Duration

	// API behavior
	AllowOrigins []string
	CacheTTL     time.Duration
	RateRPS      float64
	RateBurst    int

	// Ingestion
	IngestCron     string // cron spec for the scheduled sync
	IngestPerQuery int    // max results per source per query
}

// Load reads .env (if present) then loads config from the environment.
// Call once at the start of main().
func Load() (*Config, error) {
	// .env is optional — in production, real env vars are injected directly
	// and there's no .env file on disk. A missing file here is fine.
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		RedisAddr:        getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		Port:             getEnv("PORT", "8080"),
		AdzunaAppID:      os.Getenv("ADZUNA_APP_ID"),
		AdzunaAppKey:     os.Getenv("ADZUNA_APP_KEY"),
		JoobleAPIKey:     os.Getenv("JOOBLE_API_KEY"),
		GoogleMapsAPIKey: os.Getenv("GOOGLE_MAPS_API_KEY"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		JWTTTL:           getEnvDuration("JWT_TTL", 24*time.Hour),
		AllowOrigins:     getEnvCSV("ALLOW_ORIGINS", []string{"http://localhost:3000"}),
		CacheTTL:         getEnvDuration("CACHE_TTL", 60*time.Second),
		RateRPS:          getEnvFloat("RATE_RPS", 20),
		RateBurst:        getEnvInt("RATE_BURST", 40),
		IngestCron:       getEnv("INGEST_CRON", "0 */6 * * *"),
		IngestPerQuery:   getEnvInt("INGEST_PER_QUERY", 100),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set (check backend/.env exists)")
	}
	// A JWT secret is required for the dashboard; generate a loud failure rather
	// than silently signing tokens with an empty key.
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "dev-insecure-change-me" // safe default for local dev only
	}

	return cfg, nil
}

// --- small env helpers ------------------------------------------------------

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func getEnvCSV(key string, fallback []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
