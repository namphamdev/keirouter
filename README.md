<p align="center">
  <img src="keirouter-logo.png" alt="KeiRouter Logo" width="160">
</p>

<h1 align="center">KeiRouter</h1>

<p align="center">
  A fast, self-hostable AI gateway.
</p>

<p align="center">
  <a href="https://github.com/mydisha/keirouter/actions/workflows/ci.yml"><img src="https://github.com/mydisha/keirouter/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white" alt="Go 1.24+"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>

---

Point your coding tools (Claude Code, Cursor, Codex, Cline, OpenClaw, and any OpenAI/Anthropic-compatible client) at one local endpoint. KeiRouter routes requests across many providers with automatic fallback, token-saving compression, encrypted credential storage, and spend controls.

Written in Go for a small footprint (single static binary, ~20–30MB RAM idle, instant startup) with a React + Tailwind dashboard.

> **Status:** Active development. See the [Architecture](#architecture) section for what's implemented.

## Why KeiRouter

- **One endpoint, many providers.** Speak OpenAI or Anthropic; KeiRouter
  translates to whatever the target provider expects.
- **Never stop coding.** Routing chains fall back across accounts and providers
  on rate limits, quota exhaustion, or errors — without silently downgrading to
  a model that lacks a capability your request needs.
- **Spend less.** The Slimmer compresses bulky tool outputs (diffs, greps, file
  listings, build logs) before they reach the model. Terse mode trims output
  tokens. Budgets enforce hard USD caps per key, project, or org.
- **Secure by default.** Provider secrets are encrypted at rest with envelope
  encryption (AES-256-GCM). API keys are stored only as argon2id hashes and
  shown in plaintext exactly once. The dashboard is protected by a password
  (seeded on first run, changed via onboarding) and HMAC session cookies.
- **Observable.** Prometheus metrics at `/metrics` cover request volume,
  latency, tokens, cost, fallbacks, and cache hits.
- **Caches what repeats.** An optional semantic response cache returns stored
  answers for repeated prompts at zero cost and instant latency.

## Quick start

Install with curl (requires Go 1.22+, Node.js 20+):

```bash
curl -fsSL https://raw.githubusercontent.com/mydisha/keirouter/main/scripts/install.sh | bash
```

Or build manually:

```bash
git clone https://github.com/mydisha/keirouter.git
cd keirouter
make build
./keirouter
```

Or with Docker:

```bash
docker compose -f deploy/compose.yaml up -d --build
```

On first run, sign in at **http://localhost:20180** with the default password
`keirouter` — the onboarding flow prompts you to set a new one.

Create an API key without the dashboard:

```bash
keirouter -bootstrap   # prints a kr_ key once
```

For development with hot reload:

```bash
make dev   # backend on :20180, dashboard on :5180
```

Then add a provider account and a routing chain via the admin API (loopback
only by default):

```bash
# Add an OpenAI account.
curl -s localhost:20180/api/accounts -d '{
  "provider": "openai", "label": "personal", "api_key": "sk-..."
}'

# Create a fallback chain: try GPT-4o, then DeepSeek.
curl -s localhost:20180/api/chains -d '{
  "name": "coding",
  "steps": [
    {"provider": "openai", "model": "gpt-4o"},
    {"provider": "deepseek", "model": "deepseek-chat"}
  ]
}'
```

Point your tool at KeiRouter:

```
Base URL: http://localhost:20180/v1
API Key:  <your kr_ key from bootstrap>
Model:    openai/gpt-4o     # direct provider/model
          chain:coding      # or a named routing chain
```

## Routing model strings

The `model` field accepts:

- `provider/model` — a single explicit target, e.g. `openai/gpt-4o`.
- `chain:name` — a named routing chain with ordered fallback steps.
- `name` — shorthand for a chain named `name`. Chains are also advertised by
  `GET /v1/models` with `owned_by: "combo"`, so client tools can pick them.

## Capabilities and endpoints

Beyond chat, KeiRouter speaks the full OpenAI-style surface. Each endpoint
routes by the same `provider/model` or chain string and falls back across
accounts:

| Capability | Endpoint | Example model |
|---|---|---|
| Chat | `POST /v1/chat/completions`, `POST /v1/messages` | `openai/gpt-4o` |
| Chat (Gemini-native) | `POST /v1beta/models/{model}:generateContent` | path-encoded model |
| Embeddings | `POST /v1/embeddings` | `openai/text-embedding-3-small` |
| Image generation | `POST /v1/images/generations` | `openai/gpt-image-1` |
| Speech-to-text | `POST /v1/audio/transcriptions` | `groq/whisper-large-v3` |
| Text-to-speech | `POST /v1/audio/speech` | `openai/tts-1` |
| Web search | `POST /v1/search` | `tavily/tavily-search` |
| Web fetch | `POST /v1/web/fetch` | `firecrawl/firecrawl-scrape` |

Discover what is available per capability:

- `GET /v1/models` — chains (as combos) plus every catalogued LLM model.
- `GET /v1/models/{kind}` — models for a service kind (`llm`, `embedding`,
  `image`, `stt`, `tts`, `search`, `fetch`).
- `GET /v1/models/info?id=provider/model` — metadata for a single model.

## Token saving

Two complementary layers reduce cost, configurable from the dashboard's Token
Saving page (or `POST /api/settings/endpoint`):

- **RTK input compression** (on by default) compresses bulky tool-result
  payloads — diffs, greps, directory listings, build logs — before they reach
  the model. It is safe by design: a filter that errors or would grow the
  content is silently skipped.
- **Caveman output compression** injects a terse "caveman speak" system
  directive that keeps all technical substance while dropping filler, cutting
  output tokens. Levels: `lite`, `full`, `ultra`. A separate `terse` mode offers
  KeiRouter's own concise-output directive as an alternative.

## OAuth provider connections

Subscription/OAuth providers (Claude, Codex, Gemini CLI, GitHub Copilot, Qwen,
xAI, ...) connect without an API key from the dashboard's Connections page. Two
flows are supported: authorization-code with PKCE (browser sign-in) and device
code (enter a code on the provider's verification page). Access tokens are
sealed with envelope encryption and refreshed automatically before they expire.

## Architecture

```
backend/
  cmd/keirouter/        entrypoint
  internal/
    core/               canonical domain model (provider-agnostic)
    config/             koanf config (env + YAML)
    crypto/             envelope encryption + API key & password hashing
    store/              SQLite/Postgres repos + embedded migrations
    transform/          OpenAI / Anthropic / Gemini codecs (unary + streaming)
    connectors/         provider drivers (chat/media/web) + catalog + models
    slimmer/            RTK tool-output compression (input token saver)
    terse/              terse-mode prompt injection (output token saver)
    caveman/            caveman output compression (output token saver)
    oauth/              OAuth flows (PKCE + device code) + token refresh
    capability/         model capability matrix (anti-downgrade guard)
    dispatch/           account selection + fallback + cooldown + token refresh
    budget/             hard spend enforcement
    meter/              usage + cost recording
    cache/              semantic response cache + embedder
    observ/             Prometheus metrics
    auth/               dashboard password + session tokens
    identity/           API key issuance + authentication
    vault/              encrypted-credential <-> live-credential bridge
    pipeline/           request lifecycle orchestration
    gateway/            HTTP edge: auth, routing, admin API, /metrics
    app/                dependency wiring
frontend/               React + Vite + Tailwind dashboard
deploy/                 Dockerfile + compose
```

A request flows: gateway (auth, parse dialect) → pipeline (slimmer, terse,
budget guard) → dispatch (pick account, capability check) → connector (HTTP to
provider) → transform (translate response) → gateway (render in client dialect)
→ meter (record usage).

## Configuration

Copy `config.example.yaml` and pass it with `-config`, or use environment
variables prefixed `KEIROUTER_` with `__` for nesting (e.g.
`KEIROUTER_SERVER__PORT=8080`). SQLite is the zero-config default; set
`database.driver: postgres` with a DSN for team/VPS deployments.

## Security notes

- The admin API (`/api/*`) is restricted to loopback by default. When exposing
  KeiRouter beyond localhost, place it behind a reverse proxy with access
  control or a trusted network policy, and set a stable `master_key`.
- The master key is the root of trust for all stored credentials. Back it up;
  losing it makes encrypted credentials unrecoverable.

## Development

```bash
cd backend
go test ./...        # run the test suite
go vet ./...         # static checks
```

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for setup instructions, coding guidelines, and how to submit pull requests.

## Security

To report a vulnerability, see [SECURITY.md](SECURITY.md). Please do not open a public issue for security concerns.

## License

[MIT](LICENSE)