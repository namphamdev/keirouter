# Wire Up Existing Features — Design Spec

**Date:** 2026-06-01
**Branch:** feat/keirouter-backend-mvp
**Scope:** Make four scaffolded-but-dead features actually work, matching 9router behavior.

---

## Overview

keirouter has solid core routing (dispatch, vault, OAuth, format codecs, capability-aware fallback) but several features exist only as UI + admin API + DB rows without being wired into the request path. This spec covers wiring up:

1. **Model aliases** — bare model name → provider/model resolution
2. **Proxy-pool routing** — per-account proxy configuration applied to outbound HTTP
3. **CLI-tool config writing** — write/patch config files for 11 coding CLIs
4. **Skills** — frontend-only fix (skills are static docs, not runtime features)

---

## 1. Model Aliases

### Data Model

```sql
CREATE TABLE model_aliases (
    alias  TEXT PRIMARY KEY,   -- e.g. "fast", "claude-3", "gpt4"
    target TEXT NOT NULL       -- always "provider/model", e.g. "anthropic/claude-sonnet-4-20250514"
);
```

### Resolution Priority (in `resolve.go`)

```
1. "provider/model"  → explicit single target (has "/")
2. "chain:name"      → named chain's steps
3. bare chain name   → chain's steps (existing)
4. bare alias name   → resolve alias → single target
5. error
```

Chains beat aliases for bare names. This matches 9router exactly.

### API Endpoints

| Method | Path | Body/Query | Response |
|--------|------|------------|----------|
| GET | `/api/models/alias` | — | `{ "aliases": { "fast": "anthropic/claude-sonnet-4-20250514" } }` |
| PUT | `/api/models/alias` | `{ "alias": "fast", "target": "anthropic/claude-sonnet-4-20250514" }` | `{ "ok": true }` |
| DELETE | `/api/models/alias?alias=fast` | — | `{ "ok": true }` |

All endpoints require loopback + session auth (same as existing admin API).

### Files

| File | Change |
|------|--------|
| `backend/internal/store/repo_aliases.go` | **New** — CRUD: List, Get, Set, Delete |
| `backend/internal/store/migrate.go` | Add `model_aliases` table migration |
| `backend/internal/gateway/resolve.go` | Add alias lookup between chain check and error |
| `backend/internal/gateway/admin.go` | Mount alias routes under `/api` |

### Implementation Notes

- `resolveTargets()` gets a new `aliasSource` parameter (or the Server struct carries it).
- Alias lookup is a single `SELECT target FROM model_aliases WHERE alias = ?` — no caching needed for v1.
- Target validation: reject targets that don't contain `/` (must be `provider/model`).

---

## 2. Proxy-Pool Routing

### Data Model

The `proxy_pools` table already exists for CRUD. Add `proxy_pool_id` to accounts:

```sql
ALTER TABLE accounts ADD COLUMN proxy_pool_id TEXT DEFAULT '';
```

Pool table (already exists, confirm schema matches):

```sql
CREATE TABLE proxy_pools (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'http',  -- "http" | "vercel" | "cloudflare" | "deno"
    proxy_url   TEXT NOT NULL,
    no_proxy    TEXT DEFAULT '',
    strict      BOOLEAN DEFAULT FALSE,
    is_active   BOOLEAN DEFAULT TRUE,
    test_status TEXT DEFAULT 'unknown',
    last_tested TIMESTAMP,
    last_error  TEXT,
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);
```

### Credential Extension

Add proxy fields to `core.Credentials`:

```go
type Credentials struct {
    APIKey string
    Token  string
    // ... existing fields ...

    // Proxy config (resolved from pool at dispatch time)
    ProxyURL    string // HTTP/HTTPS/SOCKS proxy URL
    NoProxy     string // comma-separated bypass hosts
    StrictProxy bool   // fail request when proxy unreachable
    RelayURL    string // vercel/cloudflare/deno relay URL (mutually exclusive with ProxyURL)
}
```

### Request Flow

```
dispatch.Plan()
  → vault.Open(account)
  → NEW: if account.ProxyPoolID != "", resolve pool → inject proxy into Credentials
  → pipeline.Execute(credentials, connector)
  → connector.DoRequest(credentials)
  → httpclient: use credentials.ProxyURL (HTTP mode) OR credentials.RelayURL (relay mode)
```

