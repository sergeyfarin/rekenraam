-- +goose Up
CREATE TABLE home_layouts (
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  owner_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  revision INTEGER NOT NULL CHECK (revision > 0),
  document_json TEXT NOT NULL CHECK (json_valid(document_json) AND json_type(document_json) = 'object'),
  PRIMARY KEY (book_id, owner_user_id)
) WITHOUT ROWID;

-- +goose Down
DROP TABLE home_layouts;
