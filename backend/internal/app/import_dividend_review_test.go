package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
)

func TestTrading212DividendOwnerReviewRetainsKnownPayerWithoutDefaults(t *testing.T) {
	t.Parallel()
	f := newInvestTestFixture(t)
	ctx := context.Background()
	conn := f.createConnection(t, &f.cashAccountID)
	_, commodityID, _, err := f.investmentSvc.ResolveOrCreateInstrumentForImport(ctx, f.ownerUserID, 0, "", "US0378331005", "AAPL_US_EQ")
	require.NoError(t, err)
	batchID, rowID := f.stageDividendRow(t, conn.ID, trading212Dividend{Reference: "reviewed-dividend", Ticker: "AAPL_US_EQ", ISIN: "US0378331005", Amount: "4.32", Currency: "EUR", PaidOn: "2026-06-15", Type: "DIVIDEND"})
	gross, tax := "4.32", "0"
	resolution, err := json.Marshal(ImportRowResolution{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, CategoryID: &f.categoryAccountID, GrossAmount: &gross, WithholdingAmount: &tax})
	require.NoError(t, err)
	require.NoError(t, f.importRepo.UpdateImportStagedRowResolution(ctx, db.UpdateImportStagedRowResolutionParams{RowID: rowID, BatchID: batchID, DedupeStatus: "new", ResolutionJSON: string(resolution)}))
	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	require.Equal(t, 1, result.CommittedCount)
	var payer int64
	require.NoError(t, f.database.QueryRow("SELECT commodity_id FROM investment_dividend_facts").Scan(&payer))
	require.Equal(t, commodityID, payer)
}

func reviewedDividendResolution(f *investTestFixture, gross, tax string) ImportRowResolution {
	return ImportRowResolution{AccountID: f.cashAccountID, CommodityID: f.eurCommodityID, CategoryID: &f.categoryAccountID, GrossAmount: &gross, WithholdingAmount: &tax}
}

func legacyDividendFallback(t *testing.T, amount string) (*investTestFixture, int64, int64, int64, int64) {
	t.Helper()
	return legacyDividendFallbackWithPayer(t, amount, true)
}

func legacyDividendFallbackWithPayer(t *testing.T, amount string, existingPayer bool) (*investTestFixture, int64, int64, int64, int64) {
	t.Helper()
	f := newInvestTestFixture(t)
	ctx := context.Background()
	conn := f.createConnection(t, &f.cashAccountID)
	var payer int64
	if existingPayer {
		_, id, _, err := f.investmentSvc.ResolveOrCreateInstrumentForImport(ctx, f.ownerUserID, 0, "", "US0378331005", "AAPL_US_EQ")
		require.NoError(t, err)
		payer = id
	}
	batch, rowID := f.stageDividendRow(t, conn.ID, trading212Dividend{Reference: "legacy-dividend", Ticker: "AAPL_US_EQ", ISIN: "US0378331005", Amount: amount, Currency: "EUR", PaidOn: "2026-03-01", Type: "DIVIDEND"})
	f.resolveRowGeneric(t, batch, rowID, f.cashAccountID)
	row, err := f.importRepo.ImportStagedRowByID(ctx, rowID)
	require.NoError(t, err)
	resolution, err := parseResolutionJSON(row.ResolutionJSON)
	require.NoError(t, err)
	spec, err := buildTransactionSpec(row, resolution)
	require.NoError(t, err)
	tag, err := db.NewTagRepository(f.database).CreateTag(ctx, db.CreateTagParams{BookID: BookID, ActorUserID: f.ownerUserID, OriginType: "browser_api", Operation: "tag.create", CreatedAt: "2026-03-01T00:00:00Z", Spec: db.TagSpec{Name: "Owner attribution", Kind: "project", MetadataJSON: "{}"}})
	require.NoError(t, err)
	spec.Description = "Owner's dividend description"
	spec.NeedsReview = false // previously approved by the owner
	spec.PayeeName = "Owner payer label"
	spec.TagIDs = []int64{tag.ID}
	spec.NoteMarkdown = "Keep this note"
	spec.MetadataJSON = `{"owner":"preserved"}`
	spec.JournalEntries[0].Postings[0].Memo = "Owner cash memo"
	spec.JournalEntries[0].Postings[0].TagIDs = []int64{tag.ID}
	params, err := f.importService.transactionService.prepareCreateTransactionForWrite(ctx, CreateTransactionInput{OwnerUserID: f.ownerUserID, OriginType: "import", Operation: "transaction.create", ChangeReason: "legacy cash fallback fixture", Spec: spec})
	require.NoError(t, err)
	record, err := f.importRepo.CommitImportedTransaction(ctx, db.CommitImportedTransactionParams{Identity: db.CreateImportCommitIdentityParams{BookID: BookID, DedupeFingerprint: row.DedupeFingerprint, SourceKind: "trading212", AccountID: f.cashAccountID, CreatedAt: "2026-03-02T00:00:00Z"}, Row: db.CommitImportStagedRowParams{RowID: rowID, CommitStatus: "committed"}}, func(tx *sql.Tx) (int64, error) {
		record, err := f.importService.transactionService.repository.CreateTransactionInTx(ctx, tx, params)
		return record.ID, err
	})
	require.NoError(t, err)
	return f, batch, rowID, record, payer
}

