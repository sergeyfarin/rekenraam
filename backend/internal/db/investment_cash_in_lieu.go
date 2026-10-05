package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Cash in lieu (slice 5, T-147) runs through the ordinary long disposal
// writer: the fraction is disposed under the holding's election with its
// cash as proceeds, the security legs on the disposal date and the cash on
// the payment date (the trade's settlement date). Its link to the split it
// settles is written inside the same transaction, after the disposal, where
// the facts trigger rechecks that the split is still effective.

var ErrCashInLieuSplitUnavailable = errors.New("the split this cash in lieu settles is no longer effective or does not match the holding")

// CashInLieuFactWriter returns the post-write step that links a committed
// cash-in-lieu disposal to its split.
func CashInLieuFactWriter(ctx context.Context, bookID, splitOperationID int64, paymentOn string) func(*sql.Tx, int64) error {
	return func(tx *sql.Tx, transactionID int64) error {
		var operationID, auditEventID int64
		if err := tx.QueryRowContext(ctx, `SELECT o.id, o.created_audit_event_id FROM investment_operations o
			JOIN investment_operation_journal_links link ON link.operation_id = o.id AND link.role = 'primary'
			JOIN transaction_versions v ON v.id = link.transaction_version_id
			WHERE o.book_id = ? AND v.transaction_id = ? AND o.operation_kind = 'cash_in_lieu'`,
			bookID, transactionID).Scan(&operationID, &auditEventID); err != nil {
			return fmt.Errorf("read cash in lieu operation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_cash_in_lieu_facts
			(operation_id, book_id, split_operation_id, payment_on, created_audit_event_id)
			VALUES (?, ?, ?, ?, ?)`, operationID, bookID, splitOperationID, paymentOn, auditEventID); err != nil {
			if strings.Contains(err.Error(), "investment cash in lieu fact is outside its disposal or split") {
				return ErrCashInLieuSplitUnavailable
			}
			return fmt.Errorf("record cash in lieu fact: %w", err)
		}
		return nil
	}
}

// CashInLieuReplacementFactWriter links a replacement cash in lieu (T-150)
// to the split its predecessor settled; the replacement writer hands it the
// new operation and audit event.
func CashInLieuReplacementFactWriter(ctx context.Context, bookID, splitOperationID int64, paymentOn string) func(*sql.Tx, int64, int64) error {
	return func(tx *sql.Tx, operationID, auditEventID int64) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_cash_in_lieu_facts
			(operation_id, book_id, split_operation_id, payment_on, created_audit_event_id)
			VALUES (?, ?, ?, ?, ?)`, operationID, bookID, splitOperationID, paymentOn, auditEventID); err != nil {
			if strings.Contains(err.Error(), "investment cash in lieu fact is outside its disposal or split") {
				return ErrCashInLieuSplitUnavailable
			}
			return fmt.Errorf("record replacement cash in lieu fact: %w", err)
		}
		return nil
	}
}

// CashInLieuSplitTransactionID returns the posted split transaction a cash
// in lieu settles.
func (r *InvestmentRepository) CashInLieuSplitTransactionID(ctx context.Context, bookID, transactionID int64) (int64, error) {
	var splitTransactionID int64
	err := r.database.QueryRowContext(ctx, `SELECT split_version.transaction_id
		FROM investment_cash_in_lieu_facts f
		JOIN investment_operation_journal_links link ON link.operation_id = f.operation_id AND link.role = 'primary'
		JOIN transaction_versions v ON v.id = link.transaction_version_id
		JOIN investment_operation_journal_links split_link ON split_link.operation_id = f.split_operation_id
			AND split_link.role = 'primary'
		JOIN transaction_versions split_version ON split_version.id = split_link.transaction_version_id
		WHERE f.book_id = ? AND v.transaction_id = ?`, bookID, transactionID).Scan(&splitTransactionID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("read cash in lieu split: %w", err)
	}
	return splitTransactionID, nil
}
