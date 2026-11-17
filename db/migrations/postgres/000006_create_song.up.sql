-- Result of a completed song_job. Audio lives in private object storage:
-- full_audio_key (never exposed without entitlement) and preview_audio_key (separate 60 s file with fade-out).
-- expires_at: guest retention (30 days) while owner_user_id is empty.
CREATE TABLE IF NOT EXISTS song (
    id          VARCHAR(36) PRIMARY KEY,
    song_job_id             VARCHAR(36),
    questionnaire_id        VARCHAR(36),
    owner_user_id           VARCHAR(36),
    guest_session_id        VARCHAR(36),
    title                   VARCHAR(255),
    lyrics                  TEXT,
    style_prompt            TEXT,
    vocal_gender            VARCHAR(20),
    language                VARCHAR(10),
    mood                    VARCHAR(30),
    genre_code              VARCHAR(50),
    full_audio_key          VARCHAR(500),
    preview_audio_key       VARCHAR(500),
    duration_sec            INTEGER,
    preview_duration_sec    INTEGER,
    is_unlocked             BOOLEAN,
    unlocked_at             TIMESTAMPTZ,
    expires_at              TIMESTAMPTZ,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
