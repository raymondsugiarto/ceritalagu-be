-- Recipient options for questionnaire step 1 (partner, mother, friend, ...). prompt_hint sets point of view.
CREATE TABLE IF NOT EXISTS recipient_type (
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
