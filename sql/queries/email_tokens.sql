-- name: CreateEmailToken :one
INSERT INTO email_tokens(token, user_id, purpose, expires_at)
VALUES($1, $2, $3, $4)
RETURNING *;

-- name: ConsumeEmailToken :one
UPDATE email_tokens
SET used_at = NOW()
WHERE token = $1 
    AND used_at IS NULL 
    AND expires_at > NOW()
    AND purpose = $2
RETURNING user_id; 

