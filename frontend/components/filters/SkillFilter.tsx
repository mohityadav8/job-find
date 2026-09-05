'use client';

// SkillFilter — searchable, multi-select skill picker. The suggestion list is
// populated from /api/skills (the most common skills in the DB), so it stays
// relevant to what's actually posted rather than a hard-coded list.

import { useEffect, useMemo, useState } from 'react';
import { fetchSkills } from '@/lib/api';

interface Props {
  value: string[];
  onChange: (next: string[]) => void;
}

export default function SkillFilter({ value, onChange }: Props) {
  const [all, setAll] = useState<string[]>([]);
  const [query, setQuery] = useState('');

  useEffect(() => {
    fetchSkills(60).then(setAll).catch(() => setAll([]));
  }, []);

  const suggestions = useMemo(() => {
    const q = query.trim().toLowerCase();
    return all
      .filter((s) => !value.includes(s))
      .filter((s) => (q ? s.includes(q) : true))
      .slice(0, 8);
  }, [all, query, value]);

  const add = (s: string) => {
    const skill = s.trim().toLowerCase();
    if (skill && !value.includes(skill)) onChange([...value, skill]);
    setQuery('');
  };
  const remove = (s: string) => onChange(value.filter((v) => v !== s));

  return (
    <div>
      <input
        className="input"
        placeholder="Add a skill (e.g. react)…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && query.trim()) {
            e.preventDefault();
            add(query);
          }
        }}
        aria-label="Search skills"
      />

      {suggestions.length > 0 && (
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 8 }}>
          {suggestions.map((s) => (
            <button key={s} type="button" className="chip" onClick={() => add(s)}>
              + {s}
            </button>
          ))}
        </div>
      )}

      {value.length > 0 && (
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 10 }}>
          {value.map((s) => (
            <button
              key={s}
              type="button"
              className="chip"
              data-active="true"
              onClick={() => remove(s)}
              aria-label={`Remove ${s}`}
            >
              {s} ×
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
