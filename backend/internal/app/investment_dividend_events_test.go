package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
	"rekenraam/backend/internal/marketdata"
)

// dividendEventFixture holds 10 ASML (Twelve Data ASML/XAMS) bought on
// 2026-03-02 and points the dividend provider at a fake Twelve Data.
type dividendEventFixture struct {
	*investTestFixture
	pricing    *PricingService
	instrument InvestmentInstrument
	holding    Account
	body       atomic.Value
	calls      atomic.Int32
	logger     *slog.Logger
}

const dividendEventBody = `{"meta":{"symbol":"ASML","currency":"EUR","mic_code":"XAMS","exchange_timezone":"Europe/Amsterdam"},
"dividends":[{"ex_date":"2026-11-02","amount":9},{"ex_date":"2026-07-29","amount":1.6},{"ex_date":"2026-04-28","amount":"bad"},{"ex_date":"2026-02-02","amount":1.5}]}`

func newDividendEventFixture(t *testing.T) *dividendEventFixture {
	t.Helper()
	ctx := context.Background()
	f := &dividendEventFixture{investTestFixture: newInvestTestFixture(t), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	f.body.Store(dividendEventBody)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Uses the single write connection: it would hang if the fetch ran
		// inside a database transaction.
		var count int
		assert.NoError(t, f.database.QueryRowContext(r.Context(), `SELECT count(*) FROM transactions`).Scan(&count))
		f.calls.Add(1)
		assert.Equal(t, "/dividends", r.URL.Path)
		_, _ = fmt.Fprint(w, f.body.Load().(string))
	}))
	t.Cleanup(server.Close)
	f.pricing = NewPricingService(db.NewPricingRepository(f.database), marketdata.NewRegistry(marketdata.NewTwelveDataProvider(server.Client(), server.URL, "secret")))
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	f.pricing.SetNowForTest(func() time.Time { return now })
	instrument, err := f.investmentSvc.CreateInstrument(ctx, InvestmentInstrumentInput{OwnerUserID: 1, OriginType: "browser_api", ChangeReason: "fixture",
		InstrumentType: "stock", DisplayName: "ASML Holding", CommodityCode: "ASML", Symbol: "ASML", MIC: "XAMS", QuantityScale: 4, PriceScale: 4,
		QuoteCommodityID: &f.eurCommodityID, IdentifiersJSON: `{"twelve_data_symbol":"ASML"}`})
	require.NoError(t, err)
	f.instrument = instrument
	f.holding, err = f.investmentSvc.CreateHoldingAccount(ctx, HoldingAccountInput{OwnerUserID: 1, InstrumentID: instrument.ID, Name: "ASML", OpenedOn: "2020-01-01", EffectiveFrom: "2020-01-01"})
	require.NoError(t, err)
	_, err = f.investmentSvc.Buy(ctx, InvestmentTradeInput{OwnerUserID: 1, TransactionDate: "2026-03-02", CommodityID: instrument.CommodityID, HoldingAccountID: f.holding.ID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, QuantityValue: exact.MustParse("10"), QuantityScale: 0, CashAmountValue: 600000, CashAmountScale: 2})
	require.NoError(t, err)
	var sourceID int64
	require.NoError(t, f.database.QueryRow(`SELECT id FROM market_data_sources WHERE code='twelve_data'`).Scan(&sourceID))
	_, err = f.pricing.SaveSourceAssignment(ctx, PricingSourceAssignmentInput{OwnerUserID: 1, CommodityID: instrument.CommodityID, QuoteCommodityID: f.eurCommodityID, SourceID: sourceID, EffectiveFrom: "2020-01-01"})
	require.NoError(t, err)
	return f
}

func (f *dividendEventFixture) fetch(t *testing.T) {
	t.Helper()
	queued, err := f.pricing.QueueDividendEventFetch(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, queued)
	f.pricing.runDueDividendEvents(context.Background(), f.logger, "worker")
}

func (f *dividendEventFixture) suggestions(t *testing.T) []InvestmentEventSuggestion {
	t.Helper()
	suggestions, err := f.investmentSvc.ListEventSuggestions(context.Background())
	require.NoError(t, err)
	return suggestions
}

func suggestionForExDate(t *testing.T, suggestions []InvestmentEventSuggestion, exDate string) InvestmentEventSuggestion {
	t.Helper()
	for _, suggestion := range suggestions {
		if suggestion.EventDate == exDate {
			return suggestion
		}
	}
	require.Failf(t, "missing suggestion", "no suggestion for ex-date %s", exDate)
	return InvestmentEventSuggestion{}
}

func (f *dividendEventFixture) completion(date string, gross int64) *SuggestionCompletion {
	return &SuggestionCompletion{TransactionDate: date, CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, AmountValue: gross, AmountScale: 2, IncomeAccountID: &f.categoryAccountID}
}

