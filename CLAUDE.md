# CLAUDE.md — lopii-finance-bot

Personal finance Telegram bot for Argentine users (ARS/USD). Two users (a couple). Natural-language input via Telegram, automatic LLM classification, PostgreSQL persistence, with Argentine financial context (inflation, multiple exchange rates).

**v1:** Google Sheets + Apps Script in `app_scripts_v1/` — historical reference only.  
**v2:** This repo — Go rewrite.

## 1. Overview & Architecture

### Stack

- Go, Gin, GORM, Postgres (Neon)
- Goose for migrations (run automatically at server startup)
- go-telegram/bot in long-polling mode
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
    FindDefaultByCurrency(ctx context.Context, userID uint, currency currency.Currency) (*account.Account, error)
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
    return conversation.NewFlow("account_setup", steps, stepAskName)
}
```

3. Register in `server.go`:
```go
conversationEngine.Register(NewAccountSetupFlow(accountRepo))
```

4. Start from a Telegram handler:
```go
engine.Start(ctx, userID, "account_setup")
```

5. Handle the result in `handleConversationFinished`:
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
b.RegisterHandlerByCommand(bot.HandlerTypeMessageText, "/new-invite", handleNewInvite)
```

2. For HTTP admin endpoints, use the middleware:
```go
r.POST("/invitations", middleware.RequireAdmin(adminID), invitationController.Create)
```

3. Telegram deep-links: `https://t.me/<bot_username>?start=<CODE>`