### Proxy Resolution Logic

New file `backend/internal/proxy/resolve.go`:

```go
// ResolvePool looks up a proxy pool by ID and returns proxy config.
func ResolvePool(ctx context.Context, repo *store.ProxyPoolRepo, poolID string) (proxyURL, relayURL, noProxy string, strict bool, err error)
```

Priority:
1. If `poolID` is empty or `"__none__"` → no proxy
2. Look up pool by ID, check `is_active`
3. If `type` is `"http"` → return `proxy_url` as `ProxyURL`
4. If `type` is `"vercel"`, `"cloudflare"`, or `"deno"` → return `proxy_url` as `RelayURL`
5. Return `no_proxy` and `strict` flags

### HTTP Client Changes

`backend/internal/connectors/httpclient.go` — build Transport per-request:

```go
func transportFor(creds core.Credentials) *http.Transport {
    base := &http.Transport{
        TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
        // ... existing settings ...
    }
    if creds.ProxyURL != "" {
        proxyURL, _ := url.Parse(creds.ProxyURL)
        base.Proxy = http.ProxyURL(proxyURL)
    }
    return base
}
```

For relay mode, the connector rewrites the request:
- Set `x-relay-target` header = original URL origin
- Set `x-relay-path` header = original URL path + query
- Replace request URL with `RelayURL`

### Proxy Test Endpoint

`POST /api/proxy-pools/{id}/test`:
- HTTP type: HEAD through proxy to `https://google.com` (8s timeout)
- Relay type: GET to relay URL with `x-relay-target: https://httpbin.org` (10s timeout)
- Update pool: `test_status`, `last_tested`, `last_error`; auto-disable on failure

### Files

| File | Change |
|------|--------|
| `backend/internal/store/models.go` | Add `ProxyPoolID` to Account struct |
| `backend/internal/store/migrate.go` | Migration: ALTER TABLE accounts ADD COLUMN proxy_pool_id |
| `backend/internal/core/credentials.go` | Add ProxyURL, RelayURL, NoProxy, StrictProxy fields |
| `backend/internal/dispatch/dispatch.go` | Resolve pool in `Plan()`, inject into Credentials |
| `backend/internal/connectors/httpclient.go` | Use proxy fields from Credentials |
| `backend/internal/proxy/resolve.go` | **New** — pool resolution logic |
| `backend/internal/gateway/admin.go` | Add test endpoint |
| `backend/internal/store/repo_pools.go` | **New** — proxy-pool CRUD: Get by ID, List, Create, Update, Delete |

### Worker Deployment (Out of Scope)

Deploying vercel/cloudflare/deno workers is not included in this spec. The relay *forwarding* protocol (header rewriting for `x-relay-target`/`x-relay-path`) IS implemented in the connector layer. Users deploy workers externally (via their own CI, Vercel CLI, `wrangler`, etc.) and register the resulting URL as a proxy pool entry with `type: "vercel"`, `"cloudflare"`, or `"deno"`. The relay worker itself is a trivial passthrough — 9router's worker code is ~30 lines of JS.

---

## 3. CLI-Tool Config Writing

### Interface

New package `backend/internal/clitools/`:

```go
type Tool interface {
    ID() string
    Name() string
    DetectStatus(homeDir string) (installed, configured bool, configPath string, err error)
    Configure(homeDir, baseURL, apiKey string, models []string) error
    Remove(homeDir string) error
}
```

### Tool Implementations

