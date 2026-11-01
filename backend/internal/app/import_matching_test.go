package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// #485: a CSV or QIF statement row that matches a posting already in the
// ledger (a transfer entered by hand) is offered as a link instead of a new
// transaction. The link writes no journal; commit rechecks the posting in
// its own write transaction.

type importMatchFixture struct {
	exchangeFixture
	importService *ImportService
	importRepo    *db.ImportRepository
	profileID     int64
}

func newImportMatchFixture(t *testing.T) importMatchFixture {
	t.Helper()
	f := newExchangeFixture(t)
	importRepo := db.NewImportRepository(f.database)
	importService := NewImportService(importRepo, f.transactionService, db.NewAccountRepository(f.database), nil, nil, nil)
	profile, err := importService.CreateImportProfile(context.Background(), CreateImportProfileInput{
		OwnerUserID: f.ownerUserID, Name: "Statement", AdapterKind: "csv",
		ConfigJSON: `{"delimiter":"comma","date_column":"Date","payee_column":"Payee","amount_column":"Amount","date_layout":"YMD","decimal_separator":"."}`,
	})
	require.NoError(t, err)
	return importMatchFixture{exchangeFixture: f, importService: importService, importRepo: importRepo, profileID: profile.ID}
}

type statementRow struct{ date, amount, payee string }

// stageCSV uploads one statement and resolves every row to accountID in
// commodityID, with the fees category as the offset for rows kept as new.
func (f importMatchFixture) stageCSV(t *testing.T, accountID, commodityID int64, rows ...statementRow) (int64, []ImportStagedRow) {
	t.Helper()
	var csv strings.Builder
	csv.WriteString("Date,Payee,Amount\n")
	for _, row := range rows {
		csv.WriteString(row.date + "," + row.payee + "," + row.amount + "\n")
	}
	result, err := f.importService.StartImport(context.Background(), StartImportInput{
		OwnerUserID: f.ownerUserID, ProfileID: &f.profileID,
		Input: RawInput{Filename: "statement.csv", Bytes: []byte(csv.String())},
	})
	require.NoError(t, err)
	require.Len(t, result.Rows, len(rows))
	for _, row := range result.Rows {
		f.resolve(t, result.Batch.ID, row, accountID, commodityID, nil)
	}
	return result.Batch.ID, result.Rows
}

func (f importMatchFixture) resolve(t *testing.T, batchID int64, row ImportStagedRow, accountID, commodityID int64, choice *ImportMatchChoice) {
	t.Helper()
	require.NoError(t, f.patch(batchID, row, accountID, commodityID, choice))
}

func (f importMatchFixture) patch(batchID int64, row ImportStagedRow, accountID, commodityID int64, choice *ImportMatchChoice) error {
	categoryID := f.feesAccountID
	encoded, err := json.Marshal(ImportRowResolution{AccountID: accountID, CommodityID: commodityID, CategoryID: &categoryID, Match: choice})
	if err != nil {
		return err
	}
	status := row.DedupeStatus
	if status == "" {
		status = "new"
	}
	return f.importService.PatchImportBatch(context.Background(), PatchImportBatchInput{
		OwnerUserID: f.ownerUserID, BatchID: batchID,
		RowResolutions: []RowResolutionPatch{{RowID: row.ID, DedupeStatus: status, ResolutionJSON: string(encoded)}},
	})
}

func (f importMatchFixture) preview(t *testing.T, batchID int64) PreviewCommitResult {
	t.Helper()
	result, err := f.importService.PreviewCommit(context.Background(), PreviewCommitInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	return result
}

func (f importMatchFixture) commit(t *testing.T, batchID int64) CommitImportBatchResult {
	t.Helper()
	result, err := f.importService.CommitImportBatch(context.Background(), CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	return result
}

// transfer is 100.00 EUR from cash, leaving on sentOn, into savings,
// arriving on arrivedOn.
func (f importMatchFixture) transfer(t *testing.T, sentOn, arrivedOn string) Transaction {
	t.Helper()
	created, err := f.create(t, TransactionInput{
		TransactionKind: "transfer", TransactionDate: sentOn, Description: "To savings",
		JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
			{AccountID: f.cashAccountID, PostedOn: sentOn, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10000), QuantityScale: 2},
			{AccountID: f.savingsAccountID, PostedOn: arrivedOn, CommodityID: f.eurCommodityID, QuantityValue: exact.New(10000), QuantityScale: 2},
		}}},
	})
	require.NoError(t, err)
	return created
}

