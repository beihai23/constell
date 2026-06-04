-- Community membership
CREATE TABLE community_members (
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nickname    TEXT NOT NULL DEFAULT '',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (community_id, user_id)
);

CREATE INDEX idx_community_members_user ON community_members (user_id);
