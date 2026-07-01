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
