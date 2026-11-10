// Command module is a HelixOS module: one binary with embedded PocketBase that
// serves its manifest, API, MCP endpoint, host events and static UI.
//
//	module serve --http 0.0.0.0:8080 --dir ./pb_data
//	module seed --host http://localhost:8090 --org org_dev
//	module devhost   # local fake host for development (see ./dev)
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/helixml/helixos-module-template/cfg"
	"github.com/helixml/helixos-module-template/internal/api"
	"github.com/helixml/helixos-module-template/internal/devhost"
	"github.com/helixml/helixos-module-template/internal/helixos"
	"github.com/helixml/helixos-module-template/internal/module"
	"github.com/helixml/helixos-module-template/internal/seed"
	_ "github.com/helixml/helixos-module-template/migrations"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/spf13/cobra"
)

func main() {
	// The dev host is a separate process with no PocketBase and no module
	// configuration of its own.
	if len(os.Args) > 1 && os.Args[1] == "devhost" {
		if err := devhost.Main(os.Args[2:], module.DefaultKey); err != nil {
			log.Fatal(err)
		}
		return
	}

	app := pocketbase.New()
	// Schema is code: the dashboard must never generate migrations.
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{Automigrate: false})

	config, err := cfg.Load(module.DefaultKey)
	if err != nil {
		log.Fatal(err)
	}
	deps := NewDeps(app, config)

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// Modules have no PocketBase superusers (identity comes from the host),
		// so never open PocketBase's first-run installer.
		se.InstallerFunc = nil
		api.Register(se, deps)
		return se.Next()
	})
	app.RootCmd.AddCommand(seedCommand(app))

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

// NewDeps wires the module's collaborators.
func NewDeps(app core.App, c *cfg.Config) *api.Deps {
	httpClient := &http.Client{Timeout: 60 * time.Second}
	return &api.Deps{
		App: app,
		Cfg: c,
		Verifier: helixos.NewVerifier(helixos.Settings{
			ModuleKey: c.Key, Trusts: c.Trusts, HostBase: c.HostBase, HTTP: httpClient,
		}),
		Host:  &helixos.HostAPI{Credential: func(iss string) string { return c.Credentials[iss] }, HostBase: c.HostBase, HTTP: httpClient},
		Files: api.NewFileSigner(c.FileSigningKey),
	}
}

func seedCommand(app core.App) *cobra.Command {
	var host, org string
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Create fixture data for one tenant (development only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Bootstrap(); err != nil {
				return err
			}
			if err := app.RunAllMigrations(); err != nil {
				return err
			}
			return seed.Seed(app, helixos.Tenant{Host: host, Org: org})
		},
	}
	cmd.Flags().StringVar(&host, "host", "http://localhost:8090", "host issuer URL")
	cmd.Flags().StringVar(&org, "org", "org_dev", "org id")
	return cmd
}