func TestDividendProducerStagesHeldExDatesAsIncompleteEvidence(t *testing.T) {
	t.Parallel()
	f := newDividendEventFixture(t)
	f.fetch(t)

	// 2026-02-02 predates the holding and 2026-11-02 has not gone ex yet.
	suggestions := f.suggestions(t)
	require.Len(t, suggestions, 2)
	july := suggestionForExDate(t, suggestions, "2026-07-29")
	assert.Equal(t, "suggested", july.Status)
	assert.Equal(t, "dividend", july.EventFamily)
	assert.Equal(t, "twelve_data", july.SourceCode)
	assert.Equal(t, "new", july.EventStatus)
	assert.Equal(t, 7000, july.ConfidenceBPS)
	var evidence providerDividendEvidence
	require.NoError(t, json.Unmarshal([]byte(july.EventEvidenceJSON), &evidence))
	assert.Equal(t, "2026-07-28", evidence.EntitlementDate)
	assert.Equal(t, &providerDecimal{Value: "16", Scale: 1}, evidence.AmountPerShare)
	assert.Equal(t, &providerDecimal{Value: "10", Scale: 0}, evidence.EntitledUnits)
	assert.Equal(t, &providerDecimal{Value: "16", Scale: 0}, evidence.EstimatedGross)
	assert.Equal(t, "EUR", evidence.ListingCurrency)
	require.Len(t, evidence.Holdings, 1)
	assert.Equal(t, f.holding.ID, evidence.Holdings[0].AccountID)
	assert.ElementsMatch(t, []string{dividendMissingPaymentDate, dividendMissingCashAccount, dividendMissingReceivedAmount, dividendMissingCurrency}, evidence.Missing)
	assert.JSONEq(t, `{"kind":"dividend_income","complete":false,"commodity_id":`+fmt.Sprint(f.instrument.CommodityID)+
		`,"external_ref_hint":"twelve_data:dividend:ASML:XAMS:2026-07-29","missing":["payment_date","cash_account","received_amount","dividend_currency"]}`, july.ProposedTransactionJSON)

	april := suggestionForExDate(t, suggestions, "2026-04-28")
	var aprilEvidence providerDividendEvidence
	require.NoError(t, json.Unmarshal([]byte(april.EventEvidenceJSON), &aprilEvidence))
	assert.Nil(t, aprilEvidence.AmountPerShare, "an unreadable amount is kept as missing, never guessed")
	assert.Nil(t, aprilEvidence.EstimatedGross)
	assert.Contains(t, aprilEvidence.Missing, dividendMissingAmountPerShare)
	assert.Equal(t, 3000, april.ConfidenceBPS)

	var raw string
	require.NoError(t, f.database.QueryRow(`SELECT raw_json FROM investment_provider_events WHERE provider_event_id = 'twelve_data:dividend:ASML:XAMS:2026-07-29'`).Scan(&raw))
	assert.JSONEq(t, `{"meta":{"symbol":"ASML","currency":"EUR","mic_code":"XAMS"},"dividend":{"ex_date":"2026-07-29","amount":1.6},"adjust":false}`, raw)
	var dividends int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations WHERE operation_kind = 'dividend'`).Scan(&dividends))
	assert.Zero(t, dividends, "staging never posts")
}

func TestDividendProducerRefetchKeepsOwnerDecisionsAndNeverDuplicates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	f.fetch(t)
	suggestions := f.suggestions(t)
	july := suggestionForExDate(t, suggestions, "2026-07-29")
	april := suggestionForExDate(t, suggestions, "2026-04-28")
	accepted, err := f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, f.completion("2026-08-14", 1360))
	require.NoError(t, err)
	_, err = f.investmentSvc.IgnoreSuggestion(ctx, 1, 0, "req", april.ID)
	require.NoError(t, err)

	// A revised provider amount for the same ex-date does not replace the
	// first observation or reopen the owner's decisions (#254: it is kept as
	// revision evidence beside them).
	f.body.Store(`{"meta":{"symbol":"ASML","currency":"EUR","mic_code":"XAMS"},"dividends":[{"ex_date":"2026-07-29","amount":1.7},{"ex_date":"2026-04-28","amount":1.84}]}`)
	f.fetch(t)
	f.fetch(t)
	after := f.suggestions(t)
	require.Len(t, after, 2)
	july = suggestionForExDate(t, after, "2026-07-29")
	assert.Equal(t, "accepted", july.Status)
	assert.Equal(t, "matched", july.EventStatus)
	assert.Equal(t, accepted.GeneratedTransactionID, july.GeneratedTransactionID)
	assert.Contains(t, july.EventEvidenceJSON, `"value":"16"`)
	require.Len(t, july.Revisions, 1)
	assert.JSONEq(t, `{"schema":1,"amount_per_share":{"value":"17","scale":1},"listing_currency":"EUR"}`, july.Revisions[0].EvidenceJSON)
	april = suggestionForExDate(t, after, "2026-04-28")
	assert.Equal(t, "ignored", april.Status)
	assert.Equal(t, "ignored", april.EventStatus)
	require.Len(t, april.Revisions, 1, "an unreadable first amount later stated is a revision")
	assert.JSONEq(t, `{"schema":1,"amount_per_share":{"value":"184","scale":2},"listing_currency":"EUR"}`, april.Revisions[0].EvidenceJSON)
	var events, dividends int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_provider_events`).Scan(&events))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_operations WHERE operation_kind = 'dividend'`).Scan(&dividends))
	assert.Equal(t, 2, events)
	assert.Equal(t, 1, dividends)
	assert.Equal(t, int32(3), f.calls.Load())
}

func TestProviderDividendRevisionWhilePendingIsEvidenceOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	f.fetch(t)
	before := suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	revisionAudits := func() int {
		var count int
		require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM audit_events WHERE operation = 'investment.provider_event.revise'`).Scan(&count))
		return count
	}
	body := func(amount string) string {
		return `{"meta":{"symbol":"ASML","currency":"EUR","mic_code":"XAMS"},"dividends":[{"ex_date":"2026-07-29","amount":` + amount + `}]}`
	}

	// The same announcement written differently (1.60 vs 1.6) is not a revision.
	f.body.Store(body(`"1.60"`))
	f.fetch(t)
	assert.Empty(t, suggestionForExDate(t, f.suggestions(t), "2026-07-29").Revisions)
	assert.Equal(t, 0, revisionAudits())

	// A differing amount is appended once per distinct content, each under
	// its own audit event; re-fetching it changes nothing.
	f.body.Store(body("1.7"))
	f.fetch(t)
	f.fetch(t)
	f.body.Store(body("1.70"))
	f.fetch(t)
	assert.Equal(t, 1, revisionAudits())
	// Returning to the first amount is the first observation, not a revision.
	f.body.Store(body("1.6"))
	f.fetch(t)
	f.body.Store(body("1.8"))
	f.fetch(t)
	assert.Equal(t, 2, revisionAudits())

	july := suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	require.Len(t, july.Revisions, 2)
	assert.Contains(t, july.Revisions[0].EvidenceJSON, `"value":"17"`)
	assert.Contains(t, july.Revisions[1].EvidenceJSON, `"value":"18"`)
	assert.Equal(t, "suggested", july.Status)
	assert.Equal(t, "new", july.EventStatus)
	assert.Equal(t, before.EventEvidenceJSON, july.EventEvidenceJSON, "the first observation stays the event's evidence")
	assert.Equal(t, before.ProposedTransactionJSON, july.ProposedTransactionJSON)
	assert.Equal(t, before.UpdatedAt, july.UpdatedAt)
	var suggestionsCount, raw int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_event_suggestions`).Scan(&suggestionsCount))
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_provider_event_revisions WHERE raw_json LIKE '%1.8%'`).Scan(&raw))
	assert.Equal(t, 2, suggestionsCount, "a revision never queues a second review")
	assert.Equal(t, 1, raw, "the revision keeps the provider's raw row")

	// Revision evidence is immutable.
	_, err := f.database.Exec(`UPDATE investment_provider_event_revisions SET content_key = 'x'`)
	require.Error(t, err)
	_, err = f.database.Exec(`DELETE FROM investment_provider_event_revisions`)
	require.Error(t, err)

	// The pending suggestion is still completed from the owner's statement.
	accepted, err := f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, f.completion("2026-08-14", 1800))
	require.NoError(t, err)
	assert.Equal(t, "accepted", accepted.Status)
}

