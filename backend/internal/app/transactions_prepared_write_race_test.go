package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// T-94 and T-100. Both guards in this file answer the same question: a write is
// prepared against facts read outside its database transaction, so what happens
// when those facts change before it commits?
//
// The transaction half (T-94) had a hole one level above the repository's
// version check. PostTransaction reads the draft, turns it into a spec, and
// hands that spec to UpdateTransaction — which reads the draft a second time
// and supplies *that* read's version to the write. The check therefore always
// passed: the version it compared was never the one the spec came from. A draft
// edited between the two reads posted under the first read's dates while the
// candidates described the second read's, which is how a January entry could
// land behind an active January checkpoint without an override.
//
// The account half (T-100) had no check at all. Posting eligibility — does this
// account take postings, is it a holding account the investment subledger owns,
// does it fix a default commodity — is resolved before the write opens, and the
// write looked at nothing but the spec. An ordinary entry prepared against an
// unused other_asset account could commit after that account had become a
// security holding, leaving journal quantities with no lot behind them: exactly
// the state the subledger fence exists to prevent. The same window runs the
// other way, because an account restructure is admitted on the strength of
// "this account has no postings yet".

// TestDraftPromotionPreparedBeforeAnEditIsRefused runs the two halves of
// PostTransaction with an edit in between, and asserts the promotion is refused
// rather than posting its own older dates behind a live checkpoint.
func TestDraftPromotionPreparedBeforeAnEditIsRefused(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	postEUR(t, f, "2026-01-01", 10)
	checkpointID := reconcileCash(t, f, "2026-01-31", 10)

	// A producer draft inside the reconciled period. Drafts are outside the
	// ledger, so this is allowed; promotion is what has to be guarded.
	draftSpec := ordinaryEntry(f.cashAccountID, f.incomeAccountID, f.eurCommodityID, 5)
	draftSpec.Status = "draft"
	draftSpec.TransactionDate = "2026-01-02"
	draft, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "scheduled", Spec: draftSpec,
	})
	require.NoError(t, err)

	// PostTransaction's first phase: read the draft, build the promotion spec
	// from it, and remember the version both came from.
	source, err := f.transactionService.Transaction(ctx, draft.ID)
	require.NoError(t, err)
	promotion := transactionInputFromTransaction(source)
	promotion.Status = "posted"
	preparedVersionID := source.VersionID

	// Another request moves the draft out of the reconciled period.
	edited := transactionInputFromTransaction(source)
	edited.TransactionDate = "2026-02-01"
	edited.JournalEntries[0].EntryDate = "2026-02-01"
	_, err = f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: draft.ID, Spec: edited,
	})
	require.NoError(t, err)

	// PostTransaction's second phase, exactly as the command issues it.
	_, err = f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "transaction.post",
		TransactionID: draft.ID, Spec: promotion, AllowDraftPromotion: true,
		ExpectedVersionID: preparedVersionID,
	})
	require.ErrorIs(t, err, ErrTransactionVersionStale)

	after, err := f.transactionService.Transaction(ctx, draft.ID)
	require.NoError(t, err)
	require.Equal(t, "draft", after.Status, "the stale promotion did not post the draft")
	require.Equal(t, "2026-02-01", after.TransactionDate, "the intervening edit is what stands")
	require.Equal(t, []int64{checkpointID}, activeCheckpointIDs(t, f), "the January checkpoint is untouched")
}

// TestDraftPromotionWithoutItsSourceVersionIsRefused pins the contract that
// made the hole closable: a promotion spec always comes from a prior read, so a
// caller that cannot name that read has nothing to check the spec against.
func TestDraftPromotionWithoutItsSourceVersionIsRefused(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	draftSpec := ordinaryEntry(f.cashAccountID, f.incomeAccountID, f.eurCommodityID, 5)
	draftSpec.Status = "draft"
	draft, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "scheduled", Spec: draftSpec,
	})
	require.NoError(t, err)

	promotion := transactionInputFromTransaction(draft)
	promotion.Status = "posted"
	_, err = f.transactionService.UpdateTransaction(ctx, UpdateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "transaction.post",
		TransactionID: draft.ID, Spec: promotion, AllowDraftPromotion: true,
	})
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Contains(t, validation.Message, "version it was prepared from")

	after, err := f.transactionService.Transaction(ctx, draft.ID)
	require.NoError(t, err)
	require.Equal(t, "draft", after.Status)
}

