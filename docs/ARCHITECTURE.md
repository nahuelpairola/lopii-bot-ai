# Architecture Reference — lopii-finance-bot

> Index of the project's reference docs. Read the one your task touches — don't load all of them.
> For code structure use `codegraph_explore` (verbatim source + call graph); these docs carry the curated "why" codegraph can't.

## Reference docs

- **[data-model.md](data-model.md)** — DB tables + key relationships (schema authority is `migrations/`).
- **[decisions.md](decisions.md)** — key design decisions and their rationale.
- **[business-rules.md](business-rules.md)** — currencies, the accounting/money model (full), grouping, taxonomy, reminders.
- **[recipes.md](recipes.md)** — how to add a migration, flow, LLM intent, scheduled notification, admin command.
- **[dev-setup.md](dev-setup.md)** — local Postgres, config, run, migrations.
- **[grafana/README.md](grafana/README.md)** — admin dashboard: how to import it, how to read it, and the manual verification checklist.

## Per-package `AGENTS.md`

Sixteen packages carry their own file. They split into two classes, and the split is a criterion,
not a taste: **a package loads at launch when ignoring its file records money wrong and nothing
warns you.** That is `movement` (a movement's sign and account), `conversation` (the JSONB
round-trip that turns a number back into `float64`), `agent` (a turn that wrote being
re-enqueued) and `pendingjob` (a job replayed twice). The root `CLAUDE.md` imports those four, so
they are in context before anyone types anything. The list lives in those imports — the only
place it cannot go stale.

The other twelve load **on demand**, and on demand is weaker than it sounds: it fires only when an
agent reads a file in that subtree, and `codegraph_explore` does not count as reading. Open them
deliberately before editing there.

Each of the twelve also has a one-line `CLAUDE.md` next to it that imports it (Claude Code reads
`CLAUDE.md`, not `AGENTS.md`; every other agent reads the nearest `AGENTS.md`). The content lives
in the `AGENTS.md` and only there — the `CLAUDE.md` is a pointer, never a copy. The four
always-loaded packages have **no** bridge file on purpose: the root import already carries them,
and a bridge would load the same text a second time.

Each holds one thing only: **rules that compile fine and then behave wrong.** Structure is
`codegraph_explore`'s job, not theirs.

