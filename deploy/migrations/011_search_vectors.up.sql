ALTER TABLE users ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(nickname, ''))) STORED;
CREATE INDEX idx_users_search ON users USING GIN(search_vector);

ALTER TABLE channel_messages ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(content, ''))) STORED;
CREATE INDEX idx_channel_messages_search ON channel_messages USING GIN(search_vector);

ALTER TABLE dm_messages ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', coalesce(content, ''))) STORED;
CREATE INDEX idx_dm_messages_search ON dm_messages USING GIN(search_vector);
