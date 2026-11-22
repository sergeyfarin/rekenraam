package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagDirectoryUsageMatchesFilteredTransactions(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, currency := setupAccountAPITest(t, handler)
	cash := createLedgerAccount(t, handler, cookie, csrf, "Tag checking", "asset", "checking", currency, 2)
	expense := createCategoryForSession(t, handler, cookie, csrf, `{"name":"Tag expense","category_type":"expense"}`)
	tag := createTagForSession(t, handler, cookie, csrf, `{"name":"Trip 50%","kind":"project","color":"#3B82F6","icon":"sun","metadata":{"reference":9007199254740993}}`)
	unused := createTagForSession(t, handler, cookie, csrf, `{"name":"Trip 50x","kind":"project"}`)

	postingTags := fmt.Sprintf(`,"tag_ids":[%d]`, tag.ID)
	body := func(description, transactionTagField, postingTagField string) string {
		return fmt.Sprintf(`{"transaction_date":"2026-06-07","description":%q%s,
			"journal_entries":[{"entry_date":"2026-06-07","postings":[
				{"account_id":%d,"quantity_value":"-1000","quantity_scale":2,"commodity_id":%d%s},
				{"account_id":%d,"quantity_value":"1000","quantity_scale":2,"commodity_id":%d%s}
			]}]}`, description, transactionTagField, cash.ID, currency, postingTagField,
			expense.ID, currency, postingTagField)
	}
	taggedTwice := createTransactionForSession(t, handler, cookie, csrf, body("Both levels", postingTags, postingTags), http.StatusCreated)
	postingOnly := createTransactionForSession(t, handler, cookie, csrf, body("Posting only", "", postingTags), http.StatusCreated)
	transactionOnly := createTransactionForSession(t, handler, cookie, csrf, body("Transaction only", postingTags, ""), http.StatusCreated)
	removed := createTransactionForSession(t, handler, cookie, csrf, body("Removed posting tags", "", postingTags), http.StatusCreated)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPatch, fmt.Sprintf("/api/v1/transactions/%d", removed.ID),
		body("Removed posting tags", "", ""), http.StatusOK)
	deleted := createTransactionForSession(t, handler, cookie, csrf, body("Deleted", postingTags, postingTags), http.StatusCreated)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, fmt.Sprintf("/api/v1/transactions/%d/soft-delete", deleted.ID),
		`{"change_reason":"duplicate"}`, http.StatusOK)
	mutateTransaction(t, handler, cookie, csrf, http.MethodPost, fmt.Sprintf("/api/v1/transactions/%d/void", transactionOnly.ID),
		`{"change_reason":"duplicate"}`, http.StatusOK)

	tagPath := fmt.Sprintf("/api/v1/tags/%d", tag.ID)
	rename := tokenAPIRequest(handler, http.MethodPatch, tagPath, `{"name":"Travel 50%"}`, cookie, csrf, "")
	require.Equal(t, http.StatusOK, rename.Code, rename.Body.String())
	var renamed tagResponse
	require.NoError(t, json.Unmarshal(rename.Body.Bytes(), &renamed))
	assert.Equal(t, "Travel 50%", renamed.Name)
	assert.Equal(t, tag.Kind, renamed.Kind)
	assert.Equal(t, tag.Color, renamed.Color)
	assert.Equal(t, tag.Icon, renamed.Icon)
	assert.Equal(t, string(tag.Metadata), string(renamed.Metadata))

	mutateTag(t, handler, cookie, csrf, http.MethodPost, tagPath+"/archive", http.StatusOK)
	filter := fmt.Sprintf("?tag_id=%d", tag.ID)
	filtered := listTransactionsForSession(t, handler, cookie, filter)
	ids := make([]int64, 0, len(filtered.Transactions))
	for _, tx := range filtered.Transactions {
		ids = append(ids, tx.ID)
	}
	assert.ElementsMatch(t, []int64{taggedTwice.ID, postingOnly.ID, transactionOnly.ID}, ids)
	posted := listTransactionsForSession(t, handler, cookie, filter+"&status=posted&q=Posting")
	require.Len(t, posted.Transactions, 1)
	assert.Equal(t, postingOnly.ID, posted.Transactions[0].ID)
	assert.Empty(t, listTransactionsForSession(t, handler, cookie, fmt.Sprintf("?tag_id=%d", unused.ID)).Transactions)

	read := func(query string) tagDirectoryResponse {
		t.Helper()
		res := tokenAPIRequest(handler, http.MethodGet, "/api/v1/tags/directory"+query, "", cookie, "", "")
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		var result tagDirectoryResponse
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
		return result
	}
	assert.Equal(t, []tagDirectoryRow{{ID: unused.ID, Name: unused.Name, Kind: unused.Kind, Status: "active"}}, read("").Tags)
	assert.Equal(t, []tagDirectoryRow{{ID: tag.ID, Name: renamed.Name, Kind: renamed.Kind, Status: "archived", TransactionCount: 3}},
		read("?status=all&q="+url.QueryEscape("50%")).Tags)
	assert.Equal(t, read("?status=archived").Tags, read("?status=all&q=travel").Tags)

	res := tokenAPIRequest(handler, http.MethodGet, "/api/v1/transactions?tag_id=-1", "", cookie, "", "")
	require.Equal(t, http.StatusBadRequest, res.Code, res.Body.String())
}

func TestTagDirectoryCursorContinuesAcrossEqualNames(t *testing.T) {
	t.Parallel()

	handler, _ := newSetupTestHandler(t)
	cookie, csrf, _ := setupAccountAPITest(t, handler)
	for i := range 49 {
		createTagForSession(t, handler, cookie, csrf, fmt.Sprintf(`{"name":"A %02d"}`, i))
	}
	first := createTagForSession(t, handler, cookie, csrf, `{"name":"Last: Name","kind":"project"}`)
	second := createTagForSession(t, handler, cookie, csrf, `{"name":"LAST: NAME","kind":"person"}`)

	path := "/api/v1/tags/directory"
	read := func(query string) tagDirectoryResponse {
		t.Helper()
		res := tokenAPIRequest(handler, http.MethodGet, path+query, "", cookie, "", "")
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		var result tagDirectoryResponse
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
		return result
	}
	page := read("")
	require.Len(t, page.Tags, 50)
	assert.Equal(t, first.ID, page.Tags[49].ID)
	require.NotNil(t, page.NextCursor)
	next := read("?cursor=" + url.QueryEscape(*page.NextCursor))
	require.Len(t, next.Tags, 1)
	assert.Equal(t, second.ID, next.Tags[0].ID)
	assert.Nil(t, next.NextCursor)

	res := tokenAPIRequest(handler, http.MethodGet, path+"?cursor=invalid", "", cookie, "", "")
	require.Equal(t, http.StatusBadRequest, res.Code, res.Body.String())
}
