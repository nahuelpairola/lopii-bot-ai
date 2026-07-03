# Architecture Reference — lopii-finance-bot

> Single source of truth for what exists in this project.
> **Read this before starting any task. Update it when you add something.**
> For conventions, recipes, and business rules → see `CLAUDE.md`.

---

## 1. Package Map

### Where things live

| Package | Files | Owns |
|---------|-------|------|
| `cmd/server/` | `main.go` | Entrypoint — loads config, calls `server.InitServer` |
| `internal/server/` | `server.go` | Composition root — wires DB, bot, repos, engine, controllers, starts Gin |
| `internal/config/` | `config.go` | Viper TOML config struct + loader |
| `internal/database/` | `connection.go` | GORM+Postgres singleton; Goose migration runner |
| `internal/currency/` | `currency.go` | `Currency` type alias; `ARS`/`USD` constants; `SupportedCurrencies` |
| `internal/constants/` | `constants.go` | Movement type strings (`expense`, `income`) |
| `internal/health/` | `health.go` | `HealthChecker` — DB ping |
| `internal/middleware/` | `auth.go` | `RequireAdmin(adminID)` Gin middleware |
| `internal/user/` | `user.go` | `User` GORM model + repository (`FindByTelegramID`, `FindByID`, `Insert`) |
| `internal/invitation/` | `repository.go` | `Invitation` model + repository (`Create`, `FindByCode`, `MarkAsUsed`) |
| `internal/account/` | `repository.go`, `messages.go` | `Account` model + full repository + Spanish UI strings |
| `internal/subcategory/` | `repository.go`, `messages.go`, `icons.go` | `Subcategory` model + repository + Spanish UI strings + `IconFor`/`CategoryIcon` static category→emoji map |
| `internal/movement/` | `repository.go`, `last_transaction_store.go`, `messages.go` | `Movement` model + repository (`InsertBatch`, `FindSimilarForUser` (pg_trgm fuzzy search), `SoftDeleteByIDs`, `ReplaceMovements`, `SumAmountForAccount`) + `LastTransactionStore` (in-memory, per-user, most-recent transaction group) + `IconForType`/`TypeFromString` helpers |
| `internal/orchestrator/` | `orchestrator.go`, `client.go`, `types.go`, `router.go`, `create.go`, `update.go`, `delete.go` | Groq tool-calling HTTP client (plain `net/http`, no SDK) — `ClassifyIntent` (Call 1 router: CREATE/UPDATE/DELETE/QUERY), `ClassifyCreate` (Call 2 CREATE: builds 1..N movement drafts from free text + taxonomy + accounts), `ResolveUpdate` (Call 2 UPDATE: matches a candidate transaction + correction, returns corrected movement set), `ResolveDelete` (Call 2 DELETE: confirms which candidate a delete means, never rebuilds rows) |
| `internal/conversation/` | `flow.go`, `engine.go`, `repository.go`, `text_step.go`, `choice_step.go` | Conversation state-machine framework; `Step.Skip`/`ChoiceStep.SkipIf`/`TextStep.SkipIf` (opt-in, re-evaluated after every `Advance`) + `Engine.StartWithData` (start a flow pre-seeded with data, skipping already-resolved steps) |
| `internal/controller/health/` | `controller.go` | `GET /health/internal`, `HEAD /health/external` |
| `internal/controller/invitation/` | `controller.go` | `POST /invitations` (admin-only) |
| `internal/controller/messaging/` | `controller.go`, `start.go`, `messages.go`, `initial_balance_flow.go`, `free_text.go`, `movement_flow.go`, `movement_create_flow.go`, `movement_update_flow.go`, `movement_delete_flow.go`, `reference_resolution.go` | Telegram handlers: `/start` + catch-all for free text and callbacks; onboarding's mandatory `initial_balance_setup` flow; `free_text.go` routes every free-text message through Call 1 into CREATE/UPDATE/DELETE (QUERY replies "not supported yet"); `movement_create_flow.go` (frictionless insert or gap-fill `ChoiceStep`s), `movement_update_flow.go` + `movement_delete_flow.go` (two-hop pick→confirm flow chains), `reference_resolution.go` (shared `lastTransaction`/`pg_trgm` candidate lookup) |
| `migrations/` | `*.sql` | Goose migrations — **authoritative DB schema**; includes `pg_trgm` extension + GIN trigram indexes on `movements.description`/`merchant` (`20260702120000`) |
| `config/` | `local.toml`, `dev.toml`, `prd.toml` | Environment configs (secrets go here, git-ignored for local); `[groq]` section — per-call-type model name, API key, base URL, timeout |

