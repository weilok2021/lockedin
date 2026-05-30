-- name: UpsertFeed :one
INSERT INTO feeds(id, feed_url, title, site_url)
VALUES(gen_random_uuid(), $1, $2, $3)
ON CONFLICT (feed_url) DO UPDATE
SET title = EXCLUDED.title,
    site_url = EXCLUDED.site_url
RETURNING *;

