package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// shortImportFixture holds a natively recorded short position in a holding
// the Trading 212 connection maps, and builds unsupported provider fills
// that evidence it (#245). The fills are synthetic: they exercise the
// owner-driven link, not undocumented provider short semantics.
type shortImportFixture struct {
	*investTestFixture
	connectionID int64
	commodityID  int64
	holdingID    int64
}

func newShortImportFixture(t *testing.T) shortImportFixture {
	t.Helper()
	f := newInvestTestFixture(t)
	ctx := context.Background()
	conn := f.createConnection(t, &f.cashAccountID)
	instrumentID, commodityID, _, err := f.investmentSvc.ResolveOrCreateInstrumentForImport(ctx, f.ownerUserID, 0, "", "US0378331005", "AAPL_US_EQ")
	require.NoError(t, err)
	holding, err := f.investmentSvc.CreateHoldingAccount(ctx, HoldingAccountInput{
		OwnerUserID: f.ownerUserID, InstrumentID: instrumentID, Name: "AAPL short",
		OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01",
	})
	require.NoError(t, err)
	require.NoError(t, f.connService.LinkHolding(ctx, conn.ID, commodityID, holding.ID, "2026-01-01T00:00:00Z"))
	return shortImportFixture{investTestFixture: f, connectionID: conn.ID, commodityID: commodityID, holdingID: holding.ID}
}

func (f shortImportFixture) trade(date string, quantity, cents int64) InvestmentTradeInput {
	return InvestmentTradeInput{
		OwnerUserID: f.ownerUserID, TransactionDate: date, CommodityID: f.commodityID,
		HoldingAccountID: f.holdingID, CashAccountID: f.cashAccountID,
		QuantityValue: exact.New(quantity), CashAmountValue: cents, CashAmountScale: 2,
		CashCommodityID: f.eurCommodityID,
	}
}

// shortSale records 10 borrowed and sold for 1000.00 on 2026-05-01.
func (f shortImportFixture) shortSale(t *testing.T) (operationID, transactionID int64) {
	t.Helper()
	result, err := f.investmentSvc.ShortSale(context.Background(), f.trade("2026-05-01", 10, 100000))
	require.NoError(t, err)
	return f.operationOf(t, result.Transaction.ID), result.Transaction.ID
}

func (f shortImportFixture) operationOf(t *testing.T, transactionID int64) int64 {
	t.Helper()
	var operationID int64
	require.NoError(t, f.database.QueryRow(`SELECT link.operation_id FROM investment_operation_journal_links link
		JOIN transaction_versions v ON v.id = link.transaction_version_id
		WHERE v.transaction_id = ? AND link.role = 'primary'`, transactionID).Scan(&operationID))
	return operationID
}

// shortFill is an unsupported provider fill whose facts agree with shortSale.
func shortFill(id string) trading212OrderFill {
	return trading212OrderFill{
		FillType: "UNDOCUMENTED_SHORT", FillID: id, OrderID: "order-" + id, Side: "SELL",
		Ticker: "AAPL_US_EQ", ISIN: "US0378331005", Quantity: "10", Price: "100",
		Currency: "EUR", FilledAt: "2026-05-01T10:00:00Z", NetValue: "1000", NetValueCurrency: "EUR",
	}
}

type shortLinkCounts struct{ transactions, lots, decisions, audits, identities int }

func (f shortImportFixture) counts(t *testing.T) shortLinkCounts {
	t.Helper()
	var c shortLinkCounts
	for query, target := range map[string]*int{
		`SELECT COUNT(*) FROM transactions`:                   &c.transactions,
		`SELECT COUNT(*) FROM investment_lots`:                &c.lots,
		`SELECT COUNT(*) FROM investment_disposal_decisions`:  &c.decisions,
		`SELECT COUNT(*) FROM audit_events`:                   &c.audits,
		`SELECT COUNT(*) FROM import_commit_identity_effects`: &c.identities,
	} {
		require.NoError(t, f.database.QueryRow(query).Scan(target))
	}
	return c
}

func shortRowInput(f shortImportFixture, batchID, rowID int64) Trading212ShortRowInput {
	return Trading212ShortRowInput{OwnerUserID: f.ownerUserID, BatchID: batchID, RowID: rowID}
}

