-- +goose Up
CREATE TABLE email_tokens (
  token         text PRIMARY KEY,
  user_id       uuid REFERENCES users(id) NOT NULL,
  purpose       text NOT NULL,                 -- 'verify' | 'password_reset'
  expires_at    timestamptz NOT NULL,
  used_at       timestamptz
);

-- +goose Down
DROP TABLE email_tokens;