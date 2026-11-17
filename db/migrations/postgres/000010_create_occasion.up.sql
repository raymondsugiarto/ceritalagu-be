-- Occasion options for questionnaire step 3. prompt_hint guides tone in the lyrics prompt.
CREATE TABLE IF NOT EXISTS occasion (
    id          VARCHAR(36) PRIMARY KEY,
    code            VARCHAR(50),
    name            VARCHAR(100),
    prompt_hint     TEXT,
    sort_order      INTEGER,
    is_active       BOOLEAN,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
