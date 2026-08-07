-- +goose Up
-- Formato largo: la API devuelve 7 tipos de cambio y suben más con el tiempo.
-- Una fila por (date, rate_type) copia la respuesta 1:1 y no necesita migración
-- para un tipo nuevo; el formato ancho serían 14 columnas más un mapeo
-- tipo->columna.
--
-- rate_type es el `casa` de la API (oficial, blue, bolsa, tarjeta, ...).
-- bid/ask son su `compra`/`venta`: la casa te COMPRA los dólares al bid y te
-- los VENDE al ask, así que bid <= ask siempre.
CREATE TABLE usd_quotes (
    date      DATE          NOT NULL,
    rate_type TEXT          NOT NULL,
    bid       NUMERIC(18,4),
    ask       NUMERIC(18,4),
    PRIMARY KEY (date, rate_type)
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
