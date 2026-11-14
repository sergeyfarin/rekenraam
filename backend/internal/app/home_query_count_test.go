package app

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	sqlite "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countSelectCompilations counts how often SQLite compiles a SELECT on the
// pool's single connection. The authorizer runs while a statement is prepared,
// never while it executes, so the count measures query shape, not row count.
func countSelectCompilations(t *testing.T, database *sql.DB) *atomic.Int64 {
	t.Helper()
	ctx := context.Background()
	var compiled atomic.Int64
	conn, err := database.Conn(ctx)
	require.NoError(t, err)
	require.NoError(t, conn.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterAuthorizer(func(operation int, _, _, _ string) int {
			if operation == sqlite.SQLITE_SELECT {
				compiled.Add(1)
			}
			return sqlite.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, conn.Close())
	t.Cleanup(func() {
		conn, err := database.Conn(ctx)
		require.NoError(t, err)
		require.NoError(t, conn.Raw(func(raw any) error {
			raw.(*sqlite.SQLiteConn).RegisterAuthorizer(nil)
			return nil
		}))
		require.NoError(t, conn.Close())
	})
	return &compiled
}

// A composed Home read must not compile a statement per account or
// transaction. The fixture doubles each dimension; a per-row read would add
// four compilations, while a batched read keeps the same query shape.
func TestHomeReadDoesNotCompileQueriesPerRow(t *testing.T) {
	t.Parallel()
	f := newHomeWidgetFixture(t)
	ctx := context.Background()
	counter := countSelectCompilations(t, f.database)

	_, err := f.home.Home(ctx, 1)
	require.NoError(t, err)

	addAccounts := func(count int) {
		for index := range count {
			accountID := f.account(t, fmt.Sprintf("Scale account %d", index), "asset", "checking")
			f.spend(t, "2026-10-05", accountID, f.groceries, f.eurCommodityID, 100)
		}
	}
	addTransactions := func(count int) {
		for range count {
			f.spend(t, "2026-10-06", f.cashAccountID, f.groceries, f.eurCommodityID, 100)
		}
	}
	readCount := func() int64 {
		counter.Store(0)
		_, err := f.home.Home(ctx, 1)
		require.NoError(t, err)
		return counter.Load()
	}

	addAccounts(4)
	fourAccounts := readCount()
	require.Positive(t, fourAccounts)

	addAccounts(4)
	eightAccounts := readCount()
	t.Logf("Home SELECT compilations: four extra accounts=%d, eight extra accounts=%d", fourAccounts, eightAccounts)
	assert.LessOrEqual(t, eightAccounts, fourAccounts+1, "Home must not compile a query per account")

	addTransactions(4)
	fourTransactions := readCount()

	addTransactions(4)
	eightTransactions := readCount()
	t.Logf("Home SELECT compilations: four extra transactions=%d, eight extra transactions=%d", fourTransactions, eightTransactions)
	assert.LessOrEqual(t, eightTransactions, fourTransactions+1, "Home must not compile a query per transaction")
}
