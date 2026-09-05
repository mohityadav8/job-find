'use client';

// Company dashboard (README §3/§4 Phase 2). Authenticated companies add office
// pins and post roles directly, writing into the same tables ingestion fills.
// Auth is a stateless JWT stored in localStorage; this page hydrates from it.

import { useEffect, useState } from 'react';
import type { AuthResponse } from '@/types/pin';
import {
  login,
  register,
  createOffice,
  createJob,
  getToken,
  clearToken,
} from '@/lib/api';

type Session = Pick<AuthResponse, 'email' | 'company_id'> | null;

export default function DashboardPage() {
  const [session, setSession] = useState<Session>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    // A token in storage means a prior session; treat as logged in. (The API
    // still authorizes every write, so a stale token just fails server-side.)
    if (getToken()) setSession({ email: '', company_id: undefined });
    setReady(true);
  }, []);

  if (!ready) return null;

  return (
    <div style={{ position: 'fixed', inset: 0, overflowY: 'auto', background: 'var(--ink-900)' }} className="scroll-thin">
      <TopBar
        loggedIn={!!session}
        onSignOut={() => {
          clearToken();
          setSession(null);
        }}
      />
      <div style={{ maxWidth: 640, margin: '0 auto', padding: '32px 20px 80px' }}>
        {!session ? (
          <AuthPanel onAuthed={(r) => setSession({ email: r.email, company_id: r.company_id })} />
        ) : (
          <PostingPanel />
        )}
      </div>
    </div>
  );
}

function TopBar({ loggedIn, onSignOut }: { loggedIn: boolean; onSignOut: () => void }) {
  return (
    <header
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '14px 20px',
        borderBottom: '1px solid var(--ink-700)',
        position: 'sticky',
        top: 0,
        background: 'var(--ink-900)',
        zIndex: 10,
      }}
    >
      <a href="/" style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src="/job_find_logo.png" alt="" width={26} height={26} style={{ borderRadius: 6 }} />
        <span style={{ fontWeight: 700, fontSize: 16 }}>job-find</span>
        <span style={{ color: 'var(--mist-300)', fontSize: 14 }}>· for companies</span>
      </a>
      <nav style={{ display: 'flex', gap: 8 }}>
        <a className="btn-ghost" href="/" style={{ padding: '8px 12px' }}>
          ← Back to map
        </a>
        {loggedIn && (
          <button className="btn" onClick={onSignOut}>
            Sign out
          </button>
        )}
      </nav>
    </header>
  );
}

function AuthPanel({ onAuthed }: { onAuthed: (r: AuthResponse) => void }) {
  const [mode, setMode] = useState<'login' | 'register'>('register');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [companyName, setCompanyName] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      const res =
        mode === 'register'
          ? await register(email, password, companyName || undefined)
          : await login(email, password);
      onAuthed(res);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Something went wrong.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <section>
      <h1 style={{ fontSize: 26, fontWeight: 700, margin: '8px 0 6px', letterSpacing: '-0.02em' }}>
        Put your roles on the map
      </h1>
      <p style={{ color: 'var(--mist-300)', margin: '0 0 26px', lineHeight: 1.5 }}>
        Add your offices as pins and post openings directly — no waiting on a job
        board to pick them up. Your listings appear alongside everything else the
        moment you publish.
      </p>

      <div className="panel" style={{ padding: 22 }}>
        <div style={{ display: 'flex', gap: 8, marginBottom: 18 }}>
          <button className="chip" data-active={mode === 'register'} onClick={() => setMode('register')} type="button">
            Create account
          </button>
          <button className="chip" data-active={mode === 'login'} onClick={() => setMode('login')} type="button">
            Sign in
          </button>
        </div>

        <div style={{ display: 'grid', gap: 12 }}>
          {mode === 'register' && (
            <input
              className="input"
              placeholder="Company name"
              value={companyName}
              onChange={(e) => setCompanyName(e.target.value)}
            />
          )}
          <input
            className="input"
            type="email"
            placeholder="Work email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="email"
          />
          <input
            className="input"
            type="password"
            placeholder="Password (8+ characters)"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete={mode === 'register' ? 'new-password' : 'current-password'}
            onKeyDown={(e) => e.key === 'Enter' && submit()}
          />
          {err && <p style={{ color: 'var(--signal)', fontSize: 13, margin: 0 }}>{err}</p>}
          <button className="btn btn-primary" onClick={submit} disabled={busy}>
            {busy ? 'Working…' : mode === 'register' ? 'Create account' : 'Sign in'}
          </button>
        </div>
      </div>
    </section>
  );
}

function PostingPanel() {
  // Step 1: create an office (geocoded server-side) → get its id.
  // Step 2: post a role at that office.
  const [officeId, setOfficeId] = useState<number | null>(null);
  const [officeLabel, setOfficeLabel] = useState('');

  return (
    <section style={{ display: 'grid', gap: 20 }}>
      <h1 style={{ fontSize: 24, fontWeight: 700, margin: '4px 0', letterSpacing: '-0.02em' }}>
        Your workspace
      </h1>

      <OfficeForm
        onCreated={(id, label) => {
          setOfficeId(id);
          setOfficeLabel(label);
        }}
      />

      {officeId != null && (
        <JobForm officeId={officeId} officeLabel={officeLabel} />
      )}
    </section>
  );
}

