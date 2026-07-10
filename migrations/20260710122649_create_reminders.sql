-- +goose Up
CREATE TABLE reminders (
    user_id           BIGINT PRIMARY KEY REFERENCES users(id),
    window_start_min  INT  NOT NULL,
    window_end_min    INT  NOT NULL,
    enabled           BOOLEAN NOT NULL DEFAULT TRUE,
    last_reminded_on  DATE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE reminders;
