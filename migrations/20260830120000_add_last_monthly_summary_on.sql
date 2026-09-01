-- +goose Up
ALTER TABLE reminders ADD COLUMN last_monthly_summary_on DATE;

-- +goose Down
ALTER TABLE reminders DROP COLUMN last_monthly_summary_on;
