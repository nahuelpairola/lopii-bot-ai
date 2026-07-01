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
| `internal/subcategory/` | `repository.go`, `messages.go` | `Subcategory` model + repository + Spanish UI strings |
| `internal/movement/` | `repository.go` | `Movement` model + repository stub (no query methods yet) |
| `internal/conversation/` | `flow.go`, `engine.go`, `repository.go`, `text_step.go`, `choice_step.go` | Conversation state-machine framework |
| `internal/controller/health/` | `controller.go` | `GET /health/internal`, `HEAD /health/external` |
| `internal/controller/invitation/` | `controller.go` | `POST /invitations` (admin-only) |
| `internal/controller/messaging/` | `controller.go`, `start.go`, `messages.go` | Telegram handlers: `/start` + catch-all for free text and callbacks |
| `migrations/` | `*.sql` | Goose migrations — **authoritative DB schema** |
| `config/` | `local.toml`, `dev.toml`, `prd.toml` | Environment configs (secrets go here, git-ignored for local) |

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
- Health endpoints: `GET /health/internal`, `HEAD /health/external`
- Conversation engine: `Flow`, `TextStep`, `ChoiceStep`, state persisted in Postgres JSONB
- Account repository: full CRUD + default management + unique violation detection
- Subcategory repository: `FindAllForUser`, `DistinctCategoriesForUser`, `Insert` + unique violation detection
- ~90 global subcategories seeded across 15 categories (`migrations/20260625234857`)
- Movement GORM model + `InitRepository` (model only — no query methods)

### 🚧 In progress

- `internal/movement/repository.go` — model exists, query methods pending: `Insert`, `FindByUser`, `FindByDateRange`, `SoftDelete`  
  *(branch: `feat/add-movements`)*

### ❌ Not started

- LLM orchestrator (`internal/orchestrator/`) — Groq client + intent classification (record / query / update / delete / unclear)
- Movement recording conversation flow (`movement.NewRecordFlow`) — guided TextStep/ChoiceStep flow
- Query handler — LLM orchestrator → DB query → Groq formats human-readable Spanish response
- Exchange rate fetching: daily BNA/MEP/CCL/blue from `dolarapi.com`, monthly IPC from `api.argentinadatos.com`
- Rate snapshot columns on `movements` table (`bna_rate`, `mep_rate`, `ccl_rate`, `blue_rate`, `amount_usd`)
- Message deduplication (no `processed_log` equivalent in v2 yet)
- Real admin auth — `middleware.RequireAdmin` is hardcoded to ID=1
- Admin `/new-invite` Telegram command
- Cron jobs: daily exchange rates, monthly IPC, weekly summary, daily reminder
- Web panel (tech TBD)

---

## 4. Key Design Decisions

- **No Telegram commands for end users.** All interaction is free text → LLM orchestrator → intent routing. The only exceptions are `/start` (onboarding deep-link, all users) and admin commands like `/new-invite`.
- **Expense recording = guided conversation flow.** The LLM orchestrator triggers `engine.Start(userID, "record_expense")`. The bot walks the user through amount → type → category → subcategory → confirm using `TextStep`/`ChoiceStep` with inline keyboards. No single-shot LLM parsing for recording.
- **Query responses = LLM-formatted.** For `query` intents: fetch structured data from DB → pass to Groq → Groq writes the human-readable Spanish response.
- **ARS/USD strictly separated.** No implicit FX conversion anywhere. Totals and summaries are reported per currency.
- **Balance is always computed.** No `balance` column on `accounts`. Always `SUM(amount)` from `movements`.
- **Conversation state is DB-backed JSONB.** `conversation_states` table, one row per user. Do not move to in-memory without explicit discussion.
- **UPDATE = atomic DELETE + INSERT.** Editing a movement means soft-deleting the old one and inserting a new one in a single transaction. Never partial patch.
- **Taxonomy is closed.** LLM may only assign existing category/subcategory names. Unknown or low-confidence → `PENDING_REVIEW | PENDING_REVIEW`.

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
