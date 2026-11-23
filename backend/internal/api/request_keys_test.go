package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Create request keys (#660): a create retried after a lost response returns
// the first result instead of writing again.

func sendKeyed(t *testing.T, handler http.Handler, cookie *http.Cookie, csrfToken, path, body, key string, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(idempotencyKeyHeader, key)
	}
	if csrfToken != "" {
		req.Header.Set(csrfTokenHeader, csrfToken)
		setSameOrigin(req)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	require.Equalf(t, wantStatus, res.Code, "response body: %s", res.Body.String())
	return res
}

func countRows(t *testing.T, database *sql.DB, query string) int {
	t.Helper()
	var count int
	require.NoError(t, database.QueryRow(query).Scan(&count))
	return count
}

func errorCodeOf(t *testing.T, res *httptest.ResponseRecorder) string {
	t.Helper()
	var body errorResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	return body.Error.Code
}

func requestKeyTransactionBody(f investmentAPITestFixture, description string) string {
	return fmt.Sprintf(`{"transaction_date":"2026-02-01","description":%q,"journal_entries":[{"entry_date":"2026-02-01","postings":[
		{"account_id":%d,"quantity_value":"1000","quantity_scale":2,"commodity_id":%d},
		{"account_id":%d,"quantity_value":"-1000","quantity_scale":2,"commodity_id":%d}]}]}`,
		description, f.cashAccount.ID, f.commodityID, f.incomeAccount.ID, f.commodityID)
}

const countTransactions = `SELECT count(*) FROM transactions`

func TestCreateTransactionRepeatedWithSameKeyReturnsTheFirstResult(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	before := countRows(t, database, countTransactions)
	body := requestKeyTransactionBody(f, "Lost response")

	first := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", body, "entry-1", http.StatusCreated)
	assert.Empty(t, first.Header().Get(idempotentReplayedHeader))
	// The browser saves again after the 201 was lost: the same key, the same body.
	repeat := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", body, "entry-1", http.StatusCreated)

	assert.Equal(t, before+1, countRows(t, database, countTransactions), "a repeat writes nothing")
	assert.Equal(t, "true", repeat.Header().Get(idempotentReplayedHeader))
	assert.Equal(t, "application/json", repeat.Header().Get("Content-Type"))
	assert.JSONEq(t, first.Body.String(), repeat.Body.String(), "the repeat returns the stored result")
}

func TestCreateTransactionWithADifferentKeyCreatesASecondRecord(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	before := countRows(t, database, countTransactions)
	body := requestKeyTransactionBody(f, "Two identical coffees")

	first := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", body, "entry-1", http.StatusCreated)
	second := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", body, "entry-2", http.StatusCreated)
	unkeyed := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", body, "", http.StatusCreated)

	assert.Equal(t, before+3, countRows(t, database, countTransactions), "identical content under separate keys is legitimate")
	var a, b, c transactionResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &a))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &b))
	require.NoError(t, json.Unmarshal(unkeyed.Body.Bytes(), &c))
	assert.NotEqual(t, a.ID, b.ID)
	assert.NotEqual(t, b.ID, c.ID)
}

func TestCreateTransactionRepeatIgnoresKeyOrderAndWhitespace(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	before := countRows(t, database, countTransactions)
	body := requestKeyTransactionBody(f, "Reordered")
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &decoded))
	reordered := fmt.Sprintf(`{ "journal_entries" : %s , "description":"Reordered", "transaction_date":"2026-02-01" }`, mustJSON(t, decoded["journal_entries"]))

	sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", body, "entry-1", http.StatusCreated)
	repeat := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", reordered, "entry-1", http.StatusCreated)

	assert.Equal(t, "true", repeat.Header().Get(idempotentReplayedHeader))
	assert.Equal(t, before+1, countRows(t, database, countTransactions))
}

