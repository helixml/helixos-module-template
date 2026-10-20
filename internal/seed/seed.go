// Package seed creates fixture data for local development and tests. The same
// function backs the `seed` command and the integration tests, so tests run
// against exactly what a developer sees. Never point it at production data.
package seed

import (
	"github.com/helixml/helixos-module-template/internal/helixos"
	"github.com/helixml/helixos-module-template/internal/ops"
	"github.com/pocketbase/pocketbase/core"
)

// Seed adds a few notes for tenant.
func Seed(app core.App, t helixos.Tenant) error {
	for _, n := range []ops.CreateNoteInput{
		{Title: "Welcome", Body: "This module stores its data in PocketBase, scoped to your organization.", Source: "ui", By: "seed", ByName: "Seed"},
		{Title: "Try the agent", Body: "Click \"Ask an agent\" on this note: the host runs an agent that writes a follow-up note through this module's MCP tool.", Source: "ui", By: "seed", ByName: "Seed"},
	} {
		if _, err := ops.CreateNote(app, t, n); err != nil {
			return err
		}
	}
	return nil
}
