package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"rekenraam/backend/internal/exact"
)

// ReportCategoryPostingRecord is a single income/expense (category) posting in
// the reporting basis, carrying the transaction's payee so one read can serve
// both the category and payee groupings.
type ReportCategoryPostingRecord struct {
	PostingID     int64
	AccountID     int64
	CommodityID   int64
	QuantityValue exact.Coefficient
	QuantityScale int
	EntryDate     string
	PayeeID       sql.NullInt64
}

// ReportCategoryPostingsParams selects the reporting basis for the spending
// report. Empty ID slices mean "no restriction on that dimension"; a non-empty
// slice is an OR-set within the dimension and ANDs with the other dimensions.
//
// AccountIDs is a counterpart filter: it keeps category postings whose *journal
// entry* also touches one of the given accounts (already resolved to include
// descendants by the caller), which is how "what did I spend from this account"
// is expressed without inferring a bank-statement classification.
type ReportCategoryPostingsParams struct {
	BookID       int64
	StartDate    string
	EndDate      string
	CategoryType string
	CategoryIDs  []int64
	PayeeIDs     []int64
	AccountIDs   []int64
	CommodityIDs []int64
}

// ReportCategoryPostings returns every posted, non-voided, non-soft-deleted
// posting on a non-system income/expense account inside the inclusive date
// range. Coefficients are returned as strings and summed in Go — never with
// SQLite SUM.
func (r *TransactionRepository) ReportCategoryPostings(ctx context.Context, params ReportCategoryPostingsParams) ([]ReportCategoryPostingRecord, error) {
	where := []string{
		"tv.book_id = ?",
		"tv.status = 'posted'",
		"t.deleted_at IS NULL",
		"je.entry_date >= ?",
		"je.entry_date <= ?",
	}
	args := []any{params.BookID, params.StartDate, params.EndDate}

	// The category account is resolved as of the posting's own entry date, so a
	// re-classified account reports under the class it had when the money moved.
	where = append(where, `EXISTS (
		SELECT 1
		FROM accounts category_account
		JOIN account_versions category_av ON category_av.account_id = category_account.id
		WHERE category_account.id = pv.account_id
			AND category_account.book_id = tv.book_id
			AND category_account.system_role IS NULL
			AND category_av.id = (
				SELECT asof_category_av.id
				FROM account_versions asof_category_av
				WHERE asof_category_av.account_id = category_account.id
					AND asof_category_av.effective_from <= je.entry_date
				ORDER BY asof_category_av.effective_from DESC, asof_category_av.version_seq DESC
				LIMIT 1
			)
			AND category_av.account_class = ?
			AND category_av.account_kind = category_av.account_class
	)`)
	args = append(args, params.CategoryType)

	if clause, clauseArgs := inClause("pv.account_id", params.CategoryIDs); clause != "" {
		where = append(where, clause)
		args = append(args, clauseArgs...)
	}
	if clause, clauseArgs := inClause("tv.payee_id", params.PayeeIDs); clause != "" {
		where = append(where, clause)
		args = append(args, clauseArgs...)
	}
	if clause, clauseArgs := inClause("pv.commodity_id", params.CommodityIDs); clause != "" {
		where = append(where, clause)
		args = append(args, clauseArgs...)
	}
	// Correlated to the category posting's own journal entry, not merely to its
	// transaction. A multi-entry transaction can hold a groceries posting in one
	// entry and touch the filtered account only in another; counting that as
	// "spent from this account" would be false, and it would make a cashflow
	// drill-down disagree with the cashflow row it came from — cashflow
	// classifies per entry for the same reason.
	if clause, clauseArgs := inClause("counterpart_pv.account_id", params.AccountIDs); clause != "" {
		where = append(where, `EXISTS (
			SELECT 1
			FROM posting_versions counterpart_pv
			WHERE counterpart_pv.journal_entry_id = je.id
				AND `+clause+`
		)`)
		args = append(args, clauseArgs...)
	}

	rows, err := r.database.QueryContext(ctx, `
		SELECT
			pv.id,
			pv.account_id,
			pv.commodity_id,
			pv.quantity_value,
			pv.quantity_scale,
			je.entry_date,
			tv.payee_id
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY je.entry_date, pv.account_day_sequence, pv.id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("read report category postings: %w", err)
	}
	defer rows.Close()

	var postings []ReportCategoryPostingRecord
	for rows.Next() {
		var posting ReportCategoryPostingRecord
		if err := rows.Scan(
			&posting.PostingID,
			&posting.AccountID,
			&posting.CommodityID,
			&posting.QuantityValue,
			&posting.QuantityScale,
			&posting.EntryDate,
			&posting.PayeeID,
		); err != nil {
			return nil, fmt.Errorf("scan report category posting: %w", err)
		}
		postings = append(postings, posting)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report category postings: %w", err)
	}

	return postings, nil
}

// ReportPayeeNames returns the current name for each requested payee.
//
// Deliberately not ListPayees: that method clamps on a limit, and an internal
// full read that silently loses rows is the bug class T-05/import-commit came
// from. This reads exactly the requested IDs with no limit.
func (r *PayeeRepository) ReportPayeeNames(ctx context.Context, bookID int64, payeeIDs []int64) (map[int64]string, error) {
	names := map[int64]string{}
	if len(payeeIDs) == 0 {
		return names, nil
	}

	clause, args := inClause("p.id", payeeIDs)
	queryArgs := append([]any{bookID}, args...)
	rows, err := r.database.QueryContext(ctx, `
		SELECT p.id, pv.name
		FROM payees p
		JOIN payee_versions pv ON pv.payee_id = p.id
		WHERE p.book_id = ?
			AND `+clause+`
			AND pv.id = (
				SELECT current_pv.id
				FROM payee_versions current_pv
				WHERE current_pv.payee_id = p.id
				ORDER BY current_pv.effective_from DESC, current_pv.version_seq DESC
				LIMIT 1
			)
	`, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("read report payee names: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan report payee name: %w", err)
		}
		names[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report payee names: %w", err)
	}
	return names, nil
}

// inClause builds a positional `column IN (?, ?, ...)` fragment. Values are
// always bound, never interpolated.
func inClause(column string, values []int64) (string, []any) {
	if len(values) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, value := range values {
		placeholders[i] = "?"
		args[i] = value
	}
	return column + " IN (" + strings.Join(placeholders, ", ") + ")", args
}

// ReportCashflowPostingRecord is one posting inside the cashflow range, carrying
// its journal entry and the account classification in effect on the entry's own
// date.
//
// The journal entry is what makes the classification well defined: entries
// balance per commodity, so a cash posting's counterparts within the same entry
// account for exactly the movement it represents. Without the entry id the
// cashflow report would be back to guessing which counterpart belongs to which
// cash leg.
type ReportCashflowPostingRecord struct {
	PostingID      int64
	JournalEntryID int64
	AccountID      int64
	CommodityID    int64
	QuantityValue  exact.Coefficient
	QuantityScale  int
	EntryDate      string
	AccountClass   string
	AccountKind    string
	SystemRole     sql.NullString
}

// ReportCashflowPostingsParams selects every posting of every journal entry that
// falls inside the range. Filtering to the cash accounts happens in Go, because
// the counterpart postings — the ones that say whether money came from income,
// went to an expense, or merely moved — are exactly the rows a SQL-side cash
// filter would discard.
type ReportCashflowPostingsParams struct {
	BookID       int64
	StartDate    string
	EndDate      string
	CommodityIDs []int64
}

// ReportCashflowPostings returns posted, non-voided, non-soft-deleted postings
// in the inclusive range, with each account's class and kind as of the entry
// date. Coefficients come back as strings and are summed in Go.
func (r *TransactionRepository) ReportCashflowPostings(ctx context.Context, params ReportCashflowPostingsParams) ([]ReportCashflowPostingRecord, error) {
	where := []string{
		"tv.book_id = ?",
		"tv.status = 'posted'",
		"t.deleted_at IS NULL",
		"je.entry_date >= ?",
		"je.entry_date <= ?",
	}
	args := []any{params.BookID, params.StartDate, params.EndDate}

	if clause, clauseArgs := inClause("pv.commodity_id", params.CommodityIDs); clause != "" {
		where = append(where, clause)
		args = append(args, clauseArgs...)
	}

	rows, err := r.database.QueryContext(ctx, `
		SELECT
			pv.id,
			pv.journal_entry_id,
			pv.account_id,
			pv.commodity_id,
			pv.quantity_value,
			pv.quantity_scale,
			je.entry_date,
			av.account_class,
			av.account_kind,
			a.system_role
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		JOIN accounts a ON a.id = pv.account_id
		JOIN account_versions av ON av.id = (
			SELECT asof_av.id
			FROM account_versions asof_av
			WHERE asof_av.account_id = a.id
				AND asof_av.effective_from <= je.entry_date
			ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC
			LIMIT 1
		)
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY je.entry_date, pv.journal_entry_id, pv.id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("read report cashflow postings: %w", err)
	}
	defer rows.Close()

	var postings []ReportCashflowPostingRecord
	for rows.Next() {
		var posting ReportCashflowPostingRecord
		if err := rows.Scan(
			&posting.PostingID,
			&posting.JournalEntryID,
			&posting.AccountID,
			&posting.CommodityID,
			&posting.QuantityValue,
			&posting.QuantityScale,
			&posting.EntryDate,
			&posting.AccountClass,
			&posting.AccountKind,
			&posting.SystemRole,
		); err != nil {
			return nil, fmt.Errorf("scan report cashflow posting: %w", err)
		}
		postings = append(postings, posting)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report cashflow postings: %w", err)
	}

	return postings, nil
}
