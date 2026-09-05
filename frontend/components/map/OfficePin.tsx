'use client';

// OfficePin is the detail drawer shown when a map pin (an office) is clicked.
// It lists every open role at that office matching the active filters — the
// core "click a pin, see every role there" interaction from README §1.

import { useEffect, useState } from 'react';
import type { Pin, Job, Filters } from '@/types/pin';
import { fetchOffice } from '@/lib/api';

interface OfficePinProps {
  officeId: number;
  filters: Filters;
  onClose: () => void;
}

export default function OfficePin({ officeId, filters, onClose }: OfficePinProps) {
  const [office, setOffice] = useState<Pin | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    fetchOffice(officeId, filters)
      .then((o) => {
        if (!cancelled) setOffice(o);
      })
      .catch((e) => {
        if (!cancelled) setError(e.message || 'Could not load this office.');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [officeId, filters]);

  // Escape closes the drawer.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  return (
    <aside
      className="panel scroll-thin"
      style={{
        position: 'absolute',
        top: 16,
        right: 16,
        bottom: 16,
        width: 400,
        maxWidth: 'calc(100vw - 32px)',
        zIndex: 20,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}
      aria-label="Office details"
    >
      <header
        style={{
          display: 'flex',
          alignItems: 'flex-start',
          gap: 12,
          padding: '18px 18px 14px',
          borderBottom: '1px solid var(--ink-700)',
        }}
      >
        {office?.company_logo ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={office.company_logo}
            alt=""
            width={40}
            height={40}
            style={{ borderRadius: 8, objectFit: 'cover', background: 'var(--ink-900)' }}
          />
        ) : (
          <div
            aria-hidden
            style={{
              width: 40,
              height: 40,
              borderRadius: 8,
              background: 'var(--ink-900)',
              display: 'grid',
              placeItems: 'center',
              color: 'var(--signal)',
              fontWeight: 700,
            }}
          >
            {office?.company_name?.[0]?.toUpperCase() ?? '·'}
          </div>
        )}
        <div style={{ flex: 1, minWidth: 0 }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 600, lineHeight: 1.2 }}>
            {office?.company_name ?? 'Loading…'}
          </h2>
          <p style={{ margin: '4px 0 0', fontSize: 13, color: 'var(--mist-300)' }}>
            {office ? [office.city, office.country].filter(Boolean).join(', ') : ''}
          </p>
        </div>
        <button className="btn-ghost" onClick={onClose} aria-label="Close" style={{ padding: 4, fontSize: 20, lineHeight: 1 }}>
          ×
        </button>
      </header>

      <div style={{ flex: 1, overflowY: 'auto', padding: 18 }} className="scroll-thin">
        {loading && <p style={{ color: 'var(--mist-300)' }}>Loading open roles…</p>}
        {error && <p style={{ color: 'var(--signal)' }}>{error}</p>}
        {office && !loading && (
          <>
            <div style={{ fontSize: 13, color: 'var(--mist-300)', marginBottom: 14 }}>
              {office.jobs?.length ?? 0} open role{(office.jobs?.length ?? 0) === 1 ? '' : 's'}
              {office.address ? ` · ${office.address}` : ''}
            </div>
            <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'grid', gap: 10 }}>
              {(office.jobs ?? []).map((job) => (
                <JobCard key={job.id} job={job} />
              ))}
            </ul>
            {(office.jobs?.length ?? 0) === 0 && (
              <p style={{ color: 'var(--mist-300)' }}>
                No roles match your current filters at this office.
              </p>
            )}
          </>
        )}
      </div>
    </aside>
  );
}

function JobCard({ job }: { job: Job }) {
  const salary = formatSalary(job);
  const workColor =
    job.work_type === 'remote'
      ? 'var(--ok)'
      : job.work_type === 'hybrid'
        ? 'var(--signal)'
        : 'var(--mist-300)';
  return (
    <li
      style={{
        border: '1px solid var(--ink-700)',
        borderRadius: 'var(--radius-sm)',
        padding: '13px 14px',
        background: 'var(--ink-900)',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 10, alignItems: 'baseline' }}>
        <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>{job.title}</h3>
        <span style={{ fontSize: 12, color: workColor, whiteSpace: 'nowrap', textTransform: 'capitalize' }}>
          {job.work_type}
        </span>
      </div>
      <div style={{ marginTop: 4, fontSize: 12, color: 'var(--mist-300)', display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        {job.experience_level && <span style={{ textTransform: 'capitalize' }}>{job.experience_level}</span>}
        {salary && <span>· {salary}</span>}
        <span>· via {job.source}</span>
      </div>
      {job.skills?.length > 0 && (
        <div style={{ marginTop: 9, display: 'flex', gap: 5, flexWrap: 'wrap' }}>
          {job.skills.slice(0, 6).map((s) => (
            <span
              key={s}
              style={{
                fontSize: 11,
                padding: '2px 7px',
                borderRadius: 999,
                background: 'var(--ink-700)',
                color: 'var(--mist-300)',
              }}
            >
              {s}
            </span>
          ))}
        </div>
      )}
      {job.source_url && (
        <a
          href={job.source_url}
          target="_blank"
          rel="noopener noreferrer"
          className="btn btn-primary"
          style={{ marginTop: 12, display: 'inline-block', padding: '7px 12px', fontSize: 13 }}
        >
          View & apply
        </a>
      )}
    </li>
  );
}

function formatSalary(job: Job): string {
  const { salary_min, salary_max, salary_currency } = job;
  if (!salary_min && !salary_max) return '';
  const cur = salary_currency ? `${salary_currency} ` : '';
  const fmt = (n: number) => (n >= 1000 ? `${Math.round(n / 1000)}k` : String(n));
  if (salary_min && salary_max) return `${cur}${fmt(salary_min)}–${fmt(salary_max)}`;
  return `${cur}${fmt((salary_min || salary_max)!)}`;
}
