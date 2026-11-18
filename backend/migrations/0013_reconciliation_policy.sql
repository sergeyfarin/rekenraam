-- +goose Up
-- #482: the book's reconciliation out-of-date window. One setting read by the
-- account list, Home's Reconciliation widget and any other place that judges
-- reconciliation age. No row means the default of 60 days.
CREATE TABLE reconciliation_policies (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT UNIQUE,
  out_of_date_after_days INTEGER NOT NULL DEFAULT 60 CHECK (out_of_date_after_days BETWEEN 1 AND 3650),
  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

-- +goose Down
DROP TABLE reconciliation_policies;
