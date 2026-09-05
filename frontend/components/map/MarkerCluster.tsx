'use client';

// MarkerCluster — clustering configuration + helpers.
//
// The README lists marker clustering as a production non-negotiable (§7): raw
// pins at world zoom with real volume are unusable. In this implementation the
// clustering itself runs inside MapView via MapLibre's native GeoJSON cluster
// support (cluster/clusterRadius/clusterMaxZoom on the source). This module
// centralizes the tunable parameters and a couple of pure helpers so the
// behavior is documented in one place and easy to adjust without touching the
// map wiring.

export interface ClusterConfig {
  /** Pixel radius within which points merge into a cluster. */
  radius: number;
  /** Above this zoom, clusters break apart into individual pins. */
  maxZoom: number;
}

// Defaults tuned for a world → city zoom range. A larger radius means fewer,
// denser bubbles on the world view; a lower maxZoom means pins separate sooner.
export const DEFAULT_CLUSTER: ClusterConfig = {
  radius: 48,
  maxZoom: 12,
};

/**
 * clusterBadge returns the abbreviated label for a cluster bubble
 * (e.g. 1500 → "1.5k"), matching what the map renders.
 */
export function clusterBadge(count: number): string {
  if (count >= 1_000_000) return `${(count / 1_000_000).toFixed(1)}M`;
  if (count >= 1_000) return `${(count / 1_000).toFixed(count >= 10_000 ? 0 : 1)}k`;
  return String(count);
}

/**
 * clusterTier maps a cluster's point count to a size tier (0–2), used to pick
 * the bubble radius and tint. Kept here so the thresholds match the map layer's
 * step expression.
 */
export function clusterTier(count: number): 0 | 1 | 2 {
  if (count >= 100) return 2;
  if (count >= 25) return 1;
  return 0;
}
