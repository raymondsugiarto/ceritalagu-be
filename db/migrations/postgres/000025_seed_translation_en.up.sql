-- Seeds English names for the master-data rows that exist today (i18n plan §4.9), matched by code.
-- Insert-only and idempotent: a row is added only when that (entity, locale, field) has no live
-- translation yet, so admin edits are never overwritten. Codes and base (id) names are untouched.
-- Rows added later without a translation fall back to the base name until an admin fills English.
-- No down file: undoing a seed is row removal, which the project owner writes (docs/database-standards.md §4.2).
INSERT INTO translation (id, entity_type, entity_id, locale, field, value, created_at, updated_at)
SELECT gen_random_uuid()::text, v.entity_type, t.id, 'en', 'name', v.name, NOW(), NOW()
FROM (VALUES
    ('recipient_type', 'partner',  'Partner'),
    ('recipient_type', 'spouse',   'Husband/Wife'),
    ('recipient_type', 'mother',   'Mom'),
    ('recipient_type', 'father',   'Dad'),
    ('recipient_type', 'child',    'Child'),
    ('recipient_type', 'friend',   'Best Friend'),
    ('recipient_type', 'sibling',  'Sibling'),
    ('recipient_type', 'coworker', 'Coworker'),
    ('recipient_type', 'teacher',  'Teacher'),
    ('recipient_type', 'self',     'Myself')
) AS v(entity_type, code, name)
JOIN recipient_type t ON t.code = v.code AND t.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM translation x
    WHERE x.entity_type = v.entity_type AND x.entity_id = t.id AND x.locale = 'en'
      AND x.field = 'name' AND x.deleted_at IS NULL
);

INSERT INTO translation (id, entity_type, entity_id, locale, field, value, created_at, updated_at)
SELECT gen_random_uuid()::text, v.entity_type, t.id, 'en', 'name', v.name, NOW(), NOW()
FROM (VALUES
    ('occasion', 'birthday',     'Birthday'),
    ('occasion', 'anniversary',  'Anniversary'),
    ('occasion', 'wedding',      'Wedding'),
    ('occasion', 'proposal',     'Proposal'),
    ('occasion', 'parents_day',  'Mother’s/Father’s Day'),
    ('occasion', 'graduation',   'Graduation'),
    ('occasion', 'apology',      'Apology'),
    ('occasion', 'thank_you',    'Thank You'),
    ('occasion', 'farewell',     'Farewell'),
    ('occasion', 'valentine',    'Valentine’s Day'),
    ('occasion', 'just_because', 'Just Because I Love You')
) AS v(entity_type, code, name)
JOIN occasion t ON t.code = v.code AND t.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM translation x
    WHERE x.entity_type = v.entity_type AND x.entity_id = t.id AND x.locale = 'en'
      AND x.field = 'name' AND x.deleted_at IS NULL
);

INSERT INTO translation (id, entity_type, entity_id, locale, field, value, created_at, updated_at)
SELECT gen_random_uuid()::text, v.entity_type, t.id, 'en', 'name', v.name, NOW(), NOW()
FROM (VALUES
    ('genre', 'pop',       'Pop'),
    ('genre', 'ballad',    'Ballad'),
    ('genre', 'acoustic',  'Acoustic'),
    ('genre', 'rnb',       'R&B'),
    ('genre', 'rock',      'Rock'),
    ('genre', 'jazz',      'Jazz'),
    ('genre', 'dangdut',   'Dangdut'),
    ('genre', 'keroncong', 'Keroncong'),
    ('genre', 'edm',       'EDM'),
    ('genre', 'hiphop',    'Hip-Hop'),
    ('genre', 'country',   'Country'),
    ('genre', 'lullaby',   'Lullaby')
) AS v(entity_type, code, name)
JOIN genre t ON t.code = v.code AND t.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM translation x
    WHERE x.entity_type = v.entity_type AND x.entity_id = t.id AND x.locale = 'en'
      AND x.field = 'name' AND x.deleted_at IS NULL
);

INSERT INTO translation (id, entity_type, entity_id, locale, field, value, created_at, updated_at)
SELECT gen_random_uuid()::text, v.entity_type, t.id, 'en', 'name', v.name, NOW(), NOW()
FROM (VALUES
    ('mood', 'romantic',  'Romantic'),
    ('mood', 'cheerful',  'Cheerful'),
    ('mood', 'touching',  'Touching'),
    ('mood', 'funny',     'Funny'),
    ('mood', 'upbeat',    'Upbeat'),
    ('mood', 'warm',      'Warm'),
    ('mood', 'inspiring', 'Inspiring')
) AS v(entity_type, code, name)
JOIN mood t ON t.code = v.code AND t.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM translation x
    WHERE x.entity_type = v.entity_type AND x.entity_id = t.id AND x.locale = 'en'
      AND x.field = 'name' AND x.deleted_at IS NULL
);
