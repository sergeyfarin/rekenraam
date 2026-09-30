package db

import (
	"context"
	"database/sql"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/exact"
)

func TestInvestmentReplaySimulationReallocatesBasisWithoutChangingPostedHistory(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	tx, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer rollbackTx(ctx, tx)
	intents, err := investmentReplayIntentsQuery(ctx, tx, 1, 15, 2, 1, "long")
	require.NoError(t, err)
	intents[0].AmountValue = exact.New(120000) // correct the older buy from 1000 to 1200 EUR

	for _, method := range []string{"fifo", "lifo", "average_cost", "specific_lot"} {
		t.Run(method, func(t *testing.T) {
			candidate := append([]InvestmentReplayIntent(nil), intents...)
			candidate[2].CostBasisMethod = method
			if method == "specific_lot" {
				candidate[2].SpecificLots = []LotAllocation{
					{LotID: 1, QuantityValue: exact.New(1000), QuantityScale: 2},
					{LotID: 2, QuantityValue: exact.New(250), QuantityScale: 2},
				}
			}
			projection, err := simulateInvestmentReplayTx(ctx, tx, 1, 15, 2, 1, candidate)
			require.NoError(t, err)
			require.Len(t, projection.Disposals, 1)
			require.Len(t, projection.Lots, 2)
			if method == "average_cost" {
				require.Equal(t, "average_cost", projection.MethodFamily)
			} else {
				require.Equal(t, "individual_lot", projection.MethodFamily)
			}
			disposed := exact.NewScaledInt()
			for _, allocation := range projection.Disposals[0].Allocations {
				disposed.AddInt64(allocation.CostBasisValue, allocation.CostBasisScale)
			}
			require.Positive(t, disposed.Sign())
			if method == "fifo" || method == "specific_lot" {
				require.Zero(t, disposed.Cmp(exact.ScaledIntFromInt64(1500, 0)))
			}
			assertOriginalReplayRowsUnchanged(t, tx, 4)
		})
	}
}

func TestInvestmentReplaySimulationNamesImpossibleDependentDisposal(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	tx, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer rollbackTx(ctx, tx)
	intents, err := investmentReplayIntentsQuery(ctx, tx, 1, 15, 2, 1, "long")
	require.NoError(t, err)
	intents[2].CostBasisMethod = "specific_lot"
	intents[2].SpecificLots = []LotAllocation{{LotID: 2, QuantityValue: exact.New(1250), QuantityScale: 2}}
	_, err = simulateInvestmentReplayTx(ctx, tx, 1, 15, 2, 1, intents)
	require.ErrorContains(t, err, "operation 3 decision 1")
	require.ErrorIs(t, err, ErrInsufficientLots)
	assertOriginalReplayRowsUnchanged(t, tx, 4)
}

func TestInvestmentReplaySimulationRefusesLotWithoutOpeningFact(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	repo := NewInvestmentRepository(database)
	_, err := repo.CreateLot(ctx, CreateInvestmentLotParams{
		BookID: 1, AccountID: 15, CommodityID: 2, CostCommodityID: 1,
		OpenedOn: "2026-06-01", QuantityValue: exact.New(1),
		CostBasisValue: 10000, CostBasisScale: 2, MetadataJSON: "{}",
		CreatedAt: "2026-06-01T10:00:00Z", CreatedByUserID: 1,
		OriginType: "internal", Operation: "investment.test.opening", EventKind: "acquisition",
	})
	require.NoError(t, err)
	tx, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer rollbackTx(ctx, tx)
	intents, err := investmentReplayIntentsQuery(ctx, tx, 1, 15, 2, 1, "long")
	require.NoError(t, err)
	_, err = simulateInvestmentReplayTx(ctx, tx, 1, 15, 2, 1, intents)
	require.ErrorContains(t, err, "lacks an immutable opening fact")
	assertOriginalReplayRowsUnchanged(t, tx, 5)
}

func TestInvestmentReplaySimulationRejectsPositionBasisOverflow(t *testing.T) {
	ctx := context.Background()
	database := seedReplayTestBook(t)
	tx, err := database.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer rollbackTx(ctx, tx)
	intents, err := investmentReplayIntentsQuery(ctx, tx, 1, 15, 2, 1, "long")
	require.NoError(t, err)
	intents[0].AmountValue = exact.New(math.MaxInt64)
	_, err = simulateInvestmentReplayTx(ctx, tx, 1, 15, 2, 1, intents)
	require.ErrorIs(t, err, ErrInvestmentBasisRange)
	assertOriginalReplayRowsUnchanged(t, tx, 4)
}

func assertOriginalReplayRowsUnchanged(t *testing.T, tx *sql.Tx, expectedEvents int) {
	t.Helper()
	var remaining string
	require.NoError(t, tx.QueryRow(`SELECT remaining_quantity_value FROM current_investment_lots WHERE id = 2`).Scan(&remaining))
	require.Equal(t, "250", remaining)
	var basis string
	require.NoError(t, tx.QueryRow(`SELECT disposed_basis_value FROM investment_disposal_decisions WHERE id = 1`).Scan(&basis))
	require.Equal(t, "130000", basis)
	var events int
	require.NoError(t, tx.QueryRow(`SELECT COUNT(*) FROM investment_lot_events WHERE book_id = 1`).Scan(&events))
	require.Equal(t, expectedEvents, events)
}