func TestTrading212DividendUnknownEconomicsStayPendingWhileCashCommits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	batch, rowID := f.stageDividendRow(t, conn.ID, trading212Dividend{Reference: "review-later", Ticker: "NEW_US_EQ", Amount: "85.00", Currency: "EUR", PaidOn: "2026-03-01", Type: "DIVIDEND"})
	f.resolveRowGeneric(t, batch, rowID, f.cashAccountID)
	cashRows, _, err := f.importService.stageParseResult(ctx, batch, ParseResult{Rows: []StagedRow{{DedupeFingerprint: "independent-cash", Date: "2026-03-02", Amount: "10.00", CommodityHint: "EUR", Memo: "Independent receipt", Raw: map[string]string{"kind": "cash_movement"}}}})
	require.NoError(t, err)
	require.Len(t, cashRows, 2)
	f.resolveRowGeneric(t, batch, cashRows[len(cashRows)-1].ID, f.cashAccountID)
	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch})
	require.NoError(t, err)
	require.Equal(t, []int64{rowID}, result.DividendReviewRowIDs)
	require.Equal(t, "partially_committed", result.Status)
	require.Equal(t, 1, result.CommittedCount)
	instruments, err := f.investmentSvc.ListInstruments(ctx)
	require.NoError(t, err)
	require.Empty(t, instruments)
	reviews, err := f.importService.ListImportDividends(ctx, ListImportDividendsInput{OwnerUserID: f.ownerUserID})
	require.NoError(t, err)
	require.Len(t, reviews.Rows, 1)
	require.Nil(t, reviews.Rows[0].Resolution.GrossAmount)
	recorded, err := f.importService.ReviewImportDividend(ctx, ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID, Resolution: reviewedDividendResolution(f, "100.00", "15.00")})
	var validation ValidationError
	require.ErrorAs(t, err, &validation) // tax account still needs owner choice
	require.Zero(t, recorded.ID)
	taxAccount := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	resolution := reviewedDividendResolution(f, "100.00", "15.00")
	resolution.WithholdingAccountID = &taxAccount
	recorded, err = f.importService.ReviewImportDividend(ctx, ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID, Resolution: resolution})
	require.NoError(t, err)
	require.Equal(t, "investment", recorded.TransactionKind)
	reviews, err = f.importService.ListImportDividends(ctx, ListImportDividendsInput{OwnerUserID: f.ownerUserID})
	require.NoError(t, err)
	require.Empty(t, reviews.Rows)
}