func TestProviderDividendRevisionAfterLinkLeavesLinkAndPayment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	f.fetch(t)
	july := suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	recorded, err := f.investmentSvc.Dividend(ctx, DividendInput{OwnerUserID: 1, TransactionDate: "2026-08-14", CommodityID: NullableValue(f.instrument.CommodityID),
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.categoryAccountID, AmountValue: 1600, AmountScale: 2})
	require.NoError(t, err)
	linked, err := f.investmentSvc.LinkSuggestion(ctx, 1, 0, "req", july.ID, recorded.ID)
	require.NoError(t, err)
	versionsBefore := countTransactions(t, f.database)

	f.body.Store(`{"meta":{"symbol":"ASML","currency":"EUR","mic_code":"XAMS"},"dividends":[{"ex_date":"2026-07-29","amount":1.65}]}`)
	f.fetch(t)

	july = suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	require.Len(t, july.Revisions, 1)
	assert.Equal(t, "linked", july.Status)
	assert.Equal(t, "matched", july.EventStatus)
	assert.Equal(t, linked.LinkedTransactionID, july.LinkedTransactionID)
	assert.Equal(t, linked.UpdatedAt, july.UpdatedAt)
	assert.Equal(t, versionsBefore, countTransactions(t, f.database), "a revision never posts or corrects a dividend")
}

