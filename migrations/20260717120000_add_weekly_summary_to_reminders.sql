-- +goose Up
ALTER TABLE reminders
    ADD COLUMN weekly_summary_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN last_summary_on        DATE;

-- +goose Down
ALTER TABLE reminders
    DROP COLUMN weekly_summary_enabled,
    DROP COLUMN last_summary_on;
