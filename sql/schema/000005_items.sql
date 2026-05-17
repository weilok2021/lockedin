-- +goose Up
CREATE TABLE items (
  id            uuid PRIMARY KEY,
  feed_id       uuid REFERENCES feeds(id) NOT NULL,
  guid          text NOT NULL,                 -- publisher's stable identifier
  url           text NOT NULL,
  title         text NOT NULL,
  content       text,                          -- HTML body / summary from feed
  author        text,
  published_at  timestamptz,
  fetched_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (feed_id, guid)
);

-- +goose Down
DROP TABLE items;