func TestIncompleteProviderDividendNeedsOwnerCompletionAndStaysPending(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	f.fetch(t)
	july := suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	before := countTransactions(t, f.database)

	_, err := f.investmentSvc.AcceptSuggestion(ctx, 1, 0, "req", july.ID)
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Contains(t, validation.Message, "incomplete")

	missingCash := f.completion("2026-08-14", 1360)
	missingCash.CashAccountID = 0
	_, err = f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, missingCash)
	require.ErrorAs(t, err, &validation)

	// The dividend writer's own refusal (an income account that is not income)
	// is returned to the owner instead of failing the suggestion.
	wrongIncome := f.completion("2026-08-14", 1360)
	wrongIncome.IncomeAccountID = &f.cashAccountID
	_, err = f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, wrongIncome)
	require.Error(t, err)

	assert.Equal(t, before, countTransactions(t, f.database))
	july = suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	assert.Equal(t, "suggested", july.Status)
	assert.Equal(t, "new", july.EventStatus)
}

func TestCompletedProviderDividendCommitsThroughNativeCommandAtomically(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	f.fetch(t)
	july := suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	completion := f.completion("2026-08-14", 1600)
	withholding, scale := int64(240), 2
	completion.WithholdingValue, completion.WithholdingScale, completion.WithholdingAccountID = &withholding, &scale, &f.categoryAccountID
	completion.WithholdingAccountID = nil
	_, err := f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, completion)
	require.Error(t, err, "withholding without an account is refused by the native command")

	completion.WithholdingValue, completion.WithholdingScale = nil, nil
	accepted, err := f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req-accept", july.ID, completion)
	require.NoError(t, err)
	assert.Equal(t, "accepted", accepted.Status)
	assert.Equal(t, "matched", accepted.EventStatus)
	require.NotNil(t, accepted.GeneratedTransactionID)

	var payer int64
	var auditOperation, eventDate string
	require.NoError(t, f.database.QueryRow(`
		SELECT facts.commodity_id, o.event_date, a.operation
		FROM investment_operations o
		JOIN investment_dividend_facts facts ON facts.operation_id = o.id
		JOIN audit_events a ON a.id = o.created_audit_event_id
		WHERE o.operation_kind = 'dividend'`).Scan(&payer, &eventDate, &auditOperation))
	assert.Equal(t, f.instrument.CommodityID, payer, "the payer fact comes from the event instrument")
	assert.Equal(t, "2026-08-14", eventDate, "the owner's payment date, not the ex-date, is the transaction date")
	assert.Equal(t, "investment.event_suggestion.accept", auditOperation)

	_, err = f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, completion)
	require.ErrorIs(t, err, ErrInvestmentSuggestionNotPending)
	requireKnownBasisSelfCheckPasses(t, f.database)
}

func TestProviderDividendAcceptLateFailureRollsBackPosting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	f.fetch(t)
	july := suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	before := countTransactions(t, f.database)
	// The event-status write is the last statement of the accept transaction.
	_, err := f.database.ExecContext(ctx, `CREATE TEMP TRIGGER refuse_event_match BEFORE UPDATE ON investment_provider_events
		BEGIN SELECT RAISE(ABORT, 'late failure'); END`)
	require.NoError(t, err)
	_, err = f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, f.completion("2026-08-14", 1600))
	require.Error(t, err)
	_, err = f.database.ExecContext(ctx, `DROP TRIGGER temp.refuse_event_match`)
	require.NoError(t, err)

	assert.Equal(t, before, countTransactions(t, f.database))
	july = suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	assert.Equal(t, "suggested", july.Status)
	assert.Nil(t, july.GeneratedTransactionID)
	accepted, err := f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, f.completion("2026-08-14", 1600))
	require.NoError(t, err, "the rolled-back accept leaves the suggestion reviewable")
	assert.Equal(t, "accepted", accepted.Status)
}

