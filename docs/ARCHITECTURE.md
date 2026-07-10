# Architecture Reference — lopii-finance-bot

> Index of the project's reference docs. Read the one your task touches — don't load all of them.
> For code structure use `codegraph_explore` (verbatim source + call graph); these docs carry the curated "why" codegraph can't.

## Reference docs

- **[package-map.md](package-map.md)** — what each `internal/` package owns; what NOT to read (`app_scripts_v1/`).
- **[data-model.md](data-model.md)** — DB tables + key relationships (schema authority is `migrations/`).
- **[features.md](features.md)** — feature inventory: shipped vs. not started.
- **[decisions.md](decisions.md)** — key design decisions and their rationale.
- **[business-rules.md](business-rules.md)** — currencies, the accounting/money model (full), grouping, taxonomy, reminders.
- **[recipes.md](recipes.md)** — how to add a migration, flow, LLM intent, scheduled notification, admin command.
- **[dev-setup.md](dev-setup.md)** — local Postgres, config, run, migrations.

For conventions and the condensed money-model warning, see `CLAUDE.md` (always loaded).

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
- **Never insert a movement without an `account_id`.** Every expense/income/transfer attributes to a real account (see [business-rules.md](business-rules.md#the-accounting-model)). A NULL account means the balance never reflects that movement.
- **Never let the LLM decide a movement's sign, and never let a stored sign escape storage.** The app normalizes sign by type on write; user and LLM both see `abs`. Feeding a signed amount to the user or to an UPDATE/DELETE candidate is a bug (it caused a real `0.00` corruption).
- **Never trust the LLM's `amount` sign, `account_id` validity, or currency/account agreement without the guard.** `normalizeMovements` re-derives sign, resolves the account, and rejects `amount == 0` / currency mismatch — for CREATE and UPDATE alike.

---

## Update Contract

**Before closing any session that adds or changes something, update the right doc.**

| If you added or changed... | Update |
|---------------------------|--------|
| A new package | `docs/package-map.md` |
| A new DB table or migration | `docs/data-model.md` |
| A feature completed, started, or descoped | `docs/features.md` |
| A new architectural decision | `docs/decisions.md` |
| A new anti-pattern identified | `docs/ARCHITECTURE.md` (Anti-patterns, above) |
| A new recipe (flow type, step type, intent) | `docs/recipes.md` |
| A new business rule | `docs/business-rules.md` |
| A new dependency added to `go.mod` | `CLAUDE.md` → Stack |
