/** @type {import('next').NextConfig} */
const nextConfig = {
  // The Go binary serves this static export; trailingSlash makes every page a
  // directory index so a deep link (/settings/) resolves without rewrites.
  output: "export",
  trailingSlash: true,
  images: { unoptimized: true },
  // In development the Go server on :8080 proxies the UI to `next dev`
  // (MODULE_WEB_DEV_PROXY, see ./dev), so the UI and the API share an origin
  // and no rewrites are needed.
};
export default nextConfig;
