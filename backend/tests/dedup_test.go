package tests

import (
	"testing"

	"github.com/mohityadav8/job-find/backend/internal/ingestion"
)

func TestDedupHash_StableAcrossCosmeticDifferences(t *testing.T) {
	// The same role from two boards, with cosmetic formatting differences,
	// must produce an identical fingerprint (README §2/§7).
	h1 := ingestion.DedupHash("Acme Corp", "Senior Engineer", "Bengaluru", "India")
	h2 := ingestion.DedupHash("acme corp", "senior   engineer", "bengaluru", "india")
	h3 := ingestion.DedupHash("Acme, Corp.", "Senior Engineer!", "Bengaluru", "India")

	if h1 != h2 {
		t.Errorf("expected identical hashes for case/spacing variants:\n%s\n%s", h1, h2)
	}
	if h1 != h3 {
		t.Errorf("expected identical hashes for punctuation variants:\n%s\n%s", h1, h3)
	}
}

func TestDedupHash_DiffersOnRealDifferences(t *testing.T) {
	base := ingestion.DedupHash("Acme", "Engineer", "Berlin", "Germany")

	diffTitle := ingestion.DedupHash("Acme", "Manager", "Berlin", "Germany")
	diffCity := ingestion.DedupHash("Acme", "Engineer", "Munich", "Germany")
	diffCompany := ingestion.DedupHash("Globex", "Engineer", "Berlin", "Germany")

	for name, h := range map[string]string{"title": diffTitle, "city": diffCity, "company": diffCompany} {
		if h == base {
			t.Errorf("hash should differ when %s differs", name)
		}
	}
}

func TestDedupBatch_RemovesDuplicates(t *testing.T) {
	mk := func(company, title, city, country string) ingestion.Normalized {
		n := ingestion.Normalized{CompanyName: company}
		n.Job.Title = title
		n.City = city
		n.Country = country
		n.Job.DedupHash = ingestion.DedupHash(company, title, city, country)
		return n
	}

	in := []ingestion.Normalized{
		mk("Acme", "Engineer", "Berlin", "Germany"),
		mk("acme", "engineer", "berlin", "germany"), // duplicate of #1
		mk("Globex", "Designer", "Paris", "France"),
		mk("Acme", "Engineer", "Berlin", "Germany"), // duplicate of #1 again
	}

	out := ingestion.DedupBatch(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 unique records, got %d", len(out))
	}
	// Order preserved: first Acme, then Globex.
	if out[0].CompanyName != "Acme" || out[1].CompanyName != "Globex" {
		t.Errorf("order not preserved: %q, %q", out[0].CompanyName, out[1].CompanyName)
	}
}

func TestDedupBatch_KeepsHashlessRecords(t *testing.T) {
	in := []ingestion.Normalized{
		{CompanyName: "NoHash1"},
		{CompanyName: "NoHash2"},
	}
	out := ingestion.DedupBatch(in)
	if len(out) != 2 {
		t.Errorf("hashless records should be kept, got %d", len(out))
	}
}
