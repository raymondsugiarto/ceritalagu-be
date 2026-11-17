-- Versioned prompts for the lyrics & style AI. Placeholders like {{name}}, {{occasion}}, {{memory}}.
-- Exactly one is_active; every song_job records the prompt_template_id it used.
CREATE TABLE IF NOT EXISTS prompt_template (
    id          VARCHAR(36) PRIMARY KEY,
    name                    VARCHAR(100),
    version                 INTEGER,
    system_prompt           TEXT,
    user_prompt             TEXT,
    note                    TEXT,
    is_active               BOOLEAN,
    created_by_admin_id     VARCHAR(36),
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
