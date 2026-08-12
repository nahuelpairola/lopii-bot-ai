-- +goose Up
-- Release 1 del fold de merchant: sembrar description y dejar de usar la
-- columna. El DROP va en otra release, después de mirar la data — el merge es
-- heurístico, de una sola dirección, y las migraciones corren SOLAS al arrancar
-- el server.
--
-- La rama del medio importa más de lo que parece. Medido contra la base real el
-- 2026-08-11, sobre 254 movimientos: 164 no tienen merchant, 82 YA lo tienen
-- adentro de la description ("Pago en Polleria" / "Polleria"), y sólo 8 se
-- concatenan. Sin esa rama, esas 82 filas quedarían como "comida en el chino -
-- el chino".
--
-- Los acentos no se pliegan: unaccent no está instalado, y el peor caso es una
-- palabra repetida en una description, no un número mal.
UPDATE movements
SET description = CASE
  WHEN description IS NULL OR btrim(description) = ''        THEN merchant
  WHEN lower(description) LIKE '%' || lower(merchant) || '%' THEN description
  ELSE description || ' - ' || merchant
END
WHERE merchant IS NOT NULL AND btrim(merchant) <> '';

-- +goose Down
-- Irreversible por diseño: la fusión pierde la frontera entre los dos campos.
-- El rollback es restaurar de un backup, y por eso el DROP COLUMN va aparte.
SELECT 1;
