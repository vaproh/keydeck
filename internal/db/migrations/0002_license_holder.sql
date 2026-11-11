ALTER TABLE licenses
    ADD COLUMN IF NOT EXISTS holder_name TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS licenses_holder_idx ON licenses (lower(holder_name));
