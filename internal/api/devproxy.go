package api

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// devProxy reverse-proxies UI paths to a `next dev` server (MODULE_WEB_DEV_PROXY),
// including the HMR WebSocket upgrade, so the module's UI hot-reloads on the
// module's own origin: one origin for the host's iframe, the API and the UI.
//
// Only paths no module route claims reach it. /api/ is never proxied, and
// PocketBase's built-ins stay hidden by the middleware in Register.
func devProxy(target string) (func(*core.RequestEvent) error, error) {
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%q is not an http(s) URL", target)
	}
	p := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(u)
			r.SetXForwarded()
			// The dev server is only reachable through this proxy, so it is
			// told the request is same-origin (Next.js warns about, and will
			// block, cross-origin requests to its /_next dev endpoints).
			r.Out.Header.Del("Origin")
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("module: UI dev server %s: %v", target, err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, `<!doctype html><meta http-equiv="refresh" content="2"><p style="font-family:system-ui">The UI dev server is starting… (see ./dev output)</p>`)
		},
	}
	return func(e *core.RequestEvent) error {
		if strings.HasPrefix(e.Request.URL.Path, "/api/") {
			return apiErr(e, http.StatusNotFound, "not_found", "not found")
		}
		// The dev server sets its own caching headers.
		e.Response.Header().Del("Cache-Control")
		if strings.EqualFold(e.Request.Header.Get("Upgrade"), "websocket") {
			// The HMR socket outlives the server's read/write timeouts, which
			// would otherwise still apply to the hijacked connection.
			rc := http.NewResponseController(e.Response)
			_ = rc.SetReadDeadline(time.Time{})
			_ = rc.SetWriteDeadline(time.Time{})
		}
		p.ServeHTTP(e.Response, e.Request)
		return nil
	}, nil
}
