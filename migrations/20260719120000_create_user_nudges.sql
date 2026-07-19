-- +goose Up
CREATE TABLE user_nudges (
    user_id   BIGINT      NOT NULL REFERENCES users(id),
    nudge_key TEXT        NOT NULL,
    sent_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, nudge_key)
);

-- +goose Down
DROP TABLE user_nudges;
