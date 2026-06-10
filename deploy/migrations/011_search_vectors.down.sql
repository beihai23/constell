DROP INDEX IF EXISTS idx_dm_messages_search;
ALTER TABLE dm_messages DROP COLUMN IF EXISTS search_vector;
DROP INDEX IF EXISTS idx_channel_messages_search;
ALTER TABLE channel_messages DROP COLUMN IF EXISTS search_vector;
DROP INDEX IF EXISTS idx_users_search;
ALTER TABLE users DROP COLUMN IF EXISTS search_vector;
