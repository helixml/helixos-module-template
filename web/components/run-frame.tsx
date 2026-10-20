"use client";

import { useEffect, useRef, useState } from "react";

/**
 * Shows a live agent run inline (contract §6.5). The host's run_frame_url
 * carries a 10-minute view token; when the frame reports expiry (or before),
 * fetchRun gets a fresh URL from this module's backend.
 */
export function RunFrame({
  initialUrl,
  fetchUrl,
  height = 640,
}: {
  initialUrl: string;
  fetchUrl: () => Promise<string>;
  height?: number;
}) {
  const [src, setSrc] = useState(initialUrl);
  const fetchRef = useRef(fetchUrl);
  fetchRef.current = fetchUrl;

  useEffect(() => setSrc(initialUrl), [initialUrl]);

  // Run-view tokens last 10 minutes: refresh at 9.
  useEffect(() => {
    const t = setInterval(() => {
      fetchRef.current().then(setSrc).catch(() => {});
    }, 9 * 60 * 1000);
    return () => clearInterval(t);
  }, []);

  return (
    <iframe
      src={src}
      title="Agent run"
      className="run-frame"
      style={{ height }}
      allow="clipboard-read; clipboard-write; fullscreen; autoplay"
      allowFullScreen
    />
  );
}
