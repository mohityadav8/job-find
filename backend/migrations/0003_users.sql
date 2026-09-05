-- Phase 2 (README §3): company-side accounts for the self-serve dashboard.
-- Users own a company and write directly into the offices/jobs tables.

-- UpsertCompany relies on ON CONFLICT (name); enforce that uniqueness.
-- (Wrapped so re-running the migration is safe.)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'companies_name_key'
    ) THEN
        ALTER TABLE companies ADD CONSTRAINT companies_name_key UNIQUE (name);
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    company_id BIGINT REFERENCES companies(id) ON DELETE SET NULL,
    role TEXT NOT NULL DEFAULT 'company',  -- 'company' | 'admin'
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_users_company_id ON users (company_id);
