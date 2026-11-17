-- Global settings editable by admins, e.g. preview_duration_sec, guest_daily_quota, max_regenerate.
-- setting_key is unique per live row (checked in the service); value is JSONB.
CREATE TABLE IF NOT EXISTS setting (
    id          VARCHAR(36) PRIMARY KEY,
    setting_key     VARCHAR(100),
    value           JSONB,
    description     TEXT,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
