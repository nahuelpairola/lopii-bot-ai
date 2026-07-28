# CLAUDE.md — lopii-finance-bot

Personal finance Telegram bot for Argentine users (ARS/USD). Natural-language input via Telegram, LLM-based intent classification, PostgreSQL persistence, Argentine financial context (inflation, multiple exchange rates).

**v1:** Google Sheets + Apps Script in `app_scripts_v1/` — historical reference only, git-ignored (kept on disk, not tracked).  
**v2:** This repo — Go rewrite.

> **For current project state** (package map, data model, features, design decisions):
> see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — the index of reference docs. Read the one your task touches.

## 1. Overview & Architecture

### Stack

- Go, Gin, GORM, Postgres (Neon)
- Goose for migrations (run automatically at server startup)
- go-telegram/bot in webhook mode
- Viper for config (TOML per environment; env vars override)
- shopspring/decimal for money
- LLM orchestrator via Groq (tool calling, plain `net/http`, no SDK)
- Deployment: Render + Docker

### Package map (`internal/`)

| Package | Role |
|---|---|
| `config` | Reads `config/{ENV}.toml` via Viper |
| `constants` | Domain constants: currencies, movement types |
| `currency` | `Currency` type (string alias), `ARS`/`USD` constants, `SupportedCurrencies` |
| `database` | GORM+Postgres singleton. `Initialize`, `RunMigrations` (goose) |
| `health` | `HealthChecker` that pings the DB |
| `user` | Model + repository: `FindByTelegramID`, `FindByID`, `Insert` |
| `invitation` | Model + repository: `Create` (generates random code), `FindByCode`, `MarkAsUsed` |
| `account` | Model + full repository + messages for flows |
| `subcategory` | Model + repository + messages for flows |
| `movement` | Model + repository (incl. `SumForUser`/`ListForUser` read-only QUERY aggregates) + **el guard** (`guard.go`: `Normalize`, `AssignTransactionIDs`, `CheckBalances`, los sentinels `Err*`) — los invariantes de plata viven al lado del tipo que protegen, y son puros: no tocan la DB |
| `metric` | Model + repository: `Log`, `Resolve` — métricas de asertividad LLM (tabla `intent_events`) |
| `middleware` | `RequireAdmin(adminID)` |
| `conversation` | Engine: `Engine`, `Flow`, `TextStep`, `ChoiceStep`, `repository` |
| `pendingjob` | Model + repository: durable queue (`pending_llm_jobs`) for a message cached after a terminal Groq 429 — `Insert`, `ListByUserOrdered`, `ListPendingUserIDs`, `Delete`, `CountByUser` |
| `orchestrator` | Groq tool-calling HTTP client (plain `net/http`, no SDK). One chokepoint `Client.send` with retry/backoff + `RateLimitedError`. `ClassifyIntent` (router), `ClassifyCreate`, `ResolveUpdate`, `ResolveDelete`, `ClassifyOnboarding`, `ClassifyCategoryCreate`, `ResolveAccountManage`, `AnswerQuery` (read-only agent loop) |
| `queryhistory` | Model + repository: ephemeral QUERY conversation thread (`Append`, `Recent`), hard-pruned by TTL, no `deleted_at` |
| `reminder` | Model + repository: one row per user, minutes-since-ART-midnight window + weekly-summary flags. `Upsert`, `Disable`, `ListDue`, `ListWeeklyDue`, `SetWeeklySummary` |
| `notifier` | `Sweeper` — in-process `time.Ticker` goroutine driving every scheduled system→user notification (daily reminder, weekly summary, trace retention) |
| `nudge` | Model + repository: once-ever/cooldown storage for contextual tips (`WasSent`, `MarkSent`, `LastSentAt`) |
| `summary` | Weekly-summary builder + its Spanish copy. Consumer-local `MovementReader`/`AccountReader` interfaces |
| `trace` | `NewID` (crypto/rand, 32 hex) + ctx carrier for the correlation id shared by `request_traces`/`llm_calls`/`intent_events` |
| `logging` | Installs the process-wide `slog` logger; its handler stamps the ctx `trace_id` onto every record |
| `controller/health` | HTTP: `/health/internal`, `/health/external` |
| `controller/invitation` | HTTP: `POST /invitations` |
| `controller/admin` | HTTP: `POST /admin/users/:telegramID/reset` (admin-only) |
| `controller/messaging` | Telegram: `/start`, catch-all for free text and callbacks |
| `controller/miniapp` | Telegram Mini App: templ-rendered HTML views (overview, accounts, categories, period, evolution) + `auth.go` initData validation |
| `server` | Bootstrap: DB, migrations, bot, webhook, controllers, Gin |

## 2. Conventions

### Language
Telegram UI strings in Argentine Spanish, defined in `messages.go` per package. All code identifiers in English.

### Repository pattern
Each controller defines its own local interfaces for the repositories it uses. Never import concrete types cross-package. Example in `controller/messaging`:

```go
type accountRepository interface {
    Insert(*account.Account) error
}
```

### Money
Always `shopspring/decimal`. Never `float64` for monetary amounts.

