-- name: InsertItem :exec
INSERT INTO items(id, feed_id, guid, url, title, summary, author, published_at)
VALUES(gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7)
ON CONFLICT(feed_id, guid)
DO NOTHING;