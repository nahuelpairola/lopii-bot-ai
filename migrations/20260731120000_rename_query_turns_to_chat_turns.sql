-- +goose Up
-- La tabla deja de ser exclusiva de QUERY: desde la etapa 2 del agent loop
-- todos los intents comparten el mismo hilo de conversación, así que el nombre
-- mentía.
--
-- Es un rename, no un rediseño: mismas columnas, mismo TTL, mismos Append y
-- Recent. Las filas son efímeras (se podan por TTL, no se soft-deletean), así
-- que ni siquiera hay datos que preservar más allá de la ventana en curso.
ALTER TABLE query_turns RENAME TO chat_turns;

-- +goose Down
ALTER TABLE chat_turns RENAME TO query_turns;
