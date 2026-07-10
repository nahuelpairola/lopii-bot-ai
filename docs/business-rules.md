# Business Rules — lopii-finance-bot

> Domain rules. The money/accounting model below is the load-bearing one — `CLAUDE.md` carries only its condensed warning and links here.

### Currencies
- ARS and USD only. No implicit conversion between currencies.
- Currency fields use `currency.Currency` (string alias), never raw strings.
- Amount shorthands ("200k") → the LLM expands to 200000. The application code does not do this.

## The accounting model (money precision — READ THIS before touching any money path)

**Every movement is signed and attributed to a real account. No exceptions.** Accounts
hold the user's actual money (bank, Mercado Pago, broker); a balance is *always*
`SUM(amount)` over its movements, so the stored sign IS the accounting. Getting a sign
or an `account_id` wrong silently corrupts a balance — this is the one place in the
codebase where a small mistake is a financial bug, not a cosmetic one.

| Type | `account_id` | Stored sign | Meaning |
|---|---|---|---|
| `expense` | source account (required) | **negative** (`-amount.Abs()`) | money leaves an account |
| `income` | destination account (required) | **positive** (`+amount.Abs()`) | money enters an account |
| `transfer` | both legs (required) | negative out / positive in | money moves between two own accounts |

Non-negotiable rules:
- **The app owns the sign, never the LLM.** Normalization forces `expense`→negative,
  `income`→positive on write. The LLM emits positive magnitudes; app code applies the sign.
- **The sign is internal to storage.** It never escapes: both the user (receipts,
  diffs, pickers) and the LLM (UPDATE/DELETE candidates) always see `amount.Abs()`.
  Direction is conveyed by the movement type, never a `-`. Feeding a signed amount to
  either audience is a bug (it was the cause of a real `0.00` corruption).
- **Account resolution is app-side, deterministic** (the account list shown to the LLM
  does not mark the default, so the LLM structurally cannot pick it): LLM-matched
  account → it; nil → currency default (`FindDefaultByCurrency`); no account in that
  currency → gap-fill asks. Applies uniformly to expense, income, and transfer legs.
- **Insert-time invariants (the guard), enforced for CREATE and UPDATE alike:**
  sign matches type; `amount != 0`; movement currency == attributed account currency; a
  transfer's two legs reference *different* accounts and (same currency) sum to 0;
  expense/income never trigger counterparty-named account creation. Malformed → reject
  + reword (typed sentinel errors mapped to specific copy).
- **Anomaly detection — the insufficient-funds gate.** A well-formed movement that
  drives an account *into or deeper into* negative (`after < 0 && after < before`, per
  account) is neither silently inserted nor rejected: it stops at a confirm gate
  showing the shortfall — **Registrar igual / Reescribir / Falta registrar algo** (the
  last aborts stateless with a hint to log the missing movement first). Reuses the
  existing `ChoiceStep` confirm pattern; friction only on the anomaly path (CREATE
  stays frictionless otherwise). This is the point of the model: correct numbers, and a
  suspicious result surfaced for the user to resolve, never buried.
- **Never `float64`.** Always `shopspring/decimal` — see the Money convention above.
- Monthly cash-flow summaries: filter `WHERE type != 'transfer'`; report `abs(amount)`
  by type/subcategory (the sign is a storage detail, not a reporting one).

> **Enforcement status:** the signed/attributed model is enforced. `resolveAndInsertMovements`
> validates every CREATE/UPDATE through `normalizeMovements` (the guard) before insert —
> sign, currency/account agreement, transfer-group shape, and account attribution are all
> checked, never trusted from the LLM. Rows written before this shipped may still carry the
> legacy `account_id = NULL` shape; the fix is `POST /admin/users/:telegramID/reset`, not a
> retroactive migration.

### Grouped transactions
`transaction_id` (nullable UUID) groups N movements of one atomic operation. **Grouping
is signaled by the LLM (a `group` field), never inferred from movement count** — a single
message with several *independent* movements ("compré pan, medicamentos y carne") produces
several movements each with `transaction_id = NULL`, so UPDATE/DELETE touches one without
touching the others. Only the legs of a genuinely atomic operation (below) share a
`transaction_id`; card itemization is **independent** expenses, not a group.

| Operation | Movements |
|---|---|
| USD purchase | `transfer -150,000 ARS` (account_id=ars_account) + `transfer +100 USD` (account_id=usd_account), subcategory `Inversiones \| Dólares` |
| Same-currency transfer | `transfer -X` (source account) + `transfer +X` (dest account), same currency, subcategory `Sistema \| Transferencia` |
| FCI subscription | `transfer -2,500,000 ARS` (bank account) + `transfer +2,500,000 ARS` (FCI account) |
| FCI redemption with gain | 2 transfers (redemption) + 1 `income` (subcategory: `Sistema \| Rendimiento inversión`, `account_id` = the FCI account, so it ends at balance 0) |

