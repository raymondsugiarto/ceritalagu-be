-- Anonymous visitor identified by the guest_session_id cookie. Songs made as a guest are
-- claimed by a user on register/login (claimed_by_user_id). ip_hash: salted hash, never the raw IP.
CREATE TABLE IF NOT EXISTS guest_session (
    id          VARCHAR(36) PRIMARY KEY,
    ip_hash             VARCHAR(64),
    user_agent          VARCHAR(500),
    claimed_by_user_id  VARCHAR(36),
    claimed_at          TIMESTAMPTZ,
    last_seen_at        TIMESTAMPTZ,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
