-- +goose Up
CREATE TABLE users (
    id                  uuid PRIMARY KEY,
    email               text UNIQUE NOT NULL,
    hashed_password       text NOT NULL,           -- bcrypt or argon2id
    email_verified_at   timestamptz,             -- NULL until verified
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;