| # | Tool | Config Path | Format | Status Marker |
|---|------|-------------|--------|---------------|
| 1 | Claude Code | `~/.claude/settings.json` | JSON | `env.ANTHROPIC_BASE_URL` exists |
| 2 | Codex | `~/.codex/config.toml` + `auth.json` | TOML+JSON | `model_provider = "keirouter"` |
| 3 | Cline | `~/.cline/data/globalState.json` + `secrets.json` | JSON | `openAiBaseUrl` contains localhost |
| 4 | Copilot | `~/Library/Application Support/Code/User/chatLanguageModels.json` | JSON array | entry with `name === "KeiRouter"` |
| 5 | Droid | `~/.factory/settings.json` | JSON | `customModels[].id` starts with `"custom:KeiRouter"` |
| 6 | OpenClaw | `~/.openclaw/openclaw.json` | JSON | `models.providers["keirouter"]` exists |
| 7 | OpenCode | `~/.config/opencode/opencode.json` | JSON | `config.provider["keirouter"]` exists |
| 8 | Kilo | `~/.local/share/kilo/auth.json` | JSON | `auth["openai-compatible"]` with localhost |
| 9 | Hermes | `~/.hermes/config.yaml` + `.env` | YAML+dotenv | `model.provider === "custom"` with localhost |
| 10 | DeepSeek TUI | `~/.deepseek/config.toml` | TOML | `provider === "openai"` with localhost |
| 11 | jcode | `~/.jcode/config.toml` + env file | TOML+dotenv | `providers["keirouter"]` exists |

### Keys Set Per Tool

**Claude Code** — patches `env` in settings.json:
- `ANTHROPIC_BASE_URL` (with `/v1` suffix)
- `ANTHROPIC_AUTH_TOKEN` (API key)
- `ANTHROPIC_DEFAULT_OPUS_MODEL`, `ANTHROPIC_DEFAULT_SONNET_MODEL`, `ANTHROPIC_DEFAULT_HAIKU_MODEL`

**Codex** — config.toml:
- `model = <first model>`, `model_provider = "keirouter"`
- `[model_providers.keirouter]` with `name`, `base_url`, `wire_api = "responses"`
- auth.json: `OPENAI_API_KEY`, `auth_mode = "apikey"`

**Cline** — globalState.json:
- `actModeApiProvider = "openai"`, `planModeApiProvider = "openai"`
- `openAiBaseUrl` (strips `/v1` suffix)
- `openAiModelId`, `planModeOpenAiModelId`
- secrets.json: `openAiApiKey`

**Copilot** — chatLanguageModels.json array:
- Entry with `name: "KeiRouter"`, `vendor: "azure"`, `apiKey`, `models[]` array

**Droid** — settings.json:
- `customModels[]` array with `model`, `id: "custom:KeiRouter-{i}"`, `baseUrl`, `apiKey`, `provider: "openai"`

**OpenClaw** — openclaw.json:
- `models.providers["keirouter"]` with `baseUrl`, `apiKey`, `api: "openai-completions"`
- `agents.defaults.model.primary` set to `keirouter/<model>`

**OpenCode** — opencode.json:
- `provider["keirouter"]` with `npm: "@ai-sdk/openai-compatible"`, `options: { baseURL, apiKey }`
- `models` map with model entries

**Kilo** — auth.json:
- `["openai-compatible"]: { type: "api-key", apiKey, baseUrl, model }`

**Hermes** — config.yaml + .env:
- `model:` block with `default`, `provider: "custom"`, `base_url`
- .env: `OPENAI_API_KEY`

**DeepSeek TUI** — config.toml:
- `provider = "openai"`, `[providers.openai]` with `base_url`, `api_key`, `model`

**jcode** — config.toml + env file:
- `providers["keirouter"]` with `type: "openai-compatible"`, `base_url`, `auth: "bearer"`
- Separate env file: `JCODE_KEIROUTER_API_KEY`

### Common Patterns

All tools follow the same strategy:
1. **Configure**: read existing config (or `{}`), merge in keirouter keys, write back. Never overwrite unrelated keys.
2. **Remove**: read config, delete only keirouter-specific keys, write back. If file becomes empty object, delete file.
3. **DetectStatus**: read config, check for keirouter markers.

No backup files are created. The merge-not-overwrite strategy preserves existing settings.

### API Endpoints

| Method | Path | Body | Response |
|--------|------|------|----------|
| GET | `/api/cli-tools` | — | `{ "tools": [{ "id": "claude", "name": "Claude Code", "installed": true, "configured": false, "configPath": "~/.claude/settings.json" }, ...] }` |
| POST | `/api/cli-tools/{toolId}/configure` | `{ "baseUrl": "http://localhost:20180", "apiKey": "kr_...", "models": ["anthropic/claude-sonnet-4-20250514"] }` | `{ "ok": true }` |
| POST | `/api/cli-tools/{toolId}/remove` | — | `{ "ok": true }` |

