# AGENTS.md — lopii-finance-bot

Personal finance Telegram bot for Argentine users (ARS/USD). Free-text input via Telegram, an
LLM agent loop that picks a tool, PostgreSQL persistence, Argentine financial context.

**v1:** Google Sheets + Apps Script in `app_scripts_v1/` — historical reference only. Git-ignored:
on disk locally, absent from a fresh clone. **No pattern, type or logic there applies to Go v2.**
**v2:** this repo, a Go rewrite.

## Stack

- Go, Gin, GORM, Postgres (Supabase)
- Goose for migrations (run automatically at server startup)
- go-telegram/bot in webhook mode
- Viper for config (TOML per environment; env vars override)
- shopspring/decimal for money
- LLM orchestrator via Groq (tool calling, plain `net/http`, no SDK)
- Deployment: Render + Docker

## Build, test, lint

```
bash check.sh        # build + vet + errcheck + the default test suite
```

**`check.sh` is the completion evidence.** It is the one command to run before calling anything
done, and `check: OK` is the only green that counts. It knows about the expected red below and
excuses that one test *by name*, so it still exits 1 on any real failure. `errcheck` runs inside
it as **information, not a gate** — the tree carries ~215 pre-existing findings (~38 outside
tests, nearly all unchecked `bot.SendMessage`); read the ones in your own diff and ignore the
rest. There is no `make` on the Windows box this repo is developed on, which is why the old
`make lint` target was documented but unrunnable, and now gone.

Four build tags gate the suites that need something the default run does not have. **None of
them run in CI — there is no CI.** They only run when someone runs them.

| Tag | Needs | Notes |
|---|---|---|
| `integration` | local Postgres (`docker compose up -d`) | 11 files. Drains and writes real rows |
| `conv_test` | nothing | 3 files |
| `llm_eval` | `GROQ_APIKEY` | 3 files, `internal/orchestrator` |
| `query_eval` | `GROQ_APIKEY` **and** Postgres | 1 file |

**Both eval suites now FAIL LOUDLY when `GROQ_APIKEY` is unset**, instead of skipping in silence.
The tag is asked for by hand, so a skip was a green that proved nothing. They still spend real
Groq quota, which is per model and shared with production. Until 2026-08-19 the three
`llm_eval` files read `GROQ_API_KEY` — a name nothing exports — so they had *never* actually run;
`GROQ_APIKEY` (the `.env` name, and what Viper derives from `groq.apiKey`) is the only one.

Two expected reds, both deliberate — do not "fix" either:

- `TestEveryConfigFile_HasNoSameTurnModelCollision` — red since 2026-08-17 by decision. Groq left
  two usable models where the invariant needs three. **Swapping models to make it green makes the
  bot lie to users** — the three possible assignments were measured against the real eval. Full
  rationale in the test's own comment (`internal/config/config_test.go`) and at `sameTurnCalls`
  (`config.go`). It goes green on its own once a third TPM bucket exists.
- Anything needing Postgres, when Postgres is down. Check that before diagnosing.

`gofmt -l` reports **every** file on Windows: `core.autocrlf=true` and no `eol` rule for `*.go`,
so the whole tree looks unformatted. Normalize before believing it: `tr -d '\r' < f.go | gofmt -l`.
The real fix — `*.go text eol=lf` in `.gitattributes` plus `git add --renormalize .` — is a
~200-file mechanical commit, deliberately deferred until `refactor/messaging-split` merges. The
`.gitattributes` that exists today only pins `*.sh`, because a CRLF shebang breaks `check.sh`.

## Conventions

**Language.** Telegram UI strings and code comments in Argentine Spanish. All code identifiers in
English. **All documentation — this file, `CLAUDE.md`, `docs/` — in English.**

**Money.** Always `shopspring/decimal`. **Never `float64` for a monetary amount.**

**Repository pattern.** Each consumer declares its own local interface for what it needs. Never
import a concrete repository type across packages:

```go
type accountRepository interface {
    Insert(*account.Account) error
}
```

