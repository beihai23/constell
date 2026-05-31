-- Constell users table
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email       TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    nickname    TEXT NOT NULL,
    avatar_url  TEXT NOT NULL DEFAULT '',
    status_message TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_email ON users (email);
CREATE INDEX idx_users_nickname ON users (nickname);