### Do NOT read

- `app_scripts_v1/` — v1 in Google Sheets + Apps Script. **Historical reference only.** No patterns, types, or logic from here apply to Go v2.

---

## 2. Data Model

| Table | Key Fields | Notes |
|-------|-----------|-------|
| `users` | `id`, `telegram_id` (UNIQUE), `username`, `is_admin`, `deleted_at` | Soft delete |
| `invitations` | `id`, `code` (6-char UNIQUE), `created_by`→users, `used_by`→users, `expires_at`, `used_at` | 72h expiry, single-use |
| `accounts` | `id`, `user_id`→users, `name`, `type` (legacy), `currency`, `is_default`, `deleted_at` | `type` column is a legacy artifact — needs a drop migration |
| `conversation_states` | `user_id` (PK)→users, `flow_name`, `step_name`, `data` (JSONB), `updated_at` | One row per user; never access directly — use `conversation.Engine` |
| `subcategories` | `id`, `user_id` (nullable)→users, `category`, `subcategory`, `description`, `is_global`, `deleted_at` | `user_id = NULL` = global (visible to all users) |
| `movements` | `id`, `transaction_id` (UUID nullable), `user_id`→users, `account_id` (nullable)→accounts, `subcategory_id`→subcategories, `date`, `type` (expense/income/transfer), `amount` (NUMERIC 15,2), `currency`, `payment_method`, `merchant`, `description`, `deleted_at` | Rate columns pending: `bna_rate`, `mep_rate`, `ccl_rate`, `blue_rate`, `amount_usd` |

### Key relationships

- One `transaction_id` (UUID) groups N movements of a single financial event (e.g. USD purchase = 2 movements)
- `account_id = NULL` on `expense` and `income` movements — they don't touch a savings account
- `account_id NOT NULL` on `transfer` movements only
- **Account balance is always computed** — no `balance` column:
  ```sql
  SELECT SUM(amount) FROM movements WHERE account_id = $id AND deleted_at IS NULL
  ```

---

## 3. Feature Inventory

### ✅ Implemented

