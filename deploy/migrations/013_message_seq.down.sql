DROP INDEX IF EXISTS idx_channel_messages_channel_seq;
DROP INDEX IF EXISTS idx_dm_messages_conv_seq;
ALTER TABLE channel_messages DROP COLUMN IF EXISTS seq;
ALTER TABLE dm_messages DROP COLUMN IF EXISTS seq;
