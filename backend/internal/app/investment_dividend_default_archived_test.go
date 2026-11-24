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

// #664 review. Only a default target that supplies an account the posting
// uses binds the write: archiving an unused default between planning and
// writing must not refuse a dividend whose accounts were named explicitly.
func TestUnusedDividendDefaultDoesNotBindTheWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	const day = "2026-03-01"
	now := func() time.Time { return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC) }
	categories := NewCategoryService(db.NewCategoryRepository(f.database), nil)
	categories.SetNowForTest(now)
	f.investmentService.SetNowForTest(now)
	create := func(name, kind string) Category {
		category, err := categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: name, CategoryType: kind})
		require.NoError(t, err)
		return category
	}
	unusedIncome := create("Default dividends", "income")
	unusedWithholding := create("Default dividend tax", "expense")
	_, err := f.investmentService.SaveDividendDefault(ctx, DividendDefaultInput{
		OwnerUserID: f.ownerUserID, CommodityID: &f.stockCommodityID, IncomeAccountID: unusedIncome.ID,
		WithholdingAccountID: &unusedWithholding.ID, Status: "active", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)

	plan, err := f.investmentService.dividendPlan(ctx, DividendInput{OwnerUserID: f.ownerUserID, TransactionDate: day,
		CommodityID: NullableValue(f.stockCommodityID), CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		IncomeAccountID: &f.incomeAccountID, AmountValue: 1000, AmountScale: 2})
	require.NoError(t, err)
	for _, id := range []int64{unusedIncome.ID, unusedWithholding.ID} {
		_, err = categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: id, EffectiveFrom: day})
		require.NoError(t, err)
	}

	params, err := f.transactionService.prepareCreateTransactionForWriteCarrying(ctx, plan.Create, plan.AccountRuleDependencies)
	require.NoError(t, err)
	_, err = f.transactionService.repository.CreateTransactionWithPostWrite(ctx, params, nil)
	require.NoError(t, err, "neither default target is used by this posting")
}

// #664 review. An account the request says came from the default is the
// default's decision, not the owner's: the write requires the default to still
// apply it, on the server at commit, and refuses when it changed or its account
// was archived since the form showed it.
func TestDividendAccountFromDefaultIsCheckedAtWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newInvestmentsTestFixture(t)
	const day, yesterday = "2026-03-01", "2026-02-28"
	now := func() time.Time { return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC) }
	categories := NewCategoryService(db.NewCategoryRepository(f.database), nil)
	categories.SetNowForTest(now)
	f.investmentService.SetNowForTest(now)
	create := func(name string) Category {
		category, err := categories.CreateCategory(ctx, CreateCategoryInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "category.create", Name: name, CategoryType: "income"})
		require.NoError(t, err)
		return category
	}
	shown := create("Shown default")
	saved, err := f.investmentService.SaveDividendDefault(ctx, DividendDefaultInput{
		OwnerUserID: f.ownerUserID, CommodityID: &f.stockCommodityID, IncomeAccountID: shown.ID, Status: "active", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	cash := func(date string) DividendInput {
		return DividendInput{OwnerUserID: f.ownerUserID, TransactionDate: date, CommodityID: NullableValue(f.stockCommodityID),
			CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &shown.ID, IncomeAccountFromDefault: true,
			AmountValue: 1000, AmountScale: 2}
	}

	t.Run("the shown default still applying is accepted and bound to its version", func(t *testing.T) {
		plan, err := f.investmentService.dividendPlan(ctx, cash(yesterday))
		require.NoError(t, err)
		require.Contains(t, plan.AccountRuleDependencies.list(), db.AccountRuleDependency{AccountID: shown.ID, LatestVersionID: accountLatestVersionID(t, f, shown.ID)})
	})

	t.Run("a default changed since it was shown is refused", func(t *testing.T) {
		other := create("Newer default")
		_, err := f.investmentService.SaveDividendDefault(ctx, DividendDefaultInput{DefaultID: saved.ID,
			OwnerUserID: f.ownerUserID, CommodityID: &f.stockCommodityID, IncomeAccountID: other.ID, Status: "active", EffectiveFrom: "2026-01-01",
		})
		require.NoError(t, err)
		_, err = f.investmentService.Dividend(ctx, cash(yesterday))
		require.ErrorIs(t, err, ErrDividendDefaultChanged)
		_, err = acknowledgedReinvestedDividend(ctx, f.investmentService, ReinvestedDividendInput{
			OwnerUserID: f.ownerUserID, TransactionDate: yesterday, CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
			IncomeAccountID: &shown.ID, IncomeAccountFromDefault: true,
			QuantityValue: exact.New(1), QuantityScale: 0, AmountValue: 2500, AmountScale: 2, CashCommodityID: f.eurCommodityID,
		})
		require.ErrorIs(t, err, ErrDividendDefaultChanged)
		_, err = f.investmentService.SaveDividendDefault(ctx, DividendDefaultInput{DefaultID: saved.ID,
			OwnerUserID: f.ownerUserID, CommodityID: &f.stockCommodityID, IncomeAccountID: shown.ID, Status: "active", EffectiveFrom: "2026-01-01",
		})
		require.NoError(t, err)
	})

	t.Run("a default archived since it was shown is refused on a backdated entry", func(t *testing.T) {
		_, err := categories.DisableCategory(ctx, CategoryLifecycleInput{OwnerUserID: f.ownerUserID, OriginType: "browser_api", CategoryID: shown.ID, EffectiveFrom: day})
		require.NoError(t, err)
		_, err = f.investmentService.Dividend(ctx, cash(yesterday))
		require.ErrorIs(t, err, ErrDividendDefaultChanged)

		chosen := cash(yesterday)
		chosen.IncomeAccountFromDefault = false
		_, err = f.investmentService.Dividend(ctx, chosen)
		require.NoError(t, err, "the owner may still choose the category by hand for a day it was open")
	})
}
