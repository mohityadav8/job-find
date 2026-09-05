package ingestion

import (
	"net/http"
	"time"

	"github.com/mohityadav8/job-find/backend/internal/ingestion/sources"
)

// SourceConfig carries the credentials needed to build the source set. Empty
// credentials simply mean that source is not enabled.
type SourceConfig struct {
	AdzunaAppID  string
	AdzunaAppKey string
	JoobleAPIKey string
}

// BuildSources returns the enabled source adapters. RemoteOK and Remotive are
// always on (they need no keys); Adzuna and Jooble are included only when
// credentials are present. All share one tuned HTTP client.
func BuildSources(cfg SourceConfig) []sources.Source {
	client := &http.Client{Timeout: 25 * time.Second}

	out := []sources.Source{
		sources.NewRemoteOK(client),
		sources.NewRemotive(client),
	}
	if cfg.AdzunaAppID != "" && cfg.AdzunaAppKey != "" {
		out = append(out, sources.NewAdzuna(cfg.AdzunaAppID, cfg.AdzunaAppKey, client))
	}
	if cfg.JoobleAPIKey != "" {
		out = append(out, sources.NewJooble(cfg.JoobleAPIKey, client))
	}
	return out
}
