-- +goose Up
ALTER TABLE movements DROP COLUMN merchant;

-- +goose Down
ALTER TABLE movements ADD COLUMN merchant TEXT;
CREATE INDEX movements_merchant_trgm_idx ON movements USING gin (merchant gin_trgm_ops);
