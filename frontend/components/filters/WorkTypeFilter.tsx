'use client';

// WorkTypeFilter — the remote/onsite/hybrid toggle (README §1 filter set).
// Multi-select: no selection means "all work types".

import type { WorkType } from '@/types/pin';

const OPTIONS: { value: WorkType; label: string }[] = [
  { value: 'remote', label: 'Remote' },
  { value: 'hybrid', label: 'Hybrid' },
  { value: 'onsite', label: 'On-site' },
];

interface Props {
  value: WorkType[];
  onChange: (next: WorkType[]) => void;
}

export default function WorkTypeFilter({ value, onChange }: Props) {
  const toggle = (wt: WorkType) => {
    onChange(value.includes(wt) ? value.filter((v) => v !== wt) : [...value, wt]);
  };
  return (
    <div style={{ display: 'flex', gap: 7, flexWrap: 'wrap' }} role="group" aria-label="Work type">
      {OPTIONS.map((o) => (
        <button
          key={o.value}
          className="chip"
          data-active={value.includes(o.value)}
          aria-pressed={value.includes(o.value)}
          onClick={() => toggle(o.value)}
          type="button"
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
