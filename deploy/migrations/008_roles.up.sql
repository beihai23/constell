-- Roles within a community
CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    color       INT NOT NULL DEFAULT 0,
    permissions BIGINT NOT NULL DEFAULT 0,
    position    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Role assignments to members
CREATE TABLE member_roles (
    community_id UUID NOT NULL,
    user_id   UUID NOT NULL,
    role_id   UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,

    PRIMARY KEY (community_id, user_id, role_id),
    FOREIGN KEY (community_id, user_id) REFERENCES community_members(community_id, user_id) ON DELETE CASCADE
);

CREATE INDEX idx_roles_community ON roles (community_id, position);
