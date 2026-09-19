package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-100, second window. An investment command decides an account's *role* —
// this one is a holding account the subledger owns, that one is where the cash
// settles — while it builds its plan, and only reads those accounts again while
// preparing the journal. The dependency guard was fed by the second read, so it
// compared the account to itself and passed: an account changed between
// planning and preparation kept a role it no longer had. The reproduction
// committed a buy against an account that was no longer a holding account,
// leaving a ten-share lot behind a journal balance an ordinary entry could then
// take to zero.
//
// The role check now records the version it read, and the journal preparation
// adds to that same collector without displacing it, so what reaches the write
// is the version the role was decided against.
//
// Each plan below is taken to the guard through the shared create funnel its
// real sink uses (CreateTransactionAndLot, CreateTransactionAndDisposeLots and
// CreateTransactionWithPostWrite all reach createTransactionWithAuditTx), so
// what is proved is that the plan carries the earlier version — the part that
// was missing — rather than that one particular sink checks it.

// renameAccount is a change the structural lock permits on an account that
// already has postings. It still creates a new account version, which is the
// whole of what the dependency guard keys on.
func renameAccount(t *testing.T, f *investmentsTestFixture, accountID int64, name string) {
	t.Helper()
	account, err := f.accountService.Account(context.Background(), accountID)
	require.NoError(t, err)
	input := UpdateAccountInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.update",
		AccountID: account.ID, Code: account.Code, Name: name,
		AccountClass: account.AccountClass, AccountKind: account.AccountKind,
		OpenedOn: account.OpenedOn, EffectiveFrom: "2026-01-01",
	}
	if account.DefaultCommodityID != nil {
		input.DefaultCommodityID = account.DefaultCommodityID
	}
	_, err = f.accountService.UpdateAccount(context.Background(), input)
	require.NoError(t, err)
}

