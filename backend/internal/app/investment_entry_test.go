package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvestmentEntryAccountsShareDatedCommandRoles(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	liability := seedTestAccountWithClass(t, f.database, "active", true, "liability", "credit_card")
	closed := seedTestAccount(t, f.database, "closed", true)
	noPosting := seedTestAccount(t, f.database, "active", false)
	for _, id := range []int64{f.cashAccountID, liability, closed, noPosting} {
		appendEntryTestCurrency(t, f, id)
	}
	for _, date := range []string{"2026-01-01", "2026-01-02", "2026-01-03"} {
		t.Run(date, func(t *testing.T) {
			rows, err := f.investmentService.EntryAccounts(ctx, date)
			require.NoError(t, err)
			require.NotEmpty(t, rows)
			found := map[int64]InvestmentEntryAccount{}
			for _, row := range rows {
				assert.False(t, row.IsSystem)
				found[row.ID] = row
				_, holdingErr := f.investmentService.accountInRole(ctx, row.ID, date, holdingRole, nil)
				assert.Equal(t, holdingErr == nil, row.InvestmentHolding, "holding %d", row.ID)
				_, settlementErr := f.investmentService.accountInRole(ctx, row.ID, date, settlementRole, nil)
				expected := settlementErr == nil && row.DefaultCommodityID != nil
				assert.Equal(t, expected, row.InvestmentSettlement, "settlement %d", row.ID)
			}
			assert.True(t, found[f.holdingAccountID].InvestmentHolding)
			assert.False(t, found[f.holdingAccountID].InvestmentSettlement)
			assert.True(t, found[liability].InvestmentSettlement)
			assert.False(t, found[noPosting].InvestmentSettlement)
			assert.Equal(t, date <= "2026-01-02", found[closed].InvestmentSettlement)
		})
	}
}

func TestInvestmentEntryAccountsUseVersionedHierarchyAndCurrency(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	appendEntryTestCurrency(t, f, f.cashAccountID)
	// A later version must not leak its status, label or hierarchy into history.
	_, err := f.database.ExecContext(ctx, `INSERT INTO account_versions
 (account_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,opened_on,closed_on,code,name,account_class,account_kind,allows_postings,default_commodity_id,parent_account_id)
 VALUES (?,3,'2026-02-01','2026-02-01T00:00:00Z',1,'test','archived','2026-01-01','2026-02-01','CASH','Later name','asset','checking',1,?,?)`, f.cashAccountID, f.eurCommodityID, f.holdingAccountID)
	require.NoError(t, err)
	rows, err := f.investmentService.EntryAccounts(ctx, "2026-01-15")
	require.NoError(t, err)
	for _, row := range rows {
		if row.ID == f.cashAccountID {
			assert.True(t, row.InvestmentSettlement)
			assert.Equal(t, "Test Cash", row.Name)
			assert.Nil(t, row.ParentAccountID)
		}
	}
	rows, err = f.investmentService.EntryAccounts(ctx, "2026-02-01")
	require.NoError(t, err)
	for _, row := range rows {
		if row.ID == f.cashAccountID {
			assert.False(t, row.InvestmentSettlement)
			assert.Equal(t, "Later name", row.Name)
			require.NotNil(t, row.ParentAccountID)
			assert.Equal(t, f.holdingAccountID, *row.ParentAccountID)
		}
	}
	rows, err = f.investmentService.EntryAccounts(ctx, "2025-12-31")
	require.NoError(t, err)
	assert.Empty(t, rows)
	_, err = f.database.ExecContext(ctx, `INSERT INTO commodity_versions (commodity_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,symbol,display_symbol,name,standard_scale,max_quantity_scale) VALUES (?,2,'2026-01-10','2026-01-10T00:00:00Z',1,'test','archived','EUR','€','Euro',2,6)`, f.eurCommodityID)
	require.NoError(t, err)
	rows, err = f.investmentService.EntryAccounts(ctx, "2026-01-15")
	require.NoError(t, err)
	for _, row := range rows {
		if row.ID == f.cashAccountID {
			assert.False(t, row.InvestmentSettlement)
		}
	}
	for _, date := range []string{"", "2026-02-30", "not-a-date"} {
		_, err := f.investmentService.EntryAccounts(ctx, date)
		require.Error(t, err)
	}
}

func appendEntryTestCurrency(t *testing.T, f *investmentsTestFixture, id int64) {
	t.Helper()
	_, err := f.database.ExecContext(context.Background(), `INSERT INTO account_versions
 (account_id,version_seq,effective_from,recorded_at,changed_by_user_id,change_reason,status,opened_on,closed_on,code,name,account_class,account_kind,allows_postings,default_commodity_id)
 SELECT account_id,version_seq+1,effective_from,recorded_at,changed_by_user_id,change_reason,status,opened_on,closed_on,code,name,account_class,account_kind,allows_postings,?
 FROM account_versions WHERE account_id = ? ORDER BY version_seq DESC LIMIT 1`, f.eurCommodityID, id)
	require.NoError(t, err)
}
