package sources

import "strings"

// splitLocation makes a best-effort split of a free-text location string into
// (city, country). It is deliberately conservative: the authoritative
// geocoding step later resolves the full string to coordinates, so this only
// needs to be good enough for display and for building a geocode cache key.
//
// Examples:
//
//	"Bengaluru, India"        -> ("Bengaluru", "India")
//	"San Francisco, CA, USA"  -> ("San Francisco", "USA")
//	"Remote"                  -> ("", "Remote")
//	"Worldwide"               -> ("", "Worldwide")
func splitLocation(raw string) (city, country string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ""
	}

	// Treat purely-remote markers as country-less; the office for these is a
	// synthetic "Remote" pin handled downstream.
	switch strings.ToLower(s) {
	case "remote", "worldwide", "anywhere", "global":
		return "", strings.Title(strings.ToLower(s)) //nolint:staticcheck // Title is fine for single words here
	}

	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	// Drop empty segments produced by trailing commas.
	cleaned := parts[:0]
	for _, p := range parts {
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	parts = cleaned

	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		// A single token is ambiguous; treat it as the country/region so pins
		// still group sensibly.
		return "", parts[0]
	default:
		return parts[0], parts[len(parts)-1]
	}
}
