# CLAUDE.md — lopii-finance-bot

Personal finance Telegram bot for Argentine users (ARS/USD). Natural-language input via Telegram, LLM-based intent classification, PostgreSQL persistence, Argentine financial context (inflation, multiple exchange rates).

**v1:** Google Sheets + Apps Script in `app_scripts_v1/` — historical reference only.  
**v2:** This repo — Go rewrite.

> **For current project state** (package map, data model, feature inventory, design decisions):
> read `docs/ARCHITECTURE.md` — it is the authoritative reference for what exists.

@docs/ARCHITECTURE.md

## 1. Overview & Architecture

### Stack

- Go, Gin, GORM, Postgres (Neon)
- Goose for migrations (run automatically at server startup)
- go-telegram/bot in webhook mode
- Viper for config (TOML per environment; env vars override)
- shopspring/decimal for money
- LLM orchestrator via Groq (not yet implemented)
- Deployment: Render + Docker

### Package map (`internal/`)

| Package | Role |
|---|---|
| `config` | Reads `config/{ENV}.toml` via Viper |
| `constants` | Domain constants: currencies, movement types |
| `currency` | `Currency` type (string alias), `ARS`/`USD` constants, `SupportedCurrencies` |
| `database` | GORM+Postgres singleton. `Initialize`, `RunMigrations` (goose) |
| `health` | `HealthChecker` that pings the DB |
| `user` | Model + repository: `FindByTelegramID`, `FindByID`, `Insert` |
| `invitation` | Model + repository: `Create` (generates random code), `FindByCode`, `MarkAsUsed` |
| `account` | Model + full repository + messages for flows |
| `subcategory` | Model + repository + messages for flows |
| `movement` | Model + repository (stub — no query methods yet) |
| `middleware` | `RequireAdmin(adminID)` |
| `conversation` | Engine: `Engine`, `Flow`, `TextStep`, `ChoiceStep`, `repository` |
| `controller/health` | HTTP: `/health/internal`, `/health/external` |
| `controller/invitation` | HTTP: `POST /invitations` |
| `controller/messaging` | Telegram: `/start`, catch-all for free text and callbacks |
| `server` | Bootstrap: DB, migrations, bot, webhook, controllers, Gin |

## 2. Conventions

### Language
Telegram UI strings in Argentine Spanish, defined in `messages.go` per package. All code identifiers in English.

### Repository pattern
Each controller defines its own local interfaces for the repositories it uses. Never import concrete types cross-package. Example in `controller/messaging`:

```go
type accountRepository interface {
    Insert(*account.Account) error
}
```

### Money
Always `shopspring/decimal`. Never `float64` for monetary amounts.

### Sentinel errors + unique violations
Sentinel errors per package (e.g., `account.ErrAccountAlreadyExists`). To detect Postgres unique constraint violations:

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    return ErrAccountAlreadyExists
}
```

### Tests
Unit tests with mocked repositories (no real Postgres). When adding a test, mock the package's local interface — not the concrete type.

### Conversation state
DB-backed JSONB in the `conversation_states` table. Never access this table directly — all interaction goes through `conversation.Engine`.

### Admin IDs
Loaded into memory at server startup. Zero extra queries per Telegram request.

## 3. Recipes

### Recipe 1: Add a DB migration

File name: `migrations/YYYYMMDDHHMMSS_<descriptive_name>.sql`

```sql
-- +goose Up
ALTER TABLE accounts ADD COLUMN alias TEXT;

-- +goose Down
ALTER TABLE accounts DROP COLUMN alias;
```

Create with:
```bash
goose create <descriptive_name> sql -dir ./migrations
```

Migrations run automatically at startup when `runMigrations = true` in the TOML. To run manually:
```bash
goose -dir ./migrations postgres "<connection_string>" up
```

### Recipe 2: Add a conversation flow

A flow is a graph of steps that persists state in `conversation_states`. The graph is validated statically at construction — if a step references a non-existent next step, the server fails to start.

**Steps:**

1. Define step name constants in the target package:
```go
const (
    stepAskName     = "ask_name"
    stepAskCurrency = "ask_currency"
    stepConfirm     = "confirm"
)
```

2. Build the Flow:
```go
func NewAccountSetupFlow(repo accountRepository) *conversation.Flow {
    steps := map[string]conversation.Step{
        stepAskName:     conversation.NewTextStep(...),
        stepAskCurrency: conversation.NewChoiceStep(...),
        stepConfirm:     conversation.NewChoiceStep(...),
    }
    flow, err := conversation.NewFlow("account_setup", stepAskName, steps)
    if err != nil {
        panic(err) // flow graph validation failed at startup
    }
    return flow
}
```

3. Register in `server.go`:
```go
conversationEngine.Register(NewAccountSetupFlow(accountRepo))
```

4. Start from a Telegram handler:
```go
engine.Start(userID, "account_setup")
```

5. Handle the result inside `handleConversationInput` when `result.Finished == true`:
```go
switch result.FlowName {
case "account_setup":
    name := result.Data["account_name"].(string)
    // INSERT into DB
}
```

### Recipe 3: Add an LLM intent

*(The LLM package does not exist yet — document here once implemented.)*

Planned intents: `CREATE | UPDATE | DELETE | QUERY`
- Tool calling: the LLM constructs action parameters, not just the intent type
- `UPDATE` = atomic `DELETE + INSERT` in a single SQL transaction
- `lastTransaction` in memory per user to resolve implicit references ("actually it was 1200")

### Recipe 4: Add an admin command

1. Register the handler in `controller/messaging/controller.go` with a prefix match:
```go
b.RegisterHandler(bot.HandlerTypeMessageText, "/new-invite", bot.MatchTypePrefix, handleNewInvite)
```

2. For HTTP admin endpoints, use the middleware:
```go
r.POST("/invitations", middleware.RequireAdmin(adminID), invitationController.Create)
```

3. Telegram deep-links: `https://t.me/<bot_username>?start=<CODE>`

