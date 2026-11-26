package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHomeLayoutScopesUseCanonicalAmountsAndWidgetPeriods(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	ctx := context.Background()
	second := f.account(t, "Savings", "asset", "savings")
	f.move(t, "2026-09-15", f.cashAccountID, f.eurCommodityID, 10000)
	f.move(t, "2026-09-20", second, f.eurCommodityID, 4000)
	f.spend(t, "2026-10-05", f.cashAccountID, f.groceries, f.eurCommodityID, 2500)
	f.spend(t, "2026-10-09", second, f.groceries, f.eurCommodityID, 1000)

	layout := DefaultHomeLayout()
	layout.Headline = HomeHeadlineAccountSet
	layout.AccountID = f.cashAccountID
	layout.AccountSet = &HomeLayoutAccountSet{Name: "At the bank", AccountIDs: []int64{f.cashAccountID, second}}
	layout.WidgetOptions["spending"] = HomeWidgetOptions{AccountIDs: []int64{second}}
	layout.WidgetOptions["cashflow"] = HomeWidgetOptions{Period: "last_month"}
	state, err := f.home.SaveLayout(ctx, 1, 0, &layout)
	require.NoError(t, err)
	home := f.read(t)
	assert.Equal(t, state, home.Briefing.Layout)
	account := homeHeadline(t, home.Briefing, HomeHeadlineAccount)
	requireHomeCents(t, 7500, account.Value, "selected account")
	requireHomeCents(t, -2500, account.Change, "selected account's movement")
	group := homeHeadline(t, home.Briefing, HomeHeadlineAccountSet)
	requireHomeCents(t, 10500, group.Value, "named account set")
	requireHomeCents(t, -3500, group.Change, "named account set's movement")
	requireHomeCents(t, 1000, home.Spending.Summary.Total, "selected spending scope")
	assert.Equal(t, []int64{second}, home.Spending.Summary.ResolvedAccountIDs)
	require.NotNil(t, home.Cashflow.Summary)
	assert.Equal(t, "2026-09-01", home.Cashflow.Summary.StartDate)
	assert.Equal(t, "2026-09-30", home.Cashflow.Summary.EndDate)
}

func TestHomeLayoutOldWidgetAndRouteAndArchivedAccountFallBack(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	ctx := context.Background()
	archived := seedTestAccountWithClass(t, f.database, "archived", false, "asset", "savings")
	layout := DefaultHomeLayout()
	layout.Headline = HomeHeadlineAccountSet
	layout.AccountID = archived
	layout.AccountSet = &HomeLayoutAccountSet{Name: "Old set", AccountIDs: []int64{f.cashAccountID, archived}}
	layout.Shortcuts = []HomeLayoutShortcut{{ID: "retired_page"}, {ID: "account", AccountID: archived}}
	layout.Widgets = []string{"retired_widget"}
	layout.WidgetOptions["spending"] = HomeWidgetOptions{AccountIDs: []int64{archived}}
	layout.WidgetOptions["retired_widget"] = HomeWidgetOptions{Size: "wide"}
	document, err := json.Marshal(layout)
	require.NoError(t, err)
	_, err = f.transactionService.repository.SaveHomeLayout(ctx, BookID, 1, 0, string(document))
	require.NoError(t, err)

	home := f.read(t)
	effective := home.Briefing.Layout
	assert.Equal(t, int64(1), effective.Revision)
	assert.Equal(t, HomeHeadlineAuto, effective.Layout.Headline)
	assert.Zero(t, effective.Layout.AccountID)
	assert.Nil(t, effective.Layout.AccountSet)
	assert.Equal(t, effective.Defaults.Shortcuts, effective.Layout.Shortcuts)
	assert.Equal(t, effective.Defaults.Widgets, effective.Layout.Widgets)
	assert.NotContains(t, effective.Layout.WidgetOptions, "retired_widget")
	assert.Empty(t, effective.Layout.WidgetOptions["spending"].AccountIDs)
	require.Len(t, home.Briefing.Headlines, 2)
}

func TestHomeLayoutResetKeepsTheOwnerChosenDefaultRegister(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	ctx := context.Background()
	layout := DefaultHomeLayout()
	layout.Shortcuts = []HomeLayoutShortcut{{ID: "add"}, {ID: "account", AccountID: f.cashAccountID, Label: "Main"}}
	layout.Widgets = []string{}
	saved, err := f.home.SaveLayout(ctx, 1, 0, &layout)
	require.NoError(t, err)
	assert.Empty(t, saved.Layout.Widgets)
	assert.Equal(t, f.cashAccountID, saved.Defaults.ShortcutAccountID)

	saved.Layout.Shortcuts = saved.Layout.Shortcuts[:1]
	removed, err := f.home.SaveLayout(ctx, 1, saved.Revision, &saved.Layout)
	require.NoError(t, err)
	require.Len(t, removed.Layout.Shortcuts, 1)

	reset, err := f.home.SaveLayout(ctx, 1, removed.Revision, nil)
	require.NoError(t, err)
	assert.Equal(t, saved.Defaults, reset.Layout)
	assert.Contains(t, reset.Layout.Shortcuts, HomeLayoutShortcut{ID: "account", AccountID: f.cashAccountID})
}
