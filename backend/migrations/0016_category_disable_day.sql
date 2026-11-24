-- +goose Up
-- #664: a disabled category keeps its disable day. Disable archives a
-- category in one step with closed_on equal to its effective date, and
-- closed_on is the last day an account is open (#292). The baseline trigger
-- refused every archived version, so a posting on the disable day could not
-- be edited with its journal kept, voided or unvoided. An archived category
-- version now accepts entries dated on or before its closed_on, like a closed
-- account; an archived ordinary account still accepts nothing. The category
-- test is categoryAccountPredicate, written with IS so it is two-valued.
-- db.PostingAccountRule.AcceptsPostingsOn and requirePostingsStayEligibleTx
-- apply the same rule.

-- +goose StatementBegin
DROP TRIGGER posting_versions_account_version_valid;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER posting_versions_account_version_valid
BEFORE INSERT ON posting_versions
WHEN NOT EXISTS (
  SELECT 1
  FROM journal_entries je
  JOIN account_versions av ON av.account_id = NEW.account_id
  WHERE je.id = NEW.journal_entry_id
    AND av.id = (
      SELECT asof_av.id FROM account_versions asof_av
      WHERE asof_av.account_id = NEW.account_id AND asof_av.effective_from <= je.entry_date
      ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC LIMIT 1
    )
    AND (
      av.status = 'active'
      OR (av.status = 'closed' AND av.closed_on IS NOT NULL AND je.entry_date <= av.closed_on)
      OR (
        av.status = 'archived' AND av.closed_on IS NOT NULL AND je.entry_date <= av.closed_on
        AND av.account_class IN ('income', 'expense')
        AND av.account_kind = av.account_class
        AND json_extract(av.metadata_json, '$.category.type') IS av.account_class
      )
    )
    AND av.allows_postings = 1
    AND (av.opened_on IS NULL OR je.entry_date >= av.opened_on)
    AND (av.closed_on IS NULL OR je.entry_date <= av.closed_on)
    AND (av.default_commodity_id IS NULL OR av.default_commodity_id = NEW.commodity_id)
    AND (av.quantity_scale_override IS NULL OR NEW.quantity_scale <= av.quantity_scale_override)
)
BEGIN
  SELECT RAISE(ABORT, 'posting account is not eligible on entry date');
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER posting_versions_account_version_valid;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER posting_versions_account_version_valid
BEFORE INSERT ON posting_versions
WHEN NOT EXISTS (
  SELECT 1
  FROM journal_entries je
  JOIN account_versions av ON av.account_id = NEW.account_id
  WHERE je.id = NEW.journal_entry_id
    AND av.id = (
      SELECT asof_av.id FROM account_versions asof_av
      WHERE asof_av.account_id = NEW.account_id AND asof_av.effective_from <= je.entry_date
      ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC LIMIT 1
    )
    AND (
      av.status = 'active'
      OR (av.status = 'closed' AND av.closed_on IS NOT NULL AND je.entry_date <= av.closed_on)
    )
    AND av.allows_postings = 1
    AND (av.opened_on IS NULL OR je.entry_date >= av.opened_on)
    AND (av.closed_on IS NULL OR je.entry_date <= av.closed_on)
    AND (av.default_commodity_id IS NULL OR av.default_commodity_id = NEW.commodity_id)
    AND (av.quantity_scale_override IS NULL OR NEW.quantity_scale <= av.quantity_scale_override)
)
BEGIN
  SELECT RAISE(ABORT, 'posting account is not eligible on entry date');
END;
-- +goose StatementEnd