- Server bootstrap: Gin + GORM + Postgres + Goose migrations + webhook bot
- All 6 DB tables created and migrated
- Invitation system: `POST /invitations` → 6-char code + Telegram deep-link (72h, single-use)
- `/start` onboarding: validates invite code → creates user → creates default ARS + USD wallet accounts
- Onboarding: mandatory `initial_balance_setup` conversation flow, auto-started right after `/start` creates the wallets — asks ARS then USD balance (TextStep, accepts 0, rejects negative/non-numeric), confirms with a ChoiceStep (Confirmar/Corregir), and on completion inserts one opening `transfer` movement per currency (subcategory `Sistema | Saldo inicial`) in a single DB transaction via `movement.InsertBatch`
- Health endpoints: `GET /health/internal`, `HEAD /health/external`
- Conversation engine: `Flow`, `TextStep`, `ChoiceStep`, state persisted in Postgres JSONB
- Account repository: full CRUD + default management + unique violation detection
- Subcategory repository: `FindAllForUser`, `DistinctCategoriesForUser`, `FindByCategoryAndSubcategory`, `Insert` + unique violation detection
- ~90 global subcategories seeded across 15 categories (`migrations/20260625234857`)
- Movement GORM model + `InitRepository` + `InsertBatch` (transactional multi-row insert), `FindSimilarForUser` (pg_trgm fuzzy search), `SoftDeleteByIDs`/`ReplaceMovements` (ID-based, atomic DELETE+INSERT), `SumAmountForAccount`
- `handleConversationInput`'s flow-completion dispatch (`handleFlowFinished`, `switch result.FlowName`) — no longer a `"TO_REVIEW"` placeholder for registered flows
- **LLM orchestrator** (`internal/orchestrator/`) — Groq tool-calling HTTP client, 4 call shapes: `ClassifyIntent` (Call 1 router, small/fast model), `ClassifyCreate` (Call 2 CREATE, pro model), `ResolveUpdate` (Call 2 UPDATE, pro model), `ResolveDelete` (Call 2 DELETE, small model)
- **Movement recording (CREATE)** — frictionless by default: a confident Call 2 CREATE classification inserts immediately with a receipt card, no `conversation.Engine` involvement. A genuine gap (PENDING_REVIEW category, or a transfer naming a non-existent account) triggers a guided `ChoiceStep` flow (`movement_create` in `movement_create_flow.go`) to fill just that gap before inserting. Compound transactions (USD purchase, FCI subscription/redemption, credit-card itemization) share one `transaction_id`; FCI redemption gain is computed by app code (`SumAmountForAccount` + the destination account's `IsDefault` flag), never by the LLM
- **Movement correction (UPDATE)** and **deletion (DELETE)** — share one reference-resolution mechanism (`reference_resolution.go`): checks the user's `LastTransactionStore` first, falls back to a `pg_trgm` DB search anchored on a mentioned date (uncapped) or the last 7 days otherwise; 0 candidates errors out, 1 proceeds straight to confirm, 2+ shows a picker. UPDATE always requires explicit confirmation (`movement_update_pick` → `movement_update_confirm`, a two-hop flow chain since `conversation.Step`s can't make LLM calls — the second Call 2 UPDATE invocation happens in `proceedToUpdateConfirm`, skipped entirely when `LastTransactionStore` already resolved the match). DELETE is simpler — a single flow whose picker step is skippable straight to a confirm/summary gate once a candidate is known
- `Engine.StartWithData` + `Step.Skip`/`ChoiceStep.SkipIf`/`TextStep.SkipIf` — pre-seed a flow with already-known data (e.g. an LLM classification) and skip past resolved steps
- `subcategory.IconFor`/`CategoryIcon` — static category→emoji map for receipt cards, verified against the seeded taxonomy
- `pg_trgm` Postgres extension + GIN trigram indexes on `movements.description`/`merchant` (migration `20260702120000`)

### ❌ Not started

- QUERY intent execution — Call 1 routes to it, but the bot replies "no disponible todavía"; no DB query or Groq-formatted response yet
- Exchange rate fetching: daily BNA/MEP/CCL/blue from `dolarapi.com`, monthly IPC from `api.argentinadatos.com`
- Rate snapshot columns on `movements` table (`bna_rate`, `mep_rate`, `ccl_rate`, `blue_rate`, `amount_usd`)
- `payment_method` as an enum — stays free TEXT
- Message deduplication (no `processed_log` equivalent in v2 yet)
- Real admin auth — `middleware.RequireAdmin` is hardcoded to ID=1
- Admin `/new-invite` Telegram command
- Cron jobs: daily exchange rates, monthly IPC, weekly summary, daily reminder
- Web panel (tech TBD)
- Subcategory-creation conversation flow (pre-existing gap, unrelated to this feature)
- Generalizing the FCI compound-transaction pattern to other investment instruments (deliberately scoped to FCI only)

---

## 4. Key Design Decisions

