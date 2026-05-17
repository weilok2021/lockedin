-- +goose Up
CREATE TABLE feeds (
  id                  uuid PRIMARY KEY,
  feed_url            text UNIQUE NOT NULL,
  title               text,                    -- from feed <title>
  site_url            text,                    -- from feed <link>
  etag                text,                    -- HTTP If-None-Match
  last_modified       text,                    -- HTTP If-Modified-Since
  last_fetched_at     timestamptz,
  last_fetch_status   text,                    -- 'ok' | 'http_error' | 'parse_error' | 'timeout'
  last_fetch_error    text,
  created_at          timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE feeds;