func TestTrading212ShortSaleRowLinksToRecordedShortSaleWithoutDoublePosting(t *testing.T) {
	t.Parallel()
	f := newShortImportFixture(t)
	ctx := context.Background()
	operationID, transactionID := f.shortSale(t)
	batchID, rowID := f.stageOrderFillRow(t, f.connectionID, shortFill("short-1"))
	input := shortRowInput(f, batchID, rowID)

	review, err := f.importService.Trading212ShortCandidates(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, Trading212ShortSource{OperationKind: "short_sale", FillType: "UNDOCUMENTED_SHORT", Side: "SELL",
		Ticker: "AAPL_US_EQ", ISIN: "US0378331005", Date: "2026-05-01", Quantity: "10", NetValue: "1000", NetCurrency: "EUR"}, review.Source)
	require.Len(t, review.Candidates, 1)
	assert.Equal(t, Trading212ShortCandidate{OperationID: operationID, OperationKind: "short_sale", TransactionID: transactionID,
		HoldingAccountID: f.holdingID, CashAccountID: f.cashAccountID, EventDate: "2026-05-01", Quantity: "10",
		NetAmount: "1000.00", CashCurrency: "EUR", Mismatches: []string{}}, review.Candidates[0])

	before := f.counts(t)
	require.NoError(t, f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{Trading212ShortRowInput: input, OperationID: operationID}))
	after := f.counts(t)
	assert.Equal(t, shortLinkCounts{transactions: before.transactions, lots: before.lots, decisions: before.decisions,
		audits: before.audits + 1, identities: before.identities + 1}, after, "the link posts no journal, lot or disposal")
	var auditOperation string
	require.NoError(t, f.database.QueryRow(`SELECT operation FROM audit_events ORDER BY id DESC LIMIT 1`).Scan(&auditOperation))
	assert.Equal(t, "import.row.link_short_sale", auditOperation)

	row, err := f.importRepo.ImportStagedRowByID(ctx, rowID)
	require.NoError(t, err)
	assert.Equal(t, "committed", row.CommitStatus)
	assert.Equal(t, transactionID, row.CommittedTransactionID.Int64)
	require.Len(t, row.CommitEffects, 1)
	assert.Equal(t, operationID, row.CommitEffects[0].OperationID.Int64)

	// Committing the batch afterwards cannot post the row.
	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.CommittedCount)
	assert.Equal(t, after.transactions, f.counts(t).transactions)

	// A re-fetch is a duplicate; neither it nor another row can link again.
	retryBatch, retryRow := f.stageOrderFillRow(t, f.connectionID, shortFill("short-1"))
	retried, err := f.importRepo.ImportStagedRowByID(ctx, retryRow)
	require.NoError(t, err)
	assert.Equal(t, "duplicate", retried.DedupeStatus)
	require.ErrorIs(t, f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{
		Trading212ShortRowInput: shortRowInput(f, retryBatch, retryRow), OperationID: operationID}), ErrImportShortLinkUnavailable)
	otherBatch, otherRow := f.stageOrderFillRow(t, f.connectionID, shortFill("short-2"))
	other := shortRowInput(f, otherBatch, otherRow)
	review, err = f.importService.Trading212ShortCandidates(ctx, other)
	require.NoError(t, err)
	require.Len(t, review.Candidates, 1)
	assert.True(t, review.Candidates[0].Linked)
	require.ErrorIs(t, f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{
		Trading212ShortRowInput: other, OperationID: operationID}), ErrImportShortLinkUnavailable)
	assert.Equal(t, after.transactions, f.counts(t).transactions)
	requireKnownBasisSelfCheckPasses(t, f.database)
}

func TestTrading212CoverRowLinksToRecordedShortCover(t *testing.T) {
	t.Parallel()
	f := newShortImportFixture(t)
	ctx := context.Background()
	saleID, _ := f.shortSale(t)
	cover := f.trade("2026-05-20", 4, 36000)
	cover.CostBasisMethod = "fifo"
	covered, err := f.investmentSvc.ShortCover(ctx, cover)
	require.NoError(t, err)
	coverID := f.operationOf(t, covered.Transaction.ID)

	fill := shortFill("cover-1")
	fill.Side, fill.Quantity, fill.FilledAt, fill.NetValue = "BUY", "4", "2026-05-20T15:00:00Z", "-360"
	batchID, rowID := f.stageOrderFillRow(t, f.connectionID, fill)
	input := shortRowInput(f, batchID, rowID)
	review, err := f.importService.Trading212ShortCandidates(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "short_cover", review.Source.OperationKind)
	require.Len(t, review.Candidates, 1, "a BUY row is compared only with covers")
	assert.Equal(t, coverID, review.Candidates[0].OperationID)
	assert.Equal(t, "-360.00", review.Candidates[0].NetAmount)
	assert.Empty(t, review.Candidates[0].Mismatches)
	require.ErrorIs(t, f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{Trading212ShortRowInput: input, OperationID: saleID}),
		ErrImportShortLinkUnavailable, "a BUY row never evidences a short sale")

	before := f.counts(t)
	require.NoError(t, f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{Trading212ShortRowInput: input, OperationID: coverID}))
	after := f.counts(t)
	assert.Equal(t, before.decisions, after.decisions)
	assert.Equal(t, before.transactions, after.transactions)
	assert.Equal(t, before.identities+1, after.identities)
	requireKnownBasisSelfCheckPasses(t, f.database)
}

