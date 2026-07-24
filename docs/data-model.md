# Data Model — lopii-finance-bot

> Tables and relationships. Schema authority is the Goose migrations in `migrations/`; this is the map.

| Table | Key Fields | Notes |
|-------|-----------|-------|
| `users` | `id`, `telegram_id` (UNIQUE), `username`, `is_admin`, `deleted_at` | Soft delete |
| `invitations` | `id`, `code` (6-char UNIQUE), `created_by`→users, `used_by`→users, `expires_at`, `used_at` | 72h expiry, single-use |
| `accounts` | `id`, `user_id`→users, `name`, `type` (legacy), `currency`, `is_default`, `deleted_at` | `type` column is a legacy artifact — needs a drop migration; unique index is case-insensitive on name (migration 20260704120000) |
| `conversation_states` | `user_id` (PK)→users, `flow_name`, `step_name`, `data` (JSONB), `updated_at` | One row per user; never access directly — use `conversation.Engine`; `updated_at` is now read (not just written) by `Engine`'s idle-timeout resume gate |
| `subcategories` | `id`, `user_id` (nullable)→users, `category`, `subcategory`, `description`, `is_global`, `icon`, `deleted_at` | `user_id = NULL` = global (visible to all users); `icon` backfilled for the seeded taxonomy in migration `20260705120000` |
| `movements` | `id`, `transaction_id` (UUID nullable), `user_id`→users, `account_id` (nullable)→accounts, `subcategory_id`→subcategories, `date`, `type` (expense/income/transfer), `amount` (NUMERIC 15,2), `currency`, `payment_method`, `merchant`, `description`, `deleted_at` | Rate columns pending: `bna_rate`, `mep_rate`, `ccl_rate`, `blue_rate`, `amount_usd` |
| `intent_events` | `id`, `created_at`, `user_id`→users, `raw_message`, `intent`, `needs_confirmation`, `outcome`, `resolved_at`, `was_correct`, `movement_ids` (bigint[], nullable) | Correlación por "último pending" vía WIP=1; `was_correct` etiquetado a mano; `movement_ids` = ids con los que terminó la operación (create/update insertados, delete borrados) para trazabilidad mensaje→filas |
| `query_turns` | `id`, `user_id`→users, `question`, `answer`, `created_at` | QUERY conversation thread. Ephemeral — hard-pruned by `Append` past the TTL, never soft-deleted. Read only within the TTL window, capped at N turns |
| `reminders` | `user_id` (PK)→users, `window_start_min`, `window_end_min` (minutes since ART midnight), `enabled`, `last_reminded_on` (date, nullable), `created_at`, `updated_at` | One row per user. Fire target (`MidpointMin()`) is derived, never stored. Delete == disable (`enabled=false`) — no `deleted_at` |
| `pending_llm_jobs` | `id`, `user_id`→users, `kind` (`free_text`/`update_pick`), `payload` (JSONB, opaque per `kind`), `created_at` | Durable queue: a user message cached after a terminal Groq 429 (rate limit), drained FIFO per user (`created_at` order, index `(user_id, created_at)`) once quota frees up. No `chat_id` — resolved at drain time via `users.FindByID`. Operational table (the queue itself), not metrics — reading it for display/observability is fine, reading it for feature logic elsewhere is not |

### Key relationships

- One `transaction_id` (UUID) groups N movements of a single financial event (e.g. USD purchase = 2 movements)
- **Every movement is attributed to a real account and carries a signed amount** (see [business-rules.md](business-rules.md#the-accounting-model)): `expense` negative on its source account, `income` positive on its destination, `transfer` legs signed out/in. `account_id` is effectively NOT NULL for all new rows (the column stays nullable only for legacy rows). The sign is internal to storage — user and LLM both see `abs`.
- **Account balance is always computed** — no `balance` column — as a plain sum of signed amounts:
  ```sql
  SELECT SUM(amount) FROM movements WHERE account_id = $id AND deleted_at IS NULL
  ```
  > Migration status: enforced as of commit `2597c1e` (`normalizeMovements` validates every CREATE/UPDATE before insert). Legacy rows written before that land may still carry the old `account_id = NULL` / positive-expense shape — cleared via `/admin/users/:telegramID/reset`, not a retroactive migration.
