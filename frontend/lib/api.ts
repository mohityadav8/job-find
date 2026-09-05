// Typed client for the job-find Go API. Every network call the frontend makes
// goes through here, so request shaping (query params, auth headers) lives in
// one place. Same-origin '/api' is proxied to the backend in dev (next.config).

import type {
  Pin,
  PinsResponse,
  Stats,
  Filters,
  AuthResponse,
} from '@/types/pin';

const API_BASE = process.env.NEXT_PUBLIC_API_BASE || '';

// --- auth token storage (browser only) --------------------------------------

const TOKEN_KEY = 'job-find-token';

export function getToken(): string | null {
  if (typeof window === 'undefined') return null;
  return window.localStorage.getItem(TOKEN_KEY);
}
export function setToken(token: string) {
  if (typeof window !== 'undefined') window.localStorage.setItem(TOKEN_KEY, token);
}
export function clearToken() {
  if (typeof window !== 'undefined') window.localStorage.removeItem(TOKEN_KEY);
}

function authHeaders(): Record<string, string> {
  const t = getToken();
  return t ? { Authorization: `Bearer ${t}` } : {};
}

async function handle<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let msg = `request failed (${res.status})`;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* non-JSON error body; keep the default message */
    }
    throw new Error(msg);
  }
  return res.json() as Promise<T>;
}

// --- filter → query string --------------------------------------------------

export interface Viewport {
  minLng: number;
  minLat: number;
  maxLng: number;
  maxLat: number;
}

function buildPinsQuery(filters: Filters, viewport?: Viewport, limit?: number): string {
  const p = new URLSearchParams();
  if (filters.keyword) p.set('keyword', filters.keyword);
  if (filters.skills.length) p.set('skills', filters.skills.join(','));
  if (filters.workTypes.length) p.set('work_type', filters.workTypes.join(','));
  if (filters.experience) p.set('experience', filters.experience);
  if (filters.source) p.set('source', filters.source);
  if (filters.company) p.set('company', filters.company);
  if (filters.salaryMin != null) p.set('salary_min', String(filters.salaryMin));
  if (filters.salaryMax != null) p.set('salary_max', String(filters.salaryMax));
  if (viewport) {
    p.set(
      'bbox',
      [viewport.minLng, viewport.minLat, viewport.maxLng, viewport.maxLat].join(','),
    );
  }
  if (limit) p.set('limit', String(limit));
  return p.toString();
}

// --- public read endpoints --------------------------------------------------

export async function fetchPins(
  filters: Filters,
  viewport?: Viewport,
  limit = 2000,
): Promise<Pin[]> {
  const qs = buildPinsQuery(filters, viewport, limit);
  const res = await fetch(`${API_BASE}/api/pins?${qs}`);
  const data = await handle<PinsResponse>(res);
  return data.pins || [];
}

export async function fetchOffice(officeId: number, filters: Filters): Promise<Pin> {
  const qs = buildPinsQuery(filters);
  const res = await fetch(`${API_BASE}/api/offices/${officeId}?${qs}`);
  return handle<Pin>(res);
}

export async function fetchStats(): Promise<Stats> {
  const res = await fetch(`${API_BASE}/api/stats`);
  return handle<Stats>(res);
}

export async function fetchSkills(limit = 40): Promise<string[]> {
  const res = await fetch(`${API_BASE}/api/skills?limit=${limit}`);
  const data = await handle<{ skills: string[] }>(res);
  return data.skills || [];
}

// --- auth -------------------------------------------------------------------

export async function register(
  email: string,
  password: string,
  companyName?: string,
): Promise<AuthResponse> {
  const res = await fetch(`${API_BASE}/api/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password, company_name: companyName }),
  });
  const data = await handle<AuthResponse>(res);
  setToken(data.token);
  return data;
}

export async function login(email: string, password: string): Promise<AuthResponse> {
  const res = await fetch(`${API_BASE}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });
  const data = await handle<AuthResponse>(res);
  setToken(data.token);
  return data;
}

// --- dashboard (authenticated writes) ---------------------------------------

export interface NewOffice {
  city: string;
  country: string;
  address?: string;
  latitude?: number;
  longitude?: number;
}

export async function createOffice(o: NewOffice): Promise<{ office_id: number; latitude: number; longitude: number }> {
  const res = await fetch(`${API_BASE}/api/dashboard/offices`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: JSON.stringify(o),
  });
  return handle(res);
}

export interface NewJob {
  office_id: number;
  title: string;
  skills: string[];
  experience_level?: string;
  work_type: string;
  salary_min?: number;
  salary_max?: number;
  salary_currency?: string;
  source_url?: string;
}

export async function createJob(j: NewJob): Promise<unknown> {
  const res = await fetch(`${API_BASE}/api/dashboard/jobs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: JSON.stringify(j),
  });
  return handle(res);
}