function OfficeForm({ onCreated }: { onCreated: (id: number, label: string) => void }) {
  const [city, setCity] = useState('');
  const [country, setCountry] = useState('');
  const [address, setAddress] = useState('');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    setMsg(null);
    try {
      const res = await createOffice({ city, country, address: address || undefined });
      const label = [city, country].filter(Boolean).join(', ');
      setMsg(`Office pinned at ${res.latitude.toFixed(3)}, ${res.longitude.toFixed(3)}.`);
      onCreated(res.office_id, label);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Could not create office.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="panel" style={{ padding: 22 }}>
      <h2 style={{ fontSize: 16, fontWeight: 600, margin: '0 0 4px' }}>1 · Add an office</h2>
      <p style={{ color: 'var(--mist-300)', fontSize: 13, margin: '0 0 16px' }}>
        We turn the city and country into a map pin automatically.
      </p>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
        <input className="input" placeholder="City" value={city} onChange={(e) => setCity(e.target.value)} />
        <input className="input" placeholder="Country" value={country} onChange={(e) => setCountry(e.target.value)} />
      </div>
      <input
        className="input"
        placeholder="Street address (optional)"
        value={address}
        onChange={(e) => setAddress(e.target.value)}
        style={{ marginTop: 12 }}
      />
      {msg && <p style={{ color: 'var(--ok)', fontSize: 13, margin: '12px 0 0' }}>{msg}</p>}
      {err && <p style={{ color: 'var(--signal)', fontSize: 13, margin: '12px 0 0' }}>{err}</p>}
      <button className="btn btn-primary" onClick={submit} disabled={busy || (!city && !country)} style={{ marginTop: 16 }}>
        {busy ? 'Pinning…' : 'Add office'}
      </button>
    </div>
  );
}

function JobForm({ officeId, officeLabel }: { officeId: number; officeLabel: string }) {
  const [title, setTitle] = useState('');
  const [skills, setSkills] = useState('');
  const [experience, setExperience] = useState('');
  const [workType, setWorkType] = useState('onsite');
  const [salaryMin, setSalaryMin] = useState('');
  const [salaryMax, setSalaryMax] = useState('');
  const [currency, setCurrency] = useState('USD');
  const [url, setUrl] = useState('');
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    setMsg(null);
    try {
      await createJob({
        office_id: officeId,
        title,
        skills: skills.split(',').map((s) => s.trim()).filter(Boolean),
        experience_level: experience || undefined,
        work_type: workType,
        salary_min: salaryMin ? Number(salaryMin) : undefined,
        salary_max: salaryMax ? Number(salaryMax) : undefined,
        salary_currency: currency || undefined,
        source_url: url || undefined,
      });
      setMsg('Role published — it’s live on the map now.');
      setTitle('');
      setSkills('');
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Could not publish role.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="panel" style={{ padding: 22 }}>
      <h2 style={{ fontSize: 16, fontWeight: 600, margin: '0 0 4px' }}>2 · Post a role</h2>
      <p style={{ color: 'var(--mist-300)', fontSize: 13, margin: '0 0 16px' }}>
        At <strong style={{ color: 'var(--mist-100)' }}>{officeLabel}</strong>.
      </p>
      <div style={{ display: 'grid', gap: 12 }}>
        <input className="input" placeholder="Job title" value={title} onChange={(e) => setTitle(e.target.value)} />
        <input className="input" placeholder="Skills, comma-separated (react, go, sql)" value={skills} onChange={(e) => setSkills(e.target.value)} />
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          <input className="input" placeholder="Experience (e.g. senior)" value={experience} onChange={(e) => setExperience(e.target.value)} />
          <select className="input" value={workType} onChange={(e) => setWorkType(e.target.value)}>
            <option value="onsite">On-site</option>
            <option value="hybrid">Hybrid</option>
            <option value="remote">Remote</option>
          </select>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 90px', gap: 12 }}>
          <input className="input" type="number" placeholder="Salary min" value={salaryMin} onChange={(e) => setSalaryMin(e.target.value)} />
          <input className="input" type="number" placeholder="Salary max" value={salaryMax} onChange={(e) => setSalaryMax(e.target.value)} />
          <input className="input" placeholder="Cur" value={currency} onChange={(e) => setCurrency(e.target.value)} />
        </div>
        <input className="input" placeholder="Application URL (optional)" value={url} onChange={(e) => setUrl(e.target.value)} />
        {msg && <p style={{ color: 'var(--ok)', fontSize: 13, margin: 0 }}>{msg}</p>}
        {err && <p style={{ color: 'var(--signal)', fontSize: 13, margin: 0 }}>{err}</p>}
        <button className="btn btn-primary" onClick={submit} disabled={busy || !title}>
          {busy ? 'Publishing…' : 'Publish role'}
        </button>
      </div>
    </div>
  );
}
