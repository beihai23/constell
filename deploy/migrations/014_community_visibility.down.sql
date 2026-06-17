DROP INDEX IF EXISTS idx_communities_public;
DROP INDEX IF EXISTS idx_communities_search;
ALTER TABLE communities DROP COLUMN IF EXISTS search_vector;
ALTER TABLE communities DROP COLUMN IF EXISTS is_public;
