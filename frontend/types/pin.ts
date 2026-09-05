// Types mirroring the Go API responses (backend/internal/models). Keeping these
// in sync with the backend is what lets the whole data flow stay typed from the
// PostGIS query all the way to a rendered marker.

export type WorkType = 'remote' | 'onsite' | 'hybrid';

export interface Job {
  id: number;
  office_id: number;
  title: string;
  skills: string[];
  experience_level?: string;
  work_type: WorkType;
  salary_min?: number;
  salary_max?: number;
  salary_currency?: string;
  posted_date?: string;
  source: string;
  source_url?: string;
  created_at: string;
  updated_at: string;
}

// Pin is one map marker = one company office (README §2).
export interface Pin {
  office_id: number;
  latitude: number;
  longitude: number;
  city: string;
  country: string;
  address?: string;
  company_id: number;
  company_name: string;
  company_logo?: string;
  job_count: number;
  jobs?: Job[]; // populated only on office-detail fetch (a pin click)
}

export interface PinsResponse {
  pins: Pin[];
  count: number;
}

export interface Stats {
  companies: number;
  offices: number;
  jobs: number;
  remote_jobs: number;
}

// Filters is the client-side filter state, mapped to /api/pins query params.
export interface Filters {
  keyword: string;
  skills: string[];
  workTypes: WorkType[];
  experience: string;
  salaryMin?: number;
  salaryMax?: number;
  source: string;
  company: string;
}

export const emptyFilters: Filters = {
  keyword: '',
  skills: [],
  workTypes: [],
  experience: '',
  source: '',
  company: '',
};

// Auth
export interface AuthResponse {
  token: string;
  user_id: number;
  email: string;
  role: string;
  company_id?: number;
}
