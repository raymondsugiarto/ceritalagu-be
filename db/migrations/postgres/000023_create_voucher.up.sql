-- Discount vouchers for the full-song checkout. code is stored uppercase and is unique per live row
-- (checked in the service). discount_amount: whole IDR off the sale price. quota: how many live orders
-- (paid, or pending inside their payment window) may use it.
CREATE TABLE IF NOT EXISTS voucher (
    id          VARCHAR(36) PRIMARY KEY,
    code            VARCHAR(30),
    discount_amount BIGINT,
    quota           INTEGER,
    expires_at      TIMESTAMPTZ,
    is_active       BOOLEAN,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_voucher_code ON voucher (code);

-- What the customer saw at checkout: normal (strikethrough) and sale price, and the voucher applied.
-- amount stays the total actually charged.
ALTER TABLE payment_order
    ADD COLUMN IF NOT EXISTS normal_price     BIGINT,
    ADD COLUMN IF NOT EXISTS sale_price       BIGINT,
    ADD COLUMN IF NOT EXISTS voucher_id       VARCHAR(36),
    ADD COLUMN IF NOT EXISTS voucher_code     VARCHAR(30),
    ADD COLUMN IF NOT EXISTS voucher_discount BIGINT DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_payment_order_voucher_id ON payment_order (voucher_id);
