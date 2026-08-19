# lopii-finance-bot

Personal finance Telegram bot for Argentine users (ARS/USD). Free text in, an LLM agent loop
picks a tool, PostgreSQL stores the movements. Go + Gin + GORM, deployed on Render.

- **Working on this repo (human or agent): read [AGENTS.md](AGENTS.md) first.** Conventions, the
  accounting model, the build tags, and where every other doc lives.
- **Local setup:** [docs/dev-setup.md](docs/dev-setup.md) — Postgres, config, tunnel, run.
- **Verify a change:** `bash check.sh` — build, vet, errcheck and the default test suite.
- **Structure:** ask `codegraph_explore`; the repo is indexed. There is no package map on purpose.

### Database

```bash
docker compose up -d                              # local Postgres
goose create <name> sql -dir ./migrations         # new migration (they run at startup)
```