// TestPostTransactionStillPromotesAnUntouchedDraft is the other half of the
// contract: the ordinary promotion, with nobody else editing, still works.
func TestPostTransactionStillPromotesAnUntouchedDraft(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	draftSpec := ordinaryEntry(f.cashAccountID, f.incomeAccountID, f.eurCommodityID, 5)
	draftSpec.Status = "draft"
	draft, err := f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "scheduled", Spec: draftSpec,
	})
	require.NoError(t, err)

	posted, err := f.transactionService.PostTransaction(ctx, PostTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", TransactionID: draft.ID,
		ChangeReason: "promoted by the user",
	})
	require.NoError(t, err)
	require.Equal(t, "posted", posted.Status)
}

// TestPreparedPostingIsRefusedAfterTheAccountBecomesAHolding is the T-100
// reproduction: an ordinary entry validated against an unused other_asset
// account must not commit once that account is a security holding, because the
// subledger fence would refuse the same entry if it were prepared now.
func TestPreparedPostingIsRefusedAfterTheAccountBecomesAHolding(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	ordinary, err := f.accountService.CreateAccount(ctx, CreateAccountInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create",
		Name: "Unposted asset", AccountClass: "asset", AccountKind: "other_asset",
		DefaultCommodityID: &f.stockCommodityID, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)

	entry := ordinaryEntry(ordinary.ID, f.incomeAccountID, f.stockCommodityID, 10)
	params, err := f.transactionService.prepareCreateTransactionForWrite(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: entry,
	})
	require.NoError(t, err)
	require.Contains(t, params.AccountRuleDependencies, db.AccountRuleDependency{
		AccountID: ordinary.ID, LatestVersionID: accountLatestVersionID(t, f, ordinary.ID),
	}, "the prepared write names the account version its checks were decided against")

	allowsPostings := true
	changed, err := f.accountService.UpdateAccount(ctx, UpdateAccountInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.update",
		AccountID: ordinary.ID, Code: ordinary.Code, Name: ordinary.Name, AccountClass: "asset",
		AccountKind: "security_holding", AllowsPostings: &allowsPostings,
		DefaultCommodityID: &f.stockCommodityID, OpenedOn: ordinary.OpenedOn, EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	require.Equal(t, "security_holding", changed.AccountKind)

	// Preparing the same entry now is refused by the fence; the prepared one
	// must not get a different answer.
	_, err = f.transactionService.prepareCreateTransactionForWrite(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: entry,
	})
	require.Error(t, err)

	_, err = f.transactionService.repository.CreateTransaction(ctx, params)
	require.ErrorIs(t, err, db.ErrPostingAccountVersionStale)

	lots, err := f.investmentService.ListLots(ctx, ordinary.ID, f.stockCommodityID)
	require.NoError(t, err)
	require.Empty(t, lots)
	var transactionCount int
	require.NoError(t, f.database.QueryRowContext(ctx, `SELECT count(*) FROM transactions`).Scan(&transactionCount))
	require.Zero(t, transactionCount, "nothing was written")
}

