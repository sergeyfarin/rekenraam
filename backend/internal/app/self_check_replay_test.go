package app

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-134: self-check replays every long position from its effective intents
// and compares the result with the stored projection. Each damage below is
// internally consistent, so the consolidated lot and foundation readers still
// pass; only the replay verifier sees that history no longer produces it.

func requireConsolidatedReadersPass(t *testing.T, run SelfCheckRun) {
	t.Helper()
	for _, check := range []string{CheckLotReconciliation, CheckInvestmentFoundation} {
		result := resultFor(t, run, check)
		assert.Equalf(t, SelfCheckPassed, result.Status, "%s: %s", check, result.Summary)
	}
}

// An average-cost pool whose remaining basis sits on the wrong lot still sums
// to the right position basis, so lot reconciliation passes.
func TestReplaySelfCheckReportsStaleLotState(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	setHoldingCostBasisMethod(t, f, "average_cost")
	first := buyOn(t, f, "2026-01-01", 2, 2000)
	second := buyOn(t, f, "2026-01-02", 2, 4000)
	sale := sellInput(f, "2026-02-01", 1)
	sale.CostBasisMethod = "average_cost"
	_, err := f.investmentService.Sell(context.Background(), sale)
	require.NoError(t, err)
	run := mustRunInvestmentSelfCheck(t, f)
	require.Equal(t, SelfCheckPassed, resultFor(t, run, CheckInvestmentReplay).Status, resultFor(t, run, CheckInvestmentReplay).Summary)

	moveRemainingBasis(t, f, *first.LotID, *second.LotID, 100)
	run = mustRunInvestmentSelfCheck(t, f)
	requireConsolidatedReadersPass(t, run)
	replay := resultFor(t, run, CheckInvestmentReplay)
	assert.Equal(t, SelfCheckFailed, replay.Status)
	assert.Equal(t, int64(2), replay.FindingCount)
	assert.ElementsMatch(t, []int64{*first.LotID, *second.LotID}, replay.Sample)
	assert.Contains(t, replay.Summary, "2 lot balances")
}

// moveRemainingBasis shifts basis (in the lot state's own scale) from one
// lot's remaining projection to another's.
func moveRemainingBasis(t *testing.T, f *investmentsTestFixture, from, to, amount int64) {
	t.Helper()
	for _, step := range []struct {
		lotID int64
		delta int64
	}{{from, -amount}, {to, amount}} {
		var value exact.Coefficient
		var scale int
		require.NoError(t, f.database.QueryRow(`SELECT remaining_cost_basis_value, remaining_cost_basis_scale
			FROM investment_lot_state WHERE lot_id = ?`, step.lotID).Scan(&value, &scale))
		total := exact.ScaledIntFromCoefficient(value, scale)
		total.AddInt64(step.delta, scale)
		updated, err := total.Coefficient()
		require.NoError(t, err)
		_, err = f.database.Exec(`UPDATE investment_lot_state SET remaining_cost_basis_value = ? WHERE lot_id = ?`,
			updated, step.lotID)
		require.NoError(t, err)
	}
}

// A disposal revision that conserves its own totals and matches the lot
// state, but takes from a lot FIFO would not choose: what a skipped dependent
// replay leaves behind.
func TestReplaySelfCheckReportsStaleEffectiveRevision(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	first := buyOn(t, f, "2026-01-01", 1, 1000)
	second := buyOn(t, f, "2026-01-02", 1, 2000)
	sale, err := f.investmentService.Sell(context.Background(), sellInput(f, "2026-02-01", 1))
	require.NoError(t, err)

	var decisionID, operationID, auditEventID int64
	var proceeds exact.Coefficient
	var proceedsScale int
	require.NoError(t, f.database.QueryRow(`SELECT d.id, d.operation_id, d.created_audit_event_id,
		a.proceeds_value, a.proceeds_scale
		FROM investment_disposal_decisions d JOIN investment_disposal_allocations a ON a.decision_id = d.id
		WHERE d.transaction_id = ?`, sale.Transaction.ID).Scan(&decisionID, &operationID, &auditEventID,
		&proceeds, &proceedsScale))
	result, err := f.database.Exec(`INSERT INTO investment_disposal_revisions (book_id, decision_id, revision_seq,
		caused_by_operation_id, disposed_basis_value, disposed_basis_scale, created_at, created_audit_event_id)
		VALUES (1, ?, 2, ?, '2000', 2, '2026-10-04T00:00:00Z', ?)`, decisionID, operationID, auditEventID)
	require.NoError(t, err)
	revisionID, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = f.database.Exec(`INSERT INTO investment_disposal_revision_allocations (book_id, revision_id,
		allocation_seq, lot_id, quantity_value, quantity_scale, cost_basis_value, cost_basis_scale,
		proceeds_value, proceeds_scale) VALUES (1, ?, 1, ?, '1', 0, '2000', 2, ?, ?)`,
		revisionID, *second.LotID, proceeds, proceedsScale)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_lot_state SET status = 'open', remaining_quantity_value = '1',
		remaining_quantity_scale = 0, remaining_cost_basis_value = '1000', remaining_cost_basis_scale = 2
		WHERE lot_id = ?`, *first.LotID)
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_lot_state SET status = 'closed', remaining_quantity_value = '0',
		remaining_cost_basis_value = '0' WHERE lot_id = ?`, *second.LotID)
	require.NoError(t, err)

	run := mustRunInvestmentSelfCheck(t, f)
	requireConsolidatedReadersPass(t, run)
	replay := resultFor(t, run, CheckInvestmentReplay)
	assert.Equal(t, SelfCheckFailed, replay.Status)
	assert.Contains(t, replay.Summary, "1 disposal allocations")
	assert.Contains(t, replay.Summary, "decision #")
	assert.Contains(t, replay.Sample, decisionID)
	assert.Contains(t, replay.Sample, *first.LotID)
}