### Platform-Specific Paths

Use `os.UserHomeDir()` for `~` resolution. For Copilot on macOS vs Linux vs Windows, detect platform:
- macOS: `~/Library/Application Support/Code/User/chatLanguageModels.json`
- Linux: `~/.config/Code/User/chatLanguageModels.json`
- Windows: `%APPDATA%\Code\User\chatLanguageModels.json`

### Files

| File | Change |
|------|--------|
| `backend/internal/clitools/registry.go` | **New** — tool registry, Lookup by ID |
| `backend/internal/clitools/claude.go` | **New** — Claude Code implementation |
| `backend/internal/clitools/codex.go` | **New** — Codex implementation |
| `backend/internal/clitools/cline.go` | **New** — Cline implementation |
| `backend/internal/clitools/copilot.go` | **New** — Copilot implementation |
| `backend/internal/clitools/droid.go` | **New** — Droid implementation |
| `backend/internal/clitools/openclaw.go` | **New** — OpenClaw implementation |
| `backend/internal/clitools/opencode.go` | **New** — OpenCode implementation |
| `backend/internal/clitools/kilo.go` | **New** — Kilo implementation |
| `backend/internal/clitools/hermes.go` | **New** — Hermes implementation |
| `backend/internal/clitools/deepseek.go` | **New** — DeepSeek TUI implementation |
| `backend/internal/clitools/jcode.go` | **New** — jcode implementation |
| `backend/internal/gateway/clitools.go` | **Rewrite** — replace instruction handler with real config writing |

---

## 4. Skills (Frontend-Only)

### Context

9router's "skills" are **not runtime request modifiers**. They are static markdown documentation files hosted on GitHub that teach AI agents how to call specific 9router API endpoints. The dashboard lists them with copyable URLs. Zero injection into requests.

keirouter's current Skills page (`frontend/src/pages/Skills.tsx`) has CRUD + toggle + prompt injection — keirouter's own invention that goes beyond 9router. The current page treats skills as "reusable system-prompt augmentations applied to matching requests" with an `enabled` toggle and a `prompt` field. This is more than 9router does (9router has zero runtime injection), but the UX doesn't match 9router's reference-doc model. The rewrite should keep the CRUD infrastructure (it's useful) but add the reference-doc display with copyable URLs alongside the existing toggle behavior.

### Changes

- `frontend/src/pages/Skills.tsx` → add a reference-doc section alongside existing CRUD:
  - Show skill name, description, endpoint badge
  - "Copy link" button for each skill URL (points to hosted markdown, e.g. GitHub raw URL)
  - Keep existing toggle/create/delete behavior (keirouter's extension beyond 9router)
  - Update page description from "system-prompt augmentations" to include reference-doc language
- Optionally: host skill markdown files in repo under `skills/<id>/SKILL.md`

### Files

| File | Change |
|------|--------|
| `frontend/src/pages/Skills.tsx` | **Modify** — add reference-doc display with copyable URLs alongside existing CRUD |

---

## Implementation Order

1. **Model aliases** — smallest, touches resolve.go + one new repo file
2. **Skills** — frontend-only, no backend changes
3. **CLI-tool config writing** — new package, isolated from request path
4. **Proxy-pool routing** — largest, touches dispatch + credentials + httpclient

Each feature is independently testable and deployable.

---

## Out of Scope

- MITM proxy (CA generation, cert install, DNS, per-host handlers)
- Tunneling (Cloudflare, Tailscale)
- Proxy-pool worker deployment (vercel/cloudflare/deno)
- Cloud sync
- Self-updater
- Additional OAuth provider flows (cursor, kiro import, iflow, qoder)
- i18n/localization
- MCP bridge

---

## Testing Strategy

- **Model aliases**: unit test resolve.go with alias/chain/explicit priority; integration test CRUD via admin API
- **Proxy-pools**: unit test pool resolution; integration test with local HTTP proxy (e.g., mitmproxy or simple Go proxy)
- **CLI-tools**: unit test each tool's Configure/Remove with temp dirs; verify config file content matches expected keys
- **Skills**: manual UI verification (frontend-only change)
