-- name: InsertItem :exec
INSERT INTO items(id, feed_id, guid, url, title, summary, author, published_at)
VALUES(gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7)
ON CONFLICT(feed_id, guid)
DO NOTHING;

-- name: ListItemsForUser :many
SELECT i.id, i.title, i.url, i.summary, i.image_url, i.published_at,
         f.title AS source_title, f.source_type
FROM user_subscriptions AS u
INNER JOIN feeds AS f ON f.id = u.feed_id
INNER JOIN items AS i ON i.feed_id = f.id
WHERE u.user_id = $1
ORDER BY i.fetched_at DESC
LIMIT $2 OFFSET $3;

-- name: CountItemsForUser :one
-- Total items across everything this user follows. Drives page count + range.
-- No feeds join needed: we only count, and feed_id lives on both tables.
SELECT COUNT(*)
FROM user_subscriptions AS u
INNER JOIN items AS i ON i.feed_id = u.feed_id
WHERE u.user_id = $1;