func TestImportedDividendRepairKeepsTransactionCashSourceAndNativeCorrections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, batch, rowID, transactionID, payer := legacyDividendFallback(t, "85.00")
	original, err := f.importService.transactionService.Transaction(ctx, transactionID)
	require.NoError(t, err)
	reviews, err := f.importService.ListImportDividends(ctx, ListImportDividendsInput{OwnerUserID: f.ownerUserID})
	require.NoError(t, err)
	require.Len(t, reviews.Rows, 1)
	require.Equal(t, "repairable", reviews.Rows[0].State)
	tax := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	resolution := reviewedDividendResolution(f, "100.00", "15.00")
	resolution.WithholdingAccountID = &tax
	input := ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID, ExpectedTransactionVersionID: original.VersionID, Reason: "Statement identifies payer and gross/tax", Resolution: resolution}
	before := csvIncomeDurableCount(t, f)
	impact, err := f.importService.ImportDividendReviewImpact(ctx, input)
	require.NoError(t, err)
	require.Empty(t, impact.AffectedCheckpoints)
	require.Equal(t, before, csvIncomeDurableCount(t, f))
	repaired, err := f.importService.ReviewImportDividend(ctx, input)
	require.NoError(t, err)
	require.Equal(t, transactionID, repaired.ID)
	require.Equal(t, original.VersionSeq+1, repaired.VersionSeq)
	require.Equal(t, "investment", repaired.TransactionKind)
	require.Equal(t, original.Description, repaired.Description)
	require.Equal(t, original.NoteMarkdown, repaired.NoteMarkdown)
	require.False(t, repaired.NeedsReview)
	require.Equal(t, original.ExternalRefHint, repaired.ExternalRefHint)
	require.Equal(t, original.TransactionDaySequence, repaired.TransactionDaySequence)
	require.Equal(t, original.PayeeID, repaired.PayeeID)
	require.Equal(t, original.PayeeName, repaired.PayeeName)
	require.Equal(t, original.TagIDs, repaired.TagIDs)
	oldCash, newCash := original.JournalEntries[0].Postings[0], repaired.JournalEntries[0].Postings[0]
	require.Equal(t, oldCash.LineKey, newCash.LineKey)
	require.Equal(t, oldCash.Memo, newCash.Memo)
	require.Equal(t, oldCash.TagIDs, newCash.TagIDs)
	require.Equal(t, oldCash.AccountDaySequence, newCash.AccountDaySequence)
	require.JSONEq(t, original.MetadataJSON, repaired.MetadataJSON)
	var transactions int
	require.NoError(t, f.database.QueryRow("SELECT count(*) FROM transactions").Scan(&transactions))
	require.Equal(t, 1, transactions)
	rows, err := f.importRepo.ListAllImportStagedRows(ctx, batch)
	require.NoError(t, err)
	require.Len(t, rows[0].CommitEffects, 2)
	require.True(t, rows[0].CommitEffects[1].OperationID.Valid)
	require.False(t, rows[0].CommitEffects[1].TransactionID.Valid)
	chain, err := f.investmentSvc.CorrectionChain(ctx, f.ownerUserID, transactionID)
	require.NoError(t, err)
	require.Equal(t, &payer, chain.EffectiveDividend.CommodityID)
	require.Equal(t, "10000", chain.EffectiveDividend.AmountValue.String())
	_, err = f.investmentSvc.ReplaceDividend(ctx, ReplaceInvestmentDividendInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID, Reason: "owner correction", Replacement: DividendInput{TransactionDate: "2026-03-01", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.categoryAccountID, AmountValue: 9000, AmountScale: 2}})
	require.NoError(t, err)
	sourceBatch, err := f.importRepo.ImportBatchByID(ctx, BookID, batch)
	require.NoError(t, err)
	again, _ := f.stageDividendRow(t, sourceBatch.ConnectionID.Int64, trading212Dividend{Reference: "legacy-dividend", Ticker: "AAPL_US_EQ", ISIN: "US0378331005", Amount: "85.00", Currency: "EUR", PaidOn: "2026-03-01", Type: "DIVIDEND"})
	duplicates, err := f.importRepo.ListAllImportStagedRows(ctx, again)
	require.NoError(t, err)
	require.Equal(t, "duplicate", duplicates[0].DedupeStatus)
	snapshot, err := db.NewExportRepository(f.database).Snapshot(ctx)
	require.NoError(t, err)
	facts, err := db.NewExportRepository(f.database).ExportInvestmentFoundation(ctx, snapshot, BookID, "dividend-facts")
	require.NoError(t, err)
	require.Len(t, facts, 2)
	require.NoError(t, snapshot.Rollback())

}

