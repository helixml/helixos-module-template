# Working on this module

This repository is a **HelixOS module**: a separately deployed web service that a
host (HelixOS, or the cyber portal) renders inside its UI and offers to its agents.
The contract between module and host is `docs/modules/contract-v1.md` in
`helixml/helixos` (a copy is in [`docs/contract-v1.md`](docs/contract-v1.md)). Read it
before changing anything that crosses the boundary.

## Shape

```
main.go                 PocketBase app: migrations, routes, `seed` command
cfg/                    the only reader of the environment
migrations/             schema as Go migrations (additive only; never edit an applied file)
internal/module/        manifest: key, name, version, sections, scopes, tool groups
internal/helixos/       contract plumbing: token verification, host API client (stdlib only)
internal/ops/           business logic; REST and MCP both call these functions
internal/api/           HTTP routes: UI API (/api/app/*), /mcp, /helixos/events, files, static UI
internal/mcp/           the module's agent tools (JSON-RPC over POST /mcp)
internal/seed/          fixture data shared by the `seed` command and the tests
internal/integration/   in-process tests against a fake host
web/                    Next.js static export; web/lib/helixos-sdk.ts speaks the frame protocol
.helix/startup.sh       how Helix boots this as a web service
selftest                run this before every PR
```

## Rules

1. **Every record is tenant-owned.** Store `host` (the token issuer) and `org` on
   every row and filter every query by both (`ops.tenantFilter`). A record of another
   tenant must look exactly like a missing one (404).
2. **Collections have nil API rules.** PocketBase's own collection, file and realtime
   APIs are switched off outside dev (`internal/api/routes.go`). All access goes
   through this module's routes, which verify the host's token.
3. **Identity comes from the host.** UI routes use `requireUser`; host calls (MCP,
   events) use `requireHost`. Never accept an org, user or installation id from a
   request body.
4. **REST and MCP share `ops`.** Add a capability once in `internal/ops`, then expose
   it as a route and/or a tool.
5. **Tool names carry the module prefix** (`module.ToolPrefix(key) + "_…"`): they must
   be unique across the host's whole catalogue.
6. **The host owns agents.** To run an agent, call `HostAPI.StartRun`; show the run by
   nesting its `run_frame_url` (`web/components/run-frame.tsx`). Never embed a Helix
   URL or key yourself.
7. **Degrade on `not_implemented`.** Hosts differ (cyber has no Slack); check
   `helixos.NotImplemented(err)` and carry on.
8. **Migrations are additive.** Add a new file; never edit or delete an applied one.

## Running locally

```bash
# 1. a host: HelixOS dev stack on http://localhost:8090 (see helixos/README)
# 2. the module
HELIXOS_ISSUERS=http://localhost:8090 \
HELIXOS_MODULE_CREDENTIALS=http://localhost:8090=<credential from registration> \
MODULE_DEV=true go run . serve --http 0.0.0.0:4100
go run . seed --host http://localhost:8090 --org <your org id>
# 3. the UI, rebuilt on change (served by the binary from web/out)
npm --prefix web run build
# 4. register it in HelixOS: Modules → Register by URL → http://localhost:4100, then Install
```

In a Helix spec-task sandbox the host may not be reachable at its public URL; set
`HELIXOS_ISSUER_ENDPOINTS=<public iss>=<reachable url>`. For calling your own API from
a terminal, mint an 8-hour developer token in HelixOS (module page → Developer token)
and send it as `Authorization: Bearer`.

## Before a PR

```bash
./selftest
```

It runs vet, the unit and integration tests, the UI typecheck and build, and a live
smoke test of the real binary. All of it must pass.
