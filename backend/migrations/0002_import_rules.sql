-- +goose Up
CREATE TABLE import_rules (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  name TEXT NOT NULL CHECK (length(trim(name)) > 0 AND name = trim(name)),
  priority INTEGER NOT NULL DEFAULT 100 CHECK (priority BETWEEN 0 AND 1000000),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  match_field TEXT NOT NULL CHECK (match_field IN ('payee', 'description')),
  contains_text TEXT NOT NULL CHECK (length(trim(contains_text)) > 0 AND contains_text = trim(contains_text)),
  category_id INTEGER REFERENCES accounts(id) ON DELETE RESTRICT,
  payee_id INTEGER REFERENCES payees(id) ON DELETE RESTRICT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX import_rules_book_order_idx
  ON import_rules (book_id, priority ASC, id ASC);

-- +goose StatementBegin
CREATE TRIGGER import_rules_targets_same_book_insert
BEFORE INSERT ON import_rules
BEGIN
  SELECT CASE WHEN NEW.category_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM accounts WHERE id = NEW.category_id AND book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'import rule category must belong to the same book') END;
  SELECT CASE WHEN NEW.payee_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM payees WHERE id = NEW.payee_id AND book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'import rule payee must belong to the same book') END;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER import_rules_targets_same_book_update
BEFORE UPDATE OF book_id, category_id, payee_id ON import_rules
BEGIN
  SELECT CASE WHEN NEW.category_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM accounts WHERE id = NEW.category_id AND book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'import rule category must belong to the same book') END;
  SELECT CASE WHEN NEW.payee_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM payees WHERE id = NEW.payee_id AND book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'import rule payee must belong to the same book') END;
END;
-- +goose StatementEnd

CREATE TABLE import_rule_tags (
  rule_id INTEGER NOT NULL REFERENCES import_rules(id) ON DELETE CASCADE,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE RESTRICT,
  PRIMARY KEY (rule_id, tag_id)
);

CREATE INDEX import_rule_tags_book_tag_idx
  ON import_rule_tags (book_id, tag_id, rule_id);

-- +goose StatementBegin
CREATE TRIGGER import_rule_tags_same_book
BEFORE INSERT ON import_rule_tags
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1
    FROM import_rules rule
    JOIN tags tag ON tag.id = NEW.tag_id
    WHERE rule.id = NEW.rule_id
      AND rule.book_id = NEW.book_id
      AND tag.book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'import rule tag must belong to the same book') END;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS import_rule_tags_same_book;
DROP INDEX IF EXISTS import_rule_tags_book_tag_idx;
DROP TABLE IF EXISTS import_rule_tags;
DROP TRIGGER IF EXISTS import_rules_targets_same_book_update;
DROP TRIGGER IF EXISTS import_rules_targets_same_book_insert;
DROP INDEX IF EXISTS import_rules_book_order_idx;
DROP TABLE IF EXISTS import_rules;
