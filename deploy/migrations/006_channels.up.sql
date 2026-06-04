-- Channels within a community
CREATE TABLE channels (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    topic       TEXT NOT NULL DEFAULT '',
    type        TEXT NOT NULL DEFAULT 'text' CHECK (type IN ('text', 'announcement')),
    position    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_channels_community ON channels (community_id, position);
