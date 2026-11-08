package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rekenraam/backend/internal/exact"
)

// referenceBookFile is the provider-free reference book of #266, shared with
// e2e/playwright/support/reference-book.ts and the #260 sub-audits. Its
// register is hand-derived; a mismatch here is a finding about the app, never
// a reason to edit the register.
const referenceBookFile = "../../../fixtures/reference-book.json"

type referenceBook struct {
	Steps    []referenceBookStep   `json:"steps"`
	Register referenceBookRegister `json:"register"`
}

type referenceBookStep struct {
	Why       string                  `json:"why"`
	Method    string                  `json:"method"`
	Path      string                  `json:"path"`
	Status    int                     `json:"status"`
	Body      json.RawMessage         `json:"body"`
	Capture   map[string]string       `json:"capture"`
	Reconcile *referenceBookReconcile `json:"reconcile"`
}

type referenceBookReconcile struct {
	Account               string `json:"account"`
	Commodity             string `json:"commodity"`
	StatementDate         string `json:"statement_date"`
	StatementBalanceValue string `json:"statement_balance_value"`
	StatementBalanceScale int    `json:"statement_balance_scale"`
}

type referenceBookRegister struct {
	Balances        []referenceBalance `json:"balances"`
	SubtreeBalances []referenceBalance `json:"subtree_balances"`
	AccountStatus   []struct {
		Account string `json:"account"`
		Status  string `json:"status"`
	} `json:"account_status"`
	Reconciliation []struct {
		Account          string `json:"account"`
		Commodity        string `json:"commodity"`
		StatementDate    string `json:"statement_date"`
		StatementBalance string `json:"statement_balance"`
		Status           string `json:"status"`
	} `json:"reconciliation"`
	Drafts []struct {
		Description string `json:"description"`
	} `json:"drafts"`
	Period struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	} `json:"period"`
	Spending struct {
		Accounts []string                  `json:"accounts"`
		Expense  []referenceSpendingFigure `json:"expense"`
		Income   []referenceSpendingFigure `json:"income"`
	} `json:"spending"`
	Cashflow struct {
		Accounts []string `json:"accounts"`
		Rows     []struct {
			Commodity    string `json:"commodity"`
			Inflow       string `json:"inflow"`
			Outflow      string `json:"outflow"`
			OperatingNet string `json:"operating_net"`
			TransferIn   string `json:"transfer_in"`
			TransferOut  string `json:"transfer_out"`
			TransferNet  string `json:"transfer_net"`
			NetMovement  string `json:"net_movement"`
		} `json:"rows"`
	} `json:"cashflow"`
	Budget []struct {
		PeriodStart string `json:"period_start"`
		Category    string `json:"category"`
		Commodity   string `json:"commodity"`
		Target      string `json:"target"`
		Actual      string `json:"actual"`
		Remaining   string `json:"remaining"`
	} `json:"budget"`
	NetWorth []struct {
		AsOf   string `json:"as_of"`
		Totals []struct {
			Commodity string `json:"commodity"`
			Value     string `json:"value"`
		} `json:"totals"`
		ConvertedEUR *string `json:"converted_eur"`
		KnownDefect  string  `json:"known_defect"`
	} `json:"net_worth"`
	Positions []struct {
		Account        string  `json:"account"`
		Commodity      string  `json:"commodity"`
		Side           string  `json:"side"`
		Quantity       string  `json:"quantity"`
		BasisKnowledge string  `json:"basis_knowledge"`
		RemainingBasis *string `json:"remaining_basis"`
	} `json:"positions"`
	Lots []struct {
		Lot               string  `json:"lot"`
		Status            string  `json:"status"`
		RemainingQuantity string  `json:"remaining_quantity"`
		RemainingBasis    *string `json:"remaining_basis"`
		BasisKnowledge    string  `json:"basis_knowledge"`
	} `json:"lots"`
	Gains struct {
		ValuationAsOf string `json:"valuation_as_of"`
		Methods       []struct {
			Method   string `json:"method"`
			Realized []struct {
				Account      string `json:"account"`
				DisposalDate string `json:"disposal_date"`
				Basis        string `json:"basis"`
				Proceeds     string `json:"proceeds"`
				Gain         string `json:"gain"`
			} `json:"realized"`
			RealizedTotal string `json:"realized_total"`
			Unrealized    []struct {
				Account string  `json:"account"`
				Gain    *string `json:"gain"`
			} `json:"unrealized"`
		} `json:"methods"`
	} `json:"gains"`
}