func TestImportedDividendRepairLateIdentityFailureRollsBackAllFacts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, batch, rowID, transactionID, _ := legacyDividendFallback(t, "85.00")
	original, err := f.importService.transactionService.Transaction(ctx, transactionID)
	require.NoError(t, err)
	_, err = f.database.Exec(`CREATE TRIGGER fail_repaired_dividend_identity BEFORE INSERT ON import_commit_identity_effects WHEN NEW.effect_seq > 1 BEGIN SELECT RAISE(ABORT,'late repair identity failure'); END`)
	require.NoError(t, err)
	before := csvIncomeDurableCount(t, f)
	_, err = f.importService.ReviewImportDividend(ctx, ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID, ExpectedTransactionVersionID: original.VersionID, Reason: "payer repair", Resolution: reviewedDividendResolution(f, "85.00", "0")})
	require.ErrorContains(t, err, "late repair identity failure")
	require.Equal(t, before, csvIncomeDurableCount(t, f))
	current, err := f.importService.transactionService.Transaction(ctx, transactionID)
	require.NoError(t, err)
	require.Equal(t, original.VersionID, current.VersionID)
	require.Equal(t, "ordinary", current.TransactionKind)
}

// An explicit owner declaration in the existing native-import regressions;
// unlike staging, this confirms that this fixture has no withholding.
func declareDividendCashAsGross(t *testing.T, f *investTestFixture, batchID, rowID int64) {
	t.Helper()
	ctx := context.Background()
	row, err := f.importRepo.ImportStagedRowByID(ctx, rowID)
	require.NoError(t, err)
	res, err := parseResolutionJSON(row.ResolutionJSON)
	require.NoError(t, err)
	var source importDividendSource
	require.NoError(t, json.Unmarshal([]byte(row.NormalizedJSON), &source))
	zero := "0"
	res.GrossAmount = &source.Amount
	res.WithholdingAmount = &zero
	encoded, err := json.Marshal(res)
	require.NoError(t, err)
	require.NoError(t, f.importRepo.UpdateImportStagedRowResolution(ctx, db.UpdateImportStagedRowResolutionParams{RowID: rowID, BatchID: batchID, DedupeStatus: "new", ResolutionJSON: string(encoded)}))
}

func TestImportedDividendRepairPreservesUnchangedReconciliationAndRejectsStaleReview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, batch, rowID, transactionID, _ := legacyDividendFallback(t, "100.00")
	fixture := &investmentsTestFixture{database: f.database, ownerUserID: f.ownerUserID, cashAccountID: f.cashAccountID, eurCommodityID: f.eurCommodityID, transactionService: f.importService.transactionService}
	checkpoint := reconcileCash(t, fixture, "2026-04-01", 100)
	current, err := f.importService.transactionService.Transaction(ctx, transactionID)
	require.NoError(t, err)
	input := ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID, ExpectedTransactionVersionID: current.VersionID, Reason: "payer attribution only", Resolution: reviewedDividendResolution(f, "100.00", "0")}
	impact, err := f.importService.ImportDividendReviewImpact(ctx, input)
	require.NoError(t, err)
	require.Empty(t, impact.AffectedCheckpoints)
	repaired, err := f.importService.ReviewImportDividend(ctx, input)
	require.NoError(t, err)
	require.Equal(t, transactionID, repaired.ID)
	require.Equal(t, []int64{checkpoint}, activeCheckpointIDs(t, fixture))
	var cashReconciled bool
	for _, entry := range repaired.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.AccountID == f.cashAccountID {
				require.Equal(t, "reconciled", posting.ReconciliationStatus)
				cashReconciled = true
			}
		}
	}
	require.True(t, cashReconciled)
	before := csvIncomeDurableCount(t, f)
	_, err = f.importService.ReviewImportDividend(ctx, input)
	require.Error(t, err)
	require.Equal(t, before, csvIncomeDurableCount(t, f))
	snapshot, err := f.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	require.NoError(t, err)
	check, err := NewSelfCheckService(db.NewSelfCheckRepository(f.database, f.database)).investmentFoundationCheck(ctx, snapshot)
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, check.Status, check.Summary)
	require.NoError(t, snapshot.Rollback())
}

