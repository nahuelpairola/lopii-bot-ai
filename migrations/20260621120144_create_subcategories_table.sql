-- +goose Up
CREATE TABLE subcategories (
    id           SERIAL PRIMARY KEY,
    user_id      INTEGER REFERENCES users(id),
    category     TEXT NOT NULL,
    subcategory  TEXT NOT NULL,
    description  TEXT,
    is_global    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

-- user_id puede ser NULL (globales), por eso el unique se arma con índices
-- parciales en lugar de un UNIQUE constraint directo.
CREATE UNIQUE INDEX subcategories_user_unique_idx
    ON subcategories (user_id, category, subcategory)
    WHERE user_id IS NOT NULL;

CREATE UNIQUE INDEX subcategories_global_unique_idx
    ON subcategories (category, subcategory)
    WHERE is_global = TRUE;

-- +goose Down
DROP TABLE subcategories;