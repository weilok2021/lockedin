-- name: UpsertFeed :one
INSERT INTO feeds(id, feed_url, title, site_url)
VALUES(gen_random_uuid(), $1, $2, $3)
ON CONFLICT (feed_url) DO UPDATE
SET title = EXCLUDED.title,
    site_url = EXCLUDED.site_url
RETURNING *;

-- name: ListFeeds :many
SELECT * FROM feeds;

-- name: ListCatalog :many
SELECT * FROM feeds ORDER BY category, title;

-- name: ListFollowedFeedIDs :many
SELECT feed_id FROM user_subscriptions WHERE user_id = $1;