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
| `internal/subcategory/` | `repository.go`, `messages.go`, `icons.go`, `cache.go` | `Subcategory` model (incl. `Icon string`, DB-backed) + repository (`FindAll` — every row, global and user-created) + Spanish UI strings (exported: `Msg*`/`Btn*`) + `ValidIcon` (minimal emoji-input check) + `Cache` (in-memory, loaded once at server startup via `NewCache`, refreshed via `Reload()` after a user-created Insert; splits `global` (shared ~90-row seeded taxonomy) from `perUser map[uint64][]Subcategory` so custom rows never duplicate the global set; `FindByCategoryAndSubcategory(userID, ...)` and `IconForCategory(userID, ...)` check the user's own rows before falling back to global) |
| `internal/movement/` | `repository.go`, `messages.go` | `Movement` model (incl. `Subcategory *subcategory.Subcategory` GORM association, `foreignKey:SubcategoryID`) + repository (`InsertBatch`, `FindSimilarForUser` (pg_trgm fuzzy search, `until *time.Time` optional upper date bound, `Preload("Subcategory")`), `SoftDeleteByIDs`, `ReplaceMovements`, `SumAmountForAccount`) + `IconForType`/`TypeFromString` helpers |
| `internal/metric/` | `intent_event.go` | `IntentEvent` model + repository (`Log`, `Resolve`) — métricas de asertividad LLM end-to-end |
| `internal/orchestrator/` | `orchestrator.go`, `client.go`, `types.go`, `router.go`, `create.go`, `update.go`, `delete.go` | Groq tool-calling HTTP client (plain `net/http`, no SDK) — `ClassifyIntent` (Call 1 router: returns `IntentResult{Intent, NeedsConfirmation}` — CREATE/UPDATE/DELETE/QUERY/ACCOUNT_CREATE plus an ambiguity flag only meaningful for CREATE), `ClassifyCreate` (Call 2 CREATE: builds 1..N movement drafts from free text + taxonomy + accounts), `ResolveUpdate` (Call 2 UPDATE: matches a candidate transaction + correction, returns corrected movement set), `ResolveDelete` (Call 2 DELETE: confirms which candidate a delete means, never rebuilds rows) — `UpdateResult`/`DeleteResult` carry `MentionedDateFrom`/`MentionedDateTo` (a single mentioned date sets only `MentionedDateFrom`; an explicit range sets both) |
| `internal/conversation/` | `flow.go`, `engine.go`, `repository.go`, `text_step.go`, `choice_step.go` | Conversation state-machine framework; `Step.Skip`/`ChoiceStep.SkipIf`/`TextStep.SkipIf` (opt-in, re-evaluated after every `Advance`) + `Engine.StartWithData` (start a flow pre-seeded with data, skipping already-resolved steps) + `Engine`'s resume gate (idle timeout + 2-strikes consecutive-retry escalation, centralized in `Handle` before any step dispatch — `NewEngine(store, resumeLabel)` takes an injected `func(flowName string) string` for the per-flow "¿retomamos?" copy, since this package cannot import `messaging`) |
| `internal/controller/health/` | `controller.go` | `GET /health/internal`, `HEAD /health/external` |
| `internal/controller/invitation/` | `controller.go` | `POST /invitations` (admin-only) |
| `internal/controller/messaging/` | `controller.go`, `start.go`, `messages.go`, `initial_balance_flow.go`, `free_text.go`, `movement_flow.go`, `movement_create_flow.go`, `movement_confirm_flow.go`, `movement_update_flow.go`, `movement_delete_flow.go`, `reference_resolution.go`, `account_create_flow.go`, `account_create_finish.go`, `subcategory_setup_flow.go`, `subcategory_setup_finish.go` | Telegram handlers: `/start` + catch-all for free text and callbacks; onboarding's mandatory `initial_balance_setup` flow; `free_text.go` routes every free-text message through Call 1 into CREATE/UPDATE/DELETE/ACCOUNT_CREATE/CREATE_CATEGORY (QUERY replies "not supported yet"); `movement_create_flow.go` (frictionless insert or gap-fill `ChoiceStep`s, each with a Cancelar bail-out), `movement_confirm_flow.go` (the `movement_confirm_intent` gate — reescribir/cancelar only, no reroute — shown for an ambiguous CREATE), `movement_update_flow.go` + `movement_delete_flow.go` (two-hop pick→confirm flow chains), `reference_resolution.go` (`resolveCandidates` — the single `pg_trgm` candidate-lookup mechanism shared by UPDATE, DELETE, and CREATE's duplicate-check); `account_create_flow.go` + `account_create_finish.go` (ACCOUNT_CREATE intent — create an additional, purpose-specific account: investment, retirement, savings, etc. — cancelable/back-able at every step, never IsDefault); `subcategory_setup_flow.go` + `subcategory_setup_finish.go` (CREATE_CATEGORY intent — create a custom category/subcategory pair, in an existing category or a brand-new one, including a required description step that feeds `orchestrator.TaxonomyEntry.Description` for future CREATE classification — cancelable/back-able at every step, reuses `TextStep.EscapeOptions`/`OnEscape` and `account_create_flow.go`'s `onAccountCreateEscape` rather than introducing a new mechanism) |
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
| `accounts` | `id`, `user_id`→users, `name`, `type` (legacy), `currency`, `is_default`, `deleted_at` | `type` column is a legacy artifact — needs a drop migration; unique index is case-insensitive on name (migration 20260704120000) |
| `conversation_states` | `user_id` (PK)→users, `flow_name`, `step_name`, `data` (JSONB), `updated_at` | One row per user; never access directly — use `conversation.Engine`; `updated_at` is now read (not just written) by `Engine`'s idle-timeout resume gate |
| `subcategories` | `id`, `user_id` (nullable)→users, `category`, `subcategory`, `description`, `is_global`, `icon`, `deleted_at` | `user_id = NULL` = global (visible to all users); `icon` backfilled for the seeded taxonomy in migration `20260705120000` |
| `movements` | `id`, `transaction_id` (UUID nullable), `user_id`→users, `account_id` (nullable)→accounts, `subcategory_id`→subcategories, `date`, `type` (expense/income/transfer), `amount` (NUMERIC 15,2), `currency`, `payment_method`, `merchant`, `description`, `deleted_at` | Rate columns pending: `bna_rate`, `mep_rate`, `ccl_rate`, `blue_rate`, `amount_usd` |
| `intent_events` | `id`, `created_at`, `user_id`→users, `raw_message`, `intent`, `needs_confirmation`, `outcome`, `resolved_at`, `was_correct` | Correlación por "último pending" vía WIP=1; `was_correct` etiquetado a mano |

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
- **Movement recording (CREATE)** — frictionless by default: a confident Call 2 CREATE classification inserts immediately with a receipt card, no `conversation.Engine` involvement. A genuine gap (PENDING_REVIEW category, or a transfer naming a non-existent account) triggers a guided `ChoiceStep` flow (`movement_create` in `movement_create_flow.go`, every step with a Cancelar bail-out) to fill just that gap before inserting. Compound transactions (USD purchase, FCI subscription/redemption, credit-card itemization) share one `transaction_id`; FCI redemption gain is computed by app code (`SumAmountForAccount` + the destination account's `IsDefault` flag), never by the LLM
- **CREATE confirm gate for ambiguous messages** (`movement_confirm_flow.go`) — before ever running Call 2 CREATE, `startMovementCreate` trusts the router's `needs_confirmation` flag alone (Call 1 judged the message too short/ambiguous to trust — e.g. a bare amount with no verb/item/merchant). `true` routes to `movement_confirm_intent` — a single `ChoiceStep` with exactly two options, "✍️ Reescribir mensaje" and "🚫 Cancelar". Doesn't reroute to UPDATE/DELETE and doesn't touch the DB — the bot never guesses the user's real intent, it just asks them to be clearer. (Previously also ran `resolveCandidates` as a second signal even when the router said `false`; dropped — it produced false positives whenever a new CREATE happened to share an amount/description with an unrelated recent movement, second-guessing a router verdict that was already correct)
- **Movement correction (UPDATE)** and **deletion (DELETE)** — share one reference-resolution mechanism (`reference_resolution.go`, `resolveCandidates`): a `pg_trgm` DB search anchored on a mentioned date (uncapped) or the last 7 days otherwise; 0 candidates errors out, 1 proceeds straight to confirm, 2+ shows a picker. UPDATE always requires explicit confirmation (`movement_update_pick` → `movement_update_confirm`, a two-hop flow chain since `conversation.Step`s can't make LLM calls — the second Call 2 UPDATE invocation happens in `proceedToUpdateConfirm`). DELETE is simpler — a single flow whose picker step is skippable straight to a confirm/summary gate once a candidate is known
- `Engine.StartWithData` + `Step.Skip`/`ChoiceStep.SkipIf`/`TextStep.SkipIf` — pre-seed a flow with already-known data (e.g. an LLM classification) and skip past resolved steps
- `pg_trgm` Postgres extension + GIN trigram indexes on `movements.description`/`merchant` (migration `20260702120000`)
- **Movement classification follow-up improvements** (`docs/superpowers/specs/2026-07-04-movement-classification-improvements-design.md`) — 4 shipped:
  - `Movement.Subcategory *subcategory.Subcategory` GORM association (`foreignKey:SubcategoryID`), populated at construction time in `resolveAndInsertMovements`/`fciRedemptionGain`, or via `.Preload("Subcategory")` in `FindSimilarForUser`. Retired the old manual `subcategory_id → name` lookup (`buildSubcategoryIndex` + full `FindAllForUser` fetch + map) that `movementToRow` used to do per call.
  - Confirmation/receipt messages (`msgConfirmMovements`/`movementReceiptLine`, `msgConfirmUpdateDiff`, `msgConfirmDelete` in `internal/controller/messaging/messages.go`) now show category, subcategory, description, and date — not just icon/amount/currency/description.
  - UPDATE/DELETE reference resolution supports a date **range**, not just a single anchor date: `resolveCandidates(userID, message, dateFrom, dateTo)`, `FindSimilarForUser(..., until *time.Time)`. A range ("entre el 27 y el 29") bounds the search on both ends; a single date behaves as before (anchors `since`, no upper cap); neither mentioned lifts the default 7-day cap.
  - Telegram inline-keyboard buttons render as a multi-row grid (`chunkButtons`, `buttonsPerRow = 2` in `internal/controller/messaging/controller.go`) instead of one cramped row — fixes unreadably small category/subcategory/account buttons.
  - `subcategory.Cache` (`internal/subcategory/cache.go`) — in-memory, loaded once at server startup via `NewCache(subcategoryRepo)`, wired into `server.go` in place of the raw repository. No call site outside `server.go` changed signature or behavior.
- **Account creation (ACCOUNT_CREATE)** — Call 1 router recognizes an explicit request to create an account (no movement involved) and starts a dedicated `account_create` flow: name → currency → initial balance → confirm, Cancelar/Atrás at every step. On confirm, creates the `account.Account` (never `IsDefault`) and inserts one opening `transfer` movement (subcategory `Sistema | Saldo inicial`), mirroring the onboarding `initial_balance_setup` flow. The account name uniqueness guard (`accounts_user_name_currency_ci_idx`, migration `20260704120000`) is now case-insensitive, hardening this flow and the pre-existing organic account-creation gap-fill in `movement_create_flow.go` alike.
- **Subcategory creation (CREATE_CATEGORY)** — a 5th router intent starts `subcategory_setup`: choose an existing category or name a new one (+ icon), name the subcategory, write a short required description of when it applies, confirm. Cancelar/Atrás at every step via the same `TextStep.EscapeOptions`/`OnEscape` mechanism `account_create_flow.go` already established — no framework change needed. On confirm, inserts the user-scoped `subcategory.Subcategory` row and calls `Cache.Reload()` so it's immediately usable. The description step is the point of the feature, not decoration: it's written straight into `Subcategory.Description`, which is exactly the field `orchestrator.TaxonomyEntry.Description` feeds to Call 2 CREATE as a classification hint — a subcategory with no description is invisible to future auto-classification.
- **DB-backed subcategory icons** — `subcategory.Subcategory.Icon` (column `icon`, migration `20260705120000`, backfilled for all 16 seeded categories from the old static map) replaces the deleted `subcategory.CategoryIcon`/`IconFor`. Every call site now reads `.Icon` off a real row (via `Movement.Subcategory`, `movementToRow`, or a `Cache.FindByCategoryAndSubcategory`/`IconForCategory` lookup) instead of a category-name-keyed static map — required so a user-created category can carry its own icon.
- **`subcategory.Cache` per-user awareness** — `Cache` now splits `global` (the shared seeded taxonomy, one backing slice for every user) from `perUser map[uint64][]Subcategory` (each user's own custom rows). `FindByCategoryAndSubcategory`/`IconForCategory` both take a `userID` and check the user's own rows before falling back to global — needed once subcategory creation could produce two different users' same-named custom category without collision (the DB's own unique index is already scoped by `user_id`).
- **`conversation.Engine` resume gate** — centralized in `Handle`, before any step-specific logic: an idle timeout (10 min since `conversation_states.updated_at`) or a second consecutive step-validation `Retry` shows a generic "¿retomamos o cancelamos?" gate (two reserved `CallbackData` sentinels, `_resume_continue`/`_resume_cancel`) instead of the step's own raw error or a silently stuck conversation. Applies to every registered flow for free — no per-flow code needed. The cancel path sets a dedicated `Result.Data["_resume_cancelled"]` marker, checked once at the top of `handleFlowFinished`, deliberately separate from each flow's own Cancelar convention (there was no single existing convention across the 7 flows to retrofit).
- **Métricas de asertividad LLM (end-to-end)** — tabla `intent_events`, package `internal/metric`. El router loguea un pending por mensaje de movimiento (terminal directo para QUERY/ACCOUNT_CREATE/CREATE_CATEGORY); cada terminal de flow resuelve el outcome vía el último pending del usuario (correlación por WIP=1). Fire-and-forget. Sin fricción nueva; `confidence` autoreportada y confirmación en CREATE frictionless descartadas a propósito.

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
- Generalizing the FCI compound-transaction pattern to other investment instruments (deliberately scoped to FCI only)

---

## 4. Key Design Decisions

- **No Telegram commands for end users.** All interaction is free text → LLM orchestrator → intent routing. The only exceptions are `/start` (onboarding deep-link, all users) and admin commands like `/new-invite`.
- **CREATE is frictionless; UPDATE/DELETE always confirm.** A confident CREATE classification inserts immediately with no `conversation.Engine` involvement — recording new movements should have zero friction. UPDATE and DELETE, by contrast, always stop for explicit user confirmation before mutating, because they touch data the user already trusts as recorded truth. The one exception: a CREATE the router flags as ambiguous, or one `resolveCandidates` finds a plausible existing match for, stops at the `movement_confirm_intent` gate (reescribir/cancelar) before Call 2 CREATE ever runs. A genuine CREATE gap (PENDING_REVIEW category, or a transfer naming a non-existent account) drops into a guided `ChoiceStep` flow.
- **The bot never guesses the user's real intent — only the user resolves ambiguity.** When a CREATE is ambiguous, the confirm gate offers exactly "reescribir" or "cancelar", never a reroute to UPDATE/DELETE. Even if the message probably meant a correction, the bot doesn't act on that guess; the user is expected to send a clearer message. Same principle behind the gap-fill flow's Cancelar option — an escape hatch, not a smarter flow.
- **Pre-seeded, gap-only conversation flows via `Engine.StartWithData` + `Step.Skip`.** A flow can start already carrying data (e.g. an LLM classification or a resolved reference-resolution candidate) and skip every step whose data is already known, re-evaluating skip conditions after each `Advance` rather than only once at start. This is the reusable mechanism behind CREATE's gap-fill.
- **Movement mutation operates on movement IDs, not `transaction_id`.** `movement.SoftDeleteByIDs`/`ReplaceMovements` take a list of primary-key IDs. This means a standalone/ungrouped movement (`transaction_id IS NULL`) is corrected or deleted through the exact same code path as a multi-row compound transaction — no special-casing for "is this movement part of a group."
- **FCI redemption gain is computed by app code, never the LLM.** The rule is `redeemed_amount >= balance_before` (via `movement.SumAmountForAccount` on the destination account) combined with the account's `IsDefault` flag to distinguish a full/partial redemption from a subscription — both look like an identical negative-transfer leg under the same subcategory, so `IsDefault` is the actual discriminator the LLM cannot see.
- **One reference-resolution mechanism, `resolveCandidates`, used everywhere.** UPDATE, DELETE, and CREATE's duplicate-check all go through the same `pg_trgm` similarity search on `description`/`merchant`, anchored on a mentioned date (uncapped) or the last 7 days otherwise. 0 candidates errors out (or, for CREATE, just proceeds — no duplicate found), 1 proceeds to confirm, 2+ shows a picker. There used to be a second mechanism (`LastTransactionStore`, an in-memory per-user "last transaction" checked before the DB search) — removed because two overlapping "what does this refer to" mechanisms was a source of silent wrong matches, not a performance win worth keeping.
- **`Movement.Subcategory` is the one exception to "bare FK, manual lookup."** Every other FK in the codebase (`Account.UserID`, `Invitation.CreatedBy/UsedBy`, `Subcategory.UserID`, `Movement.UserID`/`AccountID`) is a bare `uint64`/`*uint64` with manual repository lookups, even though a real Postgres FK backs every one. `Movement.Subcategory *subcategory.Subcategory` breaks that pattern deliberately: the FK already existed (no migration needed, Go-level-only change), and `Movement` is the one entity with a growing reporting/analytics surface (monthly summaries today, `QUERY` intent tomorrow) where list-shaped queries with names attached recur — every future method benefits from `.Preload("Subcategory")` instead of re-implementing `buildSubcategoryIndex`-style plumbing. The other four models are single-row lookups by ID with no comparable multiplying need.
- **Query responses = LLM-formatted.** For `query` intents: fetch structured data from DB → pass to Groq → Groq writes the human-readable Spanish response. *(QUERY execution itself is not yet implemented — Call 1 routes to it, but the bot currently replies "no disponible todavía.")*
- **ARS/USD strictly separated.** No implicit FX conversion anywhere. Totals and summaries are reported per currency.
- **Balance is always computed.** No `balance` column on `accounts`. Always `SUM(amount)` from `movements`.
- **Conversation state is DB-backed JSONB.** `conversation_states` table, one row per user. Do not move to in-memory without explicit discussion.
- **UPDATE = atomic DELETE + INSERT.** Editing a movement means soft-deleting the old one(s) and inserting the new one(s) in a single transaction. Never partial patch.
- **Taxonomy is closed.** LLM may only assign existing category/subcategory names. Unknown or low-confidence → `PENDING_REVIEW | PENDING_REVIEW`.
- **Mandatory onboarding steps piggyback on the one-flow-at-a-time rule.** There's no `users.onboarding_completed` flag. A step is "mandatory" simply because it's auto-started (`engine.Start`/`startFlowIfNotBusy`) right after the previous one finishes, and `Engine.InProgress` (backed by `conversation_states`) blocks any other flow from starting until it's done. If the user disappears mid-flow, state persists in Postgres and resumes on their next message — no extra bookkeeping needed. See `initial_balance_setup` in `controller/messaging/initial_balance_flow.go` for the reference implementation.
- **Repo-specific subagents over generic ones for locate/edit/review.** `.claude/agents/repo-investigator`, `repo-builder`, `repo-reviewer` mirror the caveman plugin's `cavecrew-*` pattern (narrow tool grants, hard scope refusal, compressed output) but carry this repo's own rules — money type, currency handling, `conversation_states` access, closed taxonomy — baked into their prompts. Use them instead of generic `Explore`/`general-purpose`/`code-reviewer` agents for work scoped to this repo. `repo-builder` enforces the completion-evidence rule (`CLAUDE.md` §7) at the agent level: it will not return a receipt without a clean `go build ./...`.
- **`subcategory.Cache` splits global/perUser instead of copying global rows per user.** The seeded taxonomy is ~90 rows shared by every user; once users can create their own subcategories, duplicating those 90 rows into a per-user slice would waste memory and (worse) require re-syncing every user's copy whenever a global row changes. `global []Subcategory` stays one shared backing slice; `perUser map[uint64][]Subcategory` holds only what each user actually created — usually a handful.
- **The resume gate is `conversation.Engine`-level, not per-flow.** Every registered flow gets idle-timeout/retry-escalation handling for free, applied centrally in `Handle` before any step-specific `Process` runs — adding it to each of the 7 flows individually would mean 7 near-identical implementations and 7 places to forget it on flow #8. The one cost is that `conversation` can't know the actual Spanish copy per flow (it doesn't import `messaging`), so `NewEngine` takes an injected `resumeLabel func(flowName string) string` instead.
- **Icons moved from a static Go map to a DB column.** `subcategory.CategoryIcon`/`IconFor` was fine while only the admin-seeded taxonomy existed ("update the map by hand when a new category appears"); a user creating their own category needs to choose its icon at creation time, which a compiled-in map structurally cannot hold. The DB row is now the only source of truth; every icon lookup goes through a real `Subcategory.Icon` field or a `Cache` method, never a category-name-keyed static map.
- **Métricas end-to-end por outcome, no por confidence autoreportada ni confirmación forzada.** Se mide '¿el usuario logró lo que quería?' con la señal que los gates confirm/cancel existentes ya producen + el outcome del terminal de cada flow, correlacionado por la invariante WIP=1 (último pending). No se agregó `confidence` (cambio de prompt en la ruta caliente + autoreporte débil) ni confirmación a CREATE frictionless (rompe el principio 'CREATE sin fricción'); el feedback de CREATE frictionless se infiere del follow-up UPDATE/DELETE, que ya se loguea.

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
