-- +goose Up
CREATE TABLE accounts (
    id          SERIAL PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id),
    name        TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'standard',
    currency    TEXT NOT NULL,
    is_default  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

-- el mismo nombre puede repetirse en monedas distintas (ej "FCI" en ARS
-- y "FCI" en USD), pero no dos veces en la misma moneda. Excluye
-- soft-deleted para permitir reusar el nombre tras un borrado lógico.
CREATE UNIQUE INDEX accounts_user_name_currency_idx
    ON accounts (user_id, name, currency)
    WHERE deleted_at IS NULL;

-- una sola cuenta default por usuario y moneda, ignorando soft-deleted.
CREATE UNIQUE INDEX accounts_default_currency_idx
    ON accounts (user_id, currency)
    WHERE is_default = TRUE AND deleted_at IS NULL;

CREATE INDEX accounts_deleted_at_idx ON accounts (deleted_at);

-- +goose Down
DROP TABLE accounts;