func TestCreateTransactionKeyReusedForADifferentRequestIsRefused(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	before := countRows(t, database, countTransactions)

	sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", requestKeyTransactionBody(f, "First"), "entry-1", http.StatusCreated)
	changed := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", requestKeyTransactionBody(f, "Edited after the lost response"), "entry-1", http.StatusConflict)
	assert.Equal(t, "IDEMPOTENCY_KEY_REUSED", errorCodeOf(t, changed))

	// The same key on another create route is another request too.
	otherRoute := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/investments/dividend", fmt.Sprintf(
		`{"transaction_date":"2026-02-01","cash_account_id":%d,"cash_commodity_id":%d,"income_account_id":%d,"amount_value":"100","amount_scale":2}`,
		f.cashAccount.ID, f.commodityID, f.incomeAccount.ID), "entry-1", http.StatusConflict)
	assert.Equal(t, "IDEMPOTENCY_KEY_REUSED", errorCodeOf(t, otherRoute))

	assert.Equal(t, before+1, countRows(t, database, countTransactions))
}

func TestCreateTransactionFailedAttemptLeavesItsKeyUnused(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	before := countRows(t, database, countTransactions)
	unbalanced := strings.Replace(requestKeyTransactionBody(f, "Unbalanced"), `"-1000"`, `"-999"`, 1)

	sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", unbalanced, "entry-1", http.StatusBadRequest)
	assert.Zero(t, countRows(t, database, `SELECT count(*) FROM request_keys`), "a refused write stores no key")
	// The owner corrects the amount and saves again with the same key.
	sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", requestKeyTransactionBody(f, "Unbalanced"), "entry-1", http.StatusCreated)

	assert.Equal(t, before+1, countRows(t, database, countTransactions))
}

func TestCreateTransactionKeyWithoutAStoredResultIsReportedNotRewritten(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	body := requestKeyTransactionBody(f, "Crash after commit")
	sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", body, "entry-1", http.StatusCreated)
	before := countRows(t, database, countTransactions)
	// The process stopped after the write committed and before its response
	// was stored.
	_, err := database.Exec(`UPDATE request_keys SET response_status = NULL, response_body = NULL`)
	require.NoError(t, err)

	res := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", body, "entry-1", http.StatusConflict)

	assert.Equal(t, "IDEMPOTENCY_KEY_RESULT_UNAVAILABLE", errorCodeOf(t, res))
	assert.Equal(t, before, countRows(t, database, countTransactions))
}

func TestRequestKeyIsReplayedOnlyToTheSignedInOwner(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	body := requestKeyTransactionBody(f, "Private")
	sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", body, "entry-1", http.StatusCreated)

	res := sendKeyed(t, handler, nil, "", "/api/v1/transactions", body, "entry-1", http.StatusUnauthorized)
	assert.NotContains(t, res.Body.String(), "Private")
}

func TestRequestKeyHeaderIsValidatedAndRefusedWhereNotDeclared(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	before := countRows(t, database, countTransactions)
	body := requestKeyTransactionBody(f, "Bad key")

	for name, key := range map[string]string{
		"space":     "entry 1",
		"too long":  strings.Repeat("k", 129),
		"non-ascii": "entrée",
	} {
		res := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", body, key, http.StatusBadRequest)
		assert.Equal(t, "VALIDATION_FAILED", errorCodeOf(t, res), name)
	}
	sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/transactions", body, strings.Repeat("k", 128), http.StatusCreated)

	// A route without request keys refuses the header instead of ignoring it,
	// so a client never believes an unprotected retry is safe.
	res := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/tags", `{"name":"Keyed"}`, "tag-1", http.StatusBadRequest)
	assert.Equal(t, "VALIDATION_FAILED", errorCodeOf(t, res))
	assert.Equal(t, before+1, countRows(t, database, countTransactions))
}

func TestInvestmentCreatesRepeatedWithSameKeyWriteOnce(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapInvestmentAPITest(t, handler)
	instrument := createInstrumentForSession(t, handler, f, "KEYED")
	holding := createHoldingAccountForSession(t, handler, f, instrument.ID)
	before := countRows(t, database, countTransactions)

	buy := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 2000)
	buy.TransactionDate = "2026-01-10"
	buyBody := strings.ReplaceAll(mustJSON(t, buy), `,"reconciliation_override_checkpoint_ids":null`, "")
	sell := tradeRequestBody(f, holding.ID, instrument.CommodityID, "2", 2400)
	sell.TransactionDate = "2026-01-20"
	sellBody := strings.ReplaceAll(mustJSON(t, sell), `,"reconciliation_override_checkpoint_ids":null`, "")
	dividendBody := fmt.Sprintf(`{"transaction_date":"2026-01-15","cash_account_id":%d,"cash_commodity_id":%d,"income_account_id":%d,"commodity_id":%d,"amount_value":"500","amount_scale":2}`,
		f.cashAccount.ID, f.commodityID, f.incomeAccount.ID, instrument.CommodityID)

	for _, write := range []struct{ path, body, key string }{
		{"/api/v1/investments/buy", buyBody, "buy-1"},
		{"/api/v1/investments/dividend", dividendBody, "dividend-1"},
		// The sale disposes every unit, so writing it again would be refused
		// for want of lots; the repeat must be answered before any of that.
		{"/api/v1/investments/sell", sellBody, "sell-1"},
	} {
		first := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, write.path, write.body, write.key, http.StatusCreated)
		repeat := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, write.path, write.body, write.key, http.StatusCreated)
		assert.Equal(t, "true", repeat.Header().Get(idempotentReplayedHeader), write.path)
		assert.JSONEq(t, first.Body.String(), repeat.Body.String(), write.path)
	}

	assert.Equal(t, before+3, countRows(t, database, countTransactions))
	assert.Equal(t, 1, countRows(t, database, fmt.Sprintf(`SELECT count(*) FROM investment_lots WHERE account_id = %d`, holding.ID)))
}

func TestManualPriceRepeatedWithSameKeyWritesOneObservation(t *testing.T) {
	t.Parallel()
	handler, database := newSetupTestHandler(t)
	f := bootstrapPricingAPITest(t, handler)
	body := mustJSON(t, manualPricingInput(f))
	before := countRows(t, database, `SELECT count(*) FROM price_observations`)

	first := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/pricing/manual", body, "price-1", http.StatusCreated)
	repeat := sendKeyed(t, handler, f.sessionCookie, f.csrfToken, "/api/v1/pricing/manual", body, "price-1", http.StatusCreated)

	assert.JSONEq(t, first.Body.String(), repeat.Body.String())
	assert.Equal(t, before+1, countRows(t, database, `SELECT count(*) FROM price_observations`))
}

// The routes that accept a request key are exactly the operations whose
// OpenAPI contract declares the Idempotency-Key parameter.
func TestRequestKeyRoutesMatchTheContract(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "health.go", nil, 0)
	require.NoError(t, err)
	var registered []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "HandleFunc" || len(call.Args) != 2 {
			return true
		}
		wrapped, ok := call.Args[1].(*ast.CallExpr)
		if !ok {
			return true
		}
		if name, ok := wrapped.Fun.(*ast.Ident); ok && name.Name == "withRequestKey" {
			pattern, err := strconv.Unquote(call.Args[0].(*ast.BasicLit).Value)
			require.NoError(t, err)
			registered = append(registered, pattern)
		}
		return true
	})

	root := "../../../api/openapi"
	spec := readBodyContractYAML(t, root+"/openapi.yaml")
	var declared []string
	for path, value := range spec["paths"].(map[string]any) {
		pathFile := filepath.Join(root, value.(map[string]any)["$ref"].(string))
		for method, value := range readBodyContractYAML(t, pathFile) {
			operation, ok := value.(map[string]any)
			if !ok {
				continue
			}
			parameters, _ := operation["parameters"].([]any)
			for _, parameter := range parameters {
				reference, _ := parameter.(map[string]any)["$ref"].(string)
				if strings.HasSuffix(reference, "#/components/parameters/IdempotencyKey") {
					declared = append(declared, strings.ToUpper(method)+" "+path)
					_, has409 := operation["responses"].(map[string]any)["409"]
					assert.True(t, has409, "%s %s must document the request key conflicts", method, path)
				}
			}
		}
	}
	sort.Strings(registered)
	sort.Strings(declared)
	require.Len(t, registered, 19)
	assert.Equal(t, registered, declared)
}
