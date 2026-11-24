package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// #664 review. A dividend default is a stored automation target: it stays
// saved for repair, but applying it skips a target archived today, as import
// rules do. A category disabled on D still accepts postings dated D, so a
// default naming it would otherwise post there silently when the request
// omits the account. Naming the category explicitly is still the ledger's
// decision, and it accepts the disable day.
func TestDividendDefaultSkipsAnArchivedTarget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	const day = "2026-03-01"
	now := func() time.Time { return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC) }
	categories := NewCategoryService(db.NewCategoryRepository(f.database), nil)
	categories.SetNowForTest(now)
	f.investmentService.SetNowForTest(now)

	income, err := categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Dividends", CategoryType: "income"})
	require.NoError(t, err)
	withholding, err := categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Dividend tax", CategoryType: "expense"})
	require.NoError(t, err)
	_, err = f.investmentService.SaveDividendDefault(ctx, DividendDefaultInput{
		OwnerUserID: f.ownerUserID, CommodityID: &f.stockCommodityID,
		IncomeAccountID: income.ID, WithholdingAccountID: &withholding.ID, Status: "active", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	for _, id := range []int64{income.ID, withholding.ID} {
		_, err = categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: id, EffectiveFrom: day})
		require.NoError(t, err)
	}

	cash := func(incomeID *int64) DividendInput {
		return DividendInput{OwnerUserID: f.ownerUserID, TransactionDate: day, CommodityID: NullableValue(f.stockCommodityID),
			CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: incomeID, AmountValue: 1000, AmountScale: 2}
	}

	t.Run("a cash dividend omitting the income account is refused", func(t *testing.T) {
		_, err := f.investmentService.Dividend(ctx, cash(nil))
		require.Error(t, err)
		assert.ErrorContains(t, err, "dividend income account is required")
	})

	t.Run("a reinvested dividend omitting the income account is refused", func(t *testing.T) {
		_, err := acknowledgedReinvestedDividend(ctx, f.investmentService, ReinvestedDividendInput{
			OwnerUserID: f.ownerUserID, TransactionDate: day, CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
			QuantityValue: exact.New(1), QuantityScale: 0, AmountValue: 2500, AmountScale: 2, CashCommodityID: f.eurCommodityID,
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "dividend income account is required")
	})

	t.Run("withholding does not fall back to an archived default", func(t *testing.T) {
		withheld, scale := int64(150), 2
		_, err := f.investmentService.Dividend(ctx, DividendInput{OwnerUserID: f.ownerUserID, TransactionDate: day, CommodityID: NullableValue(f.stockCommodityID),
			CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.incomeAccountID,
			AmountValue: 1000, AmountScale: 2, WithholdingValue: &withheld, WithholdingScale: &scale})
		require.Error(t, err)
	})

	t.Run("naming the category explicitly on its disable day is accepted", func(t *testing.T) {
		recorded, err := f.investmentService.Dividend(ctx, cash(&income.ID))
		require.NoError(t, err)
		accounts := []int64{}
		for _, entry := range recorded.JournalEntries {
			for _, posting := range entry.Postings {
				accounts = append(accounts, posting.AccountID)
			}
		}
		assert.Contains(t, accounts, income.ID)
	})
}

// #664 review. The default's target is decided on the status read with it;
// an archive committed between that read and the role check must refuse the
// write, not pass it because the role check saw the newer version first.
func TestDividendDefaultDecisionIsBoundToTheVersionItRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	const day = "2026-03-01"
	now := func() time.Time { return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC) }
	categories := NewCategoryService(db.NewCategoryRepository(f.database), nil)
	categories.SetNowForTest(now)
	f.investmentService.SetNowForTest(now)
	income, err := categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: "Dividends", CategoryType: "income"})
	require.NoError(t, err)
	_, err = f.investmentService.SaveDividendDefault(ctx, DividendDefaultInput{
		OwnerUserID: f.ownerUserID, CommodityID: &f.stockCommodityID, IncomeAccountID: income.ID, Status: "active", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	input := DividendInput{OwnerUserID: f.ownerUserID, TransactionDate: day, CommodityID: NullableValue(f.stockCommodityID),
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, AmountValue: 1000, AmountScale: 2}

	dependencies := newAccountRuleDependencies()
	resolved, err := f.investmentService.withDividendDefault(ctx, input, day, dependencies)
	require.NoError(t, err)
	require.NotNil(t, resolved.IncomeAccountID)
	require.Equal(t, income.ID, *resolved.IncomeAccountID)

	// The archive lands between the default decision and the role check.
	_, err = categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: income.ID, EffectiveFrom: day})
	require.NoError(t, err)

	plan, err := f.investmentService.dividendPlanFor(ctx, resolved, day, dependencies)
	require.NoError(t, err, "the disable day still accepts the posting on its own")
	params, err := f.transactionService.prepareCreateTransactionForWriteCarrying(ctx, plan.Create, plan.AccountRuleDependencies)
	require.NoError(t, err)
	_, err = f.transactionService.repository.CreateTransactionWithPostWrite(ctx, params, nil)
	require.ErrorIs(t, err, db.ErrPostingAccountVersionStale)
}
