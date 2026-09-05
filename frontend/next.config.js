/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  images: {
    // Company logos come from arbitrary third-party hosts (job-board CDNs),
    // so we allow any https remote source rather than enumerate them.
    remotePatterns: [{ protocol: 'https', hostname: '**' }],
  },
  async rewrites() {
    // In dev, proxy /api to the Go backend so the frontend can call same-origin
    // paths without CORS friction. Override with NEXT_PUBLIC_API_BASE in prod.
    const backend = process.env.BACKEND_URL || 'http://localhost:8080';
    return [{ source: '/api/:path*', destination: `${backend}/api/:path*` }];
  },
};
module.exports = nextConfig;
