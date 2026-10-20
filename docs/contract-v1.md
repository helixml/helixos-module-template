# Module contract v1

Status: **v1 draft, implemented by HelixOS** (branch `feature/003785-helixos-module-system-on`).
Design rationale: helix-specs `design/tasks/003785_we-need-to-sort-this-out/design.md`.

This document is the whole interface between a **module** and a **host**. It is
written so that more than one host can implement it (HelixOS today; the cyber
portal is being built against it in parallel). A module that follows it must run
unchanged under any conforming host. Where something is specific to HelixOS it is
marked **[HelixOS]**; a host may omit those parts.

The key words MUST, SHOULD and MAY are used as in RFC 2119.

---

## 1. Roles and shape

- A **module** is a separately deployed HTTPS web service (normally a Helix
  project web service). It owns its pages, API, data and agent tools.
- A **host** owns identity, tenancy (orgs), navigation, agent dispatch, shared
  queues and integrations. It renders module pages inside its own UI and offers
  module tools to its agents.
- An **installation** enables one module in one org of one host, with a set of
  **granted scopes**. Its id is opaque (HelixOS: `inst_…`).
- One module deployment MAY serve many orgs and many hosts. Every request it
  receives says which host (`iss`), org (`org`) and installation (`inst`) it is
  for. **A module MUST scope all of its data by `(iss, org)`**, and MUST never
  return data from one org to a request for another.

```
 host UI ──iframe──▶ module UI ──Bearer user token──▶ module API
   │                                                   │
   │◀──── host API (module credential + installation) ─┘
   │
   ├──── MCP tools/list, tools/call (host-call token) ──▶ module /mcp
   └──── events (host-call token) ─────────────────────▶ module /helixos/events
```

## 2. Discovery and trust

### 2.1 What a module is configured with

A module deployment is configured with:

| Setting | Meaning |
|---|---|
| **Trusted issuers** | The host base URLs it accepts tokens from, e.g. `https://os.helix.ml`. Env convention: `HELIXOS_ISSUERS`, comma-separated. A token whose `iss` is not in this list MUST be rejected. |
| **Module credential(s)** | One secret per host, issued by that host at registration (§6.1). Env convention: `HELIXOS_MODULE_CREDENTIALS`, `iss=credential` pairs, comma-separated. |

Optionally, **issuer endpoints** (`HELIXOS_ISSUER_ENDPOINTS`, `iss=url` pairs) give a
different base URL to reach a host at server-to-server, for development networks
where the host's public URL is not routable from the module. Tokens are still checked
against the public `iss`.

From the issuer alone a module derives every host URL it needs:

| URL | Purpose |
|---|---|
| `<iss>/.well-known/jwks.json` | Public keys that verify host tokens |
| `<iss>/api/module-host/v1/…` | Host API (§6) |

`<iss>` is the exact `iss` claim string, with no trailing slash.

### 2.2 What a host is configured with

A host registers a module by its **base URL** (its public origin, e.g.
`https://candidate-search.apps.helix.ml`) and fetches its manifest (§3). HelixOS
also records the Helix project that serves the module. A host MUST refuse a module
whose origin shares a registrable domain with the host's own UI (cookie tossing,
same-site request forgery), and SHOULD serve module UIs from a domain that is not
same-site with any host.

## 3. Manifest

`GET <module>/.well-known/helixos-module.json` — public, no auth, `application/json`.
JSON Schema: [`manifest.schema.json`](manifest.schema.json).

```json
{
  "schema_version": 1,
  "key": "candidate-search",
  "name": "Candidate Search",
  "version": "1.4.0+3f2c9a1",
  "description": "Source candidates for client jobs with LinkedIn + Bullhorn agents.",
  "icon": "UserSearch",
  "sections": [
    { "key": "main", "label": "Candidate Search", "icon": "UserSearch", "path": "/" }
  ],
  "mcp": { "path": "/mcp" },
  "events": {
    "path": "/helixos/events",
    "subscribe": ["run.completed", "installation.changed", "findai.lead_adopted"]
  },
  "host_scopes": ["identity.read", "agents.run", "integrations.bullhorn",
                  "notify.slack", "actions.read", "engagements.read", "engagements.post"],
  "agent_tool_groups": ["bullhorn", "memory", "engagement", "human_attention"],
  "required_integrations": ["bullhorn", "linkedin"],
  "health": "/healthz"
}
```

