-- +goose Up
-- El índice original no filtra deleted_at, así que una fila soft-deleteada
-- sigue ocupando el slot único: borrar "Comida/Delivery" y volver a crearla
-- devolvía 23505 contra una fila invisible para el usuario.
DROP INDEX IF EXISTS subcategories_user_unique_idx;

CREATE UNIQUE INDEX subcategories_user_unique_idx
    ON subcategories (user_id, category, subcategory)
    WHERE user_id IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS subcategories_user_unique_idx;

CREATE UNIQUE INDEX subcategories_user_unique_idx
    ON subcategories (user_id, category, subcategory)
    WHERE user_id IS NOT NULL;