// A stale internal-transfer revision is named by the transfer.
func TestReplaySelfCheckReportsStaleTransferBasis(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	destinationID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
	buy := buyOn(t, f, "2026-05-01", 3, 3000)
	transfer := chainTransfer(t, f, f.holdingAccountID, destinationID, *buy.LotID, "2026-06-01")
	_, err := acknowledgedReplaceBuy(context.Background(), f.investmentService, replaceBuyPrice(f, buy, "2026-05-01", 3, 3300))
	require.NoError(t, err)
	requireInvestmentSelfCheckPasses(t, f)

	// Drop the latest revision's effect as a skipped propagation would: the
	// link keeps carrying the old basis, with the destination consistent.
	var revisedBasis exact.Coefficient
	var revisedScale int
	var sourceLotID int64
	require.NoError(t, f.database.QueryRow(`SELECT carried_basis_value, carried_basis_scale, source_lot_id
		FROM investment_transfer_link_revisions`).Scan(&revisedBasis, &revisedScale, &sourceLotID))
	_, err = f.database.Exec(`DROP TRIGGER investment_transfer_link_revisions_no_update`)
	require.NoError(t, err)
	var basis exact.Coefficient
	var scale int
	require.NoError(t, f.database.QueryRow(`SELECT carried_basis_value, carried_basis_scale
		FROM investment_transfer_lot_links WHERE operation_id = ?`,
		transferOperationID(t, f, transfer.Transaction.ID)).Scan(&basis, &scale))
	_, err = f.database.Exec(`UPDATE investment_transfer_link_revisions SET carried_basis_value = ?, carried_basis_scale = ?`, basis, scale)
	require.NoError(t, err)
	var stateScale int
	require.NoError(t, f.database.QueryRow(`SELECT remaining_cost_basis_scale FROM investment_lot_state WHERE lot_id = ?`,
		transfer.DestinationLotIDs[0]).Scan(&stateScale))
	carried := exact.ScaledIntFromCoefficient(basis, scale)
	carried.Align(stateScale)
	destinationBasis, err := carried.Coefficient()
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_lot_state SET remaining_cost_basis_value = ? WHERE lot_id = ?`,
		destinationBasis, transfer.DestinationLotIDs[0])
	require.NoError(t, err)
	// The source keeps the basis the transfer no longer carries, so its
	// position basis still matches its effective events.
	var sourceBasis exact.Coefficient
	var sourceScale int
	require.NoError(t, f.database.QueryRow(`SELECT remaining_cost_basis_value, remaining_cost_basis_scale
		FROM investment_lot_state WHERE lot_id = ?`, sourceLotID).Scan(&sourceBasis, &sourceScale))
	source := exact.ScaledIntFromCoefficient(sourceBasis, sourceScale)
	source.AddScaled(exact.ScaledIntFromCoefficient(revisedBasis, revisedScale))
	source.SubScaled(exact.ScaledIntFromCoefficient(basis, scale))
	source.Align(sourceScale)
	if source.Scale() != sourceScale {
		t.Fatalf("source basis needs scale %d", source.Scale())
	}
	sourceValue, err := source.Coefficient()
	require.NoError(t, err)
	_, err = f.database.Exec(`UPDATE investment_lot_state SET remaining_cost_basis_value = ? WHERE lot_id = ?`,
		sourceValue, sourceLotID)
	require.NoError(t, err)

	run := mustRunInvestmentSelfCheck(t, f)
	requireConsolidatedReadersPass(t, run)
	replay := resultFor(t, run, CheckInvestmentReplay)
	assert.Equal(t, SelfCheckFailed, replay.Status)
	assert.Contains(t, replay.Summary, "transfers carrying stale basis")
	assert.Contains(t, replay.Sample, transferOperationID(t, f, transfer.Transaction.ID))
}

// TestMeasureReplaySelfCheckRuntime records the verifier's cost on a larger
// book. It is a measurement, not a gate: run it with
// REKENRAAM_MEASURE_REPLAY=1 go test ./internal/app -run TestMeasureReplaySelfCheckRuntime -v
func TestMeasureReplaySelfCheckRuntime(t *testing.T) {
	t.Parallel()
	if os.Getenv("REKENRAAM_MEASURE_REPLAY") == "" {
		t.Skip("set REKENRAAM_MEASURE_REPLAY=1 to measure")
	}
	const positions, tradesPerPosition = 40, 50
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for p := 0; p < positions; p++ {
		account := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")
		for trade := 0; trade < tradesPerPosition; trade++ {
			date := start.AddDate(0, 0, trade).Format("2006-01-02")
			if trade%5 == 4 {
				sale := tradeOn(f, account, date, 1, 2500)
				_, err := f.investmentService.Sell(ctx, sale)
				require.NoError(t, err)
				continue
			}
			_, err := f.investmentService.Buy(ctx, tradeOn(f, account, date, 2, int64(1000+trade)))
			require.NoError(t, err)
		}
	}
	service := selfCheckOver(t, f.database)
	began := time.Now()
	equivalence, err := service.repository.InvestmentReplayEquivalence(ctx, BookID)
	elapsed := time.Since(began)
	require.NoError(t, err)
	require.Empty(t, equivalence.Mismatches)
	t.Logf("replay equivalence: %d positions, %d trades, %s (%s per position)",
		equivalence.Positions, positions*tradesPerPosition, elapsed, elapsed/time.Duration(positions))
}
