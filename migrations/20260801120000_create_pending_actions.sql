-- +goose Up
CREATE TABLE pending_actions (
    id         BIGSERIAL   PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id),
    tool       TEXT        NOT NULL,
    payload    JSONB       NOT NULL,
    questions  JSONB       NOT NULL,
    budget     INT         NOT NULL,
    position   INT         NOT NULL,
    trace_id   TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Toda lectura es "la próxima acción de este usuario": el índice es ese orden.
CREATE INDEX idx_pending_actions_user_position ON pending_actions (user_id, position, id);

-- +goose Down
DROP TABLE pending_actions;
