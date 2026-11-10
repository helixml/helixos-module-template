/**
 * HelixOS module SDK (browser side) — contract v1 §7.
 *
 * A module page runs inside a host's sandboxed iframe. This file speaks the
 * frame protocol with the host: it learns the host origin from the URL
 * fragment, asks for short-lived user tokens, applies the host theme, keeps
 * the host's address bar in step with the module's navigation, and follows
 * host back/forward. It works with any conforming host (HelixOS, cyber).
 *
 * Copy it into a module unchanged. No dependencies.
 */

export const PROTOCOL = "helixos-module/1";

export interface HostUser {
  id: string;
  email: string;
  name: string;
}

export interface HostToken {
  token: string;
  expires_at: string;
  user: HostUser;
  org: string;
  inst: string;
}

type Listener<T> = (v: T) => void;

/** Thrown by api() for non-2xx answers; `code` is the contract error code. */
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

function decodeClaims(jwt: string): Record<string, unknown> {
  try {
    const p = jwt.split(".")[1].replace(/-/g, "+").replace(/_/g, "/");
    return JSON.parse(atob(p.padEnd(p.length + ((4 - (p.length % 4)) % 4), "=")));
  } catch {
    return {};
  }
}

class HelixOSFrame {
  /** The host page origin, from #host= in the frame URL; null when not framed. */
  readonly hostOrigin: string | null = null;
  private current: HostToken | null = null;
  private waiters: Listener<HostToken>[] = [];
  private tokenListeners = new Set<Listener<HostToken>>();
  private routeListeners = new Set<Listener<{ path: string; query: Record<string, string> }>>();
  private started = false;

  constructor() {
    if (typeof window === "undefined") return;
    const frag = new URLSearchParams(window.location.hash.replace(/^#/, ""));
    if (frag.get("helixos") === "1" && frag.get("host") && window.parent !== window) {
      try {
        this.hostOrigin = new URL(frag.get("host")!).origin;
      } catch {
        this.hostOrigin = null;
      }
    }
  }

  /** True when the page is framed by a host. */
  get framed(): boolean {
    return this.hostOrigin !== null;
  }

  get token(): HostToken | null {
    return this.current;
  }

  /** Starts the handshake. Safe to call more than once. */
  start(): void {
    if (this.started || !this.framed || typeof window === "undefined") return;
    this.started = true;
    window.addEventListener("message", (ev) => this.onMessage(ev));
    this.post({ type: "ready" });
  }

  private onMessage(ev: MessageEvent) {
    if (ev.origin !== this.hostOrigin || ev.source !== window.parent) return;
    const msg = ev.data as { protocol?: string; type?: string; [k: string]: unknown };
    if (!msg || msg.protocol !== PROTOCOL) return;
    switch (msg.type) {
      case "token": {
        const t = msg as unknown as HostToken;
        // Bind the token to this parent: a token minted for another page's
        // origin is discarded (contract §7.1).
        if (decodeClaims(t.token).origin !== this.hostOrigin) return;
        this.current = { token: t.token, expires_at: t.expires_at, user: t.user, org: t.org, inst: t.inst };
        this.waiters.splice(0).forEach((w) => w(this.current!));
        this.tokenListeners.forEach((l) => l(this.current!));
        break;
      }
      case "theme":
        applyTheme(msg.mode as string, msg.tokens as Record<string, string>);
        break;
      case "route":
        this.routeListeners.forEach((l) => l({ path: String(msg.path || "/"), query: (msg.query as Record<string, string>) || {} }));
        break;
    }
  }

  private post(msg: Record<string, unknown>) {
    if (this.hostOrigin) window.parent.postMessage({ protocol: PROTOCOL, ...msg }, this.hostOrigin);
  }

  /** Resolves with a token that is valid for at least 30 seconds. */
  async getToken(): Promise<HostToken> {
    if (!this.framed) throw new ApiError(401, "unauthenticated", "Open this page from your HelixOS workspace.");
    this.start();
    const fresh = this.current && new Date(this.current.expires_at).getTime() - Date.now() > 30_000;
    if (fresh) return this.current!;
    return new Promise((resolve) => {
      this.waiters.push(resolve);
      this.post({ type: "token.request" });
    });
  }

  onToken(l: Listener<HostToken>): () => void {
    this.tokenListeners.add(l);
    return () => this.tokenListeners.delete(l);
  }

  onRoute(l: Listener<{ path: string; query: Record<string, string> }>): () => void {
    this.routeListeners.add(l);
    return () => this.routeListeners.delete(l);
  }

  /** Tell the host the module moved to path?query (updates its address bar). */
  navigate(path: string, query: Record<string, string> = {}) {
    this.post({ type: "navigate", path, query });
  }

  title(text: string) {
    this.post({ type: "title", text });
  }

  toast(text: string, kind: "info" | "success" | "error" = "info") {
    this.post({ type: "toast", kind, text });
  }

  open(url: string) {
    this.post({ type: "open", url });
  }

  /** fetch() against this module's own API as the signed-in person. */
  async api<T>(path: string, init: RequestInit = {}): Promise<T> {
    const t = await this.getToken();
    const headers: Record<string, string> = { ...(init.headers as Record<string, string>), Authorization: `Bearer ${t.token}` };
    if (init.body && !(init.body instanceof FormData) && !headers["Content-Type"]) headers["Content-Type"] = "application/json";
    let res: Response;
    try {
      res = await fetch(path, { ...init, headers });
    } catch {
      // The network failed or the module is restarting (a redeploy, ./dev).
      throw new ApiError(0, "unreachable", "Can't reach this module right now. Check your connection, then try again.");
    }
    const text = await res.text();
    let data: { code?: string; error?: string } | null = null;
    try {
      data = text ? JSON.parse(text) : null;
    } catch {
      // A proxy or gateway answered with a page, not this module's JSON.
      throw new ApiError(res.status, "bad_response", `The module is not answering properly (HTTP ${res.status}). Try again in a moment.`);
    }
    if (!res.ok) throw new ApiError(res.status, data?.code || "error", data?.error || res.statusText);
    return data as T;
  }
}

function applyTheme(mode: string, tokens: Record<string, string>) {
  const root = document.documentElement;
  root.classList.toggle("dark", mode === "dark");
  for (const [k, v] of Object.entries(tokens || {})) {
    if (k.startsWith("--")) root.style.setProperty(k, v);
  }
}

/** The page's single frame client. */
export const helixos = new HelixOSFrame();
