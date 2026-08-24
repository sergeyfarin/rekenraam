-- +goose Up

-- Scheduled backups (R3 slice 4, docs/plans/data-portability-plan.md).
--
-- The policy is stored rather than configured by environment so the owner can
-- change it from the app; where the files go stays an operator decision
-- (BACKUP_DIR), because it is a deployment fact, not a preference.
CREATE TABLE IF NOT EXISTS backup_policies (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL UNIQUE REFERENCES books(id) ON DELETE RESTRICT,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  hour_local INTEGER NOT NULL DEFAULT 3 CHECK (hour_local BETWEEN 0 AND 23),
  minute_local INTEGER NOT NULL DEFAULT 15 CHECK (minute_local BETWEEN 0 AND 59),
  retention_count INTEGER NOT NULL DEFAULT 14 CHECK (retention_count >= 1),
  retention_max_age_days INTEGER CHECK (retention_max_age_days IS NULL OR retention_max_age_days >= 1),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  updated_audit_event_id INTEGER REFERENCES audit_events(id) ON DELETE RESTRICT
);

-- One row per backup occurrence, created in the same transaction as the work
-- item that will execute it.
--
-- occurrence_key is what makes a retry idempotent where the work queue cannot:
-- the queue's uniqueness covers pending and running items only, so once an item
-- completes the same scheduled day could be enqueued again. A scheduled run's
-- key is its local date; a manual run gets its own identity because asking for
-- a backup twice is a reasonable thing to do.
CREATE TABLE IF NOT EXISTS backup_runs (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  trigger TEXT NOT NULL CHECK (trigger IN ('scheduled', 'manual')),
  occurrence_key TEXT NOT NULL CHECK (length(trim(occurrence_key)) > 0),
  status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
  -- Deterministic from the occurrence, not from the clock at execution time, so
  -- a retry writes to the same path instead of scattering near-duplicates.
  target_path TEXT NOT NULL,
  scheduled_for_local_date TEXT CHECK (scheduled_for_local_date IS NULL OR scheduled_for_local_date GLOB '????-??-??'),
  byte_size INTEGER,
  page_count INTEGER,
  verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0, 1)),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  error_summary TEXT NOT NULL DEFAULT '',
  work_item_id INTEGER REFERENCES background_work_items(id) ON DELETE SET NULL,
  requested_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,
  started_at TEXT,
  finished_at TEXT,
  pruned_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (book_id, occurrence_key)
);

CREATE INDEX IF NOT EXISTS backup_runs_book_created_idx
  ON backup_runs (book_id, created_at DESC, id DESC);

-- Pruning walks completed, unpruned runs oldest-first.
CREATE INDEX IF NOT EXISTS backup_runs_retention_idx
  ON backup_runs (book_id, status, pruned_at, created_at, id);

-- +goose Down

DROP INDEX IF EXISTS backup_runs_retention_idx;
DROP INDEX IF EXISTS backup_runs_book_created_idx;
DROP TABLE IF EXISTS backup_runs;
DROP TABLE IF EXISTS backup_policies;
