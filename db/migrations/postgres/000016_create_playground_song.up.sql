-- A full song an admin generated in the prompt playground. Audio lives at playground/<id>.<format>.
-- show_on_landing: the admin picked it as a sample/testimonial for the landing page; landing_order
-- sorts those (smallest first). caption: short line shown under the sample (e.g. who it was for).
CREATE TABLE IF NOT EXISTS playground_song (
    id          VARCHAR(36) PRIMARY KEY,
    title               VARCHAR(255),
    caption             VARCHAR(255),
    lyrics              TEXT,
    style_prompt        TEXT,
    audio_key           VARCHAR(500),
    format              VARCHAR(10),
    duration_sec        INTEGER,
    size_bytes          INTEGER,
    provider            VARCHAR(255),
    model               VARCHAR(255),
    show_on_landing     BOOLEAN,
    landing_order       INTEGER,
    created_by_admin_id VARCHAR(36),
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
