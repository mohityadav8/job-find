-- NOTE: this file was originally scaffolded to "enable postgis" — that step
-- moved into 0001_init_schema.sql because the extension must exist before
-- offices.location (a geography column) can be created. This file now holds
-- indexes instead. Keeping the original filename so it still runs second.

-- Geo index: powers "pins within this map viewport" queries
CREATE INDEX idx_offices_location ON offices USING GIST (location);

CREATE INDEX idx_offices_company_id ON offices (company_id);
CREATE INDEX idx_jobs_office_id ON jobs (office_id);
CREATE INDEX idx_jobs_work_type ON jobs (work_type);

-- GIN index: powers "jobs that require skill X" filtering
CREATE INDEX idx_jobs_skills ON jobs USING GIN (skills);

CREATE INDEX idx_jobs_posted_date ON jobs (posted_date DESC);