func lineKeyOn(t *testing.T, transaction Transaction, accountID int64) string {
	t.Helper()
	for _, entry := range transaction.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.AccountID == accountID {
				return posting.LineKey
			}
		}
	}
	t.Fatalf("transaction %d has no posting on account %d", transaction.ID, accountID)
	return ""
}

func linkTo(transaction Transaction, lineKey, postedOn string) *ImportMatchChoice {
	return &ImportMatchChoice{Decision: ImportMatchDecisionLink, TransactionID: transaction.ID, LineKey: lineKey, PostedOn: postedOn}
}

func reviewFor(result PreviewCommitResult, rowID int64) *RowMatchReview {
	for i := range result.MatchReviews {
		if result.MatchReviews[i].RowID == rowID {
			return &result.MatchReviews[i]
		}
	}
	return nil
}

func (f importMatchFixture) transactionCount(t *testing.T) int {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM transactions`).Scan(&count))
	return count
}

func (f importMatchFixture) linkCount(t *testing.T) int {
	t.Helper()
	var count int
	require.NoError(t, f.database.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM import_posting_links`).Scan(&count))
	return count
}

func TestImportMatchWindowIsFiveDaysInclusiveAroundThePostingDay(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	transfer := f.transfer(t, "2026-06-08", "2026-06-10")

	batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID,
		statementRow{"2026-06-05", "100.00", "Five before"},
		statementRow{"2026-06-15", "100.00", "Five after"},
		statementRow{"2026-06-04", "100.00", "Six before"},
		statementRow{"2026-06-16", "100.00", "Six after"},
	)
	preview := f.preview(t, batchID)

	for _, want := range []struct {
		row    ImportStagedRow
		offset int
	}{{rows[0], 5}, {rows[1], -5}} {
		review := reviewFor(preview, want.row.ID)
		require.NotNil(t, review, "a posting day five days from the row date matches")
		require.Len(t, review.Candidates, 1)
		candidate := review.Candidates[0]
		assert.Equal(t, transfer.ID, candidate.TransactionID)
		assert.Equal(t, lineKeyOn(t, transfer, f.savingsAccountID), candidate.LineKey)
		assert.Equal(t, "2026-06-10", candidate.PostedOn, "measured from the savings posting's own day, not the transaction date")
		assert.Equal(t, want.offset, candidate.DayOffset)
		assert.True(t, review.NeedsDecision, "a match is a suggestion until the owner decides")
	}
	assert.Nil(t, reviewFor(preview, rows[2].ID), "six days before is outside the window")
	assert.Nil(t, reviewFor(preview, rows[3].ID), "six days after is outside the window")
}

func TestImportMatchNeedsTheExactAmountAtAnyScale(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	f.transfer(t, "2026-06-08", "2026-06-10")

	batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID,
		statementRow{"2026-06-10", "100.000", "Three decimals"},
		statementRow{"2026-06-10", "100.01", "One cent more"},
		statementRow{"2026-06-10", "-100.00", "Opposite sign"},
	)
	preview := f.preview(t, batchID)

	assert.NotNil(t, reviewFor(preview, rows[0].ID), "100.000 is exactly 100.00")
	assert.Nil(t, reviewFor(preview, rows[1].ID), "no fuzzy amount matching")
	assert.Nil(t, reviewFor(preview, rows[2].ID), "the sign is part of the amount")
}

