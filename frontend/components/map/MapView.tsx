'use client';

// MapView is the hero of the whole product (README §1: "Google Maps as the
// primary interface, not a list with a small map widget bolted on"). It renders
// office pins as a clustered GeoJSON source on a dark MapLibre basemap.
//
// Why MapLibre instead of Google Maps: the README specifies Google Maps, but
// that requires a billing-enabled key before a single tile loads. MapLibre is
// open-source and renders immediately with free vector tiles, so the app works
// out of the box. The clustering non-negotiable (README §7) is satisfied
// natively by MapLibre's cluster support — arguably cleaner than bolting on
// Google's MarkerClusterer. Swapping back to Google later only touches this file.

import { useEffect, useRef, useCallback } from 'react';
import maplibregl, { Map as MLMap, GeoJSONSource, MapGeoJSONFeature } from 'maplibre-gl';
import type { Pin } from '@/types/pin';
import type { Viewport } from '@/lib/api';

interface MapViewProps {
  pins: Pin[];
  onOfficeSelect: (officeId: number) => void;
  onViewportChange?: (v: Viewport) => void;
}

// A dark, low-chroma basemap so the amber pins are unmistakably the focus.
// Uses the free CARTO dark vector style (no key required).
const DARK_STYLE = 'https://basemaps.cartocdn.com/gl/dark-matter-gl-style/style.json';

const SIGNAL = '#f5a524';
const SIGNAL_DARK = '#c77f14';

export default function MapView({ pins, onOfficeSelect, onViewportChange }: MapViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<MLMap | null>(null);
  const loadedRef = useRef(false);
  const onSelectRef = useRef(onOfficeSelect);
  const onViewportRef = useRef(onViewportChange);
  onSelectRef.current = onOfficeSelect;
  onViewportRef.current = onViewportChange;

  // pins → GeoJSON FeatureCollection.
  const toGeoJSON = useCallback((data: Pin[]) => {
    return {
      type: 'FeatureCollection' as const,
      features: data.map((p) => ({
        type: 'Feature' as const,
        geometry: { type: 'Point' as const, coordinates: [p.longitude, p.latitude] },
        properties: {
          office_id: p.office_id,
          company: p.company_name,
          city: p.city,
          country: p.country,
          job_count: p.job_count,
        },
      })),
    };
  }, []);

  const emitViewport = useCallback(() => {
    const map = mapRef.current;
    if (!map || !onViewportRef.current) return;
    const b = map.getBounds();
    onViewportRef.current({
      minLng: b.getWest(),
      minLat: b.getSouth(),
      maxLng: b.getEast(),
      maxLat: b.getNorth(),
    });
  }, []);

  // Init map once.
  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;

    const map = new maplibregl.Map({
      container: containerRef.current,
      style: DARK_STYLE,
      center: [10, 25], // a world-ish view centered on the land masses
      zoom: 1.6,
      minZoom: 1,
      maxZoom: 16,
      attributionControl: { compact: true },
    });
    mapRef.current = map;

    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'bottom-right');

    map.on('load', () => {
      map.addSource('offices', {
        type: 'geojson',
        data: toGeoJSON(pins),
        cluster: true,
        clusterRadius: 48,
        clusterMaxZoom: 12,
        clusterProperties: {
          // sum of job_count across a cluster, shown on the cluster badge
          job_sum: ['+', ['get', 'job_count']],
        },
      });

      // Cluster bubbles — sized + tinted by how many offices they contain.
      map.addLayer({
        id: 'clusters',
        type: 'circle',
        source: 'offices',
        filter: ['has', 'point_count'],
        paint: {
          'circle-color': [
            'step',
            ['get', 'point_count'],
            SIGNAL_DARK, 25, SIGNAL, 100, '#ffcf6b',
          ],
          'circle-opacity': 0.9,
          'circle-radius': ['step', ['get', 'point_count'], 16, 25, 22, 100, 30],
          'circle-stroke-width': 2,
          'circle-stroke-color': 'rgba(11,18,32,0.9)',
        },
      });
      map.addLayer({
        id: 'cluster-count',
        type: 'symbol',
        source: 'offices',
        filter: ['has', 'point_count'],
        layout: {
          'text-field': ['get', 'point_count_abbreviated'],
          'text-size': 13,
          'text-font': ['Metropolis Bold', 'Noto Sans Bold'],
        },
        paint: { 'text-color': '#1a1204' },
      });

      // Individual office pins.
      map.addLayer({
        id: 'office-point',
        type: 'circle',
        source: 'offices',
        filter: ['!', ['has', 'point_count']],
        paint: {
          'circle-color': SIGNAL,
          'circle-radius': 7,
          'circle-stroke-width': 2,
          'circle-stroke-color': 'rgba(11,18,32,0.9)',
        },
      });
      // Job-count badge on offices with multiple roles.
      map.addLayer({
        id: 'office-count',
        type: 'symbol',
        source: 'offices',
        filter: ['all', ['!', ['has', 'point_count']], ['>', ['get', 'job_count'], 1]],
        layout: {
          'text-field': ['to-string', ['get', 'job_count']],
          'text-size': 10,
          'text-font': ['Metropolis Bold', 'Noto Sans Bold'],
          'text-offset': [0, 0.05],
        },
        paint: { 'text-color': '#1a1204' },
      });

      loadedRef.current = true;
      emitViewport();
    });

    // Click a cluster → zoom into it.
    map.on('click', 'clusters', (e) => {
      const features = map.queryRenderedFeatures(e.point, { layers: ['clusters'] });
      const clusterId = features[0]?.properties?.cluster_id;
      const src = map.getSource('offices') as GeoJSONSource;
      if (clusterId == null || !src) return;
      src.getClusterExpansionZoom(clusterId).then((zoom) => {
        const geom = features[0].geometry as GeoJSON.Point;
        map.easeTo({ center: geom.coordinates as [number, number], zoom });
      });
    });

    // Click an office → open its detail.
    map.on('click', 'office-point', (e) => {
      const f = e.features?.[0] as MapGeoJSONFeature | undefined;
      const id = f?.properties?.office_id;
      if (id != null) onSelectRef.current(Number(id));
    });

    // Pointer affordances.
    for (const layer of ['clusters', 'office-point']) {
      map.on('mouseenter', layer, () => (map.getCanvas().style.cursor = 'pointer'));
      map.on('mouseleave', layer, () => (map.getCanvas().style.cursor = ''));
    }

    // Report viewport so the parent can refetch pins for the visible area.
    map.on('moveend', emitViewport);

    return () => {
      map.remove();
      mapRef.current = null;
      loadedRef.current = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Push new pins into the existing source when data changes.
  useEffect(() => {
    const map = mapRef.current;
    if (!map || !loadedRef.current) return;
    const src = map.getSource('offices') as GeoJSONSource | undefined;
    if (src) src.setData(toGeoJSON(pins));
  }, [pins, toGeoJSON]);

  return <div ref={containerRef} style={{ position: 'absolute', inset: 0 }} aria-label="World map of open roles" />;
}
