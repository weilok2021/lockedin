-- +goose Up
CREATE TABLE sessions (
  token         text PRIMARY KEY,
  user_id       uuid REFERENCES users(id) NOT NULL,
  expires_at    timestamptz NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE sessions; 