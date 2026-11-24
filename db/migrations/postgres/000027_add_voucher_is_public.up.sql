-- is_public: the admin promotes one voucher to the public site. The landing page banner reads it
-- and shows its code and discount, so a customer can use it at checkout. NULL (older rows) means
-- the voucher is not promoted.
ALTER TABLE voucher
    ADD COLUMN IF NOT EXISTS is_public BOOLEAN;
