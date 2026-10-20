package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Every collection is tenant-owned: it carries host + org, and its API rules
// are nil (superuser only). All reads and writes go through the module's own
// routes, which verify the host's token and scope by (host, org). Additive
// migrations only; never edit an applied file.
func init() {
	m.Register(func(app core.App) error {
		notes := core.NewBaseCollection("notes")
		notes.Fields.Add(
			&core.TextField{Name: "host", Required: true, Max: 300},
			&core.TextField{Name: "org", Required: true, Max: 200},
			&core.TextField{Name: "title", Required: true, Max: 200},
			&core.TextField{Name: "body", Max: 20000},
			&core.TextField{Name: "created_by", Max: 200},
			&core.TextField{Name: "created_by_name", Max: 200},
			&core.TextField{Name: "source", Max: 20}, // ui | agent
			&core.TextField{Name: "run_id", Max: 100},
			&core.TextField{Name: "run_status", Max: 40},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		notes.AddIndex("idx_notes_tenant", false, "host, org, created", "")
		if err := app.Save(notes); err != nil {
			return err
		}

		insts := core.NewBaseCollection("installations")
		insts.Fields.Add(
			&core.TextField{Name: "host", Required: true, Max: 300},
			&core.TextField{Name: "inst", Required: true, Max: 200},
			&core.TextField{Name: "org", Required: true, Max: 200},
			&core.BoolField{Name: "enabled"},
			&core.JSONField{Name: "scopes", MaxSize: 10000},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		insts.AddIndex("idx_installations_host_inst", true, "host, inst", "")
		if err := app.Save(insts); err != nil {
			return err
		}

		seen := core.NewBaseCollection("host_events")
		seen.Fields.Add(
			&core.TextField{Name: "host", Required: true, Max: 300},
			&core.TextField{Name: "event_id", Required: true, Max: 200},
			&core.TextField{Name: "type", Max: 100},
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		seen.AddIndex("idx_host_events_id", true, "host, event_id", "")
		return app.Save(seen)
	}, func(app core.App) error {
		for _, name := range []string{"host_events", "installations", "notes"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		return nil
	})
}
