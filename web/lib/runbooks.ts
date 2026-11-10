import { helixos } from "@/lib/helixos-sdk";

/** A runbook: the instructions an agent runs with (runbooks/*.md in the repo). */
export interface RunbookSummary {
  key: string;
  title: string;
  summary: string;
}

export interface Runbook extends RunbookSummary {
  /** Exactly the text the agent is given. */
  markdown: string;
}

export async function listRunbooks(): Promise<RunbookSummary[]> {
  return (await helixos.api<{ runbooks: RunbookSummary[] }>("/api/app/runbooks")).runbooks || [];
}

export function getRunbook(key: string): Promise<Runbook> {
  return helixos.api<Runbook>(`/api/app/runbooks/${encodeURIComponent(key)}`);
}

export const runbookHref = (key?: string) => (key ? `/runbooks/?key=${encodeURIComponent(key)}` : "/runbooks/");
