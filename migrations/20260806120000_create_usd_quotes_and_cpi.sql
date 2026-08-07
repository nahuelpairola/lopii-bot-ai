-- +goose Up
-- Formato largo: la API devuelve 7 casas y sube más con el tiempo. Una fila
-- por (fecha, casa) copia la respuesta 1:1 y no necesita migración para una
-- casa nueva; el formato ancho serían 14 columnas más un mapeo casa->columna.
CREATE TABLE usd_quotes (
    date   DATE          NOT NULL,
    casa   TEXT          NOT NULL,
    compra NUMERIC(18,4),
    venta  NUMERIC(18,4),
    PRIMARY KEY (date, casa)
);

-- value es la VARIACIÓN mensual en porcentaje (1.9 = 1.9%), no un número
-- índice, y puede ser negativa. Cualquier deflactor es un encadenado de
-- (1 + value/100), nunca un cociente entre dos valores de esta tabla.
CREATE TABLE monthly_cpi (
    month DATE         NOT NULL,  -- primer día del mes: 2026-06-01
    value NUMERIC(8,4) NOT NULL,
    PRIMARY KEY (month)
);

-- +goose Down
DROP TABLE monthly_cpi;
DROP TABLE usd_quotes;