// TestImportedDividendRepairKeepsArchivedTagAssignments pins the #650 bug
// class on the dividend repair: it restates the original's transaction and
// posting tags, so a tag archived after the import must not block the repair.
func TestImportedDividendRepairKeepsArchivedTagAssignments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, batch, rowID, transactionID, _ := legacyDividendFallback(t, "100.00")
	tags := NewTagService(db.NewTagRepository(f.database))
	tag, err := tags.CreateTag(ctx, CreateTagInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "tag.create", Name: "Retired broker", Kind: "project"})
	require.NoError(t, err)
	original, err := f.importService.transactionService.Transaction(ctx, transactionID)
	require.NoError(t, err)
	var cashLineID int64
	for _, entry := range original.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.AccountID == f.cashAccountID {
				cashLineID = posting.PostingLineID
			}
		}
	}
	require.NotZero(t, cashLineID)
	_, err = f.database.ExecContext(ctx, `INSERT INTO transaction_tags (book_id, transaction_id, tag_id, created_at, created_by_user_id) VALUES (?, ?, ?, '2026-04-01T00:00:00Z', ?)`, BookID, transactionID, tag.ID, f.ownerUserID)
	require.NoError(t, err)
	_, err = f.database.ExecContext(ctx, `INSERT INTO posting_tags (book_id, posting_line_id, tag_id, created_at, created_by_user_id) VALUES (?, ?, ?, '2026-04-01T00:00:00Z', ?)`, BookID, cashLineID, tag.ID, f.ownerUserID)
	require.NoError(t, err)
	_, err = tags.ArchiveTag(ctx, TagLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", TagID: tag.ID})
	require.NoError(t, err)

	current, err := f.importService.transactionService.Transaction(ctx, transactionID)
	require.NoError(t, err)
	repaired, err := f.importService.ReviewImportDividend(ctx, ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID, ExpectedTransactionVersionID: current.VersionID, Reason: "payer attribution only", Resolution: reviewedDividendResolution(f, "100.00", "0")})
	require.NoError(t, err)
	require.Contains(t, repaired.TagIDs, tag.ID)
	var cashTags []int64
	for _, entry := range repaired.JournalEntries {
		for _, posting := range entry.Postings {
			if posting.AccountID == f.cashAccountID {
				require.Equal(t, cashLineID, posting.PostingLineID)
				cashTags = posting.TagIDs
			}
		}
	}
	require.Contains(t, cashTags, tag.ID)
}

func TestImportedDividendRepairGrossTaxChangeRequiresReconciliationOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, batch, rowID, transactionID, _ := legacyDividendFallback(t, "85.00")
	fixture := &investmentsTestFixture{database: f.database, ownerUserID: f.ownerUserID, cashAccountID: f.cashAccountID, eurCommodityID: f.eurCommodityID, transactionService: f.importService.transactionService}
	checkpoint := reconcileCash(t, fixture, "2026-04-01", 85)
	current, err := f.importService.transactionService.Transaction(ctx, transactionID)
	require.NoError(t, err)
	tax := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	resolution := reviewedDividendResolution(f, "100.00", "15.00")
	resolution.WithholdingAccountID = &tax
	input := ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID, ExpectedTransactionVersionID: current.VersionID, Reason: "statement gross and withholding", Resolution: resolution}
	before := csvIncomeDurableCount(t, f)
	impact, err := f.importService.ImportDividendReviewImpact(ctx, input)
	require.NoError(t, err)
	require.Len(t, impact.AffectedCheckpoints, 1)
	require.Equal(t, checkpoint, impact.AffectedCheckpoints[0].CheckpointID)
	require.Equal(t, before, csvIncomeDurableCount(t, f))
	_, err = f.importService.ReviewImportDividend(ctx, input)
	require.ErrorIs(t, err, ErrReconciliationOverrideRequired)
	require.Equal(t, before, csvIncomeDurableCount(t, f))
	input.ReconciliationOverride = unboundOverride
	_, err = f.importService.ReviewImportDividend(ctx, input)
	require.NoError(t, err)
	require.Empty(t, activeCheckpointIDs(t, fixture))
}

