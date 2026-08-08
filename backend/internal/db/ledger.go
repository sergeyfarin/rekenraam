package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"rekenraam/backend/internal/exact"
)

type LedgerAccountRecord struct {
	ID              int64
	BookID          int64
	SystemRole      sql.NullString
	Status          string
	Code            sql.NullString
	Name            sql.NullString
	AccountClass    string
	AccountKind     string
	ParentAccountID sql.NullInt64
	AllowsPostings  bool
}

type LedgerPostingRecord struct {
	PostingID     int64
	AccountID     int64
	CommodityID   int64
	QuantityValue exact.Coefficient
	QuantityScale int
	EntryDate     string
	// TransactionID groups postings that belong to the same transaction.
	TransactionID int64
	// JournalEntryID groups postings that balance against each other.
	//
	// This, not TransactionID, is the unit cashflow classification works over:
	// validateBalanced enforces balance **per journal entry**, and an entry
	// carries a single entry_date. So the postings sharing a JournalEntryID sum
	// to zero per commodity *and* fall in the same reporting bucket — which is
	// what lets cashflow decompose a bucket exactly instead of guessing which
	// counterpart funded which leg.
	JournalEntryID int64
	// PayeeID is the linked payee, when the transaction names one.
	PayeeID sql.NullInt64
	// PayeeName is the transaction's free-text payee. It is the fallback label
	// for an unlinked payee and is not kept in sync with a linked payee's
	// current name, so a caller that has PayeeID should prefer the payee
	// record.
	PayeeName sql.NullString
}

func (r *TransactionRepository) LedgerAccountsAsOf(ctx context.Context, bookID int64, asOf string) ([]LedgerAccountRecord, error) {
	rows, err := r.database.QueryContext(ctx, `
		SELECT
			a.id,
			a.book_id,
			a.system_role,
			av.status,
			av.code,
			av.name,
			av.account_class,
			av.account_kind,
			av.parent_account_id,
			av.allows_postings
		FROM accounts a
		JOIN account_versions av ON av.account_id = a.id
		WHERE a.book_id = ?
			AND av.id = (
				SELECT asof_av.id
				FROM account_versions asof_av
				WHERE asof_av.account_id = a.id
					AND asof_av.effective_from <= ?
				ORDER BY asof_av.effective_from DESC, asof_av.version_seq DESC
				LIMIT 1
			)
		ORDER BY av.account_class, av.name COLLATE NOCASE, a.id
	`, bookID, asOf)
	if err != nil {
		return nil, fmt.Errorf("read ledger accounts: %w", err)
	}
	defer rows.Close()

	var accounts []LedgerAccountRecord
	for rows.Next() {
		var account LedgerAccountRecord
		var allowsPostings int
		if err := rows.Scan(
			&account.ID,
			&account.BookID,
			&account.SystemRole,
			&account.Status,
			&account.Code,
			&account.Name,
			&account.AccountClass,
			&account.AccountKind,
			&account.ParentAccountID,
			&allowsPostings,
		); err != nil {
			return nil, fmt.Errorf("scan ledger account: %w", err)
		}
		account.AllowsPostings = allowsPostings == 1
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ledger accounts: %w", err)
	}

	return accounts, nil
}

func (r *TransactionRepository) LedgerPostingsThrough(ctx context.Context, bookID int64, asOf string, status string) ([]LedgerPostingRecord, error) {
	return r.ledgerPostings(ctx, ledgerPostingQuery{
		BookID:     bookID,
		Status:     status,
		BeforeDate: asOf,
	})
}

func (r *TransactionRepository) LedgerCategoryPostings(ctx context.Context, bookID int64, afterDate string, beforeDate string, status string, categoryType string) ([]LedgerPostingRecord, error) {
	query := ledgerPostingQuery{
		BookID:       bookID,
		Status:       status,
		AfterDate:    afterDate,
		BeforeDate:   beforeDate,
		CategoryOnly: true,
		CategoryType: categoryType,
	}
	return r.ledgerPostings(ctx, query)
}

func (r *TransactionRepository) AccountRegisterRunningPostings(ctx context.Context, params ListTransactionsParams) ([]LedgerPostingRecord, error) {
	return r.ledgerPostings(ctx, ledgerPostingQuery{
		BookID:     params.BookID,
		AccountID:  params.AccountID,
		Status:     params.Status,
		BeforeDate: params.BeforeDate,
	})
}

type ledgerPostingQuery struct {
	BookID       int64
	AccountID    int64
	Status       string
	AfterDate    string
	BeforeDate   string
	CategoryOnly bool
	CategoryType string
}

func (r *TransactionRepository) ledgerPostings(ctx context.Context, query ledgerPostingQuery) ([]LedgerPostingRecord, error) {
	where := []string{"tv.book_id = ?", "t.deleted_at IS NULL"}
	args := []any{query.BookID}
	if query.Status != "" {
		where = append(where, "tv.status = ?")
		args = append(args, query.Status)
	}
	if query.AccountID > 0 {
		where = append(where, "pv.account_id = ?")
		args = append(args, query.AccountID)
	}
	if query.AfterDate != "" {
		where = append(where, "je.entry_date >= ?")
		args = append(args, query.AfterDate)
	}
	if query.BeforeDate != "" {
		where = append(where, "je.entry_date <= ?")
		args = append(args, query.BeforeDate)
	}
	if query.CategoryType != "" {
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
		args = append(args, query.CategoryType)
	} else if query.CategoryOnly {
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
				AND category_av.account_class IN ('income', 'expense')
				AND category_av.account_kind = category_av.account_class
		)`)
	}

	rows, err := r.database.QueryContext(ctx, `
		SELECT
			pv.id,
			pv.account_id,
			pv.commodity_id,
			pv.quantity_value,
			pv.quantity_scale,
			je.entry_date,
			tv.transaction_id,
			je.id,
			tv.payee_id,
			tv.payee_name
		FROM current_transaction_versions tv
		JOIN transactions t ON t.id = tv.transaction_id
		JOIN journal_entries je ON je.transaction_version_id = tv.id
		JOIN posting_versions pv ON pv.journal_entry_id = je.id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY je.entry_date, pv.account_day_sequence, pv.id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("read ledger postings: %w", err)
	}
	defer rows.Close()

	var postings []LedgerPostingRecord
	for rows.Next() {
		var posting LedgerPostingRecord
		if err := rows.Scan(
			&posting.PostingID,
			&posting.AccountID,
			&posting.CommodityID,
			&posting.QuantityValue,
			&posting.QuantityScale,
			&posting.EntryDate,
			&posting.TransactionID,
			&posting.JournalEntryID,
			&posting.PayeeID,
			&posting.PayeeName,
		); err != nil {
			return nil, fmt.Errorf("scan ledger posting: %w", err)
		}
		postings = append(postings, posting)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ledger postings: %w", err)
	}

	return postings, nil
}

func (r *TransactionRepository) EnsureAccountInBook(ctx context.Context, bookID int64, accountID int64) error {
	var exists int
	if err := r.database.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM accounts
			WHERE book_id = ?
				AND id = ?
		)
	`, bookID, accountID).Scan(&exists); err != nil {
		return fmt.Errorf("check account book: %w", err)
	}
	if exists == 0 {
		return ErrNotFound
	}
	return nil
}
