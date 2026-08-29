-- +goose Up

-- R9 recurring transactions (docs/plans/recurring-transactions-plan.md).
--
-- A template is a transaction the user has not typed yet. Its postings mirror
-- posting_versions minus everything only a real posting has — no journal
-- entry, no reconciliation state — because a template is a shape, not a fact.
CREATE TABLE recurring_templates (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  name TEXT NOT NULL CHECK (length(trim(name)) > 0 AND name = trim(name)),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  archived_at TEXT,

  -- Investment kinds are deliberately absent: a recurring buy needs a lot, a
  -- price, and a fee model on the generation date, and a wrong lot is far
  -- worse than a wrong expense. See the plan's out-of-scope list.
  transaction_kind TEXT NOT NULL CHECK (transaction_kind IN ('ordinary', 'transfer')),
  payee_id INTEGER REFERENCES payees(id) ON DELETE RESTRICT,
  payee_name TEXT,
  description TEXT NOT NULL DEFAULT '',
  note_markdown TEXT NOT NULL DEFAULT '',

  frequency TEXT NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly', 'yearly')),
  interval_count INTEGER NOT NULL DEFAULT 1 CHECK (interval_count >= 1),
  by_weekday INTEGER CHECK (by_weekday IS NULL OR by_weekday BETWEEN 0 AND 6),
  day_of_month INTEGER CHECK (day_of_month IS NULL OR day_of_month BETWEEN 1 AND 31),
  last_day_of_month INTEGER NOT NULL DEFAULT 0 CHECK (last_day_of_month IN (0, 1)),
  month_of_year INTEGER CHECK (month_of_year IS NULL OR month_of_year BETWEEN 1 AND 12),

  -- The phase anchor, not an instruction to create history: generate_from is
  -- what bounds generation, and it is set to max(starts_on, today) when the
  -- template is created. A past starts_on therefore lands the schedule on the
  -- right day of the month without producing a decade of drafts.
  starts_on TEXT NOT NULL CHECK (starts_on GLOB '????-??-??'),
  ends_on TEXT CHECK (ends_on IS NULL OR (ends_on GLOB '????-??-??' AND ends_on >= starts_on)),
  max_occurrences INTEGER CHECK (max_occurrences IS NULL OR max_occurrences >= 1),
  lead_days INTEGER NOT NULL DEFAULT 5 CHECK (lead_days BETWEEN 0 AND 90),
  generate_from TEXT NOT NULL CHECK (generate_from GLOB '????-??-??'),

  created_at TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TEXT NOT NULL,
  updated_by_user_id INTEGER REFERENCES users(id) ON DELETE RESTRICT,

  -- A template that cannot be enumerated must not reach the generator, so the
  -- per-frequency field requirements are refused here as well as in the
  -- service. Monthly and yearly take a nominal day or the last-day flag,
  -- never both and never neither.
  CHECK (frequency <> 'weekly' OR by_weekday IS NOT NULL),
  CHECK (
    frequency IN ('daily', 'weekly')
    OR (day_of_month IS NOT NULL AND last_day_of_month = 0)
    OR (day_of_month IS NULL AND last_day_of_month = 1)
  ),
  CHECK (frequency <> 'yearly' OR month_of_year IS NOT NULL),
  -- A stored template must mean one thing. A daily rule carrying a weekday, or
  -- a weekly rule carrying a day of the month, reads as two schedules at once,
  -- and the enumerator would silently honour only one of them.
  CHECK (frequency <> 'daily' OR (by_weekday IS NULL AND day_of_month IS NULL AND month_of_year IS NULL AND last_day_of_month = 0)),
  CHECK (frequency <> 'weekly' OR (day_of_month IS NULL AND month_of_year IS NULL AND last_day_of_month = 0)),
  CHECK (frequency <> 'monthly' OR month_of_year IS NULL),
  CHECK (payee_id IS NULL OR payee_name IS NULL)
);

CREATE INDEX recurring_templates_book_idx
  ON recurring_templates (book_id, name ASC, id ASC);

-- The generator's own sweep: enabled, unarchived templates only.
CREATE INDEX recurring_templates_due_idx
  ON recurring_templates (book_id, generate_from ASC, id ASC)
  WHERE enabled = 1 AND archived_at IS NULL;

-- +goose StatementBegin
CREATE TRIGGER recurring_templates_payee_same_book_insert
BEFORE INSERT ON recurring_templates
BEGIN
  SELECT CASE WHEN NEW.payee_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM payees WHERE id = NEW.payee_id AND book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'recurring template payee must belong to the same book') END;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER recurring_templates_payee_same_book_update
BEFORE UPDATE OF book_id, payee_id ON recurring_templates
BEGIN
  SELECT CASE WHEN NEW.payee_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM payees WHERE id = NEW.payee_id AND book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'recurring template payee must belong to the same book') END;
END;
-- +goose StatementEnd

