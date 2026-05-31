-- Direct messages within a DM conversation
CREATE TABLE dm_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES dm_conversations(id) ON DELETE CASCADE,
    sender_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_dm_messages_conversation ON dm_messages (conversation_id, created_at DESC);
CREATE INDEX idx_dm_messages_sender ON dm_messages (sender_id);
