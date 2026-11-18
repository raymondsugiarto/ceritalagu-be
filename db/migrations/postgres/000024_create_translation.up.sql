-- Translations of customer-facing master-data fields (i18n plan §4.3). The base row's name /
-- description stay the default-locale (id) text and the fallback; this table only holds other
-- locales. AI-facing hints (style_hint, prompt_hint) are never translated.
--   entity_type  master-data kind = table name: recipient_type | occasion | genre | mood
--   entity_id    id of the row in that table
--   locale       e.g. en (id is never stored here)
--   field        name | description
-- (entity_type, entity_id, locale, field) is unique among live rows; the service enforces it (upsert).
CREATE TABLE IF NOT EXISTS translation (
    id           VARCHAR(36) PRIMARY KEY,
    entity_type  VARCHAR(50),
    entity_id    VARCHAR(36),
    locale       VARCHAR(10),
    field        VARCHAR(50),
    value        TEXT,
    created_at   TIMESTAMPTZ,
    updated_at   TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ
);
