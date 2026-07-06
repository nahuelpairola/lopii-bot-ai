-- +goose Up
ALTER TABLE subcategories ADD COLUMN icon TEXT;

UPDATE subcategories SET icon = '🍔' WHERE category = 'Alimentación' AND is_global = TRUE;
UPDATE subcategories SET icon = '🏠' WHERE category = 'Vivienda' AND is_global = TRUE;
UPDATE subcategories SET icon = '🚗' WHERE category = 'Transporte' AND is_global = TRUE;
UPDATE subcategories SET icon = '🩺' WHERE category = 'Salud' AND is_global = TRUE;
UPDATE subcategories SET icon = '🎉' WHERE category = 'Ocio y salidas' AND is_global = TRUE;
UPDATE subcategories SET icon = '🧘' WHERE category = 'Bienestar' AND is_global = TRUE;
UPDATE subcategories SET icon = '👕' WHERE category = 'Indumentaria' AND is_global = TRUE;
UPDATE subcategories SET icon = '💻' WHERE category = 'Tecnología' AND is_global = TRUE;
UPDATE subcategories SET icon = '🔁' WHERE category = 'Suscripciones' AND is_global = TRUE;
UPDATE subcategories SET icon = '📚' WHERE category = 'Educación' AND is_global = TRUE;
UPDATE subcategories SET icon = '📈' WHERE category = 'Inversiones' AND is_global = TRUE;
UPDATE subcategories SET icon = '🗂️' WHERE category = 'Otros' AND is_global = TRUE;
UPDATE subcategories SET icon = '🏦' WHERE category = 'Finanzas' AND is_global = TRUE;
UPDATE subcategories SET icon = '💵' WHERE category = 'Ingresos' AND is_global = TRUE;
UPDATE subcategories SET icon = '⏳' WHERE category = 'PENDING_REVIEW' AND is_global = TRUE;
UPDATE subcategories SET icon = '⚙️' WHERE category = 'Sistema' AND is_global = TRUE;

-- +goose Down
ALTER TABLE subcategories DROP COLUMN icon;
