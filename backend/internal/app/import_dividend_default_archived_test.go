package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
)

// #664 review. Import review fills a dividend's income account from the stored
// default when the owner left it out; that account is the default's, not the
// owner's, so the writer checks the default still applies it. A category
// archived after the review filled it refuses the write and holds the row for
// review, where the archived default no longer fills in.
func TestImportedDividendDefaultArchivedBeforeWriteIsHeld(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	setup := func(t *testing.T) (*investTestFixture, *CategoryService, Category, int64, int64) {
		f := newInvestTestFixture(t)
		conn := f.createConnection(t, &f.cashAccountID)
		categories := NewCategoryService(db.NewCategoryRepository(f.database), nil)
		category, err := categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Imported dividends", CategoryType: "income"})
		require.NoError(t, err)
		// A global default: the row's instrument does not exist yet.
		_, err = f.investmentSvc.SaveDividendDefault(ctx, DividendDefaultInput{OwnerUserID: f.ownerUserID, IncomeAccountID: category.ID, EffectiveFrom: "2026-01-01", ChangeReason: "test fixture"})
		require.NoError(t, err)
		batch, row := f.stageDividendRow(t, conn.ID, trading212Dividend{
			Reference: "div-archived", Ticker: "AAPL_US_EQ", ISIN: "US0378331005",
			Quantity: "2", Amount: "4.32", Currency: "EUR", PaidOn: "2026-06-15T00:00:00Z", Type: "DIVIDEND",
		})
		declareDividendCashAsGross(t, f, batch, row)
		return f, categories, category, batch, row
	}

	t.Run("control: the default fills in and the row commits", func(t *testing.T) {
		f, _, category, batch, _ := setup(t)
		result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch})
		require.NoError(t, err)
		assert.Equal(t, 1, result.CommittedCount)
		var posted int
		require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM posting_versions WHERE account_id = ?`, category.ID).Scan(&posted))
		assert.Equal(t, 1, posted)
	})

	t.Run("a review list default sent back unchanged is checked; an owner's choice is not", func(t *testing.T) {
		f, categories, category, batch, row := setup(t)
		reviews, err := f.importService.ListImportDividends(ctx, ListImportDividendsInput{OwnerUserID: f.ownerUserID, BatchID: batch})
		require.NoError(t, err)
		require.Len(t, reviews.Rows, 1)
		listed := reviews.Rows[0]
		require.NotNil(t, listed.Resolution.CategoryID)
		require.Equal(t, category.ID, *listed.Resolution.CategoryID)
		require.True(t, listed.IncomeAccountFromDefault, "the review list says the default filled the account")

		_, err = categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: category.ID})
		require.NoError(t, err)

		// The browser sends the listed resolution back with its marks.
		input := ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: row, Resolution: listed.Resolution,
			IncomeAccountFromDefault: listed.IncomeAccountFromDefault, WithholdingAccountFromDefault: listed.WithholdingAccountFromDefault}
		_, err = f.importService.ImportDividendReviewImpact(ctx, input)
		require.ErrorIs(t, err, ErrDividendDefaultChanged)
		_, err = f.importService.ReviewImportDividend(ctx, input)
		require.ErrorIs(t, err, ErrDividendDefaultChanged)
		var posted int
		require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM posting_versions WHERE account_id = ?`, category.ID).Scan(&posted))
		assert.Zero(t, posted)

		// Picked by the owner for a day it was open, the category is the ledger's call.
		input.IncomeAccountFromDefault = false
		_, err = f.importService.ReviewImportDividend(ctx, input)
		require.NoError(t, err)
	})

	t.Run("archived after the review filled it, the write is refused and the row held", func(t *testing.T) {
		f, categories, category, batch, row := setup(t)
		c, err := f.importService.importDividendContext(ctx, f.ownerUserID, batch, row)
		require.NoError(t, err)
		input, res, err := f.importService.reviewedDividendInput(ctx, c, c.resolution, dividendDefaultFill{}, f.ownerUserID, 0, "")
		require.NoError(t, err)
		require.NotNil(t, input.IncomeAccountID)
		require.Equal(t, category.ID, *input.IncomeAccountID)
		require.True(t, input.IncomeAccountFromDefault, "the review marks the account it took from the default")

		_, err = categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: category.ID})
		require.NoError(t, err)

		_, err = f.importService.writeReviewedPendingDividend(ctx, c, input, res, "2026-06-16T00:00:00Z", false)
		require.ErrorIs(t, err, ErrDividendDefaultChanged)
		var posted int
		require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM posting_versions WHERE account_id = ?`, category.ID).Scan(&posted))
		assert.Zero(t, posted, "nothing posted to the archived category")

		result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batch})
		require.NoError(t, err)
		assert.Zero(t, result.CommittedCount)
		assert.Contains(t, result.DividendReviewRowIDs, row, "the row waits for review")
	})
}

// #664 review. Every journal that took an account from the stored default
// re-resolves it in its write transaction: a repair and the replacement
// journal of a correction (not only the first journal of a command) refuse a
// default repointed between preparing and committing.
func TestDividendDefaultRepointedBeforeRepairOrReplacementCommitIsRefused(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("import repair", func(t *testing.T) {
		f, batch, rowID, transactionID, payer := legacyDividendFallback(t, "85.00")
		first := seedTestAccountWithClass(t, f.database, "active", true, "income", "income")
		second := seedTestAccountWithClass(t, f.database, "active", true, "income", "income")
		saved, err := f.investmentSvc.SaveDividendDefault(ctx, DividendDefaultInput{OwnerUserID: f.ownerUserID, CommodityID: &payer, IncomeAccountID: first, EffectiveFrom: "2026-01-01", ChangeReason: "fixture"})
		require.NoError(t, err)
		original, err := f.importService.transactionService.Transaction(ctx, transactionID)
		require.NoError(t, err)
		resolution := reviewedDividendResolution(f, "85.00", "0")
		resolution.CategoryID = nil
		resolution.InstrumentCommodityID = payer
		c, err := f.importService.importDividendContext(ctx, f.ownerUserID, batch, rowID)
		require.NoError(t, err)
		params, err := f.importService.prepareImportDividendRepair(ctx, c, ReviewImportDividendInput{OwnerUserID: f.ownerUserID, BatchID: batch, RowID: rowID,
			ExpectedTransactionVersionID: original.VersionID, Reason: "statement shows the payer", Resolution: resolution})
		require.NoError(t, err)
		require.NotNil(t, params.Journal.DividendDefault)

		_, err = f.investmentSvc.SaveDividendDefault(ctx, DividendDefaultInput{DefaultID: saved.ID, OwnerUserID: f.ownerUserID, CommodityID: &payer, IncomeAccountID: second, EffectiveFrom: "2026-01-01", ChangeReason: "repointed"})
		require.NoError(t, err)
		_, err = f.importRepo.RepairImportDividend(ctx, params, false)
		require.ErrorIs(t, err, ErrDividendDefaultChanged)
	})

	t.Run("cash dividend replacement", func(t *testing.T) {
		f := newInvestTestFixture(t)
		_, payer, _, err := f.investmentSvc.ResolveOrCreateInstrumentForImport(ctx, f.ownerUserID, 0, "", "US0378331005", "AAPL_US_EQ")
		require.NoError(t, err)
		first := seedTestAccountWithClass(t, f.database, "active", true, "income", "income")
		second := seedTestAccountWithClass(t, f.database, "active", true, "income", "income")
		saved, err := f.investmentSvc.SaveDividendDefault(ctx, DividendDefaultInput{OwnerUserID: f.ownerUserID, CommodityID: &payer, IncomeAccountID: first, EffectiveFrom: "2026-01-01", ChangeReason: "fixture"})
		require.NoError(t, err)
		posted, err := f.investmentSvc.Dividend(ctx, DividendInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01", CommodityID: NullableValue(payer),
			CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.categoryAccountID, AmountValue: 500, AmountScale: 2})
		require.NoError(t, err)
		operation, inverse, replacement, err := f.investmentSvc.prepareDividendReplacementWrite(ctx, ReplaceInvestmentDividendInput{OwnerUserID: f.ownerUserID,
			TransactionID: posted.ID, Reason: "use the default income account",
			Replacement: DividendInput{TransactionDate: "2026-03-01", CommodityID: NullableValue(payer), CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, AmountValue: 500, AmountScale: 2}})
		require.NoError(t, err)
		require.Nil(t, inverse.DividendDefault)
		require.NotNil(t, replacement.DividendDefault)

		_, err = f.investmentSvc.SaveDividendDefault(ctx, DividendDefaultInput{DefaultID: saved.ID, OwnerUserID: f.ownerUserID, CommodityID: &payer, IncomeAccountID: second, EffectiveFrom: "2026-01-01", ChangeReason: "repointed"})
		require.NoError(t, err)
		_, err = f.investmentSvc.repository.ReplaceDividend(ctx, operation, inverse, replacement)
		require.ErrorIs(t, err, ErrDividendDefaultChanged)
	})
}
