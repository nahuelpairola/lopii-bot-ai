-- +goose Up
INSERT INTO users (telegram_id, username, is_admin, created_at)
VALUES ('TELEGRAM_ID', 'Admin', TRUE, NOW())
ON CONFLICT (telegram_id) DO NOTHING;

-- +goose Down
DELETE FROM users WHERE telegram_id = 'TELEGRAM_ID' AND is_admin = TRUE;