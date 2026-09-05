import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'job-find — the world map of open roles',
  description:
    'Explore a live world map where every pin is a company office. Click a pin, see every open role there. Map-first job discovery.',
  metadataBase: new URL('https://job-find.xyz'),
  openGraph: {
    title: 'job-find',
    description: 'A worldwide, map-based job discovery platform.',
    url: 'https://job-find.xyz',
    siteName: 'job-find',
    type: 'website',
  },
  icons: { icon: '/job_find_logo.png' },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        {/* Space Grotesk — geometric UI face, loaded once. */}
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="" />
        <link
          href="https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;500;600;700&display=swap"
          rel="stylesheet"
        />
        {/* MapLibre GL stylesheet. */}
        <link
          href="https://unpkg.com/maplibre-gl@4.7.1/dist/maplibre-gl.css"
          rel="stylesheet"
        />
      </head>
      <body>{children}</body>
    </html>
  );
}