## 4. Business Rules

### Currencies
- ARS and USD only. No implicit conversion between currencies.
- Currency fields use `currency.Currency` (string alias), never raw strings.
- Amount shorthands ("200k") → the LLM expands to 200000. The application code does not do this.

### Movement types
- `expense`: spending. `account_id = NULL`. Does not touch savings accounts.
- `income`: earning. `account_id = NULL`. Does not touch savings accounts.
- `transfer`: moves money into/out of a savings or investment account. `account_id NOT NULL`.
- Monthly summaries: filter `WHERE type != 'transfer'` for clean cash flow.

### Grouped transactions
`transaction_id` (nullable UUID) groups N movements of one atomic operation:

| Operation | Movements |
|---|---|
| USD purchase | `expense -100,000 ARS` (account_id=NULL) + `transfer +100 USD` (account_id=usd_wallet) |
| FCI subscription | `transfer -2,500,000 ARS` (bank account) + `transfer +2,500,000 ARS` (FCI account) |
| FCI redemption with gain | 2 transfers (redemption) + 1 `income` (subcategory: `Sistema \| Rendimiento inversión`, account_id=NULL) |

### Accounts
- Table: `id, user_id, name, currency (ARS|USD), is_default, deleted_at`
- No `type` column (current migration has `type DEFAULT 'standard'` — legacy artifact to drop)
- ARS and USD wallets are created automatically at `/start`
- Additional accounts are created organically: first time the user records a transfer to a non-existent account, the bot asks whether to create it
- Unique index: `(user_id, name, currency) WHERE deleted_at IS NULL`
- Unique index: `(user_id, currency) WHERE is_default = TRUE AND deleted_at IS NULL`

### Balances
No `balance` column on accounts. Always computed:
```sql
SELECT SUM(amount) FROM movements
WHERE account_id = $account_id AND deleted_at IS NULL
```

### Exchange rates
- Daily: BNA, MEP, CCL, blue via `dolarapi.com`
- Monthly CPI via `api.argentinadatos.com`
- Denormalized snapshot on each movement INSERT: `bna_rate`, `mep_rate`, `ccl_rate`, `blue_rate`, `amount_usd` (not yet implemented — planned addition to the movements table)

### Categories and subcategories
- Strictly two-level tree: `category > subcategory`. Never deeper.
- `user_id = NULL` → global (visible to all). `user_id NOT NULL` → user-created.
- Only admin can create global subcategories (`is_global = TRUE`).
- ~80 global subcategories seeded in migration `20260625234857`, across 14 categories.
- Reserved: `PENDING_REVIEW | PENDING_REVIEW` (low LLM confidence), `Sistema | Saldo inicial`, `Sistema | Rendimiento inversión`

### LLM classification
- Intents: `CREATE | UPDATE | DELETE | QUERY` — classified via Groq
- Tool calling: the LLM constructs action parameters, not just the intent type
- Low confidence → `PENDING_REVIEW` subcategory, bot asks for confirmation
- `UPDATE` = atomic `DELETE + INSERT` (never partial patch)
- `lastTransaction` in memory per user to resolve implicit references

### Bot interaction
- No Telegram commands for end users. Everything is free text → LLM → flow or query handler.
- Exceptions: `/start` (onboarding) and admin commands (e.g. `/new-invite`)
- Timezone: `America/Argentina/Buenos_Aires`
- Default payment method when LLM cannot infer: `transfer`

## 5. Local Dev Setup

**Prerequisites:** Go 1.26+, Docker (for local Postgres), devtunnel or ngrok (public HTTPS URL for Telegram webhooks).

### Start Postgres
```bash
docker compose up -d
```
Postgres 16 on `:5432`. Credentials: DB=`lopiibot`, user=`lopiibot`, pass=`lopiibot`. Data stored in `./db-data/` (git-ignored).

### Configure before first run

**1. Secrets in `config/local.toml`:**
```toml
[server]
baseHost = "https://<your-tunnel>.devtunnels.ms"  # public HTTPS URL for Telegram webhook

[telegram]
token = "<token from @BotFather>"
```
The DB config is already set to match Docker Compose — no changes needed.

**2. Edit the admin migration (first time only):**
`migrations/20260618230837_create_admin_user.sql` — replace `'TELEGRAM_ID'` with your numeric Telegram user ID.

### Run
```bash
cd cmd/server && ENV=local go run .
```
Migrations run automatically at startup. Working directory must be `cmd/server/` — the config path resolves as `../../config/{ENV}.toml`.

### Create a new migration
```bash
goose create <descriptive_name> sql -dir ./migrations
```

### Tests
No tests yet. When adding the first one, mock the package's local repository interface — not the concrete type.

## 6. Technical Debt

- `accounts` table has a `type DEFAULT 'standard'` column from a prior design — drop with a migration.
- Migration `20260618230837_create_admin_user.sql` has literal `telegram_id = 'TELEGRAM_ID'` — must be edited manually before each new-environment deploy.
- `middleware.RequireAdmin` is hardcoded to user ID 1 — needs real auth.
- `handleConversationInput` in `controller/messaging/controller.go` sends `"TO_REVIEW"` when a flow finishes — needs to dispatch on `result.FlowName`.
