-- +goose Up

-- The ledger self-check (R3 slice 6, docs/plans/data-portability-plan.md).
--
-- Read-only and diagnostic: it reports and explains, and never repairs. These
-- two tables are the only thing it writes, and they hold no ledger data — a
-- check that could change a balance would be a different, far more dangerous
-- feature.
CREATE TABLE IF NOT EXISTS self_check_runs (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  trigger TEXT NOT NULL CHECK (trigger IN ('manual', 'scheduled')),
  status TEXT NOT NULL CHECK (status IN ('running', 'passed', 'failed')),
  failed_check_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_check_count >= 0),
  started_at TEXT NOT NULL,
  finished_at TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS self_check_runs_book_started_idx
  ON self_check_runs (book_id, started_at DESC, id DESC);

-- One row per check per run. A table rather than a JSON blob on the run,
-- because "which check failed, how often, since when" is a question worth
-- answering with a query — the mistake T-54 records about price derivations.
CREATE TABLE IF NOT EXISTS self_check_results (
  id INTEGER PRIMARY KEY,
  run_id INTEGER NOT NULL REFERENCES self_check_runs(id) ON DELETE CASCADE,
  check_id TEXT NOT NULL CHECK (length(trim(check_id)) > 0),
  status TEXT NOT NULL CHECK (status IN ('passed', 'failed', 'not_applicable')),
  finding_count INTEGER NOT NULL DEFAULT 0 CHECK (finding_count >= 0),
  -- A capped sample of offending identifiers, for a human to go and look at.
  -- Diagnostic display only: never joined on, never counted from.
  sample_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(sample_json)),
  summary TEXT NOT NULL DEFAULT '',
  UNIQUE (run_id, check_id)
);

CREATE INDEX IF NOT EXISTS self_check_results_check_idx
  ON self_check_results (check_id, status);

-- +goose Down

DROP INDEX IF EXISTS self_check_results_check_idx;
DROP TABLE IF EXISTS self_check_results;
DROP INDEX IF EXISTS self_check_runs_book_started_idx;
DROP TABLE IF EXISTS self_check_runs;
