/** @type {import('next').NextConfig} */
const nextConfig = {
  // The Go binary serves this static export; trailingSlash makes every page a
  // directory index so a deep link (/settings/) resolves without rewrites.
  output: "export",
  trailingSlash: true,
  images: { unoptimized: true },
  // ./selftest builds into its own directory: `next build` into .next would
  // overwrite the files a running `next dev` (./dev) is serving. With a
  // static export the site is written there too, instead of out/.
  distDir: process.env.NEXT_DIST_DIR || ".next",
  // In development the Go server on :8080 proxies the UI to `next dev`
  // (MODULE_WEB_DEV_PROXY, see ./dev), so the UI and the API share an origin
  // and no rewrites are needed.
};
export default nextConfig;
