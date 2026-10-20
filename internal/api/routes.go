// Package api mounts the module's HTTP surface on PocketBase's router:
// manifest, health, the UI's REST API, MCP, host events and the static UI.
package api

import (
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/helixml/helixos-module-template/cfg"
	"github.com/helixml/helixos-module-template/internal/helixos"
	"github.com/helixml/helixos-module-template/internal/mcp"
	"github.com/helixml/helixos-module-template/internal/module"
	"github.com/helixml/helixos-module-template/internal/ops"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// Deps are what the routes need.
type Deps struct {
	App      core.App
	Cfg      *cfg.Config
	Verifier *helixos.Verifier
	Host     *helixos.HostAPI
	Files    *FileSigner
}

// pocketBaseBuiltins are PocketBase's own APIs and dashboard. Identity comes
// from the host, not PocketBase auth, so outside dev they are not exposed.
var pocketBaseBuiltins = []string{"/_/", "/api/collections/", "/api/files/", "/api/realtime", "/api/settings",
	"/api/backups", "/api/logs", "/api/batch", "/api/crons", "/api/superusers"}

// Register mounts everything. Called from OnServe.
func Register(se *core.ServeEvent, d *Deps) {
	// Module pages exist to be framed by a host (contract §7.1); PocketBase's
	// default X-Frame-Options: SAMEORIGIN would block that. Trust comes from
	// host tokens, not from the framing origin.
	se.Router.BindFunc(func(e *core.RequestEvent) error {
		e.Response.Header().Del("X-Frame-Options")
		e.Response.Header().Del("Cross-Origin-Opener-Policy")
		// Pages and API answers are revalidated on every load, so a redeploy is
		// seen at once; Next's content-hashed assets stay cacheable.
		if !strings.HasPrefix(e.Request.URL.Path, "/_next/static/") {
			e.Response.Header().Set("Cache-Control", "no-cache")
		}
		return e.Next()
	})
	if !d.Cfg.Dev {
		se.Router.BindFunc(func(e *core.RequestEvent) error {
			for _, p := range pocketBaseBuiltins {
				if strings.HasPrefix(e.Request.URL.Path, p) {
					return apiErr(e, http.StatusNotFound, "not_found", "not found")
				}
			}
			return e.Next()
		})
	}

	se.Router.GET("/.well-known/helixos-module.json", func(e *core.RequestEvent) error {
		e.Response.Header().Set("Cache-Control", "no-store")
		return e.JSON(http.StatusOK, module.Build(d.Cfg.Key))
	})
	se.Router.GET("/healthz", func(e *core.RequestEvent) error {
		return e.JSON(http.StatusOK, map[string]string{"status": "ok", "version": module.Version})
	})

	// Host → module.
	se.Router.POST("/mcp", mcp.Handler(d.App, d.Cfg.Key)).BindFunc(d.requireHost(""))
	se.Router.GET("/mcp", func(e *core.RequestEvent) error {
		return apiErr(e, http.StatusMethodNotAllowed, "method_not_allowed", "MCP is JSON-RPC over POST")
	})
	se.Router.POST("/helixos/events", d.events).BindFunc(d.requireHost(""))

	// Signed file downloads for the host (no user token: the signature is the grant).
	se.Router.GET("/files/{token}", d.Files.Serve(d.App))

	// Module UI → module API, as the signed-in person.
	g := se.Router.Group("/api/app")
	g.BindFunc(d.requireUser)
	g.GET("/me", d.me)
	g.GET("/notes", d.listNotes)
	g.POST("/notes", d.createNote)
	g.DELETE("/notes/{id}", d.deleteNote)
	g.POST("/notes/{id}/agent", d.askAgent)
	g.GET("/runs/{id}", d.getRun)

	// Static UI (Next.js export with trailingSlash, so every page is a dir index).
	if _, err := os.Stat(d.Cfg.WebDir); err == nil {
		se.Router.GET("/{path...}", staticUI(d.Cfg.WebDir))
	} else {
		log.Printf("module: no UI build at %s; serving the API only", d.Cfg.WebDir)
	}
}

// fail maps an ops error onto the contract's error shape.
func fail(e *core.RequestEvent, err error) error {
	oe := ops.AsErr(err)
	if oe.Status >= 500 {
		log.Printf("module: %s %s: %v", e.Request.Method, e.Request.URL.Path, err)
	}
	return apiErr(e, oe.Status, oe.Code, oe.Msg)
}

var errNoHostAPI = errors.New("no module credential is configured for this host")

// staticUI serves the Next.js export. PocketBase's Static redirects "/job" to
// "/job/" but drops the query on the way, and HelixOS deep links arrive as
// "/job?id=…" — so the directory redirect is done here, query kept.
func staticUI(dir string) func(*core.RequestEvent) error {
	fsys := os.DirFS(dir)
	serve := apis.Static(fsys, true)
	return func(e *core.RequestEvent) error {
		p := e.Request.URL.Path
		if p != "/" && !strings.HasSuffix(p, "/") {
			if fi, err := fs.Stat(fsys, strings.TrimPrefix(path.Clean(p), "/")); err == nil && fi.IsDir() {
				u := *e.Request.URL
				u.Path = p + "/"
				return e.Redirect(http.StatusFound, u.RequestURI())
			}
		}
		return serve(e)
	}
}
