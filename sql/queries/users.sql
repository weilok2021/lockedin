-- name: CreateUser :one
INSERT INTO users(id, email, password_hash)
VALUES (gen_random_uuid(), $1, $2)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users 
WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users 
WHERE id = $1;

-- name: UpdatePassword :exec
UPDATE users 
SET password_hash = $1, updated_at = NOW()
WHERE id = $2;

-- name: SetEmailVerified :exec
UPDATE users
SET email_verified_at = NOW(), updated_at = NOW()
WHERE id = $1;
