package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// TestBuildReleaseSeedFixture is a tool, not a test: it builds a realistic book
// through the real HTTP stack and vacuums the result to SEED_OUT, which
// scripts/build-release-seed.sh then shapes into the frozen SQL fixture that
// internal/db's upgrade test replays.
//
// It runs only when SEED_OUT is set, so the ordinary suite skips it. Building
// the book through the API rather than with hand-written INSERTs is the point:
// every invariant the app enforces is enforced here too, so the fixture is a
// book the app would actually have written, not one a test author imagined.
//
// Adding a seed for a later release means extending this and writing a NEW
// fixture file. The existing ones are frozen — see the header in
// internal/db/testdata/v01_seed.sql.
func TestBuildReleaseSeedFixture(t *testing.T) {
	out := os.Getenv("SEED_OUT")
	if out == "" {
		t.Skip("SEED_OUT is unset; run scripts/build-release-seed.sh to regenerate the fixture")
	}
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	session, csrf := f.sessionCookie, f.csrfToken

	// --- Structure -------------------------------------------------------
	savings := createLedgerAccount(t, handler, session, csrf, "Savings", "asset", "savings", f.commodityID, 2)
	card := createLedgerAccount(t, handler, session, csrf, "Credit Card", "liability", "credit_card", f.commodityID, 2)
	// Equity accounts are system-managed; the setup wizard's system-accounts
	// step already made the opening_balance one.
	var equityID int64
	require.NoError(t, database.QueryRowContext(context.Background(),
		`SELECT id FROM accounts WHERE system_role = 'opening_balance'`).Scan(&equityID))
	groceries := createCategoryForSession(t, handler, session, csrf, `{"name":"Groceries","category_type":"expense"}`)
	salary := createCategoryForSession(t, handler, session, csrf, `{"name":"Salary","category_type":"income"}`)
	payee := createPayeeForSession(t, handler, session, csrf, `{"name":"Corner Shop"}`)
	tag := createTagForSession(t, handler, session, csrf, `{"name":"reviewed"}`)

	// --- Ordinary ledger activity ---------------------------------------
	opening := createTransactionForSession(t, handler, session, csrf, `{
		"transaction_date":"2026-01-01","description":"opening balance",
		"journal_entries":[{"entry_date":"2026-01-01","postings":[
			`+posting(f.cashAccount.ID, 250000, 2, f.commodityID)+`,
			`+posting(equityID, -250000, 2, f.commodityID)+`]}]}`, http.StatusCreated)

	paycheque := createTransactionForSession(t, handler, session, csrf, `{
		"transaction_date":"2026-01-05","description":"January salary",
		"payee_id":`+strconvFormatInt(payee.ID)+`,
		"tag_ids":[`+strconvFormatInt(tag.ID)+`],
		"journal_entries":[{"entry_date":"2026-01-05","postings":[
			`+posting(f.cashAccount.ID, 320000, 2, f.commodityID)+`,
			`+posting(salary.ID, -320000, 2, f.commodityID)+`]}]}`, http.StatusCreated)

	// A split across two categories plus a transfer leg, carrying a tag that
	// nothing later strips.
	createTransactionForSession(t, handler, session, csrf, `{
		"transaction_date":"2026-01-12","description":"shopping split",
		"payee_id":`+strconvFormatInt(payee.ID)+`,
		"tag_ids":[`+strconvFormatInt(tag.ID)+`],
		"journal_entries":[{"entry_date":"2026-01-12","postings":[
			`+posting(groceries.ID, 4500, 2, f.commodityID)+`,
			`+posting(f.incomeAccount.ID, -1500, 2, f.commodityID)+`,
			`+posting(card.ID, -3000, 2, f.commodityID)+`]}]}`, http.StatusCreated)

	createTransactionForSession(t, handler, session, csrf, `{
		"transaction_date":"2026-01-20","description":"transfer to savings",
		"journal_entries":[{"entry_date":"2026-01-20","postings":[
			`+posting(savings.ID, 50000, 2, f.commodityID)+`,
			`+posting(f.cashAccount.ID, -50000, 2, f.commodityID)+`]}]}`, http.StatusCreated)

	// --- Lifecycle states ------------------------------------------------
	voided := createTransactionForSession(t, handler, session, csrf, `{
		"transaction_date":"2026-01-22","description":"duplicate, voided",
		"journal_entries":[{"entry_date":"2026-01-22","postings":[
			`+posting(groceries.ID, 900, 2, f.commodityID)+`,
			`+posting(card.ID, -900, 2, f.commodityID)+`]}]}`, http.StatusCreated)
	doInvestmentRequest(t, handler, session, csrf, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(voided.ID)+"/void",
		map[string]string{"change_reason": "entered twice"}, http.StatusOK)

	deleted := createTransactionForSession(t, handler, session, csrf, `{
		"transaction_date":"2026-01-23","description":"mistake, soft-deleted",
		"journal_entries":[{"entry_date":"2026-01-23","postings":[
			`+posting(groceries.ID, 700, 2, f.commodityID)+`,
			`+posting(card.ID, -700, 2, f.commodityID)+`]}]}`, http.StatusCreated)
	doInvestmentRequest(t, handler, session, csrf, http.MethodPost,
		"/api/v1/transactions/"+strconvFormatInt(deleted.ID)+"/soft-delete",
		map[string]string{"change_reason": "wrong account"}, http.StatusOK)

	// An edit, so at least one transaction carries a superseded version.
	doInvestmentRequest(t, handler, session, csrf, http.MethodPatch,
		"/api/v1/transactions/"+strconvFormatInt(paycheque.ID), json.RawMessage(`{
			"transaction_date":"2026-01-05","description":"January salary (corrected)",
			"change_reason":"payee was wrong",
			"journal_entries":[{"entry_date":"2026-01-05","postings":[
				`+posting(f.cashAccount.ID, 320000, 2, f.commodityID)+`,
				`+posting(salary.ID, -320000, 2, f.commodityID)+`]}]}`), http.StatusOK)

	// --- Reconciliation --------------------------------------------------
	openingPosting := postingByAccount(t, opening, f.cashAccount.ID)
	reconcilePostingForSession(t, handler, session, csrf, f.cashAccount.ID, f.commodityID, openingPosting, "2026-01-02")

	// --- Investments -----------------------------------------------------
	instrument := createInstrumentForSession(t, handler, f, "SEEDCO")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)

	buyOne := tradeRequestBody(f, holding.ID, instrument.CommodityID, "10", 100000)
	buyOne.TransactionDate = "2026-02-02"
	doInvestmentRequest(t, handler, session, csrf, http.MethodPost, "/api/v1/investments/buy", buyOne, http.StatusCreated)

	buyTwo := tradeRequestBody(f, holding.ID, instrument.CommodityID, "5", 60000)
	buyTwo.TransactionDate = "2026-03-02"
	doInvestmentRequest(t, handler, session, csrf, http.MethodPost, "/api/v1/investments/buy", buyTwo, http.StatusCreated)

	// A fractional partial sale: closes one lot, leaves another open, and
	// records a disposal decision with its allocations.
	sell := investmentTradeRequest{
		TransactionDate: "2026-04-02", CommodityID: instrument.CommodityID, HoldingAccountID: holding.ID,
		CashAccountID: f.cashAccount.ID, QuantityValue: exact.New(1250), QuantityScale: 2,
		CashAmountValue: 150000, CashAmountScale: 2, CashCommodityID: f.commodityID,
	}
	doInvestmentRequest(t, handler, session, csrf, http.MethodPost, "/api/v1/investments/sell", sell, http.StatusCreated)

	doInvestmentRequest(t, handler, session, csrf, http.MethodPost, "/api/v1/investments/dividend", dividendRequest{
		TransactionDate: "2026-05-02", CommodityID: &instrument.CommodityID,
		CashAccountID: f.cashAccount.ID, CashCommodityID: f.commodityID,
		IncomeAccountID: &f.incomeAccount.ID, AmountValue: 1234, AmountScale: 2,
	}, http.StatusCreated)

	// --- Settings, policies, and scheduled work --------------------------
	institution := createInstitutionForSession(t, handler, session, csrf, `{"name":"Seed Bank","country_code":"NL"}`)
	_ = institution

	doInvestmentRequest(t, handler, session, csrf, http.MethodPost, "/api/v1/investments/cost-basis-profiles",
		costBasisProfileRequest{Name: "Seed Average Cost", Method: "average_cost", Status: "active", Metadata: json.RawMessage(`{}`)},
		http.StatusCreated)

	doInvestmentRequest(t, handler, session, csrf, http.MethodPut,
		"/api/v1/budgets/targets/"+strconvFormatInt(groceries.ID)+"/"+strconvFormatInt(f.commodityID),
		json.RawMessage(`{"period_start":"2026-01-01","quantity_value":"40000","quantity_scale":2,"change_reason":"seed budget"}`),
		http.StatusOK)

	recurringChecking := createLedgerAccount(t, handler, session, csrf, "Recurring checking", "asset", "checking", f.commodityID, 2)
	rent := createCategoryForSession(t, handler, session, csrf, `{"name":"Rent","category_type":"expense"}`)
	template := map[string]any{
		"name": "Rent", "frequency": "monthly", "day_of_month": 1, "starts_on": "2026-01-01",
		"description": "Monthly rent", "lead_days": 7, "payee_id": payee.ID, "tag_ids": []int64{tag.ID},
		"postings": []map[string]any{
			{"account_id": recurringChecking.ID, "commodity_id": f.commodityID, "quantity_value": "-120000", "quantity_scale": 2},
			{"account_id": rent.ID, "commodity_id": f.commodityID, "quantity_value": "120000", "quantity_scale": 2},
		},
	}
	createdTemplate := doInvestmentRequest(t, handler, session, csrf, http.MethodPost, "/api/v1/recurring/templates", template, http.StatusCreated)
	var templateResponse struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createdTemplate.Body).Decode(&templateResponse))
	doInvestmentRequest(t, handler, session, csrf, http.MethodPost,
		"/api/v1/recurring/templates/"+strconvFormatInt(templateResponse.ID)+"/run-now", json.RawMessage(`{}`), http.StatusOK)

	// A self-check run, so its results table carries rows too.
	doInvestmentRequest(t, handler, session, csrf, http.MethodPost, "/api/v1/maintenance/self-check", json.RawMessage(`{}`), http.StatusOK)

	// --- Dump ------------------------------------------------------------
	_, err := database.ExecContext(context.Background(), `VACUUM INTO ?`, out)
	require.NoError(t, err)
	t.Logf("seed written to %s", out)
}