func TestImportMatchExchangeMatchesEachSideInItsOwnCurrency(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	exchange, err := f.exchange(t, "2026-06-01", "2026-06-03")
	require.NoError(t, err)
	before := f.transactionCount(t)

	usdBatch, usdRows := f.stageCSV(t, f.usdAccountID, f.usdCommodityID, statementRow{"2026-06-02", "108.00", "Exchange in"})
	eurOnUSD, eurOnUSDRows := f.stageCSV(t, f.usdAccountID, f.eurCommodityID, statementRow{"2026-06-02", "100.00", "Wrong currency"})
	eurBatch, eurRows := f.stageCSV(t, f.cashAccountID, f.eurCommodityID, statementRow{"2026-06-01", "-100.00", "Exchange out"})

	usdReview := reviewFor(f.preview(t, usdBatch), usdRows[0].ID)
	require.NotNil(t, usdReview)
	require.Len(t, usdReview.Candidates, 1, "the trading legs are never on the owner's account")
	assert.Equal(t, "2026-06-03", usdReview.Candidates[0].PostedOn)
	assert.Equal(t, f.usdCommodityID, usdReview.Candidates[0].CommodityID)
	assert.Nil(t, reviewFor(f.preview(t, eurOnUSD), eurOnUSDRows[0].ID), "the sent EUR amount never matches on the USD account")
	eurReview := reviewFor(f.preview(t, eurBatch), eurRows[0].ID)
	require.NotNil(t, eurReview)
	require.Len(t, eurReview.Candidates, 1)

	f.resolve(t, usdBatch, usdRows[0], f.usdAccountID, f.usdCommodityID, linkTo(exchange, usdReview.Candidates[0].LineKey, "2026-06-03"))
	f.resolve(t, eurBatch, eurRows[0], f.cashAccountID, f.eurCommodityID, linkTo(exchange, eurReview.Candidates[0].LineKey, "2026-06-01"))
	usdResult := f.commit(t, usdBatch)
	eurResult := f.commit(t, eurBatch)

	assert.Equal(t, 1, usdResult.LinkedCount)
	assert.Equal(t, 1, eurResult.LinkedCount)
	assert.Equal(t, before, f.transactionCount(t), "linking writes no journal")
	links, err := f.importRepo.ImportPostingLinksForTransaction(context.Background(), BookID, exchange.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{lineKeyOn(t, exchange, f.cashAccountID), lineKeyOn(t, exchange, f.usdAccountID)},
		[]string{links[0].LineKey, links[1].LineKey}, "each statement row links to its own side")
}

func TestImportMatchNeverOffersAnAlreadyLinkedPosting(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	transfer := f.transfer(t, "2026-06-08", "2026-06-10")
	savingsLine := lineKeyOn(t, transfer, f.savingsAccountID)

	first, firstRows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statementRow{"2026-06-10", "100.00", "Transfer in"})
	f.resolve(t, first, firstRows[0], f.savingsAccountID, f.eurCommodityID, linkTo(transfer, savingsLine, "2026-06-10"))
	require.Equal(t, 1, f.commit(t, first).LinkedCount)

	second, secondRows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statementRow{"2026-06-11", "100.00", "Another credit"})
	assert.Nil(t, reviewFor(f.preview(t, second), secondRows[0].ID), "a linked posting is not a candidate")

	// A posting a statement row created is linked to that row too.
	imported, importedRows := f.stageCSV(t, f.cashAccountID, f.eurCommodityID, statementRow{"2026-07-01", "-25.00", "Bakery"})
	require.Nil(t, reviewFor(f.preview(t, imported), importedRows[0].ID))
	require.Equal(t, 1, f.commit(t, imported).CommittedCount)
	later, laterRows := f.stageCSV(t, f.cashAccountID, f.eurCommodityID, statementRow{"2026-07-02", "-25.00", "Bakery corner"})
	assert.Nil(t, reviewFor(f.preview(t, later), laterRows[0].ID), "an imported posting already stands for its row")

	// The other side of an imported transfer is still unlinked on its account.
	_, err := f.create(t, TransactionInput{TransactionKind: "transfer", TransactionDate: "2026-08-01",
		JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
			{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-4000), QuantityScale: 2},
			{AccountID: f.savingsAccountID, CommodityID: f.eurCommodityID, QuantityValue: exact.New(4000), QuantityScale: 2},
		}}}})
	require.NoError(t, err)
	other, otherRows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statementRow{"2026-08-01", "40.00", "Transfer in"})
	assert.NotNil(t, reviewFor(f.preview(t, other), otherRows[0].ID))
}

