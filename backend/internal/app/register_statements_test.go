package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// #595. Every active reconciliation checkpoint appears in the account register
// as a statement row at its boundary (statement_date,
// statement_account_sequence) — the boundary the reconciliation guard uses —
// including statements that verified no new postings.

func readCashRegister(t *testing.T, f *investmentsTestFixture, input ListTransactionsInput) AccountRegisterResult {
	t.Helper()
	result, err := f.transactionService.Register(context.Background(), f.cashAccountID, input)
	require.NoError(t, err)
	return result
}

// registerRowKeys lays a page out in display order: each statement row before
// the entry its before_entry_index names. "p<posting line id>" is a posting
// row (the line outlives the versions reconciliation appends), "s<checkpoint
// id>" a statement row.
func registerRowKeys(t *testing.T, page AccountRegisterResult) []string {
	t.Helper()
	keys := make([]string, 0, len(page.Entries)+len(page.Statements))
	next := 0
	for entryIndex := 0; entryIndex <= len(page.Entries); entryIndex++ {
		for next < len(page.Statements) && page.Statements[next].BeforeEntryIndex == entryIndex {
			keys = append(keys, fmt.Sprintf("s%d", page.Statements[next].CheckpointID))
			next++
		}
		if entryIndex < len(page.Entries) {
			keys = append(keys, fmt.Sprintf("p%d", page.Entries[entryIndex].Posting.PostingLineID))
		}
	}
	require.Equal(t, len(page.Statements), next, "every statement row has an index inside the page, in order")
	return keys
}

func cashRowKey(t *testing.T, f *investmentsTestFixture, transaction Transaction) string {
	t.Helper()
	return fmt.Sprintf("p%d", cashPostingLineID(t, f, transaction))
}

// requireRunningBalanceBelowStatementEqualsStatementBalance checks the visual
// invariant: the posting directly below a statement row (the newest posting
// its boundary covers) carries a running balance equal to the statement
// balance, when every posting inside the boundary is reconciled.
func requireRunningBalanceBelowStatementEqualsStatementBalance(t *testing.T, page AccountRegisterResult) {
	t.Helper()
	for _, statement := range page.Statements {
		require.Less(t, statement.BeforeEntryIndex, len(page.Entries), "statement %d has a posting below it", statement.CheckpointID)
		below := page.Entries[statement.BeforeEntryIndex].RunningBalance
		require.Equal(t, statement.CommodityID, below.CommodityID)
		running := exact.NewScaledInt()
		running.AddCoefficient(below.QuantityValue, below.QuantityScale)
		balance := exact.NewScaledInt()
		balance.AddCoefficient(statement.StatementBalance.QuantityValue, statement.StatementBalance.QuantityScale)
		assert.Zero(t, running.Cmp(balance), "statement %d on %s: running balance %s/%d, statement balance %s/%d",
			statement.CheckpointID, statement.StatementDate, below.QuantityValue, below.QuantityScale,
			statement.StatementBalance.QuantityValue, statement.StatementBalance.QuantityScale)
	}
}

func TestRegisterStatementRowSitsAtItsCheckpointBoundary(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	// March reconciles the first 31 March posting (sequence 1); the second
	// (sequence 2) lands after March's boundary and inside April's.
	march, april, afterMarch := marchAndAprilCheckpoints(t, f)
	may := postEUR(t, f, "2026-05-02", 4)

	page := readCashRegister(t, f, ListTransactionsInput{})
	require.Len(t, page.Entries, 4)
	marchFirst := fmt.Sprintf("p%d", page.Entries[3].Posting.PostingLineID)
	april10 := fmt.Sprintf("p%d", page.Entries[1].Posting.PostingLineID)
	assert.Equal(t, []string{
		cashRowKey(t, f, may),
		fmt.Sprintf("s%d", april),
		april10,
		cashRowKey(t, f, afterMarch),
		fmt.Sprintf("s%d", march),
		marchFirst,
	}, registerRowKeys(t, page), "the March row sits between the same-day postings at sequence 2 and 1")

	require.Len(t, page.Statements, 2)
	assert.Equal(t, "2026-04-30", page.Statements[0].StatementDate)
	assert.Equal(t, int64(0), page.Statements[0].StatementAccountSequence)
	assert.Equal(t, "2026-03-31", page.Statements[1].StatementDate)
	assert.Equal(t, int64(1), page.Statements[1].StatementAccountSequence)
	assert.Equal(t, "EUR", page.Statements[0].CommodityCode)
	assert.Equal(t, exact.New(27), page.Statements[0].StatementBalance.QuantityValue)
	assert.Equal(t, exact.New(10), page.Statements[1].StatementBalance.QuantityValue)
}

