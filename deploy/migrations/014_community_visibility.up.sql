-- Community discovery: visibility flag + full-text search vector.
ALTER TABLE communities ADD COLUMN is_public BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE communities ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (
    to_tsvector('simple', coalesce(name, '') || ' ' || coalesce(description, ''))
  ) STORED;

CREATE INDEX idx_communities_search ON communities USING GIN (search_vector);
CREATE INDEX idx_communities_public ON communities (is_public);