func TestImportMatchSeveralCandidatesWaitForTheOwnersPick(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	f.transfer(t, "2026-06-08", "2026-06-09")
	second := f.transfer(t, "2026-06-09", "2026-06-11")
	before := f.transactionCount(t)

	batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statementRow{"2026-06-10", "100.00", "Transfer in"})
	review := reviewFor(f.preview(t, batchID), rows[0].ID)
	require.NotNil(t, review)
	require.Len(t, review.Candidates, 2)
	assert.True(t, review.NeedsDecision)

	held := f.commit(t, batchID)
	assert.Equal(t, []int64{rows[0].ID}, held.MatchReviewRowIDs, "never linked or posted beside an undecided match")
	assert.Equal(t, "partially_committed", held.Status)
	assert.Equal(t, before, f.transactionCount(t))
	assert.Equal(t, 0, f.linkCount(t))
	staged, err := f.importRepo.ImportStagedRowByID(context.Background(), rows[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "pending", staged.CommitStatus)
	assert.Equal(t, importMatchReviewRequiredReason, staged.CommitError.String)

	f.resolve(t, batchID, rows[0], f.savingsAccountID, f.eurCommodityID, linkTo(second, lineKeyOn(t, second, f.savingsAccountID), "2026-06-11"))
	assert.False(t, reviewFor(f.preview(t, batchID), rows[0].ID).NeedsDecision)
	result := f.commit(t, batchID)
	assert.Equal(t, 1, result.LinkedCount)
	assert.Equal(t, "committed", result.Status)
	links, err := f.importRepo.ImportPostingLinksForTransaction(context.Background(), BookID, second.ID)
	require.NoError(t, err)
	require.Len(t, links, 1, "the owner's pick, not the first candidate")
}

func TestImportMatchCommitRechecksThePostingInsideTheWriteTransaction(t *testing.T) {
	t.Parallel()

	t.Run("a description edit keeps the link", func(t *testing.T) {
		t.Parallel()
		f := newImportMatchFixture(t)
		transfer := f.transfer(t, "2026-06-08", "2026-06-10")
		batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statementRow{"2026-06-10", "100.00", "Transfer in"})
		f.resolve(t, batchID, rows[0], f.savingsAccountID, f.eurCommodityID, linkTo(transfer, lineKeyOn(t, transfer, f.savingsAccountID), "2026-06-10"))
		f.edit(t, transfer, "Renamed", "2026-06-10")

		assert.Equal(t, 1, f.commit(t, batchID).LinkedCount, "a non-financial edit leaves the line and its facts")
	})

	t.Run("a posting moved to another day is held", func(t *testing.T) {
		t.Parallel()
		f := newImportMatchFixture(t)
		transfer := f.transfer(t, "2026-06-08", "2026-06-10")
		batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statementRow{"2026-06-10", "100.00", "Transfer in"})
		f.resolve(t, batchID, rows[0], f.savingsAccountID, f.eurCommodityID, linkTo(transfer, lineKeyOn(t, transfer, f.savingsAccountID), "2026-06-10"))
		f.edit(t, transfer, "To savings", "2026-06-11")

		result := f.commit(t, batchID)
		assert.Equal(t, []int64{rows[0].ID}, result.MatchReviewRowIDs, "still in the window, but not the day the owner reviewed")
		assert.Equal(t, 0, f.linkCount(t))
		review := reviewFor(f.preview(t, batchID), rows[0].ID)
		require.NotNil(t, review)
		assert.True(t, review.NeedsDecision)
		assert.Equal(t, "2026-06-11", review.Candidates[0].PostedOn, "the next preview shows the current facts")
	})

	t.Run("a void after the commit's early reads is caught in its write transaction", func(t *testing.T) {
		t.Parallel()
		f := newImportMatchFixture(t)
		transfer := f.transfer(t, "2026-06-08", "2026-06-10")
		batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statementRow{"2026-06-10", "100.00", "Transfer in"})
		f.resolve(t, batchID, rows[0], f.savingsAccountID, f.eurCommodityID, linkTo(transfer, lineKeyOn(t, transfer, f.savingsAccountID), "2026-06-10"))
		f.importService.beforeImportRowCommitForTest = func() {
			_, err := f.transactionService.VoidTransaction(context.Background(), VoidTransactionInput{
				OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: transfer.ID, ChangeReason: "entered twice",
			})
			require.NoError(t, err)
		}

		result := f.commit(t, batchID)
		assert.Equal(t, []int64{rows[0].ID}, result.MatchReviewRowIDs)
		assert.Equal(t, 0, f.linkCount(t), "never linked to a voided posting")
		staged, err := f.importRepo.ImportStagedRowByID(context.Background(), rows[0].ID)
		require.NoError(t, err)
		assert.Equal(t, importMatchStaleReason, staged.CommitError.String)
		assert.False(t, staged.CommittedIdentityID.Valid, "no identity without its link")
	})

	t.Run("two rows choosing one posting link once", func(t *testing.T) {
		t.Parallel()
		f := newImportMatchFixture(t)
		transfer := f.transfer(t, "2026-06-08", "2026-06-10")
		batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID,
			statementRow{"2026-06-10", "100.00", "Transfer in"}, statementRow{"2026-06-11", "100.00", "Transfer in again"})
		choice := linkTo(transfer, lineKeyOn(t, transfer, f.savingsAccountID), "2026-06-10")
		f.resolve(t, batchID, rows[0], f.savingsAccountID, f.eurCommodityID, choice)
		f.resolve(t, batchID, rows[1], f.savingsAccountID, f.eurCommodityID, choice)

		result := f.commit(t, batchID)
		assert.Equal(t, 1, result.LinkedCount)
		assert.Equal(t, []int64{rows[1].ID}, result.MatchReviewRowIDs, "the second link finds the line already linked")
		assert.Equal(t, 1, f.linkCount(t))
	})
}

