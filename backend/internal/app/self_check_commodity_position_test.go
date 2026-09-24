package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// A countable commodity — a share, a fund unit, a coin — cannot go below zero.
// Money can: an overdraft and a credit-card balance are real positions. Units
// are not, and the difference matters because nothing else in the app notices.
//
// The lot reconciliation check only compares holdings against lots, so it sees
// only the accounts the investment subledger manages. A crypto wallet has no
// lots at all (T-96 leaves crypto outside the subledger fence on purpose,
// because guarding it would remove the only way to record a crypto balance),
// and an ordinary asset account can be handed an instrument without one. Both
// can be driven negative by an ordinary balanced entry: a disposal entered
// before the acquisition that covers it, or one entered twice. The entry
// balances, the transaction balances, the book balances — and net worth reports
// minus four and a half bitcoin as an asset.

// selfCheckOver wires a self-check service to a fixture's own database, so a
// check can be run over a book built through the real services. The snapshot
// needs its own read-only connection: taking it from the writer's pool would
// hold a read transaction open on the same pool the run writes its results
// through, and the two would wait for each other.
func selfCheckOver(t *testing.T, database *sql.DB) *SelfCheckService {
	t.Helper()
	var path string
	require.NoError(t, database.QueryRowContext(context.Background(),
		`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&path))
	readOnly, err := db.OpenReadOnly(context.Background(), "file:"+path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, readOnly.Close()) })
	return NewSelfCheckService(db.NewSelfCheckRepository(database, readOnly))
}

func commodityPositionResult(t *testing.T, f *investmentsTestFixture) SelfCheckResult {
	t.Helper()
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	for _, result := range run.Results {
		if result.CheckID == CheckCommodityPositionSign {
			return result
		}
	}
	t.Fatalf("self-check produced no %s result", CheckCommodityPositionSign)
	return SelfCheckResult{}
}

// cryptoFixture gives the fixture a wallet and a coin, and returns a function
// that moves coins in or out of the wallet against cash, the way an ordinary
// balanced entry does — the only way to record crypto today.
func cryptoFixture(t *testing.T, f *investmentsTestFixture) (walletID int64, btcID int64, move func(date string, coins int64, cash int64)) {
	t.Helper()
	ctx := context.Background()
	walletID = seedTestAccountWithClass(t, f.database, "active", true, "asset", "crypto_wallet")
	btcID = seedTestCryptoCommodity(t, f.database, "BTC")
	trading, err := f.investmentService.repository.CommodityTradingAccountID(ctx, BookID)
	require.NoError(t, err)

	move = func(date string, coins int64, cash int64) {
		t.Helper()
		line := func(account, commodity, value int64, scale int) PostingInput {
			return PostingInput{AccountID: account, CommodityID: commodity, QuantityValue: exact.New(value), QuantityScale: scale}
		}
		_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
			OwnerUserID: f.ownerUserID, OriginType: "browser_api",
			Spec: TransactionInput{TransactionDate: date, JournalEntries: []JournalEntryInput{{Postings: []PostingInput{
				line(walletID, btcID, coins, 8), line(trading, btcID, -coins, 8),
				line(f.cashAccountID, f.eurCommodityID, cash, 2), line(trading, f.eurCommodityID, -cash, 2),
			}}}}})
		require.NoError(t, err)
	}
	return walletID, btcID, move
}

func TestSelfCheckFindsACryptoWalletDrivenNegative(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	wallet, _, move := cryptoFixture(t, f)

	move("2026-01-01", 50000000, -10000)   // buy 0.5 BTC for 100.00
	move("2026-01-02", -500000000, 100000) // sell 5 BTC, four and a half of which do not exist

	result := commodityPositionResult(t, f)
	require.Equal(t, SelfCheckFailed, result.Status)
	require.Equal(t, int64(1), result.FindingCount)
	require.Equal(t, []int64{wallet}, result.Sample)
	require.Contains(t, result.Summary, "negative quantity")

	// The state is invisible to every other check: that is the point of adding
	// this one. Each entry balances and so does the book.
	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	for _, other := range run.Results {
		if other.CheckID == CheckCommodityPositionSign {
			continue
		}
		require.NotEqual(t, SelfCheckFailed, other.Status, "%s should not see this", other.CheckID)
	}
}

func TestSelfCheckFindsAnInstrumentDrivenNegativeOnAnOrdinaryAccount(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	// Not a holding account, so the subledger fence does not apply and the
	// position has no lots to reconcile against.
	other := seedTestAccountWithClass(t, f.database, "active", true, "asset", "other_asset")
	_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		Spec: ordinaryEntry(other, f.cashAccountID, f.stockCommodityID, -10)})
	require.NoError(t, err)

	result := commodityPositionResult(t, f)
	require.Equal(t, SelfCheckFailed, result.Status)
	require.Equal(t, []int64{other}, result.Sample)
}

// TestSelfCheckAcceptsEveryOrdinaryCommodityMovement is the control, and the
// reason the clearing account is excluded: it is the counterparty of every
// commodity movement and is negative by construction. A book that buys, sells,
// reinvests, writes off and round-trips crypto must produce no finding.
func TestSelfCheckAcceptsEveryOrdinaryCommodityMovement(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	_, _, move := cryptoFixture(t, f)

	buyOn(t, f, "2026-01-01", 10, 10000)
	move("2026-01-02", 250000000, -50000) // 2.5 BTC in
	sale := sellInput(f, "2026-02-01", 4)
	sale.CashAmountValue = 5000
	_, err := f.investmentService.Sell(ctx, sale)
	require.NoError(t, err)
	move("2026-02-02", -250000000, 60000) // and all 2.5 BTC back out
	_, err = f.investmentService.ReinvestedDividend(ctx, ReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-03-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, IncomeAccountID: &f.incomeAccountID,
		QuantityValue: exact.New(1), QuantityScale: 0, AmountValue: 1200, AmountScale: 2,
		CashCommodityID: f.eurCommodityID})
	require.NoError(t, err)
	_, err = f.investmentService.WriteOff(ctx, InvestmentWriteOffInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-04-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, QuantityValue: exact.New(7), Reason: "delisted"})
	require.NoError(t, err)

	run, err := selfCheckOver(t, f.database).RunSelfCheck(context.Background(), "manual")
	require.NoError(t, err)
	require.Equal(t, SelfCheckPassed, run.Status, "a healthy book must produce no finding at all")
	require.Zero(t, run.FailedCheckCount)
}

// A holding account the subledger manages cannot reach this state through the
// app — the fence keeps ordinary entries out and the subledger refuses to
// oversell — so the new check is about the accounts the fence does not cover.
func TestSubledgerHoldingAccountCannotBeDrivenNegative(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 1, 1000)

	_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		Spec: ordinaryEntry(f.holdingAccountID, f.cashAccountID, f.stockCommodityID, -5)})
	var validation ValidationError
	require.ErrorAs(t, err, &validation, "the subledger fence refuses ordinary entries here")

	oversell := sellInput(f, "2026-02-01", 5)
	_, err = f.investmentService.Sell(ctx, oversell)
	require.ErrorIs(t, err, ErrInvestmentLotsInsufficient)

	require.Equal(t, SelfCheckPassed, commodityPositionResult(t, f).Status)
}

func seedTestCryptoCommodity(t *testing.T, database *sql.DB, code string) int64 {
	t.Helper()
	ctx := context.Background()
	result, err := database.ExecContext(ctx, `
		INSERT INTO commodities (book_id, code, kind, is_builtin, created_at, created_by_user_id)
		VALUES (?, ?, 'crypto', 0, '2026-01-01T00:00:00Z', 1)
	`, BookID, code)
	require.NoError(t, err)
	commodityID, err := result.LastInsertId()
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, `
		INSERT INTO commodity_versions (
			commodity_id, version_seq, effective_from, recorded_at, changed_by_user_id,
			change_reason, status, symbol, display_symbol, name, standard_scale, max_quantity_scale
		) VALUES (?, 1, '2026-01-01', '2026-01-01T00:00:00Z', 1, 'test fixture', 'active', ?, ?, ?, 8, 18)
	`, commodityID, code, code, code)
	require.NoError(t, err)
	return commodityID
}
