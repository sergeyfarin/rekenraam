package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// #596. A statement showing no movement since the last reconciliation — the
// normal case for a dormant account — finishes with nothing selected. The
// difference must still be zero, so the checkpoint only records that the bank
// agrees with the last reconciled balance on that date.

func TestEmptyStatementFinishesAsChainedCheckpointWithoutPostings(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-01-15", 100)
	january := reconcileCash(t, f, "2026-01-31", 100)
	february := reconcileCash(t, f, "2026-02-28", 100)

	require.Equal(t, []int64{february, january}, activeCheckpointIDs(t, f))
	require.Zero(t, checkpointPostingCount(t, f, february))
	checkpoints, err := f.transactionService.repository.ListReconciliationCheckpoints(context.Background(), BookID, f.cashAccountID, f.eurCommodityID)
	require.NoError(t, err)
	require.Equal(t, february, checkpoints[0].ID)
	require.True(t, checkpoints[0].PreviousCheckpointID.Valid)
	require.Equal(t, january, checkpoints[0].PreviousCheckpointID.Int64)
	require.Equal(t, "2026-02-28", checkpoints[0].StatementDate)
	require.Zero(t, checkpoints[0].StatementAccountSequence)

	result := checkpointIntegrity(t, f)
	require.Equal(t, SelfCheckPassed, result.Status, result.Summary)
}

func TestEmptyStatementStillNeedsZeroDifference(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-01-15", 100)
	reconcileCash(t, f, "2026-01-31", 100)
	session := startCashReconciliation(t, f, "2026-02-28", 90)
	require.Equal(t, exact.New(-10), session.Difference.QuantityValue, "a new session shows its real difference, not 0")
	_, err := f.transactionService.FinishReconciliation(ctx, FinishReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, ChangeReason: "statement verified",
	})
	require.ErrorIs(t, err, ErrReconciliationNotBalanced)
}

func TestLatePostingInsideEmptyStatementNeedsOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-01-15", 100)
	january := reconcileCash(t, f, "2026-01-31", 100)
	february := reconcileCash(t, f, "2026-02-28", 100)

	spec := ordinaryEntry(f.cashAccountID, f.incomeAccountID, f.eurCommodityID, 5)
	spec.TransactionDate = "2026-02-10"
	_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: spec})
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)

	created := postEURWithOverride(t, f, "2026-02-10", 5)
	require.Equal(t, []int64{february}, created.InvalidatedCheckpointIDs)
	require.Equal(t, []int64{january}, activeCheckpointIDs(t, f))
}

// #596 review. A statement dated before the checkpoint it would chain from
// would move the reconciled boundary backwards: an empty March statement on
// April's balance kept April's posting covered after April was voided.
func TestStatementDatedBeforeStartingCheckpointIsRefused(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	deposit := postEUR(t, f, "2026-04-10", 100)
	april := reconcileCash(t, f, "2026-04-30", 100)

	_, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.cashAccountID,
		CommodityID: f.eurCommodityID, StatementDate: "2026-03-31", StatementBalanceValue: exact.New(100),
	})
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, []int64{april}, activeCheckpointIDs(t, f))

	// Voiding April offers its posting again.
	voidCashCheckpoint(t, f, april)
	session := startCashReconciliation(t, f, "2026-04-30", 100)
	require.Len(t, session.Candidates, 1)
	require.Equal(t, cashPostingLineID(t, f, deposit), currentPostingLine(t, f, session.Candidates[0].PostingID))

	// The same date as the starting checkpoint is allowed.
	sameDay := reconcileCash(t, f, "2026-04-30", 100)
	require.NotZero(t, sameDay)
	sameAgain := startCashReconciliation(t, f, "2026-04-30", 100)
	require.Empty(t, sameAgain.Candidates)
}

// #596 review. An empty statement on its starting checkpoint's own date keeps
// that checkpoint's same-day boundary instead of resetting it to 0, so its row
// sits above both postings it inherits and beside their running balance.
func TestSameDayEmptyStatementKeepsInheritedBoundary(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-03-31", 10)
	reconcileCash(t, f, "2026-03-31", 10)
	postEUR(t, f, "2026-03-31", 20)
	second := reconcileCash(t, f, "2026-03-31", 30)
	empty := reconcileCash(t, f, "2026-03-31", 30)

	checkpoints, err := f.transactionService.repository.ListReconciliationCheckpoints(context.Background(), BookID, f.cashAccountID, f.eurCommodityID)
	require.NoError(t, err)
	sequences := map[int64]int64{}
	for _, checkpoint := range checkpoints {
		sequences[checkpoint.ID] = checkpoint.StatementAccountSequence
	}
	require.Equal(t, int64(2), sequences[second])
	require.Equal(t, int64(2), sequences[empty], "the empty statement inherits the same-day boundary")

	page := readCashRegister(t, f, ListTransactionsInput{})
	require.Len(t, page.Statements, 3)
	requireRunningBalanceBelowStatementEqualsStatementBalance(t, page)
}

