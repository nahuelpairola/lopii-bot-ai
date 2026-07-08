-- +goose Up
alter table intent_events add column movement_ids bigint[];

-- +goose Down
alter table intent_events drop column movement_ids;
