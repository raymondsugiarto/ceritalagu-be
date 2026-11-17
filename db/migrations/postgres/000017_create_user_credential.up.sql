-- Customer password, kept apart from "user". A user made by the questionnaire contact step has no row
-- here yet: they sign in the first time with an email OTP and then set a password, which inserts it.
-- One live row per user_id (checked in the service). password_hash: bcrypt.
CREATE TABLE IF NOT EXISTS user_credential (
    id          VARCHAR(36) PRIMARY KEY,
    user_id             VARCHAR(36),
    password_hash       VARCHAR(255),
    password_set_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
