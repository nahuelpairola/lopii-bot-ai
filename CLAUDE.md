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
| `invitation` | Model + repository: `Create` (generates random code), `FindByCode`, `MarkAsUsed`, `List` (newest 50, for the admin view) |
| `account` | Model + full repository + messages for flows |
| `subcategory` | Model + repository + messages for flows |
| `movement` | Model + repository (incl. `SumForUser`/`ListForUser` read-only QUERY aggregates, and `ListForAccount`, the one listing that deliberately bypasses `MovementQuery.apply`) + **the guard** (`guard.go`: `Normalize`, `AssignTransactionIDs`, `CheckBalances`, the `Err*` sentinels) — the money invariants live next to the type they protect, and are pure: they never touch the DB |
| `metric` | Model + repository: `Log`, `Resolve` — LLM accuracy metrics (`intent_events` table) |
| `middleware` | `RequireAdmin(adminID)` |
| `conversation` | Engine: `Engine`, `Flow`, `TextStep`, `ChoiceStep`, `repository` |
| `pendingjob` | The 429-queue cluster, storage **and** behaviour: model + `Repository` (`pending_llm_jobs`), the enqueue side (`EnqueueFreeText`, `EnqueueUpdatePick`, `EnqueueBehindPending`, `HandleGroqError`, the `WithReplaying`/`IsReplaying` ctx flag) and the drain worker (`Run`, gated by an in-process `nextDrainAt`). Its `Services` interface is implemented by `*controller` via `pendingjob_services.go` |
| `orchestrator` | Groq tool-calling HTTP client (plain `net/http`, no SDK). One chokepoint `Client.send` with retry/backoff + `RateLimitedError`. **`Run` is the unified agent loop and the only path a free-text message takes** — it walks a model fallback chain on 429 (never on 400). Around it: `ClassifyCategories` (taxonomy, own model + own TPM bucket), `AnswerQuery` (read-only loop, reached via the `answer_query` tool), and the wizard-side single-shot calls `ClassifyOnboarding` / `ClassifyCategoryCreate` / `ResolveAccountManage` / `ResolveUpdate`. The router (`ClassifyIntent`), `ClassifyCreate` and `ResolveDelete` were deleted in stage 5. Repo-free by design: the controller-side delegates of its loops live in the `agent`/`query` clusters |
| `chathistory` | Model + repository: ephemeral conversation thread shared by every intent (`Append`, `Recent`), hard-pruned by TTL, no `deleted_at`. Renamed from `queryhistory` — it was never QUERY-only |
| `pendingaction` | Model + repository: durable queue (`pending_actions`) of agent-loop actions waiting on an answer from the user — `Insert`, `NextForUser`, `Delete`, `CountForUser`. Drained one at a time (WIP=1) |
| `reminder` | Model + repository: one row per user, minutes-since-ART-midnight window + weekly-summary flags. `Upsert`, `Disable`, `ListDue`, `ListWeeklyDue`, `SetWeeklySummary` |
| `notifier` | `Sweeper` — in-process `time.Ticker` goroutine driving every scheduled job (daily reminder, weekly summary, trace retention, USD quotes, CPI) |
| `quote` | Model + repository + `net/http` client for two public series: daily USD rates (`usd_quotes`, one row per date+rate_type, `bid`/`ask`) and monthly CPI (`monthly_cpi`). Write-only so far — ingestion for a later Mini App consumption stage |
| `summary` | Weekly-summary builder + its Spanish copy. Consumer-local `MovementReader`/`AccountReader`/`IconReader` interfaces |
| `trace` | `NewID` (crypto/rand, 32 hex) + ctx carrier for the correlation id shared by `request_traces`/`llm_calls`/`intent_events` |
| `logging` | Installs the process-wide `slog` logger; its handler stamps the ctx `trace_id` onto every record |
| `agent` | The unified-agent-loop cluster, consumer side (delegate of `orchestrator.Run`): `StartLoop`, `DrainNextAction`, `FinishAskUser` / `FinishMovementUpdatePick` / `ProceedToUpdateConfirm` (update-flow continuations), `BuildRecentEntities`, `SettingsArea` + consts, `StartOfTodayArgentina`. Its `agentServices` interface is implemented structurally by `*controller` via the one-line bridges in `agent_services.go` |
| `query` | QUERY feature cluster (free-text reads, extracted from messaging): exported `Run`, `SystemPrompt`, `NewExecutor`, `Tools`; unexported `services` interface (10 `Query-*` methods + `AnswerQuery`) implemented by `*controller` via `query_services.go`. Read-only — never writes. Depends on `agent.StartOfTodayArgentina` |
| `flow` | Every conversation flow, built and finished: the 15 `New*Flow` builders registered in `server/flows.go`, their steps/options/copy, the movement write pipeline (`ResolveAndInsertMovements`, the opening/counterparty logic), the finishes, and the near-duplicate gate. Talks to the DB through the narrow `runner` interface (`flow/runner.go`), implemented by `*controller` — that is why `runner`'s methods are exported |
| `nudges` | Contextual-tip dispatcher, storage **and** behaviour: model + repository (`user_nudges`, once-ever/cooldown bookkeeping) and the dispatcher (`Maybe`, `HandleCallback`) that decides *which* tip fires and when |
| `messages` | The few copy strings shared by more than one cluster. Everything else lives with its own cluster |
| `settings` | The configuration wizards — everything reachable from the agent's `manage_settings` tool. `Dispatch` picks the area, then `StartAccountManage` / `StartAccountCreate` / `StartSubcategorySetup` / `StartCategoryManage` / `StartReminderSetup` read the repos, ask the LLM and seed a `flow`. Also `SuggestMergeTarget`, which `flow` reaches through the runner because `flow` does not import the orchestrator. This is the **only** caller of `ResolveAccountManage`, `ClassifyOnboarding` and `ClassifyCategoryCreate` |
| `controller/health` | HTTP: `/health/internal`, `/health/external` |
| `controller/admin` | HTTP: `POST /admin/users/:telegramID/reset` (admin-only) |
| `controller/messaging` | Telegram: `/start`, catch-all for free text and callbacks. Free text is delegated to the `agent` and `query` clusters via one-line bridges + delegates; `userLocks` (in-memory per-user write mutex), the build-tagged eval suites and the wizard flows stay here |
| `controller/miniapp` | Telegram Mini App: templ-rendered HTML views (overview, accounts, categories, period, evolution, admin) + the two **movement leaves** that end each drill (`AccountLeaf`, `SubcategoryLeaf`) + `auth.go` initData validation and the `requireAdmin` gate |
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
(`internal/conversation/data.go`).

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
- **The app owns the arithmetic too.** A correction arrives as a structured diff
  (`field` / `op` / `value`) and the app computes the result — the model never sends a
  number it worked out itself. "They refunded me half" is `op: multiply, value: 0.5`, not
  a recomputed amount. Before stage 5 the model echoed back the whole row, which cost a
  second call and could corrupt fields nobody asked to touch. `guardRefundDirection`
  checks the *result* against the stored row, not the `op`: a refund that ends up growing
  the expense is rejected regardless of how it was expressed.
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
- `middleware.RequireAdmin` authenticates nothing: it sets `user_id = 1` in the Gin
  context and calls `Next()`. Its one remaining caller is
  `POST /admin/users/:telegramID/reset`, which is therefore open to anyone who knows
  the URL. The Mini App does **not** use it — `/app/*` authenticates via
  Telegram-signed initData and gates its admin views on `users.is_admin`
  (`miniapp/auth.go`).
