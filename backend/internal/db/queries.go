// Package db holds the data-access layer. Every SQL statement in the system
// lives here (or in migrations), so the handlers and ingestion pipeline never
// build queries themselves — they call typed methods on *Store.
package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mohityadav8/job-find/backend/internal/models"
)

// ErrNotFound is returned by lookups that resolve to zero rows.
var ErrNotFound = errors.New("not found")

// Store is the query surface over a pgx pool.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wraps a pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the underlying pool for callers that need transactions or the
// health check.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// PinFilter captures every knob the /pins endpoint supports (README §1/§9).
// Zero values mean "no constraint on this dimension".
type PinFilter struct {
	// Viewport bounding box (map bounds). When all four are zero the whole
	// world is returned.
	MinLat, MinLng, MaxLat, MaxLng float64
	HasViewport                    bool

	Keyword         string   // matched against job title (ILIKE)
	Skills          []string // job must contain ALL of these (GIN &&/@>)
	WorkTypes       []string // job.work_type IN (...)
	ExperienceLevel string   // exact-ish match on experience_level
	SalaryMin       *int     // job.salary_max >= SalaryMin (overlap semantics)
	SalaryMax       *int     // job.salary_min <= SalaryMax
	Source          string   // restrict to one source
	Company         string   // company name ILIKE

	Limit int // max pins returned; defaults applied by caller
}

