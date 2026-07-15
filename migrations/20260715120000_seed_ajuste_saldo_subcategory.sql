-- +goose Up
INSERT INTO subcategories (user_id, category, subcategory, description, is_global, icon) VALUES
(NULL, 'Sistema', 'Ajuste de saldo', 'Ajuste manual del saldo declarado de una cuenta.', TRUE, '⚙️');

-- +goose Down
DELETE FROM subcategories WHERE category = 'Sistema' AND subcategory = 'Ajuste de saldo' AND is_global = TRUE;
