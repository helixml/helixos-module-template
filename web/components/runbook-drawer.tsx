"use client";

import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import Link from "next/link";
import { ApiError } from "@/lib/helixos-sdk";
import { getRunbook, runbookHref, type Runbook } from "@/lib/runbooks";
import { RunbookMarkdown } from "@/components/runbook-markdown";

/**
 * A side drawer showing one runbook in full: the exact instructions an agent
 * runs with. Open it from wherever a run is started or shown, so a person can
 * read what the agent will do (or is doing) without leaving the page.
 */
export function RunbookDrawer({ runbookKey, onClose }: { runbookKey: string; onClose: () => void }) {
  const [rb, setRb] = useState<Runbook | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let live = true;
    getRunbook(runbookKey)
      .then((r) => live && setRb(r))
      .catch((e) => live && setError(e instanceof ApiError ? e.message : String(e)));
    return () => {
      live = false;
    };
  }, [runbookKey]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="drawer" role="dialog" aria-modal="true" aria-label="Runbook">
      <button type="button" className="drawer-backdrop" aria-label="Close runbook" onClick={onClose} />
      <aside className="drawer-panel">
        <header className="drawer-head">
          <div className="grow">
            <div className="eyebrow">Runbook</div>
            <h2>{rb?.title || (error ? "Runbook" : "Loading…")}</h2>
            {rb?.summary && <p className="muted">{rb.summary}</p>}
          </div>
          <Link href={runbookHref(runbookKey)} className="link-button">
            Full page
          </Link>
          <button type="button" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </header>
        <div className="drawer-body">
          {error && <p className="error">{error}</p>}
          {rb && (
            <>
              <p className="callout">This is the exact text the agent is given when a run starts.</p>
              <RunbookMarkdown markdown={rb.markdown} />
            </>
          )}
        </div>
      </aside>
    </div>
  );
}

/** A "View runbook" button that opens the runbook in a drawer. */
export function ViewRunbookButton({ runbookKey, label = "View runbook" }: { runbookKey: string; label?: string }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" className="link-button" onClick={() => setOpen(true)}>
        {label}
      </button>
      {/* Portalled to <body>, so the drawer inherits nothing from where the button sits. */}
      {open && createPortal(<RunbookDrawer runbookKey={runbookKey} onClose={() => setOpen(false)} />, document.body)}
    </>
  );
}