| Field | Rules |
|---|---|
| `schema_version` | MUST be `1`. |
| `key` | `^[a-z][a-z0-9-]{1,38}[a-z0-9]$`. Unique per host, immutable. Used in URLs, the tool group `module:<key>` and the token audience `module:<key>`. |
| `name` | 1–80 chars. |
| `version` | Non-empty, ≤ 64 chars. Any change makes the host re-fetch the tool list. Include the git SHA so every deploy changes it. |
| `icon` | Optional. A [Lucide](https://lucide.dev) icon name in PascalCase. Hosts MAY fall back to a default. |
| `sections` | ≥ 1 entry. `key` matches `^[a-z0-9-]{1,40}$` and is unique within the manifest. `path` starts with `/`, is relative to the module origin, and has no scheme, host, `..`, `#` or `?`. `label` 1–40 chars. |
| `mcp.path` | Optional. Present iff the module exposes agent tools (§5). |
| `events.path`, `events.subscribe` | Optional. Event types from §8. Unknown types are ignored by the host. |
| `host_scopes` | Scopes from §6.2 the module will request. The host grants a subset per installation. |
| `agent_tool_groups` | Host tool groups the module's agent runs may use (§6.4). Host-defined names; a host rejects names it does not know. |
| `required_integrations` | Informational: integrations the org must have configured. |
| `health` | Optional path that answers `200` when the module is up. Default `/healthz`. |

Paths in `mcp`, `events` and `health` follow the same rules as section paths.

A host MUST reject a manifest that fails these rules and keep the last valid one in
effect. When `host_scopes` grows, the new scopes stay **ungranted** until an org
owner approves them; existing grants are never widened automatically.

## 4. Tokens

### 4.1 Format

All host tokens are compact JWS (JWT), **ES256 only**:

- Header: `{"alg":"ES256","typ":"JWT","kid":"<key id>"}`. Verifiers MUST reject any
  other `alg`.
- Signature: the 64-byte big-endian concatenation `R || S` (RFC 7518 §3.4), base64url
  without padding. Not DER.
- Keys: `<iss>/.well-known/jwks.json` returns `{"keys":[{"kty":"EC","crv":"P-256","x","y","kid","use":"sig","alg":"ES256"}]}`.
  A host keeps retired keys published until every token they signed has expired.
  Modules SHOULD cache the JWKS for up to 10 minutes and re-fetch once on an
  unknown `kid`.
- Clock skew: verifiers allow **30 seconds** of leeway on `exp` and `nbf`.

### 4.2 Claims

| Claim | Meaning |
|---|---|
| `iss` | Host base URL. MUST be a trusted issuer (§2.1). |
| `aud` | `module:<key>` for tokens a module receives. |
| `typ` | `user`, `host` or `run`. Verifiers MUST check it, so a token minted for one use can't be replayed as another. |
| `org` | Org id. Always present and non-empty. |
| `inst` | Installation id. Present on every `user` and `run` token, and on `host` tokens tied to an installation. |
| `sub` | User id (when a person is involved). |
| `email`, `name` | The user's email and display name (`user` tokens only). |
| `brand` | Host brand/tenant id (`user` tokens only, informational). |
| `origin` | The host page origin the token was minted for (`user` tokens only; §7.1). |
| `bot` | Calling bot's slug (`host` tokens on MCP calls). |
| `run` | Run id (`host` tokens on MCP calls from a run; `run` tokens). |
| `purpose` | Optional; set on special host calls, e.g. `migrate` (§9). |
| `jti` | Unique id (`host` tokens). |
| `iat`, `nbf`, `exp` | Seconds since epoch. |

### 4.3 The three tokens

| `typ` | Issued for | TTL | Carried |
|---|---|---|---|
| `user` | A signed-in person viewing a module page | 5 min | Host → module UI by `postMessage` (§7); module UI → module API as `Authorization: Bearer <jwt>`. |
| `host` | A host → module server call (MCP, events, migration) | 60 s | `Authorization: Bearer <jwt>` on the request to the module. |
| `run` | Viewing one live agent run inside a run frame (§6.5) | 10 min | In the run frame URL. Verified by the **host**, never by the module. |

Module verification checklist, for `user` and `host` tokens:
1. Signature with the JWKS of `iss`, and `iss` trusted.
2. `aud` = `module:<its own key>`, `typ` as expected, within `nbf`/`exp`.
3. `org` non-empty. `inst` present where required.
4. For `host` tokens: reject a `jti` already seen within its lifetime (replay).

A module SHOULD remember the `(iss, inst) → org` pairs it has seen and reject a
token whose `inst` maps to a different `org` than before.

## 5. Module MCP (host → module)

The module serves MCP at `POST <module><mcp.path>` as JSON-RPC 2.0 over plain HTTP
POST (one request, one JSON response; no SSE), MCP protocol version `2024-11-05`.
Every request carries a `host` token. The module MUST implement:

- `initialize` → `{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"<key>","version":"<version>"}}`
- `tools/list` → `{"tools":[{"name","description","inputSchema"}]}`. `inputSchema` is
  a JSON Schema object.
- `tools/call` with `{"name","arguments"}` → `{"content":[{"type":"text","text":"…"}],"isError":false}`

Host behaviour:
- At install and on every version change the host calls `tools/list` with a `host`
  token **without** `bot` and caches the result as tool group `module:<key>`.
- Tool names MUST be unique across the host's whole catalogue (host tools and every
  other module's). The host refuses a version whose tools collide. Modules SHOULD
  prefix tool names with a short module-specific stem (e.g. `candidate_upsert`).
- The host lists a module's tools only to bots in orgs with an **enabled**
  installation, and only to bots granted `module:<key>`.
- On `tools/call` the host checks the grant, then forwards the JSON-RPC request
  unchanged with a fresh `host` token carrying `org`, `inst`, `bot`, `run` and `sub`
  (the run's owner), and passes the module's response back verbatim. Call timeout:
  **60 s**.
- While a module is unhealthy the host MAY omit its tools from `tools/list`.

## 6. Host API (module → host)

Base: `<iss>/api/module-host/v1`. JSON in and out.

### 6.1 Authentication

Every request carries:

| Header | Value |
|---|---|
| `Authorization` | `Bearer <module credential>` — issued by the host when the module is registered, shown once, rotatable. Identifies the module. |
| `X-HelixOS-Installation` | The installation id (`inst` from a token the module received). The host resolves the org **only** from this, and checks the installation belongs to the calling module and is enabled. |
| `X-HelixOS-User-Token` | The `user` token, on calls made on behalf of a person (marked **user** below). The host checks it is valid, for this module, and for the same installation. |

Errors: non-2xx with `{"error":"<human message>","code":"<code>"}`. Codes:
`unauthenticated` (401), `installation_disabled` (403), `scope_denied` (403),
`not_found` (404), `invalid_request` (400), `upstream_failed` (502),
`not_implemented` (501).

### 6.2 Scopes

| Scope | Grants |
|---|---|
| `identity.read` | `GET /people/{user_id}` |
| `agents.run` | Runs and bots (§6.4, §6.5) |
| `integrations.<key>` | `GET /integrations/<key>/credential` for that one integration. v1 keys: `bullhorn`. |
| `notify.slack` | `POST /notify/slack` **[HelixOS]** |
| `actions.read` | `GET /actions` **[HelixOS]** |
| `engagements.read` | `GET /engagements` **[HelixOS]** |
| `engagements.post` | `POST /engagements/post-approved` **[HelixOS]** |

A host MUST answer `scope_denied` for any call whose scope the installation was not
granted, and answers `501 not_implemented` for endpoints it does not implement (for
example a host without Slack). **Modules MUST treat `not_implemented` as "this host
lacks the feature"** and degrade gracefully, never as a fatal error.

### 6.3 Identity

`GET /whoami` (no scope) →
```json
{ "module": "candidate-search", "installation": "inst_…", "org": "org_findai",
  "org_name": "Find OS", "brand": "brand_…", "scopes": ["agents.run", "…"],
  "host": "helixos", "host_version": "…" }
```

`GET /people/{user_id}` (`identity.read`) →
```json
{ "user_id": "…", "email": "leah@example.com", "name": "Leah", "handle": "leah",
  "slack_user_id": "U0123" }
```
Absent fields are omitted. `handle` is the host's stable short name for the person.

### 6.4 Agent runs (`agents.run`)

The host owns bots and dispatch. A module asks the host to run an agent.

`POST /runs` (**user**) — start a run:
```json
{
  "bot_key": "job-123-leah",
  "title": "Acme — Senior Go Engineer",
  "system_prompt": "…standing instructions for this bot…",
  "prompt": "…what to do on this run…",
  "start_url": "https://www.linkedin.com/talent/search",
  "tool_groups": ["bullhorn", "memory", "engagement", "human_attention"],
  "files": [{ "name": "jd.pdf", "url": "https://<module>/files/…?sig=…" }],
  "meta": { "job_id": "123", "client_id": "9" }
}
```
- `bot_key` `^[a-z0-9][a-z0-9-]{0,62}$`, the module's own name for a bot. The host
  keeps one bot per `(installation, bot_key)` and namespaces it on its side
  (HelixOS: slug `m-<module>-<bot_key>`). Re-using a key updates that bot's title and
  system prompt.
- The run's owner is the user in `X-HelixOS-User-Token`.
- The bot's tools are exactly `module:<key>` ∪ (`tool_groups` ∩ manifest
  `agent_tool_groups`).
- `files[].url` MUST be fetchable by the host for at least 5 minutes without other
  credentials (signed URL). The host delivers them to the agent's workspace
  (HelixOS: `~/work/incoming/<name>`). Max 32 MB each.
- `meta` is opaque, ≤ 8 KB JSON, echoed back on run reads and `run.completed`.

→ `201`
```json
{ "run_id": "…", "task_id": "…", "status": "dispatched",
  "run_frame_url": "https://<iss>/runs/…/frame?t=…", "view_url": "…" }
```
`task_id` (optional) is the host's id for the underlying agent task, informational.

`GET /runs?bot_key_prefix=&owner=&limit=` → `{"runs":[Run]}`, newest first, limit ≤ 200 (default 50).
`GET /runs/{run_id}` → `Run` with a fresh `run_frame_url`.
`POST /runs/{run_id}/messages` (**user**) `{"text":"…","interrupt":false}` → `{"ok":true}` — sends a message to the live run.
`DELETE /runs/{run_id}` (**user**) → `{"ok":true}` — stops and archives the run.

```json
Run = { "run_id": "…", "bot_key": "…", "title": "…", "owner_user_id": "…",
        "status": "dispatched|planning|implementing|live|completed|failed|cancelled",
        "created_at": "RFC3339", "completed_at": "RFC3339|null",
        "meta": {…}, "run_frame_url": "…", "view_url": "…" }
```

`GET /bots?key_prefix=` → `{"bots":[{"bot_key","title","owner_user_id","last_run_at","live_run_id"}]}` — the module's bots in this installation, for rosters.

### 6.5 Run frames

`run_frame_url` is a host page that shows one live run. Modules embed it as a
**nested iframe** inside their own page:

```html
<iframe src="{run_frame_url}" allow="clipboard-read; clipboard-write; fullscreen; autoplay"></iframe>
```

- The URL carries a 10-minute `run` token and works without cookies.
- The module MUST NOT try to read inside the frame; it is cross-origin and the host
  keeps any upstream credentials there.
- When the token expires the frame shows an "expired" notice; the module re-fetches
  `GET /runs/{id}` and swaps the `src`. The SDK's `<RunFrame>` does this.

### 6.6 Integrations

`GET /integrations/{key}/credential` (`integrations.<key>`) → the org-level
credential, e.g. for `bullhorn`:
```json
{ "key": "bullhorn", "username": "…", "secret": { "client_id": "…", "client_secret": "…", "password": "…" } }
```
`404 not_found` when the org has none. Per-user logins are not exposed here.

### 6.7 [HelixOS] Notifications and shared queues

`POST /notify/slack` (`notify.slack`):
```json
{ "user_id": "…", "channel": "C123", "text": "Run finished",
  "link": { "path": "/job", "query": { "id": "123", "run": "…" } } }
```
Exactly one of `user_id` (DM) or `channel`. If `channel` is omitted and `user_id`
is too, the org's attention channel is used. `link` becomes a host deep link to the
module page (§7.4), appended to the text. → `{"ok":true}`

`GET /actions?ref=&status=&limit=` (`actions.read`), `GET /engagements?ref=&status=&limit=`
(`engagements.read`) → `{"actions":[…]}` / `{"engagements":[…]}`. `ref` is the
module's own reference that its agents passed as `job_id` when creating these rows.
`POST /engagements/post-approved` (`engagements.post`) `{"ref":"…"}` → `{"run_id":"…"}` —
starts (or reuses) a run that sends the approved drafts.

## 7. UI frame protocol

### 7.1 Framing

The host renders a module page in a sandboxed iframe:

```html
<iframe src="<module-origin><path>?<query>#helixos=1&host=<url-encoded host origin>"
        sandbox="allow-scripts allow-forms allow-same-origin allow-popups allow-downloads"
        allow="clipboard-read; clipboard-write; fullscreen; autoplay"></iframe>
```

- `allow-same-origin` is safe here because the module is cross-origin to the host.
- The `allow` list exists so the module can pass the same permissions to nested run
  frames; a frame never gets more than its parent.
- The host's CSP `frame-src` lists only installed module origins.
- **No cookies**. A module MUST NOT rely on its own cookies for auth inside the
  frame (third-party cookies are blocked); it keeps the `user` token in memory.

The module reads `host` from the URL fragment and MUST use it as the
`targetOrigin` of every `postMessage` and as the required `event.origin` (with
`event.source === window.parent`) of every message it accepts.

The host origin is **not** necessarily the issuer's origin: one host deployment may
serve many tenant domains (HelixOS: `os.helix.ml`, `os.we-find.ai`, …) under a single
`iss`. Trust therefore comes from the token, not the origin: every `user` token
carries an `origin` claim (the host page origin it was minted for), and the module
UI MUST discard a `token` message whose token's `origin` differs from the fragment's
`host`. Decoding the claim without verifying is enough in the browser, because the
module backend verifies the signature on every API call and a forged token gets
nothing; the module shows no data until it holds a token. A page loaded without
`#helixos=1` (not framed by a host) SHOULD show an "open this from your host" notice
instead of working. Modules MAY additionally send `Content-Security-Policy:
frame-ancestors` listing known host origins.

### 7.2 Messages

Every message is an object with `"protocol": "helixos-module/1"` and a `type`.
Anything else is ignored. Hosts check `event.origin` equals the module origin and
`event.source` is the module frame.

Module → host:

| `type` | Fields | Meaning |
|---|---|---|
| `ready` | — | The module has loaded. The host replies with `token`, `theme` and `route`. |
| `token.request` | — | Ask for a fresh `user` token. |
| `navigate` | `path`, `query` (object of strings) | The module changed page. The host updates its address bar (no reload), so deep links and back/forward work. |
| `title` | `text` | Page title for the host chrome. |
| `resize` | `height` (px) | Preferred content height. Hosts MAY ignore it and give the frame the full viewport. |
| `toast` | `kind` (`info`\|`success`\|`error`), `text` | Show a host toast. |
| `open` | `url` | Open an absolute `https:` URL in a new tab (the host may refuse other schemes). |

Host → module:

| `type` | Fields | Meaning |
|---|---|---|
| `token` | `token`, `expires_at` (RFC3339), `user` (`{id,email,name}`), `org`, `inst` | A fresh `user` token. Sent after `ready`, on every `token.request`, and proactively about 60 s before expiry. |
| `theme` | `mode` (`light`\|`dark`), `tokens` (object: CSS custom property name → value) | The host theme. Modules SHOULD apply the tokens to `:root`. |
| `route` | `path`, `query` | The host navigated (back/forward or a deep link); the module shows that page. |

### 7.3 Theme tokens

`tokens` uses these names (values are CSS colours or lengths). A module MAY ignore
any it does not use: `--background`, `--foreground`, `--card`, `--card-foreground`,
`--primary`, `--primary-foreground`, `--muted`, `--muted-foreground`, `--accent`,
`--accent-foreground`, `--border`, `--destructive`, `--radius`.

### 7.4 Deep links

A host link to a module page has the form `<host>/m/<key><path>?<query>`. **[HelixOS]**
serves this by redirecting to `/m?module=<key>&p=<path>?<query>`; other hosts may
use any URL scheme, since modules only ever see `path` and `query`.

## 8. Events (host → module)

`POST <module><events.path>` with a `host` token and body:
```json
{ "id": "evt_…", "type": "run.completed", "org": "org_findai", "inst": "inst_…",
  "occurred_at": "RFC3339", "data": { … } }
```
- Delivery is **at least once**: the module MUST dedupe on `id`, answer `2xx` within
  10 s, and do slow work after answering.
- Non-2xx or timeout: the host retries with exponential backoff (30 s doubling,
  capped at 1 h) for up to 24 h, then gives up.
- A host sends only types the manifest subscribes to.

| `type` | `data` |
|---|---|
| `run.completed` | `{run_id, bot_key, status, meta}`. `status` is a terminal status from §6.4. |
| `installation.changed` | `{enabled, scopes}` — also sent on first install. |
| `findai.lead_adopted` **[HelixOS]** | `{job_spec_id, client:{name, notes}, job:{title, brief}}`. **Synchronous**: the module answers `200` `{"client_id":"…","job_id":"…"}`, which the host stores on its lead. Not retried. |

## 9. Data migration into a module

A host moving existing data into a module calls the module's own import endpoint
with a `host` token whose `purpose` is `migrate`. Its path and body are defined by
the module (e.g. candidate search: `POST /helixos/import`). Modules MUST reject
`purpose=migrate` calls on every other route, and reject import calls without it.

## 10. Versioning

- Breaking changes create `contract-v2` with `schema_version: 2`, a new
  `protocol` string (`helixos-module/2`) and a new host API base (`/v2`). Hosts
  SHOULD serve v1 and v2 side by side for a transition period.
- Within v1, hosts and modules MUST ignore unknown JSON fields and unknown event and
  message types.

## 11. Conformance checklist

A module conforms if it:
- serves a valid manifest and a health endpoint;
- verifies `user` and `host` tokens per §4.3, trusting only configured issuers;
- scopes every read and write by `(iss, org)`, and refuses cross-org access;
- talks to the host only through §6 with its credential and an installation id;
- uses `postMessage` only with the host origin from §7.1;
- dedupes events on `id`.

A host conforms if it:
- validates manifests and caches tools per §3 and §5;
- issues tokens per §4 and publishes its JWKS;
- enforces installation, enablement and scopes on every host API call and tool call;
- frames modules per §7.1 and implements the messages in §7.2;
- delivers subscribed events per §8.