func TestReviewedDividendIdentityFailureCleansNewInstrumentAndCanRetryWithoutProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	batch, rowID := f.stageDividendRow(t, conn.ID, trading212Dividend{Reference: "retry", Ticker: "NEWPAYER", Amount: "85.00", Currency: "EUR", PaidOn: "2026-03-01", Type: "DIVIDEND"})
	input := ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID, Resolution: reviewedDividendResolution(f, "85.00", "0")}
	failInvestmentImportIdentityWrites(t, f)
	_, err := f.importService.ReviewImportDividend(ctx, input)
	require.ErrorContains(t, err, "injected investment import identity failure")
	instruments, err := f.investmentSvc.ListInstruments(ctx)
	require.NoError(t, err)
	require.Empty(t, instruments)
	require.Zero(t, transactionCount(t, f))
	_, err = f.database.Exec("DROP TRIGGER fail_investment_import_identity")
	require.NoError(t, err)
	_, err = f.importService.ReviewImportDividend(ctx, input)
	require.NoError(t, err)
	require.Equal(t, 1, transactionCount(t, f))
}

func TestImportedDividendRepairCreatesSourcePayerOnlyOnSuccessfulCommit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, batch, rowID, transactionID, _ := legacyDividendFallbackWithPayer(t, "85.00", false)
	current, err := f.importService.transactionService.Transaction(ctx, transactionID)
	require.NoError(t, err)
	input := ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID, ExpectedTransactionVersionID: current.VersionID, Reason: "Confirm source payer", Resolution: reviewedDividendResolution(f, "85.00", "0")}
	_, err = f.importService.ImportDividendReviewImpact(ctx, input)
	require.NoError(t, err)
	instruments, err := f.investmentSvc.ListInstruments(ctx)
	require.NoError(t, err)
	require.Empty(t, instruments)
	_, err = f.database.Exec(`CREATE TRIGGER fail_new_repair_identity BEFORE INSERT ON import_commit_identity_effects WHEN NEW.effect_seq>1 BEGIN SELECT RAISE(ABORT,'repair retry'); END`)
	require.NoError(t, err)
	_, err = f.importService.ReviewImportDividend(ctx, input)
	require.ErrorContains(t, err, "repair retry")
	instruments, err = f.investmentSvc.ListInstruments(ctx)
	require.NoError(t, err)
	require.Empty(t, instruments)
	require.Equal(t, 1, transactionCount(t, f))
	_, err = f.database.Exec("DROP TRIGGER fail_new_repair_identity")
	require.NoError(t, err)
	repaired, err := f.importService.ReviewImportDividend(ctx, input)
	require.NoError(t, err)
	require.Equal(t, transactionID, repaired.ID)
	instruments, err = f.investmentSvc.ListInstruments(ctx)
	require.NoError(t, err)
	require.Len(t, instruments, 1)
	var payer int64
	require.NoError(t, f.database.QueryRow("SELECT commodity_id FROM investment_dividend_facts").Scan(&payer))
	require.Equal(t, instruments[0].CommodityID, payer)
}

func TestImportedDividendRepairWriterPinsVersionAndSourceEligibility(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"owner_edit", "discard"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f, batch, rowID, transactionID, _ := legacyDividendFallback(t, "85.00")
			c, err := f.importService.importDividendContext(ctx, f.ownerUserID, batch, rowID)
			require.NoError(t, err)
			p, err := f.importService.prepareImportDividendRepair(ctx, c, ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID, ExpectedTransactionVersionID: c.original.VersionID, Reason: "Prepared review", Resolution: reviewedDividendResolution(f, "85.00", "0")})
			require.NoError(t, err)
			if action == "discard" {
				require.NoError(t, f.importService.DiscardImportBatch(ctx, DiscardImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch}))
			} else {
				spec := transactionInputFromTransaction(toTransaction(*c.original))
				spec.NoteMarkdown = "Newer owner edit"
				_, err = f.importService.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{OwnerUserID: f.ownerUserID, TransactionID: transactionID, ExpectedVersionID: c.original.VersionID, Spec: spec, OriginType: "browser_api", Operation: "transaction.update", ChangeReason: "Owner edit after review"})
				require.NoError(t, err)
			}
			before := csvIncomeDurableCount(t, f)
			_, err = f.importRepo.RepairImportDividend(ctx, p, false)
			if action == "discard" {
				require.ErrorIs(t, err, db.ErrImportDividendReviewUnavailable)
			} else {
				require.ErrorIs(t, err, db.ErrTransactionVersionStale)
			}
			require.Equal(t, before, csvIncomeDurableCount(t, f))
		})
	}
}