type referenceBalance struct {
	AsOf      string `json:"as_of"`
	Account   string `json:"account"`
	Commodity string `json:"commodity"`
	Value     string `json:"value"`
	Why       string `json:"why"`
}

type referenceSpendingFigure struct {
	Category  string `json:"category"`
	Commodity string `json:"commodity"`
	Amount    string `json:"amount"`
}

// seededReferenceBook is what a seeding run produced. ids binds every key the
// book names to the id this run created. A restore keeps ids, so the restore
// drill (#277) can seed, back up, restore, sign in, and hand the restored
// handler and the same ids to assertReferenceBookRegister.
type seededReferenceBook struct {
	book          referenceBook
	sessionCookie *http.Cookie
	csrfToken     string
	ids           map[string]int64
}

func loadReferenceBook(t *testing.T) referenceBook {
	t.Helper()
	raw, err := os.ReadFile(referenceBookFile)
	require.NoError(t, err)
	var book referenceBook
	require.NoError(t, json.Unmarshal(raw, &book))
	require.NotEmpty(t, book.Steps)
	return book
}

// seedReferenceBook sets up a fresh owner and book (EUR default, USD added,
// system accounts) and replays every step through the public API.
func seedReferenceBook(t *testing.T, handler http.Handler) seededReferenceBook {
	t.Helper()

	book := loadReferenceBook(t)
	sessionCookie, csrfToken := createOwnerSession(t, handler)
	createBookForSession(t, handler, sessionCookie, csrfToken, "Reference book")
	setup := completeCurrencySetupForSession(t, handler, sessionCookie, csrfToken, "EUR", []setupCurrencySelectionRequest{{Code: "EUR", Name: "Euro"}})
	usd := createCurrencyForSession(t, handler, sessionCookie, csrfToken, `{"code":"USD","name":"US Dollar"}`)
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)

	seeded := seededReferenceBook{book: book, sessionCookie: sessionCookie, csrfToken: csrfToken, ids: map[string]int64{
		"EUR": setup.DefaultCurrency.ID,
		"USD": usd.ID,
	}}
	var balances accountBalancesResponse
	seeded.get(t, handler, "/api/v1/ledger/account-balances?include_system=true", &balances)
	for _, account := range balances.Accounts {
		if account.SystemRole != "" {
			seeded.ids["system."+account.SystemRole] = account.AccountID
		}
	}

	for _, step := range book.Steps {
		if step.Reconcile != nil {
			seeded.reconcile(t, handler, step)
			continue
		}
		path := seeded.resolvePath(t, step.Path)
		res := seeded.request(t, handler, step.Method, path, seeded.resolveBody(t, step.Body))
		require.Equal(t, step.Status, res.Code, "%s %s (%s): %s", step.Method, path, step.Why, res.Body.String())
		if len(step.Capture) == 0 {
			continue
		}
		decoder := json.NewDecoder(res.Body)
		decoder.UseNumber()
		var response any
		require.NoError(t, decoder.Decode(&response), step.Why)
		for key, field := range step.Capture {
			_, exists := seeded.ids[key]
			require.False(t, exists, "%s captures %s twice", step.Why, key)
			seeded.ids[key] = referenceCapture(t, response, field, step.Why)
		}
	}
	return seeded
}

func referenceCapture(t *testing.T, response any, field, why string) int64 {
	t.Helper()
	value := response
	for name := range strings.SplitSeq(field, ".") {
		object, ok := value.(map[string]any)
		require.True(t, ok, "%s: %s is not an object at %s", why, field, name)
		value = object[name]
	}
	number, ok := value.(json.Number)
	require.True(t, ok, "%s: %s is %v, not a number", why, field, value)
	id, err := number.Int64()
	require.NoError(t, err, why)
	return id
}

func (s seededReferenceBook) id(t *testing.T, key string) int64 {
	t.Helper()
	id, ok := s.ids[strings.TrimPrefix(key, "@")]
	require.True(t, ok, "reference book names unknown key %q", key)
	return id
}

func (s seededReferenceBook) resolvePath(t *testing.T, path string) string {
	t.Helper()
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, "@") {
			segments[i] = strconvFormatInt(s.id(t, segment))
		}
	}
	return strings.Join(segments, "/")
}

