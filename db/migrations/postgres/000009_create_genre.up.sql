-- Genre options for questionnaire step 4. style_hint is added to the lyrics/style prompt.
CREATE TABLE IF NOT EXISTS genre (
    id          VARCHAR(36) PRIMARY KEY,
    code                VARCHAR(50),
    name                VARCHAR(100),
    description         TEXT,
    icon                VARCHAR(100),
    sample_audio_key    VARCHAR(500),
    style_hint          TEXT,
    sort_order          INTEGER,
    is_active           BOOLEAN,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
