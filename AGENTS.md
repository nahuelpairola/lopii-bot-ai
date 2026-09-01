# AGENTS.md — lopii-finance-bot

Personal finance Telegram bot for Argentine users (ARS/USD). Free-text input via Telegram, an
LLM agent loop that picks a tool, PostgreSQL persistence, Argentine financial context.

**v1:** Google Sheets + Apps Script in `app_scripts_v1/` — historical reference only, git-ignored.
**No pattern, type or logic there applies to Go v2.** **v2:** this repo, a Go rewrite.

Stack: Go, Gin, GORM, Postgres (Supabase), Goose migrations at startup, go-telegram/bot in webhook
mode, Viper config (TOML per environment, env vars override), `shopspring/decimal` for money, Groq
tool-calling over plain `net/http`, deployed to Render via Docker. Versions live in `go.mod`.

## How work gets done here

**The superpowers skill comes before the work, not after.** Not skippable for a "simple" task —
that rationalization is what each skill already warns about.

| You are about to | Invoke |
|---|---|
| Create, add or change behaviour | `superpowers:brainstorming` |
| Fix a bug, or explain unexpected behaviour | `superpowers:systematic-debugging` |
| Turn an approved spec into steps | `superpowers:writing-plans` |
| Write any implementation code | `superpowers:test-driven-development` |
| Call something done | `superpowers:verification-before-completion` |

`bash check.sh` is the completion evidence, and **`check: OK` is the only green that counts** —
never your own reading of your own diff.

A session closes leaving the tree in a state the next one can start from without archaeology.
Whatever is half-done is said **in the commit**, not remembered.

## No comments in Go source

**The code is the truth. A comment that lies is worse than no comment**, because an agent believes
it and nothing turns red. Write comments while you work if they help; **delete them before you
commit.** `TestNoCommentsInGoSource` (`internal/harness`) fails on the first one that survives.

The one exception is a **tool directive** — a line the toolchain reads, which asserts nothing about
behaviour: `//go:build`, `//go:embed`, `//lint:`, and templ's `// Code generated` and
`// templ: version:` headers.

Where the "why" goes instead:

| The claim is | It goes to |
|---|---|
| Testable | a test whose **name** states the rule |
| A measurement, an incident, a rejected alternative | `docs/decisions.md` |
| A rule that compiles fine and behaves wrong | that package's `AGENTS.md` |

**Every plan written by `writing-plans` must state this rule in its own section**, directives
included. A plan that omits it produces tasks that leave scaffolding behind.

## The update contract

**Implementing something includes updating the harness, in the same commit.** Not a follow-up.

| You added or changed | Update |
|---|---|
| A rule that compiles fine and then behaves wrong | that package's `AGENTS.md` — **nowhere else** |
| A design decision, or the measurement behind one | `docs/decisions.md` |
| An anti-pattern worth naming | `docs/ARCHITECTURE.md` |
| A DB table or migration | `docs/data-model.md` |
| A business rule | `docs/business-rules.md` |
| A flow type, step type or intent | `docs/recipes.md` |
| Anything visual in the Mini App | `DESIGN.md`, **on the same branch** |

If the change added no new trap, say so in the package's file: "no new traps" is information.
**Nothing is added to a doc that merely restates the code** — a file that is accurate but
derivable is a future lie with a timer on it.

## Build, test, lint

```
bash check.sh        # build + vet + errcheck + the default test suite
```