// #596 review. Start computes the difference with its automatic selection
// applied: a 38-digit starting balance and an equal cleared withdrawal balance
// a 0.00 statement. A difference that really is out of range is
// LEDGER_OVERFLOW, not a server error.
func TestStartAppliesClearedSelectionBeforeComputingDifference(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	large, err := exact.Parse("99999999999999999999999999999999999999")
	require.NoError(t, err)

	t.Run("balanced by the cleared withdrawal", func(t *testing.T) {
		t.Parallel()
		f := newInvestmentsTestFixture(t)
		postEURCoefficient(t, f, "2026-01-10", large)
		session, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
			OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.cashAccountID,
			CommodityID: f.eurCommodityID, StatementDate: "2026-01-31", StatementBalanceValue: large,
		})
		require.NoError(t, err)
		finishCashSession(t, f, session.ID, session.Candidates)

		withdrawal := postEURCoefficient(t, f, "2026-02-10", large.Negated())
		_, err = f.transactionService.ChangePostingReconciliationStatus(ctx, ChangePostingReconciliationStatusInput{
			OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.cashAccountID,
			PostingVersionIDs: []int64{cashPostingVersionIDOf(t, f, withdrawal)}, Status: "cleared", ClearedOn: "2026-02-10",
		})
		require.NoError(t, err)

		started, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
			OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.cashAccountID,
			CommodityID: f.eurCommodityID, StatementDate: "2026-02-28", StatementBalanceValue: exact.New(0), StatementBalanceScale: 2,
		})
		require.NoError(t, err)
		require.Len(t, started.SelectedPostings, 1, "the cleared withdrawal starts selected")
		require.Zero(t, started.Difference.QuantityValue.Sign(), "balanced as started")
	})

	t.Run("out of range is LEDGER_OVERFLOW", func(t *testing.T) {
		t.Parallel()
		f := newInvestmentsTestFixture(t)
		postEURCoefficient(t, f, "2026-01-10", large)
		session, err := f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
			OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.cashAccountID,
			CommodityID: f.eurCommodityID, StatementDate: "2026-01-31", StatementBalanceValue: large,
		})
		require.NoError(t, err)
		finishCashSession(t, f, session.ID, session.Candidates)

		_, err = f.transactionService.StartReconciliation(ctx, StartReconciliationInput{
			OriginType: "browser_api", OwnerUserID: f.ownerUserID, AccountID: f.cashAccountID,
			CommodityID: f.eurCommodityID, StatementDate: "2026-02-28", StatementBalanceValue: exact.New(0), StatementBalanceScale: 2,
		})
		var overflow LedgerOverflowError
		require.ErrorAs(t, err, &overflow)
	})
}

func postEURCoefficient(t *testing.T, f *investmentsTestFixture, date string, amount exact.Coefficient) Transaction {
	t.Helper()
	created, err := f.transactionService.CreateTransaction(context.Background(), CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		Spec: TransactionInput{
			TransactionDate: date,
			JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: amount},
				{AccountID: f.incomeAccountID, CommodityID: f.eurCommodityID, QuantityValue: amount.Negated()},
			}}},
		},
	})
	require.NoError(t, err)
	return created
}

func cashPostingVersionIDOf(t *testing.T, f *investmentsTestFixture, transaction Transaction) int64 {
	t.Helper()
	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.AccountID == f.cashAccountID {
				return posting.ID
			}
		}
	}
	t.Fatal("transaction has no cash posting")
	return 0
}

func currentPostingLine(t *testing.T, f *investmentsTestFixture, postingVersionID int64) int64 {
	t.Helper()
	var line int64
	require.NoError(t, f.database.QueryRowContext(context.Background(),
		`SELECT posting_line_id FROM posting_versions WHERE id = ?`, postingVersionID).Scan(&line))
	return line
}

func finishCashSession(t *testing.T, f *investmentsTestFixture, sessionID int64, candidates []ReconciliationPosting) {
	t.Helper()
	ctx := context.Background()
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.PostingID)
	}
	if len(ids) > 0 {
		_, err := f.transactionService.UpdateReconciliationSelection(ctx, ReconciliationSelectionInput{
			OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: sessionID, PostingVersionIDs: ids,
		})
		require.NoError(t, err)
	}
	_, err := f.transactionService.FinishReconciliation(ctx, FinishReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: sessionID, ChangeReason: "statement verified",
	})
	require.NoError(t, err)
}

// #596 review. Start refuses a backdated statement, but a session saved before
// that rule reaches finish unchecked. Finish enforces the same rule in its own
// transaction, so such a session cannot chain an earlier boundary from a later
// statement.
func TestFinishRefusesSessionDatedBeforeStartingCheckpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	deposit := postEUR(t, f, "2026-04-10", 100)
	april := reconcileCash(t, f, "2026-04-30", 100)
	session := startCashReconciliation(t, f, "2026-04-30", 100)
	// The row a pre-upgrade March session would have left.
	_, err := f.database.ExecContext(ctx, `UPDATE reconciliation_sessions SET statement_date = '2026-03-31' WHERE id = ?`, session.ID)
	require.NoError(t, err)

	_, err = f.transactionService.FinishReconciliation(ctx, FinishReconciliationInput{
		OriginType: "browser_api", OwnerUserID: f.ownerUserID, SessionID: session.ID, ChangeReason: "statement verified",
	})
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, []int64{april}, activeCheckpointIDs(t, f), "no checkpoint was written")

	voidCashCheckpoint(t, f, april)
	again := startCashReconciliation(t, f, "2026-04-30", 100)
	require.Len(t, again.Candidates, 1, "April's posting is offered again")
	require.Equal(t, cashPostingLineID(t, f, deposit), currentPostingLine(t, f, again.Candidates[0].PostingID))
}
