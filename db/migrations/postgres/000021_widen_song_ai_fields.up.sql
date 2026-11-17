-- mood and language are written by the lyrics model as free text; 30/10 chars was too tight
-- ("save song: insert song: value too long for type character varying(30)").
ALTER TABLE song ALTER COLUMN mood TYPE VARCHAR(255);
ALTER TABLE song ALTER COLUMN language TYPE VARCHAR(50);
