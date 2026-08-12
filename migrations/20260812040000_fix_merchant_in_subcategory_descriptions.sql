-- +goose Up
-- Tres descripciones globales le dicen al modelo que mande datos a merchant.
-- NO son comentarios: subcategories.description viaja a
-- orchestrator.TaxonomyEntry.Description y entra al prompt en CADA
-- clasificación. Después del fold estarían pidiendo un campo que el schema ya
-- no acepta.
--
-- Ningún grep de Go las encuentra: son datos, no código. Por eso van en su
-- propia migración y no en el barrido del paquete.
--
-- El WHERE mira el texto y no el id: un usuario pudo haber editado su copia.
UPDATE subcategories
SET description = 'Compra chica en almacén, kiosco, dietética, carnicería, verdulería o panadería (el comercio va en la descripción). NO compra grande de super.'
WHERE subcategory = 'Almacén / barrio' AND description ILIKE '%merchant%';

UPDATE subcategories
SET description = 'Ropa, zapatillas o zapatos (el rubro va en la descripción).'
WHERE subcategory = 'Ropa y calzado' AND description ILIKE '%merchant%';

UPDATE subcategories
SET description = 'Nafta, GNC, diésel o carga eléctrica de cualquier vehículo (el vehículo va en la descripción).'
WHERE subcategory = 'Combustible' AND description ILIKE '%merchant%';

-- +goose Down
SELECT 1;