**Sentinel errors.** One per package (e.g. `account.ErrAccountAlreadyExists`). Postgres unique
violations:

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    return ErrAccountAlreadyExists
}
```

**Tests.** Unit tests mock the package's own local interface — not the concrete type. No real
Postgres outside the `integration` tag.

**Conversation state.** JSONB in `conversation_states`. **Never touch that table directly** — all
access goes through `conversation.Engine`.

**Admin IDs.** Loaded into memory at startup. Zero extra queries per Telegram request.

**No duplicated literals.** A string or number used more than once becomes a named constant,
scoped to its reach:

| Reach | Home |
|---|---|
| Used 2+× in one package | unexported `const` there |
| Used across packages | `internal/constants` (raw string); typed wrappers may re-export (`currency.ARS = constants.ARS`) |
| Owned by one package, read by another | **exported** `const` in the producer (e.g. `conversation.ResumeCancelledKey`) |

One const per distinct value, and one per distinct *meaning* even when the strings collide (a
button value and a `Data` key both spelled `"edit_proposal"` are two consts). Single-use literals
stay inline. Every `conversation.Data` map key is a const (`internal/conversation/data.go`).

## The accounting model — read before touching any money path

**Every movement is signed and attributed to a real account. No exceptions.** A balance is
*always* `SUM(amount)` over an account's movements, so the stored sign IS the accounting. A wrong
sign or `account_id` silently corrupts a balance — the one place in this codebase where a small
mistake is a financial bug rather than a cosmetic one.

| Type | `account_id` | Stored sign | Meaning |
|---|---|---|---|
| `expense` | source account (required) | **negative** (`-amount.Abs()`) | money leaves an account |
| `income` | destination account (required) | **positive** (`+amount.Abs()`) | money enters an account |
| `transfer` | both legs (required) | negative out / positive in | money moves between own accounts |

- **The app owns the sign, never the LLM.** The guard normalizes on write.
- **The app owns the arithmetic too.** A correction arrives as a structured diff
  (`field`/`op`/`value`) and the app computes the result — the model never sends a number it
  worked out itself. "They refunded me half" is `op: multiply, value: 0.5`, not a recomputed
  amount. `guardRefundDirection` checks the *result* against the stored row, not the `op`.
- **The sign never escapes storage.** User and LLM both see `amount.Abs()`; direction comes from
  the movement type, never a `-`.
- **Account resolution is app-side and deterministic:** LLM-matched account → currency default
  (`FindDefaultByCurrency`) → gap-fill asks.

Full model (guard invariants, insufficient-funds gate, grouped transactions, FCI redemption) →
[docs/business-rules.md](docs/business-rules.md#the-accounting-model-money-precision--read-this-before-touching-any-money-path).

## Where knowledge lives

Four layers. **A rationale - the measurement, the incident, the rejected alternative - lives in
exactly ONE of them.** A one-line statement of a constraint may legitimately appear both at its
code site and in a package's trap list; a retold incident or a measured number may not. If you
find the same *reasoning* in two places, one of them is already stale: fix it, do not add a third.

| Layer | Holds | Read it when |
|---|---|---|
| The code | what the system does | always - it is the only source of truth for behaviour |
| A comment at a line | why *this* line is what it is: the conclusion, 1-3 lines | you are editing that line |
| `internal/<pkg>/AGENTS.md` | the traps of that package: what compiles fine and behaves wrong | you touch any file in that package |
| [docs/decisions.md](docs/decisions.md) | **why the design is what it is**: measurements, incidents, rejected alternatives | a comment points you there, or you are about to change a design choice |
| [DESIGN.md](DESIGN.md) | the Mini App's design system: the Telegram token mapping, the type ramp, the 44px rule and the three fixed-colour exceptions | before changing anything visual in the Mini App |

`docs/decisions.md` is the long form and the one most easily forgotten. It is grouped into nine
addressable sections - money model, agent loop and QUERY, Groq quota, taxonomy, flows - so a code
comment can point at one. **Before changing a design choice, check whether it is already argued
there**: most were, with the production numbers that settled them.

Two more, read on demand: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) indexes every reference doc
(data model, business rules, recipes, dev setup, Grafana) and holds the anti-pattern list.

`DESIGN.md` and `PRODUCT.md` sit at the repo root and are **generated** by `/impeccable document`
from the shipped artifact, with `.impeccable/design.json` as their sidecar. Change them by
regenerating, never by hand — an edited `DESIGN.md` drifts from the sidecar silently. Until
2026-08-29 nothing in this table named `DESIGN.md`, so it was invisible to anyone reading only
the index; that is how a comment purge nearly deleted measurements it turned out to hold.

For structure, this repo has `.codegraph/` indexed: `codegraph_explore` answers "where is X, what
calls Y, how does this flow" in one call, with verbatim source plus the call graph. Use it to get
oriented. Once you know which file you are changing, **read that file** - `Read` also pulls in its
package's `AGENTS.md`, which `codegraph_explore` does not. `Grep` still wins for plain text.

Fifteen packages carry their own `AGENTS.md`. **If you are editing one, read its file first** -
the trap is not visible in the code. Four of them - `movement`, `conversation`, `agent`,
`pendingjob` - are marked **always** below: they are the money path, where ignoring the file
records money wrong with nothing to warn you, so `CLAUDE.md` imports them at launch and they are
loaded before you start. The other eleven load only when something reads a file in that subtree,
which is not guaranteed - open them deliberately. (Each of those eleven also has a one-line
`CLAUDE.md` importing it, because Claude Code reads `CLAUDE.md` and not `AGENTS.md`. The content
lives in the `AGENTS.md` and only there.)

| Package | Loaded | The trap it exists for |
|---|---|---|
| `agent` | **always** | the only entry point for free text; a turn that wrote must never be re-enqueued; `resolveCandidates` has two windows |
| `orchestrator` | on demand | three call types silently share `createModel`; Groq's ceilings are per model; `AgentTool.Kind` |
| `query` | on demand | one `search` filter matched in SQL; the app re-attaches two facts after narration |
| `flow` | on demand | a flow must be registered in 3 places across 2 packages; `callback_data` is 64 bytes |
| `conversation` | **always** | `Data` round-trips through JSONB - numbers come back `float64` |
| `movement` | **always** | the guard covers INSERT by caller convention only; two finders need opposite date binding |
| `subcategory` | on demand | cache writes need a manual `Reload()`; `c.global` is shared by every user |
| `settings` | on demand | the only caller of three LLM calls; a 429 is answered before the wizard fallback |
| `pendingjob` | **always** | the replay flag is set once at the call site; `EnqueueBehindPending` is webhook-only |
| `nudges` | on demand | `MarkSent` is once-ever; a tip's tap jumps the engine on purpose |
| `notifier` | on demand | the sweeper sends with `ParseMode: HTML` - every emitter must be HTML-safe |
| `quote` | on demand | `usd_quotes` has irregular gaps (read `<= D`, never `= D`); `monthly_cpi.value` is a % change, not a level |
| `controller/messaging` | on demand | the bridge pattern; the per-user lock; what bypasses the engine |
| `controller/miniapp` | on demand | **auth is which Gin group you register on, and nothing else** |
| `messenger` | on demand | the only place that knows a channel exists; the core never branches on `Channel` |

Every other package is a plain model + repository. Ask codegraph.

## Technical debt

Only what no package owns. Anything package-scoped lives in that package's `AGENTS.md`; the
reasoning behind a design choice lives in [docs/decisions.md](docs/decisions.md).

- `accounts` has a `type DEFAULT 'standard'` column from a prior design - drop with a migration.
- Migration `20260618230837_create_admin_user.sql` has a literal `telegram_id = 'TELEGRAM_ID'` -
  edit by hand before each new-environment deploy.
- **`middleware.RequireAdmin` authenticates nothing**: it sets `user_id = 1` and calls `Next()`.
  Its one caller, `POST /admin/users/:telegramID/reset`, is open to anyone who knows the URL. The
  Mini App does **not** use it - `/app/*` validates Telegram-signed initData and gates admin views
  on `users.is_admin` (`miniapp/auth.go`).
- `intent_events.needs_confirmation` (NOT NULL) is vestigial - always written `false`.
- Two in-memory ceilings that both hold for **one process only**, documented at each site:
  `messaging.userLocks` and `pendingjob`'s drain gate. A second instance needs Postgres for both.
