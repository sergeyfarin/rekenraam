package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
	QuantityValue int64
	QuantityScale int
	EntryDate     string
}

func (r *TransactionRepository) LedgerAccountsAsOf(ctx context.Context, bookID int64, asOf string) ([]LedgerAccountRecord, error) {
	_ = asOf
	rows, err := r.database.QueryContext(ctx, `
		SELECT
			a.id,
			a.book_id,
			a.system_role,
			a.status,
			a.code,
			a.name,
			a.account_class,
			a.account_kind,
			a.parent_account_id,
			a.allows_postings
		FROM accounts a
		WHERE a.book_id = ?
		ORDER BY a.account_class, a.name COLLATE NOCASE, a.id
	`, bookID)
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
	where := []string{"tv.book_id = ?"}
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
			WHERE category_account.id = pv.account_id
				AND category_account.book_id = tv.book_id
				AND category_account.system_role IS NULL
				AND category_account.account_class = ?
				AND category_account.account_kind = category_account.account_class
		)`)
		args = append(args, query.CategoryType)
	} else if query.CategoryOnly {
		where = append(where, `EXISTS (
			SELECT 1
			FROM accounts category_account
			WHERE category_account.id = pv.account_id
				AND category_account.book_id = tv.book_id
				AND category_account.system_role IS NULL
				AND category_account.account_class IN ('income', 'expense')
				AND category_account.account_kind = category_account.account_class
		)`)
	}

	rows, err := r.database.QueryContext(ctx, `
		SELECT
			pv.id,
			pv.account_id,
			pv.commodity_id,
			pv.quantity_value,
			pv.quantity_scale,
			je.entry_date
		FROM transactions tv
		JOIN journal_entries je ON je.transaction_id = tv.id
		JOIN postings pv ON pv.journal_entry_id = je.id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY je.entry_date, pv.id
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
