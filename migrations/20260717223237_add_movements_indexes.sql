-- +goose Up
CREATE INDEX IF NOT EXISTS movements_account_id_idx ON movements (account_id);
CREATE INDEX IF NOT EXISTS movements_user_currency_date_idx ON movements (user_id, currency, date);

-- +goose Down
DROP INDEX IF EXISTS movements_account_id_idx;
DROP INDEX IF EXISTS movements_user_currency_date_idx;
