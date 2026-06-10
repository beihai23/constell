CREATE TABLE file_metadata (
    id            UUID PRIMARY KEY,
    uploader_id   UUID NOT NULL REFERENCES users(id),
    filename      VARCHAR(255) NOT NULL,
    content_type  VARCHAR(128) NOT NULL,
    size          BIGINT NOT NULL,
    status        VARCHAR(16) NOT NULL DEFAULT 'uploading',
    bucket        VARCHAR(64) NOT NULL DEFAULT 'constell',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_file_metadata_uploader ON file_metadata(uploader_id, created_at DESC);