**The index of which package has one, and the trap each exists for, is the table in
[AGENTS.md](../AGENTS.md#where-knowledge-lives)** — kept there because that file is loaded every
session and this one is not. It is not repeated here: two copies of a list like that drift, and
the stale one is always the copy.

For conventions and the money model, see [AGENTS.md](../AGENTS.md) (loaded every session).

## Do NOT read

- **`app_scripts_v1/`** — the v1 Google Sheets + Apps Script implementation. Historical
  reference only; no pattern, type or logic there applies to Go v2. Git-ignored: on disk
  locally, absent from a fresh clone.
- **`docs/superpowers/`** — spec/plan scratch from past sessions. Also git-ignored, so a
  spec/plan link elsewhere in `docs/` may 404 on a fresh clone. The curated "why" that survives
  lives in `decisions.md`.

---

## Anti-patterns — What NOT to do

- **Never use `float64` for money.** Always `shopspring/decimal`.
- **Never import concrete repository types across packages.** Controllers declare local interfaces for the repos they need.
- **Never create Telegram commands for end users.** Everything routes through the LLM orchestrator.
- **Never read `app_scripts_v1/`.** It's the v1 GAS implementation. No patterns from there apply here.
- **Never access `conversation_states` directly via GORM.** Always use `conversation.Engine`.
- **Never convert currencies implicitly.** ARS and USD stay separate in all computations and reports.
- **Never add a `balance` field to `accounts`.** Balance is computed from movements — always.
- **Never invent category/subcategory names in code or prompts.** The taxonomy lives in the DB, seeded via migrations.
- **Never use raw strings for currency values.** Use `currency.ARS` / `currency.USD` constants.
- **Never store a native Go number in `conversation.Data`.** Always encode as a string — the JSONB round-trip silently turns numbers into `float64`, which violates the money rule.
- **Never duplicate a literal value.** A string or number used more than once is a named constant. Scope it to its reach: an unexported `const` in the package if it's local; `internal/constants` if it's used across packages (typed wrappers may re-export it, as `currency.ARS = constants.ARS`); an **exported** const in the producing package if one package owns the value but another reads it (as `conversation.ResumeCancelledKey`). One const per distinct value; one const per distinct *meaning* even when two values share a string. Single-use literals stay inline. Every `conversation.Data` map key is an exported const in `internal/conversation/data.go`.
- **Never insert a movement without an `account_id`.** Every expense/income/transfer attributes to a real account (see [business-rules.md](business-rules.md#the-accounting-model)). A NULL account means the balance never reflects that movement.
- **Never let the LLM decide a movement's sign, and never let a stored sign escape storage.** The app normalizes sign by type on write; user and LLM both see `abs`. Feeding a signed amount to the user or to an UPDATE/DELETE candidate is a bug (it caused a real `0.00` corruption).
- **Never trust the LLM's `amount` sign, `account_id` validity, or currency/account agreement without the guard.** `movement.Normalize` re-derives sign, resolves the account, and rejects `amount == 0` / currency mismatch — for CREATE and UPDATE alike.
- **Never write a movement whose currency comes from somewhere other than its own account.** This is the invariant every write path shares, and the one that corrupts a balance *silently*: a balance is `SUM(amount)` and never looks at each row's currency, so a USD row on a peso account is simply added in. How you satisfy it depends on where the movement comes from:

  | Origin | How | Why |
  |---|---|---|
  | An LLM-built set (CREATE/UPDATE) | `movement.Normalize` | Sign, `account_id` and currency are all untrusted — the full guard is the point. |
  | An app-built movement against a known account (balance adjustment) | `movement.Normalize` too | Cheap, and it validates the flow's notion of the account against the DB's. |
  | An account's **opening** movement | Take the `*account.Account` and read both `AccountID` and `Currency` off it | The mismatch becomes unrepresentable — strictly better than checking for it. **Do not route these through `Normalize`**: an opening is a lone leg typed `Transfer` (so it stays out of cash-flow aggregates) with no counterparty and no `transaction_id`, and the guard rejects any transfer that isn't a distinct 2-leg group — it would reject *every* opening, at any amount. Its amount may also legitimately be `0`. |
- **Never call an orchestrator method from a webhook site without routing its error through `pendingjob.HandleGroqError`.** A terminal Groq 429 (`orchestrator.RateLimitedError`) must be enqueued into `pending_llm_jobs` and acked, not shown as `msgSomethingBroke` — a site that skips `pendingjob.HandleGroqError` silently drops the user's message on rate limit instead of queuing it for the drain worker (`internal/pendingjob/enqueue.go`/`drain.go`). See [recipes.md](recipes.md#recipe-5-wire-a-new-groq-calling-site-into-the-pending-jobs-queue).
- **Never turn on GORM's SQL logger in production.** `database.Initialize`'s `debug` flag gates
  it, independently of `internal/logging`. On, it prints every slow query (200ms default) with
  its values fully interpolated — raw amounts, descriptions, account names — to stdout,
  regardless of the app's configured log level. It is local-dev only.
- **Never read `usd_quotes` with `date = D`, and never treat `monthly_cpi.value` as an index level.** Both series come from public sources with shapes that produce a plausible wrong number rather than an error — irregular gaps, an inverted spread, a percentage that is not a level. The four traps and how to read around them are in [`internal/quote/AGENTS.md`](../internal/quote/AGENTS.md); read it before writing the first reader of either table.
- **Never type an optional (non-`required`) tool-schema field as a bare scalar.** Groq validates the model's tool-call against the schema we send; the model emits `null` for an absent optional, and a bare `"string"`/`"integer"` 400s on that null (a real prod failure: an optional string field emitted as `null` on "pago tarjeta"). Every property not in the schema's `required` list must be a null-union (`["string", "null"]`). `internal/orchestrator/schema_test.go` enforces this across all tool schemas — see [recipes.md Recipe 3](recipes.md#recipe-3-add-an-llm-intent) before adding or promoting a field.
- **Never gate an admin route with `middleware.RequireAdmin`.** It authenticates nothing: it sets `user_id = 1` in the Gin context and calls `Next()`, so any route behind it is open to anyone who knows the URL. A real admin surface goes on the Mini App's `authed` group behind a middleware that reads the `is_admin` flag `authInitData` stamped from the users row — the Mini App's own `/app/admin` example of that pattern was removed on 2026-08-25 (see [recipes.md Recipe 4](recipes.md#recipe-4-add-an-admin-command)), but the rule for wherever a real one lands next is unchanged. The reset endpoint's use of `RequireAdmin` is debt, not a pattern.

---

## Update contract

It lives in [AGENTS.md](../AGENTS.md#the-update-contract), which is loaded every session —
this file is not. Implementing something includes updating the harness in the same commit.

---

## Technical debt

Only what no package owns. Anything package-scoped lives in that package's `AGENTS.md`; the
reasoning behind a design choice lives in [decisions.md](decisions.md).

- `accounts` has a `type DEFAULT 'standard'` column from a prior design — drop with a migration.
- Migration `20260618230837_create_admin_user.sql` has a literal `telegram_id = 'TELEGRAM_ID'` —
  edit by hand before each new-environment deploy.
- **`middleware.RequireAdmin` authenticates nothing**: it sets `user_id = 1` and calls `Next()`.
  Its one caller, `POST /admin/users/:telegramID/reset`, is open to anyone who knows the URL. The
  Mini App does **not** use it — `/app/*` validates Telegram-signed initData and gates admin views
  on `users.is_admin`.
- `intent_events.needs_confirmation` (NOT NULL) is vestigial — always written `false`.
- Two in-memory ceilings that both hold for **one process only**: `messaging.userLocks` and
  `pendingjob`'s drain gate. A second instance needs Postgres for both.
- `ClassifyCreate` and `ResolveDelete` survive only as methods on test fakes; stage 5 removed
  every production caller.
