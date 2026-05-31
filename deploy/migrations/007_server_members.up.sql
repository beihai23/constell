-- Server membership
CREATE TABLE server_members (
    server_id   UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nickname    TEXT NOT NULL DEFAULT '',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (server_id, user_id)
);

CREATE INDEX idx_server_members_user ON server_members (user_id);
