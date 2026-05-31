-- Servers (guilds) are community containers
CREATE TABLE servers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    icon_url    TEXT NOT NULL DEFAULT '',
    owner_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_servers_owner ON servers (owner_id);
