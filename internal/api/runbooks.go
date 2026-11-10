package api

import (
	"net/http"

	"github.com/helixml/helixos-module-template/internal/module"
	"github.com/helixml/helixos-module-template/internal/runbook"
	"github.com/pocketbase/pocketbase/core"
)

// The runbooks are the instructions this module's agents run with
// (runbooks/*.md). Agent runs take their system prompt from one, and the UI
// shows them read-only on the Runbooks page and next to every run, filled in
// with the same vars, so a person reads exactly what the agent is given.

// runbookVars fills the runbooks' {{placeholders}}.
func (d *Deps) runbookVars() map[string]string {
	return map[string]string{"module_name": module.Name, "tool_prefix": module.ToolPrefix(d.Cfg.Key)}
}

// runbookText is a runbook's text as an agent gets it.
func (d *Deps) runbookText(key string) string {
	r, ok := runbook.Lookup(key)
	if !ok {
		panic("module: no runbook " + key + " in runbooks/") // a test loads every key used
	}
	return r.Text(d.runbookVars())
}

// GET /api/app/runbooks — every runbook, without its text.
func (d *Deps) listRunbooks(e *core.RequestEvent) error {
	out := []runbook.Runbook{}
	out = append(out, runbook.All()...)
	return e.JSON(http.StatusOK, map[string]interface{}{"runbooks": out})
}

// GET /api/app/runbooks/{key} — one runbook with its markdown.
func (d *Deps) getRunbook(e *core.RequestEvent) error {
	r, ok := runbook.Lookup(e.Request.PathValue("key"))
	if !ok {
		return apiErr(e, http.StatusNotFound, "not_found", "no such runbook")
	}
	return e.JSON(http.StatusOK, map[string]string{
		"key": r.Key, "title": r.Title, "summary": r.Summary, "markdown": r.Text(d.runbookVars()),
	})
}
