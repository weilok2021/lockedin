-- +goose Up
ALTER TABLE feeds
ADD COLUMN curated BOOLEAN NOT NULL DEFAULT FALSE; 

-- +goose Down
ALTER TABLE feeds
DROP COLUMN curated;