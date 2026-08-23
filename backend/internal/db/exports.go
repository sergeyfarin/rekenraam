package db

import (
	"context"
	"database/sql"
	"fmt"

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
// account (a backdated import can produce exactly that). The join is LEFT for
// the same reason: an export that silently dropped a posting would emit an
// unbalanced entry, which is the one thing this file may never do.
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
func (r *ExportRepository) StreamLedgerPostings(ctx context.Context, transaction *sql.Tx, bookID int64, visit func(LedgerExportPostingRecord) error) error {
	rows, err := transaction.QueryContext(ctx, `
		SELECT
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
		ORDER BY je.entry_date, t.id, je.entry_seq, pv.line_seq, pv.id
	`, bookID)
	if err != nil {
		return fmt.Errorf("read ledger export postings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var record LedgerExportPostingRecord
		if err := rows.Scan(
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
		SELECT a.id, av.name, av.code, av.parent_account_id, av.account_class, av.account_kind, a.system_role
		FROM accounts a
		JOIN current_account_versions av ON av.account_id = a.id
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
func (r *ExportRepository) LedgerExportTotals(ctx context.Context, transaction *sql.Tx, bookID int64) (LedgerExportTotalsRecord, error) {
	var totals LedgerExportTotalsRecord
	err := transaction.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(DISTINCT je.id),
			COUNT(DISTINCT t.id),
			COUNT(DISTINCT pv.account_id),
			COUNT(DISTINCT pv.commodity_id),
			MIN(je.entry_date),
			MAX(je.entry_date)`+exportPostingSelection+`
	`, bookID).Scan(
		&totals.PostingCount,
		&totals.JournalEntryCount,
		&totals.TransactionCount,
		&totals.AccountCount,
		&totals.CommodityCount,
		&totals.EarliestEntryDate,
		&totals.LatestEntryDate,
	)
	if err != nil {
		return LedgerExportTotalsRecord{}, fmt.Errorf("read ledger export totals: %w", err)
	}

	return totals, nil
}
