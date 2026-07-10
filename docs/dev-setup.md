# Local Dev Setup — lopii-finance-bot

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
First tests added in `internal/controller/messaging` (`onboarding_flow_test.go`) — mocked local repository interfaces, no real Postgres. Follow the same pattern for new packages.
