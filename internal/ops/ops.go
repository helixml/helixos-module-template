// Package ops is the module's business logic. REST routes and MCP tools are
// thin wrappers over these functions, so both surfaces behave identically and
// every tenant check lives in one place.
package ops

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helixos-module-template/internal/helixos"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Err is a failure safe to show the caller.
type Err struct {
	Status int
	Code   string
	Msg    string
}

func (e *Err) Error() string { return e.Msg }

func bad(f string, a ...interface{}) *Err {
	return &Err{Status: http.StatusBadRequest, Code: "invalid_request", Msg: fmt.Sprintf(f, a...)}
}

var errNotFound = &Err{Status: http.StatusNotFound, Code: "not_found", Msg: "not found"}

// AsErr converts any error into an *Err (unknown errors become a 500 with a
// generic message; the detail belongs in logs).
func AsErr(err error) *Err {
	var e *Err
	if errors.As(err, &e) {
		return e
	}
	return &Err{Status: http.StatusInternalServerError, Code: "error", Msg: "internal error"}
}

// Note is the API shape of a note.
type Note struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Body          string `json:"body"`
	CreatedBy     string `json:"created_by"`
	CreatedByName string `json:"created_by_name"`
	Source        string `json:"source"`
	RunID         string `json:"run_id,omitempty"`
	RunStatus     string `json:"run_status,omitempty"`
	Created       string `json:"created"`
}

func toNote(r *core.Record) Note {
	return Note{ID: r.Id, Title: r.GetString("title"), Body: r.GetString("body"), CreatedBy: r.GetString("created_by"),
		CreatedByName: r.GetString("created_by_name"), Source: r.GetString("source"), RunID: r.GetString("run_id"),
		RunStatus: r.GetString("run_status"), Created: r.GetDateTime("created").String()}
}

// tenantFilter is the (host, org) clause every query starts from.
const tenantFilter = "host = {:host} && org = {:org}"

func tenantParams(t helixos.Tenant, extra dbx.Params) dbx.Params {
	p := dbx.Params{"host": t.Host, "org": t.Org}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

// ListNotes returns the tenant's notes, newest first.
func ListNotes(app core.App, t helixos.Tenant, limit int) ([]Note, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	recs, err := app.FindRecordsByFilter("notes", tenantFilter, "-created", limit, 0, tenantParams(t, nil))
	if err != nil {
		return nil, err
	}
	out := make([]Note, 0, len(recs))
	for _, r := range recs {
		out = append(out, toNote(r))
	}
	return out, nil
}

// GetNote returns one of the tenant's notes. A note of another tenant is
// indistinguishable from a missing one.
func GetNote(app core.App, t helixos.Tenant, id string) (*core.Record, error) {
	r, err := app.FindFirstRecordByFilter("notes", tenantFilter+" && id = {:id}", tenantParams(t, dbx.Params{"id": id}))
	if err != nil {
		return nil, errNotFound
	}
	return r, nil
}

// CreateNoteInput is what a caller supplies.
type CreateNoteInput struct {
	Title  string
	Body   string
	Source string
	By     string
	ByName string
}

// CreateNote stores a note for the tenant.
func CreateNote(app core.App, t helixos.Tenant, in CreateNoteInput) (*Note, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return nil, bad("title is required")
	}
	if len(in.Title) > 200 {
		return nil, bad("title must be at most 200 characters")
	}
	if len(in.Body) > 20000 {
		return nil, bad("body must be at most 20000 characters")
	}
	col, err := app.FindCollectionByNameOrId("notes")
	if err != nil {
		return nil, err
	}
	r := core.NewRecord(col)
	r.Set("host", t.Host)
	r.Set("org", t.Org)
	r.Set("title", in.Title)
	r.Set("body", in.Body)
	r.Set("source", in.Source)
	r.Set("created_by", in.By)
	r.Set("created_by_name", in.ByName)
	if err := app.Save(r); err != nil {
		return nil, err
	}
	n := toNote(r)
	return &n, nil
}

// DeleteNote removes one of the tenant's notes.
func DeleteNote(app core.App, t helixos.Tenant, id string) error {
	r, err := GetNote(app, t, id)
	if err != nil {
		return err
	}
	return app.Delete(r)
}

// SetNoteRun records the agent run working on a note.
func SetNoteRun(app core.App, t helixos.Tenant, id, runID, status string) error {
	r, err := GetNote(app, t, id)
	if err != nil {
		return err
	}
	r.Set("run_id", runID)
	r.Set("run_status", status)
	return app.Save(r)
}

// RunFinished updates whichever note a finished run belonged to.
func RunFinished(app core.App, t helixos.Tenant, runID, status string) error {
	recs, err := app.FindRecordsByFilter("notes", tenantFilter+" && run_id = {:run}", "", 10, 0, tenantParams(t, dbx.Params{"run": runID}))
	if err != nil {
		return err
	}
	for _, r := range recs {
		r.Set("run_status", status)
		if err := app.Save(r); err != nil {
			return err
		}
	}
	return nil
}

// RecordInstallation mirrors installation.changed.
func RecordInstallation(app core.App, host, inst, org string, enabled bool, scopes []string) error {
	r, err := app.FindFirstRecordByFilter("installations", "host = {:host} && inst = {:inst}", dbx.Params{"host": host, "inst": inst})
	if err != nil {
		col, cerr := app.FindCollectionByNameOrId("installations")
		if cerr != nil {
			return cerr
		}
		r = core.NewRecord(col)
		r.Set("host", host)
		r.Set("inst", inst)
	}
	r.Set("org", org)
	r.Set("enabled", enabled)
	r.Set("scopes", scopes)
	return app.Save(r)
}

// FirstSeen records an event id, returning false if it was already handled
// (events are delivered at least once).
func FirstSeen(app core.App, host, eventID, typ string) (bool, error) {
	if _, err := app.FindFirstRecordByFilter("host_events", "host = {:host} && event_id = {:id}", dbx.Params{"host": host, "id": eventID}); err == nil {
		return false, nil
	}
	col, err := app.FindCollectionByNameOrId("host_events")
	if err != nil {
		return false, err
	}
	r := core.NewRecord(col)
	r.Set("host", host)
	r.Set("event_id", eventID)
	r.Set("type", typ)
	if err := app.Save(r); err != nil {
		// A concurrent delivery of the same event lost the unique index race.
		return false, nil
	}
	return true, nil
}
