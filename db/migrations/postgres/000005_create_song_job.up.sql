-- One generation run: lyrics/style AI -> music AI -> preview trim.
-- status: queued | generating_lyrics | generating_music | processing_audio | completed | failed.
-- *_provider_snapshot: provider config (without API key) frozen at creation. timings: per-step durations (ms).
-- regenerate_of_job_id: set when the user pressed "Buat Ulang".
CREATE TABLE IF NOT EXISTS song_job (
    id          VARCHAR(36) PRIMARY KEY,
    questionnaire_id            VARCHAR(36),
    guest_session_id            VARCHAR(36),
    user_id                     VARCHAR(36),
    regenerate_of_job_id        VARCHAR(36),
    status                      VARCHAR(30),
    attempt                     INTEGER,
    lyrics_provider_snapshot    JSONB,
    music_provider_snapshot     JSONB,
    prompt_template_id          VARCHAR(36),
    lyrics_style_output         JSONB,
    error_code                  VARCHAR(50),
    error_message               TEXT,
    timings                     JSONB,
    cost_estimate_usd           NUMERIC(12, 4),
    started_at                  TIMESTAMPTZ,
    finished_at                 TIMESTAMPTZ,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
