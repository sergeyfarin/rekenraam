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

	t.Run("archived after the review filled it, the write is refused and the row held", func(t *testing.T) {
		f, categories, category, batch, row := setup(t)
		c, err := f.importService.importDividendContext(ctx, f.ownerUserID, batch, row)
		require.NoError(t, err)
		input, res, err := f.importService.reviewedDividendInput(ctx, c, c.resolution, f.ownerUserID, 0, "")
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