### Accounts hold a fixed monetary amount, not asset positions
- An account's balance is always a single ARS or USD number — the current value. The bot does not model stocks/ETFs/cedears/FCI cuotapartes as units × price, does not auto-revalue, and does not accrue interest.
- An "investment account" is just an account whose current value the user states as a fixed amount.
- Gains (rendimiento) belong to the **account** that earned them, not to any instrument. A gain is an `income` (`Sistema | Rendimiento inversión`) **attributed to that account**, growing its balance — never a silent bump. Any account can have its own. Stated explicitly ("el broker rindió 10 mil") it's a plain attributed income; on a redemption of more than the balance the app back-computes it (currently FCI only — see the money model).
- `ClassifyOnboarding` extracts a monetary balance only — never units/shares/tickers.

### Accounts
- Table: `id, user_id, name, currency (ARS|USD), is_default, deleted_at`
- No `type` column (current migration has `type DEFAULT 'standard'` — legacy artifact to drop)
- Accounts are created via the onboarding free-text flow: user describes N accounts (name + currency + opening balance), the bot calls `ClassifyOnboarding`, inserts them atomically via `InsertAccountsWithOpenings` with opening `transfer` movements (subcategory `Sistema | Saldo inicial`).
- The first account per currency is flagged `IsDefault=true` — a throwaway seed that satisfies the partial unique index `(user_id, currency) WHERE is_default = TRUE AND deleted_at IS NULL`. This default may be reset to `false` later by the user if they create additional accounts in that currency.
- Additional accounts beyond the onboarding flow are created organically via ACCOUNT_CREATE intent (user explicitly asks to create an account, never `IsDefault=true`).
- Unique index: `(user_id, name, currency) WHERE deleted_at IS NULL` (case-insensitive)
- Unique index: `(user_id, currency) WHERE is_default = TRUE AND deleted_at IS NULL`

### Balances
No `balance` column on accounts. Always computed as a plain sum of **signed** amounts —
this is why the sign convention above is load-bearing, not stylistic:
```sql
SELECT SUM(amount) FROM movements
WHERE account_id = $account_id AND deleted_at IS NULL
```
Negative balances are allowed (a mis-entry or overdraft) — corrected via UPDATE, never
floored silently.

### Exchange rates
- Daily: BNA, MEP, CCL, blue via `dolarapi.com`
- Monthly CPI via `api.argentinadatos.com`
- Denormalized snapshot on each movement INSERT: `bna_rate`, `mep_rate`, `ccl_rate`, `blue_rate`, `amount_usd` (not yet implemented — planned addition to the movements table)

### Categories and subcategories
- Strictly two-level tree: `category > subcategory`. Never deeper.
- `user_id = NULL` → global (visible to all). `user_id NOT NULL` → user-created.
- Only admin can create global subcategories (`is_global = TRUE`).
- ~60 global subcategories across 16 categories (+ `Sistema`/`PENDING_REVIEW` reserved), reseeded in migration `20260710130000` (coarse-nitid redesign; replaces `20260625234857`). Fine detail lives in `merchant`/`description` free text, not in extra buckets.
- Reserved: `PENDING_REVIEW | PENDING_REVIEW` (low LLM confidence), `Sistema | Saldo inicial`, `Sistema | Rendimiento inversión`
- Reserved category names (`PENDING_REVIEW`, `Sistema`, case-insensitive) apply to user-created categories too, not just the seeded taxonomy — checked at creation time in `subcategory_setup_flow.go`.

### LLM classification
- Intents: `CREATE | UPDATE | DELETE | QUERY` — classified via Groq (Call 1 router also returns `needs_confirmation`, meaningful only for CREATE)
- Tool calling: the LLM constructs action parameters, not just the intent type
- Low confidence → `PENDING_REVIEW` subcategory, bot asks for confirmation
- A CREATE the router flags as ambiguous, or that matches an existing recent movement (`resolveCandidates`), stops at a reescribir/cancelar confirm gate instead of inserting — CREATE's frictionless default has this one exception
- `UPDATE` = atomic `DELETE + INSERT` (never partial patch)
- Implicit references ("actually it was 1200") resolve via `resolveCandidates` (in-Go token/amount match over a DB window: recency of entry `created_at`/48h by default, a mentioned date anchors on business `date`) — no in-memory last-transaction store

### Bot interaction
- No Telegram commands for end users. Everything is free text → LLM → flow or query handler.
- Exceptions: `/start` (onboarding) and admin commands (e.g. `/new-invite`)
- Timezone: `America/Argentina/Buenos_Aires`
- Default payment method when LLM cannot infer: `transfer`

### Reminders
- One reminder per user (`reminders` table, PK `user_id`). Configured/edited/disabled entirely by free text via `REMINDER_SET` — no confirm gate, like ACCOUNT_CREATE/CREATE_CATEGORY.
- Window stored as minutes-since-midnight ART (`window_start_min`/`window_end_min`), not a SQL `time` — the fire target is the midpoint (`Reminder.MidpointMin()`, derived, never stored), which needs sub-hour precision.
- Activity-aware: fires only if the user has logged **zero** movements today (any type, via `FindRecentlyCreatedForUser`) — never nags on a day already engaged.
- Delivery: `internal/notifier.Sweeper`, an in-process `time.Ticker` (default 5 min, `[reminders].sweepIntervalMinutes`), not an external cron — the bot process is already 24/7 single-instance.
- Delete == disable (`enabled=false`). No `deleted_at` — the user-facing fact is the same either way.
- Consulting the reminder ("¿a qué hora me recordás?") is QUERY's `get_reminder` tool, not REMINDER_SET.
