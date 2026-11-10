package devhost

import (
	"net/http"
)

// page is the host shell: a header and the module in a sandboxed iframe,
// speaking the frame protocol (contract §7). The module path lives in the
// address bar as ?p=<path>?<query>, so reload and back/forward work.
func (h *Host) page(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(pageHTML))
}

const pageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>HelixOS dev host</title>
<style>
  :root { color-scheme: light dark; font-family: system-ui, sans-serif; }
  * { box-sizing: border-box; }
  body { margin: 0; height: 100vh; display: flex; flex-direction: column; background: #f4f4f5; color: #18181b; }
  body.dark { background: #09090b; color: #fafafa; }
  header { display: flex; gap: 12px; align-items: center; padding: 8px 12px; border-bottom: 2px dashed #f59e0b; font-size: 13px; }
  header b { font-size: 14px; }
  header .grow { flex: 1; }
  header code { background: rgba(127,127,127,.15); padding: 2px 6px; border-radius: 4px; }
  header button { font: inherit; padding: 3px 10px; cursor: pointer; }
  iframe { flex: 1; border: 0; width: 100%; background: #fff; }
  #toasts { position: fixed; right: 12px; bottom: 12px; display: flex; flex-direction: column; gap: 6px; }
  .toast { padding: 8px 12px; border-radius: 6px; background: #18181b; color: #fff; font-size: 13px; }
  .toast.error { background: #dc2626; } .toast.success { background: #16a34a; }
</style>
</head>
<body>
<header>
  <b>HelixOS dev host</b>
  <span id="who"></span>
  <span class="grow"></span>
  <span>module <code id="path">/</code></span>
  <button id="theme" type="button">Dark</button>
  <button id="reload" type="button">Reload frame</button>
</header>
<iframe id="frame" title="module"
  sandbox="allow-scripts allow-forms allow-same-origin allow-popups allow-downloads allow-modals"
  allow="clipboard-read; clipboard-write; fullscreen; autoplay"></iframe>
<div id="toasts"></div>
<script>
(async () => {
  const PROTOCOL = "helixos-module/1";
  const cfg = await (await fetch("/devhost/config")).json();
  const moduleOrigin = new URL(cfg.module_url).origin;
  const frame = document.getElementById("frame");
  let mode = "light", token = null, refreshTimer = null;
  document.getElementById("who").textContent = cfg.user.name + " <" + cfg.user.email + "> · " + cfg.org_name + " (" + cfg.org + ") · " + cfg.module_key;

  const themes = {
    light: { "--background": "#ffffff", "--foreground": "#0a0a0a", "--card": "#ffffff", "--card-foreground": "#0a0a0a",
      "--primary": "#4f46e5", "--primary-foreground": "#ffffff", "--muted": "#f4f4f5", "--muted-foreground": "#71717a",
      "--accent": "#f4f4f5", "--accent-foreground": "#0a0a0a", "--border": "#e4e4e7", "--destructive": "#dc2626", "--radius": "0.5rem" },
    dark: { "--background": "#09090b", "--foreground": "#fafafa", "--card": "#18181b", "--card-foreground": "#fafafa",
      "--primary": "#818cf8", "--primary-foreground": "#0a0a0a", "--muted": "#27272a", "--muted-foreground": "#a1a1aa",
      "--accent": "#27272a", "--accent-foreground": "#fafafa", "--border": "#27272a", "--destructive": "#ef4444", "--radius": "0.5rem" },
  };

  function modulePath() {
    return new URLSearchParams(location.search).get("p") || "/";
  }
  function splitPath(p) {
    const u = new URL(p, "http://x");
    const query = {};
    u.searchParams.forEach((v, k) => (query[k] = v));
    return { path: u.pathname, query };
  }
  function post(msg) {
    frame.contentWindow && frame.contentWindow.postMessage(Object.assign({ protocol: PROTOCOL }, msg), moduleOrigin);
  }
  async function sendToken() {
    const res = await fetch("/devhost/token", { method: "POST" });
    token = await res.json();
    post(Object.assign({ type: "token" }, token));
    clearTimeout(refreshTimer);
    const ms = new Date(token.expires_at).getTime() - Date.now() - 60000;
    refreshTimer = setTimeout(sendToken, Math.max(ms, 5000));
  }
  function sendTheme() {
    document.body.classList.toggle("dark", mode === "dark");
    document.getElementById("theme").textContent = mode === "dark" ? "Light" : "Dark";
    post({ type: "theme", mode, tokens: themes[mode] });
  }
  function sendRoute() {
    post(Object.assign({ type: "route" }, splitPath(modulePath())));
  }
  function load() {
    const p = modulePath();
    document.getElementById("path").textContent = p;
    frame.src = moduleOrigin + p + "#helixos=1&host=" + encodeURIComponent(location.origin);
  }
  function toast(kind, text) {
    const el = document.createElement("div");
    el.className = "toast " + (kind || "info");
    el.textContent = text;
    document.getElementById("toasts").appendChild(el);
    setTimeout(() => el.remove(), 4000);
  }

  window.addEventListener("message", (ev) => {
    if (ev.origin !== moduleOrigin || ev.source !== frame.contentWindow) return;
    const m = ev.data;
    if (!m || m.protocol !== PROTOCOL) return;
    switch (m.type) {
      case "ready": sendToken(); sendTheme(); sendRoute(); break;
      case "token.request": sendToken(); break;
      case "navigate": {
        const q = new URLSearchParams(m.query || {}).toString();
        const p = (m.path || "/") + (q ? "?" + q : "");
        document.getElementById("path").textContent = p;
        if (p !== modulePath()) history.pushState(null, "", "/?p=" + encodeURIComponent(p));
        break;
      }
      case "title": document.title = (m.text ? m.text + " · " : "") + "HelixOS dev host"; break;
      case "toast": toast(m.kind, m.text); break;
      case "open": if (/^https?:/.test(String(m.url))) window.open(m.url, "_blank", "noopener"); break;
      case "resize": break;
    }
  });
  window.addEventListener("popstate", () => { document.getElementById("path").textContent = modulePath(); sendRoute(); });
  window.addEventListener("focus", () => post({ type: "active" }));
  document.getElementById("theme").onclick = () => { mode = mode === "dark" ? "light" : "dark"; sendTheme(); };
  document.getElementById("reload").onclick = load;
  load();
})();
</script>
</body>
</html>
`
