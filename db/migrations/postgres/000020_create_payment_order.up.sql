-- One checkout to unlock a song's full version, paid through a payment gateway (DOKU Checkout).
-- status: pending | paid | failed | expired. invoice_number is what DOKU knows the order by (unique,
-- checked in the service). amount: whole IDR. payment_url: the DOKU page the customer is sent to.
-- gateway_response / notification_payload: raw JSON from DOKU, kept for support and reconciliation.
CREATE TABLE IF NOT EXISTS payment_order (
    id          VARCHAR(36) PRIMARY KEY,
    invoice_number          VARCHAR(64),
    user_id                 VARCHAR(36),
    song_id                 VARCHAR(36),
    guest_session_id        VARCHAR(36),
    gateway                 VARCHAR(20),
    amount                  BIGINT,
    currency                VARCHAR(3),
    status                  VARCHAR(20),
    payment_url             TEXT,
    payment_method          VARCHAR(50),
    gateway_request_id      VARCHAR(128),
    gateway_response        JSONB,
    notification_payload    JSONB,
    expires_at              TIMESTAMPTZ,
    paid_at                 TIMESTAMPTZ,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
