"use client";

import { useCallback, useEffect, useState } from "react";
import { helixos, ApiError } from "@/lib/helixos-sdk";
import { useHost } from "@/components/host-provider";
import { RunFrame } from "@/components/run-frame";

interface Note {
  id: string;
  title: string;
  body: string;
  created_by_name: string;
  source: string;
  run_id?: string;
  run_status?: string;
  created: string;
}

interface Run {
  run_id: string;
  status: string;
  run_frame_url: string;
}

export default function NotesPage() {
  const { user } = useHost();
  const [notes, setNotes] = useState<Note[]>([]);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [run, setRun] = useState<Run | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await helixos.api<{ notes: Note[] }>("/api/app/notes");
      setNotes(res.notes);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    helixos.title("Notes");
    void load();
    const t = setInterval(load, 10_000);
    return () => clearInterval(t);
  }, [load]);

  const add = async () => {
    setBusy(true);
    try {
      await helixos.api("/api/app/notes", { method: "POST", body: JSON.stringify({ title, body }) });
      setTitle("");
      setBody("");
      helixos.toast("Note saved", "success");
      await load();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (id: string) => {
    await helixos.api(`/api/app/notes/${id}`, { method: "DELETE" });
    await load();
  };

  const askAgent = async (id: string) => {
    setBusy(true);
    setError(null);
    try {
      const r = await helixos.api<Run>(`/api/app/notes/${id}/agent`, { method: "POST" });
      setRun(r);
      await load();
    } catch (e) {
      setError(e instanceof ApiError ? `${e.message}` : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="page">
      <h1>Notes</h1>
      <p className="muted">
        {user ? `Signed in as ${user.name || user.email}. ` : "Connecting to your workspace… "}
        Notes are stored in this module and visible only to your organization.
      </p>

      {error && <p className="error">{error}</p>}

      <div className="card">
        <h2>New note</h2>
        <input placeholder="Title" value={title} onChange={(e) => setTitle(e.target.value)} />
        <div style={{ height: 8 }} />
        <textarea placeholder="Text" value={body} onChange={(e) => setBody(e.target.value)} />
        <div style={{ height: 8 }} />
        <button className="primary" disabled={busy || !title.trim()} onClick={add}>
          Save note
        </button>
      </div>

      {run && (
        <div className="card">
          <div className="row">
            <h2 className="grow">Agent run</h2>
            <span className="badge">{run.status}</span>
            <button onClick={() => setRun(null)}>Close</button>
          </div>
          <RunFrame
            initialUrl={run.run_frame_url}
            fetchUrl={async () => (await helixos.api<Run>(`/api/app/runs/${run.run_id}`)).run_frame_url}
          />
        </div>
      )}

      {notes.map((n) => (
        <div className="card" key={n.id}>
          <div className="row">
            <strong className="grow">{n.title}</strong>
            {n.source === "agent" && <span className="badge">agent</span>}
            {n.run_status && <span className="badge">run: {n.run_status}</span>}
            <button disabled={busy} onClick={() => askAgent(n.id)}>
              Ask an agent
            </button>
            <button onClick={() => remove(n.id)}>Delete</button>
          </div>
          {n.body && <pre>{n.body}</pre>}
          <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>
            {n.created_by_name} · {n.created}
          </div>
        </div>
      ))}
      {notes.length === 0 && <p className="muted">No notes yet.</p>}
    </main>
  );
}
