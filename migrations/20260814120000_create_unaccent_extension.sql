-- +goose Up
-- unaccent es lo que hace que el filtro `search` de las consultas sea
-- transparente a los acentos: "panaderia" tiene que encontrar "panadería" y
-- "alimentacion" tiene que encontrar "Alimentación".
--
-- Va como extensión de Postgres y no como un translate() de 7 runas en Go
-- porque las TRES patas del filtro —categoría, subcategoría y descripción—
-- tienen que foldear con el MISMO diccionario. Dos semánticas distintas dentro
-- del mismo parámetro lo vuelven impredecible, que es justo lo contrario de lo
-- que este cambio busca.
--
-- Efecto colateral conocido y aceptado: movements_description_trgm_idx deja de
-- aplicar a un predicado sobre unaccent(lower(description)), porque el índice
-- es sobre la columna cruda. Con el volumen actual —81 movimientos el usuario
-- más cargado de producción— es irrelevante. Si algún día importa, el arreglo
-- es un índice de expresión, no volver atrás con esto.
CREATE EXTENSION IF NOT EXISTS unaccent;

-- +goose Down
-- No se dropea: otra cosa podría empezar a depender de la extensión, y una
-- extensión de más no rompe nada. Dropearla sí rompería toda consulta que la use.
SELECT 1;
