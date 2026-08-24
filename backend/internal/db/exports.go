package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"rekenraam/backend/internal/exact"
)

// ExportRepository reads the portable ledger. It runs on the read-only pool
// (OpenReadOnly), never the writer's single connection, and every read of one
// export happens inside one snapshot transaction so the files of an archive
// cannot disagree with each other.
type ExportRepository struct {
	database *sql.DB
}

func NewExportRepository(database *sql.DB) *ExportRepository {
	return &ExportRepository{database: database}
}

// LedgerExportPostingRecord is one exported posting: the posting itself plus
// the transaction and entry it belongs to. Coefficients stay strings; nothing
// here is summed in SQL.
type LedgerExportPostingRecord struct {
	// TransactionComplete is true when every entry of this row's transaction is
	// in the export. Always true for an unfiltered export; under a filter it is
	// what tells a consumer which transaction groups it can balance.
	TransactionComplete       bool
	TransactionID             int64
	TransactionVersionID      int64
	TransactionKind           string
	Status                    string
	TransactionDate           string
	CorrectionOfTransactionID sql.NullInt64
	PayeeID                   sql.NullInt64
	PayeeName                 string
	Description               string
	ExternalRefHint           sql.NullString
	NeedsReview               bool
	TransactionTags           sql.NullString
	JournalEntryID            int64
	EntrySeq                  int64
	EntryKind                 string
	EntryDate                 string
	EntryMemo                 string
	PostingLineKey            string
	LineSeq                   int64
	AccountID                 int64
	AccountClass              string
	AccountKind               string
	QuantityValue             exact.Coefficient
	QuantityScale             int
	CommodityID               int64
	CommodityCode             string
	CommodityKind             string
	PostingMemo               string
	ReconciliationStatus      string
	ClearedOn                 sql.NullString
	PostingTags               sql.NullString
}

// ExportAccountRecord is an account as the export names it. The path is
// assembled from these rows in the service layer, not in SQL.
type ExportAccountRecord struct {
	AccountID       int64
	Name            sql.NullString
	Code            sql.NullString
	ParentAccountID sql.NullInt64
	AccountClass    string
	AccountKind     string
	SystemRole      sql.NullString
	BaseKind        string
	Status          string
	OpenedOn        string
	ClosedOn        sql.NullString
	InstitutionName sql.NullString
	CommodityCode   sql.NullString
	CommodityKind   sql.NullString
	AllowsPostings  bool
	// CategoryType, CategoryBuiltin, CategoryStarter, and CategoryBuiltinKey
	// come from the account's metadata: a category *is* an income or expense
	// account here (docs/design/categories-design.md), and these are the only
	// facts the account row does not otherwise carry.
	CategoryType       sql.NullString
	CategoryBuiltin    sql.NullBool
	CategoryStarter    sql.NullBool
	CategoryBuiltinKey sql.NullString
}

// ExportPayeeRecord, ExportCommodityRecord, ExportTagRecord, ExportLotRecord,
// and ExportPriceRecord are the archive's reference tables: flat statements of
// current state, joinable to ledger.csv by id.
type ExportPayeeRecord struct {
	PayeeID int64
	Name    string
	Status  string
}

type ExportCommodityRecord struct {
	CommodityID      int64
	Code             string
	Kind             string
	Name             string
	Symbol           string
	StandardScale    int
	MaxQuantityScale int
	Status           string
}

type ExportTagRecord struct {
	TagID  int64
	Name   string
	Kind   string
	Status string
}

type ExportLotRecord struct {
	LotID                   int64
	AccountID               int64
	CommodityID             int64
	OpenedOn                string
	Status                  string
	QuantityValue           exact.Coefficient
	QuantityScale           int
	RemainingQuantityValue  exact.Coefficient
	RemainingQuantityScale  int
	CostBasisValue          int64
	CostBasisScale          int
	RemainingCostBasisValue int64
	RemainingCostBasisScale int
	CostCommodityID         int64
	SourceTransactionID     sql.NullInt64
}

