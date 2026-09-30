package db

import (
	"context"
	"database/sql"
	"fmt"

	"rekenraam/backend/internal/exact"
)

// Current commands create one disposal per posted version. They attribute each
// complete cost-currency clearing leg, including separately paid included fees.
// A future compound command must instead supply explicit portions per decision.
func createDisposalClearingAllocationsTx(ctx context.Context, tx *sql.Tx, bookID, decisionID, versionID, currencyID int64, proceedsValue exact.Coefficient, proceedsScale int) error {
	rows, err := tx.QueryContext(ctx, `SELECT pv.id, pv.quantity_value, pv.quantity_scale
		FROM posting_versions pv JOIN accounts a ON a.id = pv.account_id AND a.book_id = pv.book_id
		WHERE pv.book_id = ? AND pv.transaction_version_id = ? AND pv.commodity_id = ?
			AND a.system_role = 'commodity_trading' ORDER BY pv.id`, bookID, versionID, currencyID)
	if err != nil {
		return fmt.Errorf("read disposal clearing legs: %w", err)
	}
	type allocation struct {
		postingID int64
		value     exact.Coefficient
		scale     int
	}
	var allocations []allocation
	total := exact.NewScaledInt()
	for rows.Next() {
		var item allocation
		if err := rows.Scan(&item.postingID, &item.value, &item.scale); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan disposal clearing leg: %w", err)
		}
		item.value = item.value.Negated()
		total.AddCoefficient(item.value, item.scale)
		allocations = append(allocations, item)
	}
	iterationErr := rows.Err()
	closeErr := rows.Close()
	if iterationErr != nil {
		return fmt.Errorf("iterate disposal clearing legs: %w", iterationErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close disposal clearing legs: %w", closeErr)
	}
	if total.Cmp(exact.ScaledIntFromCoefficient(proceedsValue, proceedsScale)) != 0 {
		return fmt.Errorf("disposal proceeds disagree with clearing allocation total")
	}
	for _, item := range allocations {
		if _, err := tx.ExecContext(ctx, `INSERT INTO investment_disposal_clearing_allocations
			(book_id, decision_id, posting_version_id, proceeds_value, proceeds_scale)
			VALUES (?, ?, ?, ?, ?)`, bookID, decisionID, item.postingID, item.value, item.scale); err != nil {
			return fmt.Errorf("insert disposal clearing allocation: %w", err)
		}
	}
	return nil
}
