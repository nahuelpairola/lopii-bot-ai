-- +goose Up
-- Los argumentos de las tool calls. Sin esto llm_calls sabe QUÉ tool eligió el
-- modelo pero nunca CON QUÉ, y el eval de la etapa 5 promete medir "selección y
-- argumentos": la mitad de argumentos tendría que volver a casos inventados a
-- mano, que es justo lo que el corpus real viene a reemplazar.
--
-- Sin redacción: los argumentos salen del mensaje del propio usuario, que
-- intent_events.raw_message ya guarda textual.
ALTER TABLE llm_calls ADD COLUMN tool_calls JSONB;

-- +goose Down
ALTER TABLE llm_calls DROP COLUMN tool_calls;
