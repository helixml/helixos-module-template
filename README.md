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
| `HELIXOS_ISSUER_ENDPOINTS` | Optional `iss=url` pairs to reach a host server-to-server |
| `MODULE_FILE_SIGNING_KEY` | Signs short-lived file URLs the host downloads |
| `MODULE_DEV` | `true` re-enables PocketBase's built-in APIs and dashboard. Never in production |

HelixOS's **Modules → New module** creates all of this for you: a Helix project with
a repo from this template, its web service, the secrets, and an agent task to build
what you described.

## Develop

See [AGENTS.md](AGENTS.md). Run `./selftest` before every PR.
