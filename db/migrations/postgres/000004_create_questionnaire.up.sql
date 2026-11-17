-- Answers to the 7-step questionnaire (draft until submitted_at is set).
-- answers JSONB: recipient_type, recipient_name, recipient_nickname, occasion, genre, voice, special,
-- memory, message, language, mood. Owner is guest_session_id and/or user_id.
CREATE TABLE IF NOT EXISTS questionnaire (
    id          VARCHAR(36) PRIMARY KEY,
    guest_session_id    VARCHAR(36),
    user_id             VARCHAR(36),
    answers             JSONB,
    submitted_at        TIMESTAMPTZ,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
