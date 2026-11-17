-- Back-office account, separate from customers.
-- role: super_admin (providers, API keys, prompts) | operator (jobs, moderation). status: active | suspended.
CREATE TABLE IF NOT EXISTS admin_user (
    id          VARCHAR(36) PRIMARY KEY,
    name            VARCHAR(255),
    email           VARCHAR(255),
    password_hash   VARCHAR(255),
    role            VARCHAR(30),
    status          VARCHAR(20),
    last_login_at   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