// edit restates a transfer with a new description and savings posting day,
// keeping both line keys.
func (f importMatchFixture) edit(t *testing.T, transfer Transaction, description, arrivedOn string) {
	t.Helper()
	_, err := f.transactionService.UpdateTransaction(context.Background(), UpdateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: transfer.ID, ChangeReason: "edit",
		Spec: TransactionInput{
			TransactionKind: "transfer", TransactionDate: transfer.TransactionDate, Description: description,
			JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				{LineKey: lineKeyOn(t, transfer, f.cashAccountID), AccountID: f.cashAccountID, PostedOn: transfer.TransactionDate, CommodityID: f.eurCommodityID, QuantityValue: exact.New(-10000), QuantityScale: 2},
				{LineKey: lineKeyOn(t, transfer, f.savingsAccountID), AccountID: f.savingsAccountID, PostedOn: arrivedOn, CommodityID: f.eurCommodityID, QuantityValue: exact.New(10000), QuantityScale: 2},
			}}},
		},
	})
	require.NoError(t, err)
}

func TestImportMatchEnteredTransferThenBothStatementsAndReimportIsANoOp(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	transfer := f.transfer(t, "2026-06-08", "2026-06-10")
	before := f.transactionCount(t)

	link := func(accountID int64, row statementRow, postedOn string) {
		batchID, rows := f.stageCSV(t, accountID, f.eurCommodityID, row)
		review := reviewFor(f.preview(t, batchID), rows[0].ID)
		require.NotNil(t, review)
		require.Len(t, review.Candidates, 1)
		f.resolve(t, batchID, rows[0], accountID, f.eurCommodityID, linkTo(transfer, review.Candidates[0].LineKey, postedOn))
		result := f.commit(t, batchID)
		require.Equal(t, 1, result.LinkedCount)
		staged, err := f.importRepo.ImportStagedRowByID(context.Background(), rows[0].ID)
		require.NoError(t, err)
		assert.Equal(t, transfer.ID, staged.CommittedTransactionID.Int64, "the row names the transaction it links to")
		assert.Empty(t, staged.CommitEffects, "a link has no effects of its own")
	}
	checking := statementRow{"2026-06-08", "-100.00", "To savings"}
	savings := statementRow{"2026-06-10", "100.00", "From checking"}
	link(f.cashAccountID, checking, "2026-06-08")
	link(f.savingsAccountID, savings, "2026-06-10")
	assert.Equal(t, before, f.transactionCount(t), "one transaction")
	assert.Equal(t, 2, f.linkCount(t))

	for _, statement := range []struct {
		accountID int64
		row       statementRow
	}{{f.cashAccountID, checking}, {f.savingsAccountID, savings}} {
		batchID, rows := f.stageCSV(t, statement.accountID, f.eurCommodityID, statement.row)
		assert.Equal(t, "duplicate", rows[0].DedupeStatus, "the identity remembers a linked row")
		result := f.commit(t, batchID)
		assert.Equal(t, 1, result.SkippedCount)
		assert.Equal(t, 0, result.LinkedCount)
	}
	assert.Equal(t, before, f.transactionCount(t), "re-import is a no-op")
	assert.Equal(t, 2, f.linkCount(t))
	requireKnownBasisSelfCheckPasses(t, f.database)
}