func TestTrading212ShortLinkRefusesMismatchedEvidenceAndIneligibleRows(t *testing.T) {
	t.Parallel()
	f := newShortImportFixture(t)
	ctx := context.Background()
	operationID, _ := f.shortSale(t)

	mismatched := shortFill("mismatch")
	mismatched.Quantity, mismatched.FilledAt, mismatched.NetValue = "12", "2026-05-02T10:00:00Z", "999.99"
	batchID, rowID := f.stageOrderFillRow(t, f.connectionID, mismatched)
	input := shortRowInput(f, batchID, rowID)
	review, err := f.importService.Trading212ShortCandidates(ctx, input)
	require.NoError(t, err)
	require.Len(t, review.Candidates, 1)
	want := []string{ShortLinkMismatchDate, ShortLinkMismatchQuantity, ShortLinkMismatchCashAmount}
	assert.Equal(t, want, review.Candidates[0].Mismatches)
	before := f.counts(t)
	err = f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{Trading212ShortRowInput: input, OperationID: operationID})
	var mismatch ImportShortLinkMismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.Equal(t, want, mismatch.Fields)
	assert.Equal(t, before, f.counts(t))

	// Another wallet currency is not the same cash, whatever the number.
	usd := shortFill("usd")
	usd.NetValueCurrency = "USD"
	usdBatch, usdRow := f.stageOrderFillRow(t, f.connectionID, usd)
	review, err = f.importService.Trading212ShortCandidates(ctx, shortRowInput(f, usdBatch, usdRow))
	require.NoError(t, err)
	assert.Equal(t, []string{ShortLinkMismatchCashCurrency}, review.Candidates[0].Mismatches)

	// A short sale recorded in another holding is not the mapped account.
	otherInstrument, _, _, err := f.investmentSvc.ResolveOrCreateInstrumentForImport(ctx, f.ownerUserID, 0, "", "US0378331005", "AAPL_US_EQ")
	require.NoError(t, err)
	otherHolding, err := f.investmentSvc.CreateHoldingAccount(ctx, HoldingAccountInput{OwnerUserID: f.ownerUserID,
		InstrumentID: otherInstrument, Name: "Other AAPL", OpenedOn: "2026-01-01", EffectiveFrom: "2026-01-01"})
	require.NoError(t, err)
	elsewhere := f.trade("2026-05-01", 10, 100000)
	elsewhere.HoldingAccountID = otherHolding.ID
	_, err = f.investmentSvc.ShortSale(ctx, elsewhere)
	require.NoError(t, err)
	review, err = f.importService.Trading212ShortCandidates(ctx, shortRowInput(f, usdBatch, usdRow))
	require.NoError(t, err)
	require.Len(t, review.Candidates, 2)
	assert.Contains(t, review.Candidates[0].Mismatches, ShortLinkMismatchAccount)

	// Only pending unsupported fills of a known security can be linked.
	for name, fill := range map[string]trading212OrderFill{
		"documented trade": func() trading212OrderFill { x := shortFill("trade"); x.FillType = "TRADE"; return x }(),
		"split":            func() trading212OrderFill { x := shortFill("split"); x.FillType = "STOCK_SPLIT"; return x }(),
		"no side":          func() trading212OrderFill { x := shortFill("sideless"); x.Side = ""; return x }(),
	} {
		b, r := f.stageOrderFillRow(t, f.connectionID, fill)
		_, err := f.importService.Trading212ShortCandidates(ctx, shortRowInput(f, b, r))
		require.ErrorIs(t, err, ErrImportShortLinkUnavailable, name)
		require.ErrorIs(t, f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{
			Trading212ShortRowInput: shortRowInput(f, b, r), OperationID: operationID}), ErrImportShortLinkUnavailable, name)
	}
	unknown := shortFill("unknown")
	unknown.Ticker, unknown.ISIN = "MSFT_US_EQ", "US5949181045"
	b, r := f.stageOrderFillRow(t, f.connectionID, unknown)
	review, err = f.importService.Trading212ShortCandidates(ctx, shortRowInput(f, b, r))
	require.NoError(t, err)
	assert.Empty(t, review.Candidates)
	require.ErrorIs(t, f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{
		Trading212ShortRowInput: shortRowInput(f, b, r), OperationID: operationID}), ErrImportShortLinkUnavailable)
	var instruments int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM investment_instruments`).Scan(&instruments))
	assert.Equal(t, 1, instruments, "reviewing a row never creates an instrument")

	// The unsupported row still never posts by itself.
	result, err := f.importService.CommitImportBatch(ctx, CommitImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID})
	require.NoError(t, err)
	assert.Zero(t, result.CommittedCount)
}

// Prepare/commit seam: the operation and the batch are rechecked inside the
// write. A correction or a discard after review refuses the link whole.
func TestTrading212ShortLinkRefusesOperationOrBatchChangedAfterReview(t *testing.T) {
	t.Parallel()
	f := newShortImportFixture(t)
	ctx := context.Background()
	operationID, transactionID := f.shortSale(t)
	batchID, rowID := f.stageOrderFillRow(t, f.connectionID, shortFill("stale"))
	short, err := f.importService.trading212ShortRow(ctx, shortRowInput(f, batchID, rowID))
	require.NoError(t, err)
	reviewed, err := f.importRepo.ShortLinkCandidateByOperation(ctx, BookID, operationID)
	require.NoError(t, err)
	link := db.LinkStagedRowToShortParams{BookID: BookID, BatchID: batchID, RowID: rowID,
		DedupeFingerprint: short.row.DedupeFingerprint, SourceKind: short.sourceKind,
		Expected: reviewed, ActorUserID: f.ownerUserID, Now: "2026-06-01T00:00:00Z"}

	replaced, err := f.investmentSvc.ReplaceShortSale(ctx, ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID,
		TransactionID: transactionID, Reason: "eleven were borrowed", Replacement: f.trade("2026-05-01", 11, 110000)})
	require.NoError(t, err)
	before := f.counts(t)
	_, err = f.importRepo.LinkStagedRowToShort(ctx, link)
	require.ErrorIs(t, err, db.ErrShortLinkChanged)
	assert.Equal(t, before, f.counts(t))
	require.ErrorIs(t, f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{
		Trading212ShortRowInput: shortRowInput(f, batchID, rowID), OperationID: operationID}), ErrImportShortLinkUnavailable,
		"a corrected operation is no longer a candidate")
	review, err := f.importService.Trading212ShortCandidates(ctx, shortRowInput(f, batchID, rowID))
	require.NoError(t, err)
	require.Len(t, review.Candidates, 1)
	assert.Equal(t, f.operationOf(t, replaced.Replacement.Transaction.ID), review.Candidates[0].OperationID)
	assert.Equal(t, []string{ShortLinkMismatchQuantity, ShortLinkMismatchCashAmount}, review.Candidates[0].Mismatches)

	current, err := f.importRepo.ShortLinkCandidateByOperation(ctx, BookID, review.Candidates[0].OperationID)
	require.NoError(t, err)
	link.Expected = current
	require.NoError(t, f.importService.DiscardImportBatch(ctx, DiscardImportBatchInput{OwnerUserID: f.ownerUserID, BatchID: batchID}))
	before = f.counts(t)
	_, err = f.importRepo.LinkStagedRowToShort(ctx, link)
	require.ErrorIs(t, err, db.ErrShortLinkUnavailable)
	assert.Equal(t, before, f.counts(t), "no partial link")
}

// Native correction keeps the source link: the identity stays bound to the
// immutable original, the replacement inherits it through its lineage and
// the provider's raw evidence is never rewritten.
func TestTrading212ShortLinkSurvivesNativeCorrection(t *testing.T) {
	t.Parallel()
	f := newShortImportFixture(t)
	ctx := context.Background()
	operationID, transactionID := f.shortSale(t)
	batchID, rowID := f.stageOrderFillRow(t, f.connectionID, shortFill("corrected"))
	require.NoError(t, f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{
		Trading212ShortRowInput: shortRowInput(f, batchID, rowID), OperationID: operationID}))
	linked, err := f.importRepo.ImportStagedRowByID(ctx, rowID)
	require.NoError(t, err)

	replaced, err := f.investmentSvc.ReplaceShortSale(ctx, ReplaceInvestmentBuyInput{OwnerUserID: f.ownerUserID,
		TransactionID: transactionID, Reason: "fee restated", Replacement: f.trade("2026-05-01", 10, 99500)})
	require.NoError(t, err)
	correction, err := db.NewInvestmentRepository(f.database).TradeCorrectionContext(ctx, BookID, replaced.Replacement.Transaction.ID)
	require.NoError(t, err)
	assert.Equal(t, linked.CommittedIdentityID.Int64, correction.SourceIdentityID)
	assert.Equal(t, "trading212", correction.SourceKind)

	after, err := f.importRepo.ImportStagedRowByID(ctx, rowID)
	require.NoError(t, err)
	assert.Equal(t, linked.RawJSON, after.RawJSON)
	assert.Equal(t, linked.CommitEffects, after.CommitEffects)

	_, retryRow := f.stageOrderFillRow(t, f.connectionID, shortFill("corrected"))
	retried, err := f.importRepo.ImportStagedRowByID(ctx, retryRow)
	require.NoError(t, err)
	assert.Equal(t, "duplicate", retried.DedupeStatus)
	otherBatch, otherRow := f.stageOrderFillRow(t, f.connectionID, shortFill("after-correction"))
	review, err := f.importService.Trading212ShortCandidates(ctx, shortRowInput(f, otherBatch, otherRow))
	require.NoError(t, err)
	require.Len(t, review.Candidates, 1)
	assert.True(t, review.Candidates[0].Linked, "the replacement carries the original's source")

	// A reversal of the linked lineage is a native correction too.
	_, err = f.investmentSvc.ReverseShortSale(ctx, ReverseInvestmentBuyInput{OwnerUserID: f.ownerUserID,
		TransactionID: replaced.Replacement.Transaction.ID, Reason: "never borrowed"})
	require.NoError(t, err)
	final, err := f.importRepo.ImportStagedRowByID(ctx, rowID)
	require.NoError(t, err)
	assert.Equal(t, linked.RawJSON, final.RawJSON)
	assert.Equal(t, "committed", final.CommitStatus)
	requireKnownBasisSelfCheckPasses(t, f.database)
}

func TestTrading212ShortLinkRollsBackOnLateFailure(t *testing.T) {
	t.Parallel()
	f := newShortImportFixture(t)
	ctx := context.Background()
	operationID, _ := f.shortSale(t)
	batchID, rowID := f.stageOrderFillRow(t, f.connectionID, shortFill("late"))
	// The last write of the link fails after its audit event and identity.
	_, err := f.database.Exec(`CREATE TEMP TRIGGER fail_short_link BEFORE UPDATE OF commit_status ON import_staged_rows
		WHEN NEW.commit_status = 'committed' BEGIN SELECT RAISE(ABORT, 'injected late failure'); END`)
	require.NoError(t, err)
	before := f.counts(t)
	err = f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{
		Trading212ShortRowInput: shortRowInput(f, batchID, rowID), OperationID: operationID})
	require.ErrorContains(t, err, "injected late failure")
	assert.Equal(t, before, f.counts(t))
	row, err := f.importRepo.ImportStagedRowByID(ctx, rowID)
	require.NoError(t, err)
	assert.NotEqual(t, "committed", row.CommitStatus)

	_, err = f.database.Exec(`DROP TRIGGER temp.fail_short_link`)
	require.NoError(t, err)
	require.NoError(t, f.importService.LinkTrading212Short(ctx, LinkTrading212ShortInput{
		Trading212ShortRowInput: shortRowInput(f, batchID, rowID), OperationID: operationID}))
}

func TestTrading212ShortLinkConcurrentRowsLinkOnce(t *testing.T) {
	t.Parallel()
	f := newShortImportFixture(t)
	ctx := context.Background()
	operationID, _ := f.shortSale(t)
	inputs := make([]LinkTrading212ShortInput, 4)
	for i := range inputs {
		batchID, rowID := f.stageOrderFillRow(t, f.connectionID, shortFill("race-"+string(rune('a'+i))))
		inputs[i] = LinkTrading212ShortInput{Trading212ShortRowInput: shortRowInput(f, batchID, rowID), OperationID: operationID}
	}
	errs := make([]error, len(inputs))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range inputs {
		wg.Go(func() {
			<-start
			for {
				errs[i] = f.importService.LinkTrading212Short(ctx, inputs[i])
				if !db.IsSQLiteBusy(errs[i]) {
					return
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
	close(start)
	wg.Wait()
	linked := 0
	for _, err := range errs {
		if err == nil {
			linked++
			continue
		}
		require.True(t, errors.Is(err, ErrImportShortLinkUnavailable), "unexpected error: %v", err)
	}
	assert.Equal(t, 1, linked)
	var effects int
	require.NoError(t, f.database.QueryRow(`SELECT COUNT(*) FROM import_commit_identity_effects WHERE operation_id = ?`, operationID).Scan(&effects))
	assert.Equal(t, 1, effects)
}
