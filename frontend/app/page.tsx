'use client';

// The map experience — the entire product on one screen. The map fills the
// viewport; the filter sidebar and office drawer float over it. Filter and
// viewport changes are debounced, then trigger a single /api/pins fetch.

import { useCallback, useEffect, useRef, useState } from 'react';
import dynamic from 'next/dynamic';
import type { Pin, Filters } from '@/types/pin';
import { emptyFilters } from '@/types/pin';
import { fetchPins, type Viewport } from '@/lib/api';
import FilterSidebar from '@/components/filters/FilterSidebar';
import OfficePin from '@/components/map/OfficePin';

// MapLibre touches window/document, so the map must be client-only (no SSR).
const MapView = dynamic(() => import('@/components/map/MapView'), {
  ssr: false,
  loading: () => (
    <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', color: 'var(--mist-300)' }}>
      Loading the world…
    </div>
  ),
});

export default function HomePage() {
  const [filters, setFilters] = useState<Filters>(emptyFilters);
  const [pins, setPins] = useState<Pin[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedOffice, setSelectedOffice] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  const viewportRef = useRef<Viewport | undefined>(undefined);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await fetchPins(filters, viewportRef.current, 3000);
      setPins(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not reach the server.');
      setPins([]);
    } finally {
      setLoading(false);
    }
  }, [filters]);

  // Refetch when filters change (debounced so typing doesn't spam the API).
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(load, 350);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [load]);

  const onViewportChange = useCallback(
    (v: Viewport) => {
      viewportRef.current = v;
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(load, 400);
    },
    [load],
  );

  return (
    <main style={{ position: 'fixed', inset: 0 }}>
      <MapView pins={pins} onOfficeSelect={setSelectedOffice} onViewportChange={onViewportChange} />

      <FilterSidebar filters={filters} onChange={setFilters} resultCount={pins.length} loading={loading} />

      {selectedOffice != null && (
        <OfficePin officeId={selectedOffice} filters={filters} onClose={() => setSelectedOffice(null)} />
      )}

      {error && (
        <div
          className="panel"
          style={{
            position: 'absolute',
            bottom: 20,
            left: '50%',
            transform: 'translateX(-50%)',
            zIndex: 30,
            padding: '11px 16px',
            fontSize: 13,
            color: 'var(--signal)',
            maxWidth: '90vw',
          }}
          role="alert"
        >
          {error} — is the backend running on :8080?
        </div>
      )}
    </main>
  );
}