// GetPins returns offices (as map pins) whose jobs match the filter, each with
// a count of matching jobs. The heavy lifting — viewport intersection, skill
// filtering, salary overlap — is pushed into SQL so PostGIS/GIN indexes do the
// work (README §3 point 3).
func (s *Store) GetPins(ctx context.Context, f PinFilter) ([]models.Pin, error) {
	where, args := buildJobWhere(f, 1)

	// The office/pin appears if it has at least one job matching the job-level
	// filters. We aggregate the matching job count per office in the same pass.
	var sb strings.Builder
	sb.WriteString(`
		SELECT o.id, ST_Y(o.location::geometry) AS lat, ST_X(o.location::geometry) AS lng,
		       o.city, o.country, COALESCE(o.address, ''),
		       c.id, c.name, COALESCE(c.logo_url, ''),
		       COUNT(j.id) AS job_count
		FROM offices o
		JOIN companies c ON c.id = o.company_id
		JOIN jobs j ON j.office_id = o.id
	`)
	if len(where) > 0 {
		sb.WriteString(" WHERE " + strings.Join(where, " AND "))
	}

	// Viewport constraint on the office location (separate from job filters).
	if f.HasViewport {
		n := len(args)
		// ST_MakeEnvelope(minLng, minLat, maxLng, maxLat, 4326)
		sb.WriteString(fmt.Sprintf(
			" AND ST_Intersects(o.location, ST_MakeEnvelope($%d,$%d,$%d,$%d,4326)::geography)",
			n+1, n+2, n+3, n+4))
		args = append(args, f.MinLng, f.MinLat, f.MaxLng, f.MaxLat)
	}

	sb.WriteString(`
		GROUP BY o.id, o.location, o.city, o.country, o.address, c.id, c.name, c.logo_url
		ORDER BY job_count DESC`)

	limit := f.Limit
	if limit <= 0 {
		limit = 2000
	}
	args = append(args, limit)
	sb.WriteString(fmt.Sprintf(" LIMIT $%d", len(args)))

	rows, err := s.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("GetPins query: %w", err)
	}
	defer rows.Close()

	var pins []models.Pin
	for rows.Next() {
		var p models.Pin
		if err := rows.Scan(
			&p.OfficeID, &p.Latitude, &p.Longitude,
			&p.City, &p.Country, &p.Address,
			&p.CompanyID, &p.CompanyName, &p.CompanyLogo,
			&p.JobCount,
		); err != nil {
			return nil, fmt.Errorf("GetPins scan: %w", err)
		}
		pins = append(pins, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pins, nil
}

// GetOfficeDetail returns a single office with its company and the full list of
// jobs matching the (optional) job filter. This backs a pin click.
func (s *Store) GetOfficeDetail(ctx context.Context, officeID int64, f PinFilter) (*models.Pin, error) {
	// Office + company header.
	var p models.Pin
	const head = `
		SELECT o.id, ST_Y(o.location::geometry), ST_X(o.location::geometry),
		       o.city, o.country, COALESCE(o.address,''),
		       c.id, c.name, COALESCE(c.logo_url,'')
		FROM offices o JOIN companies c ON c.id = o.company_id
		WHERE o.id = $1`
	err := s.pool.QueryRow(ctx, head, officeID).Scan(
		&p.OfficeID, &p.Latitude, &p.Longitude, &p.City, &p.Country, &p.Address,
		&p.CompanyID, &p.CompanyName, &p.CompanyLogo,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GetOfficeDetail head: %w", err)
	}

	// Jobs for this office, applying job-level filters so a filtered pin click
	// shows only the roles that matched.
	where, args := buildJobWhere(f, 2)
	where = append([]string{"j.office_id = $1"}, where...)

	q := `
		SELECT j.id, j.office_id, j.title, j.skills, COALESCE(j.experience_level,''),
		       j.work_type, j.salary_min, j.salary_max, COALESCE(j.salary_currency,''),
		       j.posted_date, j.source, COALESCE(j.source_url,''),
		       j.created_at, j.updated_at
		FROM jobs j
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY j.posted_date DESC NULLS LAST, j.created_at DESC`

	callArgs := append([]any{officeID}, args...)
	rows, err := s.pool.Query(ctx, q, callArgs...)
	if err != nil {
		return nil, fmt.Errorf("GetOfficeDetail jobs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		p.Jobs = append(p.Jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	p.JobCount = len(p.Jobs)
	return &p, nil
}

// buildJobWhere translates the job-level parts of a PinFilter into SQL
// fragments + args, numbering placeholders starting at startArg. It intentionally
// omits the viewport (handled separately on the office).
func buildJobWhere(f PinFilter, startArg int) ([]string, []any) {
	var where []string
	var args []any
	n := startArg - 1

	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		n++
		where = append(where, fmt.Sprintf("j.title ILIKE $%d", n))
		args = append(args, "%"+kw+"%")
	}
	if len(f.Skills) > 0 {
		n++
		// jobs.skills @> ARRAY[...] → job must contain ALL requested skills.
		where = append(where, fmt.Sprintf("j.skills @> $%d", n))
		lowered := make([]string, len(f.Skills))
		for i, s := range f.Skills {
			lowered[i] = strings.ToLower(strings.TrimSpace(s))
		}
		args = append(args, lowered)
	}
	if len(f.WorkTypes) > 0 {
		n++
		where = append(where, fmt.Sprintf("j.work_type = ANY($%d)", n))
		args = append(args, f.WorkTypes)
	}
	if exp := strings.TrimSpace(f.ExperienceLevel); exp != "" {
		n++
		where = append(where, fmt.Sprintf("j.experience_level ILIKE $%d", n))
		args = append(args, "%"+exp+"%")
	}
	if f.SalaryMin != nil {
		n++
		// Keep jobs whose max salary reaches at least the requested floor, or
		// whose salary is unknown (don't hide unsalaried postings by default).
		where = append(where, fmt.Sprintf("(j.salary_max IS NULL OR j.salary_max >= $%d)", n))
		args = append(args, *f.SalaryMin)
	}
	if f.SalaryMax != nil {
		n++
		where = append(where, fmt.Sprintf("(j.salary_min IS NULL OR j.salary_min <= $%d)", n))
		args = append(args, *f.SalaryMax)
	}
	if src := strings.TrimSpace(f.Source); src != "" {
		n++
		where = append(where, fmt.Sprintf("j.source = $%d", n))
		args = append(args, src)
	}
	if co := strings.TrimSpace(f.Company); co != "" {
		n++
		where = append(where, fmt.Sprintf("c.name ILIKE $%d", n))
		args = append(args, "%"+co+"%")
	}
	return where, args
}

// scanJob scans one jobs row in the canonical column order used above.
func scanJob(rows pgx.Rows) (models.Job, error) {
	var j models.Job
	if err := rows.Scan(
		&j.ID, &j.OfficeID, &j.Title, &j.Skills, &j.ExperienceLevel,
		&j.WorkType, &j.SalaryMin, &j.SalaryMax, &j.SalaryCurrency,
		&j.PostedDate, &j.Source, &j.SourceURL, &j.CreatedAt, &j.UpdatedAt,
	); err != nil {
		return models.Job{}, fmt.Errorf("scanJob: %w", err)
	}
	return j, nil
}

// ---- Companies -------------------------------------------------------------

// UpsertCompany inserts a company by name (idempotent) and returns its id.
// Ingestion calls this before creating offices/jobs.
func (s *Store) UpsertCompany(ctx context.Context, name, website, logoURL string) (int64, error) {
	const q = `
		INSERT INTO companies (name, website, logo_url)
		VALUES ($1, NULLIF($2,''), NULLIF($3,''))
		ON CONFLICT (name) DO UPDATE
		  SET website = COALESCE(NULLIF(EXCLUDED.website,''), companies.website),
		      logo_url = COALESCE(NULLIF(EXCLUDED.logo_url,''), companies.logo_url)
		RETURNING id`
	var id int64
	if err := s.pool.QueryRow(ctx, q, name, website, logoURL).Scan(&id); err != nil {
		return 0, fmt.Errorf("UpsertCompany: %w", err)
	}
	return id, nil
}

// ListCompanies returns companies for the browse/admin views.
func (s *Store) ListCompanies(ctx context.Context, limit int) ([]models.Company, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `SELECT id, name, COALESCE(website,''), COALESCE(logo_url,''), created_at
	           FROM companies ORDER BY name LIMIT $1`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Company
	for rows.Next() {
		var c models.Company
		if err := rows.Scan(&c.ID, &c.Name, &c.Website, &c.LogoURL, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCompany fetches one company by id.
func (s *Store) GetCompany(ctx context.Context, id int64) (*models.Company, error) {
	const q = `SELECT id, name, COALESCE(website,''), COALESCE(logo_url,''), created_at
	           FROM companies WHERE id = $1`
	var c models.Company
	err := s.pool.QueryRow(ctx, q, id).Scan(&c.ID, &c.Name, &c.Website, &c.LogoURL, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// ---- Offices ---------------------------------------------------------------

// UpsertOffice inserts (or fetches) an office for a company at a given
// city/country with resolved coordinates, and returns its id. The location is
// stored as a PostGIS geography point.
func (s *Store) UpsertOffice(ctx context.Context, companyID int64, city, country, address string, lat, lng float64) (int64, error) {
	const q = `
		INSERT INTO offices (company_id, city, country, address, location)
		VALUES ($1, $2, $3, NULLIF($4,''), ST_SetSRID(ST_MakePoint($6,$5),4326)::geography)
		ON CONFLICT (company_id, city, country) DO UPDATE
		  SET address = COALESCE(NULLIF(EXCLUDED.address,''), offices.address),
		      location = EXCLUDED.location
		RETURNING id`
	var id int64
	// Note arg order: $5=lat, $6=lng; ST_MakePoint takes (lng, lat).
	if err := s.pool.QueryRow(ctx, q, companyID, city, country, address, lat, lng).Scan(&id); err != nil {
		return 0, fmt.Errorf("UpsertOffice: %w", err)
	}
	return id, nil
}

// CreateOfficeForCompany is the dashboard-facing office creator (Phase 2).
func (s *Store) CreateOfficeForCompany(ctx context.Context, companyID int64, city, country, address string, lat, lng float64) (int64, error) {
	return s.UpsertOffice(ctx, companyID, city, country, address, lat, lng)
}

// ---- Jobs ------------------------------------------------------------------

// UpsertJob inserts a job keyed on its dedup_hash. If a job with the same hash
// already exists it's treated as a no-op update (refreshing updated_at and
// mutable fields), which is how cross-run dedup is enforced at the DB (README §7).
// Returns (inserted bool) so ingestion can report new vs. seen counts.
func (s *Store) UpsertJob(ctx context.Context, j models.Job) (bool, error) {
	const q = `
		INSERT INTO jobs
		  (office_id, title, skills, experience_level, work_type,
		   salary_min, salary_max, salary_currency, posted_date,
		   source, source_url, dedup_hash)
		VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,NULLIF($8,''),$9,$10,NULLIF($11,''),$12)
		ON CONFLICT (dedup_hash) DO UPDATE
		  SET updated_at = now(),
		      salary_min = EXCLUDED.salary_min,
		      salary_max = EXCLUDED.salary_max,
		      source_url = EXCLUDED.source_url
		RETURNING (xmax = 0) AS inserted`
	var inserted bool
	err := s.pool.QueryRow(ctx, q,
		j.OfficeID, j.Title, j.Skills, j.ExperienceLevel, j.WorkType,
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.PostedDate,
		j.Source, j.SourceURL, j.DedupHash,
	).Scan(&inserted)
	if err != nil {
		return false, fmt.Errorf("UpsertJob: %w", err)
	}
	return inserted, nil
}

// DeleteJob removes a job (dashboard).
func (s *Store) DeleteJob(ctx context.Context, jobID, companyID int64) error {
	const q = `
		DELETE FROM jobs j USING offices o
		WHERE j.id = $1 AND j.office_id = o.id AND o.company_id = $2`
	ct, err := s.pool.Exec(ctx, q, jobID, companyID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Stats -----------------------------------------------------------------

// Stats is a small dashboard/health summary.
type Stats struct {
	Companies int64 `json:"companies"`
	Offices   int64 `json:"offices"`
	Jobs      int64 `json:"jobs"`
	Remote    int64 `json:"remote_jobs"`
}

// GetStats returns aggregate counts for the landing page / admin.
func (s *Store) GetStats(ctx context.Context) (Stats, error) {
	var st Stats
	const q = `
		SELECT
		  (SELECT COUNT(*) FROM companies),
		  (SELECT COUNT(*) FROM offices),
		  (SELECT COUNT(*) FROM jobs),
		  (SELECT COUNT(*) FROM jobs WHERE work_type = 'remote')`
	if err := s.pool.QueryRow(ctx, q).Scan(&st.Companies, &st.Offices, &st.Jobs, &st.Remote); err != nil {
		return st, err
	}
	return st, nil
}

// DistinctSkills returns the most common skills for populating the filter UI.
func (s *Store) DistinctSkills(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `
		SELECT skill, COUNT(*) AS n
		FROM jobs, unnest(skills) AS skill
		GROUP BY skill ORDER BY n DESC LIMIT $1`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var skill string
		var n int
		if err := rows.Scan(&skill, &n); err != nil {
			return nil, err
		}
		out = append(out, skill)
	}
	return out, rows.Err()
}

// ---- Users (auth) ----------------------------------------------------------

// CreateUser inserts a new account and returns it. Caller supplies the bcrypt hash.
func (s *Store) CreateUser(ctx context.Context, email, passwordHash string, companyID *int64, role string) (*models.User, error) {
	const q = `
		INSERT INTO users (email, password_hash, company_id, role)
		VALUES ($1,$2,$3,$4)
		RETURNING id, email, password_hash, company_id, role, created_at`
	var u models.User
	err := s.pool.QueryRow(ctx, q, email, passwordHash, companyID, role).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.CompanyID, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("CreateUser: %w", err)
	}
	return &u, nil
}

// GetUserByEmail fetches an account for login.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	const q = `SELECT id, email, password_hash, company_id, role, created_at
	           FROM users WHERE email = $1`
	var u models.User
	err := s.pool.QueryRow(ctx, q, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.CompanyID, &u.Role, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// SetUserCompany links a user to a company (after they create one in the dashboard).
func (s *Store) SetUserCompany(ctx context.Context, userID, companyID int64) error {
	const q = `UPDATE users SET company_id = $2 WHERE id = $1`
	_, err := s.pool.Exec(ctx, q, userID, companyID)
	return err
}
