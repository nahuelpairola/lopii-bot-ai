-- +goose Up
DROP INDEX accounts_user_name_currency_idx;

-- insensible a mayúsculas: "Wallet" y "WalleT" en la misma moneda para el
-- mismo usuario deben colisionar. Excluye soft-deleted, igual que el
-- índice que reemplaza.
CREATE UNIQUE INDEX accounts_user_name_currency_ci_idx
    ON accounts (user_id, LOWER(name), currency)
    WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX accounts_user_name_currency_ci_idx;

CREATE UNIQUE INDEX accounts_user_name_currency_idx
    ON accounts (user_id, name, currency)
    WHERE deleted_at IS NULL;
