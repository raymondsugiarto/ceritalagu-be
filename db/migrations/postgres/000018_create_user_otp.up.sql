-- One-time login code sent by email. code_hash: HMAC-SHA256 of the 6-digit code, never the code itself.
-- purpose: login. attempts: wrong guesses so far (locked at 5). consumed_at: set once it was used.
CREATE TABLE IF NOT EXISTS user_otp (
    id          VARCHAR(36) PRIMARY KEY,
    user_id             VARCHAR(36),
    email               VARCHAR(255),
    purpose             VARCHAR(20),
    code_hash           VARCHAR(64),
    attempts            INTEGER,
    expires_at          TIMESTAMPTZ,
    consumed_at         TIMESTAMPTZ,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
