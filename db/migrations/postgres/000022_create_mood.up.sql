-- Mood options for questionnaire step 4 (optional). prompt_hint guides tone in the lyrics prompt.
-- Seeds the five moods that were hardcoded in the frontend, plus "Hangat" and "Inspiratif".
CREATE TABLE IF NOT EXISTS mood (
    id          VARCHAR(36) PRIMARY KEY,
    code            VARCHAR(50),
    name            VARCHAR(100),
    prompt_hint     TEXT,
    sort_order      INTEGER,
    is_active       BOOLEAN,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ,
    deleted_at  TIMESTAMPTZ
);

INSERT INTO mood (id, code, name, prompt_hint, sort_order, is_active, created_at, updated_at)
SELECT gen_random_uuid()::text, v.code, v.name, v.hint, v.sort_order, TRUE, NOW(), NOW()
FROM (VALUES
    ('romantic',  'Romantis',   'Romantic: tender, intimate, full of affection and longing.', 10),
    ('cheerful',  'Ceria',      'Cheerful: bright, playful and happy, light-hearted imagery.', 20),
    ('touching',  'Haru',       'Touching: emotional and moving, gentle build that can bring tears.', 30),
    ('funny',     'Lucu',       'Funny: witty and playful, light teasing and humorous details, still affectionate.', 40),
    ('upbeat',    'Semangat',   'Upbeat: energetic and motivating, driving rhythm, positive drive.', 50),
    ('warm',      'Hangat',     'Warm: cozy and comforting, sincere gratitude and closeness, like a heartfelt hug. Mid tempo, soft and intimate.', 60),
    ('inspiring', 'Inspiratif', 'Inspiring: uplifting and hopeful, celebrates their strength and growth, encourages chasing dreams. Builds to an anthemic chorus.', 70)
) AS v(code, name, hint, sort_order)
WHERE NOT EXISTS (SELECT 1 FROM mood m WHERE m.code = v.code);
