/** @type {import('next').NextConfig} */
const nextConfig = {
  // The Go binary serves this static export; trailingSlash makes every page a
  // directory index so a deep link (/settings/) resolves without rewrites.
  output: "export",
  trailingSlash: true,
  images: { unoptimized: true },
  // In `next dev`, proxy the module API to the Go binary on :8080.
  async rewrites() {
    return process.env.NODE_ENV === "production" ? [] : [{ source: "/api/:path*", destination: "http://localhost:8080/api/:path*" }];
  },
};
export default nextConfig;