- **No Telegram commands for end users.** All interaction is free text → LLM orchestrator → intent routing. The only exceptions are `/start` (onboarding deep-link, all users) and admin commands like `/new-invite`.
- **CREATE is frictionless; UPDATE/DELETE always confirm.** A confident CREATE classification inserts immediately with no `conversation.Engine` involvement — recording new movements should have zero friction. UPDATE and DELETE, by contrast, always stop for explicit user confirmation before mutating, because they touch data the user already trusts as recorded truth. Only a genuine CREATE gap (PENDING_REVIEW category, or a transfer naming a non-existent account) drops into a guided `ChoiceStep` flow.
- **Pre-seeded, gap-only conversation flows via `Engine.StartWithData` + `Step.Skip`.** A flow can start already carrying data (e.g. an LLM classification or a resolved reference-resolution candidate) and skip every step whose data is already known, re-evaluating skip conditions after each `Advance` rather than only once at start. This is the reusable mechanism behind CREATE's gap-fill, and behind `seedAndStartUpdateConfirm` skipping the picker step straight to confirm when `LastTransactionStore` already resolved the match.
- **Movement mutation operates on movement IDs, not `transaction_id`.** `movement.SoftDeleteByIDs`/`ReplaceMovements` take a list of primary-key IDs. This means a standalone/ungrouped movement (`transaction_id IS NULL`) is corrected or deleted through the exact same code path as a multi-row compound transaction — no special-casing for "is this movement part of a group."
- **FCI redemption gain is computed by app code, never the LLM.** The rule is `redeemed_amount >= balance_before` (via `movement.SumAmountForAccount` on the destination account) combined with the account's `IsDefault` flag to distinguish a full/partial redemption from a subscription — both look like an identical negative-transfer leg under the same subcategory, so `IsDefault` is the actual discriminator the LLM cannot see.
- **Reference resolution for UPDATE/DELETE: `lastTransaction` first, `pg_trgm` fallback.** Check the user's `LastTransactionStore` before hitting the DB. If it doesn't match, fall back to a `pg_trgm` similarity search on `description`/`merchant`, anchored on a mentioned date (uncapped) or the last 7 days otherwise. 0 candidates errors out, 1 proceeds to confirm, 2+ shows a picker.
- **Query responses = LLM-formatted.** For `query` intents: fetch structured data from DB → pass to Groq → Groq writes the human-readable Spanish response. *(QUERY execution itself is not yet implemented — Call 1 routes to it, but the bot currently replies "no disponible todavía.")*
- **ARS/USD strictly separated.** No implicit FX conversion anywhere. Totals and summaries are reported per currency.
- **Balance is always computed.** No `balance` column on `accounts`. Always `SUM(amount)` from `movements`.
- **Conversation state is DB-backed JSONB.** `conversation_states` table, one row per user. Do not move to in-memory without explicit discussion.
- **UPDATE = atomic DELETE + INSERT.** Editing a movement means soft-deleting the old one(s) and inserting the new one(s) in a single transaction. Never partial patch.
- **Taxonomy is closed.** LLM may only assign existing category/subcategory names. Unknown or low-confidence → `PENDING_REVIEW | PENDING_REVIEW`.
- **Mandatory onboarding steps piggyback on the one-flow-at-a-time rule.** There's no `users.onboarding_completed` flag. A step is "mandatory" simply because it's auto-started (`engine.Start`/`startFlowIfNotBusy`) right after the previous one finishes, and `Engine.InProgress` (backed by `conversation_states`) blocks any other flow from starting until it's done. If the user disappears mid-flow, state persists in Postgres and resumes on their next message — no extra bookkeeping needed. See `initial_balance_setup` in `controller/messaging/initial_balance_flow.go` for the reference implementation.

---

## 5. Anti-patterns — What NOT to do

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

---

## 6. Update Contract

**Before closing any session that adds or changes something, update this file.**

| If you added or changed... | Update |
|---------------------------|--------|
| A new package | Section 1 — Package Map |
| A new DB table or migration | Section 2 — Data Model |
| A feature completed, started, or descoped | Section 3 — Feature Inventory |
| A new architectural decision | Section 4 — Key Design Decisions |
| A new anti-pattern identified | Section 5 — Anti-patterns |
| A new recipe (flow type, step type, intent) | `CLAUDE.md` → Section 3 Recipes |
| A new business rule | `CLAUDE.md` → Section 4 Business Rules |
| A new dependency added to `go.mod` | `CLAUDE.md` → Section 1 Stack |
