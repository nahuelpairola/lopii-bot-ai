-- +goose Up
CREATE TABLE pending_llm_jobs (
    id         BIGSERIAL   PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id),
    kind       TEXT        NOT NULL,
    payload    JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_pending_llm_jobs_user_created ON pending_llm_jobs (user_id, created_at);

-- +goose Down
DROP TABLE pending_llm_jobs;
