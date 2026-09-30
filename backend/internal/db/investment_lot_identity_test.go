package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvestmentLotOpeningIdentityIsImmutable(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	before := captureLedgerState(t, database)
	for _, test := range []struct{ column, value string }{
		{"id", "999999"}, {"book_id", "2"}, {"account_id", "16"},
		{"commodity_id", "1"}, {"opened_on", "'2026-02-03'"},
		{"source_transaction_id", "NULL"}, {"position_side", "'short'"},
		{"quantity_value", "'11'"}, {"quantity_scale", "1"},
		{"cost_basis_value", "'100001'"}, {"cost_basis_scale", "3"},
		{"cost_commodity_id", "2"}, {"metadata_json", "'{\"changed\":true}'"},
		{"created_at", "'2026-09-30T00:00:00Z'"}, {"created_by_user_id", "2"},
		{"created_audit_event_id", "NULL"},
	} {
		t.Run(test.column, func(t *testing.T) {
			tx, err := database.BeginTx(ctx, nil)
			require.NoError(t, err)
			_, mutationErr := tx.ExecContext(ctx, "UPDATE investment_lots SET "+test.column+" = "+test.value+" WHERE id = 1")
			require.NoError(t, tx.Rollback())
			require.ErrorContains(t, mutationErr, "investment lot opening facts are immutable")
		})
	}
	require.Equal(t, before, captureLedgerState(t, database), "refused mutations must preserve all durable rows")
	_, err := database.ExecContext(ctx, "DELETE FROM investment_lots WHERE id = 1")
	require.ErrorContains(t, err, "investment lots are immutable")
}

func TestInvestmentLotProjectionUpdatesPreserveOpeningIdentity(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	// Identity and projection writes target their own tables.
	_, err := database.ExecContext(ctx, `UPDATE investment_lots SET quantity_value = quantity_value WHERE id = 2`)
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `UPDATE investment_lot_state SET
		status = 'closed', remaining_quantity_value = '0', remaining_quantity_scale = 0,
		remaining_cost_basis_value = '0', remaining_cost_basis_scale = 2,
		updated_at = '2026-09-30T00:00:00Z', updated_by_user_id = 1, updated_audit_event_id = 31
		WHERE lot_id = 2`)
	require.NoError(t, err)
	var quantity, basis, remaining string
	require.NoError(t, database.QueryRowContext(ctx, `SELECT quantity_value, cost_basis_value,
		remaining_quantity_value FROM current_investment_lots WHERE id = 2`).Scan(&quantity, &basis, &remaining))
	require.Equal(t, "5", quantity)
	require.Equal(t, "60000", basis)
	require.Equal(t, "0", remaining)
}

func TestInvestmentLotSchemaSeparatesProjection(t *testing.T) {
	database := seedReplayTestBook(t)
	var count int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'investment_lot_state'`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM pragma_table_info('investment_lots') WHERE name IN ('status', 'remaining_quantity_value', 'remaining_cost_basis_value', 'updated_at')`).Scan(&count))
	require.Zero(t, count)
}

func TestInvestmentLotStateCannotMoveAcrossLotsOrBooks(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	for _, assignment := range []string{"book_id = 2", "lot_id = 999999"} {
		_, err := database.ExecContext(ctx, "UPDATE investment_lot_state SET "+assignment+" WHERE lot_id = 1")
		require.ErrorContains(t, err, "must belong to its lot and audit book")
	}
	_, err := database.ExecContext(ctx, `INSERT INTO investment_lot_state
 (lot_id, book_id, status, remaining_quantity_value, remaining_quantity_scale,
 remaining_cost_basis_value, remaining_cost_basis_scale, updated_at, updated_by_user_id)
 VALUES (999999, 1, 'closed', '0', 0, '0', 2, '2026-09-30T00:00:00Z', 1)`)
	require.ErrorContains(t, err, "must belong to its lot and audit book")
}