### Sentinel errors + unique violations
Sentinel errors per package (e.g., `account.ErrAccountAlreadyExists`). To detect Postgres unique constraint violations:

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    return ErrAccountAlreadyExists
}
```

### Tests
Unit tests with mocked repositories (no real Postgres). When adding a test, mock the package's local interface — not the concrete type.

### Conversation state
DB-backed JSONB in the `conversation_states` table. Never access this table directly — all interaction goes through `conversation.Engine`.

### Admin IDs
Loaded into memory at server startup. Zero extra queries per Telegram request.

### No duplicated literals
A string or number used more than once is a named constant, scoped to its reach:

| Reach | Home |
|---|---|
| Used 2+× in one package | unexported `const` in that package |
| Used across packages | `internal/constants` (raw string); typed wrappers may re-export (`currency.ARS = constants.ARS`) |
| Owned by one package, read by another | **exported** `const` in the producer (e.g. `conversation.ResumeCancelledKey`) |

One const per distinct value; one const per distinct *meaning* even when strings
collide (a button value and a Data key that share `"edit_proposal"` are two consts).
Single-use literals stay inline. In `conversation.Data`, all map keys are consts
(`internal/controller/messaging/data_keys.go`).

## 3. Recipes

How to add a migration, conversation flow, LLM intent, scheduled notification, or admin command → **[docs/recipes.md](docs/recipes.md)**.

## 4. Business Rules

Full rules (currencies, grouping, taxonomy, accounts, balances, reminders) → **[docs/business-rules.md](docs/business-rules.md)**. The one that's load-bearing is inline below.

### The accounting model (money precision — READ THIS before touching any money path)

**Every movement is signed and attributed to a real account. No exceptions.** A balance is
*always* `SUM(amount)` over an account's movements, so the stored sign IS the accounting. A
wrong sign or `account_id` silently corrupts a balance — the one place in this codebase a
small mistake is a financial bug, not cosmetic.

| Type | `account_id` | Stored sign | Meaning |
|---|---|---|---|
| `expense` | source account (required) | **negative** (`-amount.Abs()`) | money leaves an account |
| `income` | destination account (required) | **positive** (`+amount.Abs()`) | money enters an account |
| `transfer` | both legs (required) | negative out / positive in | money moves between two own accounts |

- **The app owns the sign, never the LLM.** The guard normalizes `expense`→negative, `income`→positive on write.
- **The sign never escapes storage.** User and LLM both see `amount.Abs()`; direction comes from the movement type, never a `-`.
- **Account resolution is app-side, deterministic:** LLM-matched account → currency default (`FindDefaultByCurrency`) → gap-fill asks.
- **Never `float64`** — always `shopspring/decimal`.

> **Full money model** — the guard's insert-time invariants, the insufficient-funds confirm
> gate, grouped transactions, FCI redemption, enforcement status → **[docs/business-rules.md](docs/business-rules.md#the-accounting-model)**.

## 5. Local Dev Setup

Prerequisites, Postgres, config, run, migrations → **[docs/dev-setup.md](docs/dev-setup.md)**.

## 6. Technical Debt

- `accounts` table has a `type DEFAULT 'standard'` column from a prior design — drop with a migration.
- Migration `20260618230837_create_admin_user.sql` has literal `telegram_id = 'TELEGRAM_ID'` — must be edited manually before each new-environment deploy.
- `middleware.RequireAdmin` is hardcoded to user ID 1 — needs real auth.
- `intent_events.needs_confirmation` (NOT NULL) quedó vestigial tras el rediseño UNCLEAR del router: se escribe siempre `false`. Dropear con una migración si se quiere limpiar.
- Los dos movimientos de **apertura** de cuenta no pasan por `movement.Normalize` y no
  pueden: son una pata suelta tipada `Transfer` sin contraparte ni `transaction_id`, y el
  guard rechaza toda transferencia que no sea un grupo de 2 patas — rechazaría *toda*
  apertura, con cualquier monto. En vez de eso reciben el `*account.Account` entero, así el
  desajuste de moneda es irrepresentable. No es deuda, es una decisión; está documentada en
  el anti-pattern de [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#anti-patterns--what-not-to-do).

## 7. Claude Code Session Rules

- **Subagents run on `sonnet`.** Any `Agent` tool call spawned in this project (any `subagent_type`) must pass `model: "sonnet"` explicitly, unless the user asks otherwise for a specific task. Exception: `subagent_type: "fork"` always inherits the parent session's model — a `model` override is ignored for forks, so this rule doesn't apply to them.
- **Codegraph overrides skill-default exploration.** This repo has `.codegraph/` indexed. Any superpowers skill step that says "explore the codebase", "read relevant files", or spawns an `Explore`/`general-purpose` subagent for code lookup must use `codegraph_explore` first instead — one call returns verbatim source + call graph, versus dozens of raw `Read`/`Grep` round-trips a generic skill step defaults to. This applies mid-skill (brainstorming, writing-plans, systematic-debugging, etc.), not just standalone questions — skills don't know codegraph exists, so the substitution has to be made manually every time.
- **Stack mandate, always.** Codegraph before manual grep/read. The matching superpowers skill (brainstorming / systematic-debugging / writing-plans / TDD) before any feature or fix. Ponytail discipline (minimum code, no premature abstraction) on every diff. Caveman-compressed communication for agent/subagent output. None of these are skippable for a "simple" task — that rationalization is exactly what each skill/mode already warns against.
- **WIP=1 for delegated work.** One flow/fix active per subagent at a time. Don't activate a second task before the first has completion evidence (below). Parallel activation dilutes the reasoning budget available to each task — nothing finishes properly if it's split too thin.
- **Completion evidence, not self-assessment.** A subagent doesn't report a task "done" without having run `go build ./...` (or `go test ./...` if it touched tests) and shown the result. Green output is the only valid signal — not the agent's own read of its diff.

Use `repo-investigator`, `repo-builder`, and `repo-reviewer` (`.claude/agents/`) for locate/edit/review work scoped to this repo, instead of generic `Explore`/`general-purpose`/`code-reviewer` agents — they carry this repo's rules (money type, currency handling, `conversation_states` access, taxonomy) built into their prompts.