- `intent_events.needs_confirmation` (NOT NULL) went vestigial after the router's UNCLEAR redesign: it is always written `false`. Drop it with a migration if you want it cleaned up.
- **`TestEveryConfigFile_HasNoSameTurnModelCollision` is red on purpose** (since 2026-08-17) — do not "fix" it by swapping models. Groq retired `llama-3.3-70b-versatile`, the model that used to hold the invariant, leaving **two** usable ones (`openai/gpt-oss-120b`, `openai/gpt-oss-20b`; qwen emits its reasoning inside the content and returns an empty narration). `sameTurnCalls` requires `agent` to differ from `classifier`, `query`, `create` *and* `narration`, which two models cannot satisfy. All three possible assignments were measured against the real eval: the current one keeps `query_eval` green and this test red; putting narration on 120b flips it, but then the bot answers "gastaste $0" about a category that does not exist. Lying to the user lost to bouncing off the quota. Production runs the same collision — the recurring `modelo sin cupo, probando el siguiente` in the Render logs is exactly it. The red goes away on its own once a third TPM bucket exists (another tier or provider). Full rationale in the test's own comment; note that `applyDefaults` also ships `NarrationModel`/`ClassifierModel` equal to `AgentModel`, so a brand-new environment inherits the collision.
- An account's two **opening** movements do not go through `movement.Normalize`, and cannot:
  they are a lone leg typed `Transfer` with no counterparty and no `transaction_id`, and the
  guard rejects any transfer that isn't a 2-leg group — it would reject *every* opening, at any
  amount. They take the whole `*account.Account` instead, which makes a currency mismatch
  unrepresentable. This is a decision, not debt; it is documented in the anti-pattern in
  [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#anti-patterns--what-not-to-do).
- `internal/controller/messaging` was ~10k non-test lines across 52 files — the driver of this split. Extracting one cluster at a time (copy→`messages`, flows→`flow`, loop→`agent`, QUERY→`query`, tips→`nudges`, 429 queue→`pendingjob`, wizards→`settings`) cut it to **1.6k non-test lines across 20 files**. For scale: `flow` 4.4k, `agent` 2.9k, `orchestrator` 2.6k, `query` 0.7k, `nudges` 0.7k, `settings` 0.4k, `pendingjob` 0.4k.
  What is left in `messaging` is the Telegram edge itself: the webhook handlers and `handleFlowFinished`'s switch, `/start` + invitations, the bridge files, `userLocks`, tracing, and the metric outcomes.
  Tests were split on the same line as the code: the builder/step tests live in `flow`, and the ones that drive a finish through the `*controller` stayed at the edge on purpose — they exercise webhook→engine→finish, which is the edge's job. Using `flow` does not make a test a `flow` test. Those edge tests are why the five one-line finish delegators (`finishAccountCreateFlow`, `finishReminderSetup`, …) still exist.
- `orchestrator.AgentTool.Kind` is **vestigial**: `orderCallsByKind`/`kindRank` were deleted in
  stage 5 and nothing reads the field. It matters only as a warning — if read tools ever return
  to `Run`, ordering must come back with them, and the zero value must be made unrepresentable
  (an unset `Kind` used to sort as `KindRead`, so a write tool declared without one ran *after*
  the reads). See `internal/orchestrator/CLAUDE.md`.
- `messaging.userLocks` serialises a user's writes with an in-memory mutex, so it holds for
  **one process only**. With a second instance the upgrade is `pg_advisory_xact_lock(user_id)`.
  Documented at the mutex itself.
- The near-duplicate gate (`near_duplicate_offer.go`) writes **without** going through
  `movement.Normalize`. It is safe because `nearDuplicateCandidate` requires the two rows to
  share type, currency and account — hence sign — but that is an invariant held one file away
  from the write. Loosening the same-type rule requires adding the guard back.

## 7. Claude Code Session Rules

- **Subagents run on `sonnet`.** Any `Agent` tool call spawned in this project (any `subagent_type`) must pass `model: "sonnet"` explicitly, unless the user asks otherwise for a specific task. Exception: `subagent_type: "fork"` always inherits the parent session's model — a `model` override is ignored for forks, so this rule doesn't apply to them.
- **Codegraph overrides skill-default exploration.** This repo has `.codegraph/` indexed. Any superpowers skill step that says "explore the codebase", "read relevant files", or spawns an `Explore`/`general-purpose` subagent for code lookup must use `codegraph_explore` first instead — one call returns verbatim source + call graph, versus dozens of raw `Read`/`Grep` round-trips a generic skill step defaults to. This applies mid-skill (brainstorming, writing-plans, systematic-debugging, etc.), not just standalone questions — skills don't know codegraph exists, so the substitution has to be made manually every time.
- **Stack mandate, always.** Codegraph before manual grep/read. The matching superpowers skill (brainstorming / systematic-debugging / writing-plans / TDD) before any feature or fix. Ponytail discipline (minimum code, no premature abstraction) on every diff. Caveman-compressed communication for agent/subagent output. None of these are skippable for a "simple" task — that rationalization is exactly what each skill/mode already warns against.
- **WIP=1 for delegated work.** One flow/fix active per subagent at a time. Don't activate a second task before the first has completion evidence (below). Parallel activation dilutes the reasoning budget available to each task — nothing finishes properly if it's split too thin.
- **Completion evidence, not self-assessment.** A subagent doesn't report a task "done" without having run `go build ./...` (or `go test ./...` if it touched tests) and shown the result. Green output is the only valid signal — not the agent's own read of its diff.

Use `repo-investigator`, `repo-builder`, and `repo-reviewer` (`.claude/agents/`) for locate/edit/review work scoped to this repo, instead of generic `Explore`/`general-purpose`/`code-reviewer` agents — they carry this repo's rules (money type, currency handling, `conversation_states` access, taxonomy) built into their prompts.
