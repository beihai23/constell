-- Monotonic per-table sequence for backfill ("messages newer than seq").
-- BIGINT GENERATED ALWAYS AS IDENTITY backfills existing rows in physical
-- (≈ insertion) order, so existing history gets distinct ascending values.
ALTER TABLE dm_messages
    ADD COLUMN seq BIGINT GENERATED ALWAYS AS IDENTITY;

ALTER TABLE channel_messages
    ADD COLUMN seq BIGINT GENERATED ALWAYS AS IDENTITY;

CREATE INDEX idx_dm_messages_conv_seq ON dm_messages (conversation_id, seq);
CREATE INDEX idx_channel_messages_channel_seq ON channel_messages (channel_id, seq);
