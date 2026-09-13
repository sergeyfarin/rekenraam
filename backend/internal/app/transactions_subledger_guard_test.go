package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// T-96. Generic posting validation checked account eligibility, commodity and
// precision, but had no concept of an account whose balance belongs to the
// investment subledger. A perfectly balanced ordinary entry could therefore put
// shares into a holding account with no lot behind them — the register shows a
// position the positions and gains views cannot account for, and there is no
// record of what was acquired or when, so nothing can reconstruct it after the
// fact. A diagnostic reporting the mismatch is not a fix; the write has to be
// refused.
//
// The investment commands write journal and lots in one database transaction
// and remain the only way into these accounts, so these tests have to prove
// both halves: the generic paths are shut, and the investment paths still work.

func ordinaryEntry(accountID, otherAccountID, commodityID int64, quantity int64) TransactionInput {
	return TransactionInput{
		TransactionDate: "2026-01-01",
		JournalEntries: []JournalEntryInput{{
			Postings: []PostingInput{
				{AccountID: accountID, CommodityID: commodityID, QuantityValue: exact.New(quantity)},
				{AccountID: otherAccountID, CommodityID: commodityID, QuantityValue: exact.New(-quantity)},
			},
		}},
	}
}

func TestCreateTransactionRejectsPostingsToSubledgerManagedAccounts(t *testing.T) {
	for _, kind := range []string{"security_holding", "fund_holding"} {
		t.Run(kind, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			holdingAccountID := seedTestAccountWithClass(t, f.database, "active", true, "asset", kind)

			_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
				OwnerUserID: f.ownerUserID, OriginType: "browser_api",
				Spec: ordinaryEntry(holdingAccountID, f.cashAccountID, f.stockCommodityID, 10),
			})

			var validation ValidationError
			require.ErrorAs(t, err, &validation)
			require.Contains(t, validation.Message, "investment subledger")

			// And nothing was written — not the transaction, not a lot.
			lots, err := f.investmentService.ListLots(ctx, holdingAccountID, f.stockCommodityID)
			require.NoError(t, err)
			require.Empty(t, lots)
			var transactionCount int
			require.NoError(t, f.database.QueryRowContext(ctx, `SELECT count(*) FROM transactions`).Scan(&transactionCount))
			require.Zero(t, transactionCount)
		})
	}
}

// The guard is about the account, not the sign or the direction: taking shares
// out generically is as untracked as putting them in, and it would consume a
// position the subledger still thinks is open.
func TestCreateTransactionRejectsSubledgerPostingsInEitherDirection(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	holdingAccountID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")

	for _, quantity := range []int64{10, -10} {
		_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
			OwnerUserID: f.ownerUserID, OriginType: "browser_api",
			Spec: ordinaryEntry(holdingAccountID, f.cashAccountID, f.stockCommodityID, quantity),
		})
		require.Error(t, err)
	}
}

// An edit is the same write by another name. The existing investment lifecycle
// fence rejects mutations of a transaction that already has lot links, which
// cannot help here: this transaction has none, and the edit is what introduces
// the holding posting.
func TestUpdateTransactionRejectsIntroducingASubledgerManagedPosting(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	holdingAccountID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")

	created, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		Spec: ordinaryEntry(f.cashAccountID, f.incomeAccountID, f.eurCommodityID, 10),
	})
	require.NoError(t, err)

	_, err = f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: created.ID,
		ChangeReason: "moved to the holding account",
		Spec:         ordinaryEntry(holdingAccountID, f.cashAccountID, f.stockCommodityID, 10),
	})
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Contains(t, validation.Message, "investment subledger")
}

// The preview has to agree with the write. A reconciliation-impact preview that
// accepts a spec the commit refuses sends the user through a confirmation step
// for a write that was never going to land.
func TestReconciliationImpactForCreateRejectsSubledgerManagedPostings(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	holdingAccountID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "security_holding")

	_, err := f.transactionService.ReconciliationImpactForCreate(ctx, CreateReconciliationImpactInput{
		OwnerUserID: f.ownerUserID,
		Spec:        ordinaryEntry(holdingAccountID, f.cashAccountID, f.stockCommodityID, 10),
	})
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Contains(t, validation.Message, "investment subledger")
}

// The other half of the guard. Buy and Sell write to a holding account on every
// call, so an exemption that failed to reach them would break investing
// outright — and an exemption wired to the wrong place would be caught here
// rather than in production.
func TestInvestmentCommandsStillWriteToSubledgerManagedAccounts(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	trade := InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-01-01", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(10), CashAmountValue: 10000, CashAmountScale: 2,
		CashCommodityID: f.eurCommodityID,
	}
	bought, err := f.investmentService.Buy(ctx, trade)
	require.NoError(t, err, "the subledger's own command must still reach the holding account")
	require.NotNil(t, bought.LotID)

	// The preview path carries the same exemption as the write.
	trade.TransactionDate = "2026-02-01"
	_, err = f.investmentService.TradeReconciliationImpact(ctx, InvestmentImpactSell, trade)
	require.NoError(t, err, "the investment impact preview must validate the spec the commit will write")

	sold, err := f.investmentService.Sell(ctx, trade)
	require.NoError(t, err)
	require.Len(t, sold.Allocations, 1)
}

// The generic guard must not reach past the accounts the subledger actually
// manages. A brokerage cash account is an ordinary money account that salary,
// fees and transfers post to all the time.
func TestCreateTransactionAllowsOrdinaryInvestmentAdjacentAccounts(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()

	for _, kind := range []string{"brokerage_cash", "brokerage"} {
		accountID := seedTestAccountWithClass(t, f.database, "active", true, "asset", kind)
		_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
			OwnerUserID: f.ownerUserID, OriginType: "browser_api",
			Spec: ordinaryEntry(accountID, f.incomeAccountID, f.eurCommodityID, 10),
		})
		require.NoError(t, err, "%s is an ordinary posting account", kind)
	}
}

// Documented gap, pinned so it is a decision rather than an oversight: there is
// no investment command that writes a crypto position, so guarding crypto
// wallets would remove the only way to record one. See docs/backlog.md (T-96).
func TestCreateTransactionStillAllowsCryptoWalletPostings(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	walletID := seedTestAccountWithClass(t, f.database, "active", true, "asset", "crypto_wallet")

	_, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		Spec: ordinaryEntry(walletID, f.cashAccountID, f.stockCommodityID, 10),
	})
	require.NoError(t, err)
}
