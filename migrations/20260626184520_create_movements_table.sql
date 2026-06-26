-- +goose Up
CREATE TYPE movement_type AS ENUM ('expense', 'income', 'transfer');
CREATE TYPE currency_type AS ENUM ('ARS', 'USD');
CREATE TABLE movements (
    id             SERIAL PRIMARY KEY,
    transaction_id UUID,
    user_id        INTEGER NOT NULL REFERENCES users(id),
    account_id     INTEGER REFERENCES accounts(id),
    subcategory_id INTEGER NOT NULL REFERENCES subcategories(id),
    date           DATE NOT NULL ,
    type           movement_type NOT NULL,
    amount         NUMERIC(15,2) NOT NULL,
    currency       currency_type NOT NULL,
    payment_method TEXT,
    merchant       TEXT,
    description    TEXT,
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    updated_at     TIMESTAMPTZ DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ
);

-- +goose Down
DROP TABLE movements;
DROP TYPE movement_type;
DROP TYPE currency_type;
