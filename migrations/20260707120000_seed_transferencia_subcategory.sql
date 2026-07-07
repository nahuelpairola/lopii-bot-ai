-- +goose Up
INSERT INTO subcategories (user_id, category, subcategory, description, is_global, icon) VALUES
(NULL, 'Sistema', 'Transferencia', 'Movimiento de dinero entre dos cuentas propias', TRUE, '🔄');

-- +goose Down
DELETE FROM subcategories WHERE category = 'Sistema' AND subcategory = 'Transferencia' AND is_global = TRUE;
