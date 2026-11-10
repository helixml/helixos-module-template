"use client";

import { Suspense, useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { helixos, ApiError } from "@/lib/helixos-sdk";
import { getRunbook, listRunbooks, runbookHref, type Runbook, type RunbookSummary } from "@/lib/runbooks";
import { RunbookMarkdown } from "@/components/runbook-markdown";

// The Runbooks page: every workflow this module's agents follow, readable in
// full. Query-param route (?key=<runbook>), as a static export needs.
function RunbooksInner() {
  const key = useSearchParams().get("key") || "";
  return key ? <RunbookPage key={key} runbookKey={key} /> : <RunbookList />;
}

export default function RunbooksPage() {
  return (
    <Suspense fallback={null}>
      <RunbooksInner />
    </Suspense>
  );
}

const message = (e: unknown) => (e instanceof ApiError ? e.message : String(e));

function RunbookList() {
  const [runbooks, setRunbooks] = useState<RunbookSummary[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    helixos.title("Runbooks");
    listRunbooks()
      .then(setRunbooks)
      .catch((e) => setError(message(e)));
  }, []);

  return (
    <main className="page reading">
      <h1>Runbooks</h1>
      <p className="muted">
        The workflow each agent in this module follows. When an agent run starts, the agent is given its runbook word
        for word.
      </p>
      {error && <p className="error">{error}</p>}
      {runbooks?.map((r) => (
        <Link key={r.key} href={runbookHref(r.key)} className="card list-link">
          <strong>{r.title}</strong>
          <span className="muted">{r.summary}</span>
        </Link>
      ))}
      {runbooks?.length === 0 && <p className="muted">No runbooks yet.</p>}
    </main>
  );
}

function RunbookPage({ runbookKey }: { runbookKey: string }) {
  const [rb, setRb] = useState<Runbook | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    getRunbook(runbookKey)
      .then((r) => {
        setRb(r);
        helixos.title(r.title);
      })
      .catch((e) => setError(message(e)));
  }, [runbookKey]);

  return (
    <main className="page reading">
      <Link href={runbookHref()} className="back-link">
        ← All runbooks
      </Link>
      {error && <p className="error">{error}</p>}
      {rb && (
        <article>
          <header className="article-head">
            <div className="eyebrow">Runbook</div>
            <h1>{rb.title}</h1>
            <p className="muted">{rb.summary}</p>
            <p className="muted small">This is the exact text the agent is given when a run starts.</p>
          </header>
          <RunbookMarkdown markdown={rb.markdown} />
        </article>
      )}
    </main>
  );
}
