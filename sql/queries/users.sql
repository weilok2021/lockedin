-- name: CreateUser :one
INSERT INTO users(id, email, password_hash, email_verified_at, created_at, updated_at)
VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())
RETURNING *;