// resolveBody swaps every "@key" string for its id and drops "{suffix}": the
// Go reader owns a fresh database, so names need no run suffix.
func (s seededReferenceBook) resolveBody(t *testing.T, raw json.RawMessage) json.RawMessage {
	t.Helper()
	if len(raw) == 0 {
		return nil
	}
	var body any
	require.NoError(t, json.Unmarshal(raw, &body))
	var walk func(any) any
	walk = func(value any) any {
		switch typed := value.(type) {
		case map[string]any:
			for key, item := range typed {
				typed[key] = walk(item)
			}
		case []any:
			for i, item := range typed {
				typed[i] = walk(item)
			}
		case string:
			if strings.HasPrefix(typed, "@") {
				return s.id(t, typed)
			}
			return strings.ReplaceAll(typed, "{suffix}", "")
		}
		return value
	}
	encoded, err := json.Marshal(walk(body))
	require.NoError(t, err)
	return encoded
}

// request sends one API call without a status expectation, so a refused step
// reports which step and why.
func (s seededReferenceBook) request(t *testing.T, handler http.Handler, method, path string, body json.RawMessage) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set(csrfTokenHeader, s.csrfToken)
		setSameOrigin(req)
	}
	req.AddCookie(s.sessionCookie)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func (s seededReferenceBook) get(t *testing.T, handler http.Handler, path string, into any) {
	t.Helper()
	res := s.request(t, handler, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, res.Code, "GET %s: %s", path, res.Body.String())
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), into), path)
}

// reconcile starts a statement session, selects every candidate dated on or
// before the statement date, and finishes it — the checkpoint must then hold.
func (s seededReferenceBook) reconcile(t *testing.T, handler http.Handler, step referenceBookStep) {
	t.Helper()
	spec := step.Reconcile
	accountID := s.id(t, spec.Account)
	res := s.request(t, handler, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(accountID)+"/reconciliations/start", mustRawJSON(t, map[string]any{
		"commodity_id": s.id(t, spec.Commodity), "source_kind": "statement", "statement_date": spec.StatementDate,
		"statement_balance_value": spec.StatementBalanceValue, "statement_balance_scale": spec.StatementBalanceScale,
	}))
	require.Equal(t, http.StatusCreated, res.Code, "%s: %s", step.Why, res.Body.String())
	var session reconciliationSessionResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &session))
	selected := []int64{}
	for _, candidate := range session.Candidates {
		if candidate.EntryDate <= spec.StatementDate {
			selected = append(selected, candidate.PostingID)
		}
	}
	base := "/api/v1/reconciliations/" + strconvFormatInt(session.ID)
	res = s.request(t, handler, http.MethodPatch, base, mustRawJSON(t, map[string]any{"posting_version_ids": selected, "change_reason": "reference book"}))
	require.Equal(t, http.StatusOK, res.Code, "%s: %s", step.Why, res.Body.String())
	res = s.request(t, handler, http.MethodPost, base+"/finish", json.RawMessage(`{"change_reason":"reference book"}`))
	require.Equal(t, http.StatusOK, res.Code, "%s: %s", step.Why, res.Body.String())
}

func mustRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

// exactValue renders an exact decimal as a reduced rational, so 300, 300.00
// and 300.0000 compare equal and nothing passes through a float.
func exactValue(t *testing.T, decimal string) string {
	t.Helper()
	value, ok := new(big.Rat).SetString(decimal)
	require.True(t, ok, "not an exact decimal: %q", decimal)
	return value.RatString()
}

func exactCoefficientValue(t *testing.T, value exact.Coefficient, scale int) string {
	t.Helper()
	return exactValue(t, exact.Decimal(value, scale))
}

// optionalMoneyValue renders an absent figure as "null": unknown is not zero.
func optionalMoneyValue(t *testing.T, value *moneyCoefficient, scale *int) string {
	t.Helper()
	if value == nil {
		return "null"
	}
	require.NotNil(t, scale)
	return exactValue(t, exact.Decimal(exact.New(int64(*value)), *scale))
}

func optionalExpected(t *testing.T, decimal *string) string {
	t.Helper()
	if decimal == nil {
		return "null"
	}
	return exactValue(t, *decimal)
}

