package tests

import (
	"testing"

	"github.com/mohityadav8/job-find/backend/internal/ingestion"
	"github.com/mohityadav8/job-find/backend/internal/ingestion/sources"
	"github.com/mohityadav8/job-find/backend/internal/models"
)

func TestNormalize_BasicFields(t *testing.T) {
	raw := sources.RawJob{
		Source:      "remoteok",
		CompanyName: "  Acme   Corp ",
		Title:       "  Senior Go Engineer ",
		LocationRaw: "Bengaluru, India",
		WorkTypeRaw: "remote",
		SkillsRaw:   []string{"Go", "go", "Kubernetes"},
		Description: "We use PostgreSQL and Docker heavily.",
	}
	n := ingestion.Normalize(raw)

	if n.CompanyName != "Acme Corp" {
		t.Errorf("company not cleaned: %q", n.CompanyName)
	}
	if n.Job.Title != "Senior Go Engineer" {
		t.Errorf("title not cleaned: %q", n.Job.Title)
	}
	if n.City != "Bengaluru" || n.Country != "India" {
		t.Errorf("location split wrong: city=%q country=%q", n.City, n.Country)
	}
	if n.Job.WorkType != models.WorkTypeRemote {
		t.Errorf("expected remote, got %q", n.Job.WorkType)
	}
	if n.Job.ExperienceLevel != "senior" {
		t.Errorf("expected senior, got %q", n.Job.ExperienceLevel)
	}
}

func TestNormalize_SkillsDedupAndMine(t *testing.T) {
	raw := sources.RawJob{
		Source:      "remotive",
		CompanyName: "DataCo",
		Title:       "Backend Developer",
		LocationRaw: "Remote",
		SkillsRaw:   []string{"Python", "python", "  "},
		Description: "Experience with PostgreSQL, redis and Kafka required. Bonus: machine learning.",
	}
	n := ingestion.Normalize(raw)

	got := map[string]bool{}
	for _, s := range n.Job.Skills {
		got[s] = true
	}
	// Deduped source tag.
	if count := countOccurrences(n.Job.Skills, "python"); count != 1 {
		t.Errorf("expected python once, got %d (%v)", count, n.Job.Skills)
	}
	// Mined from description.
	for _, want := range []string{"postgresql", "redis", "kafka", "machine learning"} {
		if !got[want] {
			t.Errorf("expected mined skill %q in %v", want, n.Job.Skills)
		}
	}
}

func TestNormalize_WorkTypeClassification(t *testing.T) {
	cases := []struct {
		workRaw, locRaw, desc string
		want                  models.WorkType
	}{
		{"", "New York, USA", "office based", models.WorkTypeOnsite},
		{"hybrid", "London", "", models.WorkTypeHybrid},
		{"", "", "work from home role", models.WorkTypeRemote},
		{"full remote", "Anywhere", "", models.WorkTypeRemote},
	}
	for i, c := range cases {
		raw := sources.RawJob{
			CompanyName: "X", Title: "Dev",
			WorkTypeRaw: c.workRaw, LocationRaw: c.locRaw, Description: c.desc,
		}
		if got := ingestion.Normalize(raw).Job.WorkType; got != c.want {
			t.Errorf("case %d: want %q got %q", i, c.want, got)
		}
	}
}

func TestLocationKey_Canonical(t *testing.T) {
	cases := []struct {
		city, country, want string
	}{
		{"Bengaluru", "India", "bengaluru|india"},
		{"", "Remote", "remote"},
		{"Berlin", "", "berlin"},
		{"", "", "remote"},
		{"  Paris ", " France ", "paris|france"},
	}
	for _, c := range cases {
		if got := ingestion.LocationKey(c.city, c.country); got != c.want {
			t.Errorf("LocationKey(%q,%q) = %q, want %q", c.city, c.country, got, c.want)
		}
	}
}

func countOccurrences(ss []string, target string) int {
	n := 0
	for _, s := range ss {
		if s == target {
			n++
		}
	}
	return n
}
