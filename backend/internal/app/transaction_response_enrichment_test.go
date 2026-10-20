package app

import (
	"context"
	"database/sql"
	"rekenraam/backend/internal/exact"
	"strconv"
	"sync/atomic"
	"testing"

	sqlite "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"rekenraam/backend/internal/db"
)

func TestTransactionResponseEnrichmentBatchesCompoundAndRepeatedJournals(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	first := buyOn(t, f, "2026-02-01", 2, 4000)
	second := buyOn(t, f, "2026-02-02", 3, 6000)
	records := []db.TransactionRecord{}
	for _, id := range []int64{first.Transaction.ID, second.Transaction.ID} {
		record, err := f.transactionService.repository.TransactionByID(ctx, BookID, id)
		require.NoError(t, err)
		records = append(records, record)
	}
	var accountReads, commodityReads atomic.Int64
	conn, err := f.database.Conn(ctx)
	require.NoError(t, err)
	require.NoError(t, conn.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterAuthorizer(func(operation int, table, column, source string) int {
			if operation == sqlite.SQLITE_READ && table == "account_kinds" && column == "base_kind" {
				accountReads.Add(1)
			}
			if operation == sqlite.SQLITE_READ && table == "commodities" && column == "code" {
				commodityReads.Add(1)
			}
			return sqlite.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, conn.Close())
	t.Cleanup(func() {
		conn, err := f.database.Conn(ctx)
		require.NoError(t, err)
		require.NoError(t, conn.Raw(func(raw any) error { raw.(*sqlite.SQLiteConn).RegisterAuthorizer(nil); return nil }))
		require.NoError(t, conn.Close())
	})
	for _, size := range []int{2, 100} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			accountReads.Store(0)
			commodityReads.Store(0)
			batch := make([]db.TransactionRecord, size)
			for i := range batch {
				batch[i] = records[i%2]
			}
			result, err := f.transactionService.enrichRecords(ctx, batch...)
			require.NoError(t, err)
			require.Len(t, result, size)
			require.Equal(t, int64(1), accountReads.Load(), "one account batch independent of journal/posting count")
			require.Equal(t, int64(1), commodityReads.Load(), "one commodity batch independent of journal/posting count")
			for _, transaction := range result {
				for _, entry := range transaction.JournalEntries {
					for _, posting := range entry.Postings {
						require.Contains(t, []string{"asset", "equity"}, posting.AccountClass)
						require.Contains(t, []string{"TEST", "EUR"}, posting.CommodityCode)
					}
				}
			}
		})
	}
	accountReads.Store(0)
	commodityReads.Store(0)
	journals, err := f.transactionService.enrichRecords(ctx, db.TransactionRecord{})
	require.NoError(t, err)
	require.Nil(t, optionalTransaction(journals[0]), "journal-free remains nil")
	require.Zero(t, accountReads.Load())
	require.Zero(t, commodityReads.Load())
}

func TestInvestmentImportWriterDoesNotDependOnResponseEnrichment(t *testing.T) {
	t.Parallel()
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	_, err := f.database.Exec("CREATE TABLE test_import_receipt (transaction_id INTEGER NOT NULL)")
	require.NoError(t, err)
	var committed atomic.Bool
	conn, err := f.database.Conn(ctx)
	require.NoError(t, err)
	require.NoError(t, conn.Raw(func(raw any) error {
		raw.(*sqlite.SQLiteConn).RegisterAuthorizer(func(operation int, target, column, source string) int {
			if operation == sqlite.SQLITE_TRANSACTION && target == "COMMIT" {
				committed.Store(true)
			}
			if committed.Load() && operation == sqlite.SQLITE_READ && target == "account_kinds" && column == "base_kind" {
				return sqlite.SQLITE_DENY
			}
			return sqlite.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, conn.Close())
	reset := func() {
		conn, err := f.database.Conn(ctx)
		require.NoError(t, err)
		require.NoError(t, conn.Raw(func(raw any) error { raw.(*sqlite.SQLiteConn).RegisterAuthorizer(nil); return nil }))
		require.NoError(t, conn.Close())
	}
	t.Cleanup(reset)
	input := InvestmentTradeInput{OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", HoldingAccountID: f.holdingAccountID, CommodityID: f.stockCommodityID, CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, QuantityValue: exact.New(2), CashAmountValue: 4000, CashAmountScale: 2}
	result, err := f.investmentService.buyWithPostWrite(ctx, input, func(tx *sql.Tx, id int64) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO test_import_receipt VALUES (?)", id)
		return err
	})
	require.NoError(t, err, "internal import identity consumer must not fail on optional response metadata")
	reset()
	var id int64
	require.NoError(t, f.database.QueryRow("SELECT transaction_id FROM test_import_receipt").Scan(&id))
	require.Equal(t, result.Transaction.ID, id)
	require.Positive(t, id)
	var count int
	require.NoError(t, f.database.QueryRow("SELECT COUNT(*) FROM transactions").Scan(&count))
	require.Equal(t, 1, count)
}
