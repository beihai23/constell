-- Per-user read cursors anchored to message seq (source of truth for unread).
-- unread = count(messages WHERE seq > last_read_seq); has_unread = max(seq) > last_read_seq.
-- Mark-read upserts last_read_seq = GREATEST(existing, max(seq)). Monotonic, idempotent,
-- and always reconcilable against the real message tables.
CREATE TABLE channel_read_state (
    user_id       UUID    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id    UUID    NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    last_read_seq BIGINT  NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, channel_id)
);

CREATE TABLE dm_read_state (
    user_id        UUID    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id UUID   NOT NULL REFERENCES dm_conversations(id) ON DELETE CASCADE,
    last_read_seq  BIGINT  NOT NULL DEFAULT 0,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, conversation_id)
);