func TestReviewedDividendMissingPayerCanRemainUnassignedWithoutInventingTax(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	batch, rowID := f.stageDividendRow(t, conn.ID, trading212Dividend{Reference: "unassigned", Amount: "85.00", Currency: "EUR", PaidOn: "2026-03-01", Type: "DIVIDEND"})
	resolution := reviewedDividendResolution(f, "85.00", "0")
	resolution.WithholdingAmount = nil
	input := ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID, Resolution: resolution}
	_, err := f.importService.ReviewImportDividend(ctx, input)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Zero(t, transactionCount(t, f))
	zero := "0"
	input.Resolution.WithholdingAmount = &zero
	recorded, err := f.importService.ReviewImportDividend(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "investment", recorded.TransactionKind)
	chain, err := f.investmentSvc.CorrectionChain(ctx, f.ownerUserID, recorded.ID)
	require.NoError(t, err)
	require.Nil(t, chain.EffectiveDividend.CommodityID)
	instruments, err := f.investmentSvc.ListInstruments(ctx)
	require.NoError(t, err)
	require.Empty(t, instruments)
}

func TestReviewedDividendFailedBatchRetryAndQueuePagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	var rowIDs []int64
	var batchIDs []int64
	for i := range 3 {
		batch, row := f.stageDividendRow(t, conn.ID, trading212Dividend{Reference: fmt.Sprintf("page-%d", i), Ticker: "PAYER", Amount: "85.00", Currency: "EUR", PaidOn: "2026-03-01", Type: "DIVIDEND"})
		rowIDs = append(rowIDs, row)
		batchIDs = append(batchIDs, batch)
	}
	_, err := f.database.Exec("UPDATE import_staged_rows SET commit_status='failed',commit_error='interrupted import' WHERE id=?", rowIDs[0])
	require.NoError(t, err)
	_, err = f.database.Exec("UPDATE import_batches SET status='failed' WHERE id=?", batchIDs[0])
	require.NoError(t, err)
	var cursor string
	var listed []int64
	for {
		page, err := f.importService.ListImportDividends(ctx, ListImportDividendsInput{OwnerUserID: f.ownerUserID, Limit: 1, Cursor: cursor})
		require.NoError(t, err)
		require.Len(t, page.Rows, 1)
		listed = append(listed, page.Rows[0].RowID)
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	require.Equal(t, rowIDs, listed)
	_, err = f.importService.ReviewImportDividend(ctx, ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batchIDs[0], RowID: rowIDs[0], Resolution: reviewedDividendResolution(f, "85.00", "0")})
	require.NoError(t, err)
	batch, err := f.importRepo.ImportBatchByID(ctx, BookID, batchIDs[0])
	require.NoError(t, err)
	require.Equal(t, "committed", batch.Status)
}

func TestReviewedDividendPendingWriterRefusesDiscardWithoutFinancialEffects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestTestFixture(t)
	conn := f.createConnection(t, &f.cashAccountID)
	batch, rowID := f.stageDividendRow(t, conn.ID, trading212Dividend{Reference: "discard-race", Amount: "85.00", Currency: "EUR", PaidOn: "2026-03-01", Type: "DIVIDEND"})
	c, err := f.importService.importDividendContext(ctx, f.ownerUserID, batch, rowID)
	require.NoError(t, err)
	input, res, err := f.importService.reviewedDividendInput(ctx, c, reviewedDividendResolution(f, "85.00", "0"), dividendDefaultFill{}, f.ownerUserID, 0, "")
	require.NoError(t, err)
	require.NoError(t, f.importService.DiscardImportBatch(ctx, DiscardImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch}))
	before := csvIncomeDurableCount(t, f)
	_, err = f.importService.writeReviewedPendingDividend(ctx, c, input, res, f.importService.now().UTC().Format(time.RFC3339), true)
	require.ErrorIs(t, err, db.ErrImportDividendReviewUnavailable)
	require.Equal(t, before, csvIncomeDurableCount(t, f))
}