// quantityOf is one commodity's exact figure in a balance list; absent is zero.
func (s seededReferenceBook) quantityOf(t *testing.T, quantities []balanceQuantityResponse, commodity string) string {
	t.Helper()
	commodityID := s.id(t, commodity)
	for _, quantity := range quantities {
		if quantity.CommodityID == commodityID {
			return exactCoefficientValue(t, quantity.QuantityValue, quantity.QuantityScale)
		}
	}
	return "0"
}

// keyAmong names id by the one key among candidates bound to it. Ids repeat
// across tables, so a reverse lookup must say which keys it means.
func (s seededReferenceBook) keyAmong(id int64, candidates []string) string {
	for _, key := range candidates {
		if s.ids[key] == id {
			return key
		}
	}
	return "unexpected id " + strconvFormatInt(id)
}

var referenceBookCommodities = []string{"EUR", "USD", "REF", "SHRT"}

func referenceSpendingCategories(figures ...[]referenceSpendingFigure) []string {
	keys := []string{}
	for _, list := range figures {
		for _, figure := range list {
			keys = append(keys, figure.Category)
		}
	}
	return keys
}

// assertReferenceBookRegister checks every register figure against handler's
// current book. It reads only, so it is also the restore drill's comparison
// entry point: the same seeded ids against the restored handler.
func assertReferenceBookRegister(t *testing.T, handler http.Handler, s seededReferenceBook) {
	t.Helper()
	register := s.book.Register

	balancesAt := map[string]accountBalancesResponse{}
	balancesOn := func(asOf string) accountBalancesResponse {
		if cached, ok := balancesAt[asOf]; ok {
			return cached
		}
		var response accountBalancesResponse
		path := "/api/v1/ledger/account-balances?include_system=true"
		if asOf != "" {
			path += "&as_of=" + asOf
		}
		s.get(t, handler, path, &response)
		balancesAt[asOf] = response
		return response
	}
	accountOn := func(asOf, key string) accountBalanceResponse {
		accountID := s.id(t, key)
		for _, account := range balancesOn(asOf).Accounts {
			if account.AccountID == accountID {
				return account
			}
		}
		t.Fatalf("account-balances on %s has no %s", asOf, key)
		return accountBalanceResponse{}
	}
	for _, figure := range register.Balances {
		account := accountOn(figure.AsOf, figure.Account)
		assert.Equal(t, exactValue(t, figure.Value), s.quantityOf(t, account.DirectBalances, figure.Commodity),
			"balance %s %s on %s (%s)", figure.Account, figure.Commodity, figure.AsOf, figure.Why)
	}
	for _, figure := range register.SubtreeBalances {
		account := accountOn(figure.AsOf, figure.Account)
		assert.Equal(t, exactValue(t, figure.Value), s.quantityOf(t, account.SubtreeBalances, figure.Commodity),
			"subtree balance %s %s on %s (%s)", figure.Account, figure.Commodity, figure.AsOf, figure.Why)
	}
	for _, figure := range register.AccountStatus {
		assert.Equal(t, figure.Status, accountOn(register.Period.EndDate, figure.Account).Status, "status of %s", figure.Account)
	}

	for _, figure := range register.Reconciliation {
		var checkpoints reconciliationCheckpointsResponse
		s.get(t, handler, "/api/v1/accounts/"+strconvFormatInt(s.id(t, figure.Account))+"/reconciliation-checkpoints", &checkpoints)
		if assert.Len(t, checkpoints.Checkpoints, 1, "checkpoints of %s", figure.Account) {
			checkpoint := checkpoints.Checkpoints[0]
			assert.Equal(t, s.id(t, figure.Commodity), checkpoint.CommodityID)
			assert.Equal(t, figure.StatementDate, checkpoint.StatementDate)
			assert.Equal(t, figure.Status, checkpoint.Status)
			assert.Equal(t, exactValue(t, figure.StatementBalance),
				exactCoefficientValue(t, checkpoint.StatementBalance.QuantityValue, checkpoint.StatementBalance.QuantityScale), "statement balance")
		}
	}

	var drafts transactionsResponse
	s.get(t, handler, "/api/v1/transactions?status=draft", &drafts)
	gotDrafts := []string{}
	for _, draft := range drafts.Transactions {
		gotDrafts = append(gotDrafts, draft.Description)
	}
	wantDrafts := []string{}
	for _, draft := range register.Drafts {
		wantDrafts = append(wantDrafts, draft.Description)
	}
	assert.Equal(t, wantDrafts, gotDrafts, "drafts")

	s.assertSpending(t, handler)
	s.assertCashflow(t, handler)
	s.assertBudget(t, handler)
	s.assertNetWorth(t, handler)
	s.assertInvestments(t, handler)
}

