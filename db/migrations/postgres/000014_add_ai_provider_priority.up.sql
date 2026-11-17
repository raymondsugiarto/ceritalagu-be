-- priority orders the active providers of a category: the smallest number is used first, and
-- providers sharing that number are picked at random per job. Rows from before this column read as
-- NULL and the app treats them as priority 1 (no DEFAULT, see docs/database-standards.md).
ALTER TABLE ai_provider ADD COLUMN IF NOT EXISTS priority INTEGER;