func TestProviderDividendLinksOwnerRecordedDividendOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	// The owner recorded the July payment manually before any fetch.
	manual, err := f.investmentSvc.Dividend(ctx, DividendInput{OwnerUserID: 1, TransactionDate: "2026-08-14", CommodityID: NullableValue(f.instrument.CommodityID),
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.categoryAccountID, AmountValue: 1600, AmountScale: 2})
	require.NoError(t, err)
	// A dividend recorded before the ex-date is not offered for it.
	early, err := f.investmentSvc.Dividend(ctx, DividendInput{OwnerUserID: 1, TransactionDate: "2026-07-01", CommodityID: NullableValue(f.instrument.CommodityID),
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.categoryAccountID, AmountValue: 100, AmountScale: 2})
	require.NoError(t, err)
	f.fetch(t)
	suggestions := f.suggestions(t)
	july := suggestionForExDate(t, suggestions, "2026-07-29")
	require.Len(t, july.LinkCandidates, 1)
	assert.Equal(t, manual.ID, july.LinkCandidates[0].TransactionID)
	assert.Equal(t, exact.MustParse("16"), july.LinkCandidates[0].GrossValue)
	assert.Equal(t, 0, july.LinkCandidates[0].GrossScale)
	april := suggestionForExDate(t, suggestions, "2026-04-28")
	require.Len(t, april.LinkCandidates, 1, "only the July 1 payment falls in April's 90-day window")
	assert.Equal(t, early.ID, april.LinkCandidates[0].TransactionID)

	_, err = f.investmentSvc.LinkSuggestion(ctx, 1, 0, "req", july.ID, early.ID)
	var validation ValidationError
	require.ErrorAs(t, err, &validation, "a payment before the ex-date is not this dividend")
	before := countTransactions(t, f.database)
	linked, err := f.investmentSvc.LinkSuggestion(ctx, 1, 0, "req", july.ID, manual.ID)
	require.NoError(t, err)
	assert.Equal(t, "linked", linked.Status)
	assert.Equal(t, "matched", linked.EventStatus)
	assert.Equal(t, &manual.ID, linked.LinkedTransactionID)
	assert.Equal(t, before, countTransactions(t, f.database), "linking posts nothing")

	// The schema refuses a second provider event for one payment, whatever
	// the service checks.
	_, err = f.database.Exec(`UPDATE investment_event_suggestions SET linked_transaction_id = ?, status = 'linked' WHERE id = ?`, manual.ID, april.ID)
	require.Error(t, err)
	linkedEarly, err := f.investmentSvc.LinkSuggestion(ctx, 1, 0, "req", april.ID, early.ID)
	require.NoError(t, err)
	assert.Equal(t, "linked", linkedEarly.Status)
	_, err = f.investmentSvc.LinkSuggestion(ctx, 1, 0, "req", july.ID, manual.ID)
	require.ErrorIs(t, err, ErrInvestmentSuggestionNotPending)

	f.fetch(t)
	july = suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	assert.Equal(t, "linked", july.Status, "a later fetch does not reopen a linked event")
}

// goShort closes the fixture's long position and sells 5 short before the
// July ex-date.
func (f *dividendEventFixture) goShort(t *testing.T, holdingAccountID int64) {
	t.Helper()
	ctx := context.Background()
	_, err := f.investmentSvc.Sell(ctx, InvestmentTradeInput{OwnerUserID: 1, TransactionDate: "2026-06-01", CommodityID: f.instrument.CommodityID, HoldingAccountID: f.holding.ID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, QuantityValue: exact.MustParse("10"), CashAmountValue: 600000, CashAmountScale: 2})
	require.NoError(t, err)
	_, err = f.investmentSvc.ShortSale(ctx, InvestmentTradeInput{OwnerUserID: 1, TransactionDate: "2026-06-02", CommodityID: f.instrument.CommodityID, HoldingAccountID: holdingAccountID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, QuantityValue: exact.MustParse("5"), CashAmountValue: 300000, CashAmountScale: 2})
	require.NoError(t, err)
}