func TestImportMatchLinkPreservesTheReconciledCheckpoint(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	transfer := f.transfer(t, "2026-06-08", "2026-06-10")
	checkpointID := reconcileAccount(t, f.investmentsTestFixture, f.savingsAccountID, f.eurCommodityID, "2026-06-30", 100)

	batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statementRow{"2026-06-10", "100.00", "From checking"})
	f.resolve(t, batchID, rows[0], f.savingsAccountID, f.eurCommodityID, linkTo(transfer, lineKeyOn(t, transfer, f.savingsAccountID), "2026-06-10"))
	preview := f.preview(t, batchID)
	assert.Empty(t, preview.ReconciliationCheckpoints, "a link reaches no checkpoint")
	assert.Equal(t, 1, preview.LinkCount)

	result := f.commit(t, batchID)
	assert.Equal(t, 1, result.LinkedCount, "no override needed")
	assert.Empty(t, result.ReconciliationHoldRowIDs)
	assert.Equal(t, []int64{checkpointID}, activeCheckpointIDsFor(t, f.investmentsTestFixture, f.savingsAccountID, f.eurCommodityID))
	var status string
	require.NoError(t, f.database.QueryRowContext(context.Background(), `
		SELECT pv.reconciliation_status FROM posting_versions pv
		JOIN current_transaction_versions tv ON tv.id = pv.transaction_version_id
		WHERE tv.transaction_id = ? AND pv.account_id = ?`, transfer.ID, f.savingsAccountID).Scan(&status))
	assert.Equal(t, "reconciled", status, "evidence, not an edit: the posting keeps its version")
}

func TestImportMatchKeptAsNewOrUnmatchedRowsCommitAsToday(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	f.transfer(t, "2026-06-08", "2026-06-10")
	before := f.transactionCount(t)

	batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID,
		statementRow{"2026-06-10", "100.00", "Looks like the transfer"},
		statementRow{"2026-06-20", "7.00", "Interest"},
	)
	f.resolve(t, batchID, rows[0], f.savingsAccountID, f.eurCommodityID, &ImportMatchChoice{Decision: ImportMatchDecisionNew})
	preview := f.preview(t, batchID)
	require.NotNil(t, reviewFor(preview, rows[0].ID), "the candidate stays visible")
	assert.False(t, reviewFor(preview, rows[0].ID).NeedsDecision)
	assert.Nil(t, reviewFor(preview, rows[1].ID))

	result := f.commit(t, batchID)
	assert.Equal(t, 2, result.CommittedCount)
	assert.Equal(t, 0, result.LinkedCount)
	assert.Equal(t, before+2, f.transactionCount(t))
}

func TestImportMatchQIFSharesTheRule(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	transfer := f.transfer(t, "2026-06-08", "2026-06-10")

	result, err := f.importService.StartImport(context.Background(), StartImportInput{
		OwnerUserID: f.ownerUserID,
		Input:       RawInput{Filename: "savings.qif", Bytes: []byte("!Type:Bank\nD2026-06-12\nT100.00\nPFrom checking\n^\n")},
	})
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	assert.Equal(t, "qif", result.Batch.SourceKind)
	f.resolve(t, result.Batch.ID, result.Rows[0], f.savingsAccountID, f.eurCommodityID, nil)
	review := reviewFor(f.preview(t, result.Batch.ID), result.Rows[0].ID)
	require.NotNil(t, review)
	require.Len(t, review.Candidates, 1)
	assert.Equal(t, -2, review.Candidates[0].DayOffset)

	f.resolve(t, result.Batch.ID, result.Rows[0], f.savingsAccountID, f.eurCommodityID, linkTo(transfer, review.Candidates[0].LineKey, "2026-06-10"))
	assert.Equal(t, 1, f.commit(t, result.Batch.ID).LinkedCount)
}

func TestImportMatchChoiceShapeIsValidated(t *testing.T) {
	t.Parallel()
	f := newImportMatchFixture(t)
	batchID, rows := f.stageCSV(t, f.savingsAccountID, f.eurCommodityID, statementRow{"2026-06-10", "100.00", "Transfer in"})

	for _, choice := range []*ImportMatchChoice{
		{Decision: "maybe"},
		{Decision: ImportMatchDecisionLink, LineKey: "line-1", PostedOn: "2026-06-10"},
		{Decision: ImportMatchDecisionLink, TransactionID: 1, PostedOn: "2026-06-10"},
		{Decision: ImportMatchDecisionLink, TransactionID: 1, LineKey: "line-1", PostedOn: "10/06/2026"},
	} {
		var validation ValidationError
		require.ErrorAs(t, f.patch(batchID, rows[0], f.savingsAccountID, f.eurCommodityID, choice), &validation)
	}
}