type ExportPriceRecord struct {
	BaseCommodityID   int64
	QuoteCommodityID  int64
	ValuationDate     string
	PriceValue        int64
	PriceScale        int
	BaseQuantityValue int64
	BaseQuantityScale int
	QuoteType         string
	AdjustmentBasis   string
	IsManual          bool
	IsDerived         bool
	SourceCode        sql.NullString
}

// LedgerExportTotalsRecord answers "what would this export contain" without
// producing it, so the screen can say so before a download starts and a bad
// request can still fail with an error envelope.
type LedgerExportTotalsRecord struct {
	PostingCount      int64
	JournalEntryCount int64
	TransactionCount  int64
	AccountCount      int64
	CommodityCount    int64
	EarliestEntryDate sql.NullString
	LatestEntryDate   sql.NullString
	// IncompleteTransactionCount is how many exported transactions are missing
	// at least one of their own entries — zero unless a filter cut across one.
	IncompleteTransactionCount int64
}

// LedgerExportSelection is a scoped export's filter, already validated and
// already expanded (descendants resolved) by the service.
//
// It selects **journal entries**, never individual postings: dropping a
// counterpart leg would break the per-entry balance an export guarantees. Every
// posting of a selected entry is exported, including postings in accounts,
// commodities, and dates the filter never named. See ADR 0011.
type LedgerExportSelection struct {
	// From and To are inclusive; empty means unbounded on that side.
	From string
	To   string
	// DateBasis is "entry" (default) or "transaction". Under the transaction
	// basis the range picks transactions and then takes *every* entry of them,
	// including entries dated outside the range.
	DateBasis string
	// AccountIDs and CommodityIDs are OR-sets within a dimension and AND across
	// dimensions; empty means unrestricted in that dimension.
	AccountIDs   []int64
	CommodityIDs []int64
}

const (
	// DateBasisEntry selects entries by their own accounting date.
	DateBasisEntry = "entry"
	// DateBasisTransaction selects transactions by their date, then all of
	// their entries.
	DateBasisTransaction = "transaction"
)

// Filtered reports whether this selection restricts anything at all.
func (selection LedgerExportSelection) Filtered() bool {
	return selection.From != "" || selection.To != "" ||
		len(selection.AccountIDs) > 0 || len(selection.CommodityIDs) > 0
}

// entrySelectionClause builds the predicate that decides whether one journal
// entry is in the export, written against the given journal_entries alias.
//
// It is written once and used three times — the rows to export, the test for
// whether a transaction is whole, and the trial balance's selected/excluded
// split — because three copies of a selection rule is three chances for an
// export to disagree with its own manifest.
func entrySelectionClause(entryAlias string, selection LedgerExportSelection) (string, []any) {
	dateColumn := entryAlias + ".entry_date"
	if selection.DateBasis == DateBasisTransaction {
		dateColumn = "tv.transaction_date"
	}

	clauses := []string{"1 = 1"}
	var args []any

	if selection.From != "" {
		clauses = append(clauses, dateColumn+" >= ?")
		args = append(args, selection.From)
	}
	if selection.To != "" {
		clauses = append(clauses, dateColumn+" <= ?")
		args = append(args, selection.To)
	}
	if clause, clauseArgs := inClause("scope_account_pv.account_id", selection.AccountIDs); clause != "" {
		clauses = append(clauses, `EXISTS (
			SELECT 1 FROM posting_versions scope_account_pv
			WHERE scope_account_pv.journal_entry_id = `+entryAlias+`.id AND `+clause+`
		)`)
		args = append(args, clauseArgs...)
	}
	if clause, clauseArgs := inClause("scope_commodity_pv.commodity_id", selection.CommodityIDs); clause != "" {
		clauses = append(clauses, `EXISTS (
			SELECT 1 FROM posting_versions scope_commodity_pv
			WHERE scope_commodity_pv.journal_entry_id = `+entryAlias+`.id AND `+clause+`
		)`)
		args = append(args, clauseArgs...)
	}

	return "(" + strings.Join(clauses, " AND ") + ")", args
}

