-- Customer account (B2C). Registration is needed to unlock full songs.
-- status: active | suspended. email is stored lower-cased; uniqueness is checked in the service.
CREATE TABLE IF NOT EXISTS "user" (
    id          VARCHAR(36) PRIMARY KEY,
    name                VARCHAR(255),
    email               VARCHAR(255),
    password_hash       VARCHAR(255),
    status              VARCHAR(20),
    email_verified_at   TIMESTAMPTZ,
    last_login_at       TIMESTAMPTZ,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
