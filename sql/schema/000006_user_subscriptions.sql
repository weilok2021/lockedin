-- +goose Up
CREATE TABLE user_subscriptions (
  user_id          uuid REFERENCES users(id) ON DELETE CASCADE NOT NULL,
  feed_id          uuid REFERENCES feeds(id) ON DELETE CASCADE NOT NULL,
  custom_title     text,                       -- optional override
  subscribed_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, feed_id)
);


-- +goose Down
DROP TABLE user_subscriptions;