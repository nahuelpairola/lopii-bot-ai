-- +goose Up
CREATE TABLE invitations (
    id          SERIAL PRIMARY KEY,
    code        TEXT UNIQUE NOT NULL,
    created_by  INTEGER NOT NULL REFERENCES users(id),
    used_by     INTEGER REFERENCES users(id),
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- +goose Down
DROP TABLE invitations;