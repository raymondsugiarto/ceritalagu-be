-- Customer lifecycle stage (funnel). Only moves forward; set by the service on these events:
--   lead            gave email + phone on the questionnaire contact step (no password yet)
--   prospect        set a password (registered) or started a checkout to unlock a song
--   customer        first paid order
--   repeat_customer two or more paid orders (repeat buyer / advocate)
-- Anonymous visitors (guest) have no user row: they are guest_session rows only.
ALTER TABLE "user" ADD COLUMN IF NOT EXISTS customer_type VARCHAR(20);

UPDATE "user" u SET customer_type = CASE
    WHEN EXISTS (SELECT 1 FROM user_credential c WHERE c.user_id = u.id AND c.deleted_at IS NULL) THEN 'prospect'
    ELSE 'lead' END
WHERE customer_type IS NULL;
