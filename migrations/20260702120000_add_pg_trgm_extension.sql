-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX movements_description_trgm_idx ON movements USING gin (description gin_trgm_ops);
CREATE INDEX movements_merchant_trgm_idx ON movements USING gin (merchant gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS movements_merchant_trgm_idx;
DROP INDEX IF EXISTS movements_description_trgm_idx;
DROP EXTENSION IF EXISTS pg_trgm;
