-- song_alternate: the second take of a music request (Suno returns two per request). Its audio is
-- already in object storage under the future song's folder (song_id is minted up front), so
-- "Buat Ulang" can turn it into a song without calling the music AI again.
-- song_job_id: the job whose request made the take. root_job_id: the original job of that
-- regenerate chain (spares are shared within a chain). used_by_job_id / used_at: the regenerate
-- job that took it; NULL = still free.
CREATE TABLE IF NOT EXISTS song_alternate (
    id          VARCHAR(36) PRIMARY KEY,
    song_job_id             VARCHAR(36),
    root_job_id             VARCHAR(36),
    song_id                 VARCHAR(36),
    full_audio_key          VARCHAR(500),
    preview_audio_key       VARCHAR(500),
    duration_sec            INTEGER,
    preview_duration_sec    INTEGER,
    used_by_job_id          VARCHAR(36),
    used_at                 TIMESTAMPTZ,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);

-- reveal_at: a "Buat Ulang" served from a spare is finished at once, but the loading screen still
-- shows the usual steps until this time (the API derives the visible status from it).
ALTER TABLE song_job ADD COLUMN IF NOT EXISTS reveal_at TIMESTAMPTZ;
