-- +goose Up
ALTER TABLE feeds
ADD COLUMN source_type TEXT NOT NULL DEFAULT 'article' CHECK (source_type IN ('article','youtube','podcast')),
ADD COLUMN category TEXT,
ADD COLUMN description TEXT;

ALTER TABLE items RENAME COLUMN content TO summary;
ALTER TABLE items ADD COLUMN image_url TEXT;

-- +goose Down
ALTER TABLE feeds
DROP COLUMN source_type, 
DROP COLUMN category, 
DROP COLUMN description;

ALTER TABLE items DROP COLUMN image_url;
ALTER TABLE items RENAME COLUMN summary TO content;