// #255: a dividend whose ex-date found the book short only proposes a
// payment in lieu the owner completes; it commits through the native
// payment-in-lieu command and is never offered as or linked to income.
func TestShortProviderDividendCompletesIntoPaymentInLieu(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	f.goShort(t, f.holding.ID)
	// A cash dividend the owner recorded for the instrument is not this event.
	_, err := f.investmentSvc.Dividend(ctx, DividendInput{OwnerUserID: 1, TransactionDate: "2026-08-14", CommodityID: NullableValue(f.instrument.CommodityID),
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.categoryAccountID, AmountValue: 800, AmountScale: 2})
	require.NoError(t, err)
	f.fetch(t)
	july := suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	var proposal map[string]any
	require.NoError(t, json.Unmarshal([]byte(july.ProposedTransactionJSON), &proposal))
	assert.Equal(t, "payment_in_lieu", proposal["kind"])
	assert.Equal(t, "2026-07-29", proposal["ex_date"])
	assert.Equal(t, []any{float64(f.holding.ID)}, proposal["holding_account_ids"])
	assert.Contains(t, july.EventEvidenceJSON, `"estimated_gross":{"value":"8","scale":0}`, "5 short units × 1.6")
	assert.Contains(t, july.EventEvidenceJSON, `"amount_owed"`)
	assert.NotContains(t, july.EventEvidenceJSON, `"unsupported"`)
	assert.Empty(t, july.LinkCandidates, "a cash dividend is never linked to a short's payment in lieu")

	before := countTransactions(t, f.database)
	var validation ValidationError
	_, err = f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, nil)
	require.ErrorAs(t, err, &validation)
	_, err = f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, f.completion("2026-08-14", 800))
	require.ErrorAs(t, err, &validation, "income is not what a short holder posts")
	expense := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	completion := &SuggestionCompletion{TransactionDate: "2026-08-14", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		AmountValue: 800, AmountScale: 2, ExpenseAccountID: &expense}
	other := int64(999)
	completion.HoldingAccountID = &other
	_, err = f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, completion)
	require.ErrorAs(t, err, &validation, "only a holding the evidence shows short")
	assert.Equal(t, before, countTransactions(t, f.database))
	assert.Equal(t, "suggested", suggestionForExDate(t, f.suggestions(t), "2026-07-29").Status)

	completion.HoldingAccountID = nil
	accepted, err := f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, completion)
	require.NoError(t, err)
	assert.Equal(t, "accepted", accepted.Status)
	require.NotNil(t, accepted.GeneratedTransactionID)
	var kind, exDate string
	var holding int64
	require.NoError(t, f.database.QueryRow(`SELECT o.operation_kind, f.ex_date, f.holding_account_id FROM investment_operations o
		JOIN investment_operation_journal_links l ON l.operation_id = o.id AND l.role = 'primary'
		JOIN transaction_versions v ON v.id = l.transaction_version_id
		JOIN investment_payment_in_lieu_facts f ON f.operation_id = o.id
		WHERE v.transaction_id = ?`, *accepted.GeneratedTransactionID).Scan(&kind, &exDate, &holding))
	assert.Equal(t, []any{"payment_in_lieu", "2026-07-29", f.holding.ID}, []any{kind, exDate, holding})
}

func TestProviderPaymentInLieuAcceptLateFailureRollsBackPosting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	f.goShort(t, f.holding.ID)
	f.fetch(t)
	july := suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	expense := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	completion := &SuggestionCompletion{TransactionDate: "2026-08-14", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID,
		AmountValue: 800, AmountScale: 2, ExpenseAccountID: &expense}
	before := countTransactions(t, f.database)
	_, err := f.database.ExecContext(ctx, `CREATE TEMP TRIGGER refuse_event_match BEFORE UPDATE ON investment_provider_events
		BEGIN SELECT RAISE(ABORT, 'late failure'); END`)
	require.NoError(t, err)
	_, err = f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, completion)
	require.Error(t, err)
	_, err = f.database.ExecContext(ctx, `DROP TRIGGER temp.refuse_event_match`)
	require.NoError(t, err)
	assert.Equal(t, before, countTransactions(t, f.database))
	var facts int
	require.NoError(t, f.database.QueryRow(`SELECT count(*) FROM investment_payment_in_lieu_facts`).Scan(&facts))
	assert.Zero(t, facts)
	july = suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	assert.Equal(t, "suggested", july.Status)
	accepted, err := f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, completion)
	require.NoError(t, err, "the rolled-back accept leaves the suggestion reviewable")
	assert.Equal(t, "accepted", accepted.Status)
}

// A book long in one holding and short in another at the close owes neither
// a single dividend nor a single payment in lieu: the event stays
// informational and can only be ignored or recorded by hand.
func TestLongAndShortProviderDividendIsInformationalOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	second, err := f.investmentSvc.CreateHoldingAccount(ctx, HoldingAccountInput{OwnerUserID: 1, InstrumentID: f.instrument.ID, Name: "ASML short", OpenedOn: "2020-01-01", EffectiveFrom: "2020-01-01"})
	require.NoError(t, err)
	_, err = f.investmentSvc.ShortSale(ctx, InvestmentTradeInput{OwnerUserID: 1, TransactionDate: "2026-06-02", CommodityID: f.instrument.CommodityID, HoldingAccountID: second.ID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, QuantityValue: exact.MustParse("5"), CashAmountValue: 300000, CashAmountScale: 2})
	require.NoError(t, err)
	f.fetch(t)
	july := suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	assert.JSONEq(t, `{"kind":"informational","reason":"long_and_short_position"}`, july.ProposedTransactionJSON)
	assert.Contains(t, july.EventEvidenceJSON, `"unsupported":"long_and_short_position"`)
	_, err = f.investmentSvc.AcceptSuggestionWithCompletion(ctx, 1, 0, "req", july.ID, f.completion("2026-08-14", 800))
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	july = suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	assert.Equal(t, "suggested", july.Status, "an informational row stays reviewable and can be ignored")
	ignored, err := f.investmentSvc.IgnoreSuggestion(ctx, 1, 0, "req", july.ID)
	require.NoError(t, err)
	assert.Equal(t, "ignored", ignored.Status)
}

func TestDividendProducerFailureNeverBlocksManualEntry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	f.body.Store(`{"code":403,"message":"plan","status":"error"}`)
	f.fetch(t)
	failed, err := f.pricing.FailedBackgroundWork(ctx)
	require.NoError(t, err)
	require.Len(t, failed, 1, "a missing plan entitlement is terminal and visible")
	assert.Equal(t, dividendEventWorkKind, failed[0].Kind)
	assert.Empty(t, f.suggestions(t))
	_, err = f.investmentSvc.Dividend(ctx, DividendInput{OwnerUserID: 1, TransactionDate: "2026-08-14", CommodityID: NullableValue(f.instrument.CommodityID),
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.categoryAccountID, AmountValue: 1600, AmountScale: 2})
	require.NoError(t, err, "manual dividend entry is unaffected by provider coverage")

	f.body.Store(dividendEventBody)
	_, err = f.pricing.RetryBackgroundWork(ctx, failed[0].ID)
	require.NoError(t, err)
	f.pricing.runDueDividendEvents(ctx, f.logger, "worker")
	assert.Len(t, f.suggestions(t), 2)
}

func countTransactions(t *testing.T, database interface {
	QueryRow(string, ...any) *sql.Row
}) int {
	t.Helper()
	var count int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM transactions`).Scan(&count))
	return count
}

// #258: a payment-in-lieu suggestion offers only an effective payment in lieu
// of its instrument, for one of its short holdings, with its ex-date, and
// links it once without posting; a cash dividend is never offered for it.
func TestProviderPaymentInLieuLinksOwnerRecordedPaymentOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newDividendEventFixture(t)
	f.goShort(t, f.holding.ID)
	expense := seedTestAccountWithClass(t, f.database, "active", true, "expense", "expense")
	recovery := seedTestAccountWithClass(t, f.database, "active", true, "income", "income")
	pil := func(holdingAccountID, commodityID int64, exDate string, owed int64) Transaction {
		t.Helper()
		payment, err := f.investmentSvc.PaymentInLieu(ctx, PaymentInLieuInput{OwnerUserID: 1, HoldingAccountID: NullableValue(holdingAccountID), CommodityID: NullableValue(commodityID),
			ExDate: NullableValue(exDate), TransactionDate: "2026-08-14", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, ExpenseAccountID: expense,
			AmountValue: owed, AmountScale: 2})
		require.NoError(t, err)
		return payment
	}

	// The owner recorded the July payment by hand, with a recovery that
	// does not change what was owed.
	recovered := int64(100)
	manual, err := f.investmentSvc.PaymentInLieu(ctx, PaymentInLieuInput{OwnerUserID: 1, HoldingAccountID: NullableValue(f.holding.ID), CommodityID: NullableValue(f.instrument.CommodityID),
		ExDate: NullableValue("2026-07-29"), TransactionDate: "2026-08-14", CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, ExpenseAccountID: expense,
		AmountValue: 800, AmountScale: 2, RecoveryValue: &recovered, RecoveryAccountID: &recovery})
	require.NoError(t, err)
	wrongExDate := pil(f.holding.ID, f.instrument.CommodityID, "2026-07-30", 800)
	reversed := pil(f.holding.ID, f.instrument.CommodityID, "2026-07-29", 900)
	_, err = f.investmentSvc.ReversePaymentInLieu(ctx, ReverseInvestmentPaymentInLieuInput{OwnerUserID: 1, TransactionID: reversed.ID, Reason: "entered twice"})
	require.NoError(t, err)
	// A cash dividend of the instrument is never this short's charge.
	dividend, err := f.investmentSvc.Dividend(ctx, DividendInput{OwnerUserID: 1, TransactionDate: "2026-08-14", CommodityID: NullableValue(f.instrument.CommodityID),
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, IncomeAccountID: &f.categoryAccountID, AmountValue: 800, AmountScale: 2})
	require.NoError(t, err)
	// Another instrument's payment in lieu on the same ex-date is not this one.
	other, err := f.investmentSvc.CreateInstrument(ctx, InvestmentInstrumentInput{OwnerUserID: 1, OriginType: "browser_api", ChangeReason: "fixture",
		InstrumentType: "stock", DisplayName: "Other", CommodityCode: "OTHR", Symbol: "OTHR", MIC: "XAMS", QuantityScale: 4, PriceScale: 4,
		QuoteCommodityID: &f.eurCommodityID})
	require.NoError(t, err)
	otherHolding, err := f.investmentSvc.CreateHoldingAccount(ctx, HoldingAccountInput{OwnerUserID: 1, InstrumentID: other.ID, Name: "Other", OpenedOn: "2020-01-01", EffectiveFrom: "2020-01-01"})
	require.NoError(t, err)
	_, err = f.investmentSvc.ShortSale(ctx, InvestmentTradeInput{OwnerUserID: 1, TransactionDate: "2026-06-02", CommodityID: other.CommodityID, HoldingAccountID: otherHolding.ID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, QuantityValue: exact.MustParse("5"), CashAmountValue: 300000, CashAmountScale: 2})
	require.NoError(t, err)
	wrongInstrument := pil(otherHolding.ID, other.CommodityID, "2026-07-29", 800)

	f.fetch(t)
	suggestions := f.suggestions(t)
	july := suggestionForExDate(t, suggestions, "2026-07-29")
	require.Equal(t, []any{float64(f.holding.ID)}, mustProposal(t, july)["holding_account_ids"])
	require.Len(t, july.LinkCandidates, 1)
	candidate := july.LinkCandidates[0]
	assert.Equal(t, manual.ID, candidate.TransactionID)
	assert.Equal(t, "2026-08-14", candidate.TransactionDate)
	assert.Equal(t, []any{exact.MustParse("8"), 0, "EUR"}, []any{candidate.GrossValue, candidate.GrossScale, candidate.CashCode}, "the amount owed, positive, before any recovery")
	require.NotNil(t, candidate.HoldingAccountID)
	assert.Equal(t, []any{f.holding.ID, "ASML"}, []any{*candidate.HoldingAccountID, candidate.HoldingAccountName})
	for _, s := range suggestions {
		for _, c := range s.LinkCandidates {
			if s.ID != july.ID {
				assert.Nil(t, c.HoldingAccountID, "a dividend suggestion is never offered a payment in lieu")
				assert.NotEqual(t, manual.ID, c.TransactionID)
			}
		}
	}

	// A holding that went short after the fetch is not in the proposal.
	secondHolding, err := f.investmentSvc.CreateHoldingAccount(ctx, HoldingAccountInput{OwnerUserID: 1, InstrumentID: f.instrument.ID, Name: "ASML 2", OpenedOn: "2020-01-01", EffectiveFrom: "2020-01-01"})
	require.NoError(t, err)
	_, err = f.investmentSvc.ShortSale(ctx, InvestmentTradeInput{OwnerUserID: 1, TransactionDate: "2026-06-03", CommodityID: f.instrument.CommodityID, HoldingAccountID: secondHolding.ID,
		CashAccountID: f.cashAccountID, CashCommodityID: f.eurCommodityID, QuantityValue: exact.MustParse("2"), CashAmountValue: 120000, CashAmountScale: 2})
	require.NoError(t, err)
	wrongHolding := pil(secondHolding.ID, f.instrument.CommodityID, "2026-07-29", 320)
	july = suggestionForExDate(t, f.suggestions(t), "2026-07-29")
	require.Len(t, july.LinkCandidates, 1)

	before := countTransactions(t, f.database)
	for name, transactionID := range map[string]int64{"wrong ex-date": wrongExDate.ID, "wrong instrument": wrongInstrument.ID,
		"wrong holding": wrongHolding.ID, "reversed": reversed.ID, "cash dividend": dividend.ID} {
		_, err = f.investmentSvc.LinkSuggestion(ctx, 1, 0, "req", july.ID, transactionID)
		var validation ValidationError
		require.ErrorAs(t, err, &validation, name)
	}
	assert.Equal(t, "suggested", suggestionForExDate(t, f.suggestions(t), "2026-07-29").Status)

	linked, err := f.investmentSvc.LinkSuggestion(ctx, 1, 0, "req", july.ID, manual.ID)
	require.NoError(t, err)
	assert.Equal(t, []any{"linked", "matched"}, []any{linked.Status, linked.EventStatus})
	assert.Equal(t, &manual.ID, linked.LinkedTransactionID)
	assert.Equal(t, before, countTransactions(t, f.database), "linking posts nothing")

	// The migration 0011 guard admits one provider event per payment.
	april := suggestionForExDate(t, f.suggestions(t), "2026-04-28")
	_, err = f.database.Exec(`UPDATE investment_event_suggestions SET linked_transaction_id = ?, status = 'linked' WHERE id = ?`, manual.ID, april.ID)
	require.Error(t, err)
	_, err = f.investmentSvc.LinkSuggestion(ctx, 1, 0, "req", july.ID, manual.ID)
	require.ErrorIs(t, err, ErrInvestmentSuggestionNotPending)
}

func mustProposal(t *testing.T, suggestion InvestmentEventSuggestion) map[string]any {
	t.Helper()
	var proposal map[string]any
	require.NoError(t, json.Unmarshal([]byte(suggestion.ProposedTransactionJSON), &proposal))
	return proposal
}
