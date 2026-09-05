-- Enable PostGIS before creating any geography columns.
-- (Originally split into a separate 0002 file per the scaffold, but the
-- extension must exist before offices.location can be created — folded in here.)
CREATE EXTENSION IF NOT EXISTS postgis;

-- Companies
CREATE TABLE companies (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    website TEXT,
    logo_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Offices = the map pins. One company can have many (one per city/country).
CREATE TABLE offices (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    city TEXT NOT NULL,
    country TEXT NOT NULL,
    address TEXT,
    location GEOGRAPHY(Point, 4326) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, city, country)
);

-- Matches the remote/onsite/hybrid filter from the README
CREATE TYPE work_type AS ENUM ('remote', 'onsite', 'hybrid');

-- Jobs = the listings shown when a pin is clicked
CREATE TABLE jobs (
    id BIGSERIAL PRIMARY KEY,
    office_id BIGINT NOT NULL REFERENCES offices(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    skills TEXT[] NOT NULL DEFAULT '{}',
    experience_level TEXT,
    work_type work_type NOT NULL DEFAULT 'onsite',
    salary_min INTEGER,
    salary_max INTEGER,
    salary_currency TEXT,
    posted_date DATE,
    source TEXT NOT NULL,          -- 'adzuna' | 'jooble' | 'remotive' | 'remoteok' | 'self-posted'
    source_url TEXT,
    dedup_hash TEXT NOT NULL UNIQUE, -- enforces the dedup rule from README §2/§7 at the DB level
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Geocoding cache: one lookup per unique "city, country" string, ever.
-- This is the main cost control called out in README §3/§6/§7 — without it,
-- every ingestion sync would re-geocode every job.
CREATE TABLE geocode_cache (
    location_key TEXT PRIMARY KEY,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);