package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionCreateListRegisterVoidAndPayeeSnapshot(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	groceries := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Groceries",
		"category_type":"expense"
	}`)
	payee := createPayeeForSession(t, handler, sessionCookie, csrfToken, `{"name":"Albert Heijn"}`)

	transaction := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"payee_id":`+strconvFormatInt(payee.ID)+`,
		"description":"Weekly groceries",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(groceries.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusCreated)

	assert.Equal(t, "posted", transaction.Status)
	assert.Equal(t, "Albert Heijn", transaction.PayeeName)
	require.Len(t, transaction.JournalEntries, 1)
	require.Len(t, transaction.JournalEntries[0].Postings, 2)
	assert.NotEmpty(t, transaction.JournalEntries[0].Postings[0].LineKey)

	list := listTransactionsForSession(t, handler, sessionCookie, "?q=Albert")
	require.Len(t, list.Transactions, 1)
	assert.Equal(t, transaction.ID, list.Transactions[0].ID)

	register := accountRegisterForSession(t, handler, sessionCookie, checking.ID, "?after_date=2026-06-01&before_date=2026-06-30")
	require.Len(t, register.Transactions, 1)
	assert.Equal(t, transaction.ID, register.Transactions[0].ID)

	mutatePayee(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/payees/"+strconvFormatInt(payee.ID)+"/archive", `{"change_reason":"not used anymore"}`, http.StatusOK)
	read := readTransactionForSession(t, handler, sessionCookie, transaction.ID, http.StatusOK)
	assert.Equal(t, "Albert Heijn", read.PayeeName)

	voided := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(transaction.ID)+"/void", `{"change_reason":"duplicate import"}`, http.StatusOK)
	assert.Equal(t, "voided", voided.Status)
	assert.Equal(t, "duplicate import", voided.ChangeReason)

	postedList := listTransactionsForSession(t, handler, sessionCookie, "?status=posted")
	assert.Empty(t, postedList.Transactions)
}

func TestTransactionValidationAndLifecycleGuards(t *testing.T) {
	t.Parallel()

	handler, database := newSetupTestHandler(t)
	sessionCookie, csrfToken, commodityID := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", commodityID, 2)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Other",
		"category_type":"expense"
	}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":9000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-100001,"quantity_scale":3,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":100001,"quantity_scale":3,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	tag := createTagForSession(t, handler, sessionCookie, csrfToken, `{"name":"Vacation","kind":"project"}`)
	mutateTag(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/tags/"+strconvFormatInt(tag.ID)+"/archive", http.StatusOK)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"tag_ids":[`+strconvFormatInt(tag.ID)+`],
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusConflict)

	draft := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"status":"draft",
		"transaction_date":"2026-06-07"
	}`, http.StatusCreated)
	mutateTransactionNoBody(t, handler, sessionCookie, csrfToken, http.MethodDelete, "/api/v1/transactions/"+strconvFormatInt(draft.ID), http.StatusNoContent)
	readTransactionForSession(t, handler, sessionCookie, draft.ID, http.StatusNotFound)

	reconciled := createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`,"reconciliation_status":"reconciled"},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":10000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusCreated)
	mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPatch, "/api/v1/transactions/"+strconvFormatInt(reconciled.ID), `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"line_key":"`+reconciled.JournalEntries[0].Postings[0].LineKey+`","account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-11000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"line_key":"`+reconciled.JournalEntries[0].Postings[1].LineKey+`","account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":11000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusConflict)

	var versionCount int
	require.NoError(t, database.QueryRow(`
		SELECT COUNT(*)
		FROM transaction_versions
		WHERE transaction_id = ?
	`, reconciled.ID).Scan(&versionCount))
	assert.Equal(t, 1, versionCount)

	correction := mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions/"+strconvFormatInt(reconciled.ID)+"/correct", `{
		"transaction_date":"2026-06-08",
		"transaction_kind":"adjustment",
		"journal_entries":[{
			"entry_date":"2026-06-08",
			"entry_kind":"adjustment",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(commodityID)+`}
			]
		}]
	}`, http.StatusCreated)
	require.NotNil(t, correction.CorrectionOfTransactionID)
	assert.Equal(t, reconciled.ID, *correction.CorrectionOfTransactionID)
}

func TestTransactionPostingAccountDateAndCommodityValidation(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	eur := createCurrencyForSession(t, handler, sessionCookie, csrfToken, `{"code":"EUR","name":"Euro"}`)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"Validation Expense",
		"category_type":"expense"
	}`)
	laterAccount := createLedgerAccountOpenedOn(t, handler, sessionCookie, csrfToken, "Later Checking", "asset", "checking", usdID, 2, "2026-06-06")

	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-05",
		"journal_entries":[{
			"entry_date":"2026-06-05",
			"postings":[
				{"account_id":`+strconvFormatInt(laterAccount.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	closedAccount := createLedgerAccountOpenedOn(t, handler, sessionCookie, csrfToken, "Closed Checking", "asset", "checking", usdID, 2, "2026-01-01")
	mutateAccountWithBody(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/accounts/"+strconvFormatInt(closedAccount.ID)+"/close", `{
		"closed_on":"2026-06-06",
		"effective_from":"2026-06-06",
		"change_reason":"closed for test"
	}`, http.StatusOK)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(closedAccount.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "USD Checking", "asset", "checking", usdID, 2)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(eur.ID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(eur.ID)+`}
			]
		}]
	}`, http.StatusBadRequest)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"postings":[
				{"account_id":`+strconvFormatInt(checking.ID)+`,"quantity_value":-1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(usdID)+`},
				{"account_id":`+strconvFormatInt(expense.ID)+`,"quantity_value":1000,"quantity_scale":2,"commodity_id":`+strconvFormatInt(eur.ID)+`}
			]
		}]
	}`, http.StatusBadRequest)
}

func TestTransactionCommonLedgerScenarios(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	sessionCookie, csrfToken, usdID := setupAccountAPITest(t, handler)
	eur := createCurrencyForSession(t, handler, sessionCookie, csrfToken, `{"code":"EUR","name":"Euro"}`)
	mutateAccount(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/setup/system-accounts", http.StatusCreated)
	systemAccounts := listAccountsForSession(t, handler, sessionCookie, "?include_system=true")
	openingBalance := accountBySystemRole(t, systemAccounts.Accounts, "opening_balance")
	transferClearing := accountBySystemRole(t, systemAccounts.Accounts, "transfer_clearing")
	commodityTrading := accountBySystemRole(t, systemAccounts.Accounts, "commodity_trading")

	checking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Checking", "asset", "checking", usdID, 2)
	savings := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Savings", "asset", "savings", usdID, 2)
	card := createLedgerAccount(t, handler, sessionCookie, csrfToken, "Credit Card", "liability", "credit_card", usdID, 2)
	eurChecking := createLedgerAccount(t, handler, sessionCookie, csrfToken, "EUR Checking", "asset", "checking", eur.ID, 2)
	receivable := createLedgerAccountNoCommodity(t, handler, sessionCookie, csrfToken, "Receivable", "asset", "receivable")
	payable := createLedgerAccountNoCommodity(t, handler, sessionCookie, csrfToken, "Payable", "liability", "payable")
	salary := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Salary","category_type":"income"}`)
	expense := createCategoryForSession(t, handler, sessionCookie, csrfToken, `{"name":"Scenario Expense","category_type":"expense"}`)

	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, 100000, 2, usdID),
		posting(salary.ID, -100000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(card.ID, -5000, 2, usdID),
		posting(expense.ID, 5000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -5000, 2, usdID),
		posting(card.ID, 5000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -10000, 2, usdID),
		posting(savings.ID, 10000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"transfer",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"transfer_leg",
			"postings":[
				`+posting(checking.ID, -10000, 2, usdID)+`,
				`+posting(transferClearing.ID, 10000, 2, usdID)+`
			]
		},{
			"entry_date":"2026-06-08",
			"entry_kind":"transfer_leg",
			"postings":[
				`+posting(transferClearing.ID, -10000, 2, usdID)+`,
				`+posting(savings.ID, 10000, 2, usdID)+`
			]
		}]
	}`, http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"transfer",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"exchange",
			"postings":[
				`+posting(checking.ID, -108000, 2, usdID)+`,
				`+posting(commodityTrading.ID, 108000, 2, usdID)+`,
				`+posting(eurChecking.ID, 100000, 2, eur.ID)+`,
				`+posting(commodityTrading.ID, -100000, 2, eur.ID)+`
			]
		}]
	}`, http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(checking.ID, -8000, 2, usdID),
		posting(receivable.ID, 8000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(expense.ID, 4000, 2, usdID),
		posting(payable.ID, -4000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, balancedBody("2026-06-07",
		posting(receivable.ID, 4000, 2, usdID),
		posting(expense.ID, -4000, 2, usdID),
	), http.StatusCreated)
	createTransactionForSession(t, handler, sessionCookie, csrfToken, `{
		"transaction_date":"2026-06-07",
		"transaction_kind":"opening_balance",
		"journal_entries":[{
			"entry_date":"2026-06-07",
			"entry_kind":"opening_balance",
			"postings":[
				`+posting(checking.ID, 50000, 2, usdID)+`,
				`+posting(openingBalance.ID, -50000, 2, usdID)+`
			]
		}]
	}`, http.StatusCreated)
}

func createLedgerAccount(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, name string, accountClass string, accountKind string, commodityID int64, scaleOverride int) accountResponse {
	t.Helper()

	return createLedgerAccountOpenedOn(t, handler, sessionCookie, csrfToken, name, accountClass, accountKind, commodityID, scaleOverride, "2026-01-01")
}

func createLedgerAccountOpenedOn(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, name string, accountClass string, accountKind string, commodityID int64, scaleOverride int, openedOn string) accountResponse {
	t.Helper()

	return createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"`+name+`",
		"account_class":"`+accountClass+`",
		"account_kind":"`+accountKind+`",
		"default_commodity_id":`+strconvFormatInt(commodityID)+`,
		"quantity_scale_override":`+strconv.Itoa(scaleOverride)+`,
		"allows_postings":true,
		"opened_on":"`+openedOn+`",
		"effective_from":"`+openedOn+`"
	}`)
}

func createLedgerAccountNoCommodity(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, name string, accountClass string, accountKind string) accountResponse {
	t.Helper()

	return createAccountForSession(t, handler, sessionCookie, csrfToken, `{
		"name":"`+name+`",
		"account_class":"`+accountClass+`",
		"account_kind":"`+accountKind+`",
		"allows_postings":true,
		"opened_on":"2026-01-01",
		"effective_from":"2026-01-01"
	}`)
}

func createCurrencyForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string) currencyResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/currencies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusCreated, res.Code)

	var response currencyResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func accountBySystemRole(t *testing.T, accounts []accountResponse, role string) accountResponse {
	t.Helper()

	for _, account := range accounts {
		if account.SystemRole == role {
			return account
		}
	}
	require.Failf(t, "system account not found", "role %s", role)
	return accountResponse{}
}

func posting(accountID int64, quantityValue int64, quantityScale int, commodityID int64) string {
	return `{"account_id":` + strconvFormatInt(accountID) + `,"quantity_value":` + strconv.FormatInt(quantityValue, 10) + `,"quantity_scale":` + strconv.Itoa(quantityScale) + `,"commodity_id":` + strconvFormatInt(commodityID) + `}`
}

func balancedBody(transactionDate string, postings ...string) string {
	return `{
		"transaction_date":"` + transactionDate + `",
		"journal_entries":[{
			"entry_date":"` + transactionDate + `",
			"postings":[` + strings.Join(postings, ",") + `]
		}]
	}`
}

func createPayeeForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string) payeeResponse {
	t.Helper()
	return mutatePayee(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/payees", body, http.StatusCreated)
}

func mutatePayee(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, body string, wantStatus int) payeeResponse {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response payeeResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func createTransactionForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string, wantStatus int) transactionResponse {
	t.Helper()
	return mutateTransaction(t, handler, sessionCookie, csrfToken, http.MethodPost, "/api/v1/transactions", body, wantStatus)
}

func mutateTransaction(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, body string, wantStatus int) transactionResponse {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response transactionResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func mutateTransactionNoBody(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, method string, path string, wantStatus int) {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(csrfTokenHeader, csrfToken)
	setSameOrigin(req)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)
}

func readTransactionForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, transactionID int64, wantStatus int) transactionResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/"+strconvFormatInt(transactionID), nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, wantStatus, res.Code)

	var response transactionResponse
	if wantStatus >= 200 && wantStatus < 300 {
		require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	}
	return response
}

func listTransactionsForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, suffix string) transactionsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response transactionsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}

func accountRegisterForSession(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, accountID int64, suffix string) transactionsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/"+strconvFormatInt(accountID)+"/register"+suffix, nil)
	req.AddCookie(sessionCookie)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var response transactionsResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&response))
	return response
}