Four build tags gate the suites `check.sh` does not run, and `errcheck` is information rather
than a gate: [docs/dev-setup.md](docs/dev-setup.md#tests).

Two expected reds, both deliberate — do not "fix" either:

- `TestEveryConfigFile_HasNoSameTurnModelCollision`, red by decision since 2026-08-17: Groq left
  two usable models where the invariant needs three, and **swapping models to make it green makes
  the bot lie to users**. `check.sh` excuses this one by name and nothing else; it goes green on
  its own once a third TPM bucket exists.
- Anything needing Postgres, when Postgres is down. Check that before diagnosing.

`gofmt -l` reports **every** file on Windows (`core.autocrlf=true`, no `eol` rule for `*.go`).
Normalize before believing it: `tr -d '\r' < f.go | gofmt -l`.

## Conventions

**Language.** Telegram UI strings in Argentine Spanish. All code identifiers and **all
documentation** in English.

**Money.** Always `shopspring/decimal`. **Never `float64` for a monetary amount.**

**Repository pattern.** Each consumer declares its own local interface for what it needs. Never
import a concrete repository type across packages.

**Sentinel errors.** One per package. Postgres unique violations map through
`errors.As(err, &pgErr) && pgErr.Code == "23505"`.

**Tests.** Unit tests mock the package's own local interface, not the concrete type. No real
Postgres outside the `integration` tag.

**Conversation state.** JSONB in `conversation_states`. **Never touch that table directly** — all
access goes through `conversation.Engine`.

**Admin IDs.** Loaded into memory at startup. Zero extra queries per Telegram request.

**No duplicated literals.** A string or number used more than once becomes a named constant scoped
to its reach: unexported in the package, `internal/constants` when cross-package, **exported** in
the producing package when one package owns it and another reads it. One const per distinct value,
and one per distinct *meaning* even when the strings collide. Single-use literals stay inline.

**No duplicated knowledge.** Before adding anything to the harness, look for where it already
is. A rationale lives in exactly one place; finding it in two means one is already rotten — fix
that one, do not add a third. And before creating a file, ask whether something **executable**
can do the same job: a script that fails beats a paragraph that rots.

## The accounting model — read before touching any money path

**Every movement is signed and attributed to a real account. No exceptions.** A balance is *always*
`SUM(amount)` over an account's movements, so the stored sign IS the accounting. A wrong sign or
`account_id` silently corrupts a balance — the one place here where a small mistake is a financial
bug rather than a cosmetic one.

| Type | `account_id` | Stored sign | Meaning |
|---|---|---|---|
| `expense` | source account (required) | **negative** (`-amount.Abs()`) | money leaves an account |
| `income` | destination account (required) | **positive** (`+amount.Abs()`) | money enters an account |
| `transfer` | both legs (required) | negative out / positive in | money moves between own accounts |

- **The app owns the sign, never the LLM.** The guard normalizes on write.
- **The app owns the arithmetic too.** A correction arrives as a structured diff
  (`field`/`op`/`value`) and the app computes the result — the model never sends a number it worked
  out itself. "They refunded me half" is `op: multiply, value: 0.5`.
- **The sign never escapes storage.** User and LLM both see `amount.Abs()`; direction comes from
  the movement type, never a `-`.
- **Account resolution is app-side and deterministic:** LLM-matched account → currency default
  (`FindDefaultByCurrency`) → gap-fill asks.

Full model → [docs/business-rules.md](docs/business-rules.md#the-accounting-model-money-precision--read-this-before-touching-any-money-path).

## Where knowledge lives

Six layers. **A rationale — the measurement, the incident, the rejected alternative — lives in
exactly ONE.** If you find the same *reasoning* in two places, one is already stale: fix it, do not
add a third.

| Layer | Holds | Read it when |
|---|---|---|
| The code | what the system does | always — the only source of truth for behaviour |
| `internal/<pkg>/AGENTS.md` | the traps of that package: what compiles fine and behaves wrong | you touch any file in that package |
| [docs/decisions.md](docs/decisions.md) | **why the design is what it is**: measurements, incidents, rejected alternatives | a doc points you there, or you are about to change a design choice |
| [DESIGN.md](DESIGN.md) | the Mini App's design system | before changing anything visual there |
| [PRODUCT.md](PRODUCT.md) | who the product is for and what it promises | before deciding what a surface should *do* or say |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | the index of every reference doc, the anti-pattern list, and the open technical debt | you need any of those three |

Every package with traps carries its own `AGENTS.md`, and **editing a package means reading its
file first** — the trap is never visible in the code. The rest load only when something reads a
file in that subtree, which is not guaranteed, so open them deliberately.

**Never edit `DESIGN.md` by hand** — it is generated, and drifts from its sidecar silently.
How both root docs are generated: `docs/ARCHITECTURE.md`.

For structure, ask `codegraph_explore`: one call returns verbatim source plus the call graph.
Once you know which file you are changing, **read that file** — `Read` also pulls in its package's
`AGENTS.md`, which `codegraph_explore` does not. `Grep` still wins for plain text.
