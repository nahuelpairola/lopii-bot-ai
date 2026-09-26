# Data Model — lopii-finance-bot

> Tables and relationships. Schema authority is the Goose migrations in `migrations/`; this is
> the map, plus the notes a schema dump can't carry.

## Domain tables

| Table | Key Fields | Notes |
|-------|-----------|-------|
| `users` | `id`, `username`, `is_admin`, `deleted_at` | Soft delete. `is_admin` is **unused**: nothing writes or reads it (the DB default fills it) and its `DROP COLUMN` is pending — safe only once that code is deployed. Admin auth must not be built on it |
| `user_channels` | `id`, `user_id`→users, `channel`, `channel_user_id`, UNIQUE (`channel`, `channel_user_id`) | Replaced `users.telegram_id` in `20260822120000`: a user is found by channel identity, never by a column on `users` |
| `invitations` | `id`, `code` (6-char UNIQUE), `created_by`→users, `used_by`→users, `expires_at`, `used_at` | 72h expiry, single-use. State (pending/used/expired) is derived from `used_at`/`expires_at`, never stored — both the redemption check in `handleStart` and the admin view derive it independently |
| `accounts` | `id`, `user_id`→users, `name`, `type` (legacy), `currency`, `is_default`, `deleted_at` | `type` is **unused**: the Go model no longer maps it, the DB default (`'standard'`) fills it, and its `DROP COLUMN` is pending — safe only once that code is deployed. Unique index is case-insensitive on name (`20260704120000`) |
| `subcategories` | `id`, `user_id` (nullable)→users, `category`, `subcategory`, `description`, `is_global`, `icon`, `deleted_at` | `user_id = NULL` = global (visible to all). `description` is **not decorative** — it feeds LLM classification. Icons backfilled in `20260705120000` |
| `movements` | `id`, `transaction_id` (UUID nullable), `user_id`→users, `account_id` (NOT NULL)→accounts, `subcategory_id`→subcategories, `date`, `type`, `amount` (NUMERIC 15,2), `currency`, `description`, `deleted_at` | `date` is a plain `DATE` — see the binding trap in `internal/movement/AGENTS.md`. `account_id` is NOT NULL since `20260926130000`: the guard resolves it, the constraint catches a write path that skipped the guard. Rate columns pending: `bna_rate`, `mep_rate`, `ccl_rate`, `blue_rate`, `amount_usd`. The QUERY tools' `search` filter matches `description` OR the joined category/subcategory names through `unaccent(lower(...))` (extension added by `20260814120000`), so `movements_description_trgm_idx` does **not** serve it — the index is on the raw column. Irrelevant at current volume; the fix, if it ever matters, is an expression index |
| `reminders` | `user_id` (PK)→users, `window_start_min`, `window_end_min`, `enabled`, `last_reminded_on`, `weekly_summary_enabled`, `last_summary_on`, `created_at`, `updated_at` | One row per user, minutes since ART midnight. Fire target (`MidpointMin()`) is derived, never stored. Delete == disable — no `deleted_at`. Carries **both** the daily reminder and the weekly summary |
| `user_nudges` | `user_id`→users, `nudge_key`, `sent_at` | Once-ever / cooldown storage for contextual tips |

## Conversation & queue tables

| Table | Key Fields | Notes |
|-------|-----------|-------|
| `conversation_states` | `user_id` (PK)→users, `flow_name`, `step_name`, `data` (JSONB), `updated_at` | One row per user. **Never access directly** — use `conversation.Engine`. `updated_at` is read, not just written: it drives the idle-timeout resume gate. The JSONB round-trip is why `data` values need decoding helpers (`internal/conversation/AGENTS.md`) |
| `chat_turns` | `id`, `user_id`→users, `question`, `answer`, `created_at` | Ephemeral conversation thread, hard-pruned past the TTL, never soft-deleted. Renamed from `query_turns` in `20260731120000`: it is **no longer QUERY-only** — every intent shares the thread, which is what the old name got wrong |
| `pending_llm_jobs` | `id`, `user_id`→users, `kind` (`free_text`/`update_pick`), `payload` (JSONB, opaque per `kind`), `created_at` | Durable queue for a **message** cached after a terminal Groq 429, drained FIFO per user. No `chat_id` — resolved at drain via `users.FindByID` |
| `pending_actions` | `id`, `user_id`→users, `tool`, `payload` (JSONB), `questions` (JSONB), `budget`, `position`, `trace_id`, `created_at` | Durable queue for an **already-interpreted action** waiting on an answer from the user (agent loop). Sibling of `pending_llm_jobs`, not the same thing. Drained one at a time (WIP=1); `budget` is frozen at park time on purpose |

## Observability tables

Populated by the 3-layer trace; **never read for feature logic** — domain tables only.

| Table | Key Fields | Notes |
|-------|-----------|-------|
| `intent_events` | `id`, `user_id`→users, `raw_message`, `intent`, `needs_confirmation`, `outcome`, `resolved_at`, `was_correct`, `movement_ids` (bigint[]), `trace_id` | Correlated by "last pending" via WIP=1 — an event left `pending` gets flipped to `abandoned` by the user's next message. `needs_confirmation` is vestigial (always `false`). `was_correct` is hand-labelled |
| `llm_calls` | `id`, `trace_id`, `call_type`, `model`, `prompt_tokens`, `completion_tokens`, `total_tokens`, `latency_ms`, `http_status`, `attempts`, `error`, `ratelimit_remaining_requests`, `ratelimit_remaining_tokens` | One row per HTTP call to Groq. `attempts > 1` means retries after a 429; the `ratelimit_remaining_*` columns are how you diagnose TPM exhaustion |
| `request_traces` | `id`, `trace_id`, `user_id` (nullable), `update_type`, `received_at`, `latency_ms`, `error` | The spine of one Telegram update. `user_id` nullable: a pre-auth update has none |

`llm_calls` and `request_traces` are purged by retention (`Sweeper`); `intent_events` is not — it
is business data with its own retention.

## Key relationships

- One `transaction_id` (UUID) groups N movements of a single financial event (a USD purchase is
  2 movements).
- **Every movement is attributed to a real account and carries a signed amount** — see
  [business-rules.md](business-rules.md#the-accounting-model-money-precision--read-this-before-touching-any-money-path). `expense` negative on its source,
  `income` positive on its destination, `transfer` legs signed out/in. The sign never escapes
  storage: user and LLM both see `abs`.
- **Account balance is always computed** — there is no `balance` column:
  ```sql
  SELECT SUM(amount) FROM movements WHERE account_id = $id AND deleted_at IS NULL
  ```
  Because the sum never looks at each row's currency, a USD row on a peso account is simply
  added in. That is why currency-vs-account agreement is a guard invariant, not a nicety.
- Enforced as of `2597c1e` (`movement.Normalize` validates every CREATE/UPDATE before insert).
  Rows written before that may still carry the old `account_id = NULL` / positive-expense shape
  — cleared via `/admin/users/:telegramID/reset`, not a retroactive migration.
