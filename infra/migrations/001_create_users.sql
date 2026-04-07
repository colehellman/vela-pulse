-- +goose Up
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    apple_sub  TEXT        NOT NULL UNIQUE,  -- SIWA subject claim (stable across sessions)
    email      TEXT,                          -- optional; Apple may withhold after first auth
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_apple_sub ON users(apple_sub);

-- +goose Down
DROP TABLE IF EXISTS users;
DROP EXTENSION IF EXISTS "pgcrypto";
