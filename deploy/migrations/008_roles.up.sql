-- Roles within a server
CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id   UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    color       INT NOT NULL DEFAULT 0,
    permissions BIGINT NOT NULL DEFAULT 0,
    position    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Role assignments to members
CREATE TABLE member_roles (
    server_id UUID NOT NULL,
    user_id   UUID NOT NULL,
    role_id   UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,

    PRIMARY KEY (server_id, user_id, role_id),
    FOREIGN KEY (server_id, user_id) REFERENCES server_members(server_id, user_id) ON DELETE CASCADE
);

CREATE INDEX idx_roles_server ON roles (server_id, position);
