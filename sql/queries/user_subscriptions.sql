-- name: CreateUserSubscription :exec
INSERT INTO user_subscriptions (user_id, feed_id, custom_title)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, feed_id) DO NOTHING;

-- name: ListUserSubscriptions :many
SELECT f.id AS feed_id, u.custom_title,  f.last_fetch_status 
FROM user_subscriptions AS u
INNER JOIN feeds AS f
ON u.feed_id = f.id
WHERE u.user_id = $1
ORDER BY u.subscribed_at DESC;

-- name: DeleteUserSubscription :exec
DELETE FROM user_subscriptions
WHERE user_id = $1
AND feed_id = $2;
