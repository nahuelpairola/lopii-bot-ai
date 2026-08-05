-- +goose Up
ALTER TABLE user_nudges ADD COLUMN tapped_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE user_nudges DROP COLUMN tapped_at;
