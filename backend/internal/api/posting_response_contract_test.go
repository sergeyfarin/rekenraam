package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostingResponseContractEmptyAndNonemptyTags(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, csrf, commodity := setupAccountAPITest(t, handler)
	checking := createLedgerAccount(t, handler, cookie, csrf, "Contract checking", "asset", "checking", commodity, 2)
	expense := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Contract expense","category_type":"expense"}`)
	tag := createTagForSession(t, handler, cookie, csrf, `{"name":"Contract tag","kind":"custom"}`)
	transaction := createTransactionForSession(t, handler, cookie, csrf, `{
 "transaction_date":"2026-06-07","journal_entries":[{"entry_date":"2026-06-07","postings":[
 {"account_id":`+strconvFormatInt(checking.ID)+`,"commodity_id":`+strconvFormatInt(commodity)+`,"quantity_value":"-123","quantity_scale":2},
 {"account_id":`+strconvFormatInt(expense.ID)+`,"commodity_id":`+strconvFormatInt(commodity)+`,"quantity_value":"123","quantity_scale":2,"tag_ids":[`+strconvFormatInt(tag.ID)+`]}
 ]}]}`, http.StatusCreated)
	read := readTransactionForSession(t, handler, cookie, transaction.ID, http.StatusOK)
	for _, response := range []transactionResponse{transaction, read} {
		require.NotNil(t, response.JournalEntries[0].Postings[0].TagIDs, "empty tags must encode as [] rather than null")
		require.Empty(t, response.JournalEntries[0].Postings[0].TagIDs)
		require.Equal(t, []int64{tag.ID}, response.JournalEntries[0].Postings[1].TagIDs)
	}
	checkingRegister := accountRegisterForSession(t, handler, cookie, checking.ID, "")
	require.Len(t, checkingRegister.Entries, 1)
	require.NotNil(t, checkingRegister.Entries[0].TransactionTagIDs, "register empty transaction tags must encode as []")
	require.Empty(t, checkingRegister.Entries[0].TransactionTagIDs)
	require.NotNil(t, checkingRegister.Entries[0].Posting.TagIDs, "register empty tags must encode as []")
	require.Empty(t, checkingRegister.Entries[0].Posting.TagIDs)
	expenseRegister := accountRegisterForSession(t, handler, cookie, expense.ID, "")
	require.Len(t, expenseRegister.Entries, 1)
	require.Equal(t, []int64{tag.ID}, expenseRegister.Entries[0].Posting.TagIDs)
}

func TestSpendingResponseContractNamesActualExcludedSystemRoles(t *testing.T) {
	t.Parallel()
	handler, _ := newSetupTestHandler(t)
	cookie, _, _ := setupAccountAPITest(t, handler)
	request := forecastRequest(t, handler, cookie, "/api/v1/reports/spending?start_date=2026-06-01&end_date=2026-06-30&group_by=category&mode=spending", http.StatusOK)
	var body struct {
		ExcludedSystemRoles []string `json:"excluded_system_roles"`
	}
	require.NoError(t, json.Unmarshal(request.Body.Bytes(), &body))
	require.ElementsMatch(t, []string{"opening_balance", "import_imbalance", "retained_earnings", "unassigned_income", "unassigned_expense", "transfer_clearing", "commodity_trading", "external_investment_transfer_equity"}, body.ExcludedSystemRoles)
}
