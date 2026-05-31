-- DM conversations between two users
CREATE TABLE dm_conversations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_a_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_b_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (user_a_id, user_b_id),
    CHECK (user_a_id < user_b_id)
);

CREATE INDEX idx_dm_conversations_user ON dm_conversations (user_a_id, user_b_id);
