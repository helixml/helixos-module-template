"use client";

import { createContext, Suspense, useContext, useEffect, useRef, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { helixos, type HostUser } from "@/lib/helixos-sdk";

/**
 * Starts the host handshake once, exposes the signed-in person, and keeps the
 * host's address bar and this app's router in step in both directions.
 */

interface HostState {
  framed: boolean;
  user: HostUser | null;
}

const HostContext = createContext<HostState>({ framed: false, user: null });

export function useHost(): HostState {
  return useContext(HostContext);
}

export function HostProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<HostUser | null>(null);
  const [framed, setFramed] = useState(false);

  useEffect(() => {
    setFramed(helixos.framed);
    if (!helixos.framed) return;
    const off = helixos.onToken((t) => setUser(t.user));
    helixos.start();
    return off;
  }, []);

  if (typeof window !== "undefined" && !framed && !helixos.framed) {
    return (
      <main className="page">
        <div className="card notice">
          <h1>Open this from your workspace</h1>
          <p>This page is a HelixOS module. Open it from the module&apos;s entry in your HelixOS sidebar.</p>
        </div>
      </main>
    );
  }

  return (
    <HostContext.Provider value={{ framed, user }}>
      <Suspense>
        <NavSync />
      </Suspense>
      {children}
    </HostContext.Provider>
  );
}

/** Mirrors navigation: module → host (navigate) and host → module (route). */
function NavSync() {
  const pathname = usePathname();
  const params = useSearchParams();
  const router = useRouter();
  const fromHost = useRef(false);

  useEffect(() => {
    return helixos.onRoute(({ path, query }) => {
      fromHost.current = true;
      const q = new URLSearchParams(query).toString();
      router.replace(q ? `${path}?${q}` : path);
    });
  }, [router]);

  useEffect(() => {
    if (fromHost.current) {
      fromHost.current = false;
      return;
    }
    const query: Record<string, string> = {};
    params.forEach((v, k) => (query[k] = v));
    helixos.navigate(pathname.replace(/\/$/, "") || "/", query);
  }, [pathname, params]);

  return null;
}
