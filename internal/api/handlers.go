package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/helixml/helixos-module-template/internal/helixos"
	"github.com/helixml/helixos-module-template/internal/module"
	"github.com/helixml/helixos-module-template/internal/ops"
	"github.com/pocketbase/pocketbase/core"
)

// GET /api/app/me — who is calling, as the host sees it.
func (d *Deps) me(e *core.RequestEvent) error {
	c := caller(e)
	out := map[string]interface{}{
		"user": map[string]string{"id": c.UserID, "email": c.Email, "name": c.Name},
		"org":  c.Org, "installation": c.Inst, "host": c.Issuer, "module": d.Cfg.Key, "version": module.Version,
	}
	if who, err := d.Host.WhoAmI(e.Request.Context(), c); err == nil {
		out["org_name"], out["scopes"] = who.OrgName, who.Scopes
	} else {
		out["host_error"] = err.Error()
	}
	return e.JSON(http.StatusOK, out)
}

func (d *Deps) listNotes(e *core.RequestEvent) error {
	notes, err := ops.ListNotes(d.App, caller(e).Tenant(), 100)
	if err != nil {
		return fail(e, err)
	}
	return e.JSON(http.StatusOK, map[string]interface{}{"notes": notes})
}

func (d *Deps) createNote(e *core.RequestEvent) error {
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(e.Request.Body, 64<<10)).Decode(&body); err != nil {
		return apiErr(e, http.StatusBadRequest, "invalid_request", "body must be JSON")
	}
	c := caller(e)
	n, err := ops.CreateNote(d.App, c.Tenant(), ops.CreateNoteInput{Title: body.Title, Body: body.Body, Source: "ui", By: c.UserID, ByName: c.Name})
	if err != nil {
		return fail(e, err)
	}
	return e.JSON(http.StatusCreated, n)
}

func (d *Deps) deleteNote(e *core.RequestEvent) error {
	if err := ops.DeleteNote(d.App, caller(e).Tenant(), e.Request.PathValue("id")); err != nil {
		return fail(e, err)
	}
	return e.JSON(http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/app/notes/{id}/agent — ask an agent to work on a note. The host
// owns the agent: the module only sends the instructions and gets a run back,
// whose run_frame_url the UI nests to show the live run.
func (d *Deps) askAgent(e *core.RequestEvent) error {
	c := caller(e)
	rec, err := ops.GetNote(d.App, c.Tenant(), e.Request.PathValue("id"))
	if err != nil {
		return fail(e, err)
	}
	p := module.ToolPrefix(d.Cfg.Key)
	run, err := d.Host.StartRun(e.Request.Context(), c, helixos.RunRequest{
		BotKey: "note-helper-" + safeKey(c.UserID),
		Title:  "Note helper",
		SystemPrompt: "You help with notes in the " + module.Name + " module. Use the " + p + "_note_add tool to save what you produce " +
			"as a new note, and " + p + "_notes_list to read the existing notes.",
		Prompt: fmt.Sprintf("Read the note titled %q and write a short follow-up note with next steps.\n\nNote body:\n%s",
			rec.GetString("title"), rec.GetString("body")),
		ToolGroups: []string{"memory"},
		Meta:       map[string]interface{}{"note_id": rec.Id},
	})
	if err != nil {
		if helixos.NotImplemented(err) {
			return apiErr(e, http.StatusNotImplemented, "not_implemented", "this host cannot run agents")
		}
		return apiErr(e, http.StatusBadGateway, "upstream_failed", err.Error())
	}
	if err := ops.SetNoteRun(d.App, c.Tenant(), rec.Id, run.RunID, run.Status); err != nil {
		log.Printf("module: record run on note %s: %v", rec.Id, err)
	}
	return e.JSON(http.StatusCreated, run)
}

// GET /api/app/runs/{id} — a run with a fresh run_frame_url.
func (d *Deps) getRun(e *core.RequestEvent) error {
	run, err := d.Host.GetRun(e.Request.Context(), caller(e), e.Request.PathValue("id"))
	if err != nil {
		return apiErr(e, http.StatusBadGateway, "upstream_failed", err.Error())
	}
	return e.JSON(http.StatusOK, run)
}

// envelope is a host event (contract §8).
type envelope struct {
	ID   string                 `json:"id"`
	Type string                 `json:"type"`
	Org  string                 `json:"org"`
	Inst string                 `json:"inst"`
	Data map[string]interface{} `json:"data"`
}

// POST /helixos/events
func (d *Deps) events(e *core.RequestEvent) error {
	c := caller(e)
	var ev envelope
	if err := json.NewDecoder(io.LimitReader(e.Request.Body, 1<<20)).Decode(&ev); err != nil || ev.ID == "" {
		return apiErr(e, http.StatusBadRequest, "invalid_request", "bad event")
	}
	if ev.Org != c.Org || ev.Inst != c.Inst {
		return apiErr(e, http.StatusBadRequest, "invalid_request", "event does not match its token")
	}
	first, err := ops.FirstSeen(d.App, c.Issuer, ev.ID, ev.Type)
	if err != nil {
		return fail(e, err)
	}
	if !first {
		return e.JSON(http.StatusOK, map[string]bool{"duplicate": true})
	}
	switch ev.Type {
	case "installation.changed":
		enabled, _ := ev.Data["enabled"].(bool)
		var scopes []string
		if raw, ok := ev.Data["scopes"].([]interface{}); ok {
			for _, s := range raw {
				if str, ok := s.(string); ok {
					scopes = append(scopes, str)
				}
			}
		}
		if err := ops.RecordInstallation(d.App, c.Issuer, c.Inst, c.Org, enabled, scopes); err != nil {
			return fail(e, err)
		}
	case "run.completed":
		runID, _ := ev.Data["run_id"].(string)
		status, _ := ev.Data["status"].(string)
		if err := ops.RunFinished(d.App, c.Tenant(), runID, status); err != nil {
			return fail(e, err)
		}
	}
	return e.JSON(http.StatusOK, map[string]bool{"ok": true})
}

// safeKey squeezes an id into bot_key characters.
func safeKey(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r+'a'-'A')
		default:
			out = append(out, '-')
		}
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return string(out)
}
