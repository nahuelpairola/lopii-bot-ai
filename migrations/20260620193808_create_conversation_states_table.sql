-- +goose Up
CREATE TABLE conversation_states (
    user_id     INTEGER PRIMARY KEY REFERENCES users(id),
    flow_name   TEXT NOT NULL,
    step_name   TEXT NOT NULL,
    data        JSONB NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- +goose Down
DROP TABLE conversation_states;