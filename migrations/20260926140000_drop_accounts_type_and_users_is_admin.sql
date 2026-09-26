-- +goose Up
ALTER TABLE accounts DROP COLUMN type;
ALTER TABLE users DROP COLUMN is_admin;

-- +goose Down
ALTER TABLE users ADD COLUMN is_admin BOOLEAN DEFAULT FALSE;
ALTER TABLE accounts ADD COLUMN type TEXT NOT NULL DEFAULT 'standard';
