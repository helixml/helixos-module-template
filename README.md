# HelixOS module template

Starter for a [HelixOS](https://github.com/helixml/helixos) module: one Go binary
with embedded [PocketBase](https://pocketbase.io) (v0.40.4) that serves its manifest,
API, MCP tools, host events and a statically exported Next.js UI. It is deployed as a
**Helix project web service**, and it implements module contract v1, so it runs under
any conforming host (HelixOS, the cyber portal).

The example is a small **Notes** module that uses every part of the contract:

| Contract surface | Where |
|---|---|
| Manifest `/.well-known/helixos-module.json` | `internal/module/module.go` |
| User tokens (UI → API), verified against the host JWKS | `internal/helixos/verifier.go`, `internal/api/auth.go` |
| Org-scoped data in PocketBase | `migrations/`, `internal/ops/` |
| Agent tools over MCP | `internal/mcp/mcp.go` |
| Host API: whoami, agent runs | `internal/helixos/hostapi.go`, `internal/api/handlers.go` |
| Nested live run view | `web/components/run-frame.tsx` |
| Agent runbooks, shown on a Runbooks page and next to runs | `runbooks/*.md`, `internal/runbook/`, `web/app/runbooks/` |
| Events (`run.completed`, `installation.changed`), deduped | `internal/api/handlers.go` |
| Frame protocol (tokens, theme, navigation) | `web/lib/helixos-sdk.ts` |

## Deploy

Helix boots it from `.helix/startup.sh` (on the project's `helix-specs` branch):
`docker compose -f docker-compose.prod.yml up --build`, data on the durable
`HELIX_WEB_SERVICE_DATA_DIR`. Configuration arrives as Helix project secrets:

| Variable | Meaning |
|---|---|
| `MODULE_KEY` | Module key (defaults to the compiled `module.DefaultKey`) |
| `HELIXOS_ISSUERS` | Host base URLs whose tokens are accepted, comma-separated |
| `HELIXOS_MODULE_CREDENTIALS` | `iss=credential` pairs for host API calls |
| `HELIXOS_MODULE_DEV_CREDENTIALS` | Dev-scoped credentials, used when `HELIXOS_MODULE_CREDENTIALS` is empty (HelixOS gives these to the module's dev task) |
| `HELIXOS_ISSUER_ENDPOINTS` | Optional `iss=url` pairs to reach a host server-to-server |
| `MODULE_PUBLIC_URL` | Public origin for URLs the module hands out. Derived per request from `X-Forwarded-*`/`Host` when unset |
| `MODULE_FILE_SIGNING_KEY` | Signs short-lived file URLs the host downloads. A random per-process key when unset |
| `MODULE_WEB_DEV_PROXY` | Development only (`./dev`): proxy the UI to a `next dev` server instead of serving `web/out` |
| `MODULE_DEV` | `true` re-enables PocketBase's built-in APIs and dashboard. Never in production |

HelixOS's **Modules → New module** creates all of this for you: a Helix project with
a repo from this template, its web service, the secrets, and an agent task to build
what you described.

## Develop

```bash
./dev
```

- **http://localhost:8090** is a local dev host: it frames the module as a signed-in
  dev user and speaks the frame protocol, like HelixOS. Use the module there.
- **http://localhost:8080** is the module. Its UI hot-reloads (`next dev` behind the
  module's own origin), and Go changes rebuild and restart it. Data is in `.dev-data/`.
- **Dev preview**: in HelixOS, a manager of the module's owner org can switch the
  module page to *Dev preview*, which frames the dev task's `:8080` through a Helix
  preview URL inside real HelixOS. Bots, tools and events stay on production.

No HelixOS source or local HelixOS is needed. Details are in [AGENTS.md](AGENTS.md).
Deliver every change as a pull request (`propose_pull_request` from a Helix agent
task), run `./selftest` before it, and use `scripts/update-from-template` to merge
later template releases.
