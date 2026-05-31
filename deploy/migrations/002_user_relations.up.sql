-- Friend and block relationships
CREATE TABLE user_relations (
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type          TEXT NOT NULL CHECK (type IN ('friend', 'blocked')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, target_user_id),
    CHECK (user_id != target_user_id)
);

CREATE INDEX idx_user_relations_target ON user_relations (target_user_id, type);
