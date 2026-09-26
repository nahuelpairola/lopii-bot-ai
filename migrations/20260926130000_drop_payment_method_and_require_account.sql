-- +goose Up
ALTER TABLE movements DROP COLUMN payment_method;
ALTER TABLE movements ALTER COLUMN account_id SET NOT NULL;

-- +goose Down
ALTER TABLE movements ALTER COLUMN account_id DROP NOT NULL;
ALTER TABLE movements ADD COLUMN payment_method TEXT;