// transactionCompleteExpression is true when every entry of the row's
// transaction is in the export, so a consumer can tell which transaction groups
// it can balance and which it holds only part of.
func transactionCompleteExpression(selection LedgerExportSelection) (string, []any) {
	clause, args := entrySelectionClause("complete_je", selection)
	return `NOT EXISTS (
		SELECT 1 FROM journal_entries complete_je
		WHERE complete_je.transaction_version_id = tv.id
			AND NOT ` + clause + `
	)`, args
}

// exportPostingSelection is the one policy the export reads by: the current
// version of every posted, non-deleted transaction. Drafts, voided versions,
// superseded versions, and soft-deleted transactions never leave the app
// through this door — the SQLite backup is the audit-complete artifact.
//
// Both the stream and the counts share this clause verbatim, so a preview can
// never promise a different set than the download delivers.
//
// The account version is resolved as of the entry date, falling back to the
// account's earliest version when a posting predates every version of its own
// account. The join is LEFT for the same reason: an export that silently
// dropped a posting would emit an unbalanced entry, which is the one thing this
// file may never do.
//
// The write path rejects that case today (app/transactions_validate.go's as-of
// PostingAccountRule; the misleading message it produces is backlog T-63), so
// this fallback is defensive rather than load-bearing — it covers data arriving
// from outside the service layer: a backfill, a manual repair, a restored and
// patched database. A fallback that never announces itself would turn such data
// into a plausible-looking number, so the self-check counts it as
// account_version_coverage: docs/plans/data-portability-plan.md, slice 6.
const exportPostingSelection = `
	FROM current_transaction_versions tv
	JOIN transactions t ON t.id = tv.transaction_id
	JOIN journal_entries je ON je.transaction_version_id = tv.id
	JOIN posting_versions pv ON pv.journal_entry_id = je.id
	JOIN posting_lines pl ON pl.id = pv.posting_line_id
	JOIN commodities c ON c.id = pv.commodity_id
	LEFT JOIN payees payee_record ON payee_record.id = tv.payee_id
	LEFT JOIN current_payee_versions payee ON payee.payee_id = payee_record.id
	LEFT JOIN account_versions av ON av.id = COALESCE(
		(
			SELECT asof_av.id
			FROM account_versions asof_av
			WHERE asof_av.account_id = pv.account_id
				AND asof_av.effective_from <= je.entry_date
			ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC
			LIMIT 1
		),
		(
			SELECT earliest_av.id
			FROM account_versions earliest_av
			WHERE earliest_av.account_id = pv.account_id
			ORDER BY earliest_av.effective_from ASC, earliest_av.version_seq ASC
			LIMIT 1
		)
	)
	WHERE tv.book_id = ?
		AND tv.status = 'posted'
		AND t.deleted_at IS NULL`

// Snapshot begins the read transaction an export reads through. In WAL mode a
// deferred read sees one stable state for its lifetime, which is what makes a
// multi-file archive internally consistent. The caller must always roll it
// back; it never writes, so there is nothing to commit.
func (r *ExportRepository) Snapshot(ctx context.Context) (*sql.Tx, error) {
	transaction, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin export snapshot: %w", err)
	}
	return transaction, nil
}

