-- AI provider configured by an admin. category: lyrics_style | music; one active per category.
-- adapter_type picks the Go adapter (minimax_text, minimax_music, ...). api_key_encrypted: AES-GCM ciphertext,
-- api_key_last4 for the masked display. params: model defaults (temperature, audio format, ...).
CREATE TABLE IF NOT EXISTS ai_provider (
    id          VARCHAR(36) PRIMARY KEY,
    category            VARCHAR(30),
    adapter_type        VARCHAR(50),
    name                VARCHAR(100),
    base_url            VARCHAR(500),
    model               VARCHAR(100),
    api_key_encrypted   TEXT,
    api_key_last4       VARCHAR(4),
    params              JSONB,
    timeout_sec         INTEGER,
    max_retries         INTEGER,
    is_active           BOOLEAN,
    is_fallback         BOOLEAN,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