// useServiceCreatedHoldingAccount replaces the fixture's raw-SQL holding
// account with one made through AccountService, which insists on the default
// commodity a holding account needs. Only then can the account be edited
// through the service at all, which is what a test about concurrent edits
// needs.
func useServiceCreatedHoldingAccount(t *testing.T, f *investmentsTestFixture) {
	t.Helper()
	holding, err := f.accountService.CreateAccount(context.Background(), CreateAccountInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create",
		Name: "Brokerage holding", AccountClass: "asset", AccountKind: "security_holding",
		DefaultCommodityID: &f.stockCommodityID, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	f.holdingAccountID = holding.ID
}

func changeHoldingAccountKind(t *testing.T, f *investmentsTestFixture, kind string) {
	t.Helper()
	holding, err := f.accountService.Account(context.Background(), f.holdingAccountID)
	require.NoError(t, err)
	allowsPostings := true
	_, err = f.accountService.UpdateAccount(context.Background(), UpdateAccountInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.update",
		AccountID: holding.ID, Code: holding.Code, Name: holding.Name, AccountClass: "asset",
		AccountKind: kind, AllowsPostings: &allowsPostings, DefaultCommodityID: &f.stockCommodityID,
		OpenedOn: holding.OpenedOn, EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
}

// TestBuyPlannedBeforeItsHoldingAccountChangedIsRefused is the reported
// reproduction, run end to end through the sink a buy actually uses.
func TestBuyPlannedBeforeItsHoldingAccountChangedIsRefused(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	input := sellInput(f, "2026-01-01", 10)

	plan, err := f.investmentService.buyPlan(ctx, input)
	require.NoError(t, err)

	changeHoldingAccountKind(t, f, "other_asset")

	// Planning the same buy now is refused; the plan already made must not get
	// a different answer.
	_, err = f.investmentService.buyPlan(ctx, input)
	require.Error(t, err)

	params, err := f.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	require.NoError(t, err)
	_, _, err = f.investmentService.repository.CreateTransactionAndLot(ctx, params, db.CreateInvestmentLotParams{
		BookID: BookID, AccountID: f.holdingAccountID, CommodityID: f.stockCommodityID,
		OpenedOn: input.TransactionDate, QuantityValue: input.QuantityValue, QuantityScale: input.QuantityScale,
		CostBasisValue: input.CashAmountValue, CostBasisScale: input.CashAmountScale,
		CostCommodityID: f.eurCommodityID, MetadataJSON: plan.MetadataJSON,
		CreatedAt: "2026-09-19T12:00:00Z", CreatedByUserID: f.ownerUserID,
		OriginType: "browser_api", Operation: "investment.lot.create",
		ChangeReason: "created lot from buy transaction", EventKind: "acquisition",
	})
	require.ErrorIs(t, err, db.ErrPostingAccountVersionStale)

	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Empty(t, lots, "no lot without the journal shares that account for it")
	var transactionCount int
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT count(*) FROM transactions`).Scan(&transactionCount))
	require.Zero(t, transactionCount)
}

// TestEveryInvestmentPlanBindsItsRoleReadsToTheWrite is the sweep the review
// asked for: sell, write-off, dividend and reinvestment plan their accounts the
// same way buy does, so each must refuse after one of the accounts it checked
// has moved.
//
// Which move is available differs by command, and that is itself part of the
// answer. Selling needs a position, and once an account has postings the
// reciprocal structural lock already refuses a kind change — so for those the
// reachable disturbance is an ordinary edit, and what it proves is that the
// plan is bound to its reads at all. A reinvestment can be a position's first
// acquisition, so there the role change itself is still reachable and is what
// is used.
func TestEveryInvestmentPlanBindsItsRoleReadsToTheWrite(t *testing.T) {
	for _, command := range []struct {
		name string
		// openPosition is whether the command needs shares to exist first.
		openPosition bool
		// serviceEditable swaps in a holding account created through the
		// account service, so that it can also be edited through it.
		serviceEditable bool
		// plan runs the command's planning phase.
		plan func(t *testing.T, f *investmentsTestFixture) investmentTransactionPlan
		// disturb changes one of the accounts that plan's roles were checked
		// against.
		disturb func(t *testing.T, f *investmentsTestFixture)
		// subledger is whether the plan posts to a holding account and so
		// needs the investment preparation.
		subledger bool
	}{
		{
			name:            "sell",
			serviceEditable: true,
			openPosition:    true,
			plan: func(t *testing.T, f *investmentsTestFixture) investmentTransactionPlan {
				plan, err := f.investmentService.sellPlan(context.Background(), sellInput(f, "2026-02-01", 1))
				require.NoError(t, err)
				return plan
			},
			disturb: func(t *testing.T, f *investmentsTestFixture) {
				renameAccount(t, f, f.holdingAccountID, "Renamed holding")
			},
			subledger: true,
		},
		{
			name:            "write-off",
			serviceEditable: true,
			openPosition:    true,
			plan: func(t *testing.T, f *investmentsTestFixture) investmentTransactionPlan {
				writeOff := InvestmentWriteOffInput{
					OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01", CommodityID: f.stockCommodityID,
					HoldingAccountID: f.holdingAccountID, QuantityValue: exact.New(1), Reason: "delisted",
				}
				plan, err := f.investmentService.sellPlan(context.Background(), writeOff.asTradeInput())
				require.NoError(t, err)
				return plan
			},
			disturb: func(t *testing.T, f *investmentsTestFixture) {
				renameAccount(t, f, f.holdingAccountID, "Renamed holding")
			},
			subledger: true,
		},
		{
			name: "dividend",
			plan: func(t *testing.T, f *investmentsTestFixture) investmentTransactionPlan {
				plan, err := f.investmentService.dividendPlan(context.Background(), DividendInput{
					OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
					CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
					IncomeAccountID: &f.incomeAccountID, AmountValue: 5000, AmountScale: 2,
				})
				require.NoError(t, err)
				return plan
			},
			// The income account is a role the dividend checked, so a change to
			// it disqualifies the plan as surely as a change to the cash account.
			disturb: func(t *testing.T, f *investmentsTestFixture) {
				renameAccount(t, f, f.incomeAccountID, "Renamed income")
			},
		},
		{
			name: "reinvested dividend",
			plan: func(t *testing.T, f *investmentsTestFixture) investmentTransactionPlan {
				plan, err := f.investmentService.reinvestedDividendPlan(context.Background(), ReinvestedDividendInput{
					OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-01",
					CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
					IncomeAccountID: &f.incomeAccountID, QuantityValue: exact.New(1), QuantityScale: 0,
					AmountValue: 1000, AmountScale: 2, CashCommodityID: f.eurCommodityID,
				})
				require.NoError(t, err)
				return plan
			},
			// No postings yet, so the holding account can still stop being one:
			// the role race itself, on the command that can reach it.
			disturb:   func(t *testing.T, f *investmentsTestFixture) { changeHoldingAccountKind(t, f, "other_asset") },
			subledger: true,
		},
	} {
		t.Run(command.name, func(t *testing.T) {
			f := newInvestmentsTestFixture(t)
			ctx := context.Background()
			if command.serviceEditable {
				useServiceCreatedHoldingAccount(t, f)
			}
			if command.openPosition {
				buyOn(t, f, "2026-01-01", 10, 10000)
			}

			plan := command.plan(t, f)
			require.NotNil(t, plan.AccountRuleDependencies, "a plan that decided a role must carry what it decided it against")
			require.NotEmpty(t, plan.AccountRuleDependencies.list())

			command.disturb(t, f)

			var params db.CreateTransactionParams
			var err error
			if command.subledger {
				params, err = f.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
			} else {
				params, err = f.transactionService.prepareCreateTransactionForWriteCarrying(ctx, plan.Create, plan.AccountRuleDependencies)
			}
			require.NoError(t, err, "preparation still succeeds: the account is postable, it just is not what it was")

			_, err = f.transactionService.repository.CreateTransaction(ctx, params)
			require.ErrorIs(t, err, db.ErrPostingAccountVersionStale)
		})
	}
}

// TestInvestmentPlanRecordsTheVersionItsRolesRead pins the mechanism rather
// than one of its consequences: the version carried is the planning-time one,
// not whatever the later journal preparation happened to read. Without that
// distinction the guard compares an account to itself.
func TestInvestmentPlanRecordsTheVersionItsRolesRead(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	plannedVersionID := accountLatestVersionID(t, f, f.holdingAccountID)

	plan, err := f.investmentService.buyPlan(ctx, sellInput(f, "2026-01-01", 10))
	require.NoError(t, err)
	require.Contains(t, plan.AccountRuleDependencies.list(), db.AccountRuleDependency{
		AccountID: f.holdingAccountID, LatestVersionID: plannedVersionID,
	})
	require.Contains(t, plan.AccountRuleDependencies.list(), db.AccountRuleDependency{
		AccountID: f.cashAccountID, LatestVersionID: accountLatestVersionID(t, f, f.cashAccountID),
	})

	changeHoldingAccountKind(t, f, "other_asset")
	require.NotEqual(t, plannedVersionID, accountLatestVersionID(t, f, f.holdingAccountID))

	params, err := f.transactionService.prepareInvestmentTransactionForWrite(ctx, plan.Create, plan.AccountRuleDependencies)
	require.NoError(t, err)
	require.Contains(t, params.AccountRuleDependencies, db.AccountRuleDependency{
		AccountID: f.holdingAccountID, LatestVersionID: plannedVersionID,
	}, "journal preparation must not overwrite the version the role was decided against")
}

// TestOrdinaryInvestmentCommandsStillCommit is the control: nothing above makes
// an undisturbed command fail.
func TestOrdinaryInvestmentCommandsStillCommit(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	buyOn(t, f, "2026-01-01", 10, 10000)

	_, err := f.investmentService.Sell(ctx, sellInput(f, "2026-02-01", 2))
	require.NoError(t, err)
	_, err = f.investmentService.Dividend(ctx, DividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-02",
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		IncomeAccountID: &f.incomeAccountID, AmountValue: 500, AmountScale: 2,
	})
	require.NoError(t, err)
	_, err = f.investmentService.ReinvestedDividend(ctx, ReinvestedDividendInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-03",
		CommodityID: f.stockCommodityID, HoldingAccountID: f.holdingAccountID,
		IncomeAccountID: &f.incomeAccountID, QuantityValue: exact.New(1), QuantityScale: 0,
		AmountValue: 1000, AmountScale: 2, CashCommodityID: f.eurCommodityID,
	})
	require.NoError(t, err)
	_, err = f.investmentService.WriteOff(ctx, InvestmentWriteOffInput{
		OwnerUserID: f.ownerUserID, TransactionDate: "2026-02-04", CommodityID: f.stockCommodityID,
		HoldingAccountID: f.holdingAccountID, QuantityValue: exact.New(1), Reason: "delisted",
	})
	require.NoError(t, err)
}