// StreamLedgerPostings visits every exported posting in a stable order without
// materializing the result: an export is written as it is read.
//
// The order is entry date, then transaction, then entry, then line — the order
// a human reads a ledger in, and a total order, so two exports of the same
// snapshot are byte-identical.
func (r *ExportRepository) StreamLedgerPostings(ctx context.Context, transaction *sql.Tx, bookID int64, selection LedgerExportSelection, visit func(LedgerExportPostingRecord) error) error {
	completeExpression, completeArgs := transactionCompleteExpression(selection)
	selectionClause, selectionArgs := entrySelectionClause("je", selection)

	// Argument order follows the order the placeholders appear in the SQL
	// text: the SELECT-list expression first, then the book, then the WHERE.
	args := make([]any, 0, len(completeArgs)+1+len(selectionArgs))
	args = append(args, completeArgs...)
	args = append(args, bookID)
	args = append(args, selectionArgs...)

	rows, err := transaction.QueryContext(ctx, `
		SELECT
			`+completeExpression+`,
			t.id,
			tv.id,
			tv.transaction_kind,
			tv.status,
			tv.transaction_date,
			t.correction_of_transaction_id,
			tv.payee_id,
			COALESCE(payee.name, tv.payee_name, ''),
			tv.description,
			tv.external_ref_hint,
			tv.needs_review,
			(
				SELECT group_concat(tag_name, ';')
				FROM (
					SELECT tag.name AS tag_name
					FROM transaction_tags tt
					JOIN tags tag ON tag.id = tt.tag_id
					WHERE tt.transaction_id = t.id
					ORDER BY tag.name
				)
			),
			je.id,
			je.entry_seq,
			je.entry_kind,
			je.entry_date,
			je.memo,
			pl.line_key,
			pv.line_seq,
			pv.account_id,
			COALESCE(av.account_class, ''),
			COALESCE(av.account_kind, ''),
			pv.quantity_value,
			pv.quantity_scale,
			pv.commodity_id,
			c.code,
			c.kind,
			pv.memo,
			pv.reconciliation_status,
			pv.cleared_on,
			(
				SELECT group_concat(tag_name, ';')
				FROM (
					SELECT tag.name AS tag_name
					FROM posting_tags pt
					JOIN tags tag ON tag.id = pt.tag_id
					WHERE pt.posting_line_id = pv.posting_line_id
					ORDER BY tag.name
				)
			)`+exportPostingSelection+`
			AND `+selectionClause+`
		ORDER BY je.entry_date, t.id, je.entry_seq, pv.line_seq, pv.id
	`, args...)
	if err != nil {
		return fmt.Errorf("read ledger export postings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var record LedgerExportPostingRecord
		if err := rows.Scan(
			&record.TransactionComplete,
			&record.TransactionID,
			&record.TransactionVersionID,
			&record.TransactionKind,
			&record.Status,
			&record.TransactionDate,
			&record.CorrectionOfTransactionID,
			&record.PayeeID,
			&record.PayeeName,
			&record.Description,
			&record.ExternalRefHint,
			&record.NeedsReview,
			&record.TransactionTags,
			&record.JournalEntryID,
			&record.EntrySeq,
			&record.EntryKind,
			&record.EntryDate,
			&record.EntryMemo,
			&record.PostingLineKey,
			&record.LineSeq,
			&record.AccountID,
			&record.AccountClass,
			&record.AccountKind,
			&record.QuantityValue,
			&record.QuantityScale,
			&record.CommodityID,
			&record.CommodityCode,
			&record.CommodityKind,
			&record.PostingMemo,
			&record.ReconciliationStatus,
			&record.ClearedOn,
			&record.PostingTags,
		); err != nil {
			return fmt.Errorf("scan ledger export posting: %w", err)
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate ledger export postings: %w", err)
	}

	return nil
}

// ExportAccounts returns every account of every class — income and expense
// included, since categories are accounts here — so a path can be built for
// any account a posting names.
func (r *ExportRepository) ExportAccounts(ctx context.Context, transaction *sql.Tx, bookID int64) ([]ExportAccountRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT
			a.id,
			av.name,
			av.code,
			av.parent_account_id,
			av.account_class,
			av.account_kind,
			a.system_role,
			COALESCE(kind.base_kind, ''),
			av.status,
			av.opened_on,
			av.closed_on,
			institution_v.name,
			commodity.code,
			commodity.kind,
			av.allows_postings,
			json_extract(av.metadata_json, '$.category.type'),
			json_extract(av.metadata_json, '$.category.is_builtin'),
			json_extract(av.metadata_json, '$.category.is_starter'),
			json_extract(av.metadata_json, '$.category.builtin_key')
		FROM accounts a
		JOIN current_account_versions av ON av.account_id = a.id
		LEFT JOIN account_kinds kind ON kind.code = av.account_kind AND kind.account_class = av.account_class
		LEFT JOIN institutions institution ON institution.id = av.institution_id
		LEFT JOIN institution_versions institution_v ON institution_v.id = (
			SELECT current_iv.id
			FROM institution_versions current_iv
			WHERE current_iv.institution_id = institution.id
			ORDER BY current_iv.effective_from DESC, current_iv.version_seq DESC
			LIMIT 1
		)
		LEFT JOIN commodities commodity ON commodity.id = av.default_commodity_id
		WHERE a.book_id = ?
		ORDER BY a.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read export accounts: %w", err)
	}
	defer rows.Close()

	var accounts []ExportAccountRecord
	for rows.Next() {
		var account ExportAccountRecord
		if err := rows.Scan(
			&account.AccountID,
			&account.Name,
			&account.Code,
			&account.ParentAccountID,
			&account.AccountClass,
			&account.AccountKind,
			&account.SystemRole,
			&account.BaseKind,
			&account.Status,
			&account.OpenedOn,
			&account.ClosedOn,
			&account.InstitutionName,
			&account.CommodityCode,
			&account.CommodityKind,
			&account.AllowsPostings,
			&account.CategoryType,
			&account.CategoryBuiltin,
			&account.CategoryStarter,
			&account.CategoryBuiltinKey,
		); err != nil {
			return nil, fmt.Errorf("scan export account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate export accounts: %w", err)
	}

	return accounts, nil
}

// LedgerExportTotals counts what the same selection would export. COUNT and
// MIN/MAX over dates are safe in SQL; no coefficient is aggregated here.
func (r *ExportRepository) LedgerExportTotals(ctx context.Context, transaction *sql.Tx, bookID int64, selection LedgerExportSelection) (LedgerExportTotalsRecord, error) {
	completeExpression, completeArgs := transactionCompleteExpression(selection)
	selectionClause, selectionArgs := entrySelectionClause("je", selection)

	args := make([]any, 0, len(completeArgs)+1+len(selectionArgs))
	args = append(args, completeArgs...)
	args = append(args, bookID)
	args = append(args, selectionArgs...)

	var totals LedgerExportTotalsRecord
	err := transaction.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(DISTINCT je.id),
			COUNT(DISTINCT t.id),
			COUNT(DISTINCT pv.account_id),
			COUNT(DISTINCT pv.commodity_id),
			MIN(je.entry_date),
			MAX(je.entry_date),
			COUNT(DISTINCT CASE WHEN NOT `+completeExpression+` THEN t.id END)`+exportPostingSelection+`
			AND `+selectionClause+`
	`, args...).Scan(
		&totals.PostingCount,
		&totals.JournalEntryCount,
		&totals.TransactionCount,
		&totals.AccountCount,
		&totals.CommodityCount,
		&totals.EarliestEntryDate,
		&totals.LatestEntryDate,
		&totals.IncompleteTransactionCount,
	)
	if err != nil {
		return LedgerExportTotalsRecord{}, fmt.Errorf("read ledger export totals: %w", err)
	}

	return totals, nil
}

// TrialBalancePostingRecord is one posting of the whole book, flagged with
// whether the export's selection took it.
//
// The trial balance needs postings the export does not carry: an opening
// balance is built from history before the range, and the excluded movement is
// what scope left behind. Reading them here, in the same snapshot, is what lets
// the archive state figures it cannot itself justify without guessing.
type TrialBalancePostingRecord struct {
	AccountID     int64
	CommodityID   int64
	EntryDate     string
	QuantityValue exact.Coefficient
	QuantityScale int
	Selected      bool
}

// StreamTrialBalancePostings visits every posted, non-deleted posting in the
// book — not only the exported ones — with the selection flag the trial balance
// splits on. Coefficients come back as strings and are summed in Go.
func (r *ExportRepository) StreamTrialBalancePostings(ctx context.Context, transaction *sql.Tx, bookID int64, selection LedgerExportSelection, visit func(TrialBalancePostingRecord) error) error {
	selectionClause, selectionArgs := entrySelectionClause("je", selection)

	args := make([]any, 0, len(selectionArgs)+1)
	args = append(args, selectionArgs...)
	args = append(args, bookID)

	rows, err := transaction.QueryContext(ctx, `
		SELECT
			pv.account_id,
			pv.commodity_id,
			je.entry_date,
			pv.quantity_value,
			pv.quantity_scale,
			CASE WHEN `+selectionClause+` THEN 1 ELSE 0 END
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		WHERE tv.book_id = ?
			AND tv.status = 'posted'
			AND t.deleted_at IS NULL
		ORDER BY pv.account_id, pv.commodity_id, je.entry_date, pv.id
	`, args...)
	if err != nil {
		return fmt.Errorf("read trial balance postings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var record TrialBalancePostingRecord
		if err := rows.Scan(
			&record.AccountID,
			&record.CommodityID,
			&record.EntryDate,
			&record.QuantityValue,
			&record.QuantityScale,
			&record.Selected,
		); err != nil {
			return fmt.Errorf("scan trial balance posting: %w", err)
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate trial balance postings: %w", err)
	}

	return nil
}

// ExportPayees returns every payee by its current version.
func (r *ExportRepository) ExportPayees(ctx context.Context, transaction *sql.Tx, bookID int64) ([]ExportPayeeRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT p.id, pv.name, pv.status
		FROM payees p
		JOIN current_payee_versions pv ON pv.payee_id = p.id
		WHERE p.book_id = ?
		ORDER BY p.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read export payees: %w", err)
	}
	defer rows.Close()

	var payees []ExportPayeeRecord
	for rows.Next() {
		var payee ExportPayeeRecord
		if err := rows.Scan(&payee.PayeeID, &payee.Name, &payee.Status); err != nil {
			return nil, fmt.Errorf("scan export payee: %w", err)
		}
		payees = append(payees, payee)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate export payees: %w", err)
	}

	return payees, nil
}

// ExportCommodities returns every commodity with the scales that make its
// quantities readable: standard_scale is how it is normally written,
// max_quantity_scale is what the ledger will store.
func (r *ExportRepository) ExportCommodities(ctx context.Context, transaction *sql.Tx, bookID int64) ([]ExportCommodityRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT c.id, c.code, c.kind, cv.name, cv.display_symbol, cv.standard_scale, cv.max_quantity_scale, cv.status
		FROM commodities c
		JOIN current_commodity_versions cv ON cv.commodity_id = c.id
		WHERE c.book_id = ?
		ORDER BY c.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read export commodities: %w", err)
	}
	defer rows.Close()

	var commodities []ExportCommodityRecord
	for rows.Next() {
		var commodity ExportCommodityRecord
		if err := rows.Scan(
			&commodity.CommodityID,
			&commodity.Code,
			&commodity.Kind,
			&commodity.Name,
			&commodity.Symbol,
			&commodity.StandardScale,
			&commodity.MaxQuantityScale,
			&commodity.Status,
		); err != nil {
			return nil, fmt.Errorf("scan export commodity: %w", err)
		}
		commodities = append(commodities, commodity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate export commodities: %w", err)
	}

	return commodities, nil
}

// ExportTags returns every tag, so the semicolon-joined tag columns in
// ledger.csv resolve to something with a kind and a status.
func (r *ExportRepository) ExportTags(ctx context.Context, transaction *sql.Tx, bookID int64) ([]ExportTagRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT id, name, kind, status
		FROM tags
		WHERE book_id = ?
		ORDER BY id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read export tags: %w", err)
	}
	defer rows.Close()

	var tags []ExportTagRecord
	for rows.Next() {
		var tag ExportTagRecord
		if err := rows.Scan(&tag.TagID, &tag.Name, &tag.Kind, &tag.Status); err != nil {
			return nil, fmt.Errorf("scan export tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate export tags: %w", err)
	}

	return tags, nil
}

// ExportLots returns investment lots as they currently stand. Cost basis cannot
// be reconstructed from postings alone, which is why the archive carries this
// at all; it is a statement of current state, not a replayable event log.
func (r *ExportRepository) ExportLots(ctx context.Context, transaction *sql.Tx, bookID int64) ([]ExportLotRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT
			id, account_id, commodity_id, opened_on, status,
			quantity_value, quantity_scale,
			remaining_quantity_value, remaining_quantity_scale,
			cost_basis_value, cost_basis_scale,
			remaining_cost_basis_value, remaining_cost_basis_scale,
			cost_commodity_id, source_transaction_id
		FROM investment_lots
		WHERE book_id = ?
		ORDER BY account_id, commodity_id, opened_on, id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read export lots: %w", err)
	}
	defer rows.Close()

	var lots []ExportLotRecord
	for rows.Next() {
		var lot ExportLotRecord
		if err := rows.Scan(
			&lot.LotID,
			&lot.AccountID,
			&lot.CommodityID,
			&lot.OpenedOn,
			&lot.Status,
			&lot.QuantityValue,
			&lot.QuantityScale,
			&lot.RemainingQuantityValue,
			&lot.RemainingQuantityScale,
			&lot.CostBasisValue,
			&lot.CostBasisScale,
			&lot.RemainingCostBasisValue,
			&lot.RemainingCostBasisScale,
			&lot.CostCommodityID,
			&lot.SourceTransactionID,
		); err != nil {
			return nil, fmt.Errorf("scan export lot: %w", err)
		}
		lots = append(lots, lot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate export lots: %w", err)
	}

	return lots, nil
}

// ExportPrices returns non-voided price observations. A voided observation is a
// statement the book has withdrawn, so exporting it would re-assert it.
func (r *ExportRepository) ExportPrices(ctx context.Context, transaction *sql.Tx, bookID int64) ([]ExportPriceRecord, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT
			po.base_commodity_id, po.quote_commodity_id, po.valuation_date,
			po.price_value, po.price_scale,
			po.base_quantity_value, po.base_quantity_scale,
			po.quote_type, po.adjustment_basis,
			po.is_manual, po.is_derived,
			source.code
		FROM price_observations po
		LEFT JOIN market_data_sources source ON source.id = po.source_id
		WHERE po.book_id = ? AND po.voided_at IS NULL
		ORDER BY po.base_commodity_id, po.quote_commodity_id, po.valuation_date, po.id
	`, bookID)
	if err != nil {
		return nil, fmt.Errorf("read export prices: %w", err)
	}
	defer rows.Close()

	var prices []ExportPriceRecord
	for rows.Next() {
		var price ExportPriceRecord
		if err := rows.Scan(
			&price.BaseCommodityID,
			&price.QuoteCommodityID,
			&price.ValuationDate,
			&price.PriceValue,
			&price.PriceScale,
			&price.BaseQuantityValue,
			&price.BaseQuantityScale,
			&price.QuoteType,
			&price.AdjustmentBasis,
			&price.IsManual,
			&price.IsDerived,
			&price.SourceCode,
		); err != nil {
			return nil, fmt.Errorf("scan export price: %w", err)
		}
		prices = append(prices, price)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate export prices: %w", err)
	}

	return prices, nil
}

// AccountExistsInBook is the export's own filter validation. It deliberately
// does not restrict account class the way the reports filter does: "everything
// that touched Groceries" is a reasonable thing to export, while it is not a
// reasonable thing to put on a net-worth chart.
func (r *ExportRepository) AccountExistsInBook(ctx context.Context, transaction *sql.Tx, bookID int64, accountID int64) (bool, error) {
	var exists int
	err := transaction.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM accounts WHERE book_id = ? AND id = ?)
	`, bookID, accountID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check export account filter: %w", err)
	}
	return exists == 1, nil
}

// CommodityExistsInBook mirrors AccountExistsInBook for the commodity filter.
func (r *ExportRepository) CommodityExistsInBook(ctx context.Context, transaction *sql.Tx, bookID int64, commodityID int64) (bool, error) {
	var exists int
	err := transaction.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM commodities WHERE book_id = ? AND id = ?)
	`, bookID, commodityID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check export commodity filter: %w", err)
	}
	return exists == 1, nil
}
