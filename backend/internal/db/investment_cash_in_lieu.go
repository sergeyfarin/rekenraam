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

// Cash-in-lieu previews include the immutable split fact and its transactional
// guard. No source acceptance runs, and every temporary row is rolled back.
func (r *InvestmentRepository) SimulateCashInLieu(ctx context.Context, journal CreateTransactionParams, disposal DisposeLotsParams, splitOperationID int64, paymentOn string) (SimulatedInvestmentWrite, []LotDisposalRecord, DisposalDecisionRecord, error) {
	transaction, allocations, decision, err := r.writeTransactionAndDisposeLots(ctx, journal, disposal, nil, CashInLieuFactWriter(ctx, journal.BookID, splitOperationID, paymentOn), true)
	if err != nil {
		return SimulatedInvestmentWrite{}, nil, DisposalDecisionRecord{}, err
	}
	for index := range allocations {
		allocations[index].EventID = 0
	}
	decision.ID, decision.TransactionID, decision.TransactionVersionID, decision.AuditEventID = 0, 0, 0, 0
	decision.Allocations = allocations
	return simulatedInvestmentWrite(transaction), allocations, decision, nil
}

func (r *InvestmentRepository) SimulateCashInLieuReplacement(ctx context.Context, expected SaleOperationRecord, inverse, replacement CreateTransactionParams, disposal DisposeLotsParams, splitOperationID int64, paymentOn string) (SimulatedInvestmentWrite, []LotDisposalRecord, DisposalDecisionRecord, error) {
	reversed, corrected, allocations, decision, err := r.replaceSale(ctx, expected, inverse, replacement, disposal, nil, CashInLieuReplacementFactWriter(ctx, inverse.BookID, splitOperationID, paymentOn), true)
	if err != nil {
		return SimulatedInvestmentWrite{}, nil, DisposalDecisionRecord{}, err
	}
	for index := range allocations {
		allocations[index].EventID = 0
	}
	decision.ID, decision.TransactionID, decision.TransactionVersionID, decision.AuditEventID = 0, 0, 0, 0
	decision.Allocations = allocations
	return simulatedInvestmentWrite(reversed, corrected), allocations, decision, nil
}

// CashInLieuAvailableLots returns dated pre-disposal quantities. A replacement
// removes its predecessor and keeps its original same-day root slot.
func (r *InvestmentRepository) CashInLieuAvailableLots(ctx context.Context, bookID int64, split SplitOperationRecord, currencyID int64, date string, replacingTransactionID int64) ([]InvestmentTradeCorrectionAvailableLot, error) {
	tx, err := r.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rollbackTx(ctx, tx)
	intents, err := investmentReplayIntentsQuery(ctx, tx, bookID, split.AccountID, split.CommodityID, currencyID, "long")
	if err != nil {
		return nil, err
	}
	orderID := int64(0)
	var replacingID int64
	if replacingTransactionID > 0 {
		if err := tx.QueryRowContext(ctx, `SELECT f.operation_id FROM investment_cash_in_lieu_facts f
 JOIN investment_operation_journal_links l ON l.operation_id = f.operation_id AND l.role = 'primary'
 JOIN transaction_versions v ON v.id = l.transaction_version_id
 JOIN effective_investment_operations o ON o.id = f.operation_id
 WHERE f.book_id = ? AND v.transaction_id = ? AND f.split_operation_id = ?`, bookID, replacingTransactionID, split.OperationID).Scan(&replacingID); err != nil {
			return nil, ErrNotFound
		}
		orders, err := investmentReplayOrderOperationIDsQuery(ctx, tx, bookID)
		if err != nil {
			return nil, err
		}
		orderID = orders[replacingID]
	}
	before := make([]InvestmentReplayIntent, 0, len(intents))
	opened := make(map[int64]string)
	for _, intent := range intents {
		if intent.OperationID == replacingID || intent.EventDate > date || (orderID > 0 && intent.EventDate == date && intent.OrderOperationID >= orderID) {
			continue
		}
		before = append(before, intent)
		if intent.Kind == "opening" {
			opened[intent.LotID] = intent.EventDate
		}
	}
	projection, err := simulateInvestmentReplayTx(ctx, tx, bookID, split.AccountID, split.CommodityID, currencyID, before)
	if err != nil {
		return nil, err
	}
	lots := make([]InvestmentTradeCorrectionAvailableLot, 0, len(projection.Lots))
	for _, lot := range projection.Lots {
		if lot.RemainingQuantityValue.Sign() > 0 && opened[lot.LotID] != "" {
			lots = append(lots, InvestmentTradeCorrectionAvailableLot{LotID: lot.LotID, OpenedOn: opened[lot.LotID], QuantityValue: lot.RemainingQuantityValue.String(), QuantityScale: lot.RemainingQuantityScale})
		}
	}
	return lots, nil
}