func TestRegisterRunningBalanceBelowStatementEqualsStatementBalance(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	marchAndAprilCheckpoints(t, f, 3, 5)
	postEUR(t, f, "2026-05-02", 4)
	postEUR(t, f, "2026-05-02", -6)
	reconcileCash(t, f, "2026-05-31", 33)

	page := readCashRegister(t, f, ListTransactionsInput{})
	require.Len(t, page.Statements, 3)
	requireRunningBalanceBelowStatementEqualsStatementBalance(t, page)
}

func TestRegisterShowsEmptyStatementsOfDormantAccount(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	deposit := postEUR(t, f, "2026-01-15", 100)
	january := reconcileCash(t, f, "2026-01-31", 100)
	// Nothing moves for two months; each statement still verifies the balance.
	february := reconcileCash(t, f, "2026-02-28", 100)
	march := reconcileCash(t, f, "2026-03-31", 100)

	page := readCashRegister(t, f, ListTransactionsInput{})
	require.Len(t, page.Entries, 1, "a statement is not a transaction")
	assert.Equal(t, []string{
		fmt.Sprintf("s%d", march),
		fmt.Sprintf("s%d", february),
		fmt.Sprintf("s%d", january),
		cashRowKey(t, f, deposit),
	}, registerRowKeys(t, page))
	requireRunningBalanceBelowStatementEqualsStatementBalance(t, page)
	assert.Equal(t, exact.New(100), page.Entries[0].RunningBalance.QuantityValue, "statement rows move no running balance")
}

func TestRegisterHidesInvalidatedAndVoidedStatements(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-01-15", 100)
	january := reconcileCash(t, f, "2026-01-31", 100)
	february := reconcileCash(t, f, "2026-02-28", 100)
	march := reconcileCash(t, f, "2026-03-31", 100)

	voidCashCheckpoint(t, f, march)
	// A late 20 February entry invalidates February (and nothing before it).
	postEURWithOverride(t, f, "2026-02-20", 5)
	require.Equal(t, []int64{january}, activeCheckpointIDs(t, f))

	page := readCashRegister(t, f, ListTransactionsInput{})
	require.Len(t, page.Statements, 1)
	assert.Equal(t, january, page.Statements[0].CheckpointID)
	assert.NotContains(t, registerRowKeys(t, page), fmt.Sprintf("s%d", february))
	assert.NotContains(t, registerRowKeys(t, page), fmt.Sprintf("s%d", march))
}

func TestRegisterStatementsFollowLifecycleAndDateFilters(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	postEUR(t, f, "2026-01-15", 100)
	january := reconcileCash(t, f, "2026-01-31", 100)
	february := reconcileCash(t, f, "2026-02-28", 100)
	reconcileCash(t, f, "2026-03-31", 100)

	voided := readCashRegister(t, f, ListTransactionsInput{Status: "voided"})
	assert.Empty(t, voided.Statements, "the voided register lists voided transactions only")

	ranged := readCashRegister(t, f, ListTransactionsInput{AfterDate: "2026-01-20", BeforeDate: "2026-03-15"})
	require.Len(t, ranged.Statements, 2)
	assert.Equal(t, february, ranged.Statements[0].CheckpointID)
	assert.Equal(t, january, ranged.Statements[1].CheckpointID)
	assert.Empty(t, ranged.Entries, "the 15 January deposit is outside the range")
	assert.Equal(t, 0, ranged.Statements[1].BeforeEntryIndex)
}

// Paging with any limit — including pages that end on a statement row, on the
// posting just below one, and limit 1 — returns every row exactly once, in the
// order of one large page.
func TestRegisterPagesStatementRowsExactlyOnce(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	marchAndAprilCheckpoints(t, f, 3)
	reconcileCash(t, f, "2026-05-31", 30)
	reconcileCash(t, f, "2026-06-30", 30)
	postEUR(t, f, "2026-07-01", 2)

	full := readCashRegister(t, f, ListTransactionsInput{Limit: 500})
	require.Empty(t, full.NextCursor)
	want := registerRowKeys(t, full)
	require.Len(t, want, 9, "five postings and four statements")

	for limit := 1; limit <= len(want)+1; limit++ {
		t.Run(fmt.Sprintf("limit %d", limit), func(t *testing.T) {
			var got []string
			cursor := ""
			for pages := 0; ; pages++ {
				require.Less(t, pages, len(want)+2, "paging terminates")
				page := readCashRegister(t, f, ListTransactionsInput{Limit: limit, Cursor: cursor})
				require.LessOrEqual(t, len(page.Entries)+len(page.Statements), limit)
				got = append(got, registerRowKeys(t, page)...)
				if page.NextCursor == "" {
					break
				}
				cursor = page.NextCursor
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestRegisterRejectsMalformedStatementCursor(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	_, err := f.transactionService.Register(context.Background(), f.cashAccountID, ListTransactionsInput{
		Cursor: "MjAyNi0wMy0zMXwxfDV8eA", // 2026-03-31|1|5|x
	})
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
}