-- Template postings carry exact coefficients as text, exactly as
-- posting_versions does. Nothing here is ever a float.
--
-- Balance is not a trigger: the reason a template does not balance is a
-- message the user needs, and the service checks it on save and again at
-- generation.
CREATE TABLE recurring_template_postings (
  id INTEGER PRIMARY KEY,
  template_id INTEGER NOT NULL REFERENCES recurring_templates(id) ON DELETE CASCADE,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  line_seq INTEGER NOT NULL CHECK (line_seq > 0),
  line_key TEXT NOT NULL CHECK (length(trim(line_key)) > 0 AND line_key = trim(line_key)),
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
  quantity_value TEXT NOT NULL DEFAULT '0' CHECK (length(quantity_value) BETWEEN 1 AND 39),
  quantity_scale INTEGER NOT NULL DEFAULT 0 CHECK (quantity_scale BETWEEN 0 AND 24),
  commodity_id INTEGER NOT NULL REFERENCES commodities(id) ON DELETE RESTRICT,
  memo TEXT NOT NULL DEFAULT '',
  UNIQUE (template_id, line_seq),
  UNIQUE (template_id, line_key)
);

CREATE INDEX recurring_template_postings_account_idx
  ON recurring_template_postings (account_id, template_id);

-- +goose StatementBegin
CREATE TRIGGER recurring_template_postings_same_book
BEFORE INSERT ON recurring_template_postings
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1
    FROM recurring_templates template
    JOIN accounts account ON account.id = NEW.account_id
    JOIN commodities commodity ON commodity.id = NEW.commodity_id
    WHERE template.id = NEW.template_id
      AND template.book_id = NEW.book_id
      AND account.book_id = NEW.book_id
      AND commodity.book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'recurring template posting account and commodity must belong to the same book') END;
END;
-- +goose StatementEnd

CREATE TABLE recurring_template_tags (
  template_id INTEGER NOT NULL REFERENCES recurring_templates(id) ON DELETE CASCADE,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE RESTRICT,
  PRIMARY KEY (template_id, tag_id)
);

CREATE INDEX recurring_template_tags_book_tag_idx
  ON recurring_template_tags (book_id, tag_id, template_id);

-- +goose StatementBegin
CREATE TRIGGER recurring_template_tags_same_book
BEFORE INSERT ON recurring_template_tags
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1
    FROM recurring_templates template
    JOIN tags tag ON tag.id = NEW.tag_id
    WHERE template.id = NEW.template_id
      AND template.book_id = NEW.book_id
      AND tag.book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'recurring template tag must belong to the same book') END;
END;
-- +goose StatementEnd

-- One row per occurrence the generator has acted on. The future is computed by
-- internal/recur, never stored, so a row here means something happened:
-- a draft was created, the user skipped the date, or the template could not
-- produce it.
--
-- UNIQUE (template_id, occurrence_date) is the whole idempotency story, the
-- same way backup_runs.occurrence_key is: two schedulers, a restart mid-tick,
-- or a clock that steps backwards all converge on one row per date.
CREATE TABLE recurring_occurrences (
  id INTEGER PRIMARY KEY,
  book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE RESTRICT,
  template_id INTEGER NOT NULL REFERENCES recurring_templates(id) ON DELETE RESTRICT,
  occurrence_date TEXT NOT NULL CHECK (occurrence_date GLOB '????-??-??'),
  status TEXT NOT NULL CHECK (status IN ('generated', 'skipped', 'blocked')),
  transaction_id INTEGER REFERENCES transactions(id) ON DELETE RESTRICT,
  error_summary TEXT NOT NULL DEFAULT '',
  skip_reason TEXT NOT NULL DEFAULT '',
  -- UTC, per the conventions' rule that a schedule is local wall clock and
  -- every actual run is recorded in UTC.
  materialized_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (template_id, occurrence_date),
  CHECK (status <> 'generated' OR transaction_id IS NOT NULL),
  CHECK (status <> 'blocked' OR length(trim(error_summary)) > 0),
  CHECK (status = 'generated' OR transaction_id IS NULL)
);

CREATE INDEX recurring_occurrences_book_date_idx
  ON recurring_occurrences (book_id, occurrence_date DESC, id DESC);

-- The due inbox reads blocked rows and the drafts still awaiting review.
CREATE INDEX recurring_occurrences_status_idx
  ON recurring_occurrences (book_id, status, occurrence_date ASC, id ASC);

-- +goose StatementBegin
CREATE TRIGGER recurring_occurrences_same_book
BEFORE INSERT ON recurring_occurrences
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM recurring_templates
    WHERE id = NEW.template_id AND book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'recurring occurrence template must belong to the same book') END;
  SELECT CASE WHEN NEW.transaction_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM transactions
    WHERE id = NEW.transaction_id AND book_id = NEW.book_id
  ) THEN RAISE(ABORT, 'recurring occurrence transaction must belong to the same book') END;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS recurring_occurrences_same_book;
DROP INDEX IF EXISTS recurring_occurrences_status_idx;
DROP INDEX IF EXISTS recurring_occurrences_book_date_idx;
DROP TABLE IF EXISTS recurring_occurrences;
DROP TRIGGER IF EXISTS recurring_template_tags_same_book;
DROP INDEX IF EXISTS recurring_template_tags_book_tag_idx;
DROP TABLE IF EXISTS recurring_template_tags;
DROP TRIGGER IF EXISTS recurring_template_postings_same_book;
DROP INDEX IF EXISTS recurring_template_postings_account_idx;
DROP TABLE IF EXISTS recurring_template_postings;
DROP TRIGGER IF EXISTS recurring_templates_payee_same_book_update;
DROP TRIGGER IF EXISTS recurring_templates_payee_same_book_insert;
DROP INDEX IF EXISTS recurring_templates_due_idx;
DROP INDEX IF EXISTS recurring_templates_book_idx;
DROP TABLE IF EXISTS recurring_templates;