// TestPreparedInvestmentPostingIsRefusedAfterTheHoldingAccountChanges is the
// same window on the investment side: a trade prepared against a holding
// account must not commit once that account is no longer one, or its lots would
// belong to an account the subledger no longer owns.
func TestPreparedInvestmentPostingIsRefusedAfterTheHoldingAccountChanges(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	holding, err := f.accountService.Account(ctx, f.holdingAccountID)
	require.NoError(t, err)

	entry := ordinaryEntry(f.holdingAccountID, f.incomeAccountID, f.stockCommodityID, 10)
	params, err := f.transactionService.prepareInvestmentTransactionForWrite(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Spec: entry,
	}, nil)
	require.NoError(t, err)

	allowsPostings := true
	_, err = f.accountService.UpdateAccount(ctx, UpdateAccountInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.update",
		AccountID: f.holdingAccountID, Code: holding.Code, Name: holding.Name, AccountClass: "asset",
		AccountKind: "other_asset", AllowsPostings: &allowsPostings,
		DefaultCommodityID: &f.stockCommodityID, OpenedOn: holding.OpenedOn, EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)

	// The investment writes pair a journal with lots in one database
	// transaction of their own, so this has to go through that path rather
	// than the generic one.
	_, _, err = f.investmentService.repository.CreateTransactionAndLot(ctx, params, db.CreateInvestmentLotParams{
		BookID: BookID, AccountID: f.holdingAccountID, CommodityID: f.stockCommodityID,
		OpenedOn: "2026-01-01", QuantityValue: exact.New(10), QuantityScale: 0,
		CostBasisValue: 1000, CostBasisScale: 2, CostCommodityID: f.eurCommodityID,
		CreatedAt: "2026-09-13T12:00:00Z", CreatedByUserID: f.ownerUserID,
		OriginType: "browser_api", Operation: "investment.buy", ChangeReason: "bought",
		EventKind: "buy",
	})
	require.ErrorIs(t, err, db.ErrPostingAccountVersionStale)

	lots, err := f.investmentService.ListLots(ctx, f.holdingAccountID, f.stockCommodityID)
	require.NoError(t, err)
	require.Empty(t, lots, "neither the journal nor its lot was written")
}

// TestAccountStructureChangeIsRefusedWhenAPostingCommitsFirst is the reciprocal
// direction. The structural edit was admitted while the account was empty; by
// the time it commits the account has a posting, so the lock that was checked
// outside the write has to hold inside it too.
func TestAccountStructureChangeIsRefusedWhenAPostingCommitsFirst(t *testing.T) {
	f := newInvestmentsTestFixture(t)
	ctx := context.Background()
	ordinary, err := f.accountService.CreateAccount(ctx, CreateAccountInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api", Operation: "account.create",
		Name: "Unposted asset", AccountClass: "asset", AccountKind: "other_asset",
		DefaultCommodityID: &f.eurCommodityID, OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)

	allowsPostings := true
	spec, err := f.accountService.accountSpec(ctx, accountSpecInput{
		AccountID: ordinary.ID, Code: ordinary.Code, Name: ordinary.Name, AccountClass: "asset",
		AccountKind: "security_holding", AllowsPostings: &allowsPostings,
		DefaultCommodityID: &f.eurCommodityID,
	})
	require.NoError(t, err)

	// The posting the structural edit did not see.
	_, err = f.transactionService.CreateTransaction(ctx, CreateTransactionInput{
		OwnerUserID: f.ownerUserID, OriginType: "browser_api",
		Spec: ordinaryEntry(ordinary.ID, f.incomeAccountID, f.eurCommodityID, 10),
	})
	require.NoError(t, err)

	spec.OpenedOn = ordinary.OpenedOn
	_, err = f.accountService.repository.UpdateAccount(ctx, db.UpdateAccountParams{
		BookID: BookID, AccountID: ordinary.ID, ChangedByUserID: f.ownerUserID,
		OriginType: "browser_api", Operation: "account.update", Spec: spec,
		ChangeReason: "reclassified", RecordedAt: "2026-09-13T12:00:00Z", EffectiveFrom: "2026-01-01",
		RequireNoPostingsForStructure: true,
	})
	require.ErrorIs(t, err, db.ErrAccountStructureLocked)

	after, err := f.accountService.Account(ctx, ordinary.ID)
	require.NoError(t, err)
	require.Equal(t, "other_asset", after.AccountKind, "the account kept the structure its posting was made against")
}

func accountLatestVersionID(t *testing.T, f *investmentsTestFixture, accountID int64) int64 {
	t.Helper()
	var versionID int64
	require.NoError(t, f.database.QueryRowContext(context.Background(),
		`SELECT MAX(id) FROM account_versions WHERE account_id = ?`, accountID).Scan(&versionID))
	return versionID
}
