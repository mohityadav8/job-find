'use client';

// FilterSidebar — the floating control panel over the map. It owns the full
// filter set from README §1/§9 (keyword, skill, experience, work type, salary,
// source, company) and reports changes up so the map re-queries /api/pins.

import { useState } from 'react';
import type { Filters, WorkType } from '@/types/pin';
import { emptyFilters } from '@/types/pin';
import WorkTypeFilter from './WorkTypeFilter';
import SkillFilter from './SkillFilter';

interface Props {
  filters: Filters;
  onChange: (next: Filters) => void;
  resultCount: number;
  loading: boolean;
}

const EXPERIENCE_LEVELS = ['intern', 'entry', 'junior', 'mid', 'senior', 'lead', 'principal'];
const SOURCES = ['adzuna', 'jooble', 'remotive', 'remoteok', 'self-posted'];

export default function FilterSidebar({ filters, onChange, resultCount, loading }: Props) {
  const [collapsed, setCollapsed] = useState(false);

  const set = <K extends keyof Filters>(key: K, val: Filters[K]) =>
    onChange({ ...filters, [key]: val });

  const activeCount =
    (filters.keyword ? 1 : 0) +
    filters.skills.length +
    filters.workTypes.length +
    (filters.experience ? 1 : 0) +
    (filters.source ? 1 : 0) +
    (filters.company ? 1 : 0) +
    (filters.salaryMin != null ? 1 : 0);

  if (collapsed) {
    return (
      <button
        className="panel"
        onClick={() => setCollapsed(false)}
        style={{ position: 'absolute', top: 16, left: 16, zIndex: 20, padding: '11px 15px', fontSize: 14, fontWeight: 600 }}
      >
        Filters{activeCount > 0 ? ` · ${activeCount}` : ''}
      </button>
    );
  }

  return (
    <aside
      className="panel scroll-thin"
      style={{
        position: 'absolute',
        top: 16,
        left: 16,
        bottom: 16,
        width: 320,
        maxWidth: 'calc(100vw - 32px)',
        zIndex: 20,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}
      aria-label="Filters"
    >
      {/* Brand + result count */}
      <header style={{ padding: '16px 18px 12px', borderBottom: '1px solid var(--ink-700)' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <a href="/" style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src="/job_find_logo.png" alt="job-find" width={26} height={26} style={{ borderRadius: 6 }} />
            <span style={{ fontSize: 17, fontWeight: 700, letterSpacing: '-0.01em' }}>job-find</span>
          </a>
          <button className="btn-ghost" onClick={() => setCollapsed(true)} aria-label="Collapse filters" style={{ padding: 4, fontSize: 18 }}>
            ‹
          </button>
        </div>
        <p style={{ margin: '10px 0 0', fontSize: 13, color: 'var(--mist-300)' }}>
          {loading ? 'Searching the map…' : `${resultCount.toLocaleString()} office${resultCount === 1 ? '' : 's'} in view`}
        </p>
      </header>

      {/* Scrollable filter body */}
      <div style={{ flex: 1, overflowY: 'auto', padding: 18, display: 'grid', gap: 18 }} className="scroll-thin">
        <Field label="Keyword">
          <input
            className="input"
            placeholder="Job title, e.g. backend engineer"
            value={filters.keyword}
            onChange={(e) => set('keyword', e.target.value)}
          />
        </Field>

        <Field label="Skills">
          <SkillFilter value={filters.skills} onChange={(v) => set('skills', v)} />
        </Field>

        <Field label="Work type">
          <WorkTypeFilter value={filters.workTypes} onChange={(v: WorkType[]) => set('workTypes', v)} />
        </Field>

        <Field label="Experience">
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            {EXPERIENCE_LEVELS.map((lvl) => (
              <button
                key={lvl}
                type="button"
                className="chip"
                data-active={filters.experience === lvl}
                onClick={() => set('experience', filters.experience === lvl ? '' : lvl)}
                style={{ textTransform: 'capitalize' }}
              >
                {lvl}
              </button>
            ))}
          </div>
        </Field>

        <Field label="Minimum salary">
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <input
              type="range"
              min={0}
              max={300000}
              step={10000}
              value={filters.salaryMin ?? 0}
              onChange={(e) => {
                const v = Number(e.target.value);
                set('salaryMin', v === 0 ? undefined : v);
              }}
              style={{ flex: 1, accentColor: 'var(--signal)' }}
              aria-label="Minimum salary"
            />
            <span style={{ fontSize: 13, color: 'var(--mist-300)', minWidth: 54, textAlign: 'right' }}>
              {filters.salaryMin ? `${Math.round(filters.salaryMin / 1000)}k+` : 'Any'}
            </span>
          </div>
        </Field>

        <Field label="Company">
          <input
            className="input"
            placeholder="Company name"
            value={filters.company}
            onChange={(e) => set('company', e.target.value)}
          />
        </Field>

        <Field label="Source">
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            {SOURCES.map((src) => (
              <button
                key={src}
                type="button"
                className="chip"
                data-active={filters.source === src}
                onClick={() => set('source', filters.source === src ? '' : src)}
              >
                {src}
              </button>
            ))}
          </div>
        </Field>
      </div>

      {/* Footer actions */}
      <footer style={{ padding: 14, borderTop: '1px solid var(--ink-700)', display: 'flex', gap: 10 }}>
        <button
          className="btn"
          style={{ flex: 1 }}
          onClick={() => onChange(emptyFilters)}
          disabled={activeCount === 0}
        >
          Clear{activeCount > 0 ? ` (${activeCount})` : ''}
        </button>
        <a className="btn" href="/dashboard" style={{ display: 'grid', placeItems: 'center' }}>
          Post a job
        </a>
      </footer>
    </aside>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--mist-300)', marginBottom: 8 }}>{label}</div>
      {children}
    </div>
  );
}
