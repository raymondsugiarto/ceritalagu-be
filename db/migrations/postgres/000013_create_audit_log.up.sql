-- Who changed what in the back office (providers, API keys, prompts, settings).
-- actor_type: admin | user | system. diff: before/after JSON with secrets masked.
CREATE TABLE IF NOT EXISTS audit_log (
    id          VARCHAR(36) PRIMARY KEY,
    actor_type      VARCHAR(20),
    actor_id        VARCHAR(36),
    action          VARCHAR(50),
    entity_type     VARCHAR(50),
    entity_id       VARCHAR(36),
    diff            JSONB,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
