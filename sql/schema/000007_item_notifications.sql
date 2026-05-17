-- +goose Up
CREATE TABLE item_notifications (
  user_id       uuid REFERENCES users(id) NOT NULL,
  item_id       uuid REFERENCES items(id) NOT NULL,
  notified_at   timestamptz NOT NULL,
  PRIMARY KEY (user_id, item_id)
);

-- +goose Down
DROP TABLE item_notifications;