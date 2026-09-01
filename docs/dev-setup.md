# Local Dev Setup — lopii-finance-bot

**Prerequisites:** Go 1.26+, Docker (for local Postgres), devtunnel or ngrok (public HTTPS URL for Telegram webhooks).

### Start Postgres
```bash
docker compose up -d
```
Postgres 16 on `:5432`. Credentials: DB=`lopiibot`, user=`lopiibot`, pass=`lopiibot`. Data stored in `./db-data/` (git-ignored).

### Configure before first run

**1. Secrets in `.env` (git-ignored), copied from `.env.example`:**
```bash
ENV=local
TELEGRAM_TOKEN=<token from @BotFather>
GROQ_APIKEY=<key from console.groq.com>
```
`telegram.token` and `groq.apiKey` are deliberately **empty** in `config/local.toml` — no
secret is ever committed. Viper's `AutomaticEnv` fills them from the environment, so the
`.env` has to be exported into the shell before `go run` (see Run below); `go run` does not
read it on its own.

**2. Tunnel host in `config/local.toml`:**
```toml
[server]
baseHost = "https://<your-tunnel>.devtunnels.ms"  # public HTTPS URL for Telegram webhook
```
The DB config is already set to match Docker Compose — no changes needed.

**3. Edit the admin migration (first time only):**
`migrations/20260618230837_create_admin_user.sql` — replace `'TELEGRAM_ID'` with your numeric Telegram user ID.

### Run

```bash
docker ps --filter name=postgres --format '{{.Names}} {{.Status}}'   # 1. Postgres up?
cd cmd/server && set -a && . ../../.env && set +a && go run .        # 2. start
```

Then check it came up, in another shell:

```bash
curl -s http://localhost/health/internal          # "OK" — server + DB
curl -sI "$(awk -F'"' '/baseHost/{print $2}' ../../config/local.toml)/health/external"
                                                  # 200 — the tunnel reaches this server
```

Migrations run automatically at startup.

**Three ways this fails silently — `cmd/server/main.go` discards both startup errors and
exits 1 with no message, so a bad start looks identical to a crash:**

1. **Wrong working directory.** It must be `cmd/server/`; the config path resolves as
   `../../config/{ENV}.toml`, relative to the process's cwd. From the repo root the file is
   simply not found.
2. **`.env` not exported.** `go run` does not read `.env`. Without `set -a && . ../../.env`,
   `ENV` is unset, so the config path becomes `../../config/.toml`. The `set -a` matters:
   plain `.` sources the file but does not export, and Viper only reads exported vars.
   If your `.env` was saved with Windows line endings, every value carries a trailing `\r`
   and the token is rejected by Telegram — source it as
   `. <(tr -d '\r' < ../../.env)` instead.
3. **Tunnel down.** The server starts fine and serves localhost, but Telegram cannot reach
   the webhook, so the bot silently receives nothing. That is what the second curl catches —
   a devtunnel URL changes when the tunnel restarts, and `baseHost` then points nowhere.

### Create a new migration
```bash
goose create <descriptive_name> sql -dir ./migrations
```

### Tests

```bash
bash check.sh    # build + vet + errcheck + the default suite
```

Unit tests mock the package's own local interfaces — no real Postgres outside the `integration`
tag. Four build tags gate the suites that need Postgres or a Groq key. **None run in CI — there
is no CI.** They run when someone runs them.

| Tag | Needs | Notes |
|---|---|---|
| `integration` | local Postgres (`docker compose up -d`) | 11 files. Drains and writes real rows |
| `conv_test` | nothing | 3 files |
| `llm_eval` | `GROQ_APIKEY` | 3 files, `internal/orchestrator` |
| `query_eval` | `GROQ_APIKEY` **and** Postgres | 1 file |

Both eval suites **fail loudly when `GROQ_APIKEY` is unset** rather than skipping in silence: the
tag is asked for by hand, so a skip was a green that proved nothing. They spend real Groq quota,
shared with production. `GROQ_APIKEY` is the only name that works — `GROQ_API_KEY` is exported by
nothing, which is why the evals had never once run.

`errcheck` runs inside `check.sh` as **information, not a gate**: the tree carries ~215 pre-existing
findings (~38 outside tests, nearly all unchecked `bot.SendMessage`). Read the ones in your own
diff, ignore the rest.
