CREATE TABLE attachments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_type  VARCHAR(16) NOT NULL,
    message_id    UUID NOT NULL,
    file_id       UUID NOT NULL REFERENCES file_metadata(id),
    filename      VARCHAR(255) NOT NULL,
    content_type  VARCHAR(128) NOT NULL,
    size          BIGINT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_attachments_message ON attachments(message_type, message_id);
CREATE INDEX idx_attachments_file ON attachments(file_id);