func (s seededReferenceBook) reportQuery(t *testing.T, accounts []string, extra url.Values) string {
	t.Helper()
	values := url.Values{
		"start_date":          {s.book.Register.Period.StartDate},
		"end_date":            {s.book.Register.Period.EndDate},
		"include_descendants": {"false"},
	}
	for _, key := range accounts {
		values.Add("account_id", strconvFormatInt(s.id(t, key)))
	}
	maps.Copy(values, extra)
	return "?" + values.Encode()
}

func (s seededReferenceBook) assertSpending(t *testing.T, handler http.Handler) {
	t.Helper()
	spending := s.book.Register.Spending
	categories := referenceSpendingCategories(spending.Expense, spending.Income)
	for _, mode := range []string{"expense", "income"} {
		want, query := spending.Expense, url.Values{"group_by": {"category"}}
		if mode == "income" {
			want = spending.Income
			query.Set("mode", "income")
		}
		report := readSpendingForSession(t, handler, s.sessionCookie, s.reportQuery(t, spending.Accounts, query))
		got := []string{}
		for _, group := range report.Groups {
			require.NotNil(t, group.CategoryID, "spending groups by category")
			for _, total := range group.Totals {
				got = append(got, fmt.Sprintf("%s %s %s", s.keyAmong(*group.CategoryID, categories), s.keyAmong(total.CommodityID, referenceBookCommodities), exactCoefficientValue(t, total.QuantityValue, total.QuantityScale)))
			}
		}
		expected := []string{}
		for _, figure := range want {
			expected = append(expected, fmt.Sprintf("%s %s %s", figure.Category, figure.Commodity, exactValue(t, figure.Amount)))
		}
		sort.Strings(got)
		sort.Strings(expected)
		assert.Equal(t, expected, got, "%s spending", mode)
	}
}

func (s seededReferenceBook) assertCashflow(t *testing.T, handler http.Handler) {
	t.Helper()
	cashflow := s.book.Register.Cashflow
	report := readCashflowForSession(t, handler, s.sessionCookie, s.reportQuery(t, cashflow.Accounts, url.Values{"bucket": {"quarter"}}))
	require.Len(t, report.Buckets, 1, "the period is one quarter")
	bucket := report.Buckets[0]
	measured := map[string]bool{}
	for _, row := range cashflow.Rows {
		measured[row.Commodity] = true
		for _, measure := range []struct {
			name, want string
			got        []balanceQuantityResponse
		}{
			{"inflow", row.Inflow, bucket.Inflow}, {"outflow", row.Outflow, bucket.Outflow},
			{"operating_net", row.OperatingNet, bucket.OperatingNet}, {"transfer_in", row.TransferIn, bucket.TransferIn},
			{"transfer_out", row.TransferOut, bucket.TransferOut}, {"transfer_net", row.TransferNet, bucket.TransferNet},
			{"net_movement", row.NetMovement, bucket.NetMovement},
		} {
			assert.Equal(t, exactValue(t, measure.want), s.quantityOf(t, measure.got, row.Commodity), "cashflow %s %s", row.Commodity, measure.name)
		}
	}
	for _, measure := range [][]balanceQuantityResponse{bucket.Inflow, bucket.Outflow, bucket.TransferIn, bucket.TransferOut, bucket.NetMovement} {
		for _, quantity := range measure {
			assert.True(t, measured[s.keyAmong(quantity.CommodityID, referenceBookCommodities)], "cashflow carries unexpected commodity %d", quantity.CommodityID)
		}
	}
}

func (s seededReferenceBook) assertBudget(t *testing.T, handler http.Handler) {
	t.Helper()
	for _, figure := range s.book.Register.Budget {
		var month budgetMonthResponse
		s.get(t, handler, "/api/v1/budgets/month?period_start="+figure.PeriodStart, &month)
		categoryID, commodityID := s.id(t, figure.Category), s.id(t, figure.Commodity)
		found := false
		for _, category := range month.Categories {
			if category.ID != categoryID {
				continue
			}
			for _, amount := range category.Amounts {
				if amount.CommodityID != commodityID {
					continue
				}
				found = true
				label := fmt.Sprintf("budget %s %s", figure.PeriodStart, figure.Category)
				assert.Equal(t, exactValue(t, figure.Target), exactCoefficientValue(t, amount.Target.QuantityValue, amount.Target.QuantityScale), label+" target")
				assert.Equal(t, exactValue(t, figure.Actual), exactCoefficientValue(t, amount.Actual.QuantityValue, amount.Actual.QuantityScale), label+" actual")
				assert.Equal(t, exactValue(t, figure.Remaining), exactCoefficientValue(t, amount.Remaining.QuantityValue, amount.Remaining.QuantityScale), label+" remaining")
			}
		}
		assert.True(t, found, "budget %s has no %s %s row", figure.PeriodStart, figure.Category, figure.Commodity)
	}
}

func (s seededReferenceBook) assertNetWorth(t *testing.T, handler http.Handler) {
	t.Helper()
	var series netWorthSeriesResponse
	s.get(t, handler, "/api/v1/reports/net-worth"+s.reportQuery(t, nil, url.Values{
		"bucket": {"month"}, "reporting_currency_id": {strconvFormatInt(s.id(t, "EUR"))},
	}), &series)
	byDate := map[string]netWorthSeriesBucketResponse{}
	for _, bucket := range series.Buckets {
		byDate[bucket.EndDate] = bucket
	}
	for _, figure := range s.book.Register.NetWorth {
		bucket, ok := byDate[figure.AsOf]
		if !assert.True(t, ok, "net worth has no bucket ending %s", figure.AsOf) {
			continue
		}
		want, got := []string{}, []string{}
		for _, total := range figure.Totals {
			want = append(want, total.Commodity+" "+exactValue(t, total.Value))
		}
		for _, total := range bucket.Totals {
			got = append(got, s.keyAmong(total.CommodityID, referenceBookCommodities)+" "+exactCoefficientValue(t, total.QuantityValue, total.QuantityScale))
		}
		sort.Strings(want)
		sort.Strings(got)
		assert.Equal(t, want, got, "net worth totals on %s", figure.AsOf)
		converted := "null"
		if bucket.Converted != nil {
			assert.Equal(t, s.id(t, "EUR"), bucket.Converted.CommodityID)
			converted = exactCoefficientValue(t, bucket.Converted.QuantityValue, bucket.Converted.QuantityScale)
		}
		label := fmt.Sprintf("net worth in EUR on %s", figure.AsOf)
		if figure.KnownDefect != "" {
			// The register stays right; the reader insists the app is still
			// wrong, so the fix is what removes the marker.
			assert.NotEqual(t, optionalExpected(t, figure.ConvertedEUR), converted, "%s: %s looks fixed; remove its known_defect marker", label, figure.KnownDefect)
			t.Logf("%s: known defect %s (expected %s, got %s)", label, figure.KnownDefect, optionalExpected(t, figure.ConvertedEUR), converted)
			continue
		}
		assert.Equal(t, optionalExpected(t, figure.ConvertedEUR), converted, "%s; valuation %s", label, mustJSON(t, series.Valuation))
	}
}

func (s seededReferenceBook) assertInvestments(t *testing.T, handler http.Handler) {
	t.Helper()
	register := s.book.Register

	var positions investmentPositionsResponse
	s.get(t, handler, "/api/v1/investments/positions", &positions)
	for _, figure := range register.Positions {
		accountID, commodityID := s.id(t, figure.Account), s.id(t, figure.Commodity)
		found := false
		for _, position := range positions.Positions {
			if position.AccountID != accountID || position.CommodityID != commodityID {
				continue
			}
			found = true
			assert.Equal(t, figure.Side, position.PositionSide, "position side %s", figure.Account)
			assert.Equal(t, exactValue(t, figure.Quantity), exactCoefficientValue(t, position.QuantityValue, position.QuantityScale), "position quantity %s", figure.Account)
			assert.Equal(t, figure.BasisKnowledge, position.BasisKnowledge, "position basis knowledge %s", figure.Account)
			assert.Equal(t, optionalExpected(t, figure.RemainingBasis), optionalMoneyValue(t, position.RemainingCostBasisValue, position.RemainingCostBasisScale), "position basis %s", figure.Account)
		}
		assert.True(t, found, "no position for %s", figure.Account)
	}

	var lots investmentLotsResponse
	s.get(t, handler, "/api/v1/investments/lots", &lots)
	for _, figure := range register.Lots {
		lotID := s.id(t, figure.Lot)
		found := false
		for _, lot := range lots.Lots {
			if lot.ID != lotID {
				continue
			}
			found = true
			assert.Equal(t, figure.Status, lot.Status, "lot status %s", figure.Lot)
			assert.Equal(t, exactValue(t, figure.RemainingQuantity), exactCoefficientValue(t, lot.RemainingQuantityValue, lot.RemainingQuantityScale), "lot quantity %s", figure.Lot)
			assert.Equal(t, figure.BasisKnowledge, lot.BasisKnowledge, "lot basis knowledge %s", figure.Lot)
			assert.Equal(t, optionalExpected(t, figure.RemainingBasis), optionalMoneyValue(t, lot.RemainingCostBasisValue, lot.RemainingCostBasisScale), "lot basis %s", figure.Lot)
		}
		assert.True(t, found, "no lot %s", figure.Lot)
	}

	for _, method := range register.Gains.Methods {
		query := url.Values{"valuation_as_of": {register.Gains.ValuationAsOf}}
		if method.Method != "operational" {
			query.Set("alternative_method", method.Method)
		}
		res := s.request(t, handler, http.MethodGet, "/api/v1/investments/gains?"+query.Encode(), nil)
		if !assert.Equal(t, http.StatusOK, res.Code, "%s gains: %s", method.Method, res.Body.String()) {
			continue
		}
		var gains investmentGainsResponse
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &gains))
		realized, unrealized, totals := gains.Realized, gains.Unrealized, gains.RealizedTotals
		if method.Method != "operational" {
			if !assert.NotNil(t, gains.Alternative, "%s alternative", method.Method) {
				continue
			}
			realized, unrealized, totals = gains.Alternative.Realized, gains.Alternative.Unrealized, gains.Alternative.RealizedTotals
		}

		assert.Len(t, realized, len(method.Realized), "%s realized entries", method.Method)
		for _, figure := range method.Realized {
			accountID := s.id(t, figure.Account)
			found := false
			for _, entry := range realized {
				if entry.AccountID != accountID || entry.DisposalDate != figure.DisposalDate {
					continue
				}
				found = true
				label := fmt.Sprintf("%s realized %s", method.Method, figure.Account)
				assert.Equal(t, s.id(t, "USD"), entry.CostCommodityID, label+" currency")
				assert.Equal(t, optionalExpected(t, &figure.Gain), optionalMoneyValue(t, entry.RealizedGainValue, entry.RealizedGainScale), label+" gain")
				if figure.Basis != "" {
					assert.Equal(t, optionalExpected(t, &figure.Basis), optionalMoneyValue(t, entry.DisposedBasisValue, entry.DisposedBasisScale), label+" basis")
				}
				if figure.Proceeds != "" {
					assert.Equal(t, exactValue(t, figure.Proceeds), exactCoefficientValue(t, exact.New(int64(entry.ProceedsValue)), entry.ProceedsScale), label+" proceeds")
				}
			}
			assert.True(t, found, "%s has no realized entry for %s on %s", method.Method, figure.Account, figure.DisposalDate)
		}
		if assert.Len(t, totals, 1, "%s realized totals", method.Method) {
			assert.Equal(t, s.id(t, "USD"), totals[0].CostCommodityID)
			assert.Equal(t, optionalExpected(t, &method.RealizedTotal), optionalMoneyValue(t, totals[0].TotalGainValue, totals[0].TotalGainScale), "%s realized total", method.Method)
		}

		assert.Len(t, unrealized, len(method.Unrealized), "%s unrealized entries", method.Method)
		for _, figure := range method.Unrealized {
			accountID := s.id(t, figure.Account)
			found := false
			for _, entry := range unrealized {
				if entry.AccountID != accountID {
					continue
				}
				found = true
				assert.Equal(t, optionalExpected(t, figure.Gain), optionalMoneyValue(t, entry.UnrealizedGainValue, entry.UnrealizedGainScale), "%s unrealized %s", method.Method, figure.Account)
			}
			assert.True(t, found, "%s has no unrealized entry for %s", method.Method, figure.Account)
		}
	}
}

// The reference book seeds through the public API and produces every figure
// in its hand-derived register (#266).
func TestReferenceBookProducesItsRegister(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	seeded := seedReferenceBook(t, handler)
	assertReferenceBookRegister(t, handler, seeded)
}
