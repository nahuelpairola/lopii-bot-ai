# lopii-personal-finance-ai-bot

### Database

#### Create local database

Run docker compose compose command:
```
docker compose up -d
```

#### Migrations

Run migration command:

```
goose create create_users_table sql -dir ./migrations
```

