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

## Per-package `CLAUDE.md`

Fourteen packages carry their own file. They are **not** loaded at session start — Claude Code
pulls one in only when it reads a file in that subtree, and `codegraph_explore` does not count as
reading, so on most tasks you have to open it deliberately.

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
- **Never read `usd_quotes` with `date = D`, and never treat `monthly_cpi.value` as an index level.** Both series come from public sources with shapes that produce a plausible wrong number rather than an error — irregular gaps, an inverted spread, a percentage that is not a level. The four traps and how to read around them are in [`internal/quote/CLAUDE.md`](../internal/quote/CLAUDE.md); read it before writing the first reader of either table.
- **Never type an optional (non-`required`) tool-schema field as a bare scalar.** Groq validates the model's tool-call against the schema we send; the model emits `null` for an absent optional, and a bare `"string"`/`"integer"` 400s on that null (a real prod failure: an optional string field emitted as `null` on "pago tarjeta"). Every property not in the schema's `required` list must be a null-union (`["string", "null"]`). `internal/orchestrator/schema_test.go` enforces this across all tool schemas — see [recipes.md Recipe 3](recipes.md#recipe-3-add-an-llm-intent) before adding or promoting a field.
- **Never gate an admin route with `middleware.RequireAdmin`.** It authenticates nothing: it sets `user_id = 1` in the Gin context and calls `Next()`, so any route behind it is open to anyone who knows the URL. A real admin surface goes on the Mini App's `authed` group behind `requireAdmin()`, which reads the `is_admin` flag `authInitData` stamped from the users row. `/app/admin` is the worked example; the reset endpoint's use of `RequireAdmin` is debt, not a pattern.

---

## Update Contract

**Before closing any session that adds or changes something, update the right doc.**

| If you added or changed... | Update |
|---------------------------|--------|
| A new DB table or migration | `docs/data-model.md` |
| A new architectural decision | `docs/decisions.md` |
| A new anti-pattern identified | `docs/ARCHITECTURE.md` (Anti-patterns, above) |
| A new recipe (flow type, step type, intent) | `docs/recipes.md` |
| A new business rule | `docs/business-rules.md` |
| A new dependency added to `go.mod` | `AGENTS.md` → Stack |
| A rule that compiles fine and then behaves wrong | that package's `CLAUDE.md` — **not** here |
| A value reused across files or packages | define a constant scoped per the no-duplicated-literal rule (`internal/constants` if cross-package) |

**A new package needs no doc entry.** There is no package map any more: `codegraph_explore`
answers "what does this package own" in one call and never goes stale. The map that used to
live in `docs/package-map.md` was deleted for exactly that reason — nobody re-derived it, so it
drifted into naming a package that no longer existed.

**Nothing is added to a doc that merely restates the code.** A file that is accurate but
derivable is a future lie with a timer on it.
