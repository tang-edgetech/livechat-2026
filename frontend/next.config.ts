import type { NextConfig } from "next";

// Proxy /api/* to the Go backend so the browser only ever talks to one
// origin. This sidesteps cross-origin cookie rules entirely (no
// SameSite=None+Secure dance for local HTTP dev) and mirrors how
// production will sit behind a single reverse-proxy domain anyway.
// Backend location is env-driven — overview.md §5/§9 "Port & domain
// flexibility": never hardcoded.
const BACKEND_URL = process.env.BACKEND_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${BACKEND_URL}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
