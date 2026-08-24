-- +goose Up
CREATE TABLE IF NOT EXISTS user_channels (
    id              SERIAL PRIMARY KEY,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    channel         TEXT NOT NULL,
    channel_user_id TEXT NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (channel, channel_user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_channels_user_id ON user_channels (user_id);

INSERT INTO user_channels (user_id, channel, channel_user_id)
SELECT id, 'telegram', telegram_id
FROM users
WHERE telegram_id IS NOT NULL AND telegram_id <> ''
ON CONFLICT (channel, channel_user_id) DO NOTHING;

ALTER TABLE users DROP COLUMN telegram_id;

-- +goose Down
ALTER TABLE users ADD COLUMN telegram_id TEXT UNIQUE NULL;

UPDATE users u
SET telegram_id = uc.channel_user_id
FROM user_channels uc
WHERE uc.user_id = u.id AND uc.channel = 'telegram';

DROP TABLE IF EXISTS user_channels;
