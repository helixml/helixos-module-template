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
internal/seed/          fixture data shared by the `seed` command, ./dev and the tests
internal/runbook/       parses runbooks/*.md (frontmatter, includes, {{vars}})
runbooks/               agent instructions, one markdown file per runbook (embedded)
internal/devhost/       local fake host (`go run . devhost`): JWKS, tokens, frame page, whoami
internal/integration/   in-process tests against internal/devhost
web/                    Next.js static export; web/lib/helixos-sdk.ts speaks the frame protocol
dev                     the inner dev loop (module :8080, next dev, dev host :8090)
.helix/startup.sh       how Helix boots this: web service, or ./dev in an agent task
scripts/                update-from-template: merge the latest template into this module
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
9. **Agent instructions live in `runbooks/*.md`.** Humans see them on the Runbooks
   page, rendered from the same text the agent is given. Never inline long prompts in
   Go: build a run's instructions from a runbook (`d.runbookText(key)`).

## Runbooks

A runbook is the workflow an agent follows: markdown with a short frontmatter.

```
---
key: notes-agent            # = the file name (runbooks/notes-agent.md)
title: Notes agent
summary: One line on what the agent does.
order: 1                    # optional; sorts the list
---
You help with notes in the {{module_name}} module…
```

- **Runs.** Pass the runbook's text as the run's `system_prompt` and put its key in
  the run's `meta` (`internal/api/handlers.go`, `askAgent`). Per-run facts (the note,
  the role) go in the `prompt`, not the runbook.
- **Vars.** `{{name}}` placeholders are filled from `runbookVars()`
  (`internal/api/runbooks.go`: `module_name`, `tool_prefix`), for the agent and for
  the page alike, so tool names follow `MODULE_KEY`.
- **Shared text** goes in a fragment, a `_name.md` file that is not listed itself,
  pulled in by a line of its own: `<!-- include: _name.md -->`. It is expanded before
  anyone sees the text, so each runbook reads as one complete document.
- **Showing them.** `GET /api/app/runbooks` lists `{key, title, summary}`;
  `GET /api/app/runbooks/{key}` adds `markdown`. Both are read-only and need a user
  token. The **Runbooks** section (`/runbooks`, in the manifest) lists and renders
  them; wherever the UI starts or shows a run, add
  `<ViewRunbookButton runbookKey=…>` (`web/components/runbook-drawer.tsx`) so a person
  can read what the agent will do before and while it runs.
- If an exact prompt matters (a deployed module), pin it with a golden-file test
  before moving it into a runbook, and keep it.

## Running locally

```bash
./dev
```

That is the whole loop. No HelixOS checkout or login is needed. In a Helix agent task
`.helix/startup.sh` has already started it in the background (log:
`/tmp/module-dev.log`; stop it with `kill $(cat .dev-data/dev.pid)`).

| Port | What |
|---|---|
| `:8080` | The module: manifest, API, MCP, and the UI. In dev it reverse-proxies UI paths to `next dev` (`MODULE_WEB_DEV_PROXY`), so edits under `web/` hot-reload on the module's own origin. A `.go` change rebuilds and restarts it. |
| `:8090` | The **local dev host** (`go run . devhost`, `internal/devhost`). Open **http://localhost:8090** in your browser: it frames the module as a signed-in dev user (`Dev User`, org `org_dev`) and speaks the frame protocol, exactly like HelixOS does. |
| `:3000` | `next dev` (loopback only; always go through `:8080`). |

- Data lives in `.dev-data/` and is seeded for `org_dev` on the first run. Delete the
  directory to start over.
- The dev host implements `whoami`. Every other host API call (agent runs, people,
  Slack…) answers `501 not_implemented`; the module must degrade on that anyway (rule 7).
  To exercise those, use Dev preview (below).
- `./dev` trusts the dev host as an issuer for its own processes only. It does **not**
  set `MODULE_DEV`: production security stays on, because `:8080` is what Dev preview
  exposes publicly.
- To call your own API from a terminal, mint a token from the dev host:

  ```bash
  tok=$(curl -s -X POST -H 'Origin: http://localhost:8090' localhost:8090/devhost/token | jq -r .token)
  curl -H "Authorization: Bearer $tok" localhost:8080/api/app/notes
  ```

### Dev preview: your dev server inside real HelixOS

When this repo is a module's dev task in HelixOS, a manager of the module's owner org
can switch the module page from **Production** to **Dev preview**. HelixOS then frames
this task's `:8080` (through a Helix preview URL) inside the real HelixOS UI, with real
user tokens for the owner org, so changes appear there seconds after you save.

- Only the UI and the module's host-API calls use the dev build. Bots, agent tools
  (MCP), events and run views keep using the **production** deployment.
- Host API calls from the dev build use `HELIXOS_MODULE_DEV_CREDENTIALS`, a dev-scoped
  credential that only works for the owner org's installation. The production
  credential never enters the task. HelixOS provides it (and `HELIXOS_ISSUERS`) as task
  secrets; `./dev` picks them up, and derives the issuers from the credential when
  `HELIXOS_ISSUERS` is not set.
- The preview reads and writes `.dev-data/`, the task's throwaway database, under the
  real owner org, so it starts empty: production data is never touched.

## Delivering work

Always deliver with **`propose_pull_request`**, on every kind of repo: GitHub, GitLab
and Helix-hosted repos all have pull requests. Never merge to `main` yourself. Merging
the pull request to `main` redeploys the module's web service.

To pick up a newer template (contract plumbing, `./dev`, `./selftest`), run
`scripts/update-from-template`, resolve any conflicts, and run `./selftest`.

## Before a PR

```bash
./selftest
```

It runs vet, the unit and integration tests, the UI typecheck and build, and a live
smoke test of the real binary. All of it must pass.
