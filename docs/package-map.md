# Package Map — lopii-finance-bot

> What each package owns. For the data schema see [data-model.md](data-model.md); for feature history see [features.md](features.md).

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
| `internal/logging/` | `logging.go` | Installs the process-wide `slog` logger. Its handler stamps the ctx's `trace_id` onto every record, so a log line can always be joined to its `request_traces` / `llm_calls` / `intent_events` rows |
| `internal/trace/` | `trace.go` | `NewID()` — 16 random bytes → 32 hex, via `crypto/rand` (deliberately no `google/uuid` dependency) + the ctx carrier (`WithID`/`FromContext`) for the correlation id |
| `internal/nudge/` | `repository.go` | `UserNudge` model + repository (`WasSent`, `MarkSent`, `LastSentAt`) — once-ever-per-(user,key) storage for contextual tips; the primary key doubles as the dedup guard. The tip definitions themselves live in `messaging/nudge.go` |
| `internal/summary/` | `summary.go`, `messages.go` | Weekly-summary builder + its Argentine-Spanish copy (kept in one file so copy edits touch one place). Declares consumer-local `MovementReader`/`AccountReader` interfaces per the repo convention; driven by `notifier`'s sweeper |
| `internal/user/` | `user.go` | `User` GORM model + repository (`FindByTelegramID`, `FindByID`, `Insert`) |
| `internal/invitation/` | `repository.go` | `Invitation` model + repository (`Create`, `FindByCode`, `MarkAsUsed`) |
| `internal/account/` | `repository.go`, `messages.go` | `Account` model + full repository (`Insert`, `FindByUserID`, `FindDefaultByCurrency`, `UnsetDefault`, `SoftDeleteByUserID`) + Spanish UI strings |
| `internal/subcategory/` | `repository.go`, `messages.go`, `icons.go`, `cache.go` | `Subcategory` model (incl. `Icon string`, DB-backed) + repository (`FindAll` — every row, global and user-created) + Spanish UI strings (exported: `Msg*`/`Btn*`) + `ValidIcon` (minimal emoji-input check) + `Cache` (in-memory, loaded once at server startup via `NewCache`, refreshed via `Reload()` after a user-created Insert; splits `global` (shared ~90-row seeded taxonomy) from `perUser map[uint64][]Subcategory` so custom rows never duplicate the global set; `FindByCategoryAndSubcategory(userID, ...)` and `IconForCategory(userID, ...)` check the user's own rows before falling back to global) |
| `internal/movement/` | `repository.go`, `messages.go` | `Movement` model (incl. `Subcategory *subcategory.Subcategory` GORM association, `foreignKey:SubcategoryID`) + `AccountOpening` struct + repository (`InsertAccountsWithOpenings` (atomic cross-repo account+movement insert for onboarding), `InsertBatch`, `FindSimilarForUser` (pg_trgm fuzzy search, `until *time.Time` optional upper date bound, `Preload("Subcategory")`), `SoftDeleteByIDs`, `SoftDeleteByUserID`, `ReplaceMovements`, `SumAmountForAccount`, `SumForUser`/`ListForUser` (read-only QUERY aggregates + `MovementQuery`/`CategorySum`, abs amounts, transfer excluded by default)) + `IconForType`/`TypeFromString` helpers |
| `internal/metric/` | `intent_event.go` | `IntentEvent` model + repository (`Log`, `Resolve`) — métricas de asertividad LLM end-to-end |
| `internal/queryhistory/` | `repository.go` | `QueryTurn` model + `Turn` read shape + repository (`Append` — insert + prune older-than-TTL; `Recent` — last N turns within TTL, chronological). Ephemeral by design (hard-pruned, no `deleted_at`). TTL/limit injected from config. Backs the QUERY conversation thread |
| `internal/reminder/` | `repository.go`, `messages.go` | `Reminder` model (one row per user; `WindowStartMin`/`WindowEndMin` minutes-since-midnight ART, `MidpointMin()` derived fire target) + repository (`Upsert`, `Disable`, `FindByUserID`, `ListDue`, `SetLastRemindedOn`) + rotating Spanish message pool (`PickMessage`) |
| `internal/notifier/` | `sweeper.go` | `Sweeper` — in-process `time.Ticker` goroutine driving every scheduled system→user notification. Today hosts one tenant (`sweepReminders`); `send` is an injected field (reachable by tests and any future admin broadcast) |
| `internal/orchestrator/` | `orchestrator.go`, `client.go`, `client_loop.go`, `recorder.go`, `query.go`, `types.go`, `router.go`, `create.go`, `update.go`, `delete.go`, `onboarding.go` | Groq tool-calling HTTP client (plain `net/http`, no SDK) — the single chokepoint `Client.send` (`client.go`) retries transient failures (network/429/5xx, exponential backoff) and returns `*RateLimitedError{RetryAfter}` (via `errors.As`) when the last attempt was a terminal 429, wait priority `Retry-After` header → `x-ratelimit-reset-tokens` header (`recorder.go`'s `parseResetTokens`, TPM/TPD) → body `"try again in <dur>"` (`parseBodyRetryAfter`) — `ClassifyIntent` (Call 1 router: returns `IntentResult{Intent, NeedsConfirmation}` — CREATE/UPDATE/DELETE/QUERY/ACCOUNT_CREATE/CREATE_CATEGORY plus an ambiguity flag only meaningful for CREATE), `ClassifyCreate` (Call 2 CREATE: builds 1..N movement drafts from free text + taxonomy + accounts), `ResolveUpdate` (Call 2 UPDATE: matches a candidate transaction + correction, returns corrected movement set), `ResolveDelete` (Call 2 DELETE: confirms which candidate a delete means, never rebuilds rows), `ClassifyOnboarding` (Call ONBOARDING: parses N accounts from free text, monetary balances only), `AnswerQuery` (read-only QUERY agent loop — `client_loop.go`'s `tool_choice:"auto"` multi-tool loop over caller-provided `AgentTool`s + `execute`, dependency-free, `queryModel`, cap 3 + forced `"none"` final narration) — `UpdateResult`/`DeleteResult` carry `MentionedDateFrom`/`MentionedDateTo` (a single mentioned date sets only `MentionedDateFrom`; an explicit range sets both) |
| `internal/pendingjob/` | `pending_job.go`, `repository.go` | `PendingJob` model (`UserID`, `Kind`, `Payload` opaque JSON, `CreatedAt`) + repository (`Insert`, `ListByUserOrdered` (FIFO per user), `ListPendingUserIDs`, `Delete`, `CountByUser`) backing table `pending_llm_jobs` — durable cache for a user message that hit a terminal Groq 429; consumed by `internal/controller/messaging`'s drain worker, never read for feature logic elsewhere |
| `internal/conversation/` | `flow.go`, `engine.go`, `repository.go`, `text_step.go`, `choice_step.go` | Conversation state-machine framework; `Step.Skip`/`ChoiceStep.SkipIf`/`TextStep.SkipIf` (opt-in, re-evaluated after every `Advance`) + `Engine.StartWithData` (start a flow pre-seeded with data, skipping already-resolved steps) + `Engine`'s resume gate (idle timeout + 2-strikes consecutive-retry escalation, centralized in `Handle` before any step dispatch — `NewEngine(store, resumeLabel)` takes an injected `func(flowName string) string` for the per-flow "¿retomamos?" copy, since this package cannot import `messaging`) |
| `internal/controller/health/` | `controller.go` | `GET /health/internal`, `HEAD /health/external` |
| `internal/controller/invitation/` | `controller.go` | `POST /invitations` (admin-only) |
| `internal/controller/admin/` | `controller.go` | `POST /admin/users/:telegramID/reset` (admin-only) — soft-deletes user's accounts+movements, clears flow state, re-fires onboarding |
| `internal/controller/messaging/` | ~35 files, one per flow (`*_flow.go` builds the graph, `*_finish.go` runs the terminal action) | Telegram handlers: `/start` + catch-all for free text and callbacks. **For the file/symbol inventory use `codegraph_explore` — it returns the current source.** The non-obvious parts are below. |
| `internal/controller/miniapp/` | `controller.go`, `auth.go`, `static.go`, `overview.go`, `accounts.go`, `categories.go`, `period.go`, `evolution.go`, `templates/` | Telegram Mini App: server-rendered HTML views (templ) over the same Go service — no React, no JSON API. `auth.go` validates Telegram `initData` (HMAC over the bot token) on every request; that check is the only thing standing between a URL and another user's finances |
| `migrations/` | `*.sql` | Goose migrations — **authoritative DB schema**; includes `pg_trgm` extension + GIN trigram indexes on `movements.description`/`merchant` (`20260702120000`) |
| `config/` | `local.toml`, `dev.toml`, `prd.toml` | Environment configs (secrets go here, git-ignored for local); `[groq]` section — per-call-type model name, API key, base URL, timeout |

### `messaging` — lo no obvio

Lo que no se deduce leyendo los archivos. La estructura sí se deduce: usá `codegraph_explore`.

- **Un solo punto de entrada.** `handleFreeText` corre Call 1 (router) y despacha a un
  `start*` por intent. QUERY es la excepción: no abre flow, va al agent loop de `query.go`
  (tools tipadas + `execute` scopeado al usuario, invariantes en Go, nunca en el prompt).
- **Una sola resolución de referencias.** `resolveCandidates` (`reference_resolution.go`) es
  el único mecanismo de búsqueda de candidatos, compartido por UPDATE y DELETE. No agregar
  un segundo — la relevancia textual se decide en proceso (`matchesMessage`), no en la DB.
- **La cola de 429 tiene un invariante de orden.** `enqueueBehindPending` se llama **solo
  desde `handleConversationInput`** (el borde del webhook), nunca desde `handleFreeText`.
  Si se llamara desde ahí, el replay del drain se re-encolaría a sí mismo. El flag de ctx
  `isReplaying`/`withReplaying` es lo que distingue una llamada del webhook de un replay.
- **El drain reusa el handler real.** `replayJob` reproduce el mensaje por el mismo handler
  que hubiera usado el webhook, no por una copia. Pasado `maxJobAge` borra y avisa, en vez
  de reintentar para siempre.
- **REMINDER_SET no usa Call 2.** Es un `ChoiceStep` determinista (presets + ventana custom
  en horas enteras). Borrar == deshabilitar, sin gate de confirmación.
- **La descripción de una subcategoría no es decorativa.** Alimenta
  `orchestrator.TaxonomyEntry.Description`, o sea la clasificación CREATE futura. Por eso el
  paso de descripción es obligatorio al crear una categoría.

### Do NOT read

- `app_scripts_v1/` — v1 in Google Sheets + Apps Script. **Historical reference only.** No patterns, types, or logic from here apply to Go v2. Git-ignored: present on disk for local reference, not tracked, not in fresh clones.
- `docs/superpowers/` — spec/plan scratch from past sessions. Git-ignored: present on disk locally, not tracked, not in fresh clones. The curated "why" that survives lives in `docs/decisions.md`/`docs/features.md`; a spec/plan link in those files may 404 on